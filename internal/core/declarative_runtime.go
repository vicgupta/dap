package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vicgupta/dap/internal/declarative"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/mail"
	"github.com/vicgupta/dap/internal/mailbox"
	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginhost"
)

func (a *App) startDeclarative(name string, m manifest.Manifest, cfg map[string]any, token string) error {
	if m.Declarative == nil {
		err := fmt.Errorf("declarative plugin %s has no spec", name)
		a.failStart(name, err)
		return err
	}
	eng := declarative.New(*m.Declarative, cfg)
	eng.Now = a.now
	eng.Mail = declMailbox{app: a, plugin: name}
	eng.State = declState{app: a, plugin: name}
	a.mutateRT(name, func(rt *runtime) {
		rt.Token = token
		rt.NeedsApproval = false
		rt.StartError = ""
	})
	if err := a.host.StartLocal(name, declarativeHandler(eng)); err != nil {
		a.failStart(name, err)
		return err
	}
	if err := a.initialize(name); err != nil {
		a.host.Stop(name)
		a.failStart(name, err)
		a.log(ledger.Entry{Plugin: name, Kind: ledger.KindStart, Message: "Failed to start: " + err.Error()})
		return err
	}
	a.log(ledger.Entry{Plugin: name, Kind: ledger.KindStart, Message: "Started " + name + " (declarative, in-process)"})
	return nil
}

func declarativeHandler(eng *declarative.Engine) pluginhost.LocalFunc {
	return func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		switch method {
		case pluginhost.MethodInitialize:
			ready, status, msg := eng.Initialize()
			if status == "" {
				status = model.HealthOK
			}
			return pluginhost.InitResult{
				Ready:     ready,
				Interface: "dap.plugin.v1",
				Health:    model.Health{Status: status, Message: msg},
			}, nil
		case pluginhost.MethodWatch:
			var p pluginhost.WatchParams
			if len(params) > 0 && string(params) != "null" {
				_ = json.Unmarshal(params, &p)
			}
			out, err := eng.Watch(ctx, p.Since)
			if err != nil {
				return nil, err
			}
			if out.Observations == nil {
				out.Observations = []model.Observation{}
			}
			return pluginhost.WatchResult{Observations: out.Observations, Tile: out.Tile, MailRead: out.MailRead}, nil
		case pluginhost.MethodPropose:
			out := eng.Propose()
			if out.Cards == nil {
				out.Cards = []model.CardDraft{}
			}
			return pluginhost.ProposeResult{Cards: out.Cards}, nil
		case pluginhost.MethodAct:
			var p pluginhost.ActParams
			if len(params) > 0 && string(params) != "null" {
				if err := json.Unmarshal(params, &p); err != nil {
					return nil, err
				}
			}
			return eng.Act(ctx, p.Card)
		case pluginhost.MethodReport:
			return pluginhost.ReportResult{Metrics: eng.Report()}, nil
		case pluginhost.MethodShutdown:
			return pluginhost.ShutdownResult{OK: true}, nil
		default:
			return nil, fmt.Errorf("unknown method %s", method)
		}
	}
}

func (a *App) declarativeSample(m manifest.Manifest, given map[string]any) string {
	if m.Declarative == nil {
		return ""
	}
	_, text, err := declarative.Sample(*m.Declarative, given, a.now(), mail.Seed(a.now()))
	if err != nil {
		return "Sample run failed: " + err.Error()
	}
	return text
}

// declMailbox is the email service for one declarative plugin.
// Reads honor a guardrail of never or ask_first. Drafts and sends run
// only from act, which the core calls after approval.
type declMailbox struct {
	app    *App
	plugin string
}

func (m declMailbox) Providers(context.Context) ([]string, error) {
	if err := m.readAllowed(); err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	for _, st := range m.app.mail.Statuses() {
		if !st.Connected {
			continue
		}
		if st.Kind != "mail" && st.Provider != "fake" {
			continue
		}
		id := st.Provider
		if id == "graph" {
			id = "outlook"
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

func (m declMailbox) Threads(ctx context.Context, provider, window string) ([]mailbox.Thread, error) {
	if err := m.readAllowed(); err != nil {
		return nil, err
	}
	opt, err := mail.ParseListOptions("", window)
	if err != nil {
		return nil, err
	}
	return m.app.mail.ThreadsQuery(ctx, provider, opt)
}

func (m declMailbox) readAllowed() error {
	rec, err := m.app.pluginRecord(m.plugin)
	if err != nil {
		return err
	}
	res := m.app.evaluate(rec, "mail.read", nil, false)
	if res.Decision != model.ModeAllowed {
		return fmt.Errorf("%s", res.Reason)
	}
	return nil
}

func (m declMailbox) Draft(ctx context.Context, req mailbox.SendRequest) (mailbox.DraftResult, error) {
	return m.app.mail.Draft(ctx, req)
}

func (m declMailbox) Send(ctx context.Context, req mailbox.SendRequest) (mailbox.SendResult, error) {
	return m.app.MailSend(ctx, m.plugin, req)
}

type declState struct {
	app    *App
	plugin string
}

func (s declState) Load() (declarative.Blob, error) {
	raw, err := s.app.GetState(s.plugin)
	if err != nil {
		return declarative.Blob{}, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return declarative.Blob{}, nil
	}
	var b declarative.Blob
	if err := json.Unmarshal(raw, &b); err != nil {
		return declarative.Blob{}, nil
	}
	return b, nil
}

func (s declState) Save(b declarative.Blob) error {
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return s.app.PutState(s.plugin, raw)
}
