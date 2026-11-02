package chat

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// Status reports install and sign-in for the two picker providers.
// It runs the CLIs' own status commands and does not read credential files.
type Status struct {
	LookPath func(string) (string, error)
	Env      func() []string
	Now      func() time.Time

	mu       time.Time
	cached   []Probe
	cachedAt time.Time
}

// Probes returns Claude Code and Codex, in that order. Results are cached
// for a few seconds so opening the panel does not spawn the CLIs on every paint.
func (s *Status) Probes(ctx context.Context) []Probe {
	if s == nil {
		s = &Status{}
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	if s.cached != nil && now.Sub(s.cachedAt) < 20*time.Second {
		return append([]Probe(nil), s.cached...)
	}
	out := make([]Probe, 0, 2)
	for _, p := range Providers() {
		out = append(out, s.probe(ctx, p))
	}
	s.cached = out
	s.cachedAt = now
	return append([]Probe(nil), out...)
}

func (s *Status) probe(ctx context.Context, p Provider) Probe {
	info := Probe{ID: p.ID(), Label: p.Label(), Login: loginCommand(p.ID())}
	look := s.LookPath
	if look == nil {
		look = exec.LookPath
	}
	path, err := look(p.Binary())
	if err != nil {
		info.Hint = missingHint(p.ID())
		return info
	}
	info.Installed = true
	info.Path = path
	info.Version = s.version(ctx, path)
	signed, unknown := s.signedIn(ctx, p, path)
	info.SignedIn = signed
	info.AuthUnknown = unknown
	if !signed && !unknown {
		info.Hint = signedOutHint(p.ID())
	} else if unknown {
		info.Hint = "DAP could not confirm sign-in. If a turn fails, run " + info.Login + " in a terminal. DAP does not handle that credential."
	}
	return info
}

func (s *Status) version(ctx context.Context, path string) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = s.childEnv()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if len(line) > 80 {
		line = line[:80]
	}
	return line
}

func (s *Status) signedIn(ctx context.Context, p Provider, path string) (signed, unknown bool) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var args []string
	switch p.ID() {
	case ProviderClaude:
		// `claude auth status` exits 0 when signed in and 1 when not.
		// JSON includes authMethod. DAP keeps only the yes/no.
		args = []string{"auth", "status"}
	case ProviderCodex:
		// `codex login status` exits 0 when a credential is present.
		args = []string{"login", "status"}
	default:
		return false, true
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = s.childEnv()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	text := buf.String()
	lower := strings.ToLower(text)
	if strings.Contains(lower, `"authmethod":"none"`) || strings.Contains(lower, `"authmethod": "none"`) ||
		strings.Contains(lower, "authenticated: no") || strings.Contains(lower, "not logged") ||
		strings.Contains(lower, "not signed") {
		return false, false
	}
	if err == nil {
		return true, false
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return false, false
	}
	return false, true
}

func (s *Status) childEnv() []string {
	if s != nil && s.Env != nil {
		return s.Env()
	}
	return safeEnv()
}

func loginCommand(id string) string {
	switch id {
	case ProviderClaude:
		return "claude auth login"
	case ProviderCodex:
		return "codex login"
	default:
		return ""
	}
}

func missingHint(id string) string {
	switch id {
	case ProviderClaude:
		return "Claude Code is not on PATH. Install it, then run claude auth login in a terminal. DAP never sees that login."
	case ProviderCodex:
		return "The Codex CLI is not on PATH. Install Codex, then run codex login in a terminal. DAP never sees that login."
	default:
		return "This coding agent is not installed."
	}
}

func signedOutHint(id string) string {
	switch id {
	case ProviderClaude:
		return "Claude Code is installed but not signed in. Run claude auth login in a terminal. DAP does not handle the credential."
	case ProviderCodex:
		return "Codex is installed but not signed in. Run codex login in a terminal. DAP does not handle the credential."
	default:
		return "Sign in with the CLI. DAP does not handle the credential."
	}
}
