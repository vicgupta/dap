package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Host supervises plugin processes. Each plugin is one process.
type Host struct {
	mu      sync.Mutex
	plugins map[string]*proc
}

// New returns an empty host.
func New() *Host {
	return &Host{plugins: map[string]*proc{}}
}

// Options describes a process to launch.
type Options struct {
	Name    string
	Dir     string
	Argv    []string
	Env     []string
	DataDir string
}

type proc struct {
	name    string
	argv    []string
	dir     string
	env     []string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	mu      sync.Mutex
	writeMu sync.Mutex
	nextID  int
	pending map[int]chan rpcResponse
	exited  chan struct{}
	stderr  *tail
	iface   string
	done    sync.Once
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type tail struct {
	mu sync.Mutex
	b  []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > 8192 {
		t.b = append([]byte(nil), t.b[len(t.b)-4096:]...)
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}

// Start launches the process. The caller sends initialize afterwards.
func (h *Host) Start(opts Options) error {
	if len(opts.Argv) == 0 {
		return errors.New("plugin has no entrypoint")
	}
	h.mu.Lock()
	if old, ok := h.plugins[opts.Name]; ok {
		h.mu.Unlock()
		old.stop()
		h.mu.Lock()
	}
	h.mu.Unlock()

	cmd := exec.Command(opts.Argv[0], opts.Argv[1:]...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	setProcGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &tail{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", opts.Name, err)
	}
	p := &proc{
		name:    opts.Name,
		argv:    opts.Argv,
		dir:     opts.Dir,
		env:     opts.Env,
		cmd:     cmd,
		stdin:   stdin,
		pending: map[int]chan rpcResponse{},
		exited:  make(chan struct{}),
		stderr:  stderr,
		iface:   "dap.plugin.v1",
	}
	go p.readLoop(stdout)
	go func() {
		_ = cmd.Wait()
		p.finish()
	}()
	h.mu.Lock()
	h.plugins[opts.Name] = p
	h.mu.Unlock()
	return nil
}

func (p *proc) finish() {
	p.done.Do(func() {
		close(p.exited)
		p.mu.Lock()
		for id, ch := range p.pending {
			close(ch)
			delete(p.pending, id)
		}
		p.mu.Unlock()
	})
}

func (p *proc) readLoop(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			p.stderr.Write([]byte("non-json stdout: " + string(line) + "\n"))
			continue
		}
		p.mu.Lock()
		ch := p.pending[resp.ID]
		delete(p.pending, resp.ID)
		p.mu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
}

// Call invokes one method and decodes the result into out (which may be nil).
func (h *Host) Call(ctx context.Context, name, method string, params any, out any) error {
	h.mu.Lock()
	p := h.plugins[name]
	h.mu.Unlock()
	if p == nil {
		return fmt.Errorf("plugin %s is not running", name)
	}
	return p.call(ctx, method, params, out)
}

func (p *proc) call(ctx context.Context, method string, params any, out any) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	p.mu.Lock()
	select {
	case <-p.exited:
		p.mu.Unlock()
		return fmt.Errorf("plugin %s exited", p.name)
	default:
	}
	p.nextID++
	id := p.nextID
	ch := make(chan rpcResponse, 1)
	p.pending[id] = ch
	p.mu.Unlock()

	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := p.stdin.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("plugin %s: %w", p.name, err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.exited:
		return fmt.Errorf("plugin %s exited: %s", p.name, strings.TrimSpace(p.stderr.String()))
	case resp, ok := <-ch:
		if !ok {
			return fmt.Errorf("plugin %s exited", p.name)
		}
		if resp.Error != nil {
			return fmt.Errorf("%s", resp.Error.Message)
		}
		if out != nil && len(resp.Result) > 0 && string(resp.Result) != "null" {
			if err := json.Unmarshal(resp.Result, out); err != nil {
				return fmt.Errorf("decode %s result: %w", method, err)
			}
		}
		return nil
	}
}

// Stop terminates one plugin.
func (h *Host) Stop(name string) {
	h.mu.Lock()
	p := h.plugins[name]
	delete(h.plugins, name)
	h.mu.Unlock()
	if p != nil {
		p.stop()
	}
}

// StopAll terminates every plugin.
func (h *Host) StopAll() {
	h.mu.Lock()
	all := h.plugins
	h.plugins = map[string]*proc{}
	h.mu.Unlock()
	for _, p := range all {
		p.stop()
	}
}

func (p *proc) stop() {
	_ = p.stdin.Close()
	termProc(p.cmd)
	select {
	case <-p.exited:
	case <-time.After(2 * time.Second):
		killProc(p.cmd)
		<-p.exited
	}
}

// Running reports whether the process is still up.
func (h *Host) Running(name string) bool {
	h.mu.Lock()
	p := h.plugins[name]
	h.mu.Unlock()
	if p == nil || p.cmd.Process == nil {
		return false
	}
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

// Stderr returns the recent stderr tail.
func (h *Host) Stderr(name string) string {
	h.mu.Lock()
	p := h.plugins[name]
	h.mu.Unlock()
	if p == nil || p.stderr == nil {
		return ""
	}
	return strings.TrimSpace(p.stderr.String())
}

// SetInterface records the negotiated interface id.
func (h *Host) SetInterface(name, id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p := h.plugins[name]; p != nil {
		p.iface = id
	}
}

// Interface returns the negotiated interface id.
func (h *Host) Interface(name string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p := h.plugins[name]; p != nil && p.iface != "" {
		return p.iface
	}
	return "dap.plugin.v1"
}

// PluginEnv builds a small environment for a plugin process.
func PluginEnv(extra ...string) []string {
	keep := []string{"PATH", "HOME", "USER", "LOGNAME", "LANG", "LC_ALL", "LC_CTYPE", "TZ", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "LOCALAPPDATA", "USERPROFILE"}
	env := []string{"PYTHONUNBUFFERED=1", "PYTHONIOENCODING=utf-8"}
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	env = append(env, extra...)
	return env
}
