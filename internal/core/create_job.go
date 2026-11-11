package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vicgupta/dap/internal/agentrun"
	"github.com/vicgupta/dap/internal/id"
	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginauthor"
	"github.com/vicgupta/dap/internal/rundir"
)

const (
	createRunning   = "running"
	createReady     = "ready"
	createInstalled = "installed"
	createFailed    = "failed"
	createCancelled = "cancelled"

	markerDraftMessage       = "The starter plugin is still in the draft. The agent did not replace it, so DAP did not propose an install."
	unchangedManifestMessage = "The manifest is still the starter's, so DAP did not propose an install."
	unnamedDraftMessage      = "The agent did not choose a short plugin name, so nothing was proposed."
	agentFailedMessage       = "The coding agent stopped with an error, so DAP did not propose an install."
	cancelledMessage         = "Cancelled. Nothing was proposed."
	validationFailedMessage  = "Validation did not pass, so DAP did not propose an install."
	restartMessage           = "DAP restarted while this job was running. Nothing was installed. Retry to run the agent again."
	preapprovedReason        = "pre-approved at create"
	starterDescription       = "Starter plugin. Replace this manifest with the plugin you were asked to build."
)

// validatePlugin is the single conformance pass for a create job, including
// the "Checking the plugin" step. It is ValidateIn, which starts the stub API.
// Tests replace it to count calls. dataDir keeps npm and temp files in rundir.
var validatePlugin = pluginauthor.ValidateIn

// CreateRequest is the Create plugin form.
type CreateRequest struct {
	Description string         `json:"description"`
	Agent       string         `json:"agent"`
	Name        string         `json:"name,omitempty"`
	AutoInstall bool           `json:"autoInstall"`
	Template    string         `json:"template,omitempty"`
	Settings    map[string]any `json:"settings,omitempty"`
	// Plugin edits an installed plugin. Auto-install does not apply to edits.
	Plugin string `json:"plugin,omitempty"`
}

// CreateView is what GET /api/create/{id} returns.
type CreateView struct {
	OK           bool                 `json:"ok"`
	ID           string               `json:"id,omitempty"`
	Status       string               `json:"status,omitempty"`
	Step         string               `json:"step,omitempty"`
	Log          string               `json:"log,omitempty"`
	Card         *model.Card          `json:"card,omitempty"`
	FailedChecks []pluginauthor.Check `json:"failedChecks,omitempty"`
	Error        string               `json:"error,omitempty"`
	Name         string               `json:"name,omitempty"`
	Path         string               `json:"path,omitempty"`
	AutoInstall  bool                 `json:"autoInstall"`
	Template     string               `json:"template,omitempty"`
	Mode         string               `json:"mode,omitempty"`
	Description  string               `json:"description,omitempty"`
	Agent        string               `json:"agent,omitempty"`
}

type createRecord struct {
	ID            string               `json:"id"`
	Status        string               `json:"status"`
	Step          string               `json:"step"`
	Log           string               `json:"log,omitempty"`
	Card          *model.Card          `json:"card,omitempty"`
	FailedChecks  []pluginauthor.Check `json:"failedChecks,omitempty"`
	Error         string               `json:"error,omitempty"`
	Name          string               `json:"name,omitempty"`
	Path          string               `json:"path,omitempty"`
	Description   string               `json:"description,omitempty"`
	Agent         string               `json:"agent,omitempty"`
	RequestedName string               `json:"requestedName,omitempty"`
	Template      string               `json:"template,omitempty"`
	Mode          string               `json:"mode,omitempty"`
	Plugin        string               `json:"plugin,omitempty"`
	AutoInstall   bool                 `json:"autoInstall"`
	Settings      map[string]any       `json:"settings,omitempty"`
	Starter       string               `json:"starter,omitempty"`
	Provisional   bool                 `json:"provisional"`
	Dismissed     bool                 `json:"dismissed"`
	UpdatedAt     time.Time            `json:"updatedAt"`
}

type createRuntime struct {
	rec    createRecord
	cancel context.CancelFunc
	done   chan struct{}
}

