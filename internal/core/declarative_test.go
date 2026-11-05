package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicgupta/dap/internal/model"
)

func TestDeclarativeInstallRunAndUnload(t *testing.T) {
	app := openTest(t)
	dir := filepath.Join(repoRoot(t), "templates", "follow-up-nudger")
	preview, err := app.Install(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != "approval_required" || !strings.Contains(preview.Sample, "Sample run") {
		t.Fatalf("%+v", preview)
	}
	found := map[string]string{}
	for _, p := range preview.Permissions {
		found[p.Name] = p.Mode
	}
	if found["mail.read"] != "allowed" || found["mail.draft"] != "ask_first" {
		t.Fatalf("%v", found)
	}
	if _, err := app.Install(dir, true); err != nil {
		t.Fatal(err)
	}
	if err := app.StartPlugins(); err != nil {
		t.Fatal(err)
	}
	if !app.host.Running("follow-up-nudger") {
		t.Fatal("declarative plugin did not start")
	}
	if err := app.MailConnect(context.Background(), "fake"); err != nil {
		t.Fatal(err)
	}
	if err := app.RunPlugin("follow-up-nudger", "manual"); err != nil {
		t.Fatal(err)
	}
	var pending model.Card
	for _, c := range app.Inbox() {
		if c.Plugin == "follow-up-nudger" && c.Status == model.StatusPending {
			pending = c
			break
		}
	}
	if pending.ID == "" {
		t.Fatal("expected a pending card; declarative plugins must not auto-act")
	}
	for _, c := range app.store.Snapshot().Cards {
		if c.Plugin == "follow-up-nudger" && (c.Status == model.StatusDone || c.Status == model.StatusActing) {
			t.Fatalf("acted without approval: %+v", c)
		}
	}
	done, err := app.Approve(pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != model.StatusDone || done.Result == nil || done.Result.Status != "done" {
		t.Fatalf("%+v", done)
	}
	if err := app.Remove("follow-up-nudger"); err != nil {
		t.Fatal(err)
	}
	if app.host.Running("follow-up-nudger") {
		t.Fatal("handler still registered")
	}
	snap := app.store.Snapshot()
	if snap.Plugins["follow-up-nudger"] != nil {
		t.Fatal("plugin record remains")
	}
	if _, ok := snap.PluginState["follow-up-nudger"]; ok {
		t.Fatal("scratch state remains")
	}
	if _, err := os.Stat(filepath.Join(app.DataDir, "plugin-data", "follow-up-nudger")); !os.IsNotExist(err) {
		t.Fatalf("plugin-data remains: %v", err)
	}
}
