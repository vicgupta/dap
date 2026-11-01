// Package graph talks to Microsoft Graph for personal Microsoft accounts.
// The access token is supplied by the broker on each call.
package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vicgupta/dap/internal/mail/httpx"
	"github.com/vicgupta/dap/internal/mailbox"
)

const (
	// ScopeUser is the signed-in profile.
	ScopeUser = "User.Read"
	// ScopeReadWrite reads mail and saves drafts. It does not include Mail.Send.
	ScopeReadWrite = "Mail.ReadWrite"
	// ScopeOffline lets DAP refresh without asking again.
	ScopeOffline = "offline_access"
	// ScopeSend is requested only when the first send is approved.
	ScopeSend = "Mail.Send"
	// ScopeCalendar is optional and off unless the person turns it on.
	ScopeCalendar = "Calendars.Read"
)

// Client is a Graph REST client.
type Client struct {
	API  string
	HTTP *http.Client
	// OnThrottle records 429/503 even when the call eventually fails.
	OnThrottle func(httpx.Throttle)
}

func (c *Client) base() string {
	if c.API != "" {
		return strings.TrimRight(c.API, "/")
	}
	return "https://graph.microsoft.com/v1.0"
}

// Me returns the mailbox address.
func (c *Client) Me(ctx context.Context, token string) (string, error) {
	var out struct {
		Mail string `json:"mail"`
		UPN  string `json:"userPrincipalName"`
	}
	if err := c.get(ctx, token, "/me?$select=mail,userPrincipalName", &out); err != nil {
		return "", err
	}
	if out.Mail != "" {
		return out.Mail, nil
	}
	return out.UPN, nil
}

// DeltaCursor is the saved deltaLink for one folder.
type DeltaCursor struct {
	Inbox string
	Sent  string
}

// ListThreads reads inbox and sent-items deltas and folds them into conversations.
func (c *Client) ListThreads(ctx context.Context, token, me string, cur DeltaCursor) ([]mailbox.Thread, DeltaCursor, error) {
	inbox, nextInbox, err := c.delta(ctx, token, "/me/mailFolders/inbox/messages/delta", cur.Inbox)
	if err != nil {
		return nil, cur, err
	}
	sent, nextSent, err := c.delta(ctx, token, "/me/mailFolders/sentitems/messages/delta", cur.Sent)
	if err != nil {
		return nil, cur, err
	}
	cur.Inbox, cur.Sent = nextInbox, nextSent
	type agg struct {
		th mailbox.Thread
	}
	byConv := map[string]*agg{}
	order := []string{}
	add := func(m graphMessage, fromMe bool) {
		id := m.ConversationID
		if id == "" {
			id = m.ID
		}
		a := byConv[id]
		if a == nil {
			a = &agg{}
			byConv[id] = a
			order = append(order, id)
		}
		when := m.Received
		if when.IsZero() {
			when = m.Sent
		}
		if a.th.LastAt.After(when) && !a.th.LastAt.IsZero() {
			return
		}
		otherName, otherEmail := m.From.Email.Name, m.From.Email.Address
		if fromMe && len(m.To) > 0 {
			otherName, otherEmail = m.To[0].Email.Name, m.To[0].Email.Address
		}
		if otherName == "" {
			otherName = otherEmail
		}
		a.th = mailbox.Thread{
			ID:         id,
			MessageID:  m.ID,
			Provider:   "graph",
			Subject:    strings.TrimPrefix(m.Subject, "Re: "),
			WithName:   otherName,
			WithEmail:  otherEmail,
			LastAt:     when,
			LastFromMe: fromMe,
			Snippet:    m.BodyPreview,
			Short:      len(m.BodyPreview) > 0 && len(m.BodyPreview) < 80,
			Evidence:   "conversation " + id,
		}
		_ = me
	}
	for _, m := range inbox {
		add(m, false)
	}
	for _, m := range sent {
		add(m, true)
	}
	out := make([]mailbox.Thread, 0, len(order))
	for _, id := range order {
		out = append(out, byConv[id].th)
	}
	return out, cur, nil
}

type graphMessage struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Subject        string    `json:"subject"`
	BodyPreview    string    `json:"bodyPreview"`
	Received       time.Time `json:"receivedDateTime"`
	Sent           time.Time `json:"sentDateTime"`
	From           struct {
		Email struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"from"`
	To []struct {
		Email struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"toRecipients"`
}

func (c *Client) delta(ctx context.Context, token, path, link string) ([]graphMessage, string, error) {
	next := link
	if next == "" {
		next = c.base() + path + "?$select=id,conversationId,subject,bodyPreview,receivedDateTime,sentDateTime,from,toRecipients"
	}
	var all []graphMessage
	delta := ""
	for next != "" {
		var page struct {
			Value []graphMessage `json:"value"`
			Next  string         `json:"@odata.nextLink"`
			Delta string         `json:"@odata.deltaLink"`
		}
		if err := c.getAbs(ctx, token, next, &page); err != nil {
			return nil, link, err
		}
		all = append(all, page.Value...)
		next = page.Next
		if page.Delta != "" {
			delta = page.Delta
		}
	}
	if delta == "" {
		delta = link
	}
	return all, delta, nil
}

// Send replies inside the conversation when the thread id is a message id,
// and otherwise calls sendMail.
func (c *Client) Send(ctx context.Context, token string, req mailbox.SendRequest) (mailbox.SendResult, error) {
	payload := map[string]any{"comment": req.Body}
	replyTo := req.MessageID
	if replyTo == "" {
		replyTo = req.ThreadID
	}
	path := "/me/messages/" + urlEscape(replyTo) + "/reply"
	if req.ThreadID == "" {
		path = "/me/sendMail"
		payload = map[string]any{
			"message": map[string]any{
				"subject":      req.Subject,
				"body":         map[string]string{"contentType": "Text", "content": req.Body},
				"toRecipients": []any{map[string]any{"emailAddress": map[string]string{"address": req.To}}},
			},
		}
	}
	b, _ := json.Marshal(payload)
	if err := c.post(ctx, token, path, b); err != nil {
		return mailbox.SendResult{}, err
	}
	return mailbox.SendResult{
		Provider:  "graph",
		MessageID: "graph-sent",
		ThreadID:  req.ThreadID,
		To:        req.To,
		Subject:   req.Subject,
		SentAt:    time.Now().UTC(),
	}, nil
}

func (c *Client) get(ctx context.Context, token, path string, dest any) error {
	return c.getAbs(ctx, token, c.base()+path, dest)
}

func (c *Client) getAbs(ctx context.Context, token, full string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, th, err := httpx.Do(c.HTTP, req)
	c.note(th)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readJSON(resp, dest)
}

func (c *Client) post(ctx context.Context, token, path string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, th, err := httpx.Do(c.HTTP, req)
	c.note(th)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readJSON(resp, nil)
}

func (c *Client) note(th httpx.Throttle) {
	if th.Status == 0 || c.OnThrottle == nil {
		return
	}
	c.OnThrottle(th)
}

func readJSON(resp *http.Response, dest any) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("graph returned %d", resp.StatusCode)
	}
	if dest == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, dest)
}

func urlEscape(id string) string {
	return strings.ReplaceAll(id, "/", "%2F")
}