// CreatePlugin scaffolds, runs the agent, validates once, and proposes or installs.
func (a *App) CreatePlugin(req CreateRequest) (CreateView, error) {
	desc := strings.TrimSpace(req.Description)
	mode := "blank"
	var tmpl pluginauthor.Template
	if strings.TrimSpace(req.Template) != "" {
		t, err := pluginauthor.Lookup(strings.TrimSpace(req.Template))
		if err != nil {
			return CreateView{}, err
		}
		tmpl = t
		mode = t.Mode
		if desc == "" {
			desc = strings.TrimSpace(t.Description)
		}
		if desc == "" {
			desc = t.Blurb
		}
	}
	path := ""
	if strings.TrimSpace(req.Plugin) != "" {
		snap := a.store.Snapshot()
		rec := snap.Plugins[req.Plugin]
		if rec == nil {
			return CreateView{}, fmt.Errorf("%w: plugin %s", ErrNotFound, req.Plugin)
		}
		mode = "edit"
		path = rec.Dir
	}
	if mode != pluginauthor.ModeInstall && desc == "" {
		return CreateView{}, fmt.Errorf("describe what the plugin should do")
	}
	agent := strings.TrimSpace(req.Agent)
	if mode != pluginauthor.ModeInstall {
		if agent == "" {
			return CreateView{}, fmt.Errorf("choose a coding agent")
		}
		if _, _, err := agentrun.Find(agent); err != nil {
			return CreateView{}, err
		}
	}
	requested := ""
	if strings.TrimSpace(req.Name) != "" && mode != "edit" && mode != pluginauthor.ModeInstall {
		n, err := normalizePluginName(req.Name)
		if err != nil {
			return CreateView{}, fmt.Errorf("name: %w", err)
		}
		requested = n
	}
	now := time.Now().UTC()
	rec := createRecord{
		ID:            id.New("job"),
		Status:        createRunning,
		Step:          "scaffold",
		Description:   desc,
		Agent:         agent,
		RequestedName: requested,
		Template:      tmpl.ID,
		Mode:          mode,
		Plugin:        strings.TrimSpace(req.Plugin),
		AutoInstall:   req.AutoInstall,
		Settings:      req.Settings,
		Path:          path,
		Provisional:   mode == "blank" && requested == "",
		UpdatedAt:     now,
		Log:           "Started.\n",
	}
	if mode == "edit" {
		rec.Step = "agent"
		rec.Name = req.Plugin
	}
	ctx, cancel := context.WithCancel(context.Background())
	rt := &createRuntime{rec: rec, cancel: cancel, done: make(chan struct{})}
	a.jobs().Store(rec.ID, rt)
	a.writeCreate(rec)
	go a.runCreate(rt, ctx, "", false)
	return a.createView(rt), nil
}

// CreateJob returns one persisted job. The log is a tail.
func (a *App) CreateJob(id string) (CreateView, error) {
	rt, err := a.createByID(id)
	if err != nil {
		return CreateView{}, err
	}
	return a.createView(rt), nil
}

// CurrentCreate returns the newest job that has not been dismissed.
func (a *App) CurrentCreate() (CreateView, bool) {
	var best *createRuntime
	var bestAt time.Time
	a.jobs().Range(func(_, v any) bool {
		rt := v.(*createRuntime)
		a.mu.Lock()
		rec := rt.rec
		a.mu.Unlock()
		if rec.Dismissed || rec.ID == "" {
			return true
		}
		if best == nil || rec.UpdatedAt.After(bestAt) {
			best = rt
			bestAt = rec.UpdatedAt
		}
		return true
	})
	if best == nil {
		return CreateView{OK: true}, false
	}
	return a.createView(best), true
}

