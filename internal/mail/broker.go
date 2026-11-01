package mail

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vicgupta/dap/internal/desktop"
	"github.com/vicgupta/dap/internal/mail/gmail"
	"github.com/vicgupta/dap/internal/mail/graph"
	"github.com/vicgupta/dap/internal/mail/httpx"
	"github.com/vicgupta/dap/internal/mail/oauth"
	"github.com/vicgupta/dap/internal/mailbox"
	"github.com/vicgupta/dap/internal/secrets"
)

// Re-export the documents callers use. Tokens are not among them.
type (
	Thread      = mailbox.Thread
	SendRequest = mailbox.SendRequest
	SendResult  = mailbox.SendResult
	Status      = mailbox.Status
	Throttle    = mailbox.Throttle
)

const secretService = "dap.mail"

// Broker holds mail tokens and performs provider calls.
type Broker struct {
	dir     string
	secrets secrets.Keeper
	fake    *Box
	open    func(string) error
	now     func() time.Time

	mu     sync.Mutex
	public map[string]publicCfg

	GmailAPI    string
	GraphAPI    string
	GoogleAuth  string
	GoogleToken string
	GraphAuth   string
	GraphToken  string
}

type publicCfg struct {
	ClientID string `json:"client_id,omitempty"`
	Calendar bool   `json:"calendar,omitempty"`
}

type secretBlob struct {
	Account        string    `json:"account,omitempty"`
	AccessToken    string    `json:"access_token,omitempty"`
	RefreshToken   string    `json:"refresh_token,omitempty"`
	Expiry         time.Time `json:"expiry,omitempty"`
	Scopes         []string  `json:"scopes,omitempty"`
	DeltaInbox     string    `json:"delta_inbox,omitempty"`
	DeltaSent      string    `json:"delta_sent,omitempty"`
	ThrottleUntil  time.Time `json:"throttle_until,omitempty"`
	ThrottleStatus int       `json:"throttle_status,omitempty"`
	Connected      bool      `json:"connected"`
	RefreshOK      bool      `json:"refresh_ok"`
	RefreshDetail  string    `json:"refresh_detail,omitempty"`
}

// New opens the broker for a data directory.
func New(dir string, keeper secrets.Keeper) (*Broker, error) {
	if err := os.MkdirAll(filepath.Join(dir, "mail"), 0o700); err != nil {
		return nil, err
	}
	box, err := OpenBox(filepath.Join(dir, "mail"))
	if err != nil {
		return nil, err
	}
	b := &Broker{
		dir:     dir,
		secrets: keeper,
		fake:    box,
		open:    desktop.Open,
		now:     time.Now,
		public:  map[string]publicCfg{},
	}
	_ = b.loadPublic()
	return b, nil
}

// SetClock overrides the clock.
func (b *Broker) SetClock(now func() time.Time) { b.now = now }

// SetOpener overrides how the sign-in window is shown.
func (b *Broker) SetOpener(fn func(string) error) { b.open = fn }

func (b *Broker) loadPublic() error {
	raw, err := os.ReadFile(b.publicPath())
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, &b.public)
}

func (b *Broker) publicPath() string { return filepath.Join(b.dir, "mail", "public.json") }

