package core

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vicgupta/dap/internal/id"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/permissions"
	"github.com/vicgupta/dap/internal/store"
)

// preparePluginChange builds an install or reload card. It does not validate
// and it does not write the card. The caller has already checked the draft.
func (a *App) preparePluginChange(dir string, settings map[string]any) (model.Card, bool, error) {
	m, err := manifest.Load(dir)
	if err != nil {
		return model.Card{}, false, err
	}
	if settings != nil {
		if _, err := manifest.ResolveConfig(m, settings); err != nil {
			return model.Card{}, false, err
		}
	}
	reload := false
	snap := a.store.Snapshot()
	if existing := snap.Plugins[m.Name]; existing != nil {
		if filepath.Clean(existing.Dir) != filepath.Clean(dir) {
			return model.Card{}, false, fmt.Errorf("%s is already installed from a different directory", m.Name)
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
	payloadMap := map[string]any{
		"install": map[string]any{"path": dir, "reload": reload},
	}
	if len(settings) > 0 && !reload {
		payloadMap["config"] = settings
	}
	payload, _ := json.Marshal(payloadMap)
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
	return card, reload, nil
}

// commitPluginChange writes the card. approve marks it approved by the user,
// logs the proposed and approved rows, and installs through applyPluginPayload.
// A pending card is only proposed. Edits and reloads stay pending.
func (a *App) commitPluginChange(card model.Card, approve bool) (model.Card, error) {
	if approve {
		card.Status = model.StatusApproved
		card.Actor = model.ActorUser
		card.Reason = preapprovedReason
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
	if !approve {
		return card, nil
	}
	a.log(ledger.Entry{
		Plugin:  card.Plugin,
		Kind:    ledger.KindApproved,
		CardID:  card.ID,
		Message: "Approved “" + card.Title + "”",
		Data:    map[string]any{"reason": card.Reason, "actor": card.Actor},
	})
	if _, err := a.applyPluginPayload(card); err != nil {
		failed, ferr := a.failCard(card.ID, err)
		if ferr != nil {
			return failed, err
		}
		return failed, err
	}
	return a.completeCard(card.ID, &model.ActResult{Status: "done", Summary: "Installed " + card.Plugin + "."})
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
		Config map[string]any `json:"config,omitempty"`
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
	_, err = a.install(abs, true, payload.Config)
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