// RetryCreate runs the agent again in the same draft. Failed checks go into the prompt.
func (a *App) RetryCreate(id string) (CreateView, error) {
	rt, err := a.createByID(id)
	if err != nil {
		return CreateView{}, err
	}
	a.mu.Lock()
	if rt.rec.Status == createRunning {
		a.mu.Unlock()
		return CreateView{}, fmt.Errorf("this create is still running")
	}
	fix := checksReport(rt.rec.FailedChecks, rt.rec.Error)
	mode := rt.rec.Mode
	agent := rt.rec.Agent
	path := rt.rec.Path
	a.mu.Unlock()
	if agent == "" && (mode != pluginauthor.ModeInstall || fix != "") {
		return CreateView{}, fmt.Errorf("choose a coding agent")
	}
	if agent != "" {
		if _, _, err := agentrun.Find(agent); err != nil {
			return CreateView{}, err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	rt.cancel = cancel
	rt.done = make(chan struct{})
	rt.rec.Status = createRunning
	rt.rec.Step = "agent"
	if path == "" {
		rt.rec.Step = "scaffold"
	}
	rt.rec.Error = ""
	rt.rec.FailedChecks = nil
	rt.rec.Card = nil
	rt.rec.Dismissed = false
	rt.rec.UpdatedAt = time.Now().UTC()
	if rt.rec.Log != "" && !strings.HasSuffix(rt.rec.Log, "\n") {
		rt.rec.Log += "\n"
	}
	rt.rec.Log += "--- retry ---\n"
	rec := rt.rec
	a.mu.Unlock()
	a.writeCreate(rec)
	go a.runCreate(rt, ctx, fix, path != "")
	return a.createView(rt), nil
}

// CancelCreate stops the agent. A finished job is left as it is.
func (a *App) CancelCreate(id string) error {
	rt, err := a.createByID(id)
	if err != nil {
		return err
	}
	if rt.cancel != nil {
		rt.cancel()
	}
	if rt.done != nil {
		select {
		case <-rt.done:
		case <-time.After(3 * time.Second):
		}
	}
	return nil
}

// DismissCreate hides a job from the Create page. A running job is cancelled.
func (a *App) DismissCreate(id string) error {
	rt, err := a.createByID(id)
	if err != nil {
		return err
	}
	a.mu.Lock()
	running := rt.rec.Status == createRunning
	a.mu.Unlock()
	if running && rt.cancel != nil {
		rt.cancel()
		if rt.done != nil {
			select {
			case <-rt.done:
			case <-time.After(3 * time.Second):
			}
		}
	}
	a.setCreate(rt, func(rec *createRecord) { rec.Dismissed = true })
	return nil
}

func (a *App) runCreate(rt *createRuntime, ctx context.Context, fix string, resume bool) {
	defer close(rt.done)
	defer func() {
		a.mu.Lock()
		running := rt.rec.Status == createRunning
		a.mu.Unlock()
		if ctx.Err() != nil && running {
			a.setCreate(rt, func(rec *createRecord) {
				rec.Status = createCancelled
				rec.Step = "done"
				rec.Error = cancelledMessage
			})
		}
	}()

	if !resume && rt.rec.Mode != "edit" {
		a.setCreate(rt, func(rec *createRecord) { rec.Step = "scaffold" })
		if err := a.scaffoldCreate(rt); err != nil {
			a.failCreate(rt, err.Error(), nil)
			return
		}
	}
	if ctx.Err() != nil {
		return
	}

	a.mu.Lock()
	mode := rt.rec.Mode
	agentName := rt.rec.Agent
	a.mu.Unlock()
	runAgent := mode != pluginauthor.ModeInstall || strings.TrimSpace(fix) != ""
	if runAgent {
		if strings.TrimSpace(agentName) == "" {
			a.failCreate(rt, "choose a coding agent", nil)
			return
		}
		a.setCreate(rt, func(rec *createRecord) { rec.Step = "agent" })
		err := a.runCreateAgent(ctx, rt, fix)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			a.appendCreateLog(rt, err.Error())
			a.failCreate(rt, agentFailedMessage, nil)
			return
		}
		if mode != pluginauthor.ModeInstall && mode != "edit" {
			a.mu.Lock()
			dir := rt.rec.Path
			starter := rt.rec.Starter
			a.mu.Unlock()
			checks := starterChecks(dir, starter)
			if len(checks) > 0 {
				a.failCreate(rt, checks[0].Detail, checks)
				return
			}
		}
	}
	if ctx.Err() != nil {
		return
	}
	a.finishCreate(ctx, rt)
}

func (a *App) scaffoldCreate(rt *createRuntime) error {
	hexID := strings.TrimPrefix(rt.rec.ID, "job_")
	folder := "draft-" + hexID
	dir := filepath.Join(a.DataDir, "drafts", folder)
	if rt.rec.Template != "" {
		if err := pluginauthor.CopyTemplate(rt.rec.Template, dir); err != nil {
			return err
		}
	} else {
		name := folder
		if rt.rec.RequestedName != "" {
			name = rt.rec.RequestedName
		}
		path, err := pluginauthor.Scaffold(pluginauthor.Options{
			Name:        name,
			Type:        "watcher",
			Lang:        "ts",
			Description: starterDescription,
			Dir:         dir,
		})
		if err != nil {
			return err
		}
		dir = path
	}
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		return err
	}
	a.setCreate(rt, func(rec *createRecord) {
		rec.Path = dir
		rec.Starter = string(raw)
		if rec.Name == "" {
			rec.Name = folder
		}
		rec.Log = appendLog(rec.Log, "Prepared the draft.")
	})
	return nil
}

