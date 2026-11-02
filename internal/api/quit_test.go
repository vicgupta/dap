package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/core"
)

func TestQuitRejectsPluginTokenAndAcceptsAdmin(t *testing.T) {
	app, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	dir := writeQuitPlugin(t)
	if _, err := app.Install(dir, true); err != nil {
		t.Fatal(err)
	}
	if err := app.StartPlugins(); err != nil {
		t.Fatal(err)
	}
	token := app.Token("quit-probe")
	if token == "" || token == app.AdminToken() {
		t.Fatal("plugin token missing")
	}
	s := New(app, "127.0.0.1:0")
	called := make(chan struct{}, 1)
	s.OnQuit(func() { called <- struct{}{} })
	h := s.Handler()

	rr := postQuit(t, h, token, "127.0.0.1:9", map[string]string{"X-DAP-Request": "quit"})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("plugin token status %d %s", rr.Code, rr.Body.String())
	}

	rr = postQuit(t, h, app.AdminToken(), "127.0.0.1:9", nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("missing header %d %s", rr.Code, rr.Body.String())
	}
	rr = postQuit(t, h, app.AdminToken(), "127.0.0.1:9", map[string]string{
		"X-DAP-Request": "quit",
		"Origin":        "https://evil.example",
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cross-site origin %d %s", rr.Code, rr.Body.String())
	}
	rr = postQuit(t, h, app.AdminToken(), "127.0.0.1:9", map[string]string{
		"X-DAP-Request":  "quit",
		"Origin":         "http://127.0.0.1:9",
		"Sec-Fetch-Site": "cross-site",
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("sec-fetch %d %s", rr.Code, rr.Body.String())
	}
	rr = postQuit(t, h, app.AdminToken(), "8.8.8.8:9", map[string]string{"X-DAP-Request": "quit"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("remote peer %d %s", rr.Code, rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/quit", nil)
	req.RemoteAddr = "127.0.0.1:9"
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET quit %d", rr.Code)
	}

	ph := app.PluginHandler()
	preq := httptest.NewRequest(http.MethodPost, "/v1/quit-probe/quit", strings.NewReader("{}"))
	preq.Header.Set("Authorization", "Bearer "+token)
	preq.Header.Set("Content-Type", "application/json")
	preq.Header.Set("X-DAP-Request", "quit")
	rr = httptest.NewRecorder()
	ph.ServeHTTP(rr, preq)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("plugin route %d %s", rr.Code, rr.Body.String())
	}

	stReq := httptest.NewRequest(http.MethodGet, "/api/quit-status", nil)
	stReq.RemoteAddr = "127.0.0.1:9"
	stReq.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, stReq)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status plugin token %d", rr.Code)
	}

	rr = postQuit(t, h, app.AdminToken(), "127.0.0.1:9", map[string]string{
		"X-DAP-Request": "quit",
		"Origin":        "http://127.0.0.1:7391",
	})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("admin quit %d %s", rr.Code, rr.Body.String())
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("quit did not signal shutdown")
	}
}

func postQuit(t *testing.T, h http.Handler, token, remote string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/quit", strings.NewReader("{}"))
	req.RemoteAddr = remote
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func writeQuitPlugin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `import json, sys

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
    if method == "shutdown":
        return {"ok": True}
    if method == "watch":
        return {"observations": [], "tile": {"title": "q", "status": "ok", "lines": []}}
    if method == "propose":
        return {"cards": []}
    if method == "act":
        return {"status": "done", "summary": "ok"}
    if method == "report":
        return {"metrics": []}
    raise RuntimeError("unknown")

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
	manifest := `name: quit-probe
version: 0.1.0
min_dap_version: 0.1.0
type: watcher
description: Probe plugin for quit auth.
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
