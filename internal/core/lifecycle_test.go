package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/guardrails"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/store"
)

func TestPluginsRunConcurrentlyAndApprovalActs(t *testing.T) {
	app := openTest(t)
	root := repoRoot(t)
	if _, err := app.Install(filepath.Join(root, "plugins", "hello-watch"), false); err != nil {
		t.Fatal(err)
	}
	preview, err := app.Install(filepath.Join(root, "plugins", "hello-watch"), false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != "approval_required" || len(preview.Permissions) != 1 {
		t.Fatalf("preview %+v", preview)
	}
	if _, err := app.Install(filepath.Join(root, "plugins", "hello-watch"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Install(filepath.Join(root, "plugins", "workflow"), true); err != nil {
		t.Fatal(err)
	}
	if err := app.StartPlugins(); err != nil {
		t.Fatal(err)
	}
	if !app.host.Running("hello-watch") || !app.host.Running("workflow") {
		t.Fatalf("hello %v workflow %v stderr hello %q workflow %q",
			app.host.Running("hello-watch"), app.host.Running("workflow"),
			app.host.Stderr("hello-watch"), app.host.Stderr("workflow"))
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- app.RunPlugin("hello-watch", "manual")
	}()
	go func() {
		defer wg.Done()
		errs <- app.RunPlugin("workflow", "manual")
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("run: %v\nhello stderr: %s\nworkflow stderr: %s", err, app.host.Stderr("hello-watch"), app.host.Stderr("workflow"))
		}
	}

	inbox := app.Inbox()
	var helloCard, sendCard model.Card
	for _, c := range inbox {
		if c.Plugin == "hello-watch" && c.Status == model.StatusPending {
			helloCard = c
		}
		if c.Plugin == "workflow" && c.Status == model.StatusPending {
			sendCard = c
		}
	}
	if helloCard.ID == "" || sendCard.ID == "" {
		t.Fatalf("expected both cards, inbox=%+v workflow stderr=%s", inbox, app.host.Stderr("workflow"))
	}
	if len(sendCard.Effects) != 1 || sendCard.Effects[0] != "sends" {
		t.Fatalf("send card effects %+v", sendCard.Effects)
	}
	if sendCard.Risk != model.ModeAskFirst {
		t.Fatalf("risk %s", sendCard.Risk)
	}

	// A second hello run is not blocked by the pending card.
	if err := app.RunPlugin("hello-watch", "manual"); err != nil {
		t.Fatal(err)
	}
	pendingHello := 0
	for _, c := range app.Inbox() {
		if c.Plugin == "hello-watch" && c.Status == model.StatusPending {
			pendingHello++
		}
	}
	if pendingHello < 2 {
		t.Fatalf("pending cards must not block the plugin, got %d", pendingHello)
	}

	done, err := app.Approve(helloCard.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != model.StatusDone || done.Result == nil || done.Result.Summary == "" {
		t.Fatalf("act result %+v", done)
	}

	acted, err := app.Approve(sendCard.ID)
	if err != nil {
		t.Fatalf("approve send: %v stderr %s", err, app.host.Stderr("workflow"))
	}
	if acted.Status != model.StatusDone {
		t.Fatalf("workflow card %+v", acted)
	}

	rows, err := app.Activity(200)
	if err != nil {
		t.Fatal(err)
	}
	need := map[string]bool{
		ledger.KindProposed: false,
		ledger.KindApproved: false,
		ledger.KindActed:    false,
		ledger.KindLog:      false,
	}
	for _, row := range rows {
		if _, ok := need[row.Kind]; ok {
			need[row.Kind] = true
		}
	}
	for kind, ok := range need {
		if !ok {
			t.Fatalf("ledger missing %s in %+v", kind, kinds(rows))
		}
	}

	// Undeclared permission is denied.
	res, err := app.Check("hello-watch", "mail.send", []string{"sends"}, "")
	if !errors.Is(err, ErrDenied) || res.Decision != model.ModeNever {
		t.Fatalf("mail.send should be denied, res=%+v err=%v", res, err)
	}
	status, body := pluginPost(t, app, "hello-watch", "/call", map[string]any{
		"permission": "files.write",
		"summary":    "write a file",
	})
	if status != http.StatusForbidden || body["decision"] != model.ModeNever {
		t.Fatalf("http deny %d %v", status, body)
	}

	// A send without a card is not allowed, even for a declared ask-first permission.
	status, body = pluginPost(t, app, "workflow", "/call", map[string]any{
		"permission": "share.team",
		"effects":    []string{"sends"},
		"summary":    "send the note",
	})
	if status != http.StatusForbidden || body["decision"] != model.ModeAskFirst {
		t.Fatalf("send without card %d %v", status, body)
	}

	// secrets.read is not declared.
	if _, err := app.SecretGet("workflow", "token"); !errors.Is(err, ErrDenied) {
		t.Fatalf("secret get: %v", err)
	}

	if _, err := app.Reject(sendCard.ID, ""); err == nil {
		t.Fatal("reject should require a reason")
	}
}

func TestMissedRunCatchesUpOnce(t *testing.T) {
	app := openTest(t)
	root := repoRoot(t)
	if _, err := app.Install(filepath.Join(root, "plugins", "hello-watch"), true); err != nil {
		t.Fatal(err)
	}
	if err := app.StartPlugins(); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-5 * time.Hour)
	if err := app.store.Update(func(st *store.State) error {
		st.Plugins["hello-watch"].Run.NextRun = past
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	app.Tick(time.Now())
	app.Tick(time.Now())
	rows, err := app.Activity(50)
	if err != nil {
		t.Fatal(err)
	}
	missed := 0
	runs := 0
	for _, row := range rows {
		if row.Kind == ledger.KindMissed && row.Plugin == "hello-watch" {
			missed++
		}
		if row.Kind == ledger.KindRun && row.Plugin == "hello-watch" {
			runs++
		}
	}
	if missed != 1 || runs != 1 {
		t.Fatalf("missed=%d runs=%d kinds=%v stderr=%s", missed, runs, kinds(rows), app.host.Stderr("hello-watch"))
	}
	home := app.Home()
	var hello PluginSummary
	for _, p := range home.Plugins {
		if p.Name == "hello-watch" {
			hello = p
		}
	}
	if hello.Missed == nil || hello.Missed.Count < 2 || hello.Missed.Text == "" {
		t.Fatalf("missed banner %+v", hello.Missed)
	}
}

func TestChangeCardAppliesAfterApproval(t *testing.T) {
	app := openTest(t)
	root := repoRoot(t)
	if _, err := app.Install(filepath.Join(root, "plugins", "workflow"), true); err != nil {
		t.Fatal(err)
	}
	if err := app.StartPlugins(); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{
		"config": map[string]any{
			"every": "30m",
			"steps": []any{
				map[string]any{"id": "only", "title": "Only step", "detail": "Safe.", "effects": []any{}},
			},
		},
	})
	card, err := app.Propose("workflow", model.CardDraft{
		Title:      "Update workflow steps",
		Summary:    "Replace the step list.",
		Kind:       model.KindChange,
		Permission: "workflow.rules",
		Payload:    payload,
	}, model.ActorPlugin)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != model.StatusPending {
		t.Fatalf("change cards wait, got %s", card.Status)
	}
	if _, err := app.Approve(card.ID); err != nil {
		t.Fatalf("approve change: %v stderr %s", err, app.host.Stderr("workflow"))
	}
	ctx, err := app.Context("workflow")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Config["every"] != "30m" {
		t.Fatalf("config %+v", ctx.Config)
	}
	if ctx.Schedule.Every != "30m" {
		t.Fatalf("schedule %+v", ctx.Schedule)
	}
}

func TestGuardrailOverrideNeverBlocks(t *testing.T) {
	app := openTest(t)
	root := repoRoot(t)
	if _, err := app.Install(filepath.Join(root, "plugins", "workflow"), true); err != nil {
		t.Fatal(err)
	}
	if err := app.SetRule(guardrails.Rule{Plugin: "workflow", Permission: "workflow.run", Decision: model.ModeNever}); err != nil {
		t.Fatal(err)
	}
	res, err := app.Check("workflow", "workflow.run", nil, "")
	if !errors.Is(err, ErrDenied) || res.Decision != model.ModeNever {
		t.Fatalf("override %+v %v", res, err)
	}
}

func openTest(t *testing.T) *App {
	t.Helper()
	app, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "plugins", "hello-watch", "plugin.yaml")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

func pluginPost(t *testing.T, app *App, plugin, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, app.PluginAPIBase()+"/v1/"+plugin+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+app.Token(plugin))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func kinds(rows []ledger.Entry) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Plugin+":"+r.Kind)
	}
	return out
}