func (a *App) runCreateAgent(ctx context.Context, rt *createRuntime, fix string) error {
	a.mu.Lock()
	prompt := agentrun.Prompt(agentrun.PromptInput{
		Description: rt.rec.Description,
		FixReport:   fix,
		Name:        rt.rec.RequestedName,
		DirName:     filepath.Base(rt.rec.Path),
	})
	agent := rt.rec.Agent
	dir := rt.rec.Path
	a.mu.Unlock()
	return agentrun.Run(ctx, agentrun.Request{
		Agent:   agent,
		Dir:     dir,
		DataDir: a.DataDir,
		Prompt:  prompt,
		OnLine:  func(line string) { a.appendCreateLog(rt, line) },
	})
}

func (a *App) finishCreate(ctx context.Context, rt *createRuntime) {
	if ctx.Err() != nil {
		return
	}
	a.setCreate(rt, func(rec *createRecord) {
		rec.Step = "check"
		rec.Log = appendLog(rec.Log, "Checking the plugin.")
	})
	a.mu.Lock()
	path := rt.rec.Path
	settings := rt.rec.Settings
	mode := rt.rec.Mode
	provisional := rt.rec.Provisional
	auto := rt.rec.AutoInstall
	a.mu.Unlock()

	rep := validatePlugin(path, a.DataDir)
	if !rep.OK {
		var failed []pluginauthor.Check
		for _, c := range rep.Checks {
			if !c.OK {
				failed = append(failed, c)
			}
		}
		a.failCreate(rt, validationFailedMessage, failed)
		return
	}
	if ctx.Err() != nil {
		return
	}
	if mode != "edit" {
		next, err := a.renameDraftOnce(path, provisional)
		if err != nil {
			a.failCreate(rt, err.Error(), nil)
			return
		}
		path = next
		a.setCreate(rt, func(rec *createRecord) { rec.Path = path })
	}
	card, reload, err := a.preparePluginChange(path, settings)
	if err != nil {
		a.failCreate(rt, err.Error(), nil)
		return
	}
	approve := auto && !reload && mode != "edit"
	if ctx.Err() != nil {
		return
	}
	a.setCreate(rt, func(rec *createRecord) {
		if approve {
			rec.Step = "install"
			rec.Log = appendLog(rec.Log, "Installing.")
		} else {
			rec.Step = "propose"
		}
	})
	done, err := a.commitPluginChange(card, approve)
	if err != nil {
		a.setCreate(rt, func(rec *createRecord) {
			rec.Status = createFailed
			rec.Step = "done"
			rec.Error = err.Error()
			rec.Card = &done
			rec.Name = done.Plugin
		})
		return
	}
	status := createReady
	if approve {
		status = createInstalled
	}
	a.setCreate(rt, func(rec *createRecord) {
		rec.Status = status
		rec.Step = "done"
		rec.Error = ""
		rec.FailedChecks = nil
		cp := done
		rec.Card = &cp
		rec.Name = done.Plugin
		rec.Path = path
		if approve {
			rec.Log = appendLog(rec.Log, "Installed "+done.Plugin+".")
		} else {
			rec.Log = appendLog(rec.Log, "A change card is ready to review.")
		}
	})
}

func starterChecks(dir, starter string) []pluginauthor.Check {
	var out []pluginauthor.Check
	if pluginauthor.MarkerPresent(dir) {
		out = append(out, pluginauthor.Check{Name: "starter", OK: false, Detail: markerDraftMessage})
	}
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		out = append(out, pluginauthor.Check{Name: "manifest", OK: false, Detail: err.Error()})
		return out
	}
	if string(raw) == starter {
		out = append(out, pluginauthor.Check{Name: "manifest", OK: false, Detail: unchangedManifestMessage})
	}
	return out
}

func (a *App) renameDraftOnce(dir string, provisional bool) (string, error) {
	m, err := manifest.Load(dir)
	if err != nil {
		return "", err
	}
	if !pluginNameOK.MatchString(m.Name) {
		return "", errors.New("plugin name must be a short kebab-case name")
	}
	if provisional && strings.HasPrefix(m.Name, "draft-") {
		return "", errors.New(unnamedDraftMessage)
	}
	base := filepath.Base(dir)
	if m.Name == base {
		return dir, nil
	}
	next := filepath.Join(filepath.Dir(dir), m.Name)
	if _, err := os.Stat(next); err == nil {
		return "", errors.New(m.Name + " already exists")
	}
	if err := os.Rename(dir, next); err != nil {
		return "", err
	}
	return next, nil
}

