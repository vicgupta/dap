package core

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vicgupta/dap/internal/guardrails"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/permissions"
	"github.com/vicgupta/dap/internal/store"
	"github.com/vicgupta/dap/internal/version"
)

// Missed is the honest catch-up banner for Home.
type Missed struct {
	Count     int       `json:"count"`
	Scheduled time.Time `json:"scheduled"`
	Reason    string    `json:"reason"`
	Text      string    `json:"text"`
}

// PluginSummary is one plugin on Home and the Plugins page.
type PluginSummary struct {
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	Type          string         `json:"type"`
	Description   string         `json:"description,omitempty"`
	Health        model.Health   `json:"health"`
	Running       bool           `json:"running"`
	Pending       int            `json:"pending"`
	LastRun       *time.Time     `json:"last_run,omitempty"`
	NextRun       *time.Time     `json:"next_run,omitempty"`
	LastStatus    string         `json:"last_status,omitempty"`
	Schedule      string         `json:"schedule"`
	Missed        *Missed        `json:"missed,omitempty"`
	Tile          model.Tile     `json:"tile"`
	Metrics       []model.Metric `json:"metrics,omitempty"`
	UIURL         string         `json:"ui_url,omitempty"`
	UITitle       string         `json:"ui_title,omitempty"`
	UIRoute       string         `json:"ui_route,omitempty"`
	NeedsApproval bool           `json:"needs_approval"`
	Error         string         `json:"error,omitempty"`
	Permissions   []PermView     `json:"permissions,omitempty"`
	Interface     string         `json:"interface,omitempty"`
}

// HomeView is the Home payload.
type HomeView struct {
	OK      bool            `json:"ok"`
	Version string          `json:"version"`
	Pending int             `json:"pending"`
	Plugins []PluginSummary `json:"plugins"`
	Secrets string          `json:"secrets_backend"`
	Agents  []AgentInfo     `json:"agents"`
}

// AgentInfo is a local coding agent doctor can see.
type AgentInfo struct {
	Name  string `json:"name"`
	Found bool   `json:"found"`
	Path  string `json:"path,omitempty"`
}

// Home lists every installed plugin.
func (a *App) Home() HomeView {
	plugins := a.pluginSummaries(true)
	pending := 0
	for _, p := range plugins {
		pending += p.Pending
	}
	return HomeView{
		OK:      true,
		Version: version.Version,
		Pending: pending,
		Plugins: plugins,
		Secrets: a.SecretsBackend(),
		Agents:  DetectAgents(),
	}
}

