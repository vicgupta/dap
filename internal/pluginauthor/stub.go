package pluginauthor

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

const conformanceToken = "conformance"

// stubAPI is the in-process host plugins call during validation.
// It does not read a mailbox and it does not send.
type stubAPI struct {
	base string
	srv  *httptest.Server
}

func (s *stubAPI) Close() {
	if s != nil && s.srv != nil {
		s.srv.Close()
	}
}

func startStubAPI(plugin string) (*stubAPI, error) {
	srv := httptest.NewServer(stubHandler(conformanceToken))
	return &stubAPI{base: srv.URL + "/v1/" + plugin, srv: srv}, nil
}

func stubHandler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if bearerToken(r) != token {
			writeStub(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/mail/status"):
			writeStub(w, http.StatusOK, map[string]any{"ok": true, "connections": []any{}})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/mail/threads"):
			writeStub(w, http.StatusOK, map[string]any{"ok": true, "threads": []any{}})
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/mail/draft"):
			writeStub(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"draft_id": "draft_stub", "local": true, "provider": "fake"}})
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/mail/send"):
			writeStub(w, http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_id": "msg_stub", "provider": "fake", "to": "stub@example.com"}})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/inbox"):
			writeStub(w, http.StatusOK, map[string]any{"ok": true, "cards": []any{}})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/cards"):
			writeStub(w, http.StatusOK, map[string]any{"ok": true, "cards": []any{}})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/state"):
			writeStub(w, http.StatusOK, map[string]any{"ok": true, "state": map[string]any{}})
		case r.Method == http.MethodPut && strings.HasSuffix(path, "/state"):
			writeStub(w, http.StatusOK, map[string]any{"ok": true})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/context"):
			writeStub(w, http.StatusOK, map[string]any{"ok": true, "config": map[string]any{}, "state": map[string]any{}, "open_cards": []any{}})
		case r.Method == http.MethodPost && (strings.HasSuffix(path, "/check") || strings.HasSuffix(path, "/call")):
			body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if stubSensitive(body) {
				writeStub(w, http.StatusForbidden, map[string]any{"ok": false, "decision": "ask_first", "reason": "This step waits for approval."})
				return
			}
			writeStub(w, http.StatusOK, map[string]any{"ok": true, "decision": "allowed", "reason": ""})
		case r.Method == http.MethodPost && (strings.HasSuffix(path, "/tile") || strings.HasSuffix(path, "/log") || strings.HasSuffix(path, "/propose")):
			writeStub(w, http.StatusOK, map[string]any{"ok": true, "card": map[string]any{"id": "c_stub", "title": "Stub", "status": "pending"}})
		default:
			writeStub(w, http.StatusOK, map[string]any{"ok": true})
		}
	})
}

func stubSensitive(body []byte) bool {
	var payload struct {
		Effects []string `json:"effects"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return false
	}
	for _, effect := range payload.Effects {
		switch strings.ToLower(strings.TrimSpace(effect)) {
		case "sends", "spends", "deletes":
			return true
		}
	}
	return false
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

func writeStub(w http.ResponseWriter, status int, body any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
