package agentrun

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExtraBinDirsAddsNothing(t *testing.T) {
	home := t.TempDir()
	for _, rel := range []string{
		".nvm/versions/node/v22.2.0/bin",
		".volta/bin",
		".local/bin",
	} {
		if err := os.MkdirAll(filepath.Join(home, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(string) bool { return true }
	for _, goos := range []string{"darwin", "linux", "windows"} {
		if got := extraBinDirs(home, goos, exists); len(got) != 0 {
			t.Fatalf("%s: %v", goos, got)
		}
		if got := augmentPath("/usr/bin", home, goos); got != "/usr/bin" {
			t.Fatalf("%s augment: %s", goos, got)
		}
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
	env := commandEnv("opencode", "")
	var found string
	for _, e := range env {
		if strings.HasPrefix(e, "OPENCODE_CONFIG_CONTENT=") {
			found = e
		}
	}
	if found == "" || !strings.Contains(found, `"edit":"allow"`) || !strings.Contains(found, "npm *") || !strings.Contains(found, "da plugin validate") {
		t.Fatalf("%s", found)
	}
	if strings.Contains(commandEnv("claude", "")[len(commandEnv("claude", ""))-1], "OPENCODE_CONFIG_CONTENT=") && !strings.HasPrefix(os.Getenv("OPENCODE_CONFIG_CONTENT"), "") {
		// claude must not gain the opencode override unless the parent already had it
	}
	for _, e := range commandEnv("claude", "") {
		if strings.HasPrefix(e, "OPENCODE_CONFIG_CONTENT=") && os.Getenv("OPENCODE_CONFIG_CONTENT") == "" {
			t.Fatal("claude env should not invent an opencode config")
		}
	}
}

func TestCommandEnvKeepsHomeAndRedirectsTemp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", "/keep/this-home")
	t.Setenv("TMPDIR", "/system/tmp")
	env := commandEnv("claude", dir)
	got := map[string]string{}
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if ok {
			got[k] = v
		}
	}
	if got["HOME"] != "/keep/this-home" {
		t.Fatalf("HOME %q", got["HOME"])
	}
	if got["TMPDIR"] != filepath.Join(dir, "tmp") || got["TEMP"] != got["TMPDIR"] || got["TMP"] != got["TMPDIR"] {
		t.Fatalf("temp %#v", got)
	}
	if got["npm_config_cache"] != filepath.Join(dir, "cache", "npm") {
		t.Fatalf("npm %q", got["npm_config_cache"])
	}
	if got["PIP_CACHE_DIR"] != filepath.Join(dir, "cache", "pip") {
		t.Fatalf("pip %q", got["PIP_CACHE_DIR"])
	}
}
