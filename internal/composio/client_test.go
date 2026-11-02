package composio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientRedactsKey(t *testing.T) {
	const key = "ak_test_secret_value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != key {
			t.Errorf("missing key header")
		}
		http.Error(w, "bad "+key, http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, Key: key}
	_, err := c.EnsureAuthConfig(context.Background(), "gmail")
	if err == nil || strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatal(err)
	}
}

func TestLinkAndExecute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v3.1/auth_configs" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"items":[{"id":"ac_1","is_composio_managed":true}]}`)
		case r.URL.Path == "/api/v3.1/connected_accounts/link":
			_, _ = io.WriteString(w, `{"redirect_url":"https://connect.example/start","connected_account_id":"ca_1"}`)
		case r.URL.Path == "/api/v3.1/connected_accounts/ca_1":
			_, _ = io.WriteString(w, `{"id":"ca_1","status":"ACTIVE"}`)
		case strings.HasPrefix(r.URL.Path, "/api/v3.1/tools/execute/"):
			_, _ = io.WriteString(w, `{"successful":true,"data":{"emailAddress":"vic@example.com"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, Key: "ak_test"}
	id, err := c.EnsureAuthConfig(context.Background(), "gmail")
	if err != nil || id != "ac_1" {
		t.Fatal(err, id)
	}
	link, err := c.LinkAccount(context.Background(), id)
	if err != nil || link.AccountID != "ca_1" || link.RedirectURL == "" {
		t.Fatal(err, link)
	}
	raw, err := c.Execute(context.Background(), "GMAIL_GET_PROFILE", link.AccountID, map[string]any{"user_id": "me"})
	if err != nil || Email(raw) != "vic@example.com" {
		t.Fatal(err, string(raw))
	}
}
