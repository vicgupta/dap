package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vicgupta/dap/internal/agentrun"
	"github.com/vicgupta/dap/internal/id"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/permissions"
	"github.com/vicgupta/dap/internal/pluginauthor"
	"github.com/vicgupta/dap/internal/store"
)

// DraftRequest is the Create plugin form.
type DraftRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Lang        string `json:"lang"`
	UI          bool   `json:"ui"`
	Edit        string `json:"edit,omitempty"`
}

// DraftResult is a scaffolded directory plus the type DAP suggested.
type DraftResult struct {
	OK        bool   `json:"ok"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Suggested string `json:"suggested"`
}

// ScaffoldDraft writes a new plugin under the data directory's drafts folder.
func (a *App) ScaffoldDraft(req DraftRequest) (DraftResult, error) {
	suggested := pluginauthor.SuggestType(req.Description)
	kind := req.Type
	if kind == "" {
		kind = suggested
	}
	name, provisional, err := choosePluginName(req.Name, req.Description)
	if err != nil {
		return DraftResult{}, err
	}
	dir := filepath.Join(a.DataDir, "drafts", name)
	path, err := pluginauthor.Scaffold(pluginauthor.Options{
		Name:        name,
		Type:        kind,
		Lang:        req.Lang,
		UI:          req.UI,
		Description: req.Description,
		Dir:         dir,
	})
	if err != nil {
		return DraftResult{}, err
	}
	files, err := hashTree(path)
	if err != nil {
		return DraftResult{}, err
	}
	if err := a.writeDraftMeta(draftMeta{Name: name, Provisional: provisional, Files: files}); err != nil {
		return DraftResult{}, err
	}
	return DraftResult{OK: true, Path: path, Name: name, Type: kind, Suggested: suggested}, nil
}

type agentJob struct {
	ID     string
	Path   string
	Status string
	Log    string
	Err    string
	cancel context.CancelFunc
	done   chan struct{}
}

// AgentRun is the polled view of one agent process.
type AgentRun struct {
	OK      bool   `json:"ok"`
	ID      string `json:"id"`
	Status  string `json:"status"`
	Log     string `json:"log"`
	Error   string `json:"error,omitempty"`
	Changed *bool  `json:"changed,omitempty"`
	Message string `json:"message,omitempty"`
}

func (a *App) jobs() *sync.Map {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.agentJobs == nil {
		a.agentJobs = &sync.Map{}
	}
	return a.agentJobs
}

// AgentStart is one Create plugin or Edit with agent request.
type AgentStart struct {
	Path        string `json:"path"`
	Plugin      string `json:"plugin"`
	Agent       string `json:"agent"`
	Description string `json:"description"`
	Fix         string `json:"fix"`
}

// StartAgent runs a coding agent in a draft or an installed plugin directory.
func (a *App) StartAgent(req AgentStart) (AgentRun, error) {
	dir := req.Path
	if req.Plugin != "" {
		snap := a.store.Snapshot()
		rec := snap.Plugins[req.Plugin]
		if rec == nil {
			return AgentRun{}, fmt.Errorf("%w: plugin %s", ErrNotFound, req.Plugin)
		}
		dir = rec.Dir
	}
	abs, err := a.allowedPluginDir(dir)
	if err != nil {
		return AgentRun{}, err
	}
	if strings.TrimSpace(req.Description) == "" && strings.TrimSpace(req.Fix) == "" {
		return AgentRun{}, fmt.Errorf("describe what the plugin should do")
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &agentJob{ID: id.New("run"), Path: abs, Status: "running", cancel: cancel, done: make(chan struct{})}
	a.jobs().Store(job.ID, job)
	prompt := agentrun.Prompt(agentrun.PromptInput{
		Description: req.Description,
		FixReport:   req.Fix,
		DirName:     filepath.Base(abs),
	})
	go func() {
		defer close(job.done)
		err := agentrun.Run(ctx, agentrun.Request{
			Agent:  req.Agent,
			Dir:    abs,
			Prompt: prompt,
			OnLine: func(line string) { a.appendJob(job, line) },
		})
		a.mu.Lock()
		defer a.mu.Unlock()
		switch {
		case ctx.Err() != nil:
			job.Status = "cancelled"
		case err != nil:
			job.Status = "failed"
			job.Err = err.Error()
		default:
			job.Status = "finished"
		}
	}()
	return a.AgentRun(job.ID)
}

func (a *App) appendJob(job *agentJob, line string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	job.Log += line + "\n"
	if len(job.Log) > 256*1024 {
		job.Log = job.Log[len(job.Log)-256*1024:]
	}
}

// AgentRun returns the current log and status.
func (a *App) AgentRun(id string) (AgentRun, error) {
	v, ok := a.jobs().Load(id)
	if !ok {
		return AgentRun{}, fmt.Errorf("%w: run %s", ErrNotFound, id)
	}
	job := v.(*agentJob)
	a.mu.Lock()
	out := AgentRun{OK: true, ID: job.ID, Status: job.Status, Log: job.Log, Error: job.Err}
	path := job.Path
	a.mu.Unlock()
	switch out.Status {
	case "finished":
		changed, msg := a.draftChanged(path)
		out.Changed = &changed
		if !changed {
			out.Message = msg
		}
	case "failed":
		out.Message = "The coding agent stopped with an error, so DAP did not propose an install."
	case "cancelled":
		out.Message = "Cancelled. Nothing was proposed."
	}
	return out, nil
}

// CancelAgent stops a running agent.
func (a *App) CancelAgent(id string) error {
	v, ok := a.jobs().Load(id)
	if !ok {
		return fmt.Errorf("%w: run %s", ErrNotFound, id)
	}
	job := v.(*agentJob)
	job.cancel()
	select {
	case <-job.done:
	case <-time.After(3 * time.Second):
	}
	return nil
}

// ValidateDraft runs the conformance harness on an allowed directory.
func (a *App) ValidateDraft(dir, plugin string) (pluginauthor.Report, error) {
	abs, err := a.resolveCreateDir(dir, plugin)
	if err != nil {
		return pluginauthor.Report{}, err
	}
	return pluginauthor.Validate(abs), nil
}

func (a *App) resolveCreateDir(dir, plugin string) (string, error) {
	if plugin != "" {
		snap := a.store.Snapshot()
		rec := snap.Plugins[plugin]
		if rec == nil {
			return "", fmt.Errorf("%w: plugin %s", ErrNotFound, plugin)
		}
		dir = rec.Dir
	}
	return a.allowedPluginDir(dir)
}

// ProposePluginChange enqueues a change card. Approval installs a draft or reloads an edit.
func (a *App) ProposePluginChange(dir, plugin string) (model.Card, error) {
	abs, err := a.resolveCreateDir(dir, plugin)
	if err != nil {
		return model.Card{}, err
	}
	if a.isDraft(abs) {
		changed, msg := a.draftChanged(abs)
		if !changed {
			return model.Card{}, errors.New(msg)
		}
		abs, err = a.relocateDraft(abs)
		if err != nil {
			return model.Card{}, err
		}
	}
	rep := pluginauthor.Validate(abs)
	if !rep.OK {
		return model.Card{}, fmt.Errorf("plugin did not pass validation")
	}
	m, err := manifest.Load(abs)
	if err != nil {
		return model.Card{}, err
	}
	reload := false
	snap := a.store.Snapshot()
	if existing := snap.Plugins[m.Name]; existing != nil {
		if filepath.Clean(existing.Dir) != abs {
			return model.Card{}, fmt.Errorf("%s is already installed from a different directory", m.Name)
		}
		reload = true
	}
	body := pluginChangeBody(m)
	title := "Install " + m.Name
	summary := m.Description
	if reload {
		title = "Update " + m.Name
		if summary == "" {
			summary = "Reload this plugin with the files the agent wrote."
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"install": map[string]any{"path": abs, "reload": reload},
	})
	now := a.now().UTC()
	card := model.Card{
		ID:        id.New("c"),
		Plugin:    m.Name,
		Title:     title,
		Summary:   summary,
		Body:      body,
		Kind:      model.KindChange,
		Risk:      model.ModeAskFirst,
		Status:    model.StatusPending,
		Actor:     model.ActorExternal,
		Payload:   payload,
		Editable:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := a.store.Update(func(st *store.State) error {
		st.Cards = append(st.Cards, card)
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	a.log(ledger.Entry{
		Plugin:  card.Plugin,
		Kind:    ledger.KindProposed,
		CardID:  card.ID,
		Message: card.Title,
		Data:    map[string]any{"kind": card.Kind, "install": true},
	})
	return card, nil
}

func pluginChangeBody(m manifest.Manifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Type: %s\nSchedule: %s\n", m.Type, m.Schedule.Label())
	if len(m.Permissions) == 0 {
		b.WriteString("Permissions: none declared. Anything undeclared stays denied.\n")
	} else {
		b.WriteString("Permissions:\n")
		for _, p := range m.Permissions {
			fmt.Fprintf(&b, "- %s: %s\n", p.Name, permissions.Plain(p))
		}
	}
	b.WriteString("\nApproving installs or reloads this plugin. Nothing runs until you approve.")
	return b.String()
}

func (a *App) allowedPluginDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	drafts := filepath.Clean(filepath.Join(a.DataDir, "drafts"))
	if abs == drafts || strings.HasPrefix(abs, drafts+string(filepath.Separator)) {
		return abs, nil
	}
	snap := a.store.Snapshot()
	for _, p := range snap.Plugins {
		if filepath.Clean(p.Dir) == abs {
			return abs, nil
		}
	}
	return "", fmt.Errorf("path must be a draft or an installed plugin")
}

func installPayload(card model.Card) string {
	if len(card.Payload) == 0 {
		return ""
	}
	var payload struct {
		Install *struct {
			Path string `json:"path"`
		} `json:"install"`
	}
	if err := json.Unmarshal(card.Payload, &payload); err != nil || payload.Install == nil {
		return ""
	}
	return payload.Install.Path
}

func (a *App) applyPluginPayload(card model.Card) (bool, error) {
	if len(card.Payload) == 0 {
		return false, nil
	}
	var payload struct {
		Install *struct {
			Path   string `json:"path"`
			Reload bool   `json:"reload"`
		} `json:"install"`
	}
	if err := json.Unmarshal(card.Payload, &payload); err != nil || payload.Install == nil || payload.Install.Path == "" {
		return false, nil
	}
	abs, err := a.allowedPluginDir(payload.Install.Path)
	if err != nil {
		return true, err
	}
	if payload.Install.Reload {
		return true, a.reloadPlugin(abs)
	}
	_, err = a.Install(abs, true)
	return true, err
}

func (a *App) reloadPlugin(dir string) error {
	m, err := manifest.Load(dir)
	if err != nil {
		return err
	}
	snap := a.store.Snapshot()
	rec := snap.Plugins[m.Name]
	if rec == nil || filepath.Clean(rec.Dir) != filepath.Clean(dir) {
		return fmt.Errorf("%w: plugin %s", ErrNotFound, m.Name)
	}
	kept := map[string]any{}
	for _, f := range m.Config {
		if rec.Config != nil {
			if v, ok := rec.Config[f.Key]; ok {
				kept[f.Key] = v
			}
		}
	}
	cfg, err := manifest.ResolveConfig(m, kept)
	if err != nil {
		return err
	}
	if err := a.store.Update(func(st *store.State) error {
		p := st.Plugins[m.Name]
		if p == nil {
			return fmt.Errorf("%w: plugin %s", ErrNotFound, m.Name)
		}
		p.Manifest = m
		p.Approved = append([]manifest.Permission(nil), []manifest.Permission(m.Permissions)...)
		p.Config = cfg
		p.Schedule = m.Schedule
		return nil
	}); err != nil {
		return err
	}
	if !a.isUp() {
		return nil
	}
	a.mutex(m.Name).Lock()
	defer a.mutex(m.Name).Unlock()
	a.host.Stop(m.Name)
	return a.startOne(m.Name)
}
