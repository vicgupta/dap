// Package composio is DAP's connectivity layer for Composio's hosted API.
// Composio holds third-party OAuth tokens and refreshes them. DAP stores
// only the API key and one connection id per toolkit, in the secret keeper.
package composio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	defaultBase = "https://backend.composio.dev"
	apiPrefix   = "/api/v3.1"
	// UserID is the single local user DAP registers with Composio.
	UserID = "dap"
)

// Client talks to Composio's REST API. There is no official Go SDK.
type Client struct {
	Base string
	Key  string
	HTTP *http.Client
}

// Link is a hosted sign-in session.
type Link struct {
	RedirectURL string
	AccountID   string
}

// Account is the connection Composio is holding. It does not include tokens.
type Account struct {
	ID     string
	Status string
}

func (c *Client) base() string {
	if strings.TrimSpace(c.Base) == "" {
		return defaultBase
	}
	return strings.TrimRight(c.Base, "/")
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// EnsureAuthConfig returns a Composio-managed auth config id for a toolkit slug.
// An existing managed config is reused. DAP does not create its own OAuth client.
func (c *Client) EnsureAuthConfig(ctx context.Context, toolkit string) (string, error) {
	q := url.Values{}
	q.Set("toolkit_slug", toolkit)
	q.Set("limit", "20")
	var listed struct {
		Items []struct {
			ID        string `json:"id"`
			IsManaged bool   `json:"is_composio_managed"`
		} `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, apiPrefix+"/auth_configs?"+q.Encode(), nil, &listed); err != nil {
		return "", err
	}
	for _, item := range listed.Items {
		if item.IsManaged && item.ID != "" {
			return item.ID, nil
		}
	}
	for _, item := range listed.Items {
		if item.ID != "" {
			return item.ID, nil
		}
	}
	var created struct {
		AuthConfig struct {
			ID string `json:"id"`
		} `json:"auth_config"`
	}
	body := map[string]any{
		"toolkit": map[string]string{"slug": toolkit},
		"auth_config": map[string]any{
			"type":        "use_composio_managed_auth",
			"name":        "DAP " + toolkit,
			"credentials": map[string]any{},
		},
	}
	if err := c.do(ctx, http.MethodPost, apiPrefix+"/auth_configs", body, &created); err != nil {
		return "", err
	}
	if created.AuthConfig.ID == "" {
		return "", errors.New("composio did not return an auth config id")
	}
	return created.AuthConfig.ID, nil
}

// LinkAccount starts hosted sign-in and returns the URL to open.
func (c *Client) LinkAccount(ctx context.Context, authConfigID string) (Link, error) {
	var out struct {
		RedirectURL string `json:"redirect_url"`
		RedirectAlt string `json:"redirectUrl"`
		AccountID   string `json:"connected_account_id"`
		AccountAlt  string `json:"connectedAccountId"`
	}
	body := map[string]string{
		"auth_config_id": authConfigID,
		"user_id":        UserID,
	}
	if err := c.do(ctx, http.MethodPost, apiPrefix+"/connected_accounts/link", body, &out); err != nil {
		return Link{}, err
	}
	link := Link{RedirectURL: first(out.RedirectURL, out.RedirectAlt), AccountID: first(out.AccountID, out.AccountAlt)}
	if link.RedirectURL == "" || link.AccountID == "" {
		return Link{}, errors.New("composio did not return a sign-in link")
	}
	return link, nil
}

// Account fetches connection status. Tokens are not part of the decoded result.
func (c *Client) Account(ctx context.Context, id string) (Account, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, apiPrefix+"/connected_accounts/"+url.PathEscape(id), nil, &raw); err != nil {
		return Account{}, err
	}
	status := strings.ToUpper(findString(raw, "status"))
	return Account{ID: first(findString(raw, "id"), id), Status: status}, nil
}

// WaitActive polls until the connection is ACTIVE, or until ctx ends.
func (c *Client) WaitActive(ctx context.Context, id string) (Account, error) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		acc, err := c.Account(ctx, id)
		if err != nil {
			return Account{}, err
		}
		switch acc.Status {
		case "ACTIVE":
			return acc, nil
		case "FAILED", "EXPIRED", "INACTIVE":
			return Account{}, fmt.Errorf("composio connection %s", strings.ToLower(acc.Status))
		}
		select {
		case <-ctx.Done():
			return Account{}, errors.New("sign-in was not finished")
		case <-tick.C:
		}
	}
}

// Execute runs one tool. version latest avoids the frozen v3 default.
func (c *Client) Execute(ctx context.Context, tool, accountID string, args map[string]any) (json.RawMessage, error) {
	if args == nil {
		args = map[string]any{}
	}
	body := map[string]any{
		"connected_account_id": accountID,
		"user_id":              UserID,
		"version":              "latest",
		"arguments":            args,
	}
	var raw map[string]any
	if err := c.do(ctx, http.MethodPost, apiPrefix+"/tools/execute/"+url.PathEscape(tool), body, &raw); err != nil {
		return nil, err
	}
	if ok, exists := raw["successful"].(bool); exists && !ok {
		return nil, &APIError{Plain: "Composio could not complete that request."}
	}
	if data, ok := raw["data"]; ok && data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	b, err := json.Marshal(raw)
	return b, err
}

// Probe checks the key with one small authenticated call.
func (c *Client) Probe(ctx context.Context) error {
	var dest map[string]any
	return c.do(ctx, http.MethodGet, apiPrefix+"/auth_configs?limit=1", nil, &dest)
}

// DeleteAccount revokes a remote connection. A missing account is not an error.
func (c *Client) DeleteAccount(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, apiPrefix+"/connected_accounts/"+url.PathEscape(id), nil, nil)
	if err == nil {
		return nil
	}
	var api *APIError
	if errors.As(err, &api) && api.Status == http.StatusNotFound {
		return nil
	}
	return err
}

func (c *Client) do(ctx context.Context, method, path string, body any, dest any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, reader)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.Key) == "" {
		return errors.New("composio api key is missing")
	}
	req.Header.Set("x-api-key", c.Key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return &APIError{Plain: "DAP could not reach Composio. Check the network and try again."}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		return interpretStatus(resp.StatusCode, b)
	}
	if dest == nil || len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return fmt.Errorf("composio response: %w", err)
	}
	return nil
}

// APIError is a Composio failure safe to show. It never includes the key,
// a masked tail, or the raw response body.
type APIError struct {
	Status    int
	Slug      string
	RequestID string
	Plain     string
}

func (e *APIError) Error() string {
	if e == nil || e.Plain == "" {
		return "Composio could not complete that request."
	}
	return e.Plain
}

func interpretStatus(status int, body []byte) *APIError {
	var parsed struct {
		Error struct {
			Slug      string `json:"slug"`
			RequestID string `json:"request_id"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(body, &parsed)
	rid := parsed.Error.RequestID
	if rid == "" {
		rid = parsed.RequestID
	}
	return &APIError{
		Status:    status,
		Slug:      parsed.Error.Slug,
		RequestID: safeRequestID(rid),
		Plain:     explainStatus(status, parsed.Error.Slug),
	}
}

func explainStatus(status int, slug string) string {
	switch {
	case status == http.StatusUnauthorized || slug == "APIKey_InvalidAPIKey":
		return "Composio rejected this key. Paste a project API key from the developer dashboard’s API Keys page. It starts with ak_."
	case status == http.StatusNotFound || strings.Contains(slug, "AuthConfig") || strings.Contains(slug, "Toolkit"):
		return "Composio could not set up that connection. Try again in a moment."
	case status == http.StatusTooManyRequests:
		return "Composio asked DAP to slow down. Wait a moment and try again."
	case status >= 500:
		return "Composio had a problem on its side. Try again in a moment."
	default:
		return "Composio could not complete that request."
	}
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{6,80}$`)

// Public is the sentence and optional request id safe to show in the UI.
func Public(err error) (message, requestID string) {
	if err == nil {
		return "", ""
	}
	var api *APIError
	if errors.As(err, &api) {
		return scrub(api.Error()), safeRequestID(api.RequestID)
	}
	return scrub(err.Error()), ""
}

var leakedKey = regexp.MustCompile(`(?i)(?:ck|ak)_[A-Za-z0-9*]{2,}`)

func scrub(s string) string {
	if leakedKey.MatchString(s) || strings.Contains(s, `"slug"`) || strings.Contains(s, "Invalid API key") {
		return "Composio could not complete that request."
	}
	return s
}

func safeRequestID(s string) string {
	if strings.Contains(s, "ck_") || strings.Contains(s, "ak_") || strings.Contains(s, "*") {
		return ""
	}
	if !requestIDPattern.MatchString(s) {
		return ""
	}
	return s
}

func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
