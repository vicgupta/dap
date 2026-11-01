// Package guardrails is the "da check" policy.
// It answers allowed, ask first, or never before any action runs.
//
// Baked-in rule: nothing that sends, spends, or deletes runs without an approved card.
// A user override can tighten a permission (allowed → ask first → never) and cannot loosen it.
package guardrails

import (
	"strings"

	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/permissions"
)

// Rule is a person-authored tightening of a plugin permission.
// Plugin "*" applies to every plugin. Permission "*" applies to every permission of that plugin.
type Rule struct {
	Plugin     string `json:"plugin"`
	Permission string `json:"permission"`
	Decision   string `json:"decision"`
}

// Request is one action a plugin or an external agent wants to take.
type Request struct {
	Plugin       string
	Permission   string
	Effects      []string
	Declared     []manifest.Permission
	Overrides    []Rule
	CardApproved bool
}

// Result is the decision and the sentence that explains it.
type Result struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// Sensitive reports whether any effect is sends, spends, or deletes.
func Sensitive(effects []string) bool {
	for _, e := range effects {
		if model.SensitiveEffect(e) {
			return true
		}
	}
	return false
}

// NormalizeEffects lowercases known effects and rejects anything else.
func NormalizeEffects(effects []string) ([]string, error) {
	if len(effects) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(effects))
	seen := map[string]bool{}
	for _, e := range effects {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !model.SensitiveEffect(e) {
			return nil, errUnknownEffect(e)
		}
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out, nil
}

type unknownEffect string

func errUnknownEffect(e string) error { return unknownEffect(e) }

func (e unknownEffect) Error() string {
	return "unknown effect " + string(e) + " (use sends, spends, or deletes)"
}

// Evaluate applies declaration, user overrides, and the baked-in send/spend/delete rule.
func Evaluate(req Request) Result {
	look := permissions.Lookup(req.Declared, req.Permission)
	base := look.Mode
	if !look.Declared {
		return Result{
			Decision: model.ModeNever,
			Reason:   look.Plain,
		}
	}
	if ov, ok := matchOverride(req.Overrides, req.Plugin, req.Permission); ok {
		tightened := tighten(base, ov.Decision)
		if tightened != base {
			base = tightened
		}
	}
	if Sensitive(req.Effects) {
		if base == model.ModeNever {
			return Result{Decision: model.ModeNever, Reason: "Blocked by a guardrail. This will not run."}
		}
		if !req.CardApproved {
			return Result{
				Decision: model.ModeAskFirst,
				Reason:   "Sending, spending, or deleting waits for an approved card.",
			}
		}
		return Result{
			Decision: model.ModeAllowed,
			Reason:   "An approved card covers this send, spend, or delete.",
		}
	}
	switch base {
	case model.ModeNever:
		return Result{Decision: model.ModeNever, Reason: "Blocked by a guardrail. This will not run."}
	case model.ModeAskFirst:
		if req.CardApproved {
			return Result{Decision: model.ModeAllowed, Reason: "You approved this card."}
		}
		why := look.Plain
		if strings.TrimSpace(req.Permission) == "" {
			why = "This action asks first."
		}
		return Result{Decision: model.ModeAskFirst, Reason: why}
	default:
		return Result{Decision: model.ModeAllowed, Reason: look.Plain}
	}
}

func matchOverride(rules []Rule, plugin, permission string) (Rule, bool) {
	var found Rule
	best := -1
	for _, r := range rules {
		if r.Plugin != plugin && r.Plugin != "*" {
			continue
		}
		if r.Permission != permission && r.Permission != "*" {
			continue
		}
		score := 0
		if r.Plugin == plugin {
			score += 2
		}
		if r.Permission == permission {
			score += 1
		}
		if score > best {
			best = score
			found = r
		}
	}
	if best < 0 {
		return Rule{}, false
	}
	found.Decision = manifest.NormalizeMode(found.Decision)
	return found, true
}

// tighten keeps the stricter of the manifest mode and the override.
// An override of "allowed" cannot weaken ask_first or never.
func tighten(base, override string) string {
	rank := map[string]int{
		model.ModeAllowed:  0,
		model.ModeAskFirst: 1,
		model.ModeNever:    2,
	}
	br, okb := rank[base]
	or, oko := rank[override]
	if !okb {
		br = 1
	}
	if !oko {
		return base
	}
	if or > br {
		return override
	}
	return base
}

// ValidDecision reports whether a user-supplied override value is one of the three.
func ValidDecision(d string) bool {
	switch manifest.NormalizeMode(d) {
	case model.ModeAllowed, model.ModeAskFirst, model.ModeNever:
		return true
	default:
		return false
	}
}
