package mail

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseListOptions(t *testing.T) {
	opt, err := ParseListOptions("2026-11-01T00:00:00Z", "24h")
	if err != nil || opt.Since.IsZero() || opt.NewerThan != "24h" {
		t.Fatalf("%+v %v", opt, err)
	}
	if _, err := ParseListOptions("yesterday", ""); err == nil {
		t.Fatal("expected since error")
	}
	if _, err := ParseListOptions("", "2 weeks"); err == nil {
		t.Fatal("expected newer_than error")
	}
	empty, err := ParseListOptions("", "")
	if err != nil || !empty.Since.IsZero() || empty.NewerThan != "" {
		t.Fatal(err)
	}
}

func TestGmailAndOutlookWindows(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	if got := gmailSearch(ListOptions{}, now); got != "newer_than:30d" {
		t.Fatal(got)
	}
	if got := gmailSearch(ListOptions{NewerThan: "1d"}, now); got != "newer_than:1d" {
		t.Fatal(got)
	}
	if got := gmailSearch(ListOptions{NewerThan: "24h"}, now); got != "newer_than:1d" {
		t.Fatal(got)
	}
	hour := gmailSearch(ListOptions{NewerThan: "2h"}, now)
	if !strings.HasPrefix(hour, "after:") {
		t.Fatal(hour)
	}
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	q := gmailSearch(ListOptions{Since: since}, now)
	if q != "after:"+strconv.FormatInt(since.Unix(), 10) {
		t.Fatal(q)
	}
	filter := outlookFilter(ListOptions{Since: since}, now)
	if filter != "receivedDateTime ge 2026-11-01T00:00:00Z" {
		t.Fatal(filter)
	}
	if outlookFilter(ListOptions{}, now) != "" {
		t.Fatal("empty filter")
	}
	threads := filterThreads([]Thread{
		{ID: "old", LastAt: now.Add(-48 * time.Hour)},
		{ID: "new", LastAt: now.Add(-2 * time.Hour)},
		{ID: "undated"},
	}, ListOptions{NewerThan: "1d"}, now)
	if len(threads) != 1 || threads[0].ID != "new" {
		t.Fatalf("%+v", threads)
	}
}
