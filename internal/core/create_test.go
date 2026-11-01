package core

import (
	"path/filepath"
	"testing"

	"github.com/vicgupta/dap/internal/model"
)

func TestDraftValidateAndApproveInstalls(t *testing.T) {
	app, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	draft, err := app.ScaffoldDraft(DraftRequest{
		Name:        "note-watch",
		Description: "Notice a local note and ask before recording it.",
		Type:        "watcher",
		Lang:        "python",
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(draft.Path) != "note-watch" {
		t.Fatal(draft.Path)
	}
	rep, err := app.ValidateDraft(draft.Path, "")
	if err != nil || !rep.OK {
		t.Fatalf("%v %+v", err, rep.Checks)
	}
	card, err := app.ProposePluginChange(draft.Path, "")
	if err != nil {
		t.Fatal(err)
	}
	if card.Kind != model.KindChange || card.Status != model.StatusPending {
		t.Fatalf("%+v", card)
	}
	if _, ok := app.store.Snapshot().Plugins["note-watch"]; ok {
		t.Fatal("proposing a card must not install the plugin")
	}
	done, err := app.Approve(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != model.StatusDone {
		t.Fatalf("%s %v", done.Status, done.Result)
	}
	snap := app.store.Snapshot()
	if snap.Plugins["note-watch"] == nil {
		t.Fatal("plugin was not installed on approval")
	}
}

func TestProposeRejectsPathOutsideDrafts(t *testing.T) {
	app, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, err := app.ProposePluginChange(t.TempDir(), ""); err == nil {
		t.Fatal("expected path rejection")
	}
}
