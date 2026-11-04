package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// TurnLimit is how long one turn may run before DAP stops the CLI.
	TurnLimit = 10 * time.Minute
	stderrCap = 8 << 10
)

// Runner starts CLI processes and stops them on cancel and on shutdown.
type Runner struct {
	// LookPath resolves a binary. Tests point it at a fake CLI.
	LookPath func(string) (string, error)
	// Env, when set, replaces the sanitized environment. Tests use it.
	// Production leaves it nil so the child does not inherit API keys.
	Env func() []string

	mu     sync.Mutex
	active map[string]*live
}

type live struct {
	cancel context.CancelFunc
	cmd    *exec.Cmd
}

// Run is one running turn.
type Run struct {
	Events <-chan Event
	err    chan error
	cancel context.CancelFunc
}

// Cancel stops the CLI process group.
func (r *Run) Cancel() {
	if r != nil && r.cancel != nil {
		r.cancel()
	}
}

// Wait blocks until the process has exited and the event channel is closed.
func (r *Run) Wait() error {
	if r == nil {
		return nil
	}
	return <-r.err
}

// ErrBusy is returned when this session already has a turn running.
var ErrBusy = errors.New("a chat turn is already running")

// Start launches the provider CLI with its working directory set to turn.WorkDir.
func (r *Runner) Start(ctx context.Context, sessionID string, p Provider, turn Turn) (*Run, error) {
	if r == nil {
		r = &Runner{}
	}
	if strings.TrimSpace(turn.WorkDir) == "" {
		return nil, errors.New("chat working directory is empty")
	}
	look := r.LookPath
	if look == nil {
		look = exec.LookPath
	}
	bin, err := look(p.Binary())
	if err != nil {
		return nil, err
	}
	args, err := p.Args(turn)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.active == nil {
		r.active = map[string]*live{}
	}
	if _, ok := r.active[sessionID]; ok {
		r.mu.Unlock()
		return nil, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, TurnLimit)
	cmd := exec.Command(bin, args...)
	cmd.Dir = turn.WorkDir
	cmd.Env = r.environ()
	cmd.Stdin = nil
	setProcGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		r.mu.Unlock()
		return nil, err
	}
	var errBuf tailWriter
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		cancel()
		r.mu.Unlock()
		return nil, err
	}
	r.active[sessionID] = &live{cancel: cancel, cmd: cmd}
	r.mu.Unlock()

	events := make(chan Event, 32)
	errCh := make(chan error, 1)
	run := &Run{Events: events, err: errCh, cancel: cancel}
	go func() {
		defer close(events)
		waitErr := readStream(ctx, p, stdout, events, &errBuf, cmd)
		r.mu.Lock()
		if cur, ok := r.active[sessionID]; ok && cur.cmd == cmd {
			delete(r.active, sessionID)
		}
		r.mu.Unlock()
		cancel()
		errCh <- waitErr
	}()
	go func() {
		<-ctx.Done()
		termProc(cmd)
		time.Sleep(150 * time.Millisecond)
		if cmd.ProcessState == nil {
			killProc(cmd)
		}
	}()
	return run, nil
}

// Stop cancels one session's turn.
func (r *Runner) Stop(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	lv := r.active[id]
	r.mu.Unlock()
	stopLive(lv)
}

// StopAll kills every running turn. Shutdown, quit, and Ctrl+C call it.
func (r *Runner) StopAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	all := r.active
	r.active = map[string]*live{}
	r.mu.Unlock()
	for _, lv := range all {
		stopLive(lv)
	}
}

func stopLive(lv *live) {
	if lv == nil {
		return
	}
	if lv.cancel != nil {
		lv.cancel()
	}
	if lv.cmd != nil {
		termProc(lv.cmd)
		killProc(lv.cmd)
	}
}

// Running reports whether any turn is still in progress.
func (r *Runner) Running() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.active) > 0
}

func (r *Runner) environ() []string {
	if r != nil && r.Env != nil {
		return r.Env()
	}
	return safeEnv()
}

// BaseEnv is the parent environment passed to claude, codex, and opencode.
// HOME is included when set. Callers overlay temp and cache paths.
func BaseEnv() []string { return safeEnv() }

func safeEnv() []string {
	allow := []string{
		"PATH", "HOME", "USER", "LOGNAME", "LANG", "LC_ALL", "LC_CTYPE", "TZ",
		"TMPDIR", "TMP", "TEMP", "SYSTEMROOT", "WINDIR", "USERPROFILE",
		"LOCALAPPDATA", "APPDATA", "HOMEDRIVE", "HOMEPATH",
		"CODEX_HOME", "CLAUDE_CONFIG_DIR",
	}
	out := make([]string, 0, len(allow))
	for _, k := range allow {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func readStream(ctx context.Context, p Provider, stdout io.Reader, events chan<- Event, errBuf *tailWriter, cmd *exec.Cmd) error {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	sawDelta := false
	var fullText bool
	done := false
	for sc.Scan() {
		if ctx.Err() != nil {
			break
		}
		evs, err := p.ParseLine(sc.Bytes())
		if err != nil {
			continue
		}
		for _, ev := range evs {
			if ev.Kind == EventText && ev.Delta {
				sawDelta = true
			}
			if ev.Kind == EventText && !ev.Delta {
				if sawDelta {
					// The result repeats text we already streamed. Keep proposals
					// that arrived as their own events; drop the duplicate body.
					continue
				}
				fullText = true
			}
			if ev.Kind == EventDone {
				done = true
			}
			select {
			case events <- ev:
			case <-ctx.Done():
				_ = cmd.Wait()
				return ctx.Err()
			}
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !done && waitErr != nil {
		msg := redact(errBuf.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		events <- Event{Kind: EventError, Text: msg}
	}
	if !done && ctx.Err() == nil {
		events <- Event{Kind: EventDone, Done: true}
	}
	_ = fullText
	return waitErr
}

type tailWriter struct {
	mu  sync.Mutex
	buf []byte
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if len(w.buf) > stderrCap {
		w.buf = append([]byte(nil), w.buf[len(w.buf)-stderrCap:]...)
	}
	return len(p), nil
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}

func redact(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		for _, part := range []string{"sk-", "api_key", "apikey", "authorization", "bearer ", "access_token", "refresh_token", "secret"} {
			if strings.Contains(lower, part) {
				lines[i] = "[redacted]"
				break
			}
		}
	}
	out := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(out) > 500 {
		out = out[len(out)-500:]
	}
	return out
}

// JSONLine is a small helper tests use when building recorded streams.
func JSONLine(v any) []byte {
	b, _ := json.Marshal(v)
	return append(b, '\n')
}
