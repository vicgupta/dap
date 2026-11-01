// Package oauth is the installed-app OAuth flow DAP uses for mail.
// It is authorization-code with PKCE and a loopback redirect. The process
// never asks for a password. The person signs in inside the provider's window.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config describes one public client.
type Config struct {
	ClientID     string
	AuthURL      string
	TokenURL     string
	RedirectHost string // "127.0.0.1" or "localhost"
	Scopes       []string
	Extra        url.Values
	// Open shows the provider's sign-in page. Tests drive the callback themselves.
	Open func(authURL string) error
}

// Token is what the provider returned. Callers store it in the keychain.
type Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	Scopes       []string
}

// Authorize runs the loopback flow and exchanges the code.
func Authorize(ctx context.Context, cfg Config) (Token, error) {
	if cfg.ClientID == "" {
		return Token{}, fmt.Errorf("client id is required")
	}
	if cfg.AuthURL == "" || cfg.TokenURL == "" {
		return Token{}, fmt.Errorf("oauth endpoints are required")
	}
	host := cfg.RedirectHost
	if host == "" {
		host = "127.0.0.1"
	}
	verifier, challenge, err := pkce()
	if err != nil {
		return Token{}, err
	}
	state, err := randomString(16)
	if err != nil {
		return Token{}, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Token{}, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirect := fmt.Sprintf("http://%s:%d/callback", host, port)

	q := url.Values{}
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirect)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(cfg.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	for k, vs := range cfg.Extra {
		for _, v := range vs {
			q.Set(k, v)
		}
	}
	authURL := cfg.AuthURL
	if strings.Contains(authURL, "?") {
		authURL += "&" + q.Encode()
	} else {
		authURL += "?" + q.Encode()
	}

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		if errText := r.URL.Query().Get("error"); errText != "" {
			http.Error(w, "Sign-in was cancelled.", http.StatusBadRequest)
			done <- result{err: fmt.Errorf("provider returned %s", errText)}
			return
		}
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Sign-in could not be verified.", http.StatusBadRequest)
			done <- result{err: fmt.Errorf("oauth state did not match")}
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Sign-in did not return a code.", http.StatusBadRequest)
			done <- result{err: fmt.Errorf("oauth code missing")}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><meta charset=utf-8><title>DAP</title><p>Signed in. You can close this window and return to DAP.</p>"))
		done <- result{code: code}
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shut, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()

	if cfg.Open != nil {
		if err := cfg.Open(authURL); err != nil {
			return Token{}, err
		}
	}
	var code string
	select {
	case <-ctx.Done():
		return Token{}, ctx.Err()
	case res := <-done:
		if res.err != nil {
			return Token{}, res.err
		}
		code = res.code
	}
	return exchange(ctx, cfg, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirect},
		"client_id":     {cfg.ClientID},
		"code_verifier": {verifier},
	})
}

// Refresh exchanges a refresh token for a new access token.
func Refresh(ctx context.Context, cfg Config, refreshToken string) (Token, error) {
	if refreshToken == "" {
		return Token{}, fmt.Errorf("no refresh token")
	}
	tok, err := exchange(ctx, cfg, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {cfg.ClientID},
	})
	if err != nil {
		return Token{}, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}
	return tok, nil
}

func exchange(ctx context.Context, cfg Config, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return Token{}, fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Token{}, fmt.Errorf("token response was not JSON")
	}
	if raw.Error != "" || raw.AccessToken == "" {
		return Token{}, fmt.Errorf("token response did not include an access token")
	}
	scopes := cfg.Scopes
	if raw.Scope != "" {
		scopes = strings.Fields(raw.Scope)
	}
	exp := time.Now().Add(time.Hour)
	if raw.ExpiresIn > 0 {
		exp = time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)
	}
	return Token{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		Expiry:       exp,
		Scopes:       scopes,
	}, nil
}

func pkce() (verifier, challenge string, err error) {
	verifier, err = randomString(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
