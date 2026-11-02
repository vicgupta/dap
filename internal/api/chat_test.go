package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vicgupta/dap/internal/chat"
	"github.com/vicgupta/dap/internal/core"
)

func TestChatAPIAuthAndSettings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{\n  \"addr\": \"127.0.0.1:7391\",\n  \"theme\": \"dark\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := core.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h := New(app, "127.0.0.1:0").Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/chat/providers", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rr.Code)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"chat":{"provider":"claude-code"}}`))
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"claude-code"`) {
		t.Fatalf("put %d %s", rr.Code, rr.Body.String())
	}
	saved, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), `"theme": "dark"`) || !strings.Contains(string(saved), `"claude-code"`) {
		t.Fatalf("settings clobbered:\n%s", saved)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"chat":{"provider":"opencode"}}`))
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("opencode %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/chat/sessions", strings.NewReader(`{"provider":"codex"}`))
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"provider":"codex"`) {
		t.Fatalf("create %d %s", rr.Code, rr.Body.String())
	}
}

func TestChatTurnStreamsNDJSON(t *testing.T) {
	app, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	bin := buildAPIFake(t)
	events := filepath.Join(t.TempDir(), "events.ndjson")
	if err := os.WriteFile(events, []byte("{\"type\":\"stream_event\",\"event\":{\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}}\n{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"Hi\",\"session_id\":\"11111111-1111-4111-8111-111111111111\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.SetChatRunner(&chat.Runner{
		LookPath: func(string) (string, error) { return bin, nil },
		Env:      func() []string { return []string{"DAP_FAKE_EVENTS=" + events} },
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := app.CreateChatSession(chat.ProviderClaude)
	if err != nil {
		t.Fatal(err)
	}
	h := New(app, "127.0.0.1:0").Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/chat/sessions/"+sess.ID+"/turn", strings.NewReader(`{"text":"hello"}`))
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("turn %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "ndjson") {
		t.Fatal(rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(rr.Body.String(), `"delta":"Hi"`) || !strings.Contains(rr.Body.String(), `"type":"done"`) {
		t.Fatalf("body %s", rr.Body.String())
	}
}

var (
	apiFakeOnce sync.Once
	apiFakeBin  string
	apiFakeErr  error
)

func buildAPIFake(t *testing.T) string {
	t.Helper()
	apiFakeOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			apiFakeErr = err
			return
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				apiFakeErr = os.ErrNotExist
				return
			}
			dir = parent
		}
		out, err := os.MkdirTemp("", "dap-fakecli")
		if err != nil {
			apiFakeErr = err
			return
		}
		apiFakeBin = filepath.Join(out, "fakecli")
		cmd := exec.Command("go", "build", "-o", apiFakeBin, "./internal/chat/fakecli")
		cmd.Dir = dir
		msg, err := cmd.CombinedOutput()
		if err != nil {
			apiFakeErr = err
			t.Log(string(msg))
		}
	})
	if apiFakeErr != nil || apiFakeBin == "" {
		t.Fatalf("fake cli: %v", apiFakeErr)
	}
	return apiFakeBin
}
