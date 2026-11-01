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

func TestLoopbackOnly(t *testing.T) {
	if got := restrictLoopback("0.0.0.0:7391"); got != "127.0.0.1:7391" {
		t.Fatal(got)
	}
	if got := restrictLoopback("127.0.0.1:9"); got != "127.0.0.1:9" {
		t.Fatal(got)
	}
}
