// Package ledger is the append-only activity log.
// Rows are JSON lines. There is no update and no delete.
package ledger

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vicgupta/dap/internal/id"
	"github.com/vicgupta/dap/internal/model"
)

const (
	KindInstall  = "plugin.install"
	KindRemove   = "plugin.remove"
	KindStart    = "plugin.start"
	KindStop     = "plugin.stop"
	KindRun      = "schedule.run"
	KindMissed   = "schedule.missed"
	KindProposed = "card.proposed"
	KindApproved = "card.approved"
	KindEdited   = "card.edited"
	KindRejected = "card.rejected"
	KindSnoozed  = "card.snoozed"
	KindActed    = "card.acted"
	KindBlocked  = "card.blocked"
	KindFailed   = "card.failed"
	KindCheck    = "guardrail.check"
	KindDenied   = "permission.denied"
	KindOutcome  = "outcome"
	KindTile     = "tile.write"
	KindLog      = "plugin.log"
	KindChange   = "config.applied"
)

// Entry is one immutable activity row.
type Entry struct {
	ID       string           `json:"id"`
	TS       time.Time        `json:"ts"`
	Plugin   string           `json:"plugin,omitempty"`
	Kind     string           `json:"kind"`
	Message  string           `json:"message"`
	CardID   string           `json:"card_id,omitempty"`
	Evidence []model.Evidence `json:"evidence,omitempty"`
	Data     map[string]any   `json:"data,omitempty"`
}

// Ledger appends JSONL to a single file.
type Ledger struct {
	path   string
	mu     sync.Mutex
	now    func() time.Time
	closed bool
}

// Open creates the ledger file if needed. The file mode is 0600.
func Open(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "ledger.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return &Ledger{path: path, now: time.Now}, nil
}

// Path returns the JSONL file path.
func (l *Ledger) Path() string { return l.path }

// Append writes one row and fsyncs it. The id and timestamp are filled when empty.
// Close fsyncs the ledger and rejects later appends.
func (l *Ledger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	f, err := os.OpenFile(l.path, os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func (l *Ledger) Append(e Entry) (Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return Entry{}, errors.New("ledger is closed")
	}
	if e.ID == "" {
		e.ID = id.New("l")
	}
	if e.TS.IsZero() {
		e.TS = l.now().UTC()
	} else {
		e.TS = e.TS.UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return Entry{}, err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return Entry{}, err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return Entry{}, err
	}
	if err := f.Sync(); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// List reads every row. A corrupt line is returned as an error rather than skipped,
// because the file is supposed to be append-only and intact.
func (l *Ledger) List() ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(l.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Bytes()
		if len(text) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(text, &e); err != nil {
			return nil, fmt.Errorf("ledger line %d: %w", line, err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Recent returns the last n entries, newest first.
func (l *Ledger) Recent(n int) ([]Entry, error) {
	all, err := l.List()
	if err != nil {
		return nil, err
	}
	if n <= 0 || n > len(all) {
		n = len(all)
	}
	out := make([]Entry, 0, n)
	for i := len(all) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, all[i])
	}
	return out, nil
}
