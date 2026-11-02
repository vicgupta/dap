package composio

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/vicgupta/dap/internal/secrets"
)

const (
	secretService = "dap.composio"
	accountKey    = "api_key"
)

// Connection is what DAP stores per toolkit. It is not a provider token.
type Connection struct {
	ConnectionID string `json:"connection_id"`
	AuthConfigID string `json:"auth_config_id,omitempty"`
	Account      string `json:"account,omitempty"`
	Status       string `json:"status,omitempty"`
}

// Service is the connectivity layer plugins reach through the host.
type Service struct {
	dir     string
	secrets secrets.Keeper
	base    string
	open    func(string) error
}

// New binds the layer to a data directory and the secret keeper.
func New(dir string, keeper secrets.Keeper, open func(string) error) (*Service, error) {
	if err := os.MkdirAll(filepath.Join(dir, "composio"), 0o700); err != nil {
		return nil, err
	}
	return &Service{dir: dir, secrets: keeper, open: open}, nil
}

// SetBase points the client at a stand-in for tests.
func (s *Service) SetBase(base string) { s.base = strings.TrimRight(base, "/") }

// SetOpener replaces the browser opener.
func (s *Service) SetOpener(fn func(string) error) { s.open = fn }

// Registry is builtin toolkits plus composio/toolkits.json.
func (s *Service) Registry() (Registry, error) {
	return Load(filepath.Join(s.dir, "composio", "toolkits.json"))
}

// HasKey reports whether an API key is stored. It does not return the key.
func (s *Service) HasKey() bool {
	v, err := s.secrets.Get(secretService, accountKey)
	return err == nil && strings.TrimSpace(v) != ""
}

// SetKey stores the Composio API key in the secret keeper.
// An empty key deletes it. The value is never written to config or the ledger.
func (s *Service) SetKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return s.secrets.Delete(secretService, accountKey)
	}
	if strings.ContainsAny(key, "\r\n") || len(key) > 512 {
		return errors.New("that does not look like a composio api key")
	}
	return s.secrets.Set(secretService, accountKey, key)
}

func (s *Service) key() (string, error) {
	v, err := s.secrets.Get(secretService, accountKey)
	if err != nil || strings.TrimSpace(v) == "" {
		return "", errors.New("save a Composio API key on Connections first")
	}
	return strings.TrimSpace(v), nil
}

// Get returns the stored connection. The bool is false when it is missing.
func (s *Service) Get(id string) (Connection, bool) {
	raw, err := s.secrets.Get(secretService, connAccount(s.canon(id)))
	if err != nil {
		return Connection{}, false
	}
	var c Connection
	if json.Unmarshal([]byte(raw), &c) != nil || c.ConnectionID == "" {
		return Connection{}, false
	}
	return c, true
}

// Connect opens Composio's hosted sign-in and stores the connection id.
func (s *Service) Connect(ctx context.Context, id string) (Connection, error) {
	reg, err := s.Registry()
	if err != nil {
		return Connection{}, err
	}
	tk, ok := reg.ByID(id)
	if !ok {
		return Connection{}, errors.New("unknown connection")
	}
	key, err := s.key()
	if err != nil {
		return Connection{}, err
	}
	client := s.client(key)
	authID, err := client.EnsureAuthConfig(ctx, tk.Slug)
	if err != nil {
		return Connection{}, err
	}
	link, err := client.LinkAccount(ctx, authID)
	if err != nil {
		return Connection{}, err
	}
	if s.open != nil {
		if err := s.open(link.RedirectURL); err != nil {
			return Connection{}, err
		}
	}
	acc, err := client.WaitActive(ctx, link.AccountID)
	if err != nil {
		return Connection{}, err
	}
	conn := Connection{ConnectionID: acc.ID, AuthConfigID: authID, Status: acc.Status}
	if tool := tk.Actions["profile"]; tool != "" {
		raw, err := client.Execute(ctx, tool, acc.ID, map[string]any{"user_id": "me"})
		if err == nil {
			conn.Account = Email(raw)
		}
	}
	if err := s.put(tk.ID, conn); err != nil {
		return Connection{}, err
	}
	return conn, nil
}

// Revoke deletes the remote connection and the local connection id.
// The API key is kept so other toolkits stay connected.
func (s *Service) Revoke(ctx context.Context, id string) error {
	canon := s.canon(id)
	if conn, ok := s.Get(canon); ok {
		if key, err := s.key(); err == nil {
			_ = s.client(key).DeleteAccount(ctx, conn.ConnectionID)
		}
	}
	err := s.secrets.Delete(secretService, connAccount(canon))
	if errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	return err
}

// Probe checks that Composio still has an active connection.
// The detail is safe to show. It never includes the connection id or the API key.
func (s *Service) Probe(ctx context.Context, id string) (bool, string) {
	canon := s.canon(id)
	conn, ok := s.Get(canon)
	if !ok {
		return false, "not connected"
	}
	key, err := s.key()
	if err != nil {
		return false, "Composio API key is missing"
	}
	acc, err := s.client(key).Account(ctx, conn.ConnectionID)
	if err != nil {
		return false, "could not reach Composio"
	}
	if !strings.EqualFold(acc.Status, "ACTIVE") {
		conn.Status = acc.Status
		_ = s.put(canon, conn)
		return false, "Composio connection is " + strings.ToLower(acc.Status)
	}
	conn.Status = "ACTIVE"
	_ = s.put(canon, conn)
	return true, "Composio holds the token and refreshes it"
}

// Execute runs a tool on a connected toolkit. Callers enforce permissions.
func (s *Service) Execute(ctx context.Context, toolkit, tool string, args map[string]any) (json.RawMessage, error) {
	conn, ok := s.Get(toolkit)
	if !ok {
		return nil, errors.New(s.canon(toolkit) + " is not connected")
	}
	key, err := s.key()
	if err != nil {
		return nil, err
	}
	return s.client(key).Execute(ctx, tool, conn.ConnectionID, args)
}

func (s *Service) client(key string) *Client {
	return &Client{Base: s.base, Key: key}
}

func (s *Service) put(id string, conn Connection) error {
	raw, err := json.Marshal(conn)
	if err != nil {
		return err
	}
	return s.secrets.Set(secretService, connAccount(id), string(raw))
}

func (s *Service) canon(id string) string {
	reg, err := s.Registry()
	if err != nil {
		return Canonical(id)
	}
	if tk, ok := reg.ByID(id); ok {
		return tk.ID
	}
	return Canonical(id)
}

func connAccount(id string) string { return "conn." + id }
