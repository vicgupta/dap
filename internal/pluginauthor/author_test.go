package pluginauthor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaAcceptsShippedPlugins(t *testing.T) {
	for _, dir := range []string{"../../plugins/hello-watch", "../../plugins/workflow"} {
		b, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if errs := SchemaErrors(b); len(errs) > 0 {
			t.Fatalf("%s: %v", dir, errs)
		}
	}
}

func TestScaffoldPythonValidates(t *testing.T) {
	dir := t.TempDir()
	path, err := Scaffold(Options{Name: "note-watch", Type: "watcher", Lang: "python", Dir: filepath.Join(dir, "note-watch"), Description: "Notice a local note."})
	if err != nil {
		t.Fatal(err)
	}
	rep := Validate(path)
	if !rep.OK {
		t.Fatalf("%+v", rep.Checks)
	}
}

func TestScaffoldRejectsBadName(t *testing.T) {
	if _, err := Scaffold(Options{Name: "Bad Name"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestScaffoldKeepsStarterMarker(t *testing.T) {
	dir := t.TempDir()
	path, err := Scaffold(Options{
		Name:        "inbox-triage",
		Type:        "watcher",
		Lang:        "python",
		Dir:         filepath.Join(dir, "inbox-triage"),
		Description: `Create a DAP plugin called "inbox-triage". Every hour, call GET /mail/threads.`,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(path, "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "name: mail.read") || strings.Contains(text, "name: mail.draft") {
		t.Fatalf("scaffold must not guess mail permissions from the description:\n%s", text)
	}
	if !MarkerPresent(path) {
		t.Fatal("starter marker missing")
	}
	helper, err := os.ReadFile(filepath.Join(path, "dap_plugin.py"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(helper), "def mail_draft") || !strings.Contains(string(helper), "newer_than") {
		t.Fatal("python helper is missing mail_draft or newer_than")
	}
	rep := Validate(path)
	if !rep.OK {
		t.Fatalf("%+v", rep.Checks)
	}
	plain, err := Scaffold(Options{Name: "note-watch", Type: "watcher", Lang: "python", Dir: filepath.Join(dir, "note-watch"), Description: "Notice a local note."})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(plain, "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "mail.read") {
		t.Fatalf("unrelated plugin declared mail: %s", raw)
	}
}

func TestTutorialExampleValidates(t *testing.T) {
	rep := Validate(filepath.Join("..", "..", "examples", "note-watch"))
	if !rep.OK {
		t.Fatalf("%+v", rep.Checks)
	}
}

func TestHelloWatchValidates(t *testing.T) {
	rep := Validate(filepath.Join("..", "..", "plugins", "hello-watch"))
	if !rep.OK {
		t.Fatalf("%+v", rep.Checks)
	}
}

func TestPluginsThatCallTheAPIValidate(t *testing.T) {
	for _, dir := range []string{"../../plugins/inbox-followups", "../../plugins/workflow", "../../plugins/mail-fake"} {
		rep := ValidateIn(dir, t.TempDir())
		if !rep.OK {
			t.Fatalf("%s: %+v", dir, rep.Checks)
		}
	}
}

func TestCompetitorResearchRepoCopyValidates(t *testing.T) {
	dir := filepath.Join("..", "..", "templates", "competitor-research")
	bare := Validate(dir)
	if bare.OK {
		t.Fatal("expected a clear SDK message without a data directory")
	}
	detail := ""
	for _, c := range bare.Checks {
		if !c.OK {
			detail += c.Detail
		}
	}
	if !strings.Contains(detail, "vendor/plugin-sdk/dist/index.js") || strings.Contains(detail, "ERR_MODULE_NOT_FOUND") {
		t.Fatalf("detail %q", detail)
	}
	rep := ValidateIn(dir, t.TempDir())
	if !rep.OK {
		t.Fatalf("%+v", rep.Checks)
	}
}
