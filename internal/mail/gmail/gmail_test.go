package gmail

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/mailbox"
)

func TestListAndSend(t *testing.T) {
	var sentBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.URL.Path == "/gmail/v1/users/me/profile":
			_, _ = w.Write([]byte(`{"emailAddress":"vic@example.com"}`))
		case r.URL.Path == "/gmail/v1/users/me/threads" && r.URL.Query().Get("q") != "":
			_, _ = w.Write([]byte(`{"threads":[{"id":"t1"}]}`))
		case r.URL.Path == "/gmail/v1/users/me/threads/t1":
			_, _ = w.Write([]byte(`{
				"id":"t1",
				"messages":[
					{"id":"m1","snippet":"Can you send the revised pricing by Friday?","internalDate":"1730000000000",
					 "payload":{"headers":[
						{"name":"From","value":"Dana Lee <dana.lee@northwind.co>"},
						{"name":"To","value":"Vic <vic@example.com>"},
						{"name":"Subject","value":"Q4 proposal"}
					 ]}},
					{"id":"m2","snippet":"I'll send it by Friday.","internalDate":"1730100000000",
					 "payload":{"headers":[
						{"name":"From","value":"Vic <vic@example.com>"},
						{"name":"To","value":"Dana Lee <dana.lee@northwind.co>"},
						{"name":"Subject","value":"Re: Q4 proposal"}
					 ]}}
				]
			}`))
		case r.URL.Path == "/gmail/v1/users/me/messages/send":
			b, _ := io.ReadAll(r.Body)
			sentBody = string(b)
			_, _ = w.Write([]byte(`{"id":"sent1","threadId":"t1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &Client{API: srv.URL}
	email, err := c.Profile(context.Background(), "tok")
	if err != nil || email != "vic@example.com" {
		t.Fatalf("profile %q %v", email, err)
	}
	threads, err := c.ListThreads(context.Background(), "tok", email)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || !threads[0].LastFromMe || threads[0].WithEmail != "dana.lee@northwind.co" {
		t.Fatalf("%+v", threads)
	}
	res, err := c.Send(context.Background(), "tok", email, mailbox.SendRequest{
		ThreadID: "t1", To: "dana.lee@northwind.co", Subject: "Q4 proposal", Body: "Hi Dana",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "sent1" || !strings.Contains(sentBody, "threadId") {
		t.Fatalf("%+v %s", res, sentBody)
	}
	if strings.Contains(sentBody, "tok") {
		t.Fatal("token leaked into body")
	}
	_ = time.Now()
}

func TestListThreadsQueryUsesWindow(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/gmail/v1/users/me/threads":
			query = r.URL.Query().Get("q")
			_, _ = w.Write([]byte(`{"threads":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &Client{API: srv.URL}
	if _, err := c.ListThreadsQuery(context.Background(), "tok", "vic@example.com", "newer_than:1d"); err != nil {
		t.Fatal(err)
	}
	if query != "newer_than:1d" {
		t.Fatalf("q %q", query)
	}
}

func TestRetryAfterLongThrottle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := &Client{API: srv.URL}
	_, err := c.Profile(context.Background(), "tok")
	if err == nil || !strings.Contains(err.Error(), "wait") {
		t.Fatal(err)
	}
}
