package pluginauthor

import (
	"os"
	"path/filepath"
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

func TestSuggestType(t *testing.T) {
	if SuggestType("run the steps in order") != "workflow" {
		t.Fatal(SuggestType("run the steps in order"))
	}
	if SuggestType("a capability for other plugins") != "capability" {
		t.Fatal()
	}
	if SuggestType("tell me when a file changes") != "watcher" {
		t.Fatal()
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
