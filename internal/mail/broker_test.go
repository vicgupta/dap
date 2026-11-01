package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/secrets"
)

func TestFakeSendUndoAndNoToken(t *testing.T) {
	dir := t.TempDir()
	keeper, err := secrets.NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(dir, keeper)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := b.Threads(context.Background(), "fake")
	if err != nil || len(empty) != 0 {
		t.Fatal("disconnected demo should be empty")
	}
	if err := b.Connect(context.Background(), "fake"); err != nil {
		t.Fatal(err)
	}
	threads, err := b.Threads(context.Background(), "fake")
	if err != nil || len(threads) < 4 {
		t.Fatalf("%d %v", len(threads), err)
	}
	res, err := b.Send(context.Background(), SendRequest{Provider: "fake", ThreadID: "dana", To: "dana.lee@northwind.co", Subject: "Q4 proposal", Body: "Hi Dana"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := keeper.Get(secretService, "fake")
	if err == nil && strings.Contains(raw, "access_token") {
		t.Fatal("demo mailbox must not store a token")
	}
	st := b.Status("fake")
	if !st.Connected || st.Account == "" || strings.Contains(st.RefreshDetail, "ya29") {
		t.Fatalf("%+v", st)
	}
	if err := b.Undo("fake", res.MessageID); err != nil {
		t.Fatal(err)
	}
}

func TestIncrementalSendScope(t *testing.T) {
	var mu sync.Mutex
	var authURLs []string
	var n int
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		cur := n
		mu.Unlock()
		scope := "https://www.googleapis.com/auth/gmail.readonly https://www.googleapis.com/auth/gmail.compose"
		if cur > 1 {
			scope += " https://www.googleapis.com/auth/gmail.send"
		}
		_, _ = w.Write([]byte(`{"access_token":"at-` + itoa(cur) + `","refresh_token":"rt","expires_in":3600,"scope":"` + scope + `"}`))
	})
	mux.HandleFunc("/gmail/v1/users/me/profile", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("profile auth")
		}
		_, _ = w.Write([]byte(`{"emailAddress":"vic@example.com"}`))
	})
	mux.HandleFunc("/gmail/v1/users/me/messages/send", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"m","threadId":"t"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	keeper, _ := secrets.NewFile(dir)
	b, err := New(dir, keeper)
	if err != nil {
		t.Fatal(err)
	}
	b.GoogleAuth = srv.URL + "/auth"
	b.GoogleToken = srv.URL + "/token"
	b.GmailAPI = srv.URL
	b.SetOpener(func(u string) error {
		mu.Lock()
		authURLs = append(authURLs, u)
		mu.Unlock()
		parsed, err := url.Parse(u)
		if err != nil {
			return err
		}
		q := parsed.Query()
		cb, _ := url.Parse(q.Get("redirect_uri"))
		cb.RawQuery = url.Values{"code": {"c"}, "state": {q.Get("state")}}.Encode()
		resp, err := http.Get(cb.String())
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	})
	if err := b.Setup("gmail", "installed-client", false); err != nil {
		t.Fatal(err)
	}
	if err := b.Connect(context.Background(), "gmail"); err != nil {
		t.Fatal(err)
	}
	st := b.Status("gmail")
	if st.SendGranted || st.Account != "vic@example.com" {
		t.Fatalf("after connect %+v", st)
	}
	if strings.Contains(authURLs[0], "gmail.send") {
		t.Fatalf("first consent included send: %s", authURLs[0])
	}
	_, err = b.Send(context.Background(), SendRequest{Provider: "gmail", ThreadID: "t", To: "a@b.co", Subject: "Hi", Body: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(authURLs) < 2 || !strings.Contains(authURLs[1], "gmail.send") {
		t.Fatalf("incremental url missing send: %v", authURLs)
	}
	if !b.Status("gmail").SendGranted {
		t.Fatal("send scope should be stored after the incremental consent")
	}
	// The stored secret must not be readable as a plugin secret.
	if _, err := keeper.Get("dap.plugin.inbox-followups", "gmail"); err == nil {
		t.Fatal("plugin namespace must not see mail tokens")
	}
}

func itoa(n int) string {
	return string(rune('0' + n))
}

func TestCheckReportsRefresh(t *testing.T) {
	dir := t.TempDir()
	keeper, _ := secrets.NewFile(dir)
	b, _ := New(dir, keeper)
	st := b.Check(context.Background(), "gmail")
	if st.Connected || st.RefreshDetail != "not connected" {
		t.Fatalf("%+v", st)
	}
	_ = time.Second
}
