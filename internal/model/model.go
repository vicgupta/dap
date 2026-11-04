// Package model holds the documents the core, the plugins, and the UI share.
package model

import (
	"encoding/json"
	"time"
)

const (
	TypeWatcher    = "watcher"
	TypeCapability = "capability"
	TypeWorkflow   = "workflow"

	ModeAllowed  = "allowed"
	ModeAskFirst = "ask_first"
	ModeNever    = "never"

	EffectSends   = "sends"
	EffectSpends  = "spends"
	EffectDeletes = "deletes"

	KindAction = "action"
	KindChange = "change"

	ActorPlugin   = "plugin"
	ActorExternal = "external"
	ActorUser     = "user"

	StatusPending  = "pending"
	StatusActing   = "acting"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	StatusSnoozed  = "snoozed"
	StatusDone     = "done"
	StatusFailed   = "failed"
	StatusBlocked  = "blocked"

	HealthOK       = "ok"
	HealthDegraded = "degraded"
	HealthError    = "error"
)

// Evidence is a pointer back to why a card or ledger row exists.
type Evidence struct {
	Label string `json:"label"`
	Ref   string `json:"ref,omitempty"`
}

// TileLine is one row of the small status tile on Home.
type TileLine struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Tile is the plugin's current summary for Home.
type Tile struct {
	Title     string     `json:"title,omitempty"`
	Status    string     `json:"status,omitempty"`
	Lines     []TileLine `json:"lines,omitempty"`
	UpdatedAt time.Time  `json:"updated_at,omitempty"`
}

// Health is what initialize and doctor surface.
type Health struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Observation is one thing a plugin noticed during watch.
type Observation struct {
	ID      string         `json:"id"`
	At      time.Time      `json:"at"`
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data,omitempty"`
}

// Metric is one number from report.
type Metric struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value any    `json:"value"`
}

// Card is one item in the shared decision inbox.
type Card struct {
	ID          string          `json:"id"`
	Plugin      string          `json:"plugin"`
	Title       string          `json:"title"`
	Summary     string          `json:"summary,omitempty"`
	Body        string          `json:"body,omitempty"`
	Kind        string          `json:"kind"`
	Permission  string          `json:"permission,omitempty"`
	Effects     []string        `json:"effects,omitempty"`
	Risk        string          `json:"risk"`
	Status      string          `json:"status"`
	Actor       string          `json:"actor"`
	Reason      string          `json:"reason,omitempty"`
	SnoozeUntil *time.Time      `json:"snooze_until,omitempty"`
	Evidence    []Evidence      `json:"evidence,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Editable    bool            `json:"editable"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ActedAt     *time.Time      `json:"acted_at,omitempty"`
	Result      *ActResult      `json:"result,omitempty"`
}

// ActResult is what a plugin returns from act, or what an external agent posts as an outcome.
type ActResult struct {
	Status   string         `json:"status"`
	Summary  string         `json:"summary"`
	Evidence []Evidence     `json:"evidence,omitempty"`
	Output   map[string]any `json:"output,omitempty"`
}

// CardDraft is a proposal before the core assigns an id and a risk.
type CardDraft struct {
	Title      string          `json:"title"`
	Summary    string          `json:"summary,omitempty"`
	Body       string          `json:"body,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	Permission string          `json:"permission,omitempty"`
	Effects    []string        `json:"effects,omitempty"`
	Evidence   []Evidence      `json:"evidence,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Editable   *bool           `json:"editable,omitempty"`
	Actor      string          `json:"actor,omitempty"`
}

// SensitiveEffect reports whether the effect is one the core never runs unapproved.
func SensitiveEffect(effect string) bool {
	switch effect {
	case EffectSends, EffectSpends, EffectDeletes:
		return true
	default:
		return false
	}
}
