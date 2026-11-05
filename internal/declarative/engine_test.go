package declarative

import (
	"context"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/mail"
	"github.com/vicgupta/dap/internal/mailbox"
	"github.com/vicgupta/dap/internal/model"
)

func TestDryRunInboxAndNudger(t *testing.T) {
	now := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	threads := mail.Seed(now)
	inbox := mustCompile(t, "templates/inbox-triage/plugin.yaml")
	_, text, err := Sample(inbox, nil, now, threads)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(text, "Sample run", "Inbox triage") {
		t.Fatal(text)
	}
	nudge := mustCompile(t, "templates/follow-up-nudger/plugin.yaml")
	out, _, err := Sample(nudge, nil, now, threads)
	if err != nil {
		t.Fatal(err)
	}
	drafts := 0
	for _, c := range out.Cards {
		if c.Permission == "mail.draft" {
			drafts++
			if len(c.Effects) != 0 {
				t.Fatal("a draft must not send")
			}
		}
		if c.Permission == "mail.send" {
			t.Fatal("nudger must not send")
		}
	}
	if drafts < 1 {
		t.Fatalf("cards %+v", titles(out))
	}
}

func TestActWaitsAndRecords(t *testing.T) {
	raw := []byte(`
apiVersion: dap/v1
kind: declarative
name: labeler
version: 0.1.0
min_dap_version: 0.1.0
triggers:
  - schedule: every 1h
inputs:
  - service: email
    window: 7d
rules:
  - id: all
    when:
      all:
        - newer_than: 7d
output:
  actions:
    - type: label
      rule: all
      label: Review
    - type: send
      rule: all
      body: nudge
      subject: reply
`)
	p, err := Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	eng := New(p, map[string]any{})
	eng.Now = func() time.Time { return now }
	box := &recordMail{threads: mail.Seed(now)}
	eng.Mail = box
	mem := &Memory{}
	eng.State = mem
	if _, err := eng.Watch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	proposed := eng.Propose()
	if len(proposed.Cards) == 0 {
		t.Fatal("no cards")
	}
	if box.drafts != 0 || box.sends != 0 {
		t.Fatal("propose must not draft or send")
	}
	var label, send model.Card
	for _, c := range proposed.Cards {
		if c.Permission == "mail.send" && len(c.Effects) == 1 && c.Effects[0] == "sends" && send.Title == "" {
			send = model.Card{ID: "c1", Title: c.Title, Body: c.Body, Payload: c.Payload, Permission: c.Permission, Effects: c.Effects}
		}
		if c.Permission == "mail.label" && label.Title == "" {
			label = model.Card{Title: c.Title, Body: c.Body, Payload: c.Payload, Permission: c.Permission}
		}
	}
	if label.Title == "" || send.Title == "" {
		t.Fatalf("missing cards %+v", titles(proposed))
	}
	res, err := eng.Act(context.Background(), label)
	if err != nil || res.Status != "done" {
		t.Fatalf("%+v %v", res, err)
	}
	if len(mem.Blob.Labels) != 1 || mem.Blob.Labels[0].Label != "Review" {
		t.Fatalf("%+v", mem.Blob)
	}
	res, err = eng.Act(context.Background(), send)
	if err != nil || res.Status != "done" || box.sends != 1 {
		t.Fatalf("%+v %v sends %d", res, err, box.sends)
	}
}

func TestChecklistSample(t *testing.T) {
	p := mustCompile(t, "templates/scheduled-checklist/plugin.yaml")
	out, text, err := Sample(p, nil, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(text, "Sample run") {
		t.Fatal(text)
	}
	sends := 0
	for _, c := range out.Cards {
		if c.Permission != "workflow.run" {
			t.Fatalf("%+v", c)
		}
		if len(c.Effects) == 1 && c.Effects[0] == "sends" {
			sends++
		}
	}
	if len(out.Cards) != 3 || sends != 1 {
		t.Fatalf("%+v", titles(out))
	}
}

type recordMail struct {
	threads []mailbox.Thread
	drafts  int
	sends   int
}

func (r *recordMail) Providers(context.Context) ([]string, error) { return []string{"fake"}, nil }

func (r *recordMail) Threads(context.Context, string, string) ([]mailbox.Thread, error) {
	return r.threads, nil
}

func (r *recordMail) Draft(context.Context, mailbox.SendRequest) (mailbox.DraftResult, error) {
	r.drafts++
	return mailbox.DraftResult{Provider: "fake", Local: true}, nil
}

func (r *recordMail) Send(_ context.Context, req mailbox.SendRequest) (mailbox.SendResult, error) {
	r.sends++
	return mailbox.SendResult{Provider: "fake", To: req.To, Subject: req.Subject}, nil
}

func mustCompile(t *testing.T, rel string) Plugin {
	t.Helper()
	p, err := Compile(readRepo(t, rel))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func titles(out Outcome) []string {
	var s []string
	for _, c := range out.Cards {
		s = append(s, c.Title)
	}
	return s
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, part string) bool {
	return len(s) >= len(part) && (s == part || len(part) == 0 || indexOf(s, part) >= 0)
}

func indexOf(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
