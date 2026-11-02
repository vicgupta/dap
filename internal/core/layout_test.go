package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vicgupta/dap/internal/layout"
)

func TestLayoutPreservesOtherConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{\n  \"addr\": \"127.0.0.1:9\",\n  \"theme\": \"dark\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if got := app.Layout(); got.Home.View != layout.Grid || got.Inbox.View != layout.List {
		t.Fatalf("default %+v", got)
	}
	next, err := app.SetLayout(layout.Settings{
		Home:  layout.View{View: layout.List, Density: layout.Compact, Pinned: []string{"hello-watch"}},
		Inbox: layout.View{View: layout.Board, Density: layout.Comfortable},
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.Home.Density != layout.Compact || next.Inbox.View != layout.Board {
		t.Fatal(next)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err, string(raw))
	}
	if doc["theme"] != "dark" || doc["addr"] != "127.0.0.1:9" {
		t.Fatalf("config changed: %s", raw)
	}
	again := app.Layout()
	if len(again.Home.Pinned) != 1 || again.Home.Pinned[0] != "hello-watch" {
		t.Fatal(again.Home)
	}
}

func TestHomeReportsTileSize(t *testing.T) {
	app, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, err := app.Install(filepath.Join("..", "..", "plugins", "hello-watch"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Install(filepath.Join("..", "..", "plugins", "inbox-followups"), true); err != nil {
		t.Fatal(err)
	}
	var hello, follow string
	for _, p := range app.Home().Plugins {
		switch p.Name {
		case "hello-watch":
			hello = p.TileSize + "/" + p.TileMin
		case "inbox-followups":
			follow = p.TileSize + "/" + p.TileMin
		}
	}
	if hello != "small/small" {
		t.Fatalf("hello-watch tile %s", hello)
	}
	if follow != "large/medium" {
		t.Fatalf("inbox-followups tile %s", follow)
	}
}