func (b *Broker) savePublicLocked() error {
	raw, err := json.MarshalIndent(b.public, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(b.publicPath(), append(raw, '\n'), 0o600)
}

// Setup stores a public client id. Calendar applies to Outlook only and defaults off.
func (b *Broker) Setup(provider, clientID string, calendar bool) error {
	if provider != "gmail" && provider != "graph" {
		return errors.New("this provider has no client id")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	cfg := b.public[provider]
	cfg.ClientID = strings.TrimSpace(clientID)
	cfg.Calendar = calendar
	b.public[provider] = cfg
	return b.savePublicLocked()
}

// Connect starts sign-in. The demo mailbox connects locally.
func (b *Broker) Connect(ctx context.Context, provider string) error {
	switch provider {
	case "fake":
		return b.fake.Connect()
	case "gmail", "graph":
		return b.authorize(ctx, provider, false)
	default:
		return errors.New("unknown mail provider")
	}
}

// Disconnect drops tokens. The public client id is kept.
func (b *Broker) Disconnect(provider string) error {
	switch provider {
	case "fake":
		return b.fake.Disconnect()
	case "gmail", "graph":
		return b.secrets.Delete(secretService, provider)
	default:
		return errors.New("unknown mail provider")
	}
}

// Status reports one provider. Tokens are not included.
func (b *Broker) Status(provider string) Status {
	return b.status(provider)
}

// Statuses reports Gmail, Outlook, and the demo mailbox.
func (b *Broker) Statuses() []Status {
	return []Status{b.Status("gmail"), b.Status("graph"), b.Status("fake")}
}

// Check refreshes a live token so doctor can report whether refresh works.
func (b *Broker) Check(ctx context.Context, provider string) Status {
	if provider == "gmail" || provider == "graph" {
		blob, _ := b.load(provider)
		if blob.Connected {
			if _, err := b.ensureFresh(ctx, provider); err != nil {
				_ = b.noteRefresh(provider, false, "refresh failed")
			} else {
				_ = b.noteRefresh(provider, true, "token present, refresh works")
			}
		}
	}
	return b.status(provider)
}

// Threads lists conversations for a connected mailbox.
func (b *Broker) Threads(ctx context.Context, provider string) ([]Thread, error) {
	switch provider {
	case "fake":
		return b.fake.Threads(b.now()), nil
	case "gmail":
		blob, err := b.ensureFresh(ctx, provider)
		if err != nil {
			return nil, err
		}
		return (&gmail.Client{API: b.GmailAPI}).ListThreads(ctx, blob.AccessToken, blob.Account)
	case "graph":
		blob, err := b.ensureFresh(ctx, provider)
		if err != nil {
			return nil, err
		}
		client := &graph.Client{API: b.GraphAPI, OnThrottle: func(th httpx.Throttle) {
			_ = b.noteThrottle(provider, th.Until, th.Status)
		}}
		threads, cur, err := client.ListThreads(ctx, blob.AccessToken, blob.Account, graph.DeltaCursor{Inbox: blob.DeltaInbox, Sent: blob.DeltaSent})
		if err != nil {
			return nil, err
		}
		blob.DeltaInbox, blob.DeltaSent = cur.Inbox, cur.Sent
		_ = b.put(provider, blob)
		return threads, nil
	default:
		return nil, errors.New("unknown mail provider")
	}
}

// Send delivers one message. The first real send asks for the send scope if it is missing.
func (b *Broker) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	switch req.Provider {
	case "fake":
		return b.fake.Send(req, b.now())
	case "gmail":
		blob, err := b.ensureFresh(ctx, "gmail")
		if err != nil {
			return SendResult{}, err
		}
		if !hasScope(blob.Scopes, gmail.ScopeSend) {
			if err := b.authorize(ctx, "gmail", true); err != nil {
				return SendResult{}, err
			}
			blob, err = b.load("gmail")
			if err != nil {
				return SendResult{}, err
			}
		}
		return (&gmail.Client{API: b.GmailAPI}).Send(ctx, blob.AccessToken, blob.Account, req)
	case "graph":
		blob, err := b.ensureFresh(ctx, "graph")
		if err != nil {
			return SendResult{}, err
		}
		if !hasScope(blob.Scopes, graph.ScopeSend) {
			if err := b.authorize(ctx, "graph", true); err != nil {
				return SendResult{}, err
			}
			blob, err = b.load("graph")
			if err != nil {
				return SendResult{}, err
			}
		}
		client := &graph.Client{API: b.GraphAPI, OnThrottle: func(th httpx.Throttle) {
			_ = b.noteThrottle("graph", th.Until, th.Status)
		}}
		return client.Send(ctx, blob.AccessToken, req)
	default:
		return SendResult{}, errors.New("unknown mail provider")
	}
}

// Undo pulls back a demo send inside twelve minutes. Real providers cannot.
func (b *Broker) Undo(provider, messageID string) error {
	if provider != "fake" {
		return errors.New("this mailbox cannot pull a sent message back")
	}
	return b.fake.Undo(messageID, b.now())
}

func (b *Broker) authorize(ctx context.Context, provider string, incremental bool) error {
	b.mu.Lock()
	pub := b.public[provider]
	b.mu.Unlock()
	if pub.ClientID == "" {
		return errors.New("add a client id on the Connections page first")
	}
	scopes := baseScopes(provider, pub.Calendar)
	if incremental {
		scopes = append(scopes, sendScope(provider))
	}
	cfg := oauth.Config{
		ClientID:     pub.ClientID,
		AuthURL:      b.authURL(provider),
		TokenURL:     b.tokenURL(provider),
		RedirectHost: redirectHost(provider),
		Scopes:       scopes,
		Extra:        oauthExtra(provider, incremental),
		Open:         b.open,
	}
	tok, err := oauth.Authorize(ctx, cfg)
	if err != nil {
		return err
	}
	blob, _ := b.load(provider)
	blob.Connected = true
	blob.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		blob.RefreshToken = tok.RefreshToken
	}
	blob.Expiry = tok.Expiry
	blob.Scopes = tok.Scopes
	blob.RefreshOK = true
	blob.RefreshDetail = "token present, refresh works"
	if account, err := b.profile(ctx, provider, blob.AccessToken); err == nil && account != "" {
		blob.Account = account
	}
	return b.put(provider, blob)
}

