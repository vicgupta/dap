package core

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/vicgupta/dap/internal/model"
)

// ensureAPI listens on 127.0.0.1:0 for plugin callbacks.
func (a *App) ensureAPI() error {
	if a.apiSrv != nil {
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	a.apiLn = ln
	a.apiBase = "http://" + ln.Addr().String()
	srv := &http.Server{
		Handler:           a.PluginHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	a.apiSrv = srv
	go func() { _ = srv.Serve(ln) }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", ln.Addr().String(), 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

// PluginHandler is the scoped API. Paths are /v1/{plugin}/...
func (a *App) PluginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		parts := splitPath(r.URL.Path)
		if len(parts) < 3 || parts[0] != "v1" {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
			return
		}
		plugin := parts[1]
		rest := strings.Join(parts[2:], "/")
		token := bearer(r)
		if token == "" || subtleEq(token, a.Token(plugin)) == false {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		a.routePlugin(w, r, plugin, rest)
	})
}

func (a *App) routePlugin(w http.ResponseWriter, r *http.Request, plugin, rest string) {
	switch {
	case r.Method == http.MethodGet && rest == "context":
		ctx, err := a.Context(plugin)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "plugin": ctx.Plugin, "config": ctx.Config, "schedule": ctx.Schedule, "state": json.RawMessage(ctx.State), "open_cards": ctx.OpenCards, "health": ctx.Health})
	case r.Method == http.MethodGet && rest == "cards":
		cards, err := a.PluginCards(plugin)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cards": cards})
	case r.Method == http.MethodGet && rest == "state":
		raw, err := a.GetState(plugin)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": json.RawMessage(raw)})
	case r.Method == http.MethodPut && rest == "state":
		body, err := readBody(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := a.PutState(plugin, body); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodPost && rest == "log":
		var body struct {
			Message  string           `json:"message"`
			Evidence []model.Evidence `json:"evidence"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		if err := a.Log(plugin, body.Message, body.Evidence); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodPost && rest == "tile":
		var tile model.Tile
		if err := decode(r, &tile); err != nil {
			writeErr(w, err)
			return
		}
		if err := a.WriteTile(plugin, tile); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodPost && rest == "propose":
		var draft model.CardDraft
		if err := decode(r, &draft); err != nil {
			writeErr(w, err)
			return
		}
		card, err := a.Propose(plugin, draft, model.ActorPlugin)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "card": card})
	case r.Method == http.MethodPost && (rest == "check" || rest == "call"):
		var body struct {
			Permission string   `json:"permission"`
			Effects    []string `json:"effects"`
			CardID     string   `json:"card_id"`
			Summary    string   `json:"summary"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		res, err := a.Check(plugin, body.Permission, body.Effects, body.CardID)
		status := http.StatusOK
		if res.Decision != model.ModeAllowed {
			status = http.StatusForbidden
		}
		payload := map[string]any{"ok": res.Decision == model.ModeAllowed, "decision": res.Decision, "reason": res.Reason}
		if err != nil && res.Decision == "" {
			writeErr(w, err)
			return
		}
		if err != nil && res.Reason == "" {
			payload["reason"] = err.Error()
		}
		writeJSON(w, status, payload)
	case r.Method == http.MethodPost && rest == "outcome":
		var body struct {
			CardID   string           `json:"card_id"`
			Summary  string           `json:"summary"`
			Evidence []model.Evidence `json:"evidence"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		card, err := a.Outcome(plugin, body.CardID, body.Summary, body.Evidence)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "card": card})
	case r.Method == http.MethodPost && rest == "secret/set":
		var body struct {
			Account string `json:"account"`
			Secret  string `json:"secret"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		if err := a.SecretSet(plugin, body.Account, body.Secret); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodPost && rest == "secret/get":
		var body struct {
			Account string `json:"account"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		secret, err := a.SecretGet(plugin, body.Account)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "secret": secret})
	case r.Method == http.MethodPost && rest == "secret/delete":
		var body struct {
			Account string `json:"account"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		if err := a.SecretDelete(plugin, body.Account); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
	}
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if strings.HasPrefix(h, p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

func subtleEq(a, b string) bool {
	if len(a) != len(b) || a == "" {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func decode(r *http.Request, dest any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dest); err != nil {
		return err
	}
	return nil
}

func readBody(r *http.Request) (json.RawMessage, error) {
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if !json.Valid(b) {
		return nil, errors.New("state must be JSON")
	}
	return json.RawMessage(b), nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	switch {
	case errors.Is(err, ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, ErrDenied), errors.Is(err, ErrBlocked):
		code = http.StatusForbidden
	case errors.Is(err, ErrBadState):
		code = http.StatusConflict
	}
	writeJSON(w, code, map[string]any{"ok": false, "error": err.Error()})
}
