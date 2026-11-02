package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitInstallListCheck(t *testing.T) {
	dir := t.TempDir()
	if code, out, errOut := run(t, dir, "init"); code != 0 {
		t.Fatalf("init %s %s", out, errOut)
	}
	root := repoRoot(t)
	code, out, errOut := run(t, dir, "plugin", "install", filepath.Join(root, "plugins", "hello-watch"))
	if code != 0 {
		t.Fatalf("preview %s %s", out, errOut)
	}
	var preview map[string]any
	if err := json.Unmarshal([]byte(out), &preview); err != nil {
		t.Fatal(err)
	}
	if preview["status"] != "approval_required" {
		t.Fatalf("%s", out)
	}
	code, out, errOut = run(t, dir, "plugin", "install", "--approve", filepath.Join(root, "plugins", "hello-watch"))
	if code != 0 {
		t.Fatalf("install %s %s", out, errOut)
	}
	code, out, errOut = run(t, dir, "plugin", "list")
	if code != 0 || !strings.Contains(out, "hello-watch") {
		t.Fatalf("list %d %s %s", code, out, errOut)
	}
	code, out, errOut = run(t, dir, "check")
	if code == 0 {
		t.Fatal("check without stdin should fail when stdin is empty")
	}
	in := bytes.NewBufferString(`{"plugin":"hello-watch","permission":"mail.send","effects":["sends"]}`)
	var stdout, stderr bytes.Buffer
	code = Run([]string{"--data", dir, "check"}, Options{Stdin: in, Stdout: &stdout, Stderr: &stderr})
	if code != 0 {
		t.Fatalf("check code %d %s %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"decision": "never"`) {
		t.Fatalf("check output %s", stdout.String())
	}
}

func TestServeFlags(t *testing.T) {
	addr, open := serveFlags(nil, true)
	if addr != "" || !open {
		t.Fatalf("default run: %q %v", addr, open)
	}
	addr, open = serveFlags([]string{"--addr", "127.0.0.1:9", "--no-browser"}, true)
	if addr != "127.0.0.1:9" || open {
		t.Fatalf("no-browser: %q %v", addr, open)
	}
	addr, open = serveFlags([]string{"--addr=127.0.0.1:8", "--no-browser", "--open"}, false)
	if addr != "127.0.0.1:8" || !open {
		t.Fatalf("open after no-browser: %q %v", addr, open)
	}
	_, open = serveFlags([]string{"--open", "--no-browser"}, true)
	if open {
		t.Fatal("no-browser should win when it comes last")
	}
	_, open = serveFlags(nil, false)
	if open {
		t.Fatal("serve default stays closed")
	}
}

func TestUsageMentionsNoBrowser(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, Options{Stdout: &stdout, Stderr: &stderr})
	if code != 0 {
		t.Fatal(code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--no-browser") {
		t.Fatalf("usage: %s", stdout.String())
	}
}

func TestVersion(t *testing.T) {
	for _, arg := range []string{"--version", "version", "-V"} {
		var stdout, stderr bytes.Buffer
		code := Run([]string{arg}, Options{Stdout: &stdout, Stderr: &stderr})
		if code != 0 {
			t.Fatalf("%s code %d %s %s", arg, code, stdout.String(), stderr.String())
		}
		var body map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v %s", arg, err, stdout.String())
		}
		if body["ok"] != true || body["version"] != "0.1.0" {
			t.Fatalf("%s: %s", arg, stdout.String())
		}
		if _, ok := body["release"]; ok {
			t.Fatalf("%s: dev build should omit release: %s", arg, stdout.String())
		}
	}
}

func run(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	full := append([]string{"--data", dir}, args...)
	code := Run(full, Options{Stdin: bytes.NewReader(nil), Stdout: &stdout, Stderr: &stderr})
	return code, stdout.String(), stderr.String()
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod")
		}
		dir = parent
	}
}
