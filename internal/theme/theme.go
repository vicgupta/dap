// Package theme names the three appearance modes the local UI can store.
package theme

import (
	"fmt"
	"strings"
)

const (
	// System follows the operating system prefers-color-scheme.
	System = "system"
	// Light is the light palette regardless of the OS.
	Light = "light"
	// Dark is the dark palette regardless of the OS.
	Dark = "dark"

	// StorageKey is the localStorage key the browser UI uses.
	StorageKey = "dap.theme"
)

// Normalize accepts system, light, or dark, ignoring case and surrounding space.
// An empty string is system. Anything else is an error.
func Normalize(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", System:
		return System, nil
	case Light, Dark:
		return s, nil
	default:
		return "", fmt.Errorf("theme must be system, light, or dark")
	}
}

// Stored maps a value read from disk to a mode. Invalid text becomes system.
func Stored(s string) string {
	mode, err := Normalize(s)
	if err != nil {
		return System
	}
	return mode
}

// Resolve picks the concrete palette. system uses prefersDark. An invalid mode
// is treated as system.
func Resolve(mode string, prefersDark bool) string {
	switch Stored(mode) {
	case Dark:
		return Dark
	case Light:
		return Light
	default:
		if prefersDark {
			return Dark
		}
		return Light
	}
}
