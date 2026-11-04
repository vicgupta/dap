// Package secrets stores tokens for later plugins.
// Secrets live in a mode-0600 file in the data directory. DAP does not write
// the OS keychain, Credential Manager, or Secret Service.
package secrets

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Keeper stores a secret under a service and account.
// Plugin secrets use a service of "dap.plugin.<name>" so one plugin cannot read another's.
type Keeper interface {
	Backend() string
	Set(service, account, secret string) error
	Get(service, account string) (string, error)
	Delete(service, account string) error
}

// ErrNotFound is returned when the account has no secret.
var ErrNotFound = errors.New("secret not found")

type fileKeeper struct {
	path string
	mu   sync.Mutex
}

// NewFile returns the file backend. The directory is created at 0700 and the file at 0600.
func NewFile(dir string) (Keeper, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "secrets.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			return nil, err
		}
	}
	if err := tightenSecret(path); err != nil {
		return nil, err
	}
	return &fileKeeper{path: path}, nil
}

func (f *fileKeeper) Backend() string { return "file" }

func (f *fileKeeper) Set(service, account, secret string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.read()
	if err != nil {
		return err
	}
	m[key(service, account)] = secret
	return f.write(m)
}

func (f *fileKeeper) Get(service, account string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.read()
	if err != nil {
		return "", err
	}
	v, ok := m[key(service, account)]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *fileKeeper) Delete(service, account string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.read()
	if err != nil {
		return err
	}
	delete(m, key(service, account))
	return f.write(m)
}

func key(service, account string) string { return service + "\x00" + account }

func (f *fileKeeper) read() (map[string]string, error) {
	b, err := os.ReadFile(f.path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if len(b) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (f *fileKeeper) write(m map[string]string) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := tightenSecret(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, f.path); err != nil {
		return err
	}
	return tightenSecret(f.path)
}

// NewAuto stores secrets in the data directory. DAP_SECRETS is ignored so a
// process cannot be pointed at the OS keychain.
func NewAuto(dir string) (Keeper, error) {
	return NewFile(dir)
}
