package composio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinMailPair(t *testing.T) {
	reg, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	gmail, ok := reg.ByID("gmail")
	if !ok || gmail.Kind != "mail" || gmail.Actions["send"] == "" {
		t.Fatalf("%+v", gmail)
	}
	outlook, ok := reg.ByID("graph")
	if !ok || outlook.ID != "outlook" || !outlook.Fallback {
		t.Fatalf("%+v", outlook)
	}
	_, tool, ok := reg.Tool("gmail", "GMAIL_SEND_EMAIL")
	if !ok || len(tool.Effects) != 1 || tool.Effects[0] != "sends" {
		t.Fatalf("%+v", tool)
	}
	if _, _, ok := reg.Tool("gmail", "GMAIL_DELETE_MESSAGE"); ok {
		t.Fatal("undeclared tool must be denied")
	}
	for _, tk := range reg.List() {
		if tk.ID == "teams" || tk.Slug == "microsoft_teams" {
			t.Fatal("teams is the next toolkit, not a builtin connection")
		}
	}
}

func TestFileAddsToolkitButCannotReplaceGmail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "toolkits.json")
	if err := os.WriteFile(path, []byte(`{"toolkits":[{"id":"gmail","slug":"gmail","label":"Nope","tools":[{"slug":"GMAIL_SEND_EMAIL","permission":"mail.read"}]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "built in") {
		t.Fatal(err)
	}
	extra := `{"toolkits":[{"id":"tasks","slug":"todoist","label":"Tasks","tools":[
		{"slug":"TODOIST_GET_TASKS","permission":"tasks.read"},
		{"slug":"TODOIST_CREATE_TASK","permission":"tasks.write","effects":["sends"]}
	]}]}`
	if err := os.WriteFile(path, []byte(extra), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	tk, tool, ok := reg.Tool("tasks", "TODOIST_CREATE_TASK")
	if !ok || tk.Label != "Tasks" || tool.Permission != "tasks.write" {
		t.Fatalf("%+v %+v", tk, tool)
	}
	secret := `{"toolkits":[{"id":"notes","slug":"notes","label":"Notes","api_key":"nope","tools":[{"slug":"NOTES_LIST","permission":"notes.read"}]}]}`
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "api key") {
		t.Fatal(err)
	}
}
