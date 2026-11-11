package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/store"
)

func TestInstallRunsOnce(t *testing.T) {
	app := openTest(t)
	dir := filepath.Join(repoRoot(t), "templates", "scheduled-checklist")
	if _, err := app.Install(dir, true); err != nil {
		t.Fatal(err)
	}
	rec := app.store.Snapshot().Plugins["scheduled-checklist"]
	if rec == nil || rec.Run.LastRun == nil || rec.Run.LastTrigger != "install" {
		t.Fatalf("%+v", rec)
	}
	if countPending(app, "scheduled-checklist") == 0 {
		t.Fatal("install did not post the checklist cards")
	}
}

func TestAutoInstallRunsOnce(t *testing.T) {
	app := openTest(t)
	if err := app.StartPlugins(); err != nil {
		t.Fatal(err)
	}
	if err := app.MailConnect(context.Background(), "fake"); err != nil {
		t.Fatal(err)
	}
	job := waitJob(t, app, CreateRequest{Template: "daily-digest", AutoInstall: true})
	if job.Status != createInstalled {
		t.Fatalf("%+v", job)
	}
	rec := app.store.Snapshot().Plugins["daily-digest"]
	if rec == nil || rec.Run.LastRun == nil {
		t.Fatal("auto-install did not run")
	}
	if countPending(app, "daily-digest") == 0 {
		t.Fatalf("no digest card, tile %+v", rec.Tile)
	}
}

func TestChangeCardApprovalRunsOnce(t *testing.T) {
	app := openTest(t)
	if err := app.StartPlugins(); err != nil {
		t.Fatal(err)
	}
	if err := app.MailConnect(context.Background(), "fake"); err != nil {
		t.Fatal(err)
	}
	job := waitJob(t, app, CreateRequest{Template: "reply-drafter", AutoInstall: false})
	if job.Status != createReady || job.Card == nil {
		t.Fatalf("%+v", job)
	}
	done := make(chan error, 1)
	go func() {
		_, err := app.Approve(job.Card.ID)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("approve deadlocked")
	}
	rec := app.store.Snapshot().Plugins["reply-drafter"]
	if rec == nil || rec.Run.LastRun == nil {
		t.Fatal("approval did not run the plugin")
	}
	if countPending(app, "reply-drafter") == 0 {
		t.Fatalf("no reply card, tile %+v", rec.Tile)
	}
}

func TestDeclarativeTemplatesFirstRun(t *testing.T) {
	app := openTest(t)
	if err := app.MailConnect(context.Background(), "fake"); err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	names := []string{"inbox-triage", "follow-up-nudger", "daily-digest", "reply-drafter", "scheduled-checklist"}
	for _, name := range names {
		if _, err := app.Install(filepath.Join(root, "templates", name), true); err != nil {
			t.Fatal(name, err)
		}
		rec := app.store.Snapshot().Plugins[name]
		if rec == nil || rec.Run.LastRun == nil {
			t.Fatalf("%s did not run at install", name)
		}
		if countPending(app, name) == 0 {
			t.Fatalf("%s first run produced no card; tile=%+v", name, rec.Tile)
		}
	}
}

func TestMailCursorSurvivesEmptyRun(t *testing.T) {
	app := openTest(t)
	dir := filepath.Join(repoRoot(t), "templates", "inbox-triage")
	if _, err := app.Install(dir, true); err != nil {
		t.Fatal(err)
	}
	if err := app.RunPlugin("inbox-triage", "manual"); err != nil {
		t.Fatal(err)
	}
	rec := app.store.Snapshot().Plugins["inbox-triage"]
	if rec.Run.LastRun == nil {
		t.Fatal("last run was not recorded")
	}
	if rec.Run.MailChecked != nil {
		t.Fatal("mail cursor moved without a mailbox")
	}
	if n := countPending(app, "inbox-triage"); n != 0 {
		t.Fatalf("empty run posted %d cards", n)
	}
	future := time.Now().Add(24 * time.Hour).UTC()
	if err := app.store.Update(func(st *store.State) error {
		st.Plugins["inbox-triage"].Run.LastRun = &future
		st.Plugins["inbox-triage"].Run.MailChecked = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.MailConnect(context.Background(), "fake"); err != nil {
		t.Fatal(err)
	}
	if err := app.RunPlugin("inbox-triage", "manual"); err != nil {
		t.Fatal(err)
	}
	rec = app.store.Snapshot().Plugins["inbox-triage"]
	if rec.Run.MailChecked == nil {
		t.Fatal("successful read did not move the mail cursor")
	}
	if countPending(app, "inbox-triage") == 0 {
		t.Fatal("existing mail stayed hidden after the mailbox was connected")
	}
}

func TestRemoveThenRecreate(t *testing.T) {
	app := openTest(t)
	job := waitJob(t, app, CreateRequest{Template: "scheduled-checklist", AutoInstall: true})
	if job.Status != createInstalled {
		t.Fatalf("%+v", job)
	}
	card, err := app.Propose("scheduled-checklist", model.CardDraft{
		Title:      "Leftover step",
		Summary:    "Still waiting",
		Kind:       model.KindAction,
		Permission: "workflow.run",
	}, model.ActorPlugin)
	if err != nil {
		t.Fatal(err)
	}
	draft := filepath.Join(app.DataDir, "drafts", "scheduled-checklist")
	if _, err := os.Stat(draft); err != nil {
		t.Fatalf("draft folder: %v", err)
	}
	if err := app.Remove("scheduled-checklist"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(draft); !os.IsNotExist(err) {
		t.Fatalf("draft remains: %v", err)
	}
	found := false
	for _, c := range app.store.Snapshot().Cards {
		if c.ID != card.ID {
			continue
		}
		found = true
		if c.Status != model.StatusDismissed {
			t.Fatalf("status %s", c.Status)
		}
	}
	if !found {
		t.Fatal("pending card was deleted instead of dismissed")
	}
	if countPending(app, "scheduled-checklist") != 0 {
		t.Fatal("dismissed cards are still in the inbox")
	}
	rows, err := app.Activity(100)
	if err != nil {
		t.Fatal(err)
	}
	dismissed := false
	for _, row := range rows {
		if row.Kind == ledger.KindDismissed && row.CardID == card.ID {
			dismissed = true
		}
	}
	if !dismissed {
		t.Fatalf("ledger missing dismiss: %v", kinds(rows))
	}
	again := waitJob(t, app, CreateRequest{Template: "scheduled-checklist", AutoInstall: true})
	if again.Status != createInstalled {
		t.Fatalf("%+v", again)
	}
	entries, err := os.ReadDir(filepath.Join(app.DataDir, "drafts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "draft-") {
			t.Fatalf("leftover draft %s", e.Name())
		}
	}
}

func countPending(app *App, plugin string) int {
	n := 0
	for _, c := range app.Inbox() {
		if c.Plugin == plugin && c.Status == model.StatusPending {
			n++
		}
	}
	return n
}