func (b *Broker) ensureFresh(ctx context.Context, provider string) (secretBlob, error) {
	blob, err := b.load(provider)
	if err != nil || !blob.Connected || blob.AccessToken == "" {
		return secretBlob{}, errors.New(provider + " is not connected")
	}
	if blob.Expiry.After(b.now().Add(time.Minute)) {
		return blob, nil
	}
	b.mu.Lock()
	pub := b.public[provider]
	b.mu.Unlock()
	tok, err := oauth.Refresh(ctx, oauth.Config{
		ClientID: pub.ClientID,
		TokenURL: b.tokenURL(provider),
		Scopes:   blob.Scopes,
	}, blob.RefreshToken)
	if err != nil {
		_ = b.noteRefresh(provider, false, "refresh failed")
		return secretBlob{}, errors.New("could not refresh the mailbox token")
	}
	blob.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		blob.RefreshToken = tok.RefreshToken
	}
	blob.Expiry = tok.Expiry
	if len(tok.Scopes) > 0 {
		blob.Scopes = tok.Scopes
	}
	blob.RefreshOK = true
	blob.RefreshDetail = "token present, refresh works"
	if err := b.put(provider, blob); err != nil {
		return secretBlob{}, err
	}
	return blob, nil
}

func (b *Broker) profile(ctx context.Context, provider, token string) (string, error) {
	switch provider {
	case "gmail":
		return (&gmail.Client{API: b.GmailAPI}).Profile(ctx, token)
	case "graph":
		return (&graph.Client{API: b.GraphAPI}).Me(ctx, token)
	default:
		return "", nil
	}
}

func (b *Broker) status(provider string) Status {
	st := Status{Provider: provider, Label: label(provider), Broker: brokerName(provider)}
	b.mu.Lock()
	pub := b.public[provider]
	b.mu.Unlock()
	st.ClientID = pub.ClientID
	st.ClientIDSet = pub.ClientID != ""
	st.CalendarEnabled = pub.Calendar
	if provider == "fake" {
		st.Connected = b.fake.Connected()
		st.Broker = "demo"
		if st.Connected {
			st.Account = "vic@example.com"
			st.Scopes = []string{"demo"}
			st.SendGranted = true
			st.RefreshOK = true
			st.RefreshDetail = "demo mailbox, no token"
		} else {
			st.RefreshDetail = "not connected"
		}
		return st
	}
	blob, err := b.load(provider)
	if err != nil || !blob.Connected {
		st.RefreshDetail = "not connected"
		return st
	}
	st.Connected = true
	st.Account = blob.Account
	st.Scopes = append([]string(nil), blob.Scopes...)
	st.SendGranted = hasScope(blob.Scopes, sendScope(provider))
	st.RefreshOK = blob.RefreshOK
	st.RefreshDetail = blob.RefreshDetail
	if st.RefreshDetail == "" {
		st.RefreshDetail = "token present"
	}
	if !blob.ThrottleUntil.IsZero() && blob.ThrottleUntil.After(b.now()) {
		st.Throttle = Throttle{Active: true, Until: blob.ThrottleUntil, Status: blob.ThrottleStatus}
	}
	return st
}

