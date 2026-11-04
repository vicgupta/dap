package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/vicgupta/dap/internal/chat"
	"github.com/vicgupta/dap/internal/guardrails"
	"github.com/vicgupta/dap/internal/id"
	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/rundir"
	"github.com/vicgupta/dap/internal/store"
)

// ChatConfig is the provider remembered in config.json under "chat".
type ChatConfig struct {
	Provider string `json:"provider"`
}

type chatSystem struct {
	store  *chat.Store
	runner *chat.Runner
	status *chat.Status
}

func (a *App) openChat() (*chatSystem, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.chatSys != nil {
		return a.chatSys, nil
	}
	st, err := chat.OpenStore(filepath.Join(a.DataDir, "chat"))
	if err != nil {
		return nil, err
	}
	dataDir := a.DataDir
	env := func() []string {
		_ = rundir.Ensure(dataDir)
		return rundir.Apply(chat.BaseEnv(), dataDir)
	}
	a.chatSys = &chatSystem{store: st, runner: &chat.Runner{Env: env}, status: &chat.Status{Env: env}}
	return a.chatSys, nil
}

// SetChatRunner replaces the subprocess runner. Tests use it to point at a fake CLI.
func (a *App) SetChatRunner(r *chat.Runner) error {
	sys, err := a.openChat()
	if err != nil {
		return err
	}
	if r == nil {
		r = &chat.Runner{}
	}
	sys.runner = r
	return nil
}

// SetChatStatus replaces install and sign-in probes. Tests use it.
func (a *App) SetChatStatus(s *chat.Status) error {
	sys, err := a.openChat()
	if err != nil {
		return err
	}
	if s == nil {
		s = &chat.Status{}
	}
	sys.status = s
	return nil
}

// ChatConfig reads the saved provider. An unknown value becomes empty.
// Theme and layout keys are not touched.
func (a *App) ChatConfig() ChatConfig {
	raw, err := readConfigRaw(a.DataDir)
	if err != nil || len(raw["chat"]) == 0 {
		return ChatConfig{}
	}
	var cfg ChatConfig
	if json.Unmarshal(raw["chat"], &cfg) != nil {
		return ChatConfig{}
	}
	cfg.Provider = chat.StoredProvider(cfg.Provider)
	return cfg
}

// SetChatProvider stores the picker choice. Other config.json keys stay.
func (a *App) SetChatProvider(v string) (string, error) {
	id, err := chat.NormalizeProvider(v)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(ChatConfig{Provider: id})
	if err != nil {
		return "", err
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	raw, err := readConfigRaw(a.DataDir)
	if err != nil {
		return "", err
	}
	raw["chat"] = body
	if err := writeConfigRaw(a.DataDir, raw); err != nil {
		return "", err
	}
	return id, nil
}

// ChatProbes reports Claude Code and Codex. Credentials are not read.
func (a *App) ChatProbes(ctx context.Context) ([]chat.Probe, error) {
	sys, err := a.openChat()
	if err != nil {
		return nil, err
	}
	return sys.status.Probes(ctx), nil
}

// ChatRunning is true while a CLI turn is in progress.
func (a *App) ChatRunning() bool {
	a.mu.Lock()
	sys := a.chatSys
	a.mu.Unlock()
	return sys != nil && sys.runner != nil && sys.runner.Running()
}

func (a *App) stopChat() {
	a.mu.Lock()
	sys := a.chatSys
	a.mu.Unlock()
	if sys != nil && sys.runner != nil {
		sys.runner.StopAll()
	}
}

// ListChatSessions returns history newest first.
func (a *App) ListChatSessions() ([]chat.Info, error) {
	sys, err := a.openChat()
	if err != nil {
		return nil, err
	}
	return sys.store.List()
}

// CreateChatSession starts an empty session for a provider.
func (a *App) CreateChatSession(provider string) (chat.Session, error) {
	if a.isStopping() {
		return chat.Session{}, ErrStopping
	}
	id, err := chat.NormalizeProvider(provider)
	if err != nil {
		return chat.Session{}, err
	}
	sys, err := a.openChat()
	if err != nil {
		return chat.Session{}, err
	}
	sess, err := sys.store.Create(id, a.now())
	if err != nil {
		return chat.Session{}, err
	}
	label := providerLabel(id)
	a.log(ledger.Entry{
		Plugin:  chat.PluginName,
		Kind:    ledger.KindChatSession,
		Message: "Started a " + label + " chat",
		Data:    map[string]any{"provider": id, "session_id": sess.ID},
	})
	return sess, nil
}

// ChatSession loads one session and the cards it proposed.
func (a *App) ChatSession(id string) (chat.Session, map[string]model.Card, error) {
	sys, err := a.openChat()
	if err != nil {
		return chat.Session{}, nil, err
	}
	sess, err := sys.store.Load(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return chat.Session{}, nil, fmt.Errorf("%w: chat %s", ErrNotFound, id)
		}
		return chat.Session{}, nil, err
	}
	return sess, a.cardsByID(cardIDs(sess)), nil
}

