package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/mail"
	"github.com/vicgupta/dap/internal/model"
)

func TestInboxFollowupsApprovesSendAndBlocksBareSend(t *testing.T) {
	app := openTest(t)
	root := repoRoot(t)
	if _, err := app.Install(filepath.Join(root, "plugins", "mail-fake"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Install(filepath.Join(root, "plugins", "inbox-followups"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Install(filepath.Join(root, "plugins", "hello-watch"), true); err != nil {
		t.Fatal(err)
	}
	if err := app.StartPlugins(); err != nil {
		t.Fatal(err)
	}
	if err := app.MailConnect(context.Background(), "fake"); err != nil {
		t.Fatal(err)
	}
	if err := app.RunPlugin("inbox-followups", "manual"); err != nil {
		t.Fatalf("run: %v stderr %s", err, app.host.Stderr("inbox-followups"))
	}
	var dana, acks model.Card
	for _, c := range app.Inbox() {
		if c.Plugin != "inbox-followups" {
			continue
		}
		if strings.Contains(c.Title, "Dana Lee") {
			dana = c
		}
		if strings.Contains(c.Title, "acknowledgements") {
			acks = c
		}
	}
	if dana.ID == "" || acks.ID == "" {
		t.Fatalf("missing cards: %+v stderr %s", titles(app), app.host.Stderr("inbox-followups"))
	}
	if len(dana.Effects) != 1 || dana.Effects[0] != "sends" || dana.Risk != model.ModeAskFirst {
		t.Fatalf("dana card %+v", dana)
	}
	_, err := app.MailSend(context.Background(), "inbox-followups", mail.SendRequest{Provider: "fake", ThreadID: "dana", To: "dana.lee@northwind.co", Subject: "Q4", Body: "nope"})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("send without a card should be denied, got %v", err)
	}
	_, err = app.MailThreads(context.Background(), "hello-watch", "fake")
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("hello-watch must not read mail, got %v", err)
	}
	recent, err := app.MailThreadsQuery(context.Background(), "inbox-followups", "fake", mail.ListOptions{NewerThan: "1d"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 4 {
		t.Fatalf("last 24h should keep the four recent acknowledgements, got %d", len(recent))
	}
	for _, th := range recent {
		if time.Since(th.LastAt) > 24*time.Hour {
			t.Fatalf("thread %s is older than a day: %s", th.ID, th.LastAt)
		}
	}
	done, err := app.Approve(dana.ID)
	if err != nil {
		t.Fatalf("approve: %v stderr %s", err, app.host.Stderr("inbox-followups"))
	}
	if done.Status != model.StatusDone || done.Result == nil || !strings.Contains(done.Result.Summary, "dana.lee") {
		t.Fatalf("result %+v", done.Result)
	}
	raw, err := os.ReadFile(filepath.Join(app.DataDir, "mail", "fake.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "dana.lee@northwind.co") || strings.Contains(string(raw), "access_token") {
		t.Fatalf("mailbox file: %s", raw)
	}
	rows, err := app.Activity(30)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.Kind] = true
	}
	for _, kind := range []string{"card.proposed", "card.approved", "card.acted", "mail.sent"} {
		if !seen[kind] {
			t.Fatalf("ledger missing %s in %v", kind, kinds(rows))
		}
	}
	doc := app.Doctor()
	found := false
	for _, c := range doc.Checks {
		if c.Name == "mail:fake" && c.OK && strings.Contains(c.Detail, "demo") {
			found = true
		}
	}
	if !found {
		t.Fatalf("doctor mail checks %+v", doc.Checks)
	}
	_ = json.Marshal
}

func titles(app *App) []string {
	var out []string
	for _, c := range app.Inbox() {
		out = append(out, c.Plugin+": "+c.Title)
	}
	return out
}
