package api

import (
	"encoding/json"
	"net/http"

	"github.com/vicgupta/dap/internal/core"
)

func (s *Server) chatProviders(w http.ResponseWriter, r *http.Request) {
	probes, err := s.App.ChatProbes(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"provider":  s.App.ChatConfig().Provider,
		"providers": probes,
	})
}

func (s *Server) chatSessions(w http.ResponseWriter, r *http.Request) {
	list, err := s.App.ListChatSessions()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sessions": list})
}

func (s *Server) chatCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	provider := body.Provider
	if provider == "" {
		provider = s.App.ChatConfig().Provider
	}
	sess, err := s.App.CreateChatSession(provider)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session": sess})
}

func (s *Server) chatGet(w http.ResponseWriter, r *http.Request) {
	sess, cards, err := s.App.ChatSession(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session": sess, "cards": cards})
}

func (s *Server) chatCancel(w http.ResponseWriter, r *http.Request) {
	if err := s.App.CancelChat(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) chatTurn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	flusher, _ := w.(http.Flusher)
	started := false
	enc := json.NewEncoder(w)
	err := s.App.RunChatTurn(r.Context(), r.PathValue("id"), body.Text, func(ev core.ChatEvent) error {
		if !started {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusOK)
			started = true
		}
		if err := enc.Encode(ev); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	})
	if err != nil && !started {
		writeErr(w, err)
	}
}
