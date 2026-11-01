// Package cli is the `da` command. Every command reads and writes JSON.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/vicgupta/dap/internal/api"
	"github.com/vicgupta/dap/internal/core"
	"github.com/vicgupta/dap/internal/desktop"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/runinfo"
)

// Options wires the process streams. Tests pass their own.
type Options struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Run executes one da invocation.
func Run(args []string, opt Options) int {
	if opt.Stdin == nil {
		opt.Stdin = os.Stdin
	}
	if opt.Stdout == nil {
		opt.Stdout = os.Stdout
	}
	if opt.Stderr == nil {
		opt.Stderr = os.Stderr
	}
	data, rest := splitGlobal(args)
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "--help" || rest[0] == "-h" {
		fmt.Fprintln(opt.Stdout, usage)
		return 0
	}
	var (
		result any
		err    error
		code   = 0
	)
	switch rest[0] {
	case "init":
		result, err = cmdInit(data)
	case "serve":
		err = cmdServe(data, rest[1:], opt, false)
	case "run":
		err = cmdServe(data, rest[1:], opt, true)
	case "doctor":
		result, err = cmdDoctor(data)
		if err == nil && !truthyOK(result) {
			write(opt.Stdout, result)
			return 1
		}
	case "plugin":
		result, err = cmdPlugin(data, rest[1:])
	case "context":
		result, err = cmdContext(data, rest[1:])
	case "propose":
		result, err = cmdPropose(data, opt.Stdin)
	case "approved":
		result, err = cmdApproved(data)
	case "check":
		result, err = cmdCheck(data, opt.Stdin)
	case "log":
		result, err = cmdLog(data, opt.Stdin)
	case "outcome":
		result, err = cmdOutcome(data, opt.Stdin)
	case "tile":
		if len(rest) < 2 || rest[1] != "write" {
			err = errors.New("usage: da tile write  (JSON on stdin)")
			break
		}
		result, err = cmdTile(data, opt.Stdin)
	default:
		err = fmt.Errorf("unknown command %q", rest[0])
		code = 2
	}
	if err != nil {
		write(opt.Stderr, map[string]any{"ok": false, "error": err.Error()})
		if code == 0 {
			code = 1
		}
		return code
	}
	if result != nil {
		write(opt.Stdout, result)
	}
	return 0
}

const usage = `da — Dashboard Agent Platform

  da init
  da serve [--addr host:port]
  da run [--addr host:port]
  da doctor
  da plugin install [--approve] <path>
  da plugin list
  da plugin remove <name>
  da plugin approve <name>
  da context [--plugin name]
  da propose            JSON on stdin
  da approved
  da check              JSON on stdin
  da log                list, or append when stdin is JSON
  da outcome            JSON on stdin
  da tile write         JSON on stdin

Global flag: --data <dir>   (default $DAP_HOME or ~/.dap)
Every command prints JSON.`

func splitGlobal(args []string) (string, []string) {
	data := os.Getenv("DAP_HOME")
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--data" && i+1 < len(args):
			i++
			data = args[i]
		case strings.HasPrefix(a, "--data="):
			data = strings.TrimPrefix(a, "--data=")
		default:
			rest = append(rest, a)
		}
	}
	if data == "" {
		data = defaultData()
	}
	return data, rest
}

func defaultData() string {
	home, err := os.UserHomeDir()
	if err != nil {
		if u, err := user.Current(); err == nil {
			home = u.HomeDir
		}
	}
	if home == "" {
		home = "."
	}
	return filepath.Join(home, ".dap")
}

func cmdInit(dir string) (any, error) {
	cfg, err := core.InitDataDir(dir)
	if err != nil {
		return nil, err
	}
	abs, _ := filepath.Abs(dir)
	return map[string]any{"ok": true, "data_dir": abs, "addr": cfg.Addr}, nil
}

func cmdServe(dir string, args []string, opt Options, open bool) error {
	addr := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--addr" && i+1 < len(args) {
			i++
			addr = args[i]
		} else if strings.HasPrefix(args[i], "--addr=") {
			addr = strings.TrimPrefix(args[i], "--addr=")
		} else if args[i] == "--open" {
			open = true
		}
	}
	if _, err := core.InitDataDir(dir); err != nil {
		return err
	}
	if addr == "" {
		addr = core.LoadFileConfig(dir).Addr
	}
	app, err := core.Open(dir)
	if err != nil {
		return err
	}
	defer app.Close()
	if err := app.Start(); err != nil {
		return err
	}
	srv := api.New(app, addr)
	if err := srv.Start(); err != nil {
		return err
	}
	defer srv.Close()
	info := runinfo.Info{PID: os.Getpid(), Addr: srv.Addr, Token: app.AdminToken()}
	if err := runinfo.Write(app.DataDir, info); err != nil {
		return err
	}
	defer runinfo.Remove(app.DataDir)
	write(opt.Stdout, map[string]any{"ok": true, "addr": srv.Addr, "url": srv.URL(), "data_dir": app.DataDir})
	if open {
		if err := desktop.Open(srv.URL()); err != nil {
			fmt.Fprintf(opt.Stderr, "{\"ok\":false,\"error\":%q}\n", "open window: "+err.Error())
		}
	}
	waitForStop()
	return nil
}

