package agentrun

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExtraBinDirsDarwinOrder(t *testing.T) {
	home := t.TempDir()
	for _, rel := range []string{
		".nvm/versions/node/v20.10.0/bin",
		".nvm/versions/node/v22.2.0/bin",
		".nvm/versions/node/v9.1.0/bin",
		".volta/bin",
		".local/bin",
	} {
		if err := os.MkdirAll(filepath.Join(home, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dirs := extraBinDirs(home, "darwin", func(p string) bool {
		if p == "/opt/homebrew/bin" || p == "/usr/local/bin" {
			return true
		}
		st, err := os.Stat(p)
		return err == nil && st.IsDir()
	})
	want := []string{
		"/opt/homebrew/bin",
		"/usr/local/bin",
		filepath.Join(home, ".nvm/versions/node/v22.2.0/bin"),
		filepath.Join(home, ".nvm/versions/node/v20.10.0/bin"),
		filepath.Join(home, ".nvm/versions/node/v9.1.0/bin"),
		filepath.Join(home, ".volta/bin"),
		filepath.Join(home, ".local/bin"),
	}
	if strings.Join(dirs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(dirs, "\n"), strings.Join(want, "\n"))
	}
}

func TestExtraBinDirsSkippedOffDarwin(t *testing.T) {
	if got := extraBinDirs("/home/someone", "linux", func(string) bool { return true }); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	if got := extraBinDirs("/Users/someone", "windows", func(string) bool { return true }); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

func TestAugmentPathPrependsAndDedupes(t *testing.T) {
	got := joinPath("/usr/bin:/opt/homebrew/bin:/bin", []string{"/opt/homebrew/bin", "/usr/local/bin"})
	want := "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
	if runtime.GOOS == "windows" {
		want = strings.ReplaceAll(want, ":", ";")
		got = joinPath(strings.ReplaceAll("/usr/bin:/opt/homebrew/bin:/bin", ":", ";"), []string{"/opt/homebrew/bin", "/usr/local/bin"})
	}
	if got != want {
		t.Fatalf("%s", got)
	}
}

func TestLookInFindsExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := lookIn(dir, "claude")
	if err != nil || got != bin {
		t.Fatalf("%s %v", got, err)
	}
	if _, err := lookIn(dir, "missing"); err == nil {
		t.Fatal("expected miss")
	}
}

func TestCommandEnvScopesOpenCode(t *testing.T) {
	env := commandEnv("opencode")
	var found string
	for _, e := range env {
		if strings.HasPrefix(e, "OPENCODE_CONFIG_CONTENT=") {
			found = e
		}
	}
	if found == "" || !strings.Contains(found, `"edit":"allow"`) || !strings.Contains(found, "npm *") || !strings.Contains(found, "da plugin validate") {
		t.Fatalf("%s", found)
	}
	if strings.Contains(commandEnv("claude")[len(commandEnv("claude"))-1], "OPENCODE_CONFIG_CONTENT=") && !strings.HasPrefix(os.Getenv("OPENCODE_CONFIG_CONTENT"), "") {
		// claude must not gain the opencode override unless the parent already had it
	}
	for _, e := range commandEnv("claude") {
		if strings.HasPrefix(e, "OPENCODE_CONFIG_CONTENT=") && os.Getenv("OPENCODE_CONFIG_CONTENT") == "" {
			t.Fatal("claude env should not invent an opencode config")
		}
	}
}
