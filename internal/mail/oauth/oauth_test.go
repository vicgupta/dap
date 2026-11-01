package oauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAuthorizePKCEAndIncrementalScopes(t *testing.T) {
	var gotForm url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-1","refresh_token":"rt-1","expires_in":3600,"scope":"read compose"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	base := []string{"read", "compose"}
	var authURL string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tok, err := Authorize(ctx, Config{
		ClientID: "client-123",
		AuthURL:  srv.URL + "/auth",
		TokenURL: srv.URL + "/token",
		Scopes:   base,
		Extra:    url.Values{"access_type": {"offline"}, "prompt": {"consent"}},
		Open: func(u string) error {
			authURL = u
			parsed, err := url.Parse(u)
			if err != nil {
				return err
			}
			q := parsed.Query()
			cb, err := url.Parse(q.Get("redirect_uri"))
			if err != nil {
				return err
			}
			cb.RawQuery = url.Values{"code": {"abc"}, "state": {q.Get("state")}}.Encode()
			resp, err := http.Get(cb.String())
			if err != nil {
				return err
			}
			resp.Body.Close()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "at-1" || tok.RefreshToken != "rt-1" {
		t.Fatalf("token %+v", tok)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Fatalf("pkce missing: %s", authURL)
	}
	if strings.Contains(q.Get("scope"), "send") {
		t.Fatalf("sign-in must not ask for send: %s", q.Get("scope"))
	}
	if !strings.Contains(q.Get("scope"), "read") || !strings.Contains(q.Get("scope"), "compose") {
		t.Fatal(q.Get("scope"))
	}
	if gotForm.Get("code_verifier") == "" || gotForm.Get("client_secret") != "" {
		t.Fatalf("exchange form: %v", gotForm)
	}

	// Incremental consent adds the send scope and keeps the original ones.
	sendScopes := append(append([]string{}, base...), "send")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_, err = Authorize(ctx2, Config{
		ClientID: "client-123",
		AuthURL:  srv.URL + "/auth",
		TokenURL: srv.URL + "/token",
		Scopes:   sendScopes,
		Extra:    url.Values{"include_granted_scopes": {"true"}, "prompt": {"consent"}},
		Open: func(u string) error {
			authURL = u
			parsed, _ := url.Parse(u)
			qq := parsed.Query()
			cb, _ := url.Parse(qq.Get("redirect_uri"))
			cb.RawQuery = url.Values{"code": {"def"}, "state": {qq.Get("state")}}.Encode()
			resp, err := http.Get(cb.String())
			if err != nil {
				return err
			}
			resp.Body.Close()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ = url.Parse(authURL)
	scope := parsed.Query().Get("scope")
	if !strings.Contains(scope, "send") || !strings.Contains(scope, "read") {
		t.Fatalf("incremental scope = %q", scope)
	}
}

func TestRefreshKeepsRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"at-2","expires_in":10}`))
	}))
	defer srv.Close()
	tok, err := Refresh(context.Background(), Config{ClientID: "c", TokenURL: srv.URL, Scopes: []string{"read"}}, "rt-keep")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "at-2" || tok.RefreshToken != "rt-keep" {
		t.Fatalf("%+v", tok)
	}
}

func TestTokenErrorDoesNotEchoBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","access_token":"should-not-leak"}`))
	}))
	defer srv.Close()
	_, err := Refresh(context.Background(), Config{ClientID: "c", TokenURL: srv.URL}, "rt")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "should-not-leak") || strings.Contains(err.Error(), "rt") {
		t.Fatal(err)
	}
}
