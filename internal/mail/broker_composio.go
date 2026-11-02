package mail

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/vicgupta/dap/internal/composio"
	"github.com/vicgupta/dap/internal/mail/graph"
	"github.com/vicgupta/dap/internal/mailbox"
	"github.com/vicgupta/dap/internal/secrets"
)

// useDirect reports whether this provider still uses DAP's own OAuth client.
// Gmail does that only when DAP_GMAIL_DIRECT=1, the saved choice is direct,
// or a test injected GoogleAuth. Outlook's Graph path is the supported fallback.
func (b *Broker) useDirect(provider string) bool {
	provider = canonProvider(provider)
	switch provider {
	case "gmail":
		if b.GoogleAuth != "" || os.Getenv("DAP_GMAIL_DIRECT") == "1" {
			return true
		}
	case "outlook":
		if b.GraphAuth != "" || os.Getenv("DAP_OUTLOOK_DIRECT") == "1" {
			return true
		}
	default:
		return false
	}
	b.mu.Lock()
	mode := b.modeOf(provider)
	b.mu.Unlock()
	return mode == "direct"
}

func (b *Broker) modeOf(provider string) string {
	if provider == "outlook" {
		if m := b.public["outlook"].Mode; m != "" {
			return m
		}
		return b.public["graph"].Mode
	}
	return b.public[provider].Mode
}

func canonProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "graph", "outlook":
		return "outlook"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func publicKey(provider string) string {
	if canonProvider(provider) == "outlook" {
		return "graph"
	}
	return provider
}

func secretAccount(provider string) string {
	if canonProvider(provider) == "outlook" {
		return "graph"
	}
	return provider
}

func (b *Broker) connectComposio(ctx context.Context, provider string) error {
	_, err := b.connect.Connect(ctx, canonProvider(provider))
	return err
}

func (b *Broker) composioThreads(ctx context.Context, provider string) ([]Thread, error) {
	provider = canonProvider(provider)
	reg, err := b.connect.Registry()
	if err != nil {
		return nil, err
	}
	tk, ok := reg.ByID(provider)
	if !ok || tk.Kind != "mail" {
		return nil, errors.New("that connection is not a mailbox")
	}
	me := ""
	if conn, ok := b.connect.Get(provider); ok {
		me = conn.Account
	}
	var msgs []composio.Message
	switch provider {
	case "gmail":
		raw, err := b.connect.Execute(ctx, provider, "GMAIL_FETCH_EMAILS", map[string]any{
			"query": "newer_than:30d", "max_results": 40, "verbose": false, "user_id": "me",
		})
		if err != nil {
			return nil, err
		}
		msgs = composio.Messages(raw, me, false)
	case "outlook":
		inbox, err := b.connect.Execute(ctx, provider, "OUTLOOK_LIST_MESSAGES", outlookListArgs("inbox"))
		if err != nil {
			return nil, err
		}
		sent, err := b.connect.Execute(ctx, provider, "OUTLOOK_LIST_MESSAGES", outlookListArgs("sentitems"))
		if err != nil {
			return nil, err
		}
		msgs = append(composio.Messages(inbox, me, false), composio.Messages(sent, me, true)...)
	default:
		tool := tk.Actions["threads"]
		if tool == "" {
			return nil, errors.New(tk.Label + " has no threads action")
		}
		raw, err := b.connect.Execute(ctx, provider, tool, map[string]any{
			"user_id": "me", "max_results": 40, "query": "newer_than:30d",
		})
		if err != nil {
			return nil, err
		}
		msgs = composio.Messages(raw, me, false)
	}
	return toThreads(provider, composio.Latest(msgs)), nil
}

func outlookListArgs(folder string) map[string]any {
	return map[string]any{
		"folder":  folder,
		"top":     30,
		"user_id": "me",
		"select":  []string{"id", "subject", "from", "toRecipients", "receivedDateTime", "bodyPreview", "conversationId"},
	}
}

