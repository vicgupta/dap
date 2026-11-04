//go:build linux

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

// secretTool used to talk to Secret Service. DAP no longer writes there.
type secretTool struct {
	bin string
}

func newPlatformKeeper() (Keeper, error) {
	return nil, errors.New("secrets stay in the data directory")
}

func (s *secretTool) Backend() string { return "secret-service" }

func (s *secretTool) Set(service, account, secret string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.bin, "store", "--label", "DAP "+account, "service", service, "account", account)
	cmd.Stdin = strings.NewReader(secret)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("secret-tool store: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

func (s *secretTool) Get(service, account string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.bin, "lookup", "service", service, "account", account)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrNotFound
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

func (s *secretTool) Delete(service, account string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.bin, "clear", "service", service, "account", account)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("secret-tool clear: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}
