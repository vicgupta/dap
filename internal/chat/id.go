package chat

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// NormalizeProvider accepts claude-code, claude, or codex.
// Anything else, including opencode and an empty string, is an error.
func NormalizeProvider(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case ProviderClaude, "claude":
		return ProviderClaude, nil
	case ProviderCodex:
		return ProviderCodex, nil
	default:
		return "", fmt.Errorf("provider must be claude-code or codex")
	}
}

// StoredProvider maps a value read from disk. Invalid text becomes empty
// so a hand-edited config cannot select a provider the picker does not offer.
func StoredProvider(s string) string {
	id, err := NormalizeProvider(s)
	if err != nil {
		return ""
	}
	return id
}

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("chat id: %w", err))
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

// MintSessionID is a version-4 UUID. Claude Code's --session-id requires that shape.
func MintSessionID() string { return newUUID() }

// newUUID is a version-4 UUID. Claude Code's --session-id requires that shape.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("chat uuid: %w", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
