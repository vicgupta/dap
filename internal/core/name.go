package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// pluginNameOK matches the scaffold rule: lowercase, digits, hyphens, starting with a letter.
var pluginNameOK = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// called or named, with quotes or as a single token. The first match wins.
var nameCall = regexp.MustCompile("(?i)(?:\\bcalled\\b|\\bnamed\\b)\\s+(?:\"([^\"\\n]+)\"|'([^'\\n]+)'|`([^`\\n]+)`|([A-Za-z][A-Za-z0-9-]{0,63}))")

// choosePluginName uses the form's Name, then `called "x"` / `named x` in the
// description. Anything else is a draft- placeholder so a sentence never becomes the folder name.
// provisional is true when the agent still has to pick a short name.
func choosePluginName(explicit, description string) (name string, provisional bool, err error) {
	if strings.TrimSpace(explicit) != "" {
		n, nerr := normalizePluginName(explicit)
		if nerr != nil {
			return "", false, fmt.Errorf("name: %w", nerr)
		}
		return n, false, nil
	}
	if m := nameCall.FindStringSubmatch(description); len(m) > 0 {
		raw := ""
		for _, g := range m[1:] {
			if g != "" {
				raw = g
				break
			}
		}
		if n, nerr := normalizePluginName(raw); nerr == nil {
			return n, false, nil
		}
	}
	return provisionalName(), true, nil
}

func normalizePluginName(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-")
	}
	if !pluginNameOK.MatchString(out) {
		return "", fmt.Errorf("use a short name of lowercase letters, numbers, and hyphens")
	}
	return out, nil
}

func provisionalName() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "draft-plugin"
	}
	return "draft-" + hex.EncodeToString(b[:])
}
