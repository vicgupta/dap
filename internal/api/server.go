// Package api serves the local DAP interface and sandboxed plugin pages.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vicgupta/dap/internal/composio"
	"github.com/vicgupta/dap/internal/core"
	"github.com/vicgupta/dap/internal/guardrails"
	"github.com/vicgupta/dap/internal/layout"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginauthor"
	"github.com/vicgupta/dap/internal/theme"
	"github.com/vicgupta/dap/internal/web"
)

// Server is the human-facing localhost site.
type Server struct {
	App    *core.App
	Addr   string
	ln     net.Listener
	srv    *http.Server
	mu     sync.Mutex
	boxes  map[string]*sandbox
	onQuit func()
}

// New builds a server. Call Start to listen. The listener is forced onto loopback.
func New(app *core.App, addr string) *Server {
	if addr == "" {
		addr = "127.0.0.1:7391"
	}
	return &Server{App: app, Addr: restrictLoopback(addr), boxes: map[string]*sandbox{}}
}

func restrictLoopback(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "127.0.0.1:7391"
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// Start listens and opens a sandbox for every plugin that has a web UI.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.Addr, err)
	}
	s.ln = ln
	s.Addr = ln.Addr().String()
	s.srv = &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	s.SyncSandboxes()
	return nil
}

// OnQuit is called after POST /api/quit has written its response.
// da serve uses it for the same shutdown path as SIGINT and SIGTERM.
func (s *Server) OnQuit(fn func()) { s.onQuit = fn }

// URL is the base URL of the human interface.
func (s *Server) URL() string {
	host := s.Addr
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}
	return strings.TrimRight(host, "/") + "/"
}

// Close stops sandboxes and the human listener.
func (s *Server) Close() error {
	s.mu.Lock()
	boxes := s.boxes
	s.boxes = map[string]*sandbox{}
	s.mu.Unlock()
	for _, b := range boxes {
		if b.srv != nil {
			_ = b.srv.Close()
		}
	}
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}

