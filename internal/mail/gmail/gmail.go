// Package gmail talks to the Gmail API. It receives an access token from the
// broker and never stores one.
package gmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	netmail "net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/vicgupta/dap/internal/mail/httpx"
	"github.com/vicgupta/dap/internal/mailbox"
)

const (
	// ScopeRead is requested at sign-in.
	ScopeRead = "https://www.googleapis.com/auth/gmail.readonly"
	// ScopeCompose creates and edits drafts. Google's description also allows
	// send; DAP still refuses to call send until ScopeSend is granted.
	ScopeCompose = "https://www.googleapis.com/auth/gmail.compose"
	// ScopeSend is requested only when the first send is approved.
	ScopeSend = "https://www.googleapis.com/auth/gmail.send"
)

// Client is a Gmail REST client.
type Client struct {
	API  string
	HTTP *http.Client
}

func (c *Client) base() string {
	if c.API != "" {
		return strings.TrimRight(c.API, "/")
	}
	return "https://gmail.googleapis.com"
}

// Profile returns the mailbox address.
func (c *Client) Profile(ctx context.Context, token string) (string, error) {
	var out struct {
		EmailAddress string `json:"emailAddress"`
	}
	if err := c.get(ctx, token, "/gmail/v1/users/me/profile", &out); err != nil {
		return "", err
	}
	return out.EmailAddress, nil
}

// ListThreads returns recent inbox and sent conversations.
func (c *Client) ListThreads(ctx context.Context, token, me string) ([]mailbox.Thread, error) {
	return c.ListThreadsQuery(ctx, token, me, "newer_than:30d")
}

// ListThreadsQuery lists conversations matching a Gmail search query.
func (c *Client) ListThreadsQuery(ctx context.Context, token, me, query string) ([]mailbox.Thread, error) {
	if strings.TrimSpace(query) == "" {
		query = "newer_than:30d"
	}
	ids, err := c.threadIDs(ctx, token, query)
	if err != nil {
		return nil, err
	}
	var out []mailbox.Thread
	for _, id := range ids {
		th, err := c.thread(ctx, token, me, id)
		if err != nil {
			return nil, err
		}
		if th.ID != "" {
			out = append(out, th)
		}
	}
	return out, nil
}

func (c *Client) threadIDs(ctx context.Context, token, query string) ([]string, error) {
	var out struct {
		Threads []struct {
			ID string `json:"id"`
		} `json:"threads"`
	}
	q := url.Values{}
	q.Set("maxResults", "40")
	q.Set("q", query)
	path := "/gmail/v1/users/me/threads?" + q.Encode()
	if err := c.get(ctx, token, path, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Threads))
	seen := map[string]bool{}
	for _, th := range out.Threads {
		if th.ID != "" && !seen[th.ID] {
			seen[th.ID] = true
			ids = append(ids, th.ID)
		}
	}
	return ids, nil
}

func (c *Client) thread(ctx context.Context, token, me, id string) (mailbox.Thread, error) {
	var raw struct {
		ID       string `json:"id"`
		Messages []struct {
			ID       string   `json:"id"`
			Snippet  string   `json:"snippet"`
			LabelIDs []string `json:"labelIds"`
			Internal int64    `json:"internalDate,string"`
			Payload  struct {
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"payload"`
		} `json:"messages"`
	}
	if err := c.get(ctx, token, "/gmail/v1/users/me/threads/"+id+"?format=metadata&metadataHeaders=From&metadataHeaders=To&metadataHeaders=Subject&metadataHeaders=Date", &raw); err != nil {
		return mailbox.Thread{}, err
	}
	if len(raw.Messages) == 0 {
		return mailbox.Thread{}, nil
	}
	last := raw.Messages[len(raw.Messages)-1]
	header := map[string]string{}
	for _, h := range last.Payload.Headers {
		header[strings.ToLower(h.Name)] = h.Value
	}
	from := parseAddr(header["from"])
	to := parseAddr(header["to"])
	meAddr := strings.ToLower(me)
	fromMe := meAddr != "" && strings.EqualFold(from.Address, meAddr)
	other := to
	if fromMe {
		other = to
	} else {
		other = from
	}
	when := time.UnixMilli(last.Internal)
	if last.Internal == 0 {
		if t, err := mailDate(header["date"]); err == nil {
			when = t
		}
	}
	subject := header["subject"]
	return mailbox.Thread{
		ID:         raw.ID,
		MessageID:  last.ID,
		Provider:   "gmail",
		Subject:    strings.TrimPrefix(subject, "Re: "),
		WithName:   display(other),
		WithEmail:  other.Address,
		LastAt:     when,
		LastFromMe: fromMe,
		Snippet:    last.Snippet,
		Labels:     append([]string(nil), last.LabelIDs...),
		Short:      len(last.Snippet) > 0 && len(last.Snippet) < 80,
		Evidence:   "thread " + raw.ID,
	}, nil
}

// Send posts an RFC 5322 message into an existing thread.
func (c *Client) Send(ctx context.Context, token string, from string, req mailbox.SendRequest) (mailbox.SendResult, error) {
	raw := buildRFC(from, req)
	body := map[string]string{
		"raw":      base64.RawURLEncoding.EncodeToString([]byte(raw)),
		"threadId": req.ThreadID,
	}
	b, _ := json.Marshal(body)
	var out struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
	}
	if err := c.post(ctx, token, "/gmail/v1/users/me/messages/send", b, &out); err != nil {
		return mailbox.SendResult{}, err
	}
	return mailbox.SendResult{
		Provider:  "gmail",
		MessageID: out.ID,
		ThreadID:  out.ThreadID,
		To:        req.To,
		Subject:   req.Subject,
		SentAt:    time.Now().UTC(),
	}, nil
}

func buildRFC(from string, req mailbox.SendRequest) string {
	subj := req.Subject
	if subj != "" && !strings.HasPrefix(strings.ToLower(subj), "re:") {
		subj = "Re: " + subj
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", req.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", subj)
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(req.Body)
	return b.String()
}

func (c *Client) get(ctx context.Context, token, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _, err := httpx.Do(c.HTTP, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readJSON(resp, dest)
}

func (c *Client) post(ctx context.Context, token, path string, body []byte, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, _, err := httpx.Do(c.HTTP, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readJSON(resp, dest)
}

func readJSON(resp *http.Response, dest any) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gmail returned %d", resp.StatusCode)
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(b, dest)
}

func parseAddr(v string) netmail.Address {
	addr, err := netmail.ParseAddress(v)
	if err != nil {
		return netmail.Address{Address: strings.TrimSpace(v)}
	}
	return *addr
}

func display(a netmail.Address) string {
	if a.Name != "" {
		return a.Name
	}
	if i := strings.Index(a.Address, "@"); i > 0 {
		return a.Address[:i]
	}
	return a.Address
}

func mailDate(v string) (time.Time, error) {
	return netmail.ParseDate(v)
}
