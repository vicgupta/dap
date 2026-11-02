// Package agentrun launches the user's coding agent in a plugin directory.
// DAP does not host a model and does not read the agent's credentials.
package agentrun

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Spec is one supported agent CLI.
type Spec struct {
	ID   string
	Hint string
	Args []string // arguments before the prompt
}

// Agents is the runner abstraction the Builder can reuse later.
var Agents = []Spec{
	{ID: "claude", Hint: "Install Claude Code and run claude once so this machine is logged in. https://docs.anthropic.com/en/docs/claude-code", Args: []string{"-p"}},
	{ID: "codex", Hint: "Install Codex and log in with your ChatGPT account. The command is codex exec.", Args: []string{"exec"}},
	{ID: "opencode", Hint: "Install OpenCode and log in. The command is opencode run.", Args: []string{"run"}},
}

// Info is a detected agent.
type Info struct {
	ID    string `json:"id"`
	Path  string `json:"path,omitempty"`
	Found bool   `json:"found"`
	Hint  string `json:"hint"`
}

// LookPath is replaced in tests.
var LookPath = exec.LookPath

// Detect reports which agent CLIs are on PATH.
func Detect() []Info {
	out := make([]Info, 0, len(Agents))
	for _, a := range Agents {
		info := Info{ID: a.ID, Hint: a.Hint}
		if path, err := LookPath(a.ID); err == nil {
			info.Found = true
			info.Path = path
		}
		out = append(out, info)
	}
	return out
}

// Find returns the spec and binary path.
func Find(id string) (Spec, string, error) {
	for _, a := range Agents {
		if a.ID == id {
			path, err := LookPath(id)
			if err != nil {
				return Spec{}, "", fmt.Errorf("%s is not on PATH. %s", id, a.Hint)
			}
			return a, path, nil
		}
	}
	return Spec{}, "", fmt.Errorf("unknown agent %q", id)
}

// Request is one headless agent run.
type Request struct {
	Agent  string
	Dir    string
	Prompt string
	OnLine func(string)
}

// Run starts the agent and streams combined output. Cancel kills the process.
func Run(ctx context.Context, req Request) error {
	spec, bin, err := Find(req.Agent)
	if err != nil {
		return err
	}
	args := append(append([]string{}, spec.Args...), req.Prompt)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = req.Dir
	cmd.Env = os.Environ()
	setProcGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	stream := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			if req.OnLine != nil {
				req.OnLine(sc.Text())
			}
		}
	}
	wg.Add(2)
	go stream(stdout)
	go stream(stderr)
	waitErr := cmd.Wait()
	wg.Wait()
	if ctx.Err() != nil {
		killProc(cmd)
		return ctx.Err()
	}
	if waitErr != nil {
		return waitErr
	}
	return nil
}

// Prompt is what DAP tells the agent. The plugin directory already contains AGENTS.md.
func Prompt(description string, fixReport string) string {
	var b strings.Builder
	b.WriteString("Read AGENTS.md and implement the DAP plugin in this directory. ")
	b.WriteString("Do not bypass guardrails. Use the least permissions that work. ")
	b.WriteString("Do not handle raw tokens. Stdout of the plugin must be JSON-RPC only.\n\n")
	b.WriteString("User request:\n")
	b.WriteString(strings.TrimSpace(description))
	b.WriteString("\n")
	if strings.TrimSpace(fixReport) != "" {
		b.WriteString("\n`da plugin validate` failed. Fix every check below and do not stop until the plugin would pass:\n")
		b.WriteString(fixReport)
		b.WriteString("\n")
	}
	return b.String()
}
