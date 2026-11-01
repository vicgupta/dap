// Package id makes short random identifiers for cards, ledger rows, and observations.
package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New returns prefix + "_" + 16 hex chars. The prefix keeps ids recognizable in the ledger.
func New(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("id: %w", err))
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
