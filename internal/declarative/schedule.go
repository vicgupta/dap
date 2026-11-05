package declarative

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// EventPoll is the schedule the core owns when a plugin declares an event
// and no schedule. It is removed with the plugin.
const EventPoll = "15m"

// Schedule is when the core wakes the plugin. A string shorthand is accepted.
type Schedule struct {
	Every      string `json:"every,omitempty" yaml:"every,omitempty"`
	Cron       string `json:"cron,omitempty" yaml:"cron,omitempty"`
	At         string `json:"at,omitempty" yaml:"at,omitempty"`
	Recurrence string `json:"recurrence,omitempty" yaml:"recurrence,omitempty"`
	Day        string `json:"day,omitempty" yaml:"day,omitempty"`
}

// UnmarshalYAML accepts "weekdays at 08:00" or the structured form.
func (s *Schedule) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		parsed, err := ParseScheduleString(value.Value)
		if err != nil {
			return err
		}
		*s = parsed
		return nil
	}
	type plain Schedule
	var raw plain
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*s = Schedule(raw)
	return nil
}

// Empty reports whether no slot was set.
func (s Schedule) Empty() bool {
	return s.Every == "" && s.Cron == "" && s.At == ""
}

var (
	atClock    = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	cronParse  = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	weekdaySet = map[string]bool{
		"sun": true, "sunday": true,
		"mon": true, "monday": true,
		"tue": true, "tues": true, "tuesday": true,
		"wed": true, "wednesday": true,
		"thu": true, "thur": true, "thurs": true, "thursday": true,
		"fri": true, "friday": true,
		"sat": true, "saturday": true,
	}
)

// ParseScheduleString reads "every 1h", "cron 0 8 * * 1", "at 08:00",
// "weekdays at 08:00", and "weekly at 08:00".
func ParseScheduleString(s string) (Schedule, error) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "every "):
		return Schedule{Every: strings.TrimSpace(strings.TrimPrefix(s, "every "))}, nil
	case strings.HasPrefix(s, "cron "):
		return Schedule{Cron: strings.TrimSpace(strings.TrimPrefix(s, "cron "))}, nil
	case strings.HasPrefix(s, "weekdays at "):
		return Schedule{At: strings.TrimSpace(strings.TrimPrefix(s, "weekdays at ")), Recurrence: "weekdays"}, nil
	case strings.HasPrefix(s, "weekly at "):
		return Schedule{At: strings.TrimSpace(strings.TrimPrefix(s, "weekly at ")), Recurrence: "weekly"}, nil
	case strings.HasPrefix(s, "at "):
		return Schedule{At: strings.TrimSpace(strings.TrimPrefix(s, "at ")), Recurrence: "daily"}, nil
	default:
		return Schedule{}, fmt.Errorf("unrecognized schedule %q", s)
	}
}

// Validate checks the same schedule rules as a code plugin.
func (s *Schedule) Validate() error {
	n := 0
	if s.Every != "" {
		n++
	}
	if s.Cron != "" {
		n++
	}
	if s.At != "" {
		n++
	}
	if n == 0 {
		return fmt.Errorf("schedule is empty")
	}
	if n > 1 {
		return fmt.Errorf("schedule must set only one of every, cron, or at")
	}
	if s.Every != "" {
		d, err := time.ParseDuration(s.Every)
		if err != nil {
			return fmt.Errorf("schedule.every: %w", err)
		}
		if d < 5*time.Second {
			return fmt.Errorf("schedule.every must be at least 5s")
		}
	}
	if s.Cron != "" {
		if _, err := cronParse.Parse(s.Cron); err != nil {
			return fmt.Errorf("schedule.cron: %w", err)
		}
	}
	if s.At != "" {
		if !atClock.MatchString(s.At) {
			return fmt.Errorf("schedule.at must be HH:MM")
		}
		if s.Recurrence == "" {
			s.Recurrence = "daily"
		}
		switch s.Recurrence {
		case "daily", "weekdays":
		case "weekly":
			if !weekdaySet[strings.ToLower(s.Day)] {
				return fmt.Errorf("schedule.day is required for a weekly recurrence (mon, tue, ...)")
			}
		default:
			return fmt.Errorf("schedule.recurrence must be daily, weekdays, or weekly")
		}
	} else if s.Recurrence != "" || s.Day != "" {
		return fmt.Errorf("schedule.recurrence and day belong with schedule.at")
	}
	return nil
}
