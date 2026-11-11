// Package scheduler decides when a plugin is due.
//
// Intervals, cron expressions, and clock times are supported.
// If the laptop slept or DAP was not running, the overdue slot is recorded
// as a missed run and caught up once. Earlier slots are not replayed.
package scheduler

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/vicgupta/dap/internal/manifest"
)

// Grace is how late a run can be and still count as on time.
// Anything later is an honest miss, then a single catch-up.
const Grace = 2 * time.Minute

// Spec is a schedule with at most one of Every, Cron, or At set.
type Spec struct {
	Every      string
	Cron       string
	At         string
	Recurrence string
	Day        string
}

// FromManifest copies a manifest schedule into a Spec.
func FromManifest(s manifest.Schedule) Spec {
	return Spec{
		Every:      s.Every,
		Cron:       s.Cron,
		At:         s.At,
		Recurrence: s.Recurrence,
		Day:        s.Day,
	}
}

// State is the persisted cursor for one plugin.
// MailChecked is the last time a declarative plugin read mail successfully.
// It stays put when no mailbox is connected or the read fails.
type State struct {
	NextRun         time.Time  `json:"next_run"`
	LastRun         *time.Time `json:"last_run,omitempty"`
	MailChecked     *time.Time `json:"mail_checked,omitempty"`
	LastTrigger     string     `json:"last_trigger,omitempty"`
	LastStatus      string     `json:"last_status,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	MissedCount     int        `json:"missed_count,omitempty"`
	MissedAt        *time.Time `json:"missed_at,omitempty"`
	MissedScheduled *time.Time `json:"missed_scheduled,omitempty"`
	MissedReason    string     `json:"missed_reason,omitempty"`
}

// Decision is what Tick should do for one plugin.
type Decision struct {
	Fire     bool
	Trigger  string // "schedule" or "catchup"
	Missed   int
	MissedAt time.Time
	Reason   string
	Next     time.Time
}

// MissedReason is the honest explanation stored with a catch-up.
const MissedReason = "DAP was not running or the laptop was asleep"

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// Validate checks that a spec can produce a next time. An empty spec is valid (manual only).
func Validate(spec Spec) error {
	s := manifest.Schedule{
		Every:      spec.Every,
		Cron:       spec.Cron,
		At:         spec.At,
		Recurrence: spec.Recurrence,
		Day:        spec.Day,
	}
	return s.Validate()
}

// Next returns the first run strictly after from.
func Next(spec Spec, from time.Time) (time.Time, error) {
	switch {
	case spec.Every != "":
		d, err := time.ParseDuration(spec.Every)
		if err != nil {
			return time.Time{}, fmt.Errorf("every: %w", err)
		}
		if d <= 0 {
			return time.Time{}, fmt.Errorf("every must be positive")
		}
		return from.Add(d), nil
	case spec.Cron != "":
		sched, err := cronParser.Parse(spec.Cron)
		if err != nil {
			return time.Time{}, fmt.Errorf("cron: %w", err)
		}
		return sched.Next(from), nil
	case spec.At != "":
		return nextAt(spec, from)
	default:
		return time.Time{}, fmt.Errorf("plugin has no schedule")
	}
}

// Evaluate looks at the cursor and the clock.
// A zero NextRun is initialized to the following slot and does not fire.
func Evaluate(spec Spec, state State, now time.Time) (Decision, error) {
	if spec.Every == "" && spec.Cron == "" && spec.At == "" {
		return Decision{}, nil
	}
	if state.NextRun.IsZero() {
		next, err := Next(spec, now)
		if err != nil {
			return Decision{}, err
		}
		return Decision{Next: next}, nil
	}
	if now.Before(state.NextRun) {
		return Decision{Next: state.NextRun}, nil
	}
	next, err := Next(spec, now)
	if err != nil {
		return Decision{}, err
	}
	late := now.Sub(state.NextRun)
	if late > Grace {
		missed := countMissed(spec, state.NextRun, now)
		return Decision{
			Fire:     true,
			Trigger:  "catchup",
			Missed:   missed,
			MissedAt: state.NextRun,
			Reason:   MissedReason,
			Next:     next,
		}, nil
	}
	return Decision{Fire: true, Trigger: "schedule", Next: next}, nil
}

// countMissed counts scheduled instants in [first, now]. The catch-up runs
// once, and every one of those instants is reported as missed so the UI
// does not pretend they happened on time.
func countMissed(spec Spec, first, now time.Time) int {
	if first.After(now) {
		return 0
	}
	n := 1
	t := first
	for n < 500 {
		nxt, err := Next(spec, t)
		if err != nil || !nxt.After(t) {
			break
		}
		if nxt.After(now) {
			break
		}
		n++
		t = nxt
	}
	return n
}

func nextAt(spec Spec, from time.Time) (time.Time, error) {
	hour, min, err := parseHHMM(spec.At)
	if err != nil {
		return time.Time{}, err
	}
	rec := spec.Recurrence
	if rec == "" {
		rec = "daily"
	}
	loc := from.Location()
	day := time.Date(from.Year(), from.Month(), from.Day(), hour, min, 0, 0, loc)
	for i := 0; i < 370; i++ {
		cand := day.AddDate(0, 0, i)
		cand = time.Date(cand.Year(), cand.Month(), cand.Day(), hour, min, 0, 0, loc)
		if !cand.After(from) {
			continue
		}
		if matches(spec, rec, cand) {
			return cand, nil
		}
	}
	return time.Time{}, fmt.Errorf("no upcoming time for schedule")
}

func matches(spec Spec, rec string, t time.Time) bool {
	switch rec {
	case "daily":
		return true
	case "weekdays":
		wd := t.Weekday()
		return wd != time.Saturday && wd != time.Sunday
	case "weekly":
		want, ok := (manifest.Schedule{Day: spec.Day}).Weekday()
		return ok && t.Weekday() == want
	default:
		return false
	}
}

func parseHHMM(s string) (int, int, error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, 0, fmt.Errorf("at must be HH:MM")
	}
	hour := int(s[0]-'0')*10 + int(s[1]-'0')
	min := int(s[3]-'0')*10 + int(s[4]-'0')
	if hour > 23 || min > 59 {
		return 0, 0, fmt.Errorf("at must be HH:MM")
	}
	return hour, min, nil
}
