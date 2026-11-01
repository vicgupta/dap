//go:build darwin

package secrets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// keychain uses the macOS `security` CLI against the login keychain.
// The secret is passed with -w, which is visible briefly in the process list.
// Set DAP_SECRETS=file to keep secrets only in the 0600 file instead.
type keychain struct{}

func newPlatformKeeper() (Keeper, error) {
	if _, err := exec.LookPath("security"); err != nil {
		return nil, errors.New("security command is not available")
	}
	return keychain{}, nil
}

func (keychain) Backend() string { return "keychain" }

func (keychain) Set(service, account, secret string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "security", "add-generic-password", "-U", "-s", service, "-a", account, "-w", secret)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("keychain: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

func (keychain) Get(service, account string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "security", "find-generic-password", "-s", service, "-a", account, "-w")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", ErrNotFound
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

func (keychain) Delete(service, account string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "security", "delete-generic-password", "-s", service, "-a", account)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("keychain: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}
