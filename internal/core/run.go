package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/vicgupta/dap/internal/guardrails"
	"github.com/vicgupta/dap/internal/id"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginhost"
	"github.com/vicgupta/dap/internal/scheduler"
	"github.com/vicgupta/dap/internal/store"
)

// RunPlugin runs the five-step cycle once. Pending cards do not block it.
func (a *App) RunPlugin(name, trigger string) error {
	if trigger == "" {
		trigger = "manual"
	}
	a.mutex(name).Lock()
	defer a.mutex(name).Unlock()
	if !a.host.Running(name) {
		if err := a.startOne(name); err != nil {
			return err
		}
	}
	return a.runLocked(name, trigger)
}

// Tick wakes snoozed cards and runs plugins whose schedule is due.
// A late wake records a missed run and catches up once.
func (a *App) Tick(now time.Time) {
	a.wake(now)
	snap := a.store.Snapshot()
	names := make([]string, 0, len(snap.Plugins))
	for name := range snap.Plugins {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rec := snap.Plugins[name]
		if rec == nil || rec.Schedule.Empty() {
			continue
		}
		mu := a.mutex(name)
		if !mu.TryLock() {
			continue
		}
		func() {
			defer mu.Unlock()
			freshSnap := a.store.Snapshot()
			fresh := freshSnap.Plugins[name]
			if fresh == nil || fresh.Schedule.Empty() {
				return
			}
			d, err := scheduler.Evaluate(scheduler.FromManifest(fresh.Schedule), fresh.Run, now)
			if err != nil || !d.Fire {
				return
			}
			_ = a.store.Update(func(st *store.State) error {
				p := st.Plugins[name]
				if p == nil {
					return nil
				}
				p.Run.NextRun = d.Next
				if d.Trigger == "catchup" {
					p.Run.MissedCount = d.Missed
					t := now.UTC()
					at := d.MissedAt.UTC()
					p.Run.MissedAt = &t
					p.Run.MissedScheduled = &at
					p.Run.MissedReason = d.Reason
				}
				return nil
			})
			if d.Trigger == "catchup" {
				a.log(ledger.Entry{
					Plugin:  name,
					Kind:    ledger.KindMissed,
					Message: missedText(d.Missed, d.MissedAt, d.Reason),
					Data:    map[string]any{"missed": d.Missed, "scheduled": d.MissedAt},
				})
			}
			if !a.host.Running(name) {
				if err := a.startOne(name); err != nil {
					return
				}
			}
			_ = a.runLocked(name, d.Trigger)
		}()
	}
}

func missedText(count int, at time.Time, reason string) string {
	stamp := at.Local().Format("3:04 PM")
	if reason == "" {
		reason = scheduler.MissedReason
	}
	if count <= 1 {
		return fmt.Sprintf("Missed %s run — %s", stamp, reason)
	}
	return fmt.Sprintf("Missed %d runs since %s — %s. Caught up once.", count, stamp, reason)
}

// StartScheduler ticks once a second until StopScheduler.
func (a *App) StartScheduler() {
	a.mu.Lock()
	if a.schedStop != nil {
		a.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	a.schedStop = stop
	a.mu.Unlock()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-t.C:
				a.Tick(now)
			}
		}
	}()
}

// StopScheduler stops the background ticker.
func (a *App) StopScheduler() {
	a.mu.Lock()
	stop := a.schedStop
	a.schedStop = nil
	a.mu.Unlock()
	if stop != nil {
		close(stop)
	}
}

func (a *App) runLocked(name, trigger string) error {
	snap := a.store.Snapshot()
	rec := snap.Plugins[name]
	if rec == nil {
		return fmt.Errorf("%w: plugin %s", ErrNotFound, name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var watch pluginhost.WatchResult
	if err := a.host.Call(ctx, name, pluginhost.MethodWatch, pluginhost.WatchParams{Since: rec.Run.LastRun}, &watch); err != nil {
		a.noteRun(name, trigger, "error", err)
		a.log(ledger.Entry{Plugin: name, Kind: ledger.KindRun, Message: "Run failed during watch: " + err.Error()})
		return err
	}
	if watch.Tile.Title != "" || len(watch.Tile.Lines) > 0 {
		watch.Tile.UpdatedAt = a.now().UTC()
		_ = a.store.Update(func(st *store.State) error {
			if p := st.Plugins[name]; p != nil {
				p.Tile = watch.Tile
			}
			return nil
		})
	}
	var proposed pluginhost.ProposeResult
	if err := a.host.Call(ctx, name, pluginhost.MethodPropose, pluginhost.ProposeParams{Observations: watch.Observations}, &proposed); err != nil {
		a.noteRun(name, trigger, "error", err)
		a.log(ledger.Entry{Plugin: name, Kind: ledger.KindRun, Message: "Run failed during propose: " + err.Error()})
		return err
	}
	var firstErr error
	for _, draft := range proposed.Cards {
		if _, err := a.accept(name, draft, model.ActorPlugin, true); err != nil && !errors.Is(err, ErrBlocked) && firstErr == nil {
			firstErr = err
		}
	}
	reportCtx, reportCancel := context.WithTimeout(context.Background(), 15*time.Second)
	var report pluginhost.ReportResult
	_ = a.host.Call(reportCtx, name, pluginhost.MethodReport, pluginhost.ReportParams{
		Period: "week",
		From:   weekStart(a.now()),
		To:     a.now(),
	}, &report)
	reportCancel()
	if len(report.Metrics) > 0 {
		_ = a.store.Update(func(st *store.State) error {
			if p := st.Plugins[name]; p != nil {
				p.Metrics = report.Metrics
			}
			return nil
		})
	}
	status := "ok"
	if firstErr != nil {
		status = "error"
	}
	a.noteRun(name, trigger, status, firstErr)
	msg := "Ran " + name
	switch trigger {
	case "catchup":
		msg = "Caught up " + name + " once"
	case "manual":
		msg = "Ran " + name + " now"
	}
	a.log(ledger.Entry{Plugin: name, Kind: ledger.KindRun, Message: msg})
	return firstErr
}

func (a *App) noteRun(name, trigger, status string, runErr error) {
	now := a.now().UTC()
	_ = a.store.Update(func(st *store.State) error {
		p := st.Plugins[name]
		if p == nil {
			return nil
		}
		p.Run.LastRun = &now
		p.Run.LastTrigger = trigger
		p.Run.LastStatus = status
		if runErr != nil {
			p.Run.LastError = runErr.Error()
		} else {
			p.Run.LastError = ""
		}
		if trigger == "schedule" && status == "ok" {
			p.Run.MissedCount = 0
			p.Run.MissedReason = ""
			p.Run.MissedAt = nil
			p.Run.MissedScheduled = nil
		}
		return nil
	})
}

// Propose enqueues a card. It does not call the plugin.
func (a *App) Propose(plugin string, draft model.CardDraft, actor string) (model.Card, error) {
	return a.accept(plugin, draft, actor, false)
}

func (a *App) accept(plugin string, draft model.CardDraft, actor string, auto bool) (model.Card, error) {
	if stringsTrim(draft.Title) == "" {
		return model.Card{}, fmt.Errorf("a card needs a title")
	}
	if actor == "" {
		actor = model.ActorExternal
	}
	if draft.Kind == "" {
		draft.Kind = model.KindAction
	}
	if draft.Kind != model.KindAction && draft.Kind != model.KindChange {
		return model.Card{}, fmt.Errorf("card kind must be action or change")
	}
	effects, err := guardrails.NormalizeEffects(draft.Effects)
	if err != nil {
		return model.Card{}, err
	}
	snap := a.store.Snapshot()
	rec := snap.Plugins[plugin]
	if rec == nil {
		return model.Card{}, fmt.Errorf("%w: plugin %s", ErrNotFound, plugin)
	}
	res := a.evaluate(rec, draft.Permission, effects, false)
	a.rememberCheck(plugin, draft.Permission, effects, "", res)
	if res.Decision == model.ModeNever {
		a.log(ledger.Entry{
			Plugin:  plugin,
			Kind:    ledger.KindBlocked,
			Message: res.Reason,
			Data:    map[string]any{"permission": draft.Permission, "effects": effects, "title": draft.Title},
		})
		return model.Card{}, fmt.Errorf("%w: %s", ErrBlocked, res.Reason)
	}
	editable := true
	if draft.Editable != nil {
		editable = *draft.Editable
	}
	now := a.now().UTC()
	card := model.Card{
		ID:         id.New("c"),
		Plugin:     plugin,
		Title:      draft.Title,
		Summary:    draft.Summary,
		Body:       draft.Body,
		Kind:       draft.Kind,
		Permission: draft.Permission,
		Effects:    effects,
		Risk:       res.Decision,
		Status:     model.StatusPending,
		Actor:      actor,
		Evidence:   draft.Evidence,
		Payload:    draft.Payload,
		Editable:   editable,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := a.store.Update(func(st *store.State) error {
		st.Cards = append(st.Cards, card)
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	a.log(ledger.Entry{
		Plugin:   plugin,
		Kind:     ledger.KindProposed,
		CardID:   card.ID,
		Message:  card.Title,
		Evidence: card.Evidence,
		Data:     map[string]any{"risk": card.Risk, "kind": card.Kind, "effects": card.Effects},
	})
	// Change cards always wait for a person, even when the permission is allowed.
	// Safe plugin actions that the manifest marks allowed can run inside this same cycle.
	if auto && actor == model.ActorPlugin && card.Kind == model.KindAction && res.Decision == model.ModeAllowed && !guardrails.Sensitive(effects) {
		done, err := a.finishApproved(card.ID)
		if err != nil {
			return done, err
		}
		return done, nil
	}
	return card, nil
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func weekStart(now time.Time) time.Time {
	local := now.In(time.Local)
	wd := int(local.Weekday())
	if wd == 0 {
		wd = 7
	}
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -(wd - 1))
	return start
}
