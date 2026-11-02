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

	"github.com/vicgupta/dap/internal/composio"
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
	DraftResult = mailbox.DraftResult
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
	// ComposioAPI overrides the hosted API. Tests point it at a local server.
	ComposioAPI string

	connect *composio.Service
}

type publicCfg struct {
	ClientID string `json:"client_id,omitempty"`
	Calendar bool   `json:"calendar,omitempty"`
	Mode     string `json:"mode,omitempty"`
}

// Configure changes connection setup. Nil fields are left alone.
// The API key is stored in the secret keeper, never in public.json.
type Configure struct {
	APIKey   *string
	ClientID *string
	Calendar *bool
	Mode     *string
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
	cio, err := composio.New(dir, keeper, desktop.Open)
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
		connect: cio,
	}
	_ = b.loadPublic()
	return b, nil
}

// SetClock overrides the clock.
func (b *Broker) SetClock(now func() time.Time) { b.now = now }

// SetOpener overrides how the sign-in window is shown.
func (b *Broker) SetOpener(fn func(string) error) {
	b.open = fn
	if b.connect != nil {
		b.connect.SetOpener(fn)
	}
}

func (b *Broker) prepare() {
	if b.connect != nil && b.ComposioAPI != "" {
		b.connect.SetBase(b.ComposioAPI)
	}
}

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
	return b.Configure(provider, Configure{ClientID: &clientID, Calendar: &calendar})
}

