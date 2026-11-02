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
	body := "#!/bin/sh\necho agent-line\nprintf 'wrote' > \"$PWD/touched.txt\"\n"
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
}

func TestPromptIncludesFixReport(t *testing.T) {
	p := Prompt("watch a folder", "watch: timed out")
	if !strings.Contains(p, "watch a folder") || !strings.Contains(p, "timed out") || !strings.Contains(p, "guardrails") {
		t.Fatal(p)
	}
}
