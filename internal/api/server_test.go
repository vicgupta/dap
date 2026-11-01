package api

import (
	"net/http"
	"net/http/httptest"
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
}

func TestThemeSettingsRoundTrip(t *testing.T) {
	app, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	s := New(app, "127.0.0.1:0")
	h := s.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rr.Body.String()
	if !strings.Contains(body, `data-theme="system"`) || !strings.Contains(body, `name="dap-theme" content="system"`) {
		t.Fatalf("index theme: %s", body)
	}
	if !strings.Contains(body, "/theme-boot.js") || !strings.Contains(body, "/theme.css") {
		t.Fatalf("index missing theme assets: %s", body)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"theme":"dark"}`))
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"theme":"dark"`) {
		t.Fatalf("put %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rr.Body.String(), `data-theme="dark"`) {
		t.Fatalf("index after save: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"theme":"sepia"}`))
	req.Header.Set("Authorization", "Bearer "+app.AdminToken())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid theme %d", rr.Code)
	}
	if app.Theme() != "dark" {
		t.Fatal(app.Theme())
	}
}

func TestThemeQueryAndInject(t *testing.T) {
	if got := withThemeQuery("http://127.0.0.1:9/", "dark"); got != "http://127.0.0.1:9/?theme=dark" {
		t.Fatal(got)
	}
	doc := injectTheme([]byte(`<html lang="en" data-theme="system"><head>`), "light")
	if string(doc) != `<html lang="en" data-theme="light"><head>` {
		t.Fatal(string(doc))
	}
	doc = injectTheme([]byte(`<html lang="en"><head>`), "dark")
	if !strings.Contains(string(doc), `data-theme="dark"`) {
		t.Fatal(string(doc))
	}
	req := httptest.NewRequest(http.MethodGet, "/plugins/workflow/?theme=light", nil)
	if got := chosenTheme(req, "dark"); got != "light" {
		t.Fatal(got)
	}
	req = httptest.NewRequest(http.MethodGet, "/plugins/workflow/?theme=nope", nil)
	if got := chosenTheme(req, "dark"); got != "dark" {
		t.Fatal(got)
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
