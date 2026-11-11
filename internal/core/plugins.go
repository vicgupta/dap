package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/permissions"
	"github.com/vicgupta/dap/internal/pluginhost"
	"github.com/vicgupta/dap/internal/scheduler"
	"github.com/vicgupta/dap/internal/store"
	"github.com/vicgupta/dap/internal/version"
)

// InstallResult is what `da plugin install` and the Plugins page show.
type InstallResult struct {
	Status      string     `json:"status"`
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Type        string     `json:"type"`
	Description string     `json:"description,omitempty"`
	Permissions []PermView `json:"permissions"`
	Message     string     `json:"message,omitempty"`
	// Sample is the declarative dry run shown before install.
	Sample string `json:"sample,omitempty"`
}

// PermView is one permission in plain language.
type PermView struct {
	Name  string `json:"name"`
	Mode  string `json:"mode"`
	Plain string `json:"plain"`
}

// Install validates a plugin directory. Without approve it only previews permissions.
func (a *App) Install(dir string, approve bool) (InstallResult, error) {
	return a.install(dir, approve, nil)
}

// install is Install with an optional config map. A nil map uses manifest defaults.
func (a *App) install(dir string, approve bool, given map[string]any) (InstallResult, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return InstallResult{}, err
	}
	m, err := manifest.Load(abs)
	if err != nil {
		return InstallResult{}, err
	}
	res := InstallResult{
		Name:        m.Name,
		Version:     m.Version,
		Type:        m.Type,
		Description: m.Description,
		Permissions: permViews([]manifest.Permission(m.Permissions)),
		Sample:      a.declarativeSample(m, given),
	}
	if !approve {
		res.Status = "approval_required"
		res.Message = "Review the permissions, then install again with approval. Nothing was changed."
		return res, nil
	}
	snap := a.store.Snapshot()
	if _, ok := snap.Plugins[m.Name]; ok {
		return res, fmt.Errorf("%s is already installed", m.Name)
	}
	ms := make([]manifest.Manifest, 0, len(snap.Plugins)+1)
	for _, p := range snap.Plugins {
		ms = append(ms, p.Manifest)
	}
	ms = append(ms, m)
	if _, err := manifest.Order(ms); err != nil {
		return res, err
	}
	cfg, err := manifest.ResolveConfig(m, given)
	if err != nil {
		return res, err
	}
	var next time.Time
	if !m.Schedule.Empty() {
		next, err = scheduler.Next(scheduler.FromManifest(m.Schedule), a.now())
		if err != nil {
			return res, err
		}
	}
	rec := &store.PluginRecord{
		Name:        m.Name,
		Dir:         abs,
		Config:      cfg,
		Schedule:    m.Schedule,
		Run:         scheduler.State{NextRun: next},
		InstalledAt: a.now().UTC(),
		Manifest:    m,
		Approved:    append([]manifest.Permission(nil), []manifest.Permission(m.Permissions)...),
	}
	if err := a.store.Update(func(st *store.State) error {
		if st.Plugins[m.Name] != nil {
			return fmt.Errorf("%s is already installed", m.Name)
		}
		st.Plugins[m.Name] = rec
		return nil
	}); err != nil {
		return res, err
	}
	a.log(ledger.Entry{
		Plugin:  m.Name,
		Kind:    ledger.KindInstall,
		Message: "Installed " + m.Name + " " + m.Version,
		Data:    map[string]any{"type": m.Type, "permissions": permViews([]manifest.Permission(m.Permissions))},
	})
	res.Status = "installed"
	res.Message = "Installed " + m.Name + "."
	if err := a.ensureAPI(); err != nil {
		res.Message = "Installed " + m.Name + ", but it did not start: " + err.Error()
		return res, nil
	}
	a.mutex(m.Name).Lock()
	startErr := a.startOne(m.Name)
	if startErr == nil {
		a.runIfNew(m.Name)
	}
	a.mutex(m.Name).Unlock()
	if startErr != nil {
		res.Message = "Installed " + m.Name + ", but it did not start: " + startErr.Error()
	}
	return res, nil
}