// Handler is the admin site. Static files are public on localhost; /api requires the admin token.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(web.Static, "static")
	fonts, _ := fs.Sub(static, "fonts")
	mux.Handle("GET /fonts/", http.StripPrefix("/fonts/", http.FileServer(http.FS(fonts))))
	mux.HandleFunc("GET /theme.css", file(static, "theme.css"))
	mux.HandleFunc("GET /theme-boot.js", file(static, "theme-boot.js"))
	mux.HandleFunc("GET /layout.css", file(static, "layout.css"))
	mux.HandleFunc("GET /layout-boot.js", file(static, "layout-boot.js"))
	mux.HandleFunc("GET /layout-logic.js", file(static, "layout-logic.js"))
	mux.HandleFunc("GET /app.css", file(static, "app.css"))
	mux.HandleFunc("GET /app.js", file(static, "app.js"))
	mux.HandleFunc("GET /favicon.svg", file(static, "favicon.svg"))
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /api/settings", s.auth(s.getSettings))
	mux.HandleFunc("PUT /api/settings", s.auth(s.putSettings))
	mux.HandleFunc("GET /api/chat/providers", s.auth(s.chatProviders))
	mux.HandleFunc("GET /api/chat/sessions", s.auth(s.chatSessions))
	mux.HandleFunc("POST /api/chat/sessions", s.auth(s.chatCreate))
	mux.HandleFunc("GET /api/chat/sessions/{id}", s.auth(s.chatGet))
	mux.HandleFunc("POST /api/chat/sessions/{id}/turn", s.auth(s.chatTurn))
	mux.HandleFunc("POST /api/chat/sessions/{id}/cancel", s.auth(s.chatCancel))
	mux.HandleFunc("GET /api/home", s.auth(s.home))
	mux.HandleFunc("GET /api/inbox", s.auth(s.inbox))
	mux.HandleFunc("POST /api/inbox/{id}/approve", s.auth(s.approve))
	mux.HandleFunc("POST /api/inbox/{id}/edit", s.auth(s.edit))
	mux.HandleFunc("POST /api/inbox/{id}/reject", s.auth(s.reject))
	mux.HandleFunc("POST /api/inbox/{id}/snooze", s.auth(s.snooze))
	mux.HandleFunc("GET /api/activity", s.auth(s.activity))
	mux.HandleFunc("GET /api/guardrails", s.auth(s.guardrails))
	mux.HandleFunc("POST /api/guardrails", s.auth(s.setRule))
	mux.HandleFunc("GET /api/plugins", s.auth(s.plugins))
	mux.HandleFunc("POST /api/plugins/install", s.auth(s.install))
	mux.HandleFunc("POST /api/plugins/{name}/remove", s.auth(s.remove))
	mux.HandleFunc("POST /api/plugins/{name}/approve", s.auth(s.approveManifest))
	mux.HandleFunc("POST /api/plugins/{name}/run", s.auth(s.run))
	mux.HandleFunc("GET /api/templates", s.auth(s.templates))
	mux.HandleFunc("POST /api/create", s.auth(s.createPlugin))
	mux.HandleFunc("GET /api/create/current", s.auth(s.createCurrent))
	mux.HandleFunc("GET /api/create/{id}", s.auth(s.createGet))
	mux.HandleFunc("POST /api/create/{id}/retry", s.auth(s.createRetry))
	mux.HandleFunc("POST /api/create/{id}/cancel", s.auth(s.createCancel))
	mux.HandleFunc("POST /api/create/{id}/dismiss", s.auth(s.createDismiss))
	mux.HandleFunc("GET /api/doctor", s.auth(s.doctor))
	mux.HandleFunc("GET /api/connections", s.auth(s.connections))
	mux.HandleFunc("POST /api/connections/{provider}/setup", s.auth(s.connectionSetup))
	mux.HandleFunc("POST /api/connections/{provider}/connect", s.auth(s.connectionConnect))
	mux.HandleFunc("POST /api/connections/{provider}/disconnect", s.auth(s.connectionDisconnect))
	mux.HandleFunc("GET /api/approved", s.auth(s.approved))
	mux.HandleFunc("GET /api/context", s.auth(s.context))
	mux.HandleFunc("POST /api/propose", s.auth(s.propose))
	mux.HandleFunc("POST /api/check", s.auth(s.check))
	mux.HandleFunc("POST /api/log", s.auth(s.log))
	mux.HandleFunc("POST /api/outcome", s.auth(s.outcome))
	mux.HandleFunc("POST /api/tile", s.auth(s.tile))
	mux.HandleFunc("GET /api/quit-status", s.auth(s.quitStatus))
	mux.HandleFunc("POST /api/quit", s.auth(s.quit))
	mux.HandleFunc("GET /plugins/{name}/", s.pluginRedirect)
	mux.HandleFunc("GET /plugins/{name}", s.pluginRedirect)
	return mux
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(web.Static, "static/index.html")
	if err != nil {
		http.Error(w, "missing ui", http.StatusInternalServerError)
		return
	}
	mode := s.App.Theme()
	prefs := s.App.Layout()
	raw, _ := json.Marshal(prefs)
	boot := `<meta name="dap-token" content="` + s.App.AdminToken() + `" />` +
		`<meta name="dap-version" content="0.1.0" />` +
		`<meta name="dap-theme" content="` + mode + `" />` +
		`<meta name="dap-density" content="` + prefs.PluginDensity() + `" />` +
		`<meta name="dap-layout" content="` + html.EscapeString(string(raw)) + `" />`
	page := injectRoot(bytes.Replace(b, []byte("<!--DAP_BOOT-->"), []byte(boot), 1), map[string]string{
		"data-theme":      mode,
		"data-density":    prefs.PluginDensity(),
		"data-home-view":  prefs.Home.View,
		"data-inbox-view": prefs.Inbox.View,
	})
	secure(w, s.URL())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.App.ChatConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"theme":  s.App.Theme(),
		"layout": s.App.Layout(),
		"chat":   cfg,
	})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Theme  *string          `json:"theme"`
		Layout *layout.Settings `json:"layout"`
		Chat   *struct {
			Provider string `json:"provider"`
		} `json:"chat"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	mode := s.App.Theme()
	if body.Theme != nil {
		next, err := s.App.SetTheme(*body.Theme)
		if err != nil {
			writeErr(w, err)
			return
		}
		mode = next
	}
	prefs := s.App.Layout()
	if body.Layout != nil {
		next, err := s.App.SetLayout(*body.Layout)
		if err != nil {
			writeErr(w, err)
			return
		}
		prefs = next
	}
	chatCfg := s.App.ChatConfig()
	if body.Chat != nil {
		next, err := s.App.SetChatProvider(body.Chat.Provider)
		if err != nil {
			writeErr(w, err)
			return
		}
		chatCfg.Provider = next
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "theme": mode, "layout": prefs, "chat": chatCfg})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !tokenOK(r, s.App.AdminToken()) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.App.Home())
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cards": s.App.Inbox()})
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	card, err := s.App.Approve(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "card": card})
}

func (s *Server) edit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title   *string          `json:"title"`
		Summary *string          `json:"summary"`
		Body    *string          `json:"body"`
		Payload *json.RawMessage `json:"payload"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	card, err := s.App.Edit(r.PathValue("id"), core.Edit{
		Title: body.Title, Summary: body.Summary, Body: body.Body, Payload: body.Payload,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "card": card})
}

