// Package fake is a demo mailbox so the inbox UI can run without a real account.
package mail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vicgupta/dap/internal/mailbox"
)

// Box is the on-disk demo mailbox.
type Box struct {
	path string
	mu   sync.Mutex
	data store
}

type store struct {
	Connected bool          `json:"connected"`
	Sent      []sentMessage `json:"sent"`
}

type sentMessage struct {
	ID      string    `json:"id"`
	Thread  string    `json:"thread"`
	To      string    `json:"to"`
	Subject string    `json:"subject"`
	Body    string    `json:"body"`
	At      time.Time `json:"at"`
	Undone  bool      `json:"undone"`
}

// OpenBox loads or creates the demo mailbox.
func OpenBox(dir string) (*Box, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	b := &Box{path: filepath.Join(dir, "fake.json")}
	raw, err := os.ReadFile(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			return b, b.save()
		}
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &b.data); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// Connect marks the demo mailbox connected. There is no token.
func (b *Box) Connect() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data.Connected = true
	return b.save()
}

// Disconnect drops the connection and keeps the sent log.
func (b *Box) Disconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data.Connected = false
	return b.save()
}

// Connected reports whether the demo mailbox is on.
func (b *Box) Connected() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Connected
}

// Threads returns the mockup conversations, aged from now.
func (b *Box) Threads(now time.Time) []mailbox.Thread {
	b.mu.Lock()
	on := b.data.Connected
	b.mu.Unlock()
	if !on {
		return nil
	}
	return Seed(now)
}

// Send records a message. The id is stable enough for undo.
func (b *Box) Send(req mailbox.SendRequest, now time.Time) (mailbox.SendResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.data.Connected {
		return mailbox.SendResult{}, errClosed
	}
	id := "fake_" + now.UTC().Format("150405.000")
	b.data.Sent = append(b.data.Sent, sentMessage{
		ID: id, Thread: req.ThreadID, To: req.To, Subject: req.Subject, Body: req.Body, At: now.UTC(),
	})
	if err := b.save(); err != nil {
		return mailbox.SendResult{}, err
	}
	return mailbox.SendResult{
		Provider: "fake", MessageID: id, ThreadID: req.ThreadID, To: req.To, Subject: req.Subject, SentAt: now.UTC(),
	}, nil
}

// Undo marks a recent demo send as not sent. Real providers cannot do this.
func (b *Box) Undo(id string, now time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.data.Sent {
		if b.data.Sent[i].ID != id {
			continue
		}
		if now.Sub(b.data.Sent[i].At) > 12*time.Minute {
			return errLate
		}
		b.data.Sent[i].Undone = true
		return b.save()
	}
	return errMissing
}

type strErr string

func (e strErr) Error() string { return string(e) }

const (
	errLate    strErr = "that send is older than 12 minutes"
	errMissing strErr = "message not found"
	errClosed  strErr = "demo mailbox is not connected"
)

func (b *Box) save() error {
	raw, err := json.MarshalIndent(b.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(b.path, append(raw, '\n'), 0o600)
}

// Seed is the example mailbox from the follow-ups mockup.
func Seed(now time.Time) []mailbox.Thread {
	return []mailbox.Thread{
		{
			ID: "dana", Provider: "fake", Subject: "Q4 proposal", WithName: "Dana Lee", WithEmail: "dana.lee@northwind.co",
			LastAt: now.Add(-4 * 24 * time.Hour), LastFromMe: true, Promise: true,
			Snippet:  `She asked for revised pricing on Oct 28 and you said "by Friday."`,
			Title:    "Send follow-up to Dana Lee re: Q4 proposal",
			Why:      `She asked for revised pricing on Oct 28 and you said "by Friday." No reply yet.`,
			Draft:    "Hi Dana — as promised, attached is the revised Q4 pricing with the volume tier we discussed. Happy to walk through it Tuesday if useful.\nBest, Vic",
			Evidence: "thread, Oct 28",
		},
		{
			ID: "marcus", Provider: "fake", Subject: "SOW v3", WithName: "Marcus Chen", WithEmail: "m.chen@acmeanalytics.com",
			LastAt: now.Add(-5 * 24 * time.Hour), LastFromMe: true, Promise: true,
			Snippet:  "Waiting on the signed SOW.",
			Title:    "Nudge Marcus Chen for the signed SOW",
			Why:      "Waiting 5 days. Kickoff on Nov 4 depends on it.",
			Draft:    "Hi Marcus — quick check on the SOW. If signature by Monday works, we can keep the Nov 4 kickoff.",
			Evidence: "thread, Oct 27",
		},
		{
			ID: "priya", Provider: "fake", Subject: "Thursday sync", WithName: "Priya Shah", WithEmail: "priya.shah@northwind.co",
			LastAt: now.Add(-26 * time.Hour), LastFromMe: false,
			Snippet:  "Can we lock Thursday at 2?",
			Title:    "Reply to Priya Shah: confirm Thursday 2pm",
			Why:      "Calendar is free · draft is one line",
			Draft:    "Thursday 2pm still works on my side. I'll send a hold.",
			Evidence: "thread, Oct 31",
		},
		{
			ID: "elena-deck", Provider: "fake", Subject: "Board deck", WithName: "Elena Ruiz", WithEmail: "elena.ruiz@northwind.co",
			LastAt: now.Add(-48 * time.Hour), LastFromMe: false,
			Snippet:  "Could you glance at the board deck before Friday?",
			Title:    "Reply to Elena Ruiz: Board deck",
			Why:      "She asked for a look before Friday.",
			Draft:    "Hi Elena — I can review the board deck this afternoon and send notes.",
			Evidence: "thread, Oct 30",
		},
		ack(now, "tom", "Tom Alvarez", "tom@northwind.co"),
		ack(now, "elena-ack", "Elena Ruiz", "elena.ruiz@northwind.co"),
		ack(now, "ravi", "Ravi Patel", "ravi.patel@northwind.co"),
		ack(now, "jun", "Jun Ito", "jun.ito@northwind.co"),
	}
}

func ack(now time.Time, id, name, email string) mailbox.Thread {
	return mailbox.Thread{
		ID: id, Provider: "fake", Subject: "Thanks", WithName: name, WithEmail: email,
		LastAt: now.Add(-6 * time.Hour), LastFromMe: false, Short: true,
		Snippet:  "Thanks, got it",
		Draft:    "Thanks, got it",
		Evidence: "thread",
	}
}
