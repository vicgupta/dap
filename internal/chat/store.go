package chat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store is the on-disk session history under <data>/chat.
type Store struct {
	dir string
	mu  sync.Mutex
}

// OpenStore creates the chat directory. Sessions are files, mode 0600.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "work"), 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Dir is the chat root, not the data directory. CLI working directories
// live under Dir/work so a turn cannot read secrets.json or run.json.
func (s *Store) Dir() string { return s.dir }

// WorkDir is the per-session directory the CLI uses as its cwd.
func (s *Store) WorkDir(id string) (string, error) {
	if !validID(id) {
		return "", errors.New("invalid session id")
	}
	return filepath.Join(s.dir, "work", id), nil
}

// Create starts an empty session and its workspace, including AGENTS.md.
func (s *Store) Create(provider string, now time.Time) (Session, error) {
	id, err := NormalizeProvider(provider)
	if err != nil {
		return Session{}, err
	}
	now = now.UTC()
	sess := Session{
		ID:        newID("s"),
		Title:     "New chat",
		Provider:  id,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.Save(sess); err != nil {
		return Session{}, err
	}
	if err := s.prepareWork(sess.ID); err != nil {
		return Session{}, err
	}
	return sess, nil
}

func (s *Store) prepareWork(id string) error {
	dir, err := s.WorkDir(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "AGENTS.md")
	return os.WriteFile(path, []byte(Rules), 0o600)
}

// RulesFile is the AGENTS.md path passed to Claude's append-system-prompt flag.
func (s *Store) RulesFile(id string) (string, error) {
	dir, err := s.WorkDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "AGENTS.md"), nil
}

// Save writes the session. The caller owns the struct; it is copied.
func (s *Store) Save(sess Session) error {
	if !validID(sess.ID) {
		return errors.New("invalid session id")
	}
	raw, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	path := s.path(sess.ID)
	tmp := path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads one session.
func (s *Store) Load(id string) (Session, error) {
	if !validID(id) {
		return Session{}, errors.New("invalid session id")
	}
	b, err := os.ReadFile(s.path(id))
	if err != nil {
		if os.IsNotExist(err) {
			return Session{}, os.ErrNotExist
		}
		return Session{}, err
	}
	var sess Session
	if err := json.Unmarshal(b, &sess); err != nil {
		return Session{}, err
	}
	return sess, nil
}

// List returns history newest first, without message bodies.
func (s *Store) List() ([]Info, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "sessions"))
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		sess, err := s.Load(id)
		if err != nil {
			continue
		}
		out = append(out, Info{
			ID: sess.ID, Title: sess.Title, Provider: sess.Provider, UpdatedAt: sess.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if len(out) > 100 {
		out = out[:100]
	}
	if out == nil {
		out = []Info{}
	}
	return out, nil
}

func (s *Store) path(id string) string {
	return filepath.Join(s.dir, "sessions", id+".json")
}

func validID(id string) bool {
	if len(id) < 3 || len(id) > 80 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-':
		default:
			return false
		}
	}
	return !strings.Contains(id, "..")
}

// Lock serializes turns and saves for one process.
func (s *Store) Lock()   { s.mu.Lock() }
func (s *Store) Unlock() { s.mu.Unlock() }
