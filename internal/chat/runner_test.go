package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerStreamsFakeCLIAndStaysInWorkspace(t *testing.T) {
	bin := buildFake(t)
	events := filepath.Join(t.TempDir(), "events.ndjson")
	body := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"11111111-1111-4111-8111-111111111111"}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}}`,
		`{"type":"result","subtype":"success","result":"Hello","session_id":"11111111-1111-4111-8111-111111111111"}`,
	}, "\n")
	if err := os.WriteFile(events, []byte(body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "invoke.log")
	work := t.TempDir()
	r := &Runner{
		LookPath: func(string) (string, error) { return bin, nil },
		Env: func() []string {
			return []string{"DAP_FAKE_EVENTS=" + events, "DAP_FAKE_LOG=" + logPath, "PATH=/usr/bin"}
		},
	}
	run, err := r.Start(context.Background(), "s_1", Claude{}, Turn{
		Prompt: "hi", WorkDir: work, ProviderSessionID: "11111111-1111-4111-8111-111111111111",
	})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for ev := range run.Events {
		if ev.Kind == EventText && ev.Delta {
			text += ev.Text
		}
	}
	if err := run.Wait(); err != nil {
		t.Fatal(err)
	}
	if text != "Hello" {
		t.Fatal(text)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "cwd="+work) {
		t.Fatalf("cwd not scoped:\n%s", logged)
	}
	if strings.Contains(string(logged), "dangerously") || !strings.Contains(string(logged), "--output-format\tstream-json") {
		t.Fatalf("argv:\n%s", logged)
	}
}

func TestRunnerCancelStopsHeldProcess(t *testing.T) {
	bin := buildFake(t)
	r := &Runner{
		LookPath: func(string) (string, error) { return bin, nil },
		Env:      func() []string { return []string{"DAP_FAKE_HOLD=1", "DAP_FAKE_EVENTS="} },
	}
	// An empty events file still lets the process reach the hold.
	events := filepath.Join(t.TempDir(), "empty.ndjson")
	if err := os.WriteFile(events, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.Env = func() []string { return []string{"DAP_FAKE_HOLD=1", "DAP_FAKE_EVENTS=" + events} }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run, err := r.Start(ctx, "s_hold", Claude{}, Turn{Prompt: "hi", WorkDir: t.TempDir(), ProviderSessionID: "11111111-1111-4111-8111-111111111111"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if !r.Running() {
		t.Fatal("expected a running turn")
	}
	run.Cancel()
	done := make(chan error, 1)
	go func() { done <- run.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not stop the process")
	}
	if r.Running() {
		t.Fatal("runner still tracks the turn")
	}
}