// Configure stores non-secret connection choices. The API key goes to the secret keeper.
func (b *Broker) Configure(provider string, cfg Configure) error {
	if cfg.APIKey != nil {
		if err := b.connect.SetKey(*cfg.APIKey); err != nil {
			return err
		}
	}
	if provider == "" || provider == "composio" {
		return nil
	}
	provider = canonProvider(provider)
	if provider != "gmail" && provider != "outlook" {
		if cfg.Mode != nil && strings.TrimSpace(*cfg.Mode) == "direct" {
			return errors.New("only Outlook can fall back to a direct connection")
		}
		return nil
	}
	if cfg.Mode != nil {
		mode := strings.TrimSpace(*cfg.Mode)
		if mode != "" && mode != "composio" && mode != "direct" {
			return errors.New("mode must be composio or direct")
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := publicKey(provider)
	cur := b.public[key]
	if cfg.ClientID != nil {
		cur.ClientID = strings.TrimSpace(*cfg.ClientID)
	}
	if cfg.Calendar != nil {
		cur.Calendar = *cfg.Calendar
	}
	if cfg.Mode != nil {
		cur.Mode = strings.TrimSpace(*cfg.Mode)
	}
	b.public[key] = cur
	return b.savePublicLocked()
}

// Connect starts sign-in. Gmail and Outlook use Composio unless a direct path is selected.
func (b *Broker) Connect(ctx context.Context, provider string) error {
	b.prepare()
	switch provider {
	case "fake":
		return b.fake.Connect()
	case "gmail":
		if b.useDirect("gmail") {
			return b.authorize(ctx, "gmail", false)
		}
		return b.connectComposio(ctx, "gmail")
	case "graph", "outlook":
		if b.useDirect("outlook") {
			return b.authorize(ctx, "graph", false)
		}
		return b.connectComposio(ctx, "outlook")
	default:
		return b.connectComposio(ctx, provider)
	}
}

// Disconnect drops a connection. A public client id, if any, is kept.
// The Composio API key is kept so other toolkits stay connected.
func (b *Broker) Disconnect(provider string) error {
	switch provider {
	case "fake":
		return b.fake.Disconnect()
	default:
		b.prepare()
		canon := canonProvider(provider)
		if canon == "gmail" || canon == "outlook" {
			_ = ignoreMissing(b.secrets.Delete(secretService, secretAccount(canon)))
		}
		if b.connect == nil {
			return errors.New("unknown mail provider")
		}
		return b.connect.Revoke(context.Background(), canon)
	}
}

// Status reports one provider. Tokens, the API key, and connection ids are not included.
func (b *Broker) Status(provider string) Status {
	if provider == "fake" {
		return b.status("fake")
	}
	want := canonProvider(provider)
	for _, row := range b.composioRows() {
		if row.Provider == want {
			return row
		}
	}
	return b.status(provider)
}

// Statuses reports every toolkit, then the demo mailbox.
func (b *Broker) Statuses() []Status {
	return append(b.composioRows(), b.status("fake"))
}

// Check refreshes a live token, or asks Composio if that connection is still active.
func (b *Broker) Check(ctx context.Context, provider string) Status {
	b.prepare()
	switch canonProvider(provider) {
	case "gmail", "outlook":
		if b.useDirect(provider) {
			account := secretAccount(canonProvider(provider))
			blob, _ := b.load(account)
			if blob.Connected {
				if _, err := b.ensureFresh(ctx, account); err != nil {
					_ = b.noteRefresh(account, false, "refresh failed")
				} else {
					_ = b.noteRefresh(account, true, "token present, refresh works")
				}
			}
		} else if b.connect != nil {
			_, _ = b.connect.Probe(ctx, canonProvider(provider))
		}
	}
	return b.Status(provider)
}

// Threads lists conversations for a connected mailbox.
func (b *Broker) Threads(ctx context.Context, provider string) ([]Thread, error) {
	b.prepare()
	switch provider {
	case "fake":
		return b.fake.Threads(b.now()), nil
	case "gmail":
		if !b.useDirect("gmail") {
			return b.composioThreads(ctx, "gmail")
		}
		blob, err := b.ensureFresh(ctx, "gmail")
		if err != nil {
			return nil, err
		}
		return (&gmail.Client{API: b.GmailAPI}).ListThreads(ctx, blob.AccessToken, blob.Account)
	case "graph", "outlook":
		if !b.useDirect("outlook") {
			return b.composioThreads(ctx, "outlook")
		}
		blob, err := b.ensureFresh(ctx, "graph")
		if err != nil {
			return nil, err
		}
		client := &graph.Client{API: b.GraphAPI, OnThrottle: func(th httpx.Throttle) {
			_ = b.noteThrottle("graph", th.Until, th.Status)
		}}
		threads, cur, err := client.ListThreads(ctx, blob.AccessToken, blob.Account, graph.DeltaCursor{Inbox: blob.DeltaInbox, Sent: blob.DeltaSent})
		if err != nil {
			return nil, err
		}
		blob.DeltaInbox, blob.DeltaSent = cur.Inbox, cur.Sent
		_ = b.put("graph", blob)
		for i := range threads {
			threads[i].Provider = "outlook"
		}
		return threads, nil
	default:
		if !b.useDirect(provider) {
			return b.composioThreads(ctx, provider)
		}
		return nil, errors.New("unknown mail provider")
	}
}

// Draft creates a provider draft. Direct Gmail and Graph keep the draft on the card.
func (b *Broker) Draft(ctx context.Context, req SendRequest) (DraftResult, error) {
	b.prepare()
	switch canonProvider(req.Provider) {
	case "fake":
		return DraftResult{Provider: "fake", Local: true}, nil
	case "gmail":
		if b.useDirect("gmail") {
			return DraftResult{Provider: "gmail", Local: true}, nil
		}
		return b.composioDraft(ctx, req)
	case "outlook":
		if b.useDirect("outlook") {
			return DraftResult{Provider: "outlook", Local: true}, nil
		}
		return b.composioDraft(ctx, req)
	default:
		return b.composioDraft(ctx, req)
	}
}

// Send delivers one message. The first direct send asks for the send scope if it is missing.
// Composio sends still have to be called only after an approved card. The host gate does that.
func (b *Broker) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	b.prepare()
	switch req.Provider {
	case "fake":
		return b.fake.Send(req, b.now())
	case "gmail":
		if !b.useDirect("gmail") {
			return b.composioSend(ctx, req)
		}
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
	case "graph", "outlook":
		if !b.useDirect("outlook") {
			return b.composioSend(ctx, req)
		}
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
		res, err := client.Send(ctx, blob.AccessToken, req)
		if err != nil {
			return SendResult{}, err
		}
		res.Provider = "outlook"
		return res, nil
	default:
		if canonProvider(req.Provider) != "fake" {
			return b.composioSend(ctx, req)
		}
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
	case "graph", "outlook":
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