func (s *Server) reject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	card, err := s.App.Reject(r.PathValue("id"), body.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "card": card})
}

func (s *Server) snooze(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Preset string `json:"preset"`
		Until  string `json:"until"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	until, err := snoozeWhen(body.Preset, body.Until, time.Now())
	if err != nil {
		writeErr(w, err)
		return
	}
	card, err := s.App.Snooze(r.PathValue("id"), until)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "card": card})
}

func snoozeWhen(preset, until string, now time.Time) (time.Time, error) {
	switch preset {
	case "1h":
		return now.Add(time.Hour), nil
	case "4h":
		return now.Add(4 * time.Hour), nil
	case "tomorrow":
		local := now.In(time.Local)
		next := time.Date(local.Year(), local.Month(), local.Day(), 8, 0, 0, 0, time.Local).AddDate(0, 0, 1)
		return next, nil
	}
	if until == "" {
		return time.Time{}, errors.New("choose a snooze preset or an until time")
	}
	t, err := time.Parse(time.RFC3339, until)
	if err != nil {
		return time.Time{}, fmt.Errorf("until must be RFC3339")
	}
	return t, nil
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	rows, err := s.App.Activity(200)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entries": rows})
}

func (s *Server) guardrails(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.App.Guardrails())
}

func (s *Server) setRule(w http.ResponseWriter, r *http.Request) {
	var rule guardrails.Rule
	if err := readJSON(r, &rule); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.App.SetRule(rule); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) plugins(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "plugins": s.App.Home().Plugins})
}

func (s *Server) install(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path    string `json:"path"`
		Approve bool   `json:"approve"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.App.Install(body.Path, body.Approve)
	if err != nil {
		writeErr(w, err)
		return
	}
	if body.Approve {
		s.SyncSandboxes()
	}
	writeJSON(w, http.StatusOK, struct {
		core.InstallResult
		OK bool `json:"ok"`
	}{InstallResult: res, OK: true})
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.App.Remove(name); err != nil {
		writeErr(w, err)
		return
	}
	s.SyncSandboxes()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) approveManifest(w http.ResponseWriter, r *http.Request) {
	if err := s.App.ApproveManifest(r.PathValue("name")); err != nil {
		writeErr(w, err)
		return
	}
	s.SyncSandboxes()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	if err := s.App.RunPlugin(r.PathValue("name"), "manual"); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) templates(w http.ResponseWriter, r *http.Request) {
	list, err := pluginauthor.ListTemplates()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "templates": list})
}

