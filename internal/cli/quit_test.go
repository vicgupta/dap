package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vicgupta/dap/internal/lockfile"
	"github.com/vicgupta/dap/internal/runinfo"
)

func TestQuitWhenStopped(t *testing.T) {
	dir := t.TempDir()
	for _, cmd := range []string{"quit", "stop"} {
		code, _, errOut := run(t, dir, cmd)
		if code != 1 || !strings.Contains(errOut, "DAP is not running") {
			t.Fatalf("%s code %d %s", cmd, code, errOut)
		}
	}
}

func TestQuitStopsServe(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- Run([]string{"--data", dir, "serve", "--addr", "127.0.0.1:0"}, Options{
			Stdin: bytes.NewReader(nil), Stdout: &stdout, Stderr: &stderr,
		})
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := runinfo.Read(dir); err == nil {
			break
		}
		select {
		case code := <-done:
			t.Fatalf("serve exited early %d\n%s\n%s", code, stdout.String(), stderr.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("run.json missing\n%s\n%s", stdout.String(), stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	code, out, errOut := run(t, dir, "quit")
	if code != 0 {
		t.Fatalf("quit %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, `"ok": true`) {
		t.Fatalf("quit body %s", out)
	}
	select {
	case serveCode := <-done:
		if serveCode != 0 {
			t.Fatalf("serve exit %d\n%s\n%s", serveCode, stdout.String(), stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not exit after quit")
	}
	if _, err := os.Stat(runinfo.Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("run.json: %v", err)
	}
	lk, err := lockfile.Acquire(filepath.Join(dir, "dap.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := lk.Release(); err != nil {
		t.Fatal(err)
	}
}