func toThreads(provider string, msgs []composio.Message) []Thread {
	out := make([]Thread, 0, len(msgs))
	for _, m := range msgs {
		otherName, otherEmail := m.FromName, m.FromEmail
		if m.FromMe {
			otherEmail = m.ToEmail
			otherName = ""
		}
		if otherName == "" && otherEmail != "" {
			if i := strings.Index(otherEmail, "@"); i > 0 {
				otherName = otherEmail[:i]
			} else {
				otherName = otherEmail
			}
		}
		id := m.ThreadID
		if id == "" {
			id = m.ID
		}
		out = append(out, Thread{
			ID: id, MessageID: m.ID, Provider: provider,
			Subject:  strings.TrimPrefix(m.Subject, "Re: "),
			WithName: otherName, WithEmail: otherEmail, LastAt: m.At,
			LastFromMe: m.FromMe, Snippet: m.Snippet,
			Short:    len(m.Snippet) > 0 && len(m.Snippet) < 80,
			Evidence: "thread " + id,
		})
	}
	return out
}

func (b *Broker) composioDraft(ctx context.Context, req SendRequest) (mailbox.DraftResult, error) {
	provider := canonProvider(req.Provider)
	tool, args, local, err := draftCall(provider, req, b)
	if err != nil {
		return mailbox.DraftResult{}, err
	}
	if local {
		return mailbox.DraftResult{Provider: provider, Local: true}, nil
	}
	raw, err := b.connect.Execute(ctx, provider, tool, args)
	if err != nil {
		return mailbox.DraftResult{}, err
	}
	return mailbox.DraftResult{Provider: provider, DraftID: composio.Identifier(raw)}, nil
}

func draftCall(provider string, req SendRequest, b *Broker) (string, map[string]any, bool, error) {
	switch provider {
	case "gmail":
		args := map[string]any{"user_id": "me", "body": req.Body, "recipient_email": req.To}
		if req.ThreadID != "" {
			args["thread_id"] = req.ThreadID
		} else if req.Subject != "" {
			args["subject"] = replySubject(req.Subject)
		}
		return "GMAIL_CREATE_EMAIL_DRAFT", args, false, nil
	case "outlook":
		if req.MessageID != "" {
			return "OUTLOOK_CREATE_DRAFT_REPLY", map[string]any{
				"user_id": "me", "message_id": req.MessageID, "comment": req.Body,
			}, false, nil
		}
		return "OUTLOOK_CREATE_DRAFT", map[string]any{
			"user_id": "me", "subject": replySubject(req.Subject), "body": req.Body,
			"to_recipients": []string{req.To},
		}, false, nil
	default:
		reg, err := b.connect.Registry()
		if err != nil {
			return "", nil, false, err
		}
		tk, ok := reg.ByID(provider)
		if !ok || tk.Kind != "mail" {
			return "", nil, false, errors.New("that connection cannot draft")
		}
		tool := tk.Actions["draft"]
		if tool == "" {
			return "", nil, true, nil
		}
		return tool, map[string]any{
			"user_id": "me", "body": req.Body, "subject": req.Subject,
			"recipient_email": req.To, "thread_id": req.ThreadID, "message_id": req.MessageID,
		}, false, nil
	}
}

func (b *Broker) composioSend(ctx context.Context, req SendRequest) (SendResult, error) {
	provider := canonProvider(req.Provider)
	tool, args, err := sendCall(provider, req, b)
	if err != nil {
		return SendResult{}, err
	}
	raw, err := b.connect.Execute(ctx, provider, tool, args)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{
		Provider: provider, MessageID: composio.Identifier(raw), ThreadID: req.ThreadID,
		To: req.To, Subject: req.Subject, SentAt: time.Now().UTC(),
	}, nil
}

func sendCall(provider string, req SendRequest, b *Broker) (string, map[string]any, error) {
	switch provider {
	case "gmail":
		if req.ThreadID != "" {
			return "GMAIL_REPLY_TO_THREAD", map[string]any{
				"user_id": "me", "thread_id": req.ThreadID, "recipient_email": req.To,
				"message_body": req.Body, "include_signature": false,
			}, nil
		}
		return "GMAIL_SEND_EMAIL", map[string]any{
			"user_id": "me", "recipient_email": req.To, "subject": replySubject(req.Subject),
			"body": req.Body, "include_signature": false,
		}, nil
	case "outlook":
		if req.MessageID != "" {
			return "OUTLOOK_REPLY_EMAIL", map[string]any{
				"user_id": "me", "message_id": req.MessageID, "comment": req.Body,
			}, nil
		}
		return "OUTLOOK_SEND_EMAIL", map[string]any{
			"user_id": "me", "to": req.To, "subject": replySubject(req.Subject), "body": req.Body,
		}, nil
	default:
		reg, err := b.connect.Registry()
		if err != nil {
			return "", nil, err
		}
		tk, ok := reg.ByID(provider)
		if !ok || tk.Kind != "mail" {
			return "", nil, errors.New("that connection cannot send")
		}
		tool := tk.Actions["send"]
		if req.ThreadID != "" && tk.Actions["reply"] != "" {
			tool = tk.Actions["reply"]
		}
		if tool == "" {
			return "", nil, errors.New(tk.Label + " has no send action")
		}
		return tool, map[string]any{
			"user_id": "me", "body": req.Body, "message_body": req.Body,
			"subject": replySubject(req.Subject), "recipient_email": req.To, "to": req.To,
			"thread_id": req.ThreadID, "message_id": req.MessageID,
		}, nil
	}
}

func replySubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" || strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

// composioRows lists every toolkit. Connection ids and the API key are omitted.
func (b *Broker) composioRows() []Status {
	if b.connect == nil {
		return nil
	}
	reg, err := b.connect.Registry()
	if err != nil {
		return []Status{{
			Provider: "gmail", Label: "Gmail", Kind: "mail", Mode: "composio",
			RefreshDetail: err.Error(),
		}}
	}
	keySaved := b.connect.HasKey()
	var out []Status
	for _, tk := range reg.List() {
		b.mu.Lock()
		pub := b.public[publicKey(tk.ID)]
		b.mu.Unlock()
		if b.useDirect(tk.ID) {
			st := b.directStatus(tk.ID)
			st.KeySaved = keySaved
			st.DirectFallback = tk.Fallback
			st.Kind = tk.Kind
			st.Mode = "direct"
			st.Summary = directSummary(tk.ID)
			out = append(out, st)
			continue
		}
		st := Status{
			Provider: tk.ID, Label: tk.Label, Kind: tk.Kind, Summary: tk.Summary,
			Mode: "composio", Broker: "composio", KeySaved: keySaved, DirectFallback: tk.Fallback,
			ClientID: pub.ClientID, ClientIDSet: pub.ClientID != "", CalendarEnabled: pub.Calendar,
		}
		if conn, ok := b.connect.Get(tk.ID); ok {
			st.Connected = true
			st.Via = "composio"
			st.Account = conn.Account
			st.SendGranted = tk.Kind == "mail"
			st.Scopes = []string{"composio:" + tk.Slug}
			if conn.Status != "" && !strings.EqualFold(conn.Status, "ACTIVE") {
				st.RefreshOK = false
				st.RefreshDetail = "Composio connection is " + strings.ToLower(conn.Status)
			} else {
				st.RefreshOK = true
				st.RefreshDetail = "Composio holds the token and refreshes it"
			}
		} else {
			st.RefreshDetail = "not connected"
		}
		out = append(out, st)
	}
	return out
}

func (b *Broker) directStatus(provider string) Status {
	account := secretAccount(provider)
	st := b.status(account)
	st.Provider = canonProvider(provider)
	st.Label = label(st.Provider)
	st.Broker = directBroker(provider)
	return st
}

func directBroker(provider string) string {
	if canonProvider(provider) == "outlook" {
		return graph.BrokerName()
	}
	return "system-browser"
}

func directSummary(provider string) string {
	if canonProvider(provider) == "outlook" {
		return "This laptop holds the Microsoft token. Graph is the fallback."
	}
	return "This laptop holds the Google token. Direct Google is the legacy path."
}

// Registry is the toolkit list, including composio/toolkits.json.
func (b *Broker) Registry() (composio.Registry, error) {
	if b.connect == nil {
		return composio.Registry{}, errors.New("composio is not configured")
	}
	return b.connect.Registry()
}

// Execute runs one declared Composio tool. The host has already checked permission.
func (b *Broker) Execute(ctx context.Context, toolkit, tool string, args map[string]any) (json.RawMessage, error) {
	b.prepare()
	return b.connect.Execute(ctx, toolkit, tool, args)
}

// HasComposioKey reports whether the API key is stored.
func (b *Broker) HasComposioKey() bool {
	return b.connect != nil && b.connect.HasKey()
}

// SaveComposioKey probes the key and stores it only when Composio accepts it.
func (b *Broker) SaveComposioKey(ctx context.Context, key string) (composio.KeyCheck, error) {
	b.prepare()
	if b.connect == nil {
		return composio.KeyCheck{}, errors.New("composio is not configured")
	}
	return b.connect.SaveKey(ctx, key)
}

func ignoreMissing(err error) error {
	if err == nil || errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "not found") || strings.Contains(msg, "could not be found") {
		return nil
	}
	return err
}
