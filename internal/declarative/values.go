package declarative

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ageRe = regexp.MustCompile(`^(\d+)([mhd])$`)

// parseAge accepts 30m, 24h, 7d, and Go durations of at least one minute.
func parseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("duration is empty")
	}
	if m := ageRe.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil || n <= 0 || n > 3650 {
			return 0, fmt.Errorf("duration %q is out of range", s)
		}
		switch m[2] {
		case "m":
			return time.Duration(n) * time.Minute, nil
		case "h":
			return time.Duration(n) * time.Hour, nil
		case "d":
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("duration must look like 30m, 24h, or 7d")
	}
	if d < time.Minute {
		return 0, fmt.Errorf("duration must be at least 1m")
	}
	return d, nil
}

func durationAtLeast5s(s string) error {
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return fmt.Errorf("expected a duration such as 30s or 1h")
	}
	if d < 5*time.Second {
		return fmt.Errorf("must be at least 5s")
	}
	return nil
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			f, err2 := n.Float64()
			if err2 != nil {
				return 0, false
			}
			return int(f), true
		}
		return int(i), true
	default:
		return 0, false
	}
}

func asStrings(v any) ([]string, bool) {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return nil, false
		}
		return []string{t}, true
	case []string:
		return t, true
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

func asBool(v any) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}

func settingString(cfg map[string]any, key string) (string, error) {
	v, ok := cfg[key]
	if !ok {
		return "", fmt.Errorf("setting %s is not set", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("setting %s must be a string", key)
	}
	return s, nil
}