// Remove stops a plugin and forgets it. The ledger is kept.
func (a *App) Remove(name string) error {
	a.mutex(name).Lock()
	defer a.mutex(name).Unlock()
	snap := a.store.Snapshot()
	declarativePlugin := false
	if rec := snap.Plugins[name]; rec != nil && rec.Manifest.Runtime == "declarative" {
		declarativePlugin = true
	}
	a.host.Stop(name)
	now := a.now().UTC()
	type dropped struct {
		id, title string
	}
	var droppedCards []dropped
	if err := a.store.Update(func(st *store.State) error {
		if st.Plugins[name] == nil {
			return fmt.Errorf("%w: plugin %s", ErrNotFound, name)
		}
		delete(st.Plugins, name)
		delete(st.PluginState, name)
		for i := range st.Cards {
			c := &st.Cards[i]
			if c.Plugin != name {
				continue
			}
			if c.Status != model.StatusPending && c.Status != model.StatusSnoozed {
				continue
			}
			c.Status = model.StatusDismissed
			c.Reason = "Dismissed because the plugin was removed."
			c.UpdatedAt = now
			droppedCards = append(droppedCards, dropped{id: c.ID, title: c.Title})
		}
		return nil
	}); err != nil {
		return err
	}
	a.mu.Lock()
	delete(a.rt, name)
	a.mu.Unlock()
	if declarativePlugin {
		// The in-process handler is already stopped. Drop the scratch directory
		// the core created for this plugin. Plugin state was deleted above.
		_ = os.RemoveAll(filepath.Join(a.DataDir, "plugin-data", name))
	}
	a.removeDraft(name)
	for _, c := range droppedCards {
		title := c.title
		if title == "" {
			title = c.id
		}
		a.log(ledger.Entry{
			Plugin:  name,
			Kind:    ledger.KindDismissed,
			CardID:  c.id,
			Message: "Dismissed “" + title + "” because the plugin was removed.",
		})
	}
	a.log(ledger.Entry{Plugin: name, Kind: ledger.KindRemove, Message: "Removed " + name})
	return nil
}

// removeDraft deletes .dap/drafts/<name> so a later create can use that name.
// A plugin installed from another directory is left where it is.
func (a *App) removeDraft(name string) {
	drafts := filepath.Clean(filepath.Join(a.DataDir, "drafts"))
	target := filepath.Clean(filepath.Join(drafts, name))
	if target == drafts || !strings.HasPrefix(target, drafts+string(filepath.Separator)) {
		return
	}
	_ = os.RemoveAll(target)
}

// ApproveManifest accepts the plugin's current permission list and starts it.
func (a *App) ApproveManifest(name string) error {
	a.mutex(name).Lock()
	defer a.mutex(name).Unlock()
	snap := a.store.Snapshot()
	rec := snap.Plugins[name]
	if rec == nil {
		return fmt.Errorf("%w: plugin %s", ErrNotFound, name)
	}
	m, err := manifest.Load(rec.Dir)
	if err != nil {
		return err
	}
	if err := a.store.Update(func(st *store.State) error {
		p := st.Plugins[name]
		if p == nil {
			return fmt.Errorf("%w: plugin %s", ErrNotFound, name)
		}
		p.Manifest = m
		p.Approved = append([]manifest.Permission(nil), []manifest.Permission(m.Permissions)...)
		return nil
	}); err != nil {
		return err
	}
	a.mutateRT(name, func(rt *runtime) { rt.NeedsApproval = false })
	if !a.isUp() {
		return nil
	}
	if err := a.startOne(name); err != nil {
		return err
	}
	a.runIfNew(name)
	return nil
}

// StartPlugins loads manifests, starts dependencies first, and serves the plugin API.
func (a *App) StartPlugins() error {
	if err := a.ensureAPI(); err != nil {
		return err
	}
	a.recoverActing()
	snap := a.store.Snapshot()
	var manifests []manifest.Manifest
	loaded := map[string]manifest.Manifest{}
	for name, rec := range snap.Plugins {
		m, err := manifest.Load(rec.Dir)
		if err != nil {
			a.mutateRT(name, func(rt *runtime) {
				rt.StartError = err.Error()
				rt.Health = model.Health{Status: model.HealthError, Message: err.Error()}
			})
			continue
		}
		if !manifest.SamePermissions(rec.Approved, []manifest.Permission(m.Permissions)) {
			a.mutateRT(name, func(rt *runtime) {
				rt.NeedsApproval = true
				rt.StartError = "permissions changed and need approval"
				rt.Health = model.Health{Status: model.HealthDegraded, Message: "Waiting for permission approval"}
			})
			continue
		}
		loaded[name] = m
		manifests = append(manifests, m)
	}
	order, err := manifest.Order(manifests)
	if err != nil {
		a.mu.Lock()
		a.orderErr = err.Error()
		a.pluginsUp = true
		a.mu.Unlock()
		return nil
	}
	a.mu.Lock()
	a.orderErr = ""
	a.mu.Unlock()
	started := map[string]bool{}
	for _, name := range order {
		ready := true
		for _, d := range loaded[name].Dependencies {
			if !started[d.Name] {
				ready = false
				dep := d.Name
				a.mutateRT(name, func(rt *runtime) {
					rt.StartError = "dependency " + dep + " is not running"
					rt.Health = model.Health{Status: model.HealthError, Message: rt.StartError}
				})
				break
			}
		}
		if !ready {
			continue
		}
		a.mutex(name).Lock()
		err := a.startOne(name)
		if err == nil {
			started[name] = true
			a.runIfNew(name)
		}
		a.mutex(name).Unlock()
	}
	a.mu.Lock()
	a.pluginsUp = true
	a.mu.Unlock()
	return nil
}

