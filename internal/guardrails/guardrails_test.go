package guardrails

import (
	"testing"

	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
)

func declared() []manifest.Permission {
	return []manifest.Permission{
		{Name: "workflow.run", Mode: model.ModeAllowed, Description: "Run a safe step"},
		{Name: "share.team", Mode: model.ModeAskFirst, Description: "Share with the team"},
		{Name: "mail.send", Mode: model.ModeAllowed, Description: "Send mail"},
	}
}

func TestUndeclaredIsNever(t *testing.T) {
	res := Evaluate(Request{Plugin: "hello", Permission: "files.write", Declared: declared()})
	if res.Decision != model.ModeNever {
		t.Fatalf("%+v", res)
	}
}

func TestAskFirstWithoutCard(t *testing.T) {
	res := Evaluate(Request{Plugin: "workflow", Permission: "share.team", Declared: declared()})
	if res.Decision != model.ModeAskFirst {
		t.Fatalf("%+v", res)
	}
	res = Evaluate(Request{Plugin: "workflow", Permission: "share.team", Declared: declared(), CardApproved: true})
	if res.Decision != model.ModeAllowed {
		t.Fatalf("approved card should pass: %+v", res)
	}
}

func TestSendRequiresCardEvenWhenAllowed(t *testing.T) {
	res := Evaluate(Request{
		Plugin:     "mail",
		Permission: "mail.send",
		Effects:    []string{"sends"},
		Declared:   declared(),
	})
	if res.Decision != model.ModeAskFirst {
		t.Fatalf("send without a card must ask first: %+v", res)
	}
	res = Evaluate(Request{
		Plugin:       "mail",
		Permission:   "mail.send",
		Effects:      []string{"sends"},
		Declared:     declared(),
		CardApproved: true,
	})
	if res.Decision != model.ModeAllowed {
		t.Fatalf("approved send should pass: %+v", res)
	}
}

func TestOverrideCanTightenNotLoosen(t *testing.T) {
	res := Evaluate(Request{
		Plugin:     "workflow",
		Permission: "workflow.run",
		Declared:   declared(),
		Overrides:  []Rule{{Plugin: "workflow", Permission: "workflow.run", Decision: "never"}},
	})
	if res.Decision != model.ModeNever {
		t.Fatalf("override should block: %+v", res)
	}
	// Asking to loosen share.team from ask_first to allowed does nothing.
	res = Evaluate(Request{
		Plugin:     "workflow",
		Permission: "share.team",
		Declared:   declared(),
		Overrides:  []Rule{{Plugin: "*", Permission: "share.team", Decision: model.ModeAllowed}},
	})
	if res.Decision != model.ModeAskFirst {
		t.Fatalf("override must not loosen: %+v", res)
	}
}

func TestSpendAndDelete(t *testing.T) {
	for _, effect := range []string{"spends", "deletes"} {
		res := Evaluate(Request{
			Plugin:     "workflow",
			Permission: "workflow.run",
			Effects:    []string{effect},
			Declared:   declared(),
		})
		if res.Decision != model.ModeAskFirst {
			t.Fatalf("%s: %+v", effect, res)
		}
	}
}

func TestBadEffect(t *testing.T) {
	if _, err := NormalizeEffects([]string{"sends", "teleports"}); err == nil {
		t.Fatal("expected error")
	}
	got, err := NormalizeEffects([]string{"Sends", "sends", " spends "})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "sends" || got[1] != "spends" {
		t.Fatalf("%v", got)
	}
}

func TestAllowedSafe(t *testing.T) {
	res := Evaluate(Request{Plugin: "workflow", Permission: "workflow.run", Declared: declared()})
	if res.Decision != model.ModeAllowed {
		t.Fatalf("%+v", res)
	}
}
