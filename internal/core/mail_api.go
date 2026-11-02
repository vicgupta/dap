package core

import (
	"context"
	"strings"
	"time"

	"fmt"

	"github.com/vicgupta/dap/internal/guardrails"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/mail"
	"github.com/vicgupta/dap/internal/model"
)

// MailStatuses is the Connections screen. Installed is true when that capability plugin is present.
func (a *App) MailStatuses() []mail.Status {
	out := a.mail.Statuses()
	installed := map[string]bool{}
	snap := a.store.Snapshot()
	for name := range snap.Plugins {
		switch name {
		case "mail-gmail":
			installed["gmail"] = true
		case "mail-graph":
			installed["graph"] = true
		case "mail-fake":
			installed["fake"] = true
		}
	}
	for i := range out {
		out[i].Installed = installed[out[i].Provider]
	}
	return out
}

// MailChecks is what doctor adds for each installed mailbox.
func (a *App) MailChecks(ctx context.Context) []DoctorCheck {
	var out []DoctorCheck
	for _, st := range a.MailStatuses() {
		if !st.Installed && st.Provider != "fake" && st.Provider != "gmail" && st.Provider != "graph" {
			continue
		}
		if !st.Installed {
			continue
		}
		checked := st
		if st.Provider == "gmail" || st.Provider == "graph" {
			checked = a.mail.Check(ctx, st.Provider)
			checked.Installed = true
		}
		level := "info"
		ok := true
		detail := checked.RefreshDetail
		if checked.Connected && !checked.RefreshOK && checked.Provider != "fake" {
			ok = false
			level = "error"
			if detail == "" {
				detail = "refresh failed"
			}
		}
		if checked.Throttle.Active {
			detail = strings.TrimSpace(detail + " · throttled until " + checked.Throttle.Until.Local().Format(time.RFC3339))
		}
		if checked.Connected {
			scope := "no send scope yet"
			if checked.SendGranted {
				scope = "send scope granted"
			}
			detail = strings.TrimSpace(detail + " · " + scope)
		}
		out = append(out, DoctorCheck{Name: "mail:" + checked.Provider, OK: ok, Detail: detail, Level: level})
	}
	return out
}

// MailSetup stores a public client id for Gmail or Outlook.
func (a *App) MailSetup(provider, clientID string, calendar bool) error {
	return a.mail.Setup(provider, clientID, calendar)
}

// MailConnect starts provider sign-in. The demo mailbox connects without a browser.
func (a *App) MailConnect(ctx context.Context, provider string) error {
	err := a.mail.Connect(ctx, provider)
	if err == nil {
		a.logMail(provider, "mail.connect", "Connected "+provider)
	}
	return err
}

// MailDisconnect drops a mailbox token.
func (a *App) MailDisconnect(provider string) error {
	if err := a.mail.Disconnect(provider); err != nil {
		return err
	}
	a.logMail(provider, "mail.disconnect", "Disconnected "+provider)
	return nil
}

// MailThreads lists a mailbox for a plugin that declared mail.read.
// Allowed reads are not written to the ledger; the UI polls this often.
func (a *App) MailThreads(ctx context.Context, plugin, provider string) ([]mail.Thread, error) {
	if err := a.gate(plugin, "mail.read", nil, "", false); err != nil {
		return nil, err
	}
	return a.mail.Threads(ctx, provider)
}

// MailSend sends one message. The plugin must declare mail.send and hold an approved card that covers sends.
func (a *App) MailSend(ctx context.Context, plugin string, req mail.SendRequest) (mail.SendResult, error) {
	if err := a.gate(plugin, "mail.send", []string{model.EffectSends}, req.CardID, true); err != nil {
		return mail.SendResult{}, err
	}
	res, err := a.mail.Send(ctx, req)
	if err != nil {
		return mail.SendResult{}, err
	}
	a.logMail(plugin, "mail.sent", "Sent to "+res.To+" · "+res.Subject)
	return res, nil
}

// MailUndo pulls back a demo send. Real providers refuse.
func (a *App) MailUndo(plugin, provider, messageID string) error {
	if _, err := a.pluginRecord(plugin); err != nil {
		return err
	}
	if err := a.mail.Undo(provider, messageID); err != nil {
		return err
	}
	a.logMail(plugin, "mail.undo", "Undid send "+messageID)
	return nil
}

// gate applies the same guardrails as da check. audit records denials in the ledger.
func (a *App) gate(plugin, permission string, effects []string, cardID string, audit bool) error {
	rec, err := a.pluginRecord(plugin)
	if err != nil {
		return err
	}
	effects, err = guardrails.NormalizeEffects(effects)
	if err != nil {
		return err
	}
	approved := false
	if cardID != "" {
		snap := a.store.Snapshot()
		if card, ok := findCard(snap.Cards, cardID); ok && cardCovers(card, plugin, permission, effects) {
			approved = true
		}
	}
	res := a.evaluate(rec, permission, effects, approved)
	if res.Decision != model.ModeAllowed {
		if audit || res.Decision == model.ModeNever {
			a.rememberCheck(plugin, permission, effects, cardID, res)
			kind := "guardrail.check"
			if res.Decision == model.ModeNever {
				kind = "permission.denied"
			}
			a.logMail(plugin, kind, res.Reason)
		}
		return fmt.Errorf("%w: %s", ErrDenied, res.Reason)
	}
	return nil
}

func (a *App) logMail(plugin, kind, message string) {
	a.log(ledger.Entry{Plugin: plugin, Kind: kind, Message: message})
}

// PluginInbox returns this plugin's cards, including bodies, for its own UI.
func (a *App) PluginInbox(plugin string) ([]model.Card, error) {
	if _, err := a.pluginRecord(plugin); err != nil {
		return nil, err
	}
	snap := a.store.Snapshot()
	var out []model.Card
	for _, c := range snap.Cards {
		if c.Plugin != plugin {
			continue
		}
		switch c.Status {
		case model.StatusPending, model.StatusSnoozed, model.StatusDone, model.StatusApproved, model.StatusFailed:
			out = append(out, c)
		}
	}
	return out, nil
}

// PluginApprove approves one of the plugin's own cards. Guardrails still run.
func (a *App) PluginApprove(plugin, id string) (model.Card, error) {
	if err := a.ownCard(plugin, id); err != nil {
		return model.Card{}, err
	}
	return a.Approve(id)
}

// PluginEdit edits one of the plugin's own waiting cards.
func (a *App) PluginEdit(plugin, id string, edit Edit) (model.Card, error) {
	if err := a.ownCard(plugin, id); err != nil {
		return model.Card{}, err
	}
	return a.Edit(id, edit)
}

// PluginReject rejects one of the plugin's own cards.
func (a *App) PluginReject(plugin, id, reason string) (model.Card, error) {
	if err := a.ownCard(plugin, id); err != nil {
		return model.Card{}, err
	}
	return a.Reject(id, reason)
}

// PluginSnooze snoozes one of the plugin's own cards.
func (a *App) PluginSnooze(plugin, id string, until time.Time) (model.Card, error) {
	if err := a.ownCard(plugin, id); err != nil {
		return model.Card{}, err
	}
	return a.Snooze(id, until)
}

func (a *App) ownCard(plugin, id string) error {
	owner, err := a.cardPlugin(id)
	if err != nil {
		return err
	}
	if owner != plugin {
		return fmt.Errorf("%w: that card belongs to another plugin", ErrDenied)
	}
	return nil
}