type session struct {
	dir  string
	info *runinfo.Info
	app  *core.App
}

func openSession(dir string, start bool) (*session, error) {
	if info, ok := liveServer(dir); ok {
		return &session{dir: dir, info: &info}, nil
	}
	app, err := core.Open(dir)
	if err != nil {
		return nil, err
	}
	if start {
		if err := app.Start(); err != nil {
			_ = app.Close()
			return nil, err
		}
	}
	return &session{dir: dir, app: app}, nil
}

func (s *session) close() {
	if s != nil && s.app != nil {
		_ = s.app.Close()
	}
}

func liveServer(dir string) (runinfo.Info, bool) {
	info, err := runinfo.Read(dir)
	if err != nil {
		return runinfo.Info{}, false
	}
	if !pidAlive(info.PID) {
		_ = runinfo.Remove(dir)
		return runinfo.Info{}, false
	}
	return info, true
}

func (s *session) call(method, path string, in any) (any, error) {
	out, status, err := s.do(method, path, in)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, errorFrom(out, status)
	}
	return out, nil
}

func (s *session) do(method, path string, in any) (any, int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://"+s.info.Addr+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+s.info.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var out any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, resp.StatusCode, err
	}
	return out, resp.StatusCode, nil
}

func errorFrom(out any, status int) error {
	if m, ok := out.(map[string]any); ok {
		if msg, _ := m["error"].(string); msg != "" {
			return errors.New(msg)
		}
		if msg, _ := m["reason"].(string); msg != "" {
			return errors.New(msg)
		}
	}
	return fmt.Errorf("HTTP %d", status)
}

func truthyOK(v any) bool {
	switch t := v.(type) {
	case core.DoctorReport:
		return t.OK
	case map[string]any:
		b, ok := t["ok"].(bool)
		return ok && b
	default:
		return true
	}
}

func cmdDoctor(dir string) (any, error) {
	s, err := openSession(dir, true)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if s.info != nil {
		return s.call(http.MethodGet, "/api/doctor", nil)
	}
	return s.app.Doctor(), nil
}

func cmdPlugin(dir string, args []string) (any, error) {
	if len(args) == 0 {
		return nil, errors.New("usage: da plugin install|list|remove|approve")
	}
	switch args[0] {
	case "list":
		s, err := openSession(dir, false)
		if err != nil {
			return nil, err
		}
		defer s.close()
		if s.info != nil {
			return s.call(http.MethodGet, "/api/plugins", nil)
		}
		return map[string]any{"ok": true, "plugins": s.app.Home().Plugins}, nil
	case "install":
		approve := false
		var path string
		for _, a := range args[1:] {
			if a == "--approve" {
				approve = true
				continue
			}
			path = a
		}
		if path == "" {
			return nil, errors.New("usage: da plugin install [--approve] <path>")
		}
		s, err := openSession(dir, false)
		if err != nil {
			return nil, err
		}
		defer s.close()
		if s.info != nil {
			return s.call(http.MethodPost, "/api/plugins/install", map[string]any{"path": path, "approve": approve})
		}
		res, err := s.app.Install(path, approve)
		if err != nil {
			return nil, err
		}
		return struct {
			core.InstallResult
			OK bool `json:"ok"`
		}{InstallResult: res, OK: true}, nil
	case "remove":
		if len(args) < 2 {
			return nil, errors.New("usage: da plugin remove <name>")
		}
		s, err := openSession(dir, false)
		if err != nil {
			return nil, err
		}
		defer s.close()
		if s.info != nil {
			return s.call(http.MethodPost, "/api/plugins/"+args[1]+"/remove", map[string]any{})
		}
		if err := s.app.Remove(args[1]); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "approve":
		if len(args) < 2 {
			return nil, errors.New("usage: da plugin approve <name>")
		}
		s, err := openSession(dir, true)
		if err != nil {
			return nil, err
		}
		defer s.close()
		if s.info != nil {
			return s.call(http.MethodPost, "/api/plugins/"+args[1]+"/approve", map[string]any{})
		}
		if err := s.app.ApproveManifest(args[1]); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	default:
		return nil, fmt.Errorf("unknown plugin command %q", args[0])
	}
}