func checksReport(checks []pluginauthor.Check, errText string) string {
	var b strings.Builder
	for _, c := range checks {
		if c.OK {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", c.Name, c.Detail)
	}
	if b.Len() == 0 {
		b.WriteString(strings.TrimSpace(errText))
	}
	return b.String()
}

func (a *App) failCreate(rt *createRuntime, msg string, checks []pluginauthor.Check) {
	a.setCreate(rt, func(rec *createRecord) {
		rec.Status = createFailed
		rec.Step = "done"
		rec.Error = msg
		rec.FailedChecks = checks
	})
}

func (a *App) appendCreateLog(rt *createRuntime, line string) {
	a.setCreate(rt, func(rec *createRecord) {
		rec.Log = appendLog(rec.Log, line)
	})
}

func appendLog(log, line string) string {
	if line == "" {
		return log
	}
	if log != "" && !strings.HasSuffix(log, "\n") {
		log += "\n"
	}
	log += line + "\n"
	if len(log) > 256*1024 {
		log = log[len(log)-256*1024:]
	}
	return log
}

func (a *App) setCreate(rt *createRuntime, fn func(*createRecord)) {
	a.mu.Lock()
	fn(&rt.rec)
	rt.rec.UpdatedAt = time.Now().UTC()
	rec := rt.rec
	a.mu.Unlock()
	a.writeCreate(rec)
}

func (a *App) createView(rt *createRuntime) CreateView {
	a.mu.Lock()
	rec := rt.rec
	a.mu.Unlock()
	return CreateView{
		OK:           true,
		ID:           rec.ID,
		Status:       rec.Status,
		Step:         rec.Step,
		Log:          logTail(rec.Log, 80),
		Card:         rec.Card,
		FailedChecks: rec.FailedChecks,
		Error:        rec.Error,
		Name:         rec.Name,
		Path:         rec.Path,
		AutoInstall:  rec.AutoInstall,
		Template:     rec.Template,
		Mode:         rec.Mode,
		Description:  rec.Description,
		Agent:        rec.Agent,
	}
}

func logTail(log string, n int) string {
	if n <= 0 {
		n = 80
	}
	lines := strings.Split(log, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func (a *App) jobs() *sync.Map {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.agentJobs == nil {
		a.agentJobs = &sync.Map{}
	}
	return a.agentJobs
}

func (a *App) createByID(id string) (*createRuntime, error) {
	v, ok := a.jobs().Load(id)
	if !ok {
		return nil, fmt.Errorf("%w: create %s", ErrNotFound, id)
	}
	return v.(*createRuntime), nil
}

func (a *App) createDir() string {
	return rundir.Jobs(a.DataDir)
}

func (a *App) writeCreate(rec createRecord) {
	dir := a.createDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(dir, rec.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func (a *App) loadCreateJobs() {
	if a.agentJobs == nil {
		a.agentJobs = &sync.Map{}
	}
	entries, err := os.ReadDir(a.createDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(a.createDir(), e.Name()))
		if err != nil {
			continue
		}
		var rec createRecord
		if err := json.Unmarshal(raw, &rec); err != nil || rec.ID == "" {
			continue
		}
		if rec.Status == createRunning {
			rec.Status = createFailed
			rec.Step = "done"
			rec.Error = restartMessage
			rec.UpdatedAt = time.Now().UTC()
			a.writeCreate(rec)
		}
		done := make(chan struct{})
		close(done)
		a.agentJobs.Store(rec.ID, &createRuntime{rec: rec, done: done})
	}
}

func (a *App) cancelCreateJobs() {
	if a.agentJobs == nil {
		return
	}
	var waits []chan struct{}
	a.agentJobs.Range(func(_, v any) bool {
		rt := v.(*createRuntime)
		a.mu.Lock()
		running := rt.rec.Status == createRunning
		a.mu.Unlock()
		if running && rt.cancel != nil {
			rt.cancel()
			if rt.done != nil {
				waits = append(waits, rt.done)
			}
		}
		return true
	})
	timeout := time.After(3 * time.Second)
	for _, ch := range waits {
		select {
		case <-ch:
		case <-timeout:
			return
		}
	}
}
