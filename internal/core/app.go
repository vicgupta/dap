// Package core is the DAP process: plugins, scheduler, inbox, guardrails, and ledger.
package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/lockfile"
	"github.com/vicgupta/dap/internal/mail"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginhost"
	"github.com/vicgupta/dap/internal/secrets"
	"github.com/vicgupta/dap/internal/store"
)

// App is one local DAP instance bound to a data directory.
type App struct {
	DataDir string

	store   *store.Store
	ledger  *ledger.Ledger
	secrets secrets.Keeper
	mail    *mail.Broker
	host    *pluginhost.Host
	lock    *lockfile.Lock

	mu         sync.Mutex
	runMu      map[string]*sync.Mutex
	rt         map[string]*runtime
	checks     []CheckRecord
	orderErr   string
	pluginsUp  bool
	clock      func() time.Time
	schedStop  chan struct{}
	apiLn      net.Listener
	apiSrv     *http.Server
	apiBase    string
	adminToken string
}

type runtime struct {
	Health        model.Health
	Token         string
	UIURL         string
	NeedsApproval bool
	StartError    string
}

// CheckRecord is one recent guardrail decision, newest last.
type CheckRecord struct {
	At         time.Time `json:"at"`
	Plugin     string    `json:"plugin"`
	Permission string    `json:"permission,omitempty"`
	Effects    []string  `json:"effects,omitempty"`
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason"`
	CardID     string    `json:"card_id,omitempty"`
}

// FileConfig is the small on-disk config next to the ledger.
type FileConfig struct {
	Addr string `json:"addr"`
}

// Open locks the data directory and loads state. It does not start plugins.
func Open(dir string) (*App, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	lk, err := lockfile.Acquire(filepath.Join(abs, "dap.lock"))
	if err != nil {
		return nil, fmt.Errorf("%w (%s)", err, abs)
	}
	st, err := store.Open(abs)
	if err != nil {
		_ = lk.Release()
		return nil, err
	}
	led, err := ledger.Open(abs)
	if err != nil {
		_ = lk.Release()
		return nil, err
	}
	sec, err := secrets.NewFile(abs)
	if err != nil {
		_ = lk.Release()
		return nil, err
	}
	// Optional OS keychain. A failure falls back to the file created above
	// when DAP_SECRETS is not forced to "os" — NewAuto does that itself.
	if os.Getenv("DAP_SECRETS") != "file" {
		if auto, err := secrets.NewAuto(abs); err == nil {
			sec = auto
		}
	}
	box, err := mail.New(abs, sec)
	if err != nil {
		_ = lk.Release()
		return nil, err
	}
	return &App{
		DataDir:    abs,
		store:      st,
		ledger:     led,
		secrets:    sec,
		mail:       box,
		host:       pluginhost.New(),
		lock:       lk,
		runMu:      map[string]*sync.Mutex{},
		rt:         map[string]*runtime{},
		adminToken: randomToken(),
	}, nil
}

// Close stops the scheduler, plugins, and plugin API, then releases the lock.
func (a *App) Close() error {
	a.StopScheduler()
	if a.apiSrv != nil {
		_ = a.apiSrv.Close()
	}
	a.host.StopAll()
	if a.lock != nil {
		return a.lock.Release()
	}
	return nil
}

// Start brings up the scoped plugin API, the plugins, and the scheduler.
func (a *App) Start() error {
	if err := a.StartPlugins(); err != nil {
		return err
	}
	a.StartScheduler()
	return nil
}

// AdminToken authenticates the local UI and the da CLI while this process is up.
func (a *App) AdminToken() string { return a.adminToken }

// PluginAPIBase is the loopback root plugins call, without the plugin suffix.
func (a *App) PluginAPIBase() string { return a.apiBase }

// SecretsBackend names the active token store.
func (a *App) SecretsBackend() string {
	if a.secrets == nil {
		return ""
	}
	return a.secrets.Backend()
}

// InitDataDir creates the directory and a default config.
func InitDataDir(dir string) (FileConfig, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return FileConfig{}, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return FileConfig{}, err
	}
	return ensureConfig(abs)
}

func ensureConfig(dir string) (FileConfig, error) {
	cfg := FileConfig{Addr: "127.0.0.1:7391"}
	path := filepath.Join(dir, "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return FileConfig{}, err
		}
		raw, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return FileConfig{}, err
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
			return FileConfig{}, err
		}
		return cfg, nil
	}
	if len(b) > 0 {
		_ = json.Unmarshal(b, &cfg)
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:7391"
	}
	return cfg, nil
}

// LoadFileConfig reads config.json, applying the default address if needed.
func LoadFileConfig(dir string) FileConfig {
	cfg, err := ensureConfig(dir)
	if err != nil {
		return FileConfig{Addr: "127.0.0.1:7391"}
	}
	return cfg
}

func (a *App) now() time.Time {
	a.mu.Lock()
	fn := a.clock
	a.mu.Unlock()
	if fn == nil {
		return time.Now()
	}
	return fn()
}

func (a *App) mutex(name string) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()
	m, ok := a.runMu[name]
	if !ok {
		m = &sync.Mutex{}
		a.runMu[name] = m
	}
	return m
}

func (a *App) isUp() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pluginsUp
}

func (a *App) runtimeOf(name string) runtime {
	a.mu.Lock()
	defer a.mu.Unlock()
	rt := a.rt[name]
	if rt == nil {
		return runtime{}
	}
	return *rt
}

func (a *App) mutateRT(name string, fn func(*runtime)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rt := a.rt[name]
	if rt == nil {
		rt = &runtime{}
		a.rt[name] = rt
	}
	fn(rt)
}

func (a *App) log(e ledger.Entry) {
	if e.TS.IsZero() {
		e.TS = a.now().UTC()
	}
	_, _ = a.ledger.Append(e)
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("token: %w", err))
	}
	return hex.EncodeToString(b[:])
}

// Token returns the in-memory bearer token for a plugin.
func (a *App) Token(plugin string) string {
	return a.runtimeOf(plugin).Token
}

// SetUIURL records the sandboxed origin for a plugin's web UI.
func (a *App) SetUIURL(plugin, url string) {
	a.mutateRT(plugin, func(rt *runtime) { rt.UIURL = url })
}

// SetClock replaces the clock. Tests use this; the server does not.
func (a *App) SetClock(fn func() time.Time) {
	a.mu.Lock()
	a.clock = fn
	a.mu.Unlock()
}
