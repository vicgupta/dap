package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidAndShorthand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hello.py"), "print('hi')\n")
	writeFile(t, filepath.Join(dir, "ui", "index.html"), "<p>hi</p>\n")
	raw := `
name: competitor-research
version: 0.1.0
min_dap_version: 0.1.0
type: watcher
description: Compare public pages.
entrypoint: hello.py
dependencies:
  - web-watch ^0.3
  - name: rss-news
    version: ">=0.2.0"
permissions:
  web.read: public pages
  files.write: allowed
  share.team: ask first
provides: [alerts, brief]
schedule: every 2h
ui:
  entry: ui/index.html
  route: /plugins/competitor-research/
  title: Comparison board
`
	m, err := Parse([]byte(raw), dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Interface != "dap.plugin.v1" {
		t.Fatal(m.Interface)
	}
	if m.Schedule.Every != "2h" {
		t.Fatal(m.Schedule)
	}
	rawTile := raw + "tile:\n  size: large\n  min: medium\n"
	mt, err := Parse([]byte(rawTile), dir)
	if err != nil {
		t.Fatal(err)
	}
	if mt.Tile == nil || mt.Tile.Size != "large" || mt.Tile.Min != "medium" {
		t.Fatalf("tile %+v", mt.Tile)
	}
	if len(m.Dependencies) != 2 || m.Dependencies[0].Version != "^0.3" {
		t.Fatalf("deps %+v", m.Dependencies)
	}
	web, ok := m.FindPermission("web.read")
	if !ok || web.Mode != ModeAskFirst || web.Description != "public pages" {
		t.Fatalf("web.read %+v", web)
	}
	files, _ := m.FindPermission("files.write")
	if files.Mode != ModeAllowed {
		t.Fatal(files)
	}
	share, _ := m.FindPermission("share.team")
	if share.Mode != ModeAskFirst {
		t.Fatal(share)
	}
	argv, err := m.Entrypoint.Command(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) != 2 || !strings.HasSuffix(argv[1], "hello.py") {
		t.Fatal(argv)
	}
}

func TestRejects(t *testing.T) {
	base := func() string {
		return `
name: hello-watch
version: 0.1.0
min_dap_version: 0.1.0
type: watcher
entrypoint: hello.py
permissions:
  - name: demo.greet
    mode: ask_first
    description: Say hello
`
	}
	cases := map[string]string{
		"bad version":    strings.Replace(base(), "version: 0.1.0", "version: beta", 1),
		"bad type":       strings.Replace(base(), "type: watcher", "type: robot", 1),
		"bad mode":       strings.Replace(base(), "mode: ask_first", "mode: sometimes", 1),
		"bad range":      base() + "dependencies:\n  - other >>>1\n",
		"dup perm":       strings.Replace(base(), "demo.greet", "demo.greet", 1) + "  - name: demo.greet\n    mode: allowed\n",
		"future dap":     strings.Replace(base(), "min_dap_version: 0.1.0", "min_dap_version: 99.0.0", 1),
		"bad name":       strings.Replace(base(), "name: hello-watch", "name: Hello", 1),
		"two schedules":  base() + "schedule:\n  every: 1h\n  cron: \"0 * * * *\"\n",
		"short every":    base() + "schedule:\n  every: 1s\n",
		"bad cron":       base() + "schedule:\n  cron: \"nope\"\n",
		"escape entry":   strings.Replace(base(), "entrypoint: hello.py", "entrypoint: ../hello.py", 1),
		"bad tile":       base() + "tile:\n  size: huge\n",
		"tile below min": base() + "tile:\n  size: small\n  min: large\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "hello.py"), "x")
			if _, err := Parse([]byte(raw), dir); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestResolveConfig(t *testing.T) {
	m := Manifest{Config: []ConfigField{
		{Key: "every", Type: "duration", Default: "1h"},
		{Key: "steps", Type: "list", Default: []any{}},
	}}
	cfg, err := ResolveConfig(m, map[string]any{"every": "30m"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg["every"] != "30m" {
		t.Fatal(cfg)
	}
	if _, err := ResolveConfig(m, map[string]any{"nope": true}); err == nil {
		t.Fatal("unknown key should fail")
	}
	if _, err := ResolveConfig(m, map[string]any{"every": "1s"}); err == nil {
		t.Fatal("short duration should fail")
	}
}

func TestOrder(t *testing.T) {
	ms := []Manifest{
		{Name: "workflow", Version: "0.1.0", Dependencies: []Dependency{{Name: "watcher", Version: ">=0.1.0"}}},
		{Name: "cap", Version: "0.2.0", Type: TypeCapability},
		{Name: "watcher", Version: "0.1.0", Dependencies: []Dependency{{Name: "cap", Version: "^0.2.0"}}},
	}
	order, err := Order(ms)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "cap,watcher,workflow" {
		t.Fatalf("order %v", order)
	}
	_, err = Order([]Manifest{{Name: "a", Version: "0.1.0", Dependencies: []Dependency{{Name: "missing", Version: "*"}}}})
	if err == nil {
		t.Fatal("expected missing dependency")
	}
	_, err = Order([]Manifest{
		{Name: "a", Version: "0.1.0", Dependencies: []Dependency{{Name: "b", Version: "^2.0.0"}}},
		{Name: "b", Version: "1.0.0"},
	})
	if err == nil {
		t.Fatal("expected range mismatch")
	}
	_, err = Order([]Manifest{
		{Name: "a", Version: "0.1.0", Dependencies: []Dependency{{Name: "b", Version: "*"}}},
		{Name: "b", Version: "0.1.0", Dependencies: []Dependency{{Name: "a", Version: "*"}}},
	})
	if err == nil {
		t.Fatal("expected cycle")
	}
}

func TestSamePermissions(t *testing.T) {
	approved := []Permission{{Name: "web.read", Mode: ModeAskFirst}}
	if !SamePermissions(approved, []Permission{{Name: "web.read", Mode: ModeAskFirst}}) {
		t.Fatal("same set should match")
	}
	if SamePermissions(approved, []Permission{{Name: "web.read", Mode: ModeAllowed}}) {
		t.Fatal("loosening must require re-approval")
	}
	if SamePermissions(approved, []Permission{{Name: "web.read", Mode: ModeAskFirst}, {Name: "mail.send", Mode: ModeAskFirst}}) {
		t.Fatal("new permission must require re-approval")
	}
	// Tightening in the manifest is still covered by the old approval.
	if !SamePermissions([]Permission{{Name: "web.read", Mode: ModeAllowed}}, []Permission{{Name: "web.read", Mode: ModeAskFirst}}) {
		t.Fatal("tighter manifest should still be covered")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
