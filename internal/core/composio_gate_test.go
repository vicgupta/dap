package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vicgupta/dap/internal/mail"
)

func TestComposioSendStaysBehindACard(t *testing.T) {
	var mu sync.Mutex
	var sends int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v3.1/auth_configs" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"items":[{"id":"ac","is_composio_managed":true}]}`)
		case r.URL.Path == "/api/v3.1/connected_accounts/link":
			_, _ = io.WriteString(w, `{"redirect_url":"http://`+r.Host+`/sign-in","connected_account_id":"ca"}`)
		case r.URL.Path == "/sign-in":
			_, _ = io.WriteString(w, "ok")
		case r.URL.Path == "/api/v3.1/connected_accounts/ca":
			_, _ = io.WriteString(w, `{"id":"ca","status":"ACTIVE"}`)
		case strings.HasPrefix(r.URL.Path, "/api/v3.1/tools/execute/"):
			tool := strings.TrimPrefix(r.URL.Path, "/api/v3.1/tools/execute/")
			if tool == "GMAIL_SEND_EMAIL" || tool == "GMAIL_REPLY_TO_THREAD" {
				mu.Lock()
				sends++
				mu.Unlock()
			}
			_, _ = io.WriteString(w, `{"successful":true,"data":{"emailAddress":"vic@example.com","messages":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	app := openTest(t)
	app.mail.ComposioAPI = srv.URL
	app.mail.SetOpener(func(u string) error {
		resp, err := http.Get(u)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	})
	key := "ak_gate_test"
	if err := app.ConfigureConnection("composio", nil, nil, nil, &key); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Install(filepath.Join(repoRoot(t), "plugins", "mail-gmail"), true); err != nil {
		t.Fatal(err)
	}
	if err := app.MailConnect(context.Background(), "gmail"); err != nil {
		t.Fatal(err)
	}
	_, err := app.MailSend(context.Background(), "mail-gmail", mail.SendRequest{
		Provider: "gmail", ThreadID: "t1", To: "a@b.co", Subject: "Hi", Body: "Hello",
	})
	if err == nil {
		t.Fatal("send without a card should be refused")
	}
	_, err = app.ConnectExecute(context.Background(), "mail-gmail", "gmail", "GMAIL_SEND_EMAIL", map[string]any{
		"recipient_email": "a@b.co", "subject": "Hi", "body": "Hello",
	}, "")
	if err == nil {
		t.Fatal("generic send without a card should be refused")
	}
	if _, err := app.ConnectExecute(context.Background(), "mail-gmail", "gmail", "GMAIL_FETCH_EMAILS", map[string]any{"max_results": 1}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ConnectExecute(context.Background(), "mail-gmail", "gmail", "GMAIL_DELETE_MESSAGE", nil, ""); err == nil {
		t.Fatal("undeclared tool should be refused")
	}
	rows, err := app.Activity(20)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if strings.Contains(row.Message, key) {
			t.Fatalf("ledger leaked a secret: %+v", row)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if sends != 0 {
		t.Fatalf("send tool ran %d times without a card", sends)
	}
}
