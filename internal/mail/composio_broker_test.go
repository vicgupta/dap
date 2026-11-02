package mail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/secrets"
)

func TestComposioGmailAndOutlook(t *testing.T) {
	const apiKey = "ak_test_broker_key"
	var mu sync.Mutex
	status := "INITIATED"
	var tools []string
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/api/v3.1/auth_configs" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"items":[{"id":"ac_mail","is_composio_managed":true}]}`)
		case r.URL.Path == "/api/v3.1/connected_accounts/link":
			_, _ = io.WriteString(w, `{"redirect_url":"`+srvURL(r)+`/sign-in","connected_account_id":"ca_mail"}`)
		case r.URL.Path == "/sign-in":
			status = "ACTIVE"
			_, _ = io.WriteString(w, "ok")
		case r.URL.Path == "/api/v3.1/connected_accounts/ca_mail" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"id":"ca_mail","status":"`+status+`"}`)
		case r.URL.Path == "/api/v3.1/connected_accounts/ca_mail" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/api/v3.1/tools/execute/"):
			tool := strings.TrimPrefix(r.URL.Path, "/api/v3.1/tools/execute/")
			rawBody, _ := io.ReadAll(r.Body)
			tools = append(tools, tool)
			bodies = append(bodies, tool+" "+string(rawBody))
			switch tool {
			case "GMAIL_GET_PROFILE":
				_, _ = io.WriteString(w, `{"successful":true,"data":{"emailAddress":"vic@example.com"}}`)
			case "GMAIL_FETCH_EMAILS":
				_, _ = io.WriteString(w, `{"successful":true,"data":{"messages":[{"messageId":"m1","threadId":"t1","subject":"Q4","sender":"Dana Lee <dana@northwind.co>","to":"vic@example.com","messageTimestamp":"2026-10-01T12:00:00Z","preview":"hello","labelIds":["INBOX"]}]}}`)
			case "GMAIL_CREATE_EMAIL_DRAFT":
				_, _ = io.WriteString(w, `{"successful":true,"data":{"id":"r-draft"}}`)
			case "GMAIL_REPLY_TO_THREAD":
				_, _ = io.WriteString(w, `{"successful":true,"data":{"id":"sent-1","threadId":"t1"}}`)
			case "OUTLOOK_GET_PROFILE":
				_, _ = io.WriteString(w, `{"successful":true,"data":{"mail":"vic@outlook.com"}}`)
			case "OUTLOOK_LIST_MESSAGES":
				_, _ = io.WriteString(w, `{"successful":true,"data":{"value":[{"id":"AAMk","conversationId":"conv","subject":"Hello","from":{"emailAddress":{"name":"Dana","address":"dana@northwind.co"}},"receivedDateTime":"2026-10-02T12:00:00Z","bodyPreview":"hi"}]}}`)
			case "OUTLOOK_CREATE_DRAFT_REPLY":
				_, _ = io.WriteString(w, `{"successful":true,"data":{"id":"draft-ol"}}`)
			case "OUTLOOK_REPLY_EMAIL":
				_, _ = io.WriteString(w, `{"successful":true,"data":{"id":"sent-ol"}}`)
			default:
				http.Error(w, "unexpected "+tool, http.StatusBadRequest)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	keeper, err := secrets.NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(dir, keeper)
	if err != nil {
		t.Fatal(err)
	}
	b.ComposioAPI = srv.URL
	b.SetClock(func() time.Time { return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) })
	b.SetOpener(func(u string) error {
		resp, err := http.Get(u)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	})
	if err := b.Configure("composio", Configure{APIKey: strPtr(apiKey)}); err != nil {
		t.Fatal(err)
	}
	if !b.HasComposioKey() {
		t.Fatal("key should be stored")
	}
	if err := b.Connect(context.Background(), "gmail"); err != nil {
		t.Fatal(err)
	}
	st := b.Status("gmail")
	raw, _ := json.Marshal(st)
	if strings.Contains(string(raw), apiKey) || strings.Contains(string(raw), "ca_mail") {
		t.Fatalf("status leaked a secret: %s", raw)
	}
	pub, _ := os.ReadFile(filepath.Join(dir, "mail", "public.json"))
	if strings.Contains(string(pub), apiKey) {
		t.Fatalf("public.json leaked the key: %s", pub)
	}
	secret, err := keeper.Get("dap.composio", "conn.gmail")
	if err != nil || !strings.Contains(secret, "ca_mail") || strings.Contains(secret, "access_token") {
		t.Fatalf("connection secret: %v %s", err, secret)
	}
	key, err := keeper.Get("dap.composio", "api_key")
	if err != nil || key != apiKey {
		t.Fatalf("api key secret: %v %s", err, key)
	}
	if !st.Connected || st.Account != "vic@example.com" || st.Mode != "composio" || st.Via != "composio" {
		t.Fatalf("%+v", st)
	}
	threads, err := b.Threads(context.Background(), "gmail")
	if err != nil || len(threads) != 1 || threads[0].WithEmail != "dana@northwind.co" || threads[0].Provider != "gmail" {
		t.Fatalf("%v %+v", err, threads)
	}
	day, err := b.ThreadsQuery(context.Background(), "gmail", ListOptions{NewerThan: "1d"})
	if err != nil {
		t.Fatal(err)
	}
	if len(day) != 0 {
		t.Fatalf("2026-10-01 thread should fall outside a 1d window from now: %+v", day)
	}
	draft, err := b.Draft(context.Background(), SendRequest{Provider: "gmail", ThreadID: "t1", To: "dana@northwind.co", Body: "Thanks"})
	if err != nil || draft.DraftID != "r-draft" || draft.Local {
		t.Fatalf("%v %+v", err, draft)
	}
	sent, err := b.Send(context.Background(), SendRequest{Provider: "gmail", ThreadID: "t1", To: "dana@northwind.co", Subject: "Q4", Body: "Thanks"})
	if err != nil || sent.MessageID != "sent-1" {
		t.Fatalf("%v %+v", err, sent)
	}

	status = "INITIATED"
	if err := b.Connect(context.Background(), "outlook"); err != nil {
		t.Fatal(err)
	}
	ol := b.Status("graph")
	if ol.Provider != "outlook" || ol.Account != "vic@outlook.com" || !ol.DirectFallback {
		t.Fatalf("%+v", ol)
	}
	othreads, err := b.Threads(context.Background(), "graph")
	if err != nil || len(othreads) != 1 || othreads[0].Provider != "outlook" {
		t.Fatalf("%v %+v", err, othreads)
	}
	_, err = b.ThreadsQuery(context.Background(), "outlook", ListOptions{Since: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	odraft, err := b.Draft(context.Background(), SendRequest{Provider: "graph", MessageID: "AAMk", To: "dana@northwind.co", Body: "Thanks"})
	if err != nil || odraft.DraftID != "draft-ol" {
		t.Fatalf("%v %+v", err, odraft)
	}
	osent, err := b.Send(context.Background(), SendRequest{Provider: "outlook", MessageID: "AAMk", To: "dana@northwind.co", Subject: "Hello", Body: "Thanks"})
	if err != nil || osent.MessageID != "sent-ol" {
		t.Fatalf("%v %+v", err, osent)
	}
	if err := b.Disconnect("gmail"); err != nil {
		t.Fatal(err)
	}
	if _, err := keeper.Get("dap.composio", "conn.gmail"); err == nil {
		t.Fatal("gmail connection id should be gone")
	}
	if kept, err := keeper.Get("dap.composio", "api_key"); err != nil || kept != apiKey {
		t.Fatal("api key should survive disconnect", err)
	}
	mu.Lock()
	joined := strings.Join(tools, ",")
	bodyJoined := strings.Join(bodies, "\n")
	mu.Unlock()
	if !strings.Contains(bodyJoined, "newer_than:30d") || !strings.Contains(bodyJoined, "newer_than:1d") {
		t.Fatalf("gmail query missing from %s", bodyJoined)
	}
	if !strings.Contains(bodyJoined, "receivedDateTime ge 2026-11-01T00:00:00Z") {
		t.Fatalf("outlook filter missing from %s", bodyJoined)
	}
	for _, want := range []string{"GMAIL_FETCH_EMAILS", "GMAIL_CREATE_EMAIL_DRAFT", "GMAIL_REPLY_TO_THREAD", "OUTLOOK_LIST_MESSAGES", "OUTLOOK_CREATE_DRAFT_REPLY", "OUTLOOK_REPLY_EMAIL"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
}

func strPtr(s string) *string { return &s }

func srvURL(r *http.Request) string {
	return "http://" + r.Host
}
