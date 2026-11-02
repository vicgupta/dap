package composio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientHidesInvalidKey(t *testing.T) {
	const key = "ak_test_secret_value"
	const body = `{"error":{"message":"Invalid API key: ck_**oG8d","code":801,"slug":"APIKey_InvalidAPIKey","status":401,"request_id":"req_abc123"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != key {
			t.Errorf("missing key header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, Key: key}
	err := c.Probe(context.Background())
	if err == nil {
		t.Fatal("expected rejection")
	}
	msg, rid := Public(err)
	if strings.Contains(msg, key) || strings.Contains(msg, "ck_") || strings.Contains(msg, "oG8d") || strings.Contains(msg, "**") || strings.Contains(msg, "Invalid API key") {
		t.Fatalf("leaked key material: %s", msg)
	}
	if !strings.Contains(msg, "rejected this key") || rid != "req_abc123" {
		t.Fatalf("msg=%s rid=%s", msg, rid)
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
