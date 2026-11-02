package composio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vicgupta/dap/internal/secrets"
)

func TestSaveKeyProbesAndRefusesConsumerKey(t *testing.T) {
	const project = "ak_project_key_value"
	var probed int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3.1/auth_configs" || r.URL.Query().Get("limit") != "1" {
			http.NotFound(w, r)
			return
		}
		probed++
		w.Header().Set("Content-Type", "application/json")
		got := r.Header.Get("x-api-key")
		if got != project && got != "custom-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"Invalid API key: ck_**oG8d","slug":"APIKey_InvalidAPIKey","request_id":"req_deadbeef"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	keeper, err := secrets.NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := New(dir, keeper, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetBase(srv.URL)

	_, err = svc.SaveKey(context.Background(), "ck_consumer_secret")
	if err == nil || !strings.Contains(err.Error(), "consumer key") || leaked(err.Error()) {
		t.Fatal(err)
	}
	if svc.HasKey() {
		t.Fatal("consumer key was stored")
	}
	if probed != 0 {
		t.Fatal("consumer key should not be sent")
	}

	_, err = svc.SaveKey(context.Background(), "sk_other_value")
	msg, rid := Public(err)
	if err == nil || !strings.Contains(msg, "rejected this key") || rid != "req_deadbeef" || leaked(msg) {
		t.Fatalf("%v %s %s", err, msg, rid)
	}
	if svc.HasKey() {
		t.Fatal("rejected key was stored")
	}

	check, err := svc.SaveKey(context.Background(), project)
	if err != nil || !check.Verified || check.Warning != "" {
		t.Fatalf("%v %+v", err, check)
	}
	if !svc.HasKey() {
		t.Fatal("verified key should be stored")
	}

	warn, err := svc.SaveKey(context.Background(), "custom-token")
	if err != nil || !warn.Verified || warn.Warning == "" || leaked(warn.Warning) {
		t.Fatalf("soft warning %+v %v", warn, err)
	}
}

func leaked(s string) bool {
	return strings.Contains(s, "ck_") || strings.Contains(s, "oG8d") || strings.Contains(s, "**") || strings.Contains(s, "Invalid API key")
}
