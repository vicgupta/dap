package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vicgupta/dap/internal/guardrails"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginhost"
	"github.com/vicgupta/dap/internal/scheduler"
	"github.com/vicgupta/dap/internal/store"
)

// Edit is a partial update of a pending card.
type Edit struct {
	Title   *string
	Summary *string
	Body    *string
	Payload *json.RawMessage
}

// Approve runs guardrails and, for a plugin action, calls act.
// External cards stay approved until outcome. Change cards apply config and re-initialize.
func (a *App) Approve(id string) (model.Card, error) {
	plugin, err := a.cardPlugin(id)
	if err != nil {
		return model.Card{}, err
	}
	a.mutex(plugin).Lock()
	defer a.mutex(plugin).Unlock()
	return a.finishApproved(id)
}

func (a *App) finishApproved(id string) (model.Card, error) {
	snap := a.store.Snapshot()
	existing, ok := findCard(snap.Cards, id)
	if !ok {
		return model.Card{}, fmt.Errorf("%w: card %s", ErrNotFound, id)
	}
	rec := snap.Plugins[existing.Plugin]
	if rec == nil {
		return model.Card{}, fmt.Errorf("%w: plugin %s", ErrNotFound, existing.Plugin)
	}
	if existing.Status != model.StatusPending && existing.Status != model.StatusSnoozed {
		return existing, fmt.Errorf("%w: card is %s", ErrBadState, existing.Status)
	}
	res := a.evaluate(rec, existing.Permission, existing.Effects, true)
	a.rememberCheck(existing.Plugin, existing.Permission, existing.Effects, id, res)
	if res.Decision == model.ModeNever {
		_ = a.store.Update(func(st *store.State) error {
			c, i, err := indexCard(st.Cards, id)
			if err != nil {
				return err
			}
			_ = c
			st.Cards[i].Status = model.StatusBlocked
			st.Cards[i].Reason = res.Reason
			st.Cards[i].UpdatedAt = a.now().UTC()
			return nil
		})
		a.log(ledger.Entry{Plugin: existing.Plugin, Kind: ledger.KindBlocked, CardID: id, Message: res.Reason})
		return model.Card{}, fmt.Errorf("%w: %s", ErrBlocked, res.Reason)
	}

	var card model.Card
	if err := a.store.Update(func(st *store.State) error {
		_, i, err := indexCard(st.Cards, id)
		if err != nil {
			return err
		}
		if st.Cards[i].Status != model.StatusPending && st.Cards[i].Status != model.StatusSnoozed {
			return fmt.Errorf("%w: card is %s", ErrBadState, st.Cards[i].Status)
		}
		if existing.Actor == model.ActorExternal && existing.Kind != model.KindChange {
			st.Cards[i].Status = model.StatusApproved
		} else {
			st.Cards[i].Status = model.StatusActing
		}
		st.Cards[i].UpdatedAt = a.now().UTC()
		card = cloneCard(st.Cards[i])
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	a.log(ledger.Entry{Plugin: card.Plugin, Kind: ledger.KindApproved, CardID: card.ID, Message: "Approved “" + card.Title + "”"})

	if card.Kind == model.KindChange {
		if err := a.applyChange(card); err != nil {
			return a.failCard(card.ID, err)
		}
		return a.completeCard(card.ID, &model.ActResult{Status: "done", Summary: "Applied the approved change."})
	}
	if card.Actor == model.ActorExternal {
		return card, nil
	}
	if !a.host.Running(card.Plugin) {
		if err := a.startOne(card.Plugin); err != nil {
			return a.failCard(card.ID, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var result model.ActResult
	if err := a.host.Call(ctx, card.Plugin, pluginhost.MethodAct, pluginhost.ActParams{Card: card}, &result); err != nil {
		return a.failCard(card.ID, err)
	}
	if result.Status == "" {
		result.Status = "done"
	}
	if result.Summary == "" {
		result.Summary = "Done."
	}
	return a.completeCard(card.ID, &result)
}

func (a *App) applyChange(card model.Card) error {
	snap := a.store.Snapshot()
	rec := snap.Plugins[card.Plugin]
	if rec == nil {
		return fmt.Errorf("%w: plugin %s", ErrNotFound, card.Plugin)
	}
	prevCfg := rec.Config
	prevSched := rec.Schedule
	prevNext := rec.Run.NextRun
	var payload struct {
		Config   map[string]any     `json:"config"`
		Schedule *manifest.Schedule `json:"schedule"`
	}
	if len(card.Payload) > 0 && string(card.Payload) != "null" {
		if err := json.Unmarshal(card.Payload, &payload); err != nil {
			return fmt.Errorf("change payload: %w", err)
		}
	}
	if payload.Config == nil {
		payload.Config = map[string]any{}
	}
	if err := a.store.Update(func(st *store.State) error {
		p := st.Plugins[card.Plugin]
		if p == nil {
			return fmt.Errorf("%w: plugin %s", ErrNotFound, card.Plugin)
		}
		cfg, err := manifest.ResolveConfig(p.Manifest, payload.Config)
		if err != nil {
			return err
		}
		if payload.Schedule != nil {
			if err := payload.Schedule.Validate(); err != nil {
				return err
			}
			p.Schedule = *payload.Schedule
		} else if every, ok := cfg["every"].(string); ok && every != "" && p.Schedule.Cron == "" && p.Schedule.At == "" {
			p.Schedule.Every = every
			if err := p.Schedule.Validate(); err != nil {
				return err
			}
		}
		if !p.Schedule.Empty() {
			next, err := scheduler.Next(scheduler.FromManifest(p.Schedule), a.now())
			if err != nil {
				return err
			}
			p.Run.NextRun = next
		}
		p.Config = cfg
		return nil
	}); err != nil {
		return err
	}
	if a.host.Running(card.Plugin) {
		if err := a.initialize(card.Plugin); err != nil {
			_ = a.store.Update(func(st *store.State) error {
				p := st.Plugins[card.Plugin]
				if p == nil {
					return nil
				}
				p.Config = prevCfg
				p.Schedule = prevSched
				p.Run.NextRun = prevNext
				return nil
			})
			if a.host.Running(card.Plugin) {
				_ = a.initialize(card.Plugin)
			}
			return err
		}
	}
	a.log(ledger.Entry{Plugin: card.Plugin, Kind: ledger.KindChange, CardID: card.ID, Message: "Applied a change to " + card.Plugin})
	return nil
}

func (a *App) completeCard(id string, result *model.ActResult) (model.Card, error) {
	var card model.Card
	now := a.now().UTC()
	if err := a.store.Update(func(st *store.State) error {
		_, i, err := indexCard(st.Cards, id)
		if err != nil {
			return err
		}
		st.Cards[i].Status = model.StatusDone
		st.Cards[i].Result = result
		st.Cards[i].ActedAt = &now
		st.Cards[i].UpdatedAt = now
		card = cloneCard(st.Cards[i])
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	ev := []model.Evidence{}
	summary := "Done."
	if result != nil {
		summary = result.Summary
		ev = result.Evidence
	}
	a.log(ledger.Entry{Plugin: card.Plugin, Kind: ledger.KindActed, CardID: id, Message: summary, Evidence: ev})
	return card, nil
}

func (a *App) failCard(id string, cause error) (model.Card, error) {
	var card model.Card
	now := a.now().UTC()
	_ = a.store.Update(func(st *store.State) error {
		_, i, err := indexCard(st.Cards, id)
		if err != nil {
			return err
		}
		st.Cards[i].Status = model.StatusFailed
		st.Cards[i].Result = &model.ActResult{Status: "failed", Summary: cause.Error()}
		st.Cards[i].UpdatedAt = now
		card = cloneCard(st.Cards[i])
		return nil
	})
	a.log(ledger.Entry{Plugin: card.Plugin, Kind: ledger.KindFailed, CardID: id, Message: cause.Error()})
	return card, cause
}

// Edit updates the text of a card that is still waiting.
func (a *App) Edit(id string, edit Edit) (model.Card, error) {
	var card model.Card
	if err := a.store.Update(func(st *store.State) error {
		_, i, err := indexCard(st.Cards, id)
		if err != nil {
			return err
		}
		if st.Cards[i].Status != model.StatusPending && st.Cards[i].Status != model.StatusSnoozed {
			return fmt.Errorf("%w: card is %s", ErrBadState, st.Cards[i].Status)
		}
		if !st.Cards[i].Editable {
			return fmt.Errorf("this card cannot be edited")
		}
		if edit.Title != nil {
			if stringsTrim(*edit.Title) == "" {
				return fmt.Errorf("a card needs a title")
			}
			st.Cards[i].Title = *edit.Title
		}
		if edit.Summary != nil {
			st.Cards[i].Summary = *edit.Summary
		}
		if edit.Body != nil {
			st.Cards[i].Body = *edit.Body
		}
		if edit.Payload != nil {
			st.Cards[i].Payload = append([]byte(nil), (*edit.Payload)...)
		}
		st.Cards[i].UpdatedAt = a.now().UTC()
		card = cloneCard(st.Cards[i])
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	a.log(ledger.Entry{Plugin: card.Plugin, Kind: ledger.KindEdited, CardID: id, Message: "Edited “" + card.Title + "”"})
	return card, nil
}

// Reject closes a card. A reason is required.
func (a *App) Reject(id, reason string) (model.Card, error) {
	if stringsTrim(reason) == "" {
		return model.Card{}, fmt.Errorf("a rejection needs a reason")
	}
	var card model.Card
	if err := a.store.Update(func(st *store.State) error {
		_, i, err := indexCard(st.Cards, id)
		if err != nil {
			return err
		}
		if st.Cards[i].Status != model.StatusPending && st.Cards[i].Status != model.StatusSnoozed {
			return fmt.Errorf("%w: card is %s", ErrBadState, st.Cards[i].Status)
		}
		st.Cards[i].Status = model.StatusRejected
		st.Cards[i].Reason = reason
		st.Cards[i].UpdatedAt = a.now().UTC()
		card = cloneCard(st.Cards[i])
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	a.log(ledger.Entry{Plugin: card.Plugin, Kind: ledger.KindRejected, CardID: id, Message: "Rejected “" + card.Title + "”: " + reason})
	return card, nil
}

// Snooze hides a card until until.
func (a *App) Snooze(id string, until time.Time) (model.Card, error) {
	if !until.After(a.now()) {
		return model.Card{}, fmt.Errorf("snooze time must be in the future")
	}
	var card model.Card
	until = until.UTC()
	if err := a.store.Update(func(st *store.State) error {
		_, i, err := indexCard(st.Cards, id)
		if err != nil {
			return err
		}
		if st.Cards[i].Status != model.StatusPending && st.Cards[i].Status != model.StatusSnoozed {
			return fmt.Errorf("%w: card is %s", ErrBadState, st.Cards[i].Status)
		}
		st.Cards[i].Status = model.StatusSnoozed
		st.Cards[i].SnoozeUntil = &until
		st.Cards[i].UpdatedAt = a.now().UTC()
		card = cloneCard(st.Cards[i])
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	a.log(ledger.Entry{
		Plugin:  card.Plugin,
		Kind:    ledger.KindSnoozed,
		CardID:  id,
		Message: "Snoozed “" + card.Title + "” until " + until.Local().Format("Jan 2 3:04 PM"),
	})
	return card, nil
}

func (a *App) wake(now time.Time) {
	_ = a.store.Update(func(st *store.State) error {
		changed := false
		for i := range st.Cards {
			c := &st.Cards[i]
			if c.Status == model.StatusSnoozed && c.SnoozeUntil != nil && !c.SnoozeUntil.After(now) {
				c.Status = model.StatusPending
				c.SnoozeUntil = nil
				c.UpdatedAt = now.UTC()
				changed = true
			}
		}
		if !changed {
			return fmt.Errorf("no snoozes due")
		}
		return nil
	})
}

// Outcome records the result of an external approved card.
func (a *App) Outcome(plugin, cardID, summary string, evidence []model.Evidence) (model.Card, error) {
	if stringsTrim(summary) == "" {
		return model.Card{}, fmt.Errorf("outcome needs a summary")
	}
	var card model.Card
	now := a.now().UTC()
	if err := a.store.Update(func(st *store.State) error {
		_, i, err := indexCard(st.Cards, cardID)
		if err != nil {
			return err
		}
		if st.Cards[i].Plugin != plugin {
			return fmt.Errorf("%w: card belongs to %s", ErrDenied, st.Cards[i].Plugin)
		}
		switch st.Cards[i].Status {
		case model.StatusApproved, model.StatusDone, model.StatusActing:
		default:
			return fmt.Errorf("%w: card is %s", ErrBadState, st.Cards[i].Status)
		}
		st.Cards[i].Status = model.StatusDone
		st.Cards[i].Result = &model.ActResult{Status: "done", Summary: summary, Evidence: evidence}
		if st.Cards[i].ActedAt == nil {
			st.Cards[i].ActedAt = &now
		}
		st.Cards[i].UpdatedAt = now
		card = cloneCard(st.Cards[i])
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	a.log(ledger.Entry{Plugin: plugin, Kind: ledger.KindOutcome, CardID: cardID, Message: summary, Evidence: evidence})
	return card, nil
}

// Check is `da check`. cardID, when it covers the action, counts as approval.
func (a *App) Check(plugin, permission string, effects []string, cardID string) (guardrails.Result, error) {
	effects, err := guardrails.NormalizeEffects(effects)
	if err != nil {
		return guardrails.Result{}, err
	}
	snap := a.store.Snapshot()
	rec := snap.Plugins[plugin]
	if rec == nil {
		return guardrails.Result{}, fmt.Errorf("%w: plugin %s", ErrNotFound, plugin)
	}
	approved := false
	if cardID != "" {
		if card, ok := findCard(snap.Cards, cardID); ok && cardCovers(card, plugin, permission, effects) {
			approved = true
		}
	}
	res := a.evaluate(rec, permission, effects, approved)
	a.rememberCheck(plugin, permission, effects, cardID, res)
	kind := ledger.KindCheck
	if res.Decision == model.ModeNever {
		kind = ledger.KindDenied
	}
	a.log(ledger.Entry{
		Plugin:  plugin,
		Kind:    kind,
		CardID:  cardID,
		Message: res.Reason,
		Data:    map[string]any{"decision": res.Decision, "permission": permission, "effects": effects},
	})
	if res.Decision == model.ModeNever {
		return res, fmt.Errorf("%w: %s", ErrDenied, res.Reason)
	}
	return res, nil
}

func (a *App) evaluate(rec *store.PluginRecord, permission string, effects []string, approved bool) guardrails.Result {
	return guardrails.Evaluate(guardrails.Request{
		Plugin:       rec.Name,
		Permission:   permission,
		Effects:      effects,
		Declared:     rec.Approved,
		Overrides:    a.store.Snapshot().Rules,
		CardApproved: approved,
	})
}

func (a *App) rememberCheck(plugin, permission string, effects []string, cardID string, res guardrails.Result) {
	rec := CheckRecord{
		At:         a.now().UTC(),
		Plugin:     plugin,
		Permission: permission,
		Effects:    effects,
		Decision:   res.Decision,
		Reason:     res.Reason,
		CardID:     cardID,
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.checks = append(a.checks, rec)
	if len(a.checks) > 40 {
		a.checks = append([]CheckRecord(nil), a.checks[len(a.checks)-40:]...)
	}
}

func cardCovers(card model.Card, plugin, permission string, effects []string) bool {
	if card.Plugin != plugin {
		return false
	}
	switch card.Status {
	case model.StatusActing, model.StatusApproved, model.StatusDone:
	default:
		return false
	}
	if permission != "" && card.Permission != "" && card.Permission != permission {
		return false
	}
	have := map[string]bool{}
	for _, e := range card.Effects {
		have[e] = true
	}
	for _, e := range effects {
		if !have[e] {
			return false
		}
	}
	return true
}

// Log appends a plugin note to the ledger.
func (a *App) Log(plugin, message string, evidence []model.Evidence) error {
	if stringsTrim(message) == "" {
		return fmt.Errorf("log message is empty")
	}
	if _, err := a.pluginRecord(plugin); err != nil {
		return err
	}
	a.log(ledger.Entry{Plugin: plugin, Kind: ledger.KindLog, Message: message, Evidence: evidence})
	return nil
}

// WriteTile replaces the plugin's home tile.
func (a *App) WriteTile(plugin string, tile model.Tile) error {
	if _, err := a.pluginRecord(plugin); err != nil {
		return err
	}
	tile.UpdatedAt = a.now().UTC()
	if err := a.store.Update(func(st *store.State) error {
		p := st.Plugins[plugin]
		if p == nil {
			return fmt.Errorf("%w: plugin %s", ErrNotFound, plugin)
		}
		p.Tile = tile
		return nil
	}); err != nil {
		return err
	}
	a.log(ledger.Entry{Plugin: plugin, Kind: ledger.KindTile, Message: "Updated the " + plugin + " tile"})
	return nil
}

// GetState returns the plugin's scratch document.
func (a *App) GetState(plugin string) (json.RawMessage, error) {
	if _, err := a.pluginRecord(plugin); err != nil {
		return nil, err
	}
	snap := a.store.Snapshot()
	raw := snap.PluginState[plugin]
	if len(raw) == 0 {
		return json.RawMessage("null"), nil
	}
	return raw, nil
}

// PutState replaces the plugin's scratch document.
func (a *App) PutState(plugin string, raw json.RawMessage) error {
	if _, err := a.pluginRecord(plugin); err != nil {
		return err
	}
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	if !json.Valid(raw) {
		return fmt.Errorf("state must be JSON")
	}
	return a.store.Update(func(st *store.State) error {
		if st.PluginState == nil {
			st.PluginState = map[string]json.RawMessage{}
		}
		st.PluginState[plugin] = append([]byte(nil), raw...)
		return nil
	})
}

// SecretSet stores a secret for this plugin only, if secrets.write is declared and allowed.
func (a *App) SecretSet(plugin, account, secret string) error {
	res, err := a.Check(plugin, "secrets.write", nil, "")
	if err != nil {
		return err
	}
	if res.Decision != model.ModeAllowed {
		return fmt.Errorf("%w: %s", ErrDenied, res.Reason)
	}
	return a.secrets.Set(secretService(plugin), account, secret)
}

// SecretGet reads a secret for this plugin only.
func (a *App) SecretGet(plugin, account string) (string, error) {
	res, err := a.Check(plugin, "secrets.read", nil, "")
	if err != nil {
		return "", err
	}
	if res.Decision != model.ModeAllowed {
		return "", fmt.Errorf("%w: %s", ErrDenied, res.Reason)
	}
	return a.secrets.Get(secretService(plugin), account)
}

// SecretDelete removes one of the plugin's secrets.
func (a *App) SecretDelete(plugin, account string) error {
	res, err := a.Check(plugin, "secrets.write", nil, "")
	if err != nil {
		return err
	}
	if res.Decision != model.ModeAllowed {
		return fmt.Errorf("%w: %s", ErrDenied, res.Reason)
	}
	return a.secrets.Delete(secretService(plugin), account)
}

func secretService(plugin string) string { return "dap.plugin." + plugin }

func (a *App) pluginRecord(name string) (*store.PluginRecord, error) {
	snap := a.store.Snapshot()
	rec := snap.Plugins[name]
	if rec == nil {
		return nil, fmt.Errorf("%w: plugin %s", ErrNotFound, name)
	}
	return rec, nil
}

func (a *App) cardPlugin(id string) (string, error) {
	snap := a.store.Snapshot()
	card, ok := findCard(snap.Cards, id)
	if !ok {
		return "", fmt.Errorf("%w: card %s", ErrNotFound, id)
	}
	return card.Plugin, nil
}

func findCard(cards []model.Card, id string) (model.Card, bool) {
	for _, c := range cards {
		if c.ID == id {
			return c, true
		}
	}
	return model.Card{}, false
}

func indexCard(cards []model.Card, id string) (model.Card, int, error) {
	for i, c := range cards {
		if c.ID == id {
			return c, i, nil
		}
	}
	return model.Card{}, 0, fmt.Errorf("%w: card %s", ErrNotFound, id)
}

func cloneCard(c model.Card) model.Card {
	if c.Payload != nil {
		c.Payload = append([]byte(nil), c.Payload...)
	}
	if c.Evidence != nil {
		c.Evidence = append([]model.Evidence(nil), c.Evidence...)
	}
	if c.Effects != nil {
		c.Effects = append([]string(nil), c.Effects...)
	}
	if c.Result != nil {
		cp := *c.Result
		if cp.Evidence != nil {
			cp.Evidence = append([]model.Evidence(nil), cp.Evidence...)
		}
		c.Result = &cp
	}
	return c
}
