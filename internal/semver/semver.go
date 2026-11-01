// Package semver parses a practical subset of semantic versions and ranges.
// Versions are MAJOR.MINOR.PATCH. Pre-release tags are rejected so manifests stay boring.
// Ranges accept exact, =, >, >=, <, <=, ^, ~, *, and space- or comma-separated AND groups.
// A partial version such as 0.3 is read as 0.3.0.
package semver

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Version is a three-part semantic version.
type Version struct {
	Major int
	Minor int
	Patch int
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns -1, 0, or 1.
func (v Version) Compare(o Version) int {
	if v.Major != o.Major {
		return cmpInt(v.Major, o.Major)
	}
	if v.Minor != o.Minor {
		return cmpInt(v.Minor, o.Minor)
	}
	return cmpInt(v.Patch, o.Patch)
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// Parse reads MAJOR.MINOR.PATCH, with a leading v allowed. One or two parts are padded.
func Parse(s string) (Version, error) {
	raw := strings.TrimSpace(s)
	raw = strings.TrimPrefix(raw, "v")
	raw = strings.TrimPrefix(raw, "V")
	if raw == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 3 {
		return Version{}, fmt.Errorf("invalid version %q", s)
	}
	nums := []int{0, 0, 0}
	for i, p := range parts {
		if p == "" || !allDigits(p) {
			return Version{}, fmt.Errorf("invalid version %q", s)
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version %q", s)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// Satisfies reports whether version meets the constraint.
// An empty constraint or "*" matches anything.
func Satisfies(version, constraint string) (bool, error) {
	v, err := Parse(version)
	if err != nil {
		return false, err
	}
	return v.Satisfies(constraint)
}

// Satisfies reports whether v meets the constraint.
func (v Version) Satisfies(constraint string) (bool, error) {
	c := strings.TrimSpace(constraint)
	if c == "" || c == "*" {
		return true, nil
	}
	parts := splitAND(c)
	if len(parts) == 0 {
		return false, fmt.Errorf("invalid range %q", constraint)
	}
	for _, p := range parts {
		ok, err := satisfiesOne(v, p)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// ValidRange reports whether constraint is syntactically acceptable.
func ValidRange(constraint string) error {
	c := strings.TrimSpace(constraint)
	if c == "" || c == "*" {
		return nil
	}
	parts := splitAND(c)
	if len(parts) == 0 {
		return fmt.Errorf("invalid range %q", constraint)
	}
	zero := Version{}
	for _, p := range parts {
		if _, err := satisfiesOne(zero, p); err != nil {
			return err
		}
	}
	return nil
}

func splitAND(c string) []string {
	c = strings.ReplaceAll(c, ",", " ")
	fields := strings.Fields(c)
	return fields
}

func satisfiesOne(v Version, token string) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" || token == "*" {
		return true, nil
	}
	op, rest := splitOp(token)
	switch op {
	case "^":
		base, err := Parse(rest)
		if err != nil {
			return false, fmt.Errorf("invalid range %q", token)
		}
		if v.Compare(base) < 0 {
			return false, nil
		}
		upper := caretUpper(base)
		return v.Compare(upper) < 0, nil
	case "~":
		base, err := Parse(rest)
		if err != nil {
			return false, fmt.Errorf("invalid range %q", token)
		}
		if v.Compare(base) < 0 {
			return false, nil
		}
		upper := Version{Major: base.Major, Minor: base.Minor + 1, Patch: 0}
		return v.Compare(upper) < 0, nil
	case ">=", ">", "<=", "<", "=":
		base, err := Parse(rest)
		if err != nil {
			return false, fmt.Errorf("invalid range %q", token)
		}
		cmp := v.Compare(base)
		switch op {
		case ">=":
			return cmp >= 0, nil
		case ">":
			return cmp > 0, nil
		case "<=":
			return cmp <= 0, nil
		case "<":
			return cmp < 0, nil
		default:
			return cmp == 0, nil
		}
	case "":
		base, err := Parse(rest)
		if err != nil {
			return false, fmt.Errorf("invalid range %q", token)
		}
		return v.Compare(base) == 0, nil
	default:
		return false, fmt.Errorf("invalid range %q", token)
	}
}

func splitOp(token string) (string, string) {
	for _, op := range []string{">=", "<=", ">", "<", "=", "^", "~"} {
		if strings.HasPrefix(token, op) {
			return op, strings.TrimSpace(token[len(op):])
		}
	}
	return "", token
}

// caretUpper is the exclusive upper bound for a ^ range, following npm's rule
// that a leading zero narrows the compatible set.
func caretUpper(v Version) Version {
	if v.Major > 0 {
		return Version{Major: v.Major + 1}
	}
	if v.Minor > 0 {
		return Version{Major: 0, Minor: v.Minor + 1}
	}
	return Version{Major: 0, Minor: 0, Patch: v.Patch + 1}
}
