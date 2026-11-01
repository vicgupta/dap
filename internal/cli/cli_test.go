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

func TestVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		code, out, errOut := run(t, t.TempDir(), args...)
		if code != 0 || !strings.Contains(out, `"version"`) || errOut != "" {
			t.Fatalf("%v code %d out %s err %s", args, code, out, errOut)
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