func (a *App) startOne(name string) error {
	snap := a.store.Snapshot()
	rec := snap.Plugins[name]
	if rec == nil {
		return fmt.Errorf("%w: plugin %s", ErrNotFound, name)
	}
	m, err := manifest.Load(rec.Dir)
	if err != nil {
		a.failStart(name, err)
		return err
	}
	if !manifest.SamePermissions(rec.Approved, []manifest.Permission(m.Permissions)) {
		err := fmt.Errorf("%s has permissions that need approval", name)
		a.mutateRT(name, func(rt *runtime) {
			rt.NeedsApproval = true
			rt.StartError = err.Error()
			rt.Health = model.Health{Status: model.HealthDegraded, Message: "Waiting for permission approval"}
		})
		return err
	}
	cfg, err := manifest.ResolveConfig(m, rec.Config)
	if err != nil {
		a.failStart(name, err)
		return err
	}
	_ = a.store.Update(func(st *store.State) error {
		p := st.Plugins[name]
		if p == nil {
			return nil
		}
		p.Manifest = m
		p.Config = cfg
		return nil
	})
	token := randomToken()
	dataDir := filepath.Join(a.DataDir, "plugin-data", name)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	if m.Runtime == "declarative" {
		return a.startDeclarative(name, m, cfg, token)
	}
	argv, err := m.Launch(rec.Dir)
	if err != nil {
		a.failStart(name, err)
		return err
	}
	apiBase := a.apiBase + "/v1/" + name
	a.mutateRT(name, func(rt *runtime) {
		rt.Token = token
		rt.NeedsApproval = false
		rt.StartError = ""
	})
	env := pluginhost.PluginEnvIn(a.DataDir, "DAP_PLUGIN="+name, "DAP_API="+apiBase, "DAP_TOKEN="+token)
	if err := a.host.Start(pluginhost.Options{Name: name, Dir: rec.Dir, Argv: argv, Env: env}); err != nil {
		a.failStart(name, err)
		return err
	}
	if err := a.initialize(name); err != nil {
		a.host.Stop(name)
		a.failStart(name, err)
		a.log(ledger.Entry{Plugin: name, Kind: ledger.KindStart, Message: "Failed to start: " + err.Error()})
		return err
	}
	a.log(ledger.Entry{Plugin: name, Kind: ledger.KindStart, Message: "Started " + name + " (" + a.host.Interface(name) + ")"})
	return nil
}

func (a *App) initialize(name string) error {
	snap := a.store.Snapshot()
	rec := snap.Plugins[name]
	if rec == nil {
		return fmt.Errorf("%w: plugin %s", ErrNotFound, name)
	}
	if !a.host.Running(name) {
		return fmt.Errorf("plugin %s is not running", name)
	}
	dataDir := filepath.Join(a.DataDir, "plugin-data", name)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var init pluginhost.InitResult
	err := a.host.Call(ctx, name, pluginhost.MethodInitialize, pluginhost.InitParams{
		Interface:  rec.Manifest.Interface,
		DAPVersion: version.Version,
		Config:     rec.Config,
		PluginDir:  rec.Dir,
		DataDir:    dataDir,
		APIBase:    a.apiBase + "/v1/" + name,
		Token:      a.Token(name),
	}, &init)
	if err != nil {
		return err
	}
	if !init.Ready {
		msg := init.Health.Message
		if msg == "" {
			msg = "plugin reported not ready"
		}
		return fmt.Errorf("%s", msg)
	}
	iface, err := pluginhost.Negotiated(init.Interface, rec.Manifest.Interface)
	if err != nil {
		return err
	}
	a.host.SetInterface(name, iface)
	if init.Health.Status == "" {
		init.Health.Status = model.HealthOK
		init.Health.Message = "Ready"
	}
	a.mutateRT(name, func(rt *runtime) {
		rt.Health = init.Health
		rt.StartError = ""
	})
	return nil
}

func (a *App) failStart(name string, err error) {
	a.mutateRT(name, func(rt *runtime) {
		rt.StartError = err.Error()
		rt.Health = model.Health{Status: model.HealthError, Message: err.Error()}
	})
}

func (a *App) recoverActing() {
	_ = a.store.Update(func(st *store.State) error {
		for i := range st.Cards {
			if st.Cards[i].Status == model.StatusActing {
				st.Cards[i].Status = model.StatusPending
				st.Cards[i].UpdatedAt = a.now().UTC()
			}
		}
		return nil
	})
}

func permViews(list []manifest.Permission) []PermView {
	out := make([]PermView, 0, len(list))
	for _, p := range list {
		out = append(out, PermView{Name: p.Name, Mode: manifest.NormalizeMode(p.Mode), Plain: permissions.Plain(p)})
	}
	return out
}
