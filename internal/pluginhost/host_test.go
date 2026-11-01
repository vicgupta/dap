package pluginhost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLifecycleOrder(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.txt")
	script := `import json, sys
path = sys.argv[1]
def reply(i, result):
    sys.stdout.write(json.dumps({"jsonrpc":"2.0","id":i,"result":result}) + "\n")
    sys.stdout.flush()
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    req = json.loads(line)
    with open(path, "a", encoding="utf-8") as f:
        f.write(req["method"] + "\n")
    method = req["method"]
    if method == "initialize":
        reply(req["id"], {"ready": True, "interface": "dap.plugin.v1", "health": {"status": "ok", "message": "ready"}})
    elif method == "watch":
        reply(req["id"], {"observations": [{"id": "o1", "at": "2026-11-01T00:00:00Z", "summary": "tick"}], "tile": {"title": "t", "status": "ok", "lines": []}})
    elif method == "propose":
        reply(req["id"], {"cards": [{"title": "Say hello", "permission": "demo.greet", "effects": []}]})
    elif method == "act":
        reply(req["id"], {"status": "done", "summary": "hello", "evidence": [{"label": "note", "ref": "tick"}]})
    elif method == "report":
        reply(req["id"], {"metrics": [{"key": "n", "label": "N", "value": 1}]})
    else:
        sys.stdout.write(json.dumps({"jsonrpc":"2.0","id":req["id"],"error":{"code":-32601,"message":"unknown method"}}) + "\n")
        sys.stdout.flush()
`
	path := filepath.Join(dir, "plugin.py")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h := New()
	defer h.StopAll()
	py, err := lookPython()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(Options{Name: "echo", Dir: dir, Argv: []string{py, path, logPath}, Env: PluginEnv()}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var init InitResult
	if err := h.Call(ctx, "echo", MethodInitialize, InitParams{Interface: "dap.plugin.v1"}, &init); err != nil {
		t.Fatal(err)
	}
	if !init.Ready || init.Health.Status != "ok" {
		t.Fatalf("%+v", init)
	}
	var watch WatchResult
	if err := h.Call(ctx, "echo", MethodWatch, WatchParams{}, &watch); err != nil {
		t.Fatal(err)
	}
	if len(watch.Observations) != 1 {
		t.Fatalf("%+v", watch)
	}
	var prop ProposeResult
	if err := h.Call(ctx, "echo", MethodPropose, ProposeParams{Observations: watch.Observations}, &prop); err != nil {
		t.Fatal(err)
	}
	if len(prop.Cards) != 1 {
		t.Fatal(prop)
	}
	var act modelAct
	if err := h.Call(ctx, "echo", MethodAct, map[string]any{"card": map[string]any{"id": "c1"}}, &act); err != nil {
		t.Fatal(err)
	}
	if act.Summary != "hello" {
		t.Fatal(act)
	}
	var report ReportResult
	if err := h.Call(ctx, "echo", MethodReport, ReportParams{Period: "week"}, &report); err != nil {
		t.Fatal(err)
	}
	h.Stop("echo")
	if h.Running("echo") {
		t.Fatal("process should have stopped")
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := "initialize\nwatch\npropose\nact\nreport\n"
	if got != want {
		t.Fatalf("order:\n%s", got)
	}
}

type modelAct struct {
	Summary string `json:"summary"`
}

func lookPython() (string, error) {
	return "python3", nil
}
