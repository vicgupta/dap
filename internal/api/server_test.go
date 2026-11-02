package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vicgupta/dap/internal/core"
)

func TestAdminRequiresTokenAndServesUI(t *testing.T) {
	app, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	s := New(app, "127.0.0.1:0")
	h := s.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/home", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rr.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/home", nil)
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("home %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), app.AdminToken()) {
		t.Fatalf("index missing token: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal(rr.Header().Get("Content-Security-Policy"))
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-density="comfortable"`) || !strings.Contains(body, `name="dap-layout"`) {
		t.Fatalf("index missing layout boot: %s", body)
	}
	if !strings.Contains(body, `/layout-boot.js`) {
		t.Fatal("layout boot script missing")
	}
}

func TestLayoutSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{\n  \"addr\": \"127.0.0.1:7391\",\n  \"theme\": \"dark\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := core.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	s := New(app, "127.0.0.1:0")
	h := s.Handler()

	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"layout":{"home":{"view":"board","density":"compact"},"inbox":{"view":"split"}}}`))
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("put %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"view":"board"`) || !strings.Contains(rr.Body.String(), `"view":"split"`) {
		t.Fatalf("get %d %s", rr.Code, rr.Body.String())
	}

	saved, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), `"theme": "dark"`) {
		t.Fatalf("theme key was dropped: %s", saved)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"layout":{"home":{"view":"split"}}}`))
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad view %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"theme":"light"}`))
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"view":"board"`) {
		t.Fatalf("theme-only body should leave layout: %d %s", rr.Code, rr.Body.String())
	}
}

func TestLoopbackOnly(t *testing.T) {
	if got := restrictLoopback("0.0.0.0:7391"); got != "127.0.0.1:7391" {
		t.Fatal(got)
	}
	if got := restrictLoopback("127.0.0.1:9"); got != "127.0.0.1:9" {
		t.Fatal(got)
	}
}
