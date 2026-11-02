package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectAndRunFakeAgent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "claude")
	body := "#!/bin/sh\nprintf '%s\n' \"$@\" > \"$PWD/args.txt\"\necho agent-line\nprintf 'wrote' > \"$PWD/touched.txt\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := LookPath
	LookPath = func(name string) (string, error) {
		if name == "claude" {
			return script, nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { LookPath = orig })

	found := false
	for _, info := range Detect() {
		if info.ID == "claude" && info.Found {
			found = true
		}
	}
	if !found {
		t.Fatal("claude not detected")
	}
	work := t.TempDir()
	var lines []string
	err := Run(context.Background(), Request{
		Agent:  "claude",
		Dir:    work,
		Prompt: "build a plugin",
		OnLine: func(line string) { lines = append(lines, line) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(lines, "\n") != "agent-line" {
		t.Fatalf("%q", lines)
	}
	b, err := os.ReadFile(filepath.Join(work, "touched.txt"))
	if err != nil || string(b) != "wrote" {
		t.Fatalf("%q %v", b, err)
	}
	args, err := os.ReadFile(filepath.Join(work, "args.txt"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(args)
	for _, want := range []string{"-p", "--permission-mode", "acceptEdits", "--allowedTools", "Read,Edit,Write", "Bash(npm *)", "Bash(npx *)", "Bash(node *)", "Bash(da plugin validate *)", "build a plugin"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func TestPromptIncludesFixReport(t *testing.T) {
	p := Prompt(PromptInput{Description: "watch a folder", FixReport: "watch: timed out", DirName: "draft-ab12"})
	for _, want := range []string{
		"watch a folder", "timed out", "guardrails",
		"current folder", "AGENTS.md", "vendored SDK", "never clone the repo",
		"da plugin validate", "DAP_SCAFFOLD_STARTER", "draft- placeholder",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("missing %q in %s", want, p)
		}
	}
	named := Prompt(PromptInput{Description: "watch a folder", DirName: "inbox-triage"})
	if strings.Contains(named, "placeholder") {
		t.Fatal(named)
	}
}

func TestOtherAgentsGetWriteFlags(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "agent")
	body := "#!/bin/sh\nprintf '%s\n' \"$@\" > \"$PWD/args.txt\"\nprintf '%s' \"$OPENCODE_CONFIG_CONTENT\" > \"$PWD/oc.txt\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := LookPath
	t.Cleanup(func() { LookPath = orig })
	for _, id := range []string{"codex", "opencode"} {
		LookPath = func(name string) (string, error) {
			if name == id {
				return script, nil
			}
			return "", os.ErrNotExist
		}
		work := t.TempDir()
		if err := Run(context.Background(), Request{Agent: id, Dir: work, Prompt: "build"}); err != nil {
			t.Fatal(err)
		}
		args, err := os.ReadFile(filepath.Join(work, "args.txt"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(args)
		switch id {
		case "codex":
			for _, want := range []string{"exec", "--sandbox", "workspace-write", "--skip-git-repo-check", "build"} {
				if !strings.Contains(text, want) {
					t.Fatalf("%s missing %q in %s", id, want, text)
				}
			}
		case "opencode":
			if !strings.Contains(text, "run") || !strings.Contains(text, "--auto") || !strings.Contains(text, "build") {
				t.Fatalf("%s %s", id, text)
			}
			oc, err := os.ReadFile(filepath.Join(work, "oc.txt"))
			if err != nil || !strings.Contains(string(oc), `"edit":"allow"`) || !strings.Contains(string(oc), "external_directory") {
				t.Fatalf("%s config %s %v", id, oc, err)
			}
		}
	}
}
