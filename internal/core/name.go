package core

import (
	"fmt"
	"regexp"
	"strings"
)

// pluginNameOK matches the scaffold rule: lowercase, digits, hyphens, starting with a letter.
var pluginNameOK = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// normalizePluginName turns a typed name into the kebab-case form plugin.yaml uses.
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
