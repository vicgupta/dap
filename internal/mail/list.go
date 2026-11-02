package mail

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ListOptions narrows GET /mail/threads. Zero values keep the provider default
// (about the last 30 days for Gmail and Outlook).
type ListOptions struct {
	Since     time.Time
	NewerThan string
}

var newerRe = regexp.MustCompile(`^(\d+)([mhd])$`)

// ParseListOptions accepts since as RFC3339 and newer_than as 30m, 24h, or 7d.
func ParseListOptions(since, newerThan string) (ListOptions, error) {
	var opt ListOptions
	since = strings.TrimSpace(since)
	newerThan = strings.TrimSpace(strings.ToLower(newerThan))
	if since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return ListOptions{}, fmt.Errorf("since must be an RFC3339 timestamp")
		}
		opt.Since = t
	}
	if newerThan != "" {
		if _, ok := parseNewerThan(newerThan); !ok {
			return ListOptions{}, fmt.Errorf("newer_than must look like 30m, 24h, or 7d")
		}
		opt.NewerThan = newerThan
	}
	return opt, nil
}

func parseNewerThan(s string) (time.Duration, bool) {
	m := newerRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(s)))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 || n > 3650 {
		return 0, false
	}
	switch m[2] {
	case "m":
		return time.Duration(n) * time.Minute, true
	case "h":
		return time.Duration(n) * time.Hour, true
	case "d":
		return time.Duration(n) * 24 * time.Hour, true
	default:
		return 0, false
	}
}

// Cutoff is the later of since and now-newer_than, so both limits apply.
func (o ListOptions) Cutoff(now time.Time) (time.Time, bool) {
	var cut time.Time
	set := false
	if !o.Since.IsZero() {
		cut = o.Since
		set = true
	}
	if d, ok := parseNewerThan(o.NewerThan); ok {
		t := now.Add(-d)
		if !set || t.After(cut) {
			cut = t
			set = true
		}
	}
	return cut, set
}

// gmailSearch is the Gmail q parameter. Days use newer_than. Hours that are
// whole days do too (24h -> 1d). Anything else uses after:<unix>, which Gmail accepts.
func gmailSearch(opt ListOptions, now time.Time) string {
	if opt.Since.IsZero() && opt.NewerThan != "" {
		m := newerRe.FindStringSubmatch(strings.ToLower(opt.NewerThan))
		if m != nil && m[2] == "d" {
			return "newer_than:" + m[1] + "d"
		}
		if m != nil && m[2] == "h" {
			n, _ := strconv.Atoi(m[1])
			if n > 0 && n%24 == 0 {
				return fmt.Sprintf("newer_than:%dd", n/24)
			}
		}
	}
	if cut, ok := opt.Cutoff(now); ok {
		return "after:" + strconv.FormatInt(cut.Unix(), 10)
	}
	return "newer_than:30d"
}

// outlookFilter is a Graph receivedDateTime filter for Composio and direct Graph.
func outlookFilter(opt ListOptions, now time.Time) string {
	cut, ok := opt.Cutoff(now)
	if !ok {
		return ""
	}
	return "receivedDateTime ge " + cut.UTC().Format(time.RFC3339)
}

func filterThreads(threads []Thread, opt ListOptions, now time.Time) []Thread {
	cut, ok := opt.Cutoff(now)
	if !ok {
		return threads
	}
	out := make([]Thread, 0, len(threads))
	for _, th := range threads {
		if th.LastAt.IsZero() || th.LastAt.Before(cut) {
			continue
		}
		out = append(out, th)
	}
	return out
}
