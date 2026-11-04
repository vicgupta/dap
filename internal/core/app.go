// Package core is the DAP process: plugins, scheduler, inbox, guardrails, and ledger.
package core

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vicgupta/dap/internal/agentrun"
	"github.com/vicgupta/dap/internal/layout"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/lockfile"
	"github.com/vicgupta/dap/internal/mail"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginhost"
	"github.com/vicgupta/dap/internal/rundir"
	"github.com/vicgupta/dap/internal/secrets"
	"github.com/vicgupta/dap/internal/store"
	"github.com/vicgupta/dap/internal/theme"
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

	mu           sync.Mutex
	configMu     sync.Mutex
	runMu        map[string]*sync.Mutex
	rt           map[string]*runtime
	checks       []CheckRecord
	orderErr     string
	pluginsUp    bool
	clock        func() time.Time
	schedStop    chan struct{}
	apiLn        net.Listener
	apiSrv       *http.Server
	apiBase      string
	adminToken   string
	agentJobs    *sync.Map
	stopping     bool
	inflightOps  int
	inflightActs int
	busy         map[string]int
	shutOnce     sync.Once
	shutErr      error
	chatSys      *chatSystem
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
	Addr  string `json:"addr"`
	Theme string `json:"theme,omitempty"`
}

// Open locks the data directory and loads state. It does not start plugins.
func Open(dir string) (*App, error) {
	agentrun.UseHostPath()
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	if err := rundir.Ensure(abs); err != nil {
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
		busy:       map[string]int{},
		adminToken: randomToken(),
	}, nil
}

// Close shuts DAP down and releases the data directory.
// Quit, Ctrl+C, and a normal process exit all use Shutdown.
func (a *App) Close() error {
	return a.Shutdown()
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
	if err := rundir.Ensure(abs); err != nil {
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
		cfg.Theme = theme.System
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
	cfg.Theme = theme.Stored(cfg.Theme)
	return cfg, nil
}

func saveFileConfig(dir string, cfg FileConfig) error {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:7391"
	}
	mode, err := theme.Normalize(cfg.Theme)
	if err != nil {
		return err
	}
	raw, err := readConfigRaw(dir)
	if err != nil {
		return err
	}
	addr, err := json.Marshal(cfg.Addr)
	if err != nil {
		return err
	}
	themeRaw, err := json.Marshal(mode)
	if err != nil {
		return err
	}
	raw["addr"] = addr
	raw["theme"] = themeRaw
	return writeConfigRaw(dir, raw)
}

// Theme is the saved appearance mode: system, light, or dark.
func (a *App) Theme() string {
	return LoadFileConfig(a.DataDir).Theme
}

// SetTheme stores the appearance mode in config.json. Empty is rejected.
func (a *App) SetTheme(v string) (string, error) {
	if strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("theme must be system, light, or dark")
	}
	mode, err := theme.Normalize(v)
	if err != nil {
		return "", err
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	cfg := LoadFileConfig(a.DataDir)
	cfg.Theme = mode
	if err := saveFileConfig(a.DataDir, cfg); err != nil {
		return "", err
	}
	return mode, nil
}

// LoadFileConfig reads config.json, applying the default address if needed.
func LoadFileConfig(dir string) FileConfig {
	cfg, err := ensureConfig(dir)
	if err != nil {
		return FileConfig{Addr: "127.0.0.1:7391", Theme: theme.System}
	}
	return cfg
}

// Layout is the card layout saved in config.json. Missing or invalid
// text becomes the defaults. Other keys in the file are left alone.
func (a *App) Layout() layout.Settings {
	raw, err := readConfigRaw(a.DataDir)
	if err != nil || len(raw["layout"]) == 0 {
		return layout.Default()
	}
	var in layout.Settings
	if err := json.Unmarshal(raw["layout"], &in); err != nil {
		return layout.Default()
	}
	return layout.Stored(in)
}

// SetLayout validates and stores the card layout. Keys other than "layout"
// (for example a theme saved by another change) stay in config.json.
func (a *App) SetLayout(in layout.Settings) (layout.Settings, error) {
	next, err := layout.Normalize(in)
	if err != nil {
		return layout.Settings{}, err
	}
	body, err := json.Marshal(next)
	if err != nil {
		return layout.Settings{}, err
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	raw, err := readConfigRaw(a.DataDir)
	if err != nil {
		return layout.Settings{}, err
	}
	raw["layout"] = body
	if err := writeConfigRaw(a.DataDir, raw); err != nil {
		return layout.Settings{}, err
	}
	return next, nil
}

func readConfigRaw(dir string) (map[string]json.RawMessage, error) {
	path := filepath.Join(dir, "config.json")
	out := map[string]json.RawMessage{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			out["addr"] = json.RawMessage(`"127.0.0.1:7391"`)
			return out, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(b)) > 0 {
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, err
		}
	}
	if len(out["addr"]) == 0 || string(out["addr"]) == `""` {
		out["addr"] = json.RawMessage(`"127.0.0.1:7391"`)
	}
	return out, nil
}

func writeConfigRaw(dir string, raw map[string]json.RawMessage) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		if k != "addr" && k != "theme" && k != "layout" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	ordered := make([]string, 0, len(keys)+3)
	for _, k := range []string{"addr", "theme", "layout"} {
		if _, ok := raw[k]; ok {
			ordered = append(ordered, k)
		}
	}
	ordered = append(ordered, keys...)
	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, k := range ordered {
		key, err := json.Marshal(k)
		if err != nil {
			return err
		}
		val := bytes.TrimSpace(raw[k])
		if len(val) == 0 {
			val = []byte("null")
		}
		buf.WriteString("  ")
		buf.Write(key)
		buf.WriteString(": ")
		if val[0] == '{' || val[0] == '[' {
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, val, "  ", "  "); err == nil {
				val = pretty.Bytes()
			}
		}
		buf.Write(val)
		if i < len(ordered)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
	path := filepath.Join(dir, "config.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
