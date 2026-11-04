package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/agentrun"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginauthor"
)

func TestCreateJobProposesOnce(t *testing.T) {
	app := openWithAgent(t, writeAgentScript)
	calls := 0
	orig := validatePlugin
	validatePlugin = func(dir, dataDir string) pluginauthor.Report {
		calls++
		return orig(dir, dataDir)
	}
	t.Cleanup(func() { validatePlugin = orig })
	job := waitJob(t, app, CreateRequest{
		Description: vicPrompt,
		Agent:       "claude",
		AutoInstall: false,
	})
	if job.Status != createReady || job.Card == nil {
		t.Fatalf("%+v", job)
	}
	if calls != 1 {
		t.Fatalf("validate ran %d times", calls)
	}
	if job.Card.Plugin != "inbox-triage" || job.Card.Status != model.StatusPending || job.Card.Actor != model.ActorExternal {
		t.Fatalf("%+v", job.Card)
	}
	if _, ok := app.store.Snapshot().Plugins["inbox-triage"]; ok {
		t.Fatal("a pending card must not install")
	}
	done, err := app.Approve(job.Card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != model.StatusDone {
		t.Fatalf("%s", done.Status)
	}
	if app.store.Snapshot().Plugins["inbox-triage"] == nil {
		t.Fatal("approval did not install")
	}
}

func TestCreateReloadMidRunPersists(t *testing.T) {
	app := openWithAgent(t, "#!/bin/sh\necho started\nsleep 30\n")
	started, err := app.CreatePlugin(CreateRequest{Description: "Watch a folder.", Agent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var view CreateView
	for time.Now().Before(deadline) {
		view, err = app.CreateJob(started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(view.Log, "started") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(view.Log, "started") || view.Status != createRunning {
		t.Fatalf("%+v", view)
	}
	raw, err := os.ReadFile(filepath.Join(app.DataDir, "create", started.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"status": "running"`) || !strings.Contains(string(raw), "started") {
		t.Fatalf("disk job lost the run:\n%s", raw)
	}
	if err := app.CancelCreate(started.ID); err != nil {
		t.Fatal(err)
	}
	view, err = app.CreateJob(started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != createCancelled {
		t.Fatalf("%+v", view)
	}
}

func TestCreateRestartShowsJob(t *testing.T) {
	dir := t.TempDir()
	app, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	rec := createRecord{
		ID:        "job_restart",
		Status:    createRunning,
		Step:      "agent",
		Log:       "still going\n",
		Path:      filepath.Join(dir, "drafts", "draft-restart"),
		UpdatedAt: time.Now().UTC(),
	}
	if err := os.MkdirAll(filepath.Join(dir, "create"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.MarshalIndent(rec, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "create", rec.ID+".json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	view, ok := app.CurrentCreate()
	if !ok {
		t.Fatal("restart hid the job")
	}
	if view.Status != createFailed || !strings.Contains(view.Error, "restarted") || !strings.Contains(view.Log, "still going") {
		t.Fatalf("%+v", view)
	}
}

func TestAutoInstallSuccess(t *testing.T) {
	app := openWithAgent(t, "#!/bin/sh\necho should-not-run\nexit 1\n")
	calls := 0
	orig := agentrun.LookPath
	agentrun.LookPath = func(name string) (string, error) {
		calls++
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { agentrun.LookPath = orig })
	job := waitJob(t, app, CreateRequest{Template: "inbox-triage", AutoInstall: true})
	if calls != 0 {
		t.Fatalf("install template started an agent %d times", calls)
	}
	if job.Status != createInstalled || job.Card == nil {
		t.Fatalf("%+v", job)
	}
	if job.Card.Actor != model.ActorUser || job.Card.Reason != preapprovedReason {
		t.Fatalf("%+v", job.Card)
	}
	if job.Card.Status != model.StatusDone && job.Card.Status != model.StatusApproved {
		t.Fatalf("%s", job.Card.Status)
	}
	plug := app.store.Snapshot().Plugins["inbox-triage"]
	if plug == nil {
		t.Fatal("not installed")
	}
	if plug.Config["provider"] != "gmail" {
		t.Fatalf("config %+v", plug.Config)
	}
	rows := ledgerKinds(t, app)
	if !rows[ledger.KindProposed] || !rows[ledger.KindApproved] {
		t.Fatalf("%v", rows)
	}
}

func TestAutoInstallFailedCheck(t *testing.T) {
	app := openWithAgent(t, breakAgentScript)
	job := waitJob(t, app, CreateRequest{
		Description: "Watch a folder and tell me when a file changes.",
		Agent:       "claude",
		AutoInstall: true,
	})
	if job.Status != createFailed {
		t.Fatalf("%+v", job)
	}
	if len(job.FailedChecks) == 0 {
		t.Fatalf("missing failed checks: %+v", job)
	}
	if job.Card != nil && (job.Card.Status == model.StatusApproved || job.Card.Status == model.StatusDone) {
		t.Fatalf("failed check wrote an approved card: %+v", job.Card)
	}
	if len(app.store.Snapshot().Plugins) != 0 {
		t.Fatal("installed despite a failed check")
	}
	for _, card := range app.store.Snapshot().Cards {
		if card.Status == model.StatusApproved || card.Status == model.StatusDone {
			t.Fatalf("approved card %s", card.Status)
		}
	}
}

func TestInstallTemplatesSkipAgent(t *testing.T) {
	app, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	orig := agentrun.LookPath
	agentrun.LookPath = func(name string) (string, error) {
		t.Fatalf("agent lookup for %s", name)
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { agentrun.LookPath = orig })
	list, err := pluginauthor.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, tmpl := range list {
		if tmpl.Mode != pluginauthor.ModeInstall {
			continue
		}
		n++
		job := waitJob(t, app, CreateRequest{Template: tmpl.ID, AutoInstall: true, Settings: map[string]any{}})
		if job.Status != createInstalled {
			t.Fatalf("%s: %+v", tmpl.ID, job)
		}
		if app.store.Snapshot().Plugins[tmpl.ID] == nil {
			t.Fatalf("%s not installed", tmpl.ID)
		}
	}
	if n < 4 {
		t.Fatalf("install templates: %d", n)
	}
}

func TestCustomizeRunsAgent(t *testing.T) {
	app := openWithAgent(t, "#!/bin/sh\necho customizing\nexit 0\n")
	job := waitJob(t, app, CreateRequest{Template: "follow-up-nudger", Agent: "claude", AutoInstall: true})
	if job.Status != createFailed || !strings.Contains(job.Error, "starter") {
		t.Fatalf("%+v", job)
	}
	if !strings.Contains(job.Log, "customizing") {
		t.Fatalf("%+v", job)
	}
	if app.store.Snapshot().Plugins["follow-up-nudger"] != nil {
		t.Fatal("customize installed without a change")
	}
}

func TestRetryFeedsFailedChecks(t *testing.T) {
	app := openWithAgent(t, "#!/bin/sh\ncat > \"$PWD/prompt.txt\"\necho pass\nexit 0\n")
	job := waitJob(t, app, CreateRequest{Description: "Watch a folder of notes.", Agent: "claude"})
	if job.Status != createFailed {
		t.Fatalf("%+v", job)
	}
	again, err := app.RetryCreate(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	again = waitExisting(t, app, again.ID)
	raw, err := os.ReadFile(filepath.Join(app.DataDir, "drafts", filepath.Base(job.Path), "prompt.txt"))
	if err != nil {
		// The folder may still be the draft path recorded on the first view.
		raw, err = os.ReadFile(filepath.Join(job.Path, "prompt.txt"))
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "starter") && !strings.Contains(string(raw), "DAP_SCAFFOLD_STARTER") {
		t.Fatalf("retry prompt %s\njob %+v", raw, again)
	}
}

func TestEditStaysPendingWhenAutoInstall(t *testing.T) {
	app := openWithAgent(t, editAgentScript)
	first := waitJob(t, app, CreateRequest{Template: "daily-digest", AutoInstall: true})
	if first.Status != createInstalled {
		t.Fatalf("%+v", first)
	}
	second := waitJob(t, app, CreateRequest{
		Plugin:      "daily-digest",
		Description: "Mention the weekday in the morning card.",
		Agent:       "claude",
		AutoInstall: true,
	})
	if second.Status != createReady || second.Card == nil {
		t.Fatalf("%+v", second)
	}
	if second.Card.Status != model.StatusPending || second.Card.Actor == model.ActorUser {
		t.Fatalf("%+v", second.Card)
	}
	if second.Card.Title != "Update daily-digest" {
		t.Fatalf("%s", second.Card.Title)
	}
}

func TestNameComesFromManifest(t *testing.T) {
	app := openWithAgent(t, renameAgentScript)
	job := waitJob(t, app, CreateRequest{
		Description: "Watch a folder and tell me when a file changes every hour.",
		Agent:       "claude",
	})
	if job.Status != createReady || job.Card == nil || job.Card.Plugin != "folder-watch" {
		t.Fatalf("%+v", job)
	}
	if _, err := os.Stat(job.Path); err != nil || filepath.Base(job.Path) != "folder-watch" {
		t.Fatalf("path %s %v", job.Path, err)
	}
}

func TestCreateRejectsOutsideName(t *testing.T) {
	app, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, err := app.CreatePlugin(CreateRequest{Plugin: "missing", Description: "edit", Agent: "claude"}); err == nil {
		t.Fatal("expected missing plugin")
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

func waitJob(t *testing.T, app *App, req CreateRequest) CreateView {
	t.Helper()
	started, err := app.CreatePlugin(req)
	if err != nil {
		t.Fatal(err)
	}
	return waitExisting(t, app, started.ID)
}

func waitExisting(t *testing.T, app *App, id string) CreateView {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		job, err := app.CreateJob(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != createRunning {
			return job
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("create job did not finish")
	return CreateView{}
}

func ledgerKinds(t *testing.T, app *App) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(app.DataDir, "ledger.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var row struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal([]byte(line), &row); err == nil {
			out[row.Kind] = true
		}
	}
	return out
}

const vicPrompt = "Triage my inbox every hour. Label the new threads and never send."

const writeAgentScript = `#!/bin/sh
cat >/dev/null
python3 - <<'PY'
from pathlib import Path
import os, time
src = Path("src/index.ts")
text = src.read_text().replace("DAP_SCAFFOLD_STARTER", "inbox triage")
src.write_text(text)
Path("plugin.yaml").write_text("""name: inbox-triage
version: 0.1.0
min_dap_version: 0.1.0
type: watcher
interface: dap.plugin.v1
description: Triage the inbox and never send.
runtime: node
entrypoint: dist/index.js
permissions:
  - name: mail.read
    mode: allowed
    description: Read threads
schedule:
  every: 1h
""")
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
    else if (method === "propose") reply({ cards: [] });
    else if (method === "act") reply({ status: "done", summary: "Recorded." });
    else if (method === "report") reply({ metrics: [{ key: "runs", label: "Runs", value: 1 }] });
    else if (method === "shutdown") reply({ ok: true });
    else reply(null, "unknown method " + method);
  }
});
''')
future = time.time() + 5
os.utime("dist/index.js", (future, future))
PY
echo wrote
`

const breakAgentScript = `#!/bin/sh
python3 - <<'PY'
from pathlib import Path
import os, time
src = Path("src/index.ts")
src.write_text(src.read_text().replace("DAP_SCAFFOLD_STARTER", "broken"))
Path("plugin.yaml").write_text(Path("plugin.yaml").read_text().replace(
    "Starter plugin. Replace this manifest with the plugin you were asked to build.",
    "A folder watch that does not build.",
))
Path("dist").mkdir(exist_ok=True)
Path("dist/index.js").write_text("this is not javascript\n")
future = time.time() + 5
os.utime("dist/index.js", (future, future))
PY
echo broke
`

const renameAgentScript = `#!/bin/sh
python3 - <<'PY'
from pathlib import Path
text = Path("plugin.yaml").read_text()
text = text.replace(text.splitlines()[0], "name: folder-watch", 1)
text = text.replace(
    "Starter plugin. Replace this manifest with the plugin you were asked to build.",
    "Watch a folder and report new files.",
)
Path("plugin.yaml").write_text(text)
src = Path("src/index.ts")
src.write_text(src.read_text().replace("DAP_SCAFFOLD_STARTER", "watching files"))
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
    if (req.method === "initialize") reply({ ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "ready" } });
    else if (req.method === "watch") reply({ observations: [], tile: { title: "folder-watch", status: "ok", lines: [] } });
    else if (req.method === "propose") reply({ cards: [] });
    else if (req.method === "act") reply({ status: "done", summary: "Recorded." });
    else if (req.method === "report") reply({ metrics: [] });
    else if (req.method === "shutdown") reply({ ok: true });
    else reply(null, "unknown method " + req.method);
  }
});
''')
import os, time
future = time.time() + 5
os.utime("dist/index.js", (future, future))
PY
echo renamed
`

const editAgentScript = `#!/bin/sh
python3 - <<'PY'
from pathlib import Path
p = Path("plugin.yaml")
p.write_text(p.read_text().replace("Nothing is sent.", "Nothing is sent. Edited.", 1))
PY
echo edited
`