func (b *Broker) load(provider string) (secretBlob, error) {
	raw, err := b.secrets.Get(secretService, provider)
	if err != nil {
		return secretBlob{}, err
	}
	var blob secretBlob
	if err := json.Unmarshal([]byte(raw), &blob); err != nil {
		return secretBlob{}, err
	}
	return blob, nil
}

func (b *Broker) put(provider string, blob secretBlob) error {
	raw, err := json.Marshal(blob)
	if err != nil {
		return err
	}
	return b.secrets.Set(secretService, provider, string(raw))
}

func (b *Broker) noteRefresh(provider string, ok bool, detail string) error {
	blob, err := b.load(provider)
	if err != nil {
		return err
	}
	blob.RefreshOK = ok
	blob.RefreshDetail = detail
	return b.put(provider, blob)
}

func (b *Broker) noteThrottle(provider string, until time.Time, status int) error {
	blob, err := b.load(provider)
	if err != nil {
		return err
	}
	blob.ThrottleUntil = until
	blob.ThrottleStatus = status
	return b.put(provider, blob)
}

func (b *Broker) authURL(provider string) string {
	if provider == "gmail" && b.GoogleAuth != "" {
		return b.GoogleAuth
	}
	if provider == "graph" && b.GraphAuth != "" {
		return b.GraphAuth
	}
	if provider == "gmail" {
		return "https://accounts.google.com/o/oauth2/v2/auth"
	}
	return "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize"
}

func (b *Broker) tokenURL(provider string) string {
	if provider == "gmail" && b.GoogleToken != "" {
		return b.GoogleToken
	}
	if provider == "graph" && b.GraphToken != "" {
		return b.GraphToken
	}
	if provider == "gmail" {
		return "https://oauth2.googleapis.com/token"
	}
	return "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"
}

func baseScopes(provider string, calendar bool) []string {
	if provider == "gmail" {
		return []string{gmail.ScopeRead, gmail.ScopeCompose}
	}
	scopes := []string{graph.ScopeUser, graph.ScopeReadWrite, graph.ScopeOffline}
	if calendar {
		scopes = append(scopes, graph.ScopeCalendar)
	}
	return scopes
}

func sendScope(provider string) string {
	if provider == "gmail" {
		return gmail.ScopeSend
	}
	return graph.ScopeSend
}

func oauthExtra(provider string, incremental bool) url.Values {
	q := url.Values{}
	if provider == "gmail" {
		q.Set("access_type", "offline")
		q.Set("prompt", "consent")
		if incremental {
			q.Set("include_granted_scopes", "true")
		}
		return q
	}
	q.Set("prompt", "select_account")
	return q
}

func redirectHost(provider string) string {
	if provider == "graph" {
		return "localhost"
	}
	return "127.0.0.1"
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

func label(provider string) string {
	switch provider {
	case "gmail":
		return "Gmail"
	case "graph":
		return "Outlook"
	case "fake":
		return "Demo mailbox"
	default:
		return provider
	}
}

func brokerName(provider string) string {
	if provider == "graph" {
		return graph.BrokerName()
	}
	if provider == "gmail" {
		return "system-browser"
	}
	return "demo"
}

// Connected is used by doctor.
func (b *Broker) connected(provider string) bool { return b.Status(provider).Connected }