func (a *App) pluginSummaries(withPerms bool) []PluginSummary {
	snap := a.store.Snapshot()
	out := make([]PluginSummary, 0, len(snap.Plugins))
	for _, rec := range snap.Plugins {
		rt := a.runtimeOf(rec.Name)
		pending := 0
		for _, c := range snap.Cards {
			if c.Plugin == rec.Name && (c.Status == model.StatusPending || c.Status == model.StatusSnoozed) {
				if c.Status == model.StatusPending {
					pending++
				}
			}
		}
		health := rt.Health
		if health.Status == "" {
			if rt.NeedsApproval {
				health = model.Health{Status: model.HealthDegraded, Message: "Waiting for permission approval"}
			} else if !a.isUp() {
				health = model.Health{Status: model.HealthDegraded, Message: "Not started"}
			} else {
				health = model.Health{Status: model.HealthDegraded, Message: "Not running"}
			}
		}
		sum := PluginSummary{
			Name:          rec.Name,
			Version:       rec.Manifest.Version,
			Type:          rec.Manifest.Type,
			Description:   rec.Manifest.Description,
			Health:        health,
			Running:       a.host.Running(rec.Name),
			Pending:       pending,
			LastRun:       rec.Run.LastRun,
			LastStatus:    rec.Run.LastStatus,
			Schedule:      rec.Schedule.Label(),
			Tile:          rec.Tile,
			Metrics:       rec.Metrics,
			UIURL:         rt.UIURL,
			NeedsApproval: rt.NeedsApproval,
			Error:         rt.StartError,
			Interface:     rec.Manifest.Interface,
		}
		if !rec.Run.NextRun.IsZero() {
			t := rec.Run.NextRun
			sum.NextRun = &t
		}
		if rec.Run.MissedCount > 0 && rec.Run.MissedScheduled != nil {
			sum.Missed = &Missed{
				Count:     rec.Run.MissedCount,
				Scheduled: *rec.Run.MissedScheduled,
				Reason:    rec.Run.MissedReason,
				Text:      missedText(rec.Run.MissedCount, *rec.Run.MissedScheduled, rec.Run.MissedReason),
			}
		}
		if rec.Manifest.UI != nil {
			sum.UITitle = rec.Manifest.UI.Title
			sum.UIRoute = rec.Manifest.UI.Route
		}
		if withPerms {
			sum.Permissions = permViews(rec.Approved)
			for i, p := range rec.Approved {
				eff := guardrails.Evaluate(guardrails.Request{
					Plugin: rec.Name, Permission: p.Name, Declared: rec.Approved, Overrides: snap.Rules,
				})
				sum.Permissions[i].Mode = eff.Decision
				plain := strings.TrimSpace(p.Description)
				if plain == "" {
					plain = permissions.Plain(p)
				}
				if eff.Decision == model.ModeNever {
					plain += " — blocked by a guardrail"
				}
				sum.Permissions[i].Plain = plain
			}
		}
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (a *App) PluginUI(name string) (root string, ok bool) {
	rec, err := a.pluginRecord(name)
	if err != nil || rec.Manifest.UI == nil {
		return "", false
	}
	entry, err := manifest.SafePath(rec.Dir, rec.Manifest.UI.Entry)
	if err != nil {
		return "", false
	}
	return filepath.Dir(entry), true
}

// Inbox returns waiting cards, pending first, then snoozed.
func (a *App) Inbox() []model.Card {
	snap := a.store.Snapshot()
	var pending, snoozed []model.Card
	for _, c := range snap.Cards {
		switch c.Status {
		case model.StatusPending:
			pending = append(pending, c)
		case model.StatusSnoozed:
			snoozed = append(snoozed, c)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].CreatedAt.Before(pending[j].CreatedAt) })
	sort.Slice(snoozed, func(i, j int) bool { return snoozed[i].CreatedAt.Before(snoozed[j].CreatedAt) })
	return append(pending, snoozed...)
}

// Approved returns cards a person has approved that are waiting on an external actor.
func (a *App) Approved(limit int) []model.Card {
	if limit <= 0 {
		limit = 50
	}
	snap := a.store.Snapshot()
	var out []model.Card
	for i := len(snap.Cards) - 1; i >= 0; i-- {
		c := snap.Cards[i]
		if c.Status == model.StatusApproved {
			out = append(out, c)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// Activity returns recent ledger rows, newest first.
func (a *App) Activity(limit int) ([]ledger.Entry, error) {
	if limit <= 0 {
		limit = 200
	}
	return a.ledger.Recent(limit)
}

// GuardView is the Guardrails page.
type GuardView struct {
	OK      bool              `json:"ok"`
	Rules   []guardrails.Rule `json:"rules"`
	Checks  []CheckRecord     `json:"checks"`
	Plugins []PluginSummary   `json:"plugins"`
	Policy  []PolicyNote      `json:"policy"`
}

// PolicyNote is one baked-in sentence on the Guardrails page.
type PolicyNote struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Guardrails returns rules, recent checks, and the baked-in policy.
func (a *App) Guardrails() GuardView {
	snap := a.store.Snapshot()
	a.mu.Lock()
	checks := append([]CheckRecord(nil), a.checks...)
	a.mu.Unlock()
	for i, j := 0, len(checks)-1; i < j; i, j = i+1, j-1 {
		checks[i], checks[j] = checks[j], checks[i]
	}
	return GuardView{
		OK:      true,
		Rules:   snap.Rules,
		Checks:  checks,
		Plugins: a.pluginSummaries(true),
		Policy: []PolicyNote{
			{Title: "Approval rule", Body: "Anything that sends, spends, or deletes waits in the decision inbox. A plugin changes its own steps or rules only through an approved change card."},
			{Title: "Same contract", Body: "Every plugin implements initialize, watch, propose, act, and report. The core calls act only after the guardrail check passes and you approve the card."},
			{Title: "Local and honest", Body: "DAP runs on this computer. The activity ledger is append-only. If the laptop was asleep, or DAP was not running, the missed run is labeled and caught up once."},
		},
	}
}

// SetRule stores a tightening. Loosening is ignored later by the guardrail engine.
func (a *App) SetRule(rule guardrails.Rule) error {
	rule.Decision = manifest.NormalizeMode(rule.Decision)
	if !guardrails.ValidDecision(rule.Decision) {
		return errString("decision must be allowed, ask_first, or never")
	}
	if rule.Plugin == "" {
		return errString("plugin is required")
	}
	if rule.Permission == "" {
		return errString("permission is required")
	}
	return a.store.Update(func(st *store.State) error {
		replaced := false
		for i := range st.Rules {
			if st.Rules[i].Plugin == rule.Plugin && st.Rules[i].Permission == rule.Permission {
				st.Rules[i] = rule
				replaced = true
			}
		}
		if !replaced {
			st.Rules = append(st.Rules, rule)
		}
		return nil
	})
}

type errString string

func (e errString) Error() string { return string(e) }

// DoctorCheck is one row from `da doctor`.
type DoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Level  string `json:"level,omitempty"`
}

// DoctorReport is the full doctor result.
type DoctorReport struct {
	OK      bool          `json:"ok"`
	Version string        `json:"version"`
	Checks  []DoctorCheck `json:"checks"`
}

// Doctor checks the data directory, plugins, scheduler, permissions, and local agents.
func (a *App) Doctor() DoctorReport {
	rep := DoctorReport{OK: true, Version: version.Version}
	add := func(c DoctorCheck) {
		if c.Level == "" {
			c.Level = "info"
		}
		if !c.OK && c.Level != "info" {
			rep.OK = false
		}
		rep.Checks = append(rep.Checks, c)
	}
	add(DoctorCheck{Name: "data_dir", OK: true, Detail: a.DataDir, Level: "info"})
	if _, err := a.ledger.Recent(1); err != nil {
		add(DoctorCheck{Name: "ledger", OK: false, Detail: err.Error(), Level: "error"})
	} else {
		add(DoctorCheck{Name: "ledger", OK: true, Detail: "append-only JSONL", Level: "info"})
	}
	add(DoctorCheck{Name: "secrets", OK: true, Detail: a.SecretsBackend(), Level: "info"})
	a.mu.Lock()
	orderErr := a.orderErr
	a.mu.Unlock()
	if orderErr != "" {
		add(DoctorCheck{Name: "dependencies", OK: false, Detail: orderErr, Level: "error"})
	} else {
		add(DoctorCheck{Name: "dependencies", OK: true, Detail: "resolved", Level: "info"})
	}
	for _, p := range a.pluginSummaries(false) {
		ok := p.Running && p.Health.Status == model.HealthOK && !p.NeedsApproval && p.Error == ""
		detail := p.Health.Message
		if p.NeedsApproval {
			detail = "permissions need approval"
		}
		if p.Error != "" {
			detail = p.Error
		}
		level := "error"
		if ok {
			level = "info"
		}
		if !p.ScheduleOK() && p.Schedule != "manual" && p.NextRun == nil {
			add(DoctorCheck{Name: "scheduler:" + p.Name, OK: false, Detail: "no next run", Level: "error"})
		} else if p.Schedule != "manual" {
			when := "unscheduled"
			if p.NextRun != nil {
				when = p.NextRun.Local().Format(time.RFC3339)
			}
			add(DoctorCheck{Name: "scheduler:" + p.Name, OK: true, Detail: p.Schedule + " · next " + when, Level: "info"})
		}
		add(DoctorCheck{Name: "plugin:" + p.Name, OK: ok, Detail: detail, Level: level})
		add(DoctorCheck{Name: "permissions:" + p.Name, OK: !p.NeedsApproval, Detail: p.Interface, Level: map[bool]string{true: "info", false: "error"}[!p.NeedsApproval]})
	}
	agents := DetectAgents()
	parts := make([]string, 0, len(agents))
	for _, ag := range agents {
		if ag.Found {
			parts = append(parts, ag.Name+" at "+ag.Path)
		} else {
			parts = append(parts, ag.Name+" not on PATH")
		}
	}
	add(DoctorCheck{Name: "agents", OK: true, Detail: strings.Join(parts, "; "), Level: "info"})
	for _, c := range a.MailChecks(context.Background()) {
		add(c)
	}
	return rep
}

// ScheduleOK is a placeholder so the doctor loop reads clearly. Plugins with a
// schedule always have a label other than "manual".
func (p PluginSummary) ScheduleOK() bool { return p.NextRun != nil }

// DetectAgents looks for the coding agents DAP can hand work to later.
func DetectAgents() []AgentInfo {
	out := make([]AgentInfo, 0, 3)
	for _, name := range []string{"claude", "codex", "opencode"} {
		info := AgentInfo{Name: name}
		if path, err := exec.LookPath(name); err == nil {
			info.Found = true
			info.Path = path
		}
		out = append(out, info)
	}
	return out
}

// CardBrief is a plugin-scoped view of a card.
type CardBrief struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Status  string   `json:"status"`
	Kind    string   `json:"kind"`
	StepID  string   `json:"step_id,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Effects []string `json:"effects,omitempty"`
}

// PluginContext is what a plugin's UI and process are allowed to see about themselves.
type PluginContext struct {
	Plugin    string            `json:"plugin"`
	Config    map[string]any    `json:"config"`
	Schedule  manifest.Schedule `json:"schedule"`
	State     json.RawMessage   `json:"state"`
	OpenCards []CardBrief       `json:"open_cards"`
	Health    model.Health      `json:"health"`
	LastRun   *time.Time        `json:"last_run,omitempty"`
	NextRun   *time.Time        `json:"next_run,omitempty"`
	Missed    string            `json:"missed,omitempty"`
	Pending   int               `json:"pending"`
}

// Context returns one plugin's config, scratch state, and open cards.
func (a *App) Context(plugin string) (PluginContext, error) {
	rec, err := a.pluginRecord(plugin)
	if err != nil {
		return PluginContext{}, err
	}
	raw, err := a.GetState(plugin)
	if err != nil {
		return PluginContext{}, err
	}
	ctx := PluginContext{
		Plugin:    plugin,
		Config:    rec.Config,
		Schedule:  rec.Schedule,
		State:     raw,
		OpenCards: a.pluginCards(plugin, true),
		Health:    a.runtimeOf(plugin).Health,
		LastRun:   rec.Run.LastRun,
	}
	if !rec.Run.NextRun.IsZero() {
		t := rec.Run.NextRun
		ctx.NextRun = &t
	}
	if rec.Run.MissedCount > 0 && rec.Run.MissedScheduled != nil {
		ctx.Missed = missedText(rec.Run.MissedCount, *rec.Run.MissedScheduled, rec.Run.MissedReason)
	}
	for _, c := range a.store.Snapshot().Cards {
		if c.Plugin == plugin && c.Status == model.StatusPending {
			ctx.Pending++
		}
	}
	return ctx, nil
}

// PluginCards lists cards for one plugin. openOnly keeps cards that still block a workflow.
func (a *App) PluginCards(plugin string) ([]CardBrief, error) {
	if _, err := a.pluginRecord(plugin); err != nil {
		return nil, err
	}
	return a.pluginCards(plugin, false), nil
}

func (a *App) pluginCards(plugin string, openOnly bool) []CardBrief {
	snap := a.store.Snapshot()
	var out []CardBrief
	for i := len(snap.Cards) - 1; i >= 0; i-- {
		c := snap.Cards[i]
		if c.Plugin != plugin {
			continue
		}
		if openOnly {
			switch c.Status {
			case model.StatusPending, model.StatusSnoozed, model.StatusActing, model.StatusApproved:
			default:
				continue
			}
		}
		out = append(out, CardBrief{
			ID: c.ID, Title: c.Title, Status: c.Status, Kind: c.Kind,
			StepID: stepID(c.Payload), Reason: c.Reason, Effects: c.Effects,
		})
	}
	return out
}

func stepID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var body struct {
		StepID string `json:"step_id"`
	}
	_ = json.Unmarshal(raw, &body)
	return body.StepID
}
