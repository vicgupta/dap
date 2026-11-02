package chat

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestClaudeArgsAreReadOnly(t *testing.T) {
	args, err := Claude{}.Args(Turn{
		Prompt:            "hello",
		ProviderSessionID: "11111111-1111-4111-8111-111111111111",
		Resume:            false,
		RulesFile:         filepath.Join(t.TempDir(), "AGENTS.md"),
		WorkDir:           t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(args, " ")
	for _, want := range []string{
		"-p",
		"--output-format stream-json",
		"--verbose",
		"--include-partial-messages",
		"--permission-mode plan",
		"--permission-prompts none",
		"--tools Read,Glob,Grep",
		"--session-id 11111111-1111-4111-8111-111111111111",
		"--append-system-prompt-file",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	for _, ban := range []string{"dangerously", "bypass", "acceptEdits", "--resume"} {
		if strings.Contains(got, ban) {
			t.Fatalf("flag %q must not be set: %s", ban, got)
		}
	}
	if args[len(args)-1] != "hello" {
		t.Fatalf("prompt last, got %q", args[len(args)-1])
	}

	resumed, err := Claude{}.Args(Turn{
		Prompt: "next", ProviderSessionID: "11111111-1111-4111-8111-111111111111", Resume: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(resumed, " "), "--resume 11111111-1111-4111-8111-111111111111") {
		t.Fatalf("resume: %v", resumed)
	}
	if strings.Contains(strings.Join(resumed, " "), "--session-id") {
		t.Fatalf("resume should not mint a new id: %v", resumed)
	}
}

func TestCodexArgsAreReadOnly(t *testing.T) {
	work := t.TempDir()
	args, err := Codex{}.Args(Turn{Prompt: "hello", WorkDir: work})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(args, "\n")
	for _, want := range []string{"exec", "--json", "--sandbox", "read-only", "--skip-git-repo-check", "-C", work} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	for _, ban := range []string{"danger", "workspace-write", "full-access", "bypass", "--ephemeral"} {
		if strings.Contains(got, ban) {
			t.Fatalf("flag %q must not be set: %s", ban, got)
		}
	}
	if !strings.Contains(args[len(args)-1], "User message:\nhello") {
		t.Fatal("codex prompt should carry the DAP rules and the user text")
	}
	if !strings.Contains(args[len(args)-1], "read-only") {
		t.Fatal("rules missing")
	}

	resumed, err := Codex{}.Args(Turn{
		Prompt: "next", WorkDir: work, Resume: true, ProviderSessionID: "thread-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if i := indexArg(resumed, "resume"); i < 0 || resumed[i+1] != "thread-1" {
		t.Fatalf("resume placement: %v", resumed)
	}
}

func TestParseRecordedClaudeStream(t *testing.T) {
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"11111111-1111-4111-8111-111111111111"}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"AGENTS.md"}},{"type":"tool_use","name":"Write","input":{"file_path":"notes.md","content":"x"}}]}}`,
		`{"type":"result","subtype":"success","result":"Hello","session_id":"11111111-1111-4111-8111-111111111111"}`,
		`not json`,
	}
	var kinds []string
	var p Claude
	for _, line := range lines {
		evs, err := p.ParseLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range evs {
			kinds = append(kinds, ev.Kind)
			if ev.Kind == EventSession && ev.SessionID != "11111111-1111-4111-8111-111111111111" {
				t.Fatal(ev.SessionID)
			}
			if ev.Tool != nil && ev.Tool.Name == "Write" && !ev.Tool.Blocked {
				t.Fatal("write tool should be blocked")
			}
		}
	}
	joined := strings.Join(kinds, " ")
	for _, want := range []string{"session", "text", "tool", "proposal", "done"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
}

func TestParseRecordedCodexStream(t *testing.T) {
	lines := []string{
		`{"type":"thread.started","thread_id":"thread-9"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"ls","status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"Repo is quiet."}}`,
		`{"type":"item.completed","item":{"type":"file_change","changes":[{"path":"notes.md","kind":"add"}]}}`,
		`{"type":"turn.completed","usage":{"input_tokens":1}}`,
	}
	var sawSession, sawTool, sawText, sawProposal, sawDone bool
	var p Codex
	for _, line := range lines {
		evs, err := p.ParseLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range evs {
			switch ev.Kind {
			case EventSession:
				sawSession = ev.SessionID == "thread-9"
			case EventTool:
				sawTool = true
				if ev.Tool.Name == "File" && !ev.Tool.Blocked {
					t.Fatal("file change should be blocked")
				}
			case EventText:
				sawText = strings.Contains(ev.Text, "quiet")
			case EventProposal:
				sawProposal = true
			case EventDone:
				sawDone = true
			}
		}
	}
	if !sawSession || !sawTool || !sawText || !sawProposal || !sawDone {
		t.Fatalf("session %v tool %v text %v proposal %v done %v", sawSession, sawTool, sawText, sawProposal, sawDone)
	}
}