func (s *Server) createPlugin(w http.ResponseWriter, r *http.Request) {
	var req core.CreateRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.App.CreatePlugin(req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) createCurrent(w http.ResponseWriter, r *http.Request) {
	res, ok := s.App.CurrentCreate()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) createGet(w http.ResponseWriter, r *http.Request) {
	res, err := s.App.CreateJob(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) createRetry(w http.ResponseWriter, r *http.Request) {
	res, err := s.App.RetryCreate(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) createCancel(w http.ResponseWriter, r *http.Request) {
	if err := s.App.CancelCreate(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) createDismiss(w http.ResponseWriter, r *http.Request) {
	if err := s.App.DismissCreate(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "key_saved": s.App.ComposioKeySaved(),
		"connections": s.App.MailStatuses(), "agents": core.DetectAgents(),
	})
}

func (s *Server) connectionSetup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientID *string `json:"client_id"`
		Calendar *bool   `json:"calendar"`
		Mode     *string `json:"mode"`
		APIKey   *string `json:"api_key"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	verified := false
	warning := ""
	if body.APIKey != nil && strings.TrimSpace(*body.APIKey) != "" {
		check, err := s.App.SaveComposioKey(r.Context(), *body.APIKey)
		if err != nil {
			writeErr(w, err)
			return
		}
		verified = check.Verified
		warning = check.Warning
		body.APIKey = nil
	}
	if err := s.App.ConfigureConnection(r.PathValue("provider"), body.ClientID, body.Calendar, body.Mode, body.APIKey); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "key_saved": s.App.ComposioKeySaved(), "key_verified": verified, "warning": warning,
	})
}

func (s *Server) connectionConnect(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	if err := s.App.MailConnect(ctx, r.PathValue("provider")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) connectionDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.App.MailDisconnect(r.PathValue("provider")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) doctor(w http.ResponseWriter, r *http.Request) {
	rep := s.App.Doctor()
	writeJSON(w, http.StatusOK, map[string]any{"ok": rep.OK, "version": rep.Version, "checks": rep.Checks})
}

func (s *Server) approved(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cards": s.App.Approved(50)})
}

func (s *Server) context(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("plugin")
	if name == "" {
		writeJSON(w, http.StatusOK, s.App.Home())
		return
	}
	ctx, err := s.App.Context(name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "context": ctx})
}

func (s *Server) propose(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plugin string `json:"plugin"`
		model.CardDraft
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	actor := body.Actor
	if actor == "" {
		actor = model.ActorExternal
	}
	card, err := s.App.Propose(body.Plugin, body.CardDraft, actor)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "card": card})
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plugin     string   `json:"plugin"`
		Permission string   `json:"permission"`
		Effects    []string `json:"effects"`
		CardID     string   `json:"card_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.App.Check(body.Plugin, body.Permission, body.Effects, body.CardID)
	code := http.StatusOK
	if res.Decision != "" && res.Decision != model.ModeAllowed {
		code = http.StatusForbidden
	}
	if err != nil && res.Decision == "" {
		writeErr(w, err)
		return
	}
	writeJSON(w, code, map[string]any{"ok": res.Decision == model.ModeAllowed, "decision": res.Decision, "reason": res.Reason})
}

func (s *Server) log(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plugin   string           `json:"plugin"`
		Message  string           `json:"message"`
		Evidence []model.Evidence `json:"evidence"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.App.Log(body.Plugin, body.Message, body.Evidence); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) outcome(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plugin   string           `json:"plugin"`
		CardID   string           `json:"card_id"`
		Summary  string           `json:"summary"`
		Evidence []model.Evidence `json:"evidence"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	card, err := s.App.Outcome(body.Plugin, body.CardID, body.Summary, body.Evidence)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "card": card})
}

func (s *Server) quitStatus(w http.ResponseWriter, r *http.Request) {
	st := s.App.QuitStatus()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"running":          st.Running,
		"plugins_running":  st.PluginsRunning,
		"inflight_actions": st.InflightActions,
		"acting":           st.Acting,
	})
}

func (s *Server) quit(w http.ResponseWriter, r *http.Request) {
	if !loopbackPeer(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "quit is only available on this machine"})
		return
	}
	if !quitCSRF(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "quit must be a same-origin POST"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	fn := s.onQuit
	if fn == nil {
		return
	}
	go func() {
		// Let the response leave before serve starts closing listeners.
		time.Sleep(150 * time.Millisecond)
		fn()
	}()
}

