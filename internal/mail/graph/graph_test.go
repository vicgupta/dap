package graph

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vicgupta/dap/internal/mail/httpx"
	"github.com/vicgupta/dap/internal/mailbox"
)

func TestDeltaConversationReplyAndRetry(t *testing.T) {
	var sentHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing bearer")
		}
		switch {
		case r.URL.Path == "/v1.0/me":
			_, _ = w.Write([]byte(`{"mail":"vic@outlook.com"}`))
		case strings.Contains(r.URL.Path, "/inbox/messages/delta") && r.URL.Query().Get("token") == "":
			_, _ = w.Write([]byte(`{
				"value":[{
					"id":"m1","conversationId":"conv-9","subject":"Thursday sync",
					"bodyPreview":"Can we do Thursday at 2?","receivedDateTime":"2026-10-31T15:00:00Z",
					"from":{"emailAddress":{"name":"Priya Shah","address":"priya@northwind.co"}}
				}],
				"@odata.deltaLink":"http://` + r.Host + `/v1.0/me/mailFolders/inbox/messages/delta?token=abc"
			}`))
		case strings.Contains(r.URL.RawQuery, "token=abc"):
			_, _ = w.Write([]byte(`{"value":[],"@odata.deltaLink":"http://` + r.Host + `/v1.0/me/mailFolders/inbox/messages/delta?token=abc"}`))
		case strings.Contains(r.URL.Path, "/sentitems/messages/delta"):
			n := atomic.AddInt32(&sentHits, 1)
			if n == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_, _ = w.Write([]byte(`{"value":[],"@odata.deltaLink":"http://` + r.Host + `/v1.0/me/mailFolders/sentitems/messages/delta?token=s"}`))
		case strings.HasSuffix(r.URL.Path, "/reply"):
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), "Thursday") {
				t.Errorf("body %s", b)
			}
			if !strings.Contains(r.URL.Path, "m1") {
				t.Errorf("reply path %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var saw httpx.Throttle
	c := &Client{API: srv.URL + "/v1.0", OnThrottle: func(th httpx.Throttle) { saw = th }}
	email, err := c.Me(context.Background(), "tok")
	if err != nil || email != "vic@outlook.com" {
		t.Fatalf("me %q %v", email, err)
	}
	threads, cur, err := c.ListThreads(context.Background(), "tok", email, DeltaCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].ID != "conv-9" || threads[0].MessageID != "m1" {
		t.Fatalf("%+v", threads)
	}
	if cur.Inbox == "" || !strings.Contains(cur.Inbox, "token=abc") {
		t.Fatalf("delta cursor %q", cur.Inbox)
	}
	if saw.Status != http.StatusTooManyRequests {
		t.Fatalf("throttle not recorded: %+v", saw)
	}
	// Second poll uses the delta link and does not return the old message again.
	again, _, err := c.ListThreads(context.Background(), "tok", email, cur)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("delta should be empty, got %+v", again)
	}
	_, err = c.Send(context.Background(), "tok", mailbox.SendRequest{
		ThreadID: "conv-9", MessageID: "m1", To: "priya@northwind.co", Subject: "Thursday sync", Body: "Thursday 2pm works.",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLongRetryAfterStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "45")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := &Client{API: srv.URL + "/v1.0"}
	_, err := c.Me(context.Background(), "tok")
	if err == nil || !strings.Contains(err.Error(), "wait") {
		t.Fatal(err)
	}
}