func TestProposalsAndPaths(t *testing.T) {
	text := "Before\n```dap-proposal\n{\"title\":\"Save a note\",\"summary\":\"Write notes.md\",\"op\":\"write\",\"path\":\"notes.md\",\"content\":\"hello\"}\n```\nAfter"
	clean, props := StripProposals(text)
	if strings.Contains(clean, "dap-proposal") || len(props) != 1 || props[0].Path != "notes.md" {
		t.Fatalf("clean %q props %+v", clean, props)
	}
	work := t.TempDir()
	if _, err := SafeJoin(work, "../secrets.json"); err == nil {
		t.Fatal("escaped path")
	}
	if _, err := SafeJoin(work, "/etc/passwd"); err == nil {
		t.Fatal("absolute path")
	}
	got, err := SafeJoin(work, "notes/todo.md")
	if err != nil || !strings.HasPrefix(got, work) {
		t.Fatal(got, err)
	}
	if _, err := NormalizeProvider("opencode"); err == nil {
		t.Fatal("opencode is not selectable yet")
	}
	if got, err := NormalizeProvider("claude"); err != nil || got != ProviderClaude {
		t.Fatal(got, err)
	}
}

func TestProbeDoesNotReadCredentials(t *testing.T) {
	bin := buildFake(t)
	st := &Status{
		LookPath: func(name string) (string, error) {
			if name == "codex" {
				return "", os.ErrNotExist
			}
			return bin, nil
		},
		Env: func() []string { return []string{"DAP_FAKE_AUTH=no", "PATH=" + filepath.Dir(bin)} },
	}
	probes := st.Probes(context.Background())
	if len(probes) != 2 {
		t.Fatal(len(probes))
	}
	claude := probes[0]
	if !claude.Installed || claude.SignedIn || claude.Version == "" {
		t.Fatalf("%+v", claude)
	}
	if !strings.Contains(claude.Hint, "claude auth login") {
		t.Fatal(claude.Hint)
	}
	raw := claude.Path + claude.Hint + claude.Version + claude.Login
	for _, secret := range []string{"sk-", "token", "auth.json", "api_key"} {
		if strings.Contains(strings.ToLower(raw), secret) {
			t.Fatalf("probe leaked %s", secret)
		}
	}
	codex := probes[1]
	if codex.Installed || !strings.Contains(codex.Hint, "codex login") {
		t.Fatalf("%+v", codex)
	}
}

var (
	fakeOnce sync.Once
	fakeBin  string
	fakeErr  error
)

func buildFake(t *testing.T) string {
	t.Helper()
	fakeOnce.Do(func() {
		root := moduleRoot(t)
		dir, err := os.MkdirTemp("", "dap-fakecli")
		if err != nil {
			fakeErr = err
			return
		}
		fakeBin = filepath.Join(dir, "fakecli")
		cmd := exec.Command("go", "build", "-o", fakeBin, "./internal/chat/fakecli")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			fakeErr = err
			fakeBin = ""
			t.Log(string(out))
		}
	})
	if fakeErr != nil || fakeBin == "" {
		t.Fatalf("build fake cli: %v", fakeErr)
	}
	return fakeBin
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func indexArg(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}
