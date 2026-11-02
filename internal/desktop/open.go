// Package desktop opens the local UI in a chromeless browser window when one is installed.
package desktop

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type browserProc struct {
	cmd  *exec.Cmd
	done chan struct{}
}

var browser struct {
	mu   sync.Mutex
	proc *browserProc
}

// Open shows url in an app-style window. It falls back to the system browser.
// This does not take ownership of the process. Mail sign-in uses it.
func Open(url string) error {
	return open(url, "")
}

// OpenApp shows url in a private chromeless profile under dataDir.
// Close stops that process on quit so the window does not outlive DAP.
func OpenApp(url, dataDir string) error {
	return open(url, dataDir)
}

func open(url, dataDir string) error {
	profile := ""
	if dataDir != "" {
		profile = filepath.Join(dataDir, "browser-profile")
		if err := os.MkdirAll(profile, 0o700); err != nil {
			profile = ""
		}
	}
	var last error
	for _, c := range launches(url, profile) {
		bin, err := look(c.bin)
		if err != nil {
			last = err
			continue
		}
		cmd := exec.Command(bin, c.args...)
		cmd.Stdout = nil
		cmd.Stderr = nil
		// Only the private app profile is ours to stop. A sign-in window and
		// the xdg-open / open fallbacks stay with the user's browser.
		own := c.own && profile != ""
		if own {
			prepareBrowser(cmd)
		}
		if err := cmd.Start(); err != nil {
			last = err
			continue
		}
		if own {
			remember(cmd)
		} else {
			go func() { _ = cmd.Wait() }()
		}
		return nil
	}
	if last == nil {
		last = errors.New("no browser found")
	}
	return last
}

// Close stops the chromeless window started by OpenApp, if that process is still up.
func Close() {
	browser.mu.Lock()
	p := browser.proc
	browser.proc = nil
	browser.mu.Unlock()
	if p == nil {
		return
	}
	termBrowser(p.cmd)
	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
		killBrowser(p.cmd)
		<-p.done
	}
}

func remember(cmd *exec.Cmd) {
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	browser.mu.Lock()
	browser.proc = &browserProc{cmd: cmd, done: done}
	browser.mu.Unlock()
}

type launch struct {
	bin  string
	args []string
	own  bool
}

func launches(url, profile string) []launch {
	app := func(args ...string) []string {
		var out []string
		if profile != "" {
			out = append(out, "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check")
		}
		return append(out, args...)
	}
	switch runtime.GOOS {
	case "darwin":
		return []launch{
			{bin: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", args: app("--app="+url, "--new-window"), own: true},
			{bin: "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", args: app("--app="+url, "--new-window"), own: true},
			{bin: "open", args: []string{url}},
		}
	case "windows":
		return []launch{
			{bin: "msedge", args: app("--app=" + url), own: true},
			{bin: "chrome", args: app("--app=" + url), own: true},
			{bin: "cmd", args: []string{"/c", "start", "", url}},
		}
	default:
		return []launch{
			{bin: "google-chrome", args: app("--app="+url, "--new-window"), own: true},
			{bin: "google-chrome-stable", args: app("--app="+url, "--new-window"), own: true},
			{bin: "chromium", args: app("--app="+url, "--new-window"), own: true},
			{bin: "chromium-browser", args: app("--app="+url, "--new-window"), own: true},
			{bin: "xdg-open", args: []string{url}},
		}
	}
}

func look(bin string) (string, error) {
	if len(bin) > 0 && (bin[0] == '/' || (len(bin) > 2 && bin[1] == ':')) {
		if _, err := os.Stat(bin); err != nil {
			return "", err
		}
		return bin, nil
	}
	return exec.LookPath(bin)
}
