// Package agentrun launches the user's coding agent in a plugin directory.
// DAP does not host a model and does not read the agent's credentials.
package agentrun

import (
	"bufio"
	"context"
	"fmt"
	"io"
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

// claudeAllowedTools is the headless allow-list. acceptEdits covers writes in
// the working directory. Bash stays limited to the build and the validator.
// Rule syntax is the Claude Code permission form: a space before * is a prefix match.
// https://code.claude.com/docs/en/headless
const claudeAllowedTools = "Read,Edit,Write,Bash(npm *),Bash(npx *),Bash(node *),Bash(da plugin validate *)"

// openCodePermissions allows edits in the working directory and the same
// build commands. external_directory stays denied so the run cannot write
// outside the draft. https://opencode.ai/docs/permissions/
const openCodePermissions = `{"permission":{"read":"allow","edit":"allow","external_directory":"deny","webfetch":"deny","websearch":"deny","bash":{"*":"deny","npm *":"allow","npx *":"allow","node *":"allow","da plugin validate":"allow","da plugin validate *":"allow"}}}`

// Agents is the runner abstraction the Builder can reuse later.
// Codex exec is read-only unless --sandbox workspace-write is set, and it
// refuses a directory that is not a git checkout unless --skip-git-repo-check
// is set. Drafts are not repositories.
// https://developers.openai.com/codex/noninteractive
var Agents = []Spec{
	{ID: "claude", Hint: "Install Claude Code and run claude once so this machine is logged in. https://docs.anthropic.com/en/docs/claude-code", Args: []string{"-p", "--permission-mode", "acceptEdits", "--allowedTools", claudeAllowedTools}},
	{ID: "codex", Hint: "Install Codex and log in with your ChatGPT account. The command is codex exec.", Args: []string{"exec", "--sandbox", "workspace-write", "--skip-git-repo-check"}},
	{ID: "opencode", Hint: "Install OpenCode and log in. The command is opencode run.", Args: []string{"run", "--auto"}},
}

// Info is a detected agent.
type Info struct {
	ID    string `json:"id"`
	Path  string `json:"path,omitempty"`
	Found bool   `json:"found"`
	Hint  string `json:"hint"`
}

// LookPath is replaced in tests. The default also searches Homebrew, nvm,
// Volta, and ~/.local/bin so a macOS app launched outside a shell can find claude and node.
var LookPath = lookAugmented

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
	cmd.Env = commandEnv(req.Agent)
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

// PromptInput is one instruction to the coding agent.
type PromptInput struct {
	Description string
	FixReport   string
	// DirName is the draft folder. A draft- prefix is a placeholder the agent must replace.
	DirName string
}

// Prompt is what DAP tells the agent. The plugin directory already contains AGENTS.md.
func Prompt(in PromptInput) string {
	var b strings.Builder
	b.WriteString("Work only in the current folder using AGENTS.md and the vendored SDK, never clone the repo, and run the build and da plugin validate before finishing. ")
	b.WriteString("Remove the DAP_SCAFFOLD_STARTER marker and replace the idle starter with the plugin the user asked for. ")
	b.WriteString("Do not bypass guardrails. Use the least permissions that work. ")
	b.WriteString("Do not handle raw tokens. Stdout of the plugin must be JSON-RPC only.\n")
	if strings.HasPrefix(in.DirName, "draft-") {
		b.WriteString("This folder name is a placeholder. Set name in plugin.yaml to a short kebab-case name (lowercase letters, numbers, and hyphens) and do not leave the draft- placeholder.\n")
	}
	b.WriteString("\nUser request:\n")
	b.WriteString(strings.TrimSpace(in.Description))
	b.WriteString("\n")
	if strings.TrimSpace(in.FixReport) != "" {
		b.WriteString("\n`da plugin validate` failed. Fix every check below and do not stop until the plugin would pass:\n")
		b.WriteString(in.FixReport)
		b.WriteString("\n")
	}
	return b.String()
}