// CancelChat stops the running turn for a session. The turn logs the stop.
func (a *App) CancelChat(id string) error {
	sys, err := a.openChat()
	if err != nil {
		return err
	}
	if sys.runner != nil {
		sys.runner.Stop(id)
	}
	return nil
}

// ChatEvent is one line of the turn stream sent to the browser.
type ChatEvent struct {
	Type     string         `json:"type"`
	Delta    string         `json:"delta,omitempty"`
	Text     string         `json:"text,omitempty"`
	Name     string         `json:"name,omitempty"`
	Detail   string         `json:"detail,omitempty"`
	Blocked  bool           `json:"blocked,omitempty"`
	Card     *model.Card    `json:"card,omitempty"`
	Message  string         `json:"message,omitempty"`
	Session  string         `json:"session_id,omitempty"`
	Provider string         `json:"provider_session_id,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// RunChatTurn starts one read-only CLI turn and calls emit for each event.
// The working directory is the session workspace under the data directory,
// never the data directory itself.
func (a *App) RunChatTurn(ctx context.Context, sessionID, text string, emit func(ChatEvent) error) error {
	if a.isStopping() {
		return ErrStopping
	}
	end, err := a.beginCall(chat.PluginName, false)
	if err != nil {
		return err
	}
	defer end()
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("message is empty")
	}
	if len(text) > 32<<10 {
		return fmt.Errorf("message is too long")
	}
	sys, err := a.openChat()
	if err != nil {
		return err
	}
	sys.store.Lock()
	sess, err := sys.store.Load(sessionID)
	if err != nil {
		sys.store.Unlock()
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: chat %s", ErrNotFound, sessionID)
		}
		return err
	}
	p, ok := chat.ByID(sess.Provider)
	if !ok {
		sys.store.Unlock()
		return fmt.Errorf("provider must be claude-code or codex")
	}
	work, err := sys.store.WorkDir(sess.ID)
	if err != nil {
		sys.store.Unlock()
		return err
	}
	rules, err := sys.store.RulesFile(sess.ID)
	if err != nil {
		sys.store.Unlock()
		return err
	}
	if err := os.MkdirAll(work, 0o700); err != nil {
		sys.store.Unlock()
		return err
	}
	if _, err := os.Stat(rules); err != nil {
		if werr := os.WriteFile(rules, []byte(chat.Rules), 0o600); werr != nil {
			sys.store.Unlock()
			return werr
		}
	}
	// Refuse to launch with the data directory as cwd. That folder holds
	// run.json and the secret file.
	if samePath(work, a.DataDir) {
		sys.store.Unlock()
		return fmt.Errorf("chat working directory must be a session folder")
	}
	turn := chat.Turn{
		Prompt:            text,
		WorkDir:           work,
		RulesFile:         rules,
		ProviderSessionID: sess.ProviderSessionID,
		Resume:            sess.Bound && sess.ProviderSessionID != "",
	}
	if p.ID() == chat.ProviderClaude && strings.TrimSpace(sess.ProviderSessionID) == "" {
		sess.ProviderSessionID = chat.MintSessionID()
		turn.ProviderSessionID = sess.ProviderSessionID
		turn.Resume = false
	}
	now := a.now().UTC()
	if sess.Title == "" || sess.Title == "New chat" {
		sess.Title = chatTitle(text)
	}
	user := chat.Message{ID: id.New("m"), Role: "user", Text: text, At: now}
	asst := chat.Message{ID: id.New("m"), Role: "assistant", At: now}
	sess.Messages = append(sess.Messages, user, asst)
	sess.UpdatedAt = now
	if p.ID() == chat.ProviderClaude {
		sess.Bound = true
	}
	if err := sys.store.Save(sess); err != nil {
		sys.store.Unlock()
		return err
	}
	sys.store.Unlock()

	run, err := sys.runner.Start(ctx, sess.ID, p, turn)
	if err != nil {
		asst.Error = err.Error()
		sess.Messages[len(sess.Messages)-1] = asst
		if !turn.Resume {
			sess.Bound = false
		}
		sys.store.Lock()
		_ = sys.store.Save(sess)
		sys.store.Unlock()
		if errors.Is(err, chat.ErrBusy) {
			return ErrBusy
		}
		if errors.Is(err, exec.ErrNotFound) {
			if p.ID() == chat.ProviderCodex {
				return fmt.Errorf("the Codex CLI is not on PATH")
			}
			return fmt.Errorf("Claude Code is not on PATH")
		}
		return err
	}
	defer run.Cancel()

	var b strings.Builder
	seenProp := map[string]bool{}
	cancelled := false
	var runErr string
	for ev := range run.Events {
		if ctx.Err() != nil {
			cancelled = true
			break
		}
		switch ev.Kind {
		case chat.EventSession:
			if ev.SessionID != "" {
				sess.ProviderSessionID = ev.SessionID
				sess.Bound = true
				sys.store.Lock()
				if cur, err := sys.store.Load(sess.ID); err == nil {
					cur.ProviderSessionID = ev.SessionID
					cur.Bound = true
					_ = sys.store.Save(cur)
				}
				sys.store.Unlock()
				_ = emit(ChatEvent{Type: "session", Provider: ev.SessionID, Session: sess.ID})
			}
		case chat.EventText:
			if ev.Delta {
				b.WriteString(ev.Text)
				if err := emit(ChatEvent{Type: "text", Delta: ev.Text}); err != nil {
					cancelled = true
					run.Cancel()
				}
			} else if b.Len() == 0 {
				b.WriteString(ev.Text)
				if err := emit(ChatEvent{Type: "text", Delta: ev.Text}); err != nil {
					cancelled = true
					run.Cancel()
				}
			}
		case chat.EventTool:
			if ev.Tool == nil {
				continue
			}
			asst.Tools = append(asst.Tools, *ev.Tool)
			if err := emit(ChatEvent{Type: "tool", Name: ev.Tool.Name, Detail: ev.Tool.Detail, Blocked: ev.Tool.Blocked}); err != nil {
				cancelled = true
				run.Cancel()
			}
		case chat.EventProposal:
			if ev.Proposal == nil {
				continue
			}
			card, err := a.proposeFromChat(sess.ID, *ev.Proposal, seenProp)
			if err != nil {
				runErr = err.Error()
				_ = emit(ChatEvent{Type: "error", Message: err.Error()})
				continue
			}
			if card.ID == "" {
				continue
			}
			asst.Cards = append(asst.Cards, card.ID)
			if err := emit(ChatEvent{Type: "proposal", Card: &card}); err != nil {
				cancelled = true
				run.Cancel()
			}
		case chat.EventError:
			runErr = ev.Text
			_ = emit(ChatEvent{Type: "error", Message: ev.Text})
		case chat.EventDone:
		}
		if cancelled {
			break
		}
	}
	waitErr := run.Wait()
	if ctx.Err() != nil || errors.Is(waitErr, context.Canceled) {
		cancelled = true
	}
	clean, extra := chat.StripProposals(b.String())
	for _, p := range extra {
		card, err := a.proposeFromChat(sess.ID, p, seenProp)
		if err != nil || card.ID == "" {
			continue
		}
		asst.Cards = append(asst.Cards, card.ID)
		_ = emit(ChatEvent{Type: "proposal", Card: &card})
	}
	asst.Text = clean
	asst.Cancelled = cancelled
	asst.Error = runErr
	if asst.Text == "" && runErr != "" && !cancelled {
		asst.Text = ""
	}
	sess.Messages[len(sess.Messages)-1] = asst
	sess.UpdatedAt = a.now().UTC()
	sys.store.Lock()
	// Reload in case a session id landed, then overlay the messages we built.
	if cur, err := sys.store.Load(sess.ID); err == nil && cur.ProviderSessionID != "" {
		sess.ProviderSessionID = cur.ProviderSessionID
	}
	_ = sys.store.Save(sess)
	sys.store.Unlock()

	kind := ledger.KindChatTurn
	msg := providerLabel(sess.Provider) + " · " + oneLine(text, 120)
	if cancelled {
		kind = ledger.KindChatCancel
		msg = "Stopped a " + providerLabel(sess.Provider) + " turn"
	}
	a.log(ledger.Entry{
		Plugin:  chat.PluginName,
		Kind:    kind,
		Message: msg,
		Data: map[string]any{
			"provider":   sess.Provider,
			"session_id": sess.ID,
			"cancelled":  cancelled,
		},
	})
	_ = emit(ChatEvent{Type: "done", Text: clean})
	return nil
}

func (a *App) proposeFromChat(sessionID string, p chat.Proposal, seen map[string]bool) (model.Card, error) {
	key := p.Op + "|" + p.Path + "|" + p.Title
	if seen[key] {
		return model.Card{}, nil
	}
	seen[key] = true
	return a.proposeChat(sessionID, p)
}

func (a *App) proposeChat(sessionID string, p chat.Proposal) (model.Card, error) {
	if strings.TrimSpace(p.Title) == "" {
		return model.Card{}, fmt.Errorf("a card needs a title")
	}
	effects, err := guardrails.NormalizeEffects(p.Effects)
	if err != nil {
		return model.Card{}, err
	}
	perm := p.Perm
	if perm == "" {
		perm = chat.PermWrite
	}
	res := guardrails.Evaluate(guardrails.Request{
		Plugin:     chat.PluginName,
		Permission: perm,
		Effects:    effects,
		Declared:   chatPermissions(),
		Overrides:  a.store.Snapshot().Rules,
	})
	a.rememberCheck(chat.PluginName, perm, effects, "", res)
	if res.Decision == model.ModeNever {
		a.log(ledger.Entry{
			Plugin:  chat.PluginName,
			Kind:    ledger.KindBlocked,
			Message: res.Reason,
			Data:    map[string]any{"permission": perm, "title": p.Title, "session_id": sessionID},
		})
		return model.Card{}, fmt.Errorf("%w: %s", ErrBlocked, res.Reason)
	}
	body := strings.TrimSpace(p.Body)
	if p.Op == "write" && p.Content != "" {
		if body != "" {
			body += "\n\n"
		}
		body += p.Content
	}
	if p.Op == "send" && p.Content != "" {
		if body != "" {
			body += "\n\n"
		}
		body += p.Content
	}
	payload, err := json.Marshal(chatPayload{
		Op: p.Op, SessionID: sessionID, Path: p.Path, Content: p.Content, Target: p.Target,
	})
	if err != nil {
		return model.Card{}, err
	}
	kind := p.Kind
	if kind != model.KindAction && kind != model.KindChange {
		kind = model.KindChange
	}
	now := a.now().UTC()
	card := model.Card{
		ID:         id.New("c"),
		Plugin:     chat.PluginName,
		Title:      p.Title,
		Summary:    p.Summary,
		Body:       body,
		Kind:       kind,
		Permission: perm,
		Effects:    effects,
		Risk:       res.Decision,
		Status:     model.StatusPending,
		Actor:      model.ActorExternal,
		Evidence: []model.Evidence{{
			Label: "Chat",
			Ref:   sessionID,
		}},
		Payload:   payload,
		Editable:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := a.store.Update(func(st *store.State) error {
		st.Cards = append(st.Cards, card)
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	a.log(ledger.Entry{
		Plugin:   chat.PluginName,
		Kind:     ledger.KindProposed,
		CardID:   card.ID,
		Message:  card.Title,
		Evidence: card.Evidence,
		Data:     map[string]any{"risk": card.Risk, "kind": card.Kind, "effects": card.Effects, "session_id": sessionID, "op": p.Op},
	})
	return card, nil
}

func chatPermissions() []manifest.Permission {
	return []manifest.Permission{
		{Name: chat.PermWrite, Mode: manifest.ModeAskFirst, Description: "Write a file in this chat's workspace"},
		{Name: chat.PermDelete, Mode: manifest.ModeAskFirst, Description: "Delete a file in this chat's workspace"},
		{Name: chat.PermSend, Mode: manifest.ModeAskFirst, Description: "Send a message"},
	}
}

type chatPayload struct {
	Op        string `json:"op"`
	SessionID string `json:"session_id"`
	Path      string `json:"path,omitempty"`
	Content   string `json:"content,omitempty"`
	Target    string `json:"target,omitempty"`
}

func (a *App) finishChatCard(existing model.Card) (model.Card, error) {
	if existing.Status != model.StatusPending && existing.Status != model.StatusSnoozed {
		return existing, fmt.Errorf("%w: card is %s", ErrBadState, existing.Status)
	}
	var payload chatPayload
	if len(existing.Payload) > 0 && string(existing.Payload) != "null" {
		if err := json.Unmarshal(existing.Payload, &payload); err != nil {
			return model.Card{}, fmt.Errorf("change payload: %w", err)
		}
	}
	res := guardrails.Evaluate(guardrails.Request{
		Plugin:       chat.PluginName,
		Permission:   existing.Permission,
		Effects:      existing.Effects,
		Declared:     chatPermissions(),
		Overrides:    a.store.Snapshot().Rules,
		CardApproved: true,
	})
	a.rememberCheck(chat.PluginName, existing.Permission, existing.Effects, existing.ID, res)
	if res.Decision == model.ModeNever {
		_ = a.store.Update(func(st *store.State) error {
			_, i, err := indexCard(st.Cards, existing.ID)
			if err != nil {
				return err
			}
			st.Cards[i].Status = model.StatusBlocked
			st.Cards[i].Reason = res.Reason
			st.Cards[i].UpdatedAt = a.now().UTC()
			return nil
		})
		a.log(ledger.Entry{Plugin: chat.PluginName, Kind: ledger.KindBlocked, CardID: existing.ID, Message: res.Reason})
		return model.Card{}, fmt.Errorf("%w: %s", ErrBlocked, res.Reason)
	}
	if payload.Op == "send" {
		return a.approveChatSend(existing)
	}
	if err := a.store.Update(func(st *store.State) error {
		_, i, err := indexCard(st.Cards, existing.ID)
		if err != nil {
			return err
		}
		st.Cards[i].Status = model.StatusActing
		st.Cards[i].UpdatedAt = a.now().UTC()
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	a.log(ledger.Entry{Plugin: chat.PluginName, Kind: ledger.KindApproved, CardID: existing.ID, Message: "Approved “" + existing.Title + "”"})
	if err := a.applyChatPayload(existing.ID, payload); err != nil {
		return a.failCard(existing.ID, err)
	}
	summary := "Saved in the chat workspace."
	switch payload.Op {
	case "delete":
		summary = "Deleted the file in the chat workspace."
	case "record":
		summary = "Recorded the blocked action in the chat workspace. It was not run."
	}
	a.log(ledger.Entry{Plugin: chat.PluginName, Kind: ledger.KindChange, CardID: existing.ID, Message: summary})
	return a.completeCard(existing.ID, &model.ActResult{Status: "done", Summary: summary})
}

func (a *App) approveChatSend(existing model.Card) (model.Card, error) {
	summary := "Approved. Nothing was sent. Chat cannot send on its own."
	var card model.Card
	now := a.now().UTC()
	if err := a.store.Update(func(st *store.State) error {
		_, i, err := indexCard(st.Cards, existing.ID)
		if err != nil {
			return err
		}
		st.Cards[i].Status = model.StatusApproved
		st.Cards[i].UpdatedAt = now
		st.Cards[i].Result = &model.ActResult{Status: "approved", Summary: summary}
		card = cloneCard(st.Cards[i])
		return nil
	}); err != nil {
		return model.Card{}, err
	}
	a.log(ledger.Entry{Plugin: chat.PluginName, Kind: ledger.KindApproved, CardID: card.ID, Message: summary})
	return card, nil
}

func (a *App) applyChatPayload(cardID string, payload chatPayload) error {
	sys, err := a.openChat()
	if err != nil {
		return err
	}
	work, err := sys.store.WorkDir(payload.SessionID)
	if err != nil {
		return err
	}
	switch payload.Op {
	case "write":
		dest, err := chat.SafeJoin(work, payload.Path)
		if err != nil {
			return err
		}
		if len(payload.Content) > 256<<10 {
			return fmt.Errorf("file is too large")
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		return os.WriteFile(dest, []byte(payload.Content), 0o600)
	case "delete":
		dest, err := chat.SafeJoin(work, payload.Path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(dest)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("refusing to delete a directory")
		}
		return os.Remove(dest)
	case "record":
		rel := filepath.Join("proposals", cardID+".txt")
		dest, err := chat.SafeJoin(work, rel)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		body := payload.Content
		if body == "" {
			body = payload.Path
		}
		return os.WriteFile(dest, []byte(body), 0o600)
	case "send":
		return fmt.Errorf("chat does not send")
	default:
		return fmt.Errorf("unknown chat action %q", payload.Op)
	}
}

func (a *App) cardsByID(ids []string) map[string]model.Card {
	if len(ids) == 0 {
		return map[string]model.Card{}
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := map[string]model.Card{}
	for _, c := range a.store.Snapshot().Cards {
		if want[c.ID] {
			out[c.ID] = c
		}
	}
	return out
}

func cardIDs(sess chat.Session) []string {
	var ids []string
	seen := map[string]bool{}
	for _, m := range sess.Messages {
		for _, id := range m.Cards {
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func providerLabel(id string) string {
	if p, ok := chat.ByID(id); ok {
		return p.Label()
	}
	return "Chat"
}

func chatTitle(text string) string {
	text = oneLine(text, 60)
	if text == "" {
		return "New chat"
	}
	return text
}

func oneLine(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return aa == bb
}

// chatPending counts decision cards proposed from chat.
func (a *App) chatPending() int {
	n := 0
	for _, c := range a.store.Snapshot().Cards {
		if c.Plugin == chat.PluginName && c.Status == model.StatusPending {
			n++
		}
	}
	return n
}