func (s *Server) tile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plugin string `json:"plugin"`
		model.Tile
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.App.WriteTile(body.Plugin, body.Tile); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) pluginRedirect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.mu.Lock()
	box := s.boxes[name]
	s.mu.Unlock()
	if box == nil {
		http.Error(w, "this plugin has no web UI", http.StatusNotFound)
		return
	}
	target := withQuery(box.url, "theme", chosenTheme(r, s.App.Theme()))
	target = withQuery(target, "density", layout.ChosenDensity(r.URL.Query().Get("density"), s.App.Layout().PluginDensity()))
	http.Redirect(w, r, target, http.StatusFound)
}

// injectTheme sets data-theme on the root element before the browser paints.
func injectTheme(doc []byte, mode string) []byte {
	mode = theme.Stored(mode)
	attr := []byte(`<html lang="en" data-theme="` + mode + `">`)
	if bytes.Contains(doc, []byte(`<html lang="en" data-theme="`)) {
		start := bytes.Index(doc, []byte(`<html lang="en" data-theme="`))
		end := bytes.Index(doc[start:], []byte(">"))
		if end > 0 {
			return append(append([]byte{}, doc[:start]...), append(attr, doc[start+end+1:]...)...)
		}
	}
	return bytes.Replace(doc, []byte("<html lang=\"en\">"), attr, 1)
}

func chosenTheme(r *http.Request, fallback string) string {
	q := strings.TrimSpace(r.URL.Query().Get("theme"))
	if q == "" {
		return theme.Stored(fallback)
	}
	mode, err := theme.Normalize(q)
	if err != nil {
		return theme.Stored(fallback)
	}
	return mode
}

func withThemeQuery(raw, mode string) string {
	return withQuery(raw, "theme", theme.Stored(mode))
}

func withQuery(raw, key, value string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}

func injectRoot(doc []byte, attrs map[string]string) []byte {
	start := bytes.Index(doc, []byte("<html"))
	if start < 0 {
		return doc
	}
	rel := bytes.IndexByte(doc[start:], '>')
	if rel < 0 {
		return doc
	}
	end := start + rel
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(`<html lang="en"`)
	for _, k := range keys {
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteString(`="`)
		b.WriteString(html.EscapeString(attrs[k]))
		b.WriteString(`"`)
	}
	b.WriteByte('>')
	out := append([]byte{}, doc[:start]...)
	out = append(out, b.String()...)
	out = append(out, doc[end+1:]...)
	return out
}

func file(static fs.FS, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(static, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		secure(w, "")
		switch {
		case strings.HasSuffix(name, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(name, ".js"):
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case strings.HasSuffix(name, ".svg"):
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	}
}

func secure(w http.ResponseWriter, origin string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	csp := "default-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'"
	if origin != "" {
		csp += "; frame-src http://127.0.0.1:* http://localhost:*"
	}
	w.Header().Set("Content-Security-Policy", csp)
}

func loopbackPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return loopbackHost(host)
}

func loopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return loopbackHost(u.Hostname())
}

// quitCSRF rejects cross-site form posts. The admin token is a bearer header,
// and browsers will not attach a custom header on a simple cross-site POST.
func quitCSRF(r *http.Request) bool {
	if r.Header.Get("X-DAP-Request") != "quit" {
		return false
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && !loopbackOrigin(origin) {
		return false
	}
	return true
}

func tokenOK(r *http.Request, token string) bool {
	h := r.Header.Get("Authorization")
	const pfx = "Bearer "
	if !strings.HasPrefix(h, pfx) {
		return false
	}
	got := strings.TrimSpace(h[len(pfx):])
	if len(got) != len(token) || token == "" {
		return false
	}
	var v byte
	for i := 0; i < len(got); i++ {
		v |= got[i] ^ token[i]
	}
	return v == 0
}

func readJSON(r *http.Request, dest any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	switch {
	case errors.Is(err, core.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, core.ErrDenied), errors.Is(err, core.ErrBlocked):
		code = http.StatusForbidden
	case errors.Is(err, core.ErrBadState), errors.Is(err, core.ErrBusy):
		code = http.StatusConflict
	}
	msg, requestID := composio.Public(err)
	body := map[string]any{"ok": false, "error": msg}
	if requestID != "" {
		body["request_id"] = requestID
	}
	var api *composio.APIError
	if errors.As(err, &api) && api.Status == http.StatusUnauthorized {
		code = http.StatusUnauthorized
	}
	writeJSON(w, code, body)
}