func cmdContext(dir string, args []string) (any, error) {
	plugin := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--plugin" && i+1 < len(args) {
			i++
			plugin = args[i]
		}
	}
	s, err := openSession(dir, false)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if s.info != nil {
		path := "/api/context"
		if plugin != "" {
			path += "?plugin=" + plugin
		}
		return s.call(http.MethodGet, path, nil)
	}
	if plugin == "" {
		return s.app.Home(), nil
	}
	ctx, err := s.app.Context(plugin)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "context": ctx}, nil
}

func cmdPropose(dir string, stdin io.Reader) (any, error) {
	var body struct {
		Plugin string `json:"plugin"`
		model.CardDraft
	}
	if _, err := readInput(stdin, &body); err != nil {
		return nil, err
	}
	s, err := openSession(dir, false)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if s.info != nil {
		return s.call(http.MethodPost, "/api/propose", body)
	}
	actor := body.Actor
	if actor == "" {
		actor = model.ActorExternal
	}
	card, err := s.app.Propose(body.Plugin, body.CardDraft, actor)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "card": card}, nil
}

func cmdApproved(dir string) (any, error) {
	s, err := openSession(dir, false)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if s.info != nil {
		return s.call(http.MethodGet, "/api/approved", nil)
	}
	return map[string]any{"ok": true, "cards": s.app.Approved(50)}, nil
}

func cmdCheck(dir string, stdin io.Reader) (any, error) {
	var body struct {
		Plugin     string   `json:"plugin"`
		Permission string   `json:"permission"`
		Effects    []string `json:"effects"`
		CardID     string   `json:"card_id"`
	}
	if _, err := readInput(stdin, &body); err != nil {
		return nil, err
	}
	s, err := openSession(dir, false)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if s.info != nil {
		out, status, err := s.do(http.MethodPost, "/api/check", body)
		if err != nil {
			return nil, err
		}
		if status >= 400 && status != http.StatusForbidden {
			return nil, errorFrom(out, status)
		}
		return out, nil
	}
	res, err := s.app.Check(body.Plugin, body.Permission, body.Effects, body.CardID)
	if err != nil && res.Decision == "" {
		return nil, err
	}
	return map[string]any{"ok": res.Decision == model.ModeAllowed, "decision": res.Decision, "reason": res.Reason}, nil
}

func cmdLog(dir string, stdin io.Reader) (any, error) {
	var body struct {
		Plugin   string           `json:"plugin"`
		Message  string           `json:"message"`
		Evidence []model.Evidence `json:"evidence"`
	}
	has, err := readInput(stdin, &body)
	if err != nil {
		return nil, err
	}
	s, err := openSession(dir, false)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if !has {
		if s.info != nil {
			return s.call(http.MethodGet, "/api/activity", nil)
		}
		rows, err := s.app.Activity(200)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "entries": rows}, nil
	}
	if s.info != nil {
		return s.call(http.MethodPost, "/api/log", body)
	}
	if err := s.app.Log(body.Plugin, body.Message, body.Evidence); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func cmdOutcome(dir string, stdin io.Reader) (any, error) {
	var body struct {
		Plugin   string           `json:"plugin"`
		CardID   string           `json:"card_id"`
		Summary  string           `json:"summary"`
		Evidence []model.Evidence `json:"evidence"`
	}
	if _, err := readInput(stdin, &body); err != nil {
		return nil, err
	}
	s, err := openSession(dir, false)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if s.info != nil {
		return s.call(http.MethodPost, "/api/outcome", body)
	}
	card, err := s.app.Outcome(body.Plugin, body.CardID, body.Summary, body.Evidence)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "card": card}, nil
}

func cmdTile(dir string, stdin io.Reader) (any, error) {
	var body struct {
		Plugin string `json:"plugin"`
		model.Tile
	}
	if _, err := readInput(stdin, &body); err != nil {
		return nil, err
	}
	s, err := openSession(dir, false)
	if err != nil {
		return nil, err
	}
	defer s.close()
	if s.info != nil {
		return s.call(http.MethodPost, "/api/tile", body)
	}
	if err := s.app.WriteTile(body.Plugin, body.Tile); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func readInput(r io.Reader, dest any) (bool, error) {
	if r == nil {
		return false, errors.New("expected JSON on stdin")
	}
	if f, ok := r.(*os.File); ok {
		fi, err := f.Stat()
		if err != nil {
			return false, err
		}
		if fi.Mode()&os.ModeCharDevice != 0 {
			return false, nil
		}
	}
	dec := json.NewDecoder(r)
	if err := dec.Decode(dest); err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func write(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
