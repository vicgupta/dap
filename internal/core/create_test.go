package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/agentrun"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginauthor"
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
	if _, err := app.ProposePluginChange(draft.Path, ""); err == nil || !strings.Contains(err.Error(), "starter") {
		t.Fatalf("unchanged scaffold must not be proposed: %v", err)
	}
	src := filepath.Join(draft.Path, "plugin.py")
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(string(body), pluginauthor.ScaffoldMarker, "watching notes"))
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
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

const vicPrompt = `Create a DAP plugin called "inbox-triage" in TypeScript, following docs/plugin-authoring.md. Every hour, call GET /mail/threads for the last 24h, score threads by how critical they are and whether they need a reply, show the top 5 on its tile. Read-only; offer "Draft reply" as a change card; never send.`

func TestNameComesFromCalledPhraseNotSentence(t *testing.T) {
	app, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	draft, err := app.ScaffoldDraft(DraftRequest{Description: vicPrompt, Lang: "ts"})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Name != "inbox-triage" || filepath.Base(draft.Path) != "inbox-triage" {
		t.Fatalf("%+v", draft)
	}
	if strings.Contains(draft.Name, "create-a-dap") {
		t.Fatal(draft.Name)
	}
	raw, err := os.ReadFile(filepath.Join(draft.Path, "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "name: mail.read") || !strings.Contains(string(raw), "name: mail.draft") {
		t.Fatalf("%s", raw)
	}
	explicit, err := app.ScaffoldDraft(DraftRequest{Name: "Folder Watch", Description: `called "inbox-triage"`, Lang: "python"})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Name != "folder-watch" {
		t.Fatal(explicit.Name)
	}
	plain, err := app.ScaffoldDraft(DraftRequest{Description: "Watch a folder and tell me when a file changes every hour with lots of detail about the files", Lang: "python"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain.Name, "draft-") || strings.Contains(plain.Name, "watch-a-folder") {
		t.Fatal(plain.Name)
	}
}

func TestAgentRefusalDoesNotPropose(t *testing.T) {
	app := openWithAgent(t, "#!/bin/sh\necho refusing to edit\nexit 0\n")
	draft := scaffoldVic(t, app)
	run := waitRun(t, app, draft.Path)
	if run.Status != "finished" || run.Changed == nil || *run.Changed {
		t.Fatalf("%+v", run)
	}
	if run.Message == "" || !strings.Contains(run.Log, "refusing") {
		t.Fatalf("%+v", run)
	}
	if _, err := app.ProposePluginChange(draft.Path, ""); err == nil {
		t.Fatal("unchanged scaffold was proposed")
	}
	if len(app.store.Snapshot().Cards) != 0 {
		t.Fatal("install card was created")
	}
}

func TestAgentFailureDoesNotPropose(t *testing.T) {
	app := openWithAgent(t, "#!/bin/sh\necho boom >&2\nexit 1\n")
	draft := scaffoldVic(t, app)
	run := waitRun(t, app, draft.Path)
	if run.Status != "failed" || run.Message == "" || !strings.Contains(run.Message, "did not propose") {
		t.Fatalf("%+v", run)
	}
	if _, err := app.ProposePluginChange(draft.Path, ""); err == nil {
		t.Fatal("failed run still proposed")
	}
}

func TestAgentWriteProposesInboxTriage(t *testing.T) {
	app := openWithAgent(t, writeAgentScript)
	draft := scaffoldVic(t, app)
	run := waitRun(t, app, draft.Path)
	if run.Status != "finished" || run.Changed == nil || !*run.Changed {
		t.Fatalf("%+v", run)
	}
	card, err := app.ProposePluginChange(draft.Path, "")
	if err != nil {
		t.Fatal(err)
	}
	if card.Plugin != "inbox-triage" || card.Title != "Install inbox-triage" {
		t.Fatalf("%+v", card)
	}
	if !strings.Contains(card.Body, "mail.read") {
		t.Fatalf("%s", card.Body)
	}
	if _, ok := app.store.Snapshot().Plugins["inbox-triage"]; ok {
		t.Fatal("proposing must not install")
	}
}

func TestAgentRenameBeforePropose(t *testing.T) {
	app := openWithAgent(t, renameAgentScript)
	draft, err := app.ScaffoldDraft(DraftRequest{
		Description: "Watch a folder and tell me when a file changes every hour with lots of detail",
		Lang:        "python",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(draft.Name, "draft-") {
		t.Fatal(draft.Name)
	}
	run := waitRun(t, app, draft.Path)
	if run.Status != "finished" || run.Changed == nil || !*run.Changed {
		t.Fatalf("%+v", run)
	}
	card, err := app.ProposePluginChange(draft.Path, "")
	if err != nil {
		t.Fatal(err)
	}
	if card.Plugin != "folder-watch" || !strings.Contains(card.Title, "folder-watch") {
		t.Fatalf("%+v", card)
	}
	if _, err := os.Stat(draft.Path); !os.IsNotExist(err) {
		t.Fatal("old draft folder should be renamed")
	}
}

func openWithAgent(t *testing.T, script string) *App {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := agentrun.LookPath
	agentrun.LookPath = func(name string) (string, error) {
		if name == "claude" {
			return bin, nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { agentrun.LookPath = orig })
	app, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func scaffoldVic(t *testing.T, app *App) DraftResult {
	t.Helper()
	draft, err := app.ScaffoldDraft(DraftRequest{Description: vicPrompt, Lang: "ts", Type: "watcher"})
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func waitRun(t *testing.T, app *App, path string) AgentRun {
	t.Helper()
	started, err := app.StartAgent(AgentStart{Path: path, Agent: "claude", Description: vicPrompt})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		run, err := app.AgentRun(started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != "running" {
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not finish")
	return AgentRun{}
}

const writeAgentScript = `#!/bin/sh
echo writing plugin
python3 - <<'PY'
from pathlib import Path
src = Path("src/index.ts")
text = src.read_text().replace("DAP_SCAFFOLD_STARTER", "inbox triage").replace('value: "idle"', 'value: "5 threads"')
src.write_text(text)
Path("dist").mkdir(exist_ok=True)
Path("dist/index.js").write_text(r'''
let buf = "";
process.stdin.setEncoding("utf8");
process.stdin.on("data", (chunk) => {
  buf += chunk;
  let nl;
  while ((nl = buf.indexOf("\n")) >= 0) {
    const line = buf.slice(0, nl).trim();
    buf = buf.slice(nl + 1);
    if (!line) continue;
    const req = JSON.parse(line);
    const reply = (result, error) => {
      const msg = { jsonrpc: "2.0", id: req.id };
      if (error) msg.error = { code: -32601, message: error };
      else msg.result = result;
      process.stdout.write(JSON.stringify(msg) + "\n");
    };
    const method = req.method;
    if (method === "initialize") reply({ ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "ready" } });
    else if (method === "watch") reply({ observations: [], tile: { title: "inbox-triage", status: "ok", lines: [{ label: "Top", value: "5 threads" }] } });
    else if (method === "propose") reply({ cards: [{ title: "Draft reply", summary: "Nothing is sent.", kind: "change", permission: "mail.draft", effects: [] }] });
    else if (method === "act") reply({ status: "done", summary: "Draft saved." });
    else if (method === "report") reply({ metrics: [{ key: "runs", label: "Runs", value: 1 }] });
    else if (method === "shutdown") reply({ ok: true });
    else reply(null, "unknown method " + method);
  }
});
''')
PY
touch dist/index.js
`

const renameAgentScript = `#!/bin/sh
python3 - <<'PY'
from pathlib import Path
yaml = Path("plugin.yaml")
lines = yaml.read_text().splitlines()
lines[0] = "name: folder-watch"
yaml.write_text("\n".join(lines) + "\n")
src = Path("plugin.py")
src.write_text(src.read_text().replace("DAP_SCAFFOLD_STARTER", "watching files").replace("idle", "watching"))
PY
echo renamed
`
