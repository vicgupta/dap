package declarative

import (
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/mailbox"
)

func TestRuleVocabulary(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	thread := mailbox.Thread{
		ID: "t1", Subject: "Invoice for Q4", WithEmail: "dana.lee@northwind.co",
		LastAt: now.Add(-48 * time.Hour), LastFromMe: true, Snippet: "please pay",
		Labels: []string{"flagged"},
	}
	p := Plugin{}
	cfg := map[string]any{"words": []any{"invoice"}, "hours": 24}
	cases := []struct {
		name string
		rule Rule
		want bool
	}{
		{"sender", Rule{When: When{All: []Clause{{Key: "sender", Strings: []string{"dana.lee@northwind.co"}}}}}, true},
		{"domain", Rule{When: When{All: []Clause{{Key: "sender_domain", Strings: []string{"northwind.co"}}}}}, true},
		{"subject setting", Rule{When: When{Any: []Clause{{Key: "subject_contains", Setting: "words"}}}}, true},
		{"snippet miss", Rule{When: When{All: []Clause{{Key: "snippet_contains", Strings: []string{"refund"}}}}}, false},
		{"label", Rule{When: When{All: []Clause{{Key: "label", Strings: []string{"flagged"}}}}}, true},
		{"older", Rule{When: When{All: []Clause{{Key: "older_than", Strings: []string{"24h"}}}}}, true},
		{"newer miss", Rule{When: When{All: []Clause{{Key: "newer_than", Strings: []string{"1h"}}}}}, false},
		{"no reply", Rule{When: When{All: []Clause{{Key: "no_reply_for", Setting: "hours"}}}}, true},
		{"from me", Rule{When: When{All: []Clause{{Key: "last_from_me", Bool: boolPtr(true)}}}}, true},
		{"any misses", Rule{When: When{Any: []Clause{
			{Key: "sender", Strings: []string{"nope@example.com"}},
			{Key: "subject_contains", Strings: []string{"refund"}},
		}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := tc.rule.match(p, cfg, thread, now)
			if err != nil {
				t.Fatal(err)
			}
			if ok != tc.want {
				t.Fatalf("got %v", ok)
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }
