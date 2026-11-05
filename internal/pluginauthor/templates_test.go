package pluginauthor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vicgupta/dap/internal/manifest"
)

func TestEachTemplateValidates(t *testing.T) {
	list, err := ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 6 {
		t.Fatalf("%d templates", len(list))
	}
	want := []string{"inbox-triage", "follow-up-nudger", "daily-digest", "reply-drafter", "scheduled-checklist", "competitor-research"}
	installs := 0
	for i, tmpl := range list {
		if tmpl.ID != want[i] {
			t.Fatalf("order %d: %s", i, tmpl.ID)
		}
		if tmpl.Mode == ModeInstall {
			installs++
		} else if tmpl.Mode != ModeCustomize {
			t.Fatal(tmpl.Mode)
		}
		dir := filepath.Join(t.TempDir(), tmpl.ID)
		if err := CopyTemplate(tmpl.ID, dir); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "template.yaml")); !os.IsNotExist(err) {
			t.Fatal("template.yaml was copied into the plugin")
		}
		m, err := manifest.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, perm := range tmpl.Permissions {
			if _, ok := m.FindPermission(perm); !ok {
				t.Fatalf("%s missing %s in %+v", tmpl.ID, perm, m.Permissions)
			}
		}
		marked := MarkerPresent(dir)
		if tmpl.Mode == ModeInstall && marked {
			t.Fatalf("%s is an install template and still has the starter marker", tmpl.ID)
		}
		if tmpl.Mode == ModeCustomize && !marked {
			t.Fatalf("%s should start with the scaffold marker", tmpl.ID)
		}
		data := t.TempDir()
		rep := ValidateIn(dir, data)
		if !rep.OK {
			t.Fatalf("%s: %+v", tmpl.ID, rep.Checks)
		}
	}
	if installs < 1 {
		t.Fatal("expected an install-mode template")
	}
}
