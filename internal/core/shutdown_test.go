package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/lockfile"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/runinfo"
)

func TestShutdownDrainsActAndReleases(t *testing.T) {
	app := openTest(t)
	if st := app.QuitStatus(); st.Running || st.PluginsRunning != 0 {
		t.Fatalf("idle status %+v", st)
	}
	dir := writeSlowPlugin(t)
	if _, err := app.Install(dir, true); err != nil {
		t.Fatal(err)
	}
	if err := app.StartPlugins(); err != nil {
		t.Fatal(err)
	}
	if !app.host.Running("slow-act") {
		t.Fatalf("plugin did not start: %s", app.host.Stderr("slow-act"))
	}
	if st := app.QuitStatus(); !st.Running || st.PluginsRunning != 1 {
		t.Fatalf("running status %+v", st)
	}
	if err := runinfo.Write(app.DataDir, runinfo.Info{
		PID: os.Getpid(), Addr: "127.0.0.1:9", Token: app.AdminToken(), Started: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	card, err := app.Propose("slow-act", model.CardDraft{
		Title: "Say hello", Permission: "demo.greet", Kind: model.KindAction,
	}, model.ActorPlugin)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := app.Approve(card.ID); done <- err }()
	waitFile(t, filepath.Join(dir, "act-start"))

	st := app.QuitStatus()
	if st.Acting == 0 || st.InflightActions == 0 {
		t.Fatalf("expected in-flight act, %+v", st)
	}
	if err := app.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("act: %v stderr %s", err, app.host.Stderr("slow-act"))
	}
	if app.host.Running("slow-act") {
		t.Fatal("plugin still running")
	}
	for _, name := range []string{"act-done", "shutdown"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(app.DataDir, "ledger.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"kind":"plugin.stop"`) && !strings.Contains(string(body), `"kind": "plugin.stop"`) {
		t.Fatalf("ledger was not flushed with plugin.stop:\n%s", body)
	}
	if _, err := os.Stat(runinfo.Path(app.DataDir)); !os.IsNotExist(err) {
		t.Fatalf("run.json still present: %v", err)
	}
	stateRaw, err := os.ReadFile(filepath.Join(app.DataDir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateRaw, &state); err != nil {
		t.Fatalf("state.json not flushed: %v", err)
	}
	lk, err := lockfile.Acquire(filepath.Join(app.DataDir, "dap.lock"))
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	if err := lk.Release(); err != nil {
		t.Fatal(err)
	}
	if err := app.Shutdown(); err != nil {
		t.Fatal(err)
	}
}

func writeSlowPlugin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `import json, os, sys, time
root = os.path.dirname(os.path.abspath(__file__))

def mark(name):
    with open(os.path.join(root, name), "w", encoding="utf-8") as f:
        f.write("1")

def reply(rid, result=None, error=None):
    msg = {"jsonrpc": "2.0", "id": rid}
    if error is not None:
        msg["error"] = {"code": -32000, "message": str(error)}
    else:
        msg["result"] = result
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()

def handle(method, params):
    if method == "initialize":
        return {"ready": True, "interface": "dap.plugin.v1", "health": {"status": "ok", "message": "ready"}}
    if method == "watch":
        return {"observations": [], "tile": {"title": "Slow", "status": "ok", "lines": []}}
    if method == "propose":
        return {"cards": []}
    if method == "act":
        mark("act-start")
        time.sleep(0.4)
        mark("act-done")
        return {"status": "done", "summary": "slow hello"}
    if method == "report":
        return {"metrics": []}
    if method == "shutdown":
        mark("shutdown")
        return {"ok": True}
    raise RuntimeError("unknown method " + str(method))

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    req = json.loads(line)
    try:
        reply(req.get("id"), result=handle(req.get("method"), req.get("params") or {}))
    except Exception as exc:
        reply(req.get("id"), error=str(exc))
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.py"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `name: slow-act
version: 0.1.0
min_dap_version: 0.1.0
type: watcher
description: Slow act for shutdown tests.
entrypoint: plugin.py
permissions:
  - name: demo.greet
    mode: ask_first
    description: Record a hello
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func waitFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
