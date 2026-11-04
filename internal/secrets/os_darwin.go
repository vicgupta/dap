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

// keychain used to call the macOS `security` CLI. DAP no longer writes there.
type keychain struct{}

func newPlatformKeeper() (Keeper, error) {
	return nil, errors.New("secrets stay in the data directory")
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
