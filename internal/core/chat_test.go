package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/chat"
	"github.com/vicgupta/dap/internal/layout"
	"github.com/vicgupta/dap/internal/model"
)

func TestChatTurnProposesCardAndResume(t *testing.T) {
	app := openTest(t)
	bin := buildChatFake(t)
	events := filepath.Join(t.TempDir(), "events.ndjson")
	fence := "```dap-proposal\n{\"title\":\"Save a note\",\"summary\":\"Write notes.md in this chat\",\"op\":\"write\",\"path\":\"notes.md\",\"content\":\"hello from chat\\n\"}\n```"
	assistant, err := json.Marshal(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "tool_use", "name": "Read", "input": map[string]any{"file_path": "AGENTS.md"}},
				map[string]any{"type": "text", "text": fence},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"11111111-1111-4111-8111-111111111111"}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"I can read this. "}}}`,
		string(assistant),
		`{"type":"result","subtype":"success","result":"done","session_id":"11111111-1111-4111-8111-111111111111"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(events, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "invoke.log")
	if err := app.SetChatRunner(&chat.Runner{
		LookPath: func(string) (string, error) { return bin, nil },
		Env: func() []string {
			return []string{"DAP_FAKE_EVENTS=" + events, "DAP_FAKE_LOG=" + logPath}
		},
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := app.CreateChatSession(chat.ProviderClaude)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var cardID string
	err = app.RunChatTurn(context.Background(), sess.ID, "Please save a note", func(ev ChatEvent) error {
		if ev.Type == "text" {
			text += ev.Delta
		}
		if ev.Type == "proposal" && ev.Card != nil {
			cardID = ev.Card.ID
		}
		if ev.Type == "done" && strings.Contains(ev.Text, "dap-proposal") {
			t.Fatal("done text still contains the proposal fence")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "I can read this") {
		t.Fatal(text)
	}
	if cardID == "" {
		t.Fatal("no card")
	}
	got, err := app.Approve(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusDone {
		t.Fatalf("status %s", got.Status)
	}
	sys, err := app.openChat()
	if err != nil {
		t.Fatal(err)
	}
	work, err := sys.store.WorkDir(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(work, "notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != "hello from chat\n" {
		t.Fatalf("file %q", saved)
	}
	if _, err := os.Stat(filepath.Join(app.DataDir, "notes.md")); !os.IsNotExist(err) {
		t.Fatal("write escaped the session workspace")
	}
	led, err := os.ReadFile(filepath.Join(app.DataDir, "ledger.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"chat.session", "chat.turn", "card.proposed", "card.approved", "config.applied"} {
		if !strings.Contains(string(led), kind) {
			t.Fatalf("ledger missing %s:\n%s", kind, led)
		}
	}
	if strings.Contains(string(led), "hello from chat") {
		t.Fatal("ledger stored the file body")
	}

	// Second turn resumes the CLI session id captured from the stream.
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	follow := `{"type":"result","subtype":"success","result":"Still here.","session_id":"11111111-1111-4111-8111-111111111111"}` + "\n"
	if err := os.WriteFile(events, []byte(follow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.RunChatTurn(context.Background(), sess.ID, "And then?", func(ChatEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "--resume\t11111111-1111-4111-8111-111111111111") {
		t.Fatalf("resume argv:\n%s", logged)
	}
	if !strings.Contains(string(logged), "cwd="+work) {
		t.Fatalf("cwd:\n%s", logged)
	}
}

func TestChatSendIsNotTransmitted(t *testing.T) {
	app := openTest(t)
	bin := buildChatFake(t)
	events := filepath.Join(t.TempDir(), "events.ndjson")
	proposal := "```dap-proposal\n{\"title\":\"Email Ada\",\"summary\":\"Send the note\",\"op\":\"send\",\"target\":\"ada\",\"content\":\"Hello Ada\"}\n```"
	msg, err := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{"id": "m1", "type": "agent_message", "text": proposal},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := "{\"type\":\"thread.started\",\"thread_id\":\"thread-1\"}\n" + string(msg) + "\n{\"type\":\"turn.completed\"}\n"
	if err := os.WriteFile(events, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.SetChatRunner(&chat.Runner{
		LookPath: func(string) (string, error) { return bin, nil },
		Env:      func() []string { return []string{"DAP_FAKE_EVENTS=" + events} },
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := app.CreateChatSession("codex")
	if err != nil {
		t.Fatal(err)
	}
	var cardID string
	if err := app.RunChatTurn(context.Background(), sess.ID, "Send this to Ada", func(ev ChatEvent) error {
		if ev.Type == "proposal" && ev.Card != nil {
			cardID = ev.Card.ID
			if len(ev.Card.Effects) != 1 || ev.Card.Effects[0] != "sends" {
				t.Fatalf("effects %+v", ev.Card.Effects)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if cardID == "" {
		t.Fatal("no send card")
	}
	got, err := app.Approve(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusApproved {
		t.Fatalf("status %s", got.Status)
	}
	if got.Result == nil || !strings.Contains(got.Result.Summary, "Nothing was sent") {
		t.Fatalf("result %+v", got.Result)
	}
}

func TestChatSettingsDoNotClobberThemeOrLayout(t *testing.T) {
	app := openTest(t)
	if _, err := app.SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetLayout(layout.Settings{
		Home:  layout.View{View: layout.Board, Density: layout.Compact},
		Inbox: layout.View{View: layout.Split, Density: layout.Comfortable},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetChatProvider("codex"); err != nil {
		t.Fatal(err)
	}
	if app.Theme() != "dark" {
		t.Fatal(app.Theme())
	}
	if app.Layout().Home.View != layout.Board || app.Layout().Inbox.View != layout.Split {
		t.Fatalf("%+v", app.Layout())
	}
	if _, err := app.SetTheme("light"); err != nil {
		t.Fatal(err)
	}
	if app.ChatConfig().Provider != chat.ProviderCodex {
		t.Fatal(app.ChatConfig())
	}
	if _, err := app.SetChatProvider("opencode"); err == nil {
		t.Fatal("opencode should be rejected")
	}
	raw, err := os.ReadFile(filepath.Join(app.DataDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"theme"`, `"light"`, `"layout"`, `"board"`, `"chat"`, `"codex"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("config missing %s:\n%s", want, text)
		}
	}
}

func TestShutdownStopsChatProcess(t *testing.T) {
	app := openTest(t)
	bin := buildChatFake(t)
	events := filepath.Join(t.TempDir(), "events.ndjson")
	if err := os.WriteFile(events, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.SetChatRunner(&chat.Runner{
		LookPath: func(string) (string, error) { return bin, nil },
		Env:      func() []string { return []string{"DAP_FAKE_HOLD=1", "DAP_FAKE_EVENTS=" + events} },
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := app.CreateChatSession(chat.ProviderClaude)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- app.RunChatTurn(context.Background(), sess.ID, "hold", func(ChatEvent) error { return nil })
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !app.ChatRunning() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !app.ChatRunning() {
		t.Fatal(<-done)
	}
	if st := app.QuitStatus(); st.ChatTurns == 0 || !st.Running {
		t.Fatalf("quit status %+v", st)
	}
	if err := app.Shutdown(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown left the chat process running")
	}
	if app.ChatRunning() {
		t.Fatal("still running")
	}
}

var (
	chatFakeOnce sync.Once
	chatFakeBin  string
	chatFakeErr  error
)

func buildChatFake(t *testing.T) string {
	t.Helper()
	chatFakeOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			chatFakeErr = err
			return
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				chatFakeErr = os.ErrNotExist
				return
			}
			dir = parent
		}
		outDir, err := os.MkdirTemp("", "dap-fakecli")
		if err != nil {
			chatFakeErr = err
			return
		}
		chatFakeBin = filepath.Join(outDir, "fakecli")
		cmd := exec.Command("go", "build", "-o", chatFakeBin, "./internal/chat/fakecli")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			chatFakeErr = err
			t.Log(string(out))
		}
	})
	if chatFakeErr != nil || chatFakeBin == "" {
		t.Fatalf("build fake cli: %v", chatFakeErr)
	}
	return chatFakeBin
}
