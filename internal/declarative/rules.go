package declarative

import (
	"fmt"
	"strings"
	"time"

	"github.com/vicgupta/dap/internal/mailbox"
)

// match reports whether one thread satisfies the rule.
func (r Rule) match(p Plugin, cfg map[string]any, th mailbox.Thread, now time.Time) (bool, error) {
	clauses := r.When.All
	anyMode := false
	if len(clauses) == 0 {
		clauses = r.When.Any
		anyMode = true
	}
	if anyMode {
		for _, c := range clauses {
			ok, err := c.match(cfg, th, now)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	for _, c := range clauses {
		ok, err := c.match(cfg, th, now)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func (c Clause) match(cfg map[string]any, th mailbox.Thread, now time.Time) (bool, error) {
	switch c.Key {
	case "sender":
		vals, err := c.text(cfg)
		if err != nil {
			return false, err
		}
		email := strings.ToLower(strings.TrimSpace(th.WithEmail))
		for _, v := range vals {
			if email == strings.ToLower(strings.TrimSpace(v)) {
				return true, nil
			}
		}
		return false, nil
	case "sender_domain":
		vals, err := c.text(cfg)
		if err != nil {
			return false, err
		}
		domain := domainOf(th.WithEmail)
		for _, v := range vals {
			if domain == normalizeDomain(v) && domain != "" {
				return true, nil
			}
		}
		return false, nil
	case "subject_contains":
		return containsAny(th.Subject, c, cfg)
	case "snippet_contains":
		return containsAny(th.Snippet, c, cfg)
	case "label":
		vals, err := c.text(cfg)
		if err != nil {
			return false, err
		}
		for _, have := range th.Labels {
			for _, want := range vals {
				if strings.EqualFold(strings.TrimSpace(have), strings.TrimSpace(want)) {
					return true, nil
				}
			}
		}
		return false, nil
	case "last_from_me":
		want, err := c.boolean(cfg)
		if err != nil {
			return false, err
		}
		return th.LastFromMe == want, nil
	case "older_than", "newer_than", "no_reply_for":
		d, err := c.age(cfg)
		if err != nil {
			return false, err
		}
		if th.LastAt.IsZero() {
			return false, nil
		}
		age := now.Sub(th.LastAt)
		switch c.Key {
		case "older_than":
			return age >= d, nil
		case "newer_than":
			return age >= 0 && age <= d, nil
		default:
			return th.LastFromMe && age >= d, nil
		}
	default:
		return false, fmt.Errorf("unknown comparison %s", c.Key)
	}
}

func containsAny(hay string, c Clause, cfg map[string]any) (bool, error) {
	vals, err := c.text(cfg)
	if err != nil {
		return false, err
	}
	hay = strings.ToLower(hay)
	for _, v := range vals {
		needle := strings.ToLower(strings.TrimSpace(v))
		if needle != "" && strings.Contains(hay, needle) {
			return true, nil
		}
	}
	return false, nil
}

func (c Clause) text(cfg map[string]any) ([]string, error) {
	if c.Setting == "" {
		return c.Strings, nil
	}
	v, ok := cfg[c.Setting]
	if !ok {
		return nil, fmt.Errorf("setting %s is not set", c.Setting)
	}
	vals, ok := asStrings(v)
	if !ok || len(vals) == 0 {
		return nil, fmt.Errorf("setting %s must be a string or a list of strings", c.Setting)
	}
	return vals, nil
}

func (c Clause) boolean(cfg map[string]any) (bool, error) {
	if c.Setting == "" {
		if c.Bool == nil {
			return false, fmt.Errorf("%s needs true or false", c.Key)
		}
		return *c.Bool, nil
	}
	v, ok := cfg[c.Setting]
	if !ok {
		return false, fmt.Errorf("setting %s is not set", c.Setting)
	}
	b, ok := asBool(v)
	if !ok {
		return false, fmt.Errorf("setting %s must be a bool", c.Setting)
	}
	return b, nil
}

func (c Clause) age(cfg map[string]any) (time.Duration, error) {
	if c.Setting == "" {
		if len(c.Strings) != 1 {
			return 0, fmt.Errorf("%s needs one duration", c.Key)
		}
		return parseAge(c.Strings[0])
	}
	v, ok := cfg[c.Setting]
	if !ok {
		return 0, fmt.Errorf("setting %s is not set", c.Setting)
	}
	if n, ok := asInt(v); ok {
		if n < 1 {
			return 0, fmt.Errorf("setting %s must be at least 1 hour", c.Setting)
		}
		return time.Duration(n) * time.Hour, nil
	}
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("setting %s must be a number of hours or a duration", c.Setting)
	}
	return parseAge(s)
}

func domainOf(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	i := strings.LastIndex(email, "@")
	if i < 0 || i == len(email)-1 {
		return ""
	}
	return email[i+1:]
}

func normalizeDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "@")
	return s
}
