// Package desktop opens the local UI in a chromeless browser window when one is installed.
package desktop

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
)

// Open shows url in an app-style window. It falls back to the system browser.
func Open(url string) error {
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{
			{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "--app=" + url, "--new-window"},
			{"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", "--app=" + url, "--new-window"},
			{"open", url},
		}
	case "windows":
		candidates = [][]string{
			{"msedge", "--app=" + url},
			{"chrome", "--app=" + url},
			{"cmd", "/c", "start", "", url},
		}
	default:
		candidates = [][]string{
			{"google-chrome", "--app=" + url, "--new-window"},
			{"google-chrome-stable", "--app=" + url, "--new-window"},
			{"chromium", "--app=" + url, "--new-window"},
			{"chromium-browser", "--app=" + url, "--new-window"},
			{"xdg-open", url},
		}
	}
	var last error
	for _, c := range candidates {
		bin, err := look(c[0])
		if err != nil {
			last = err
			continue
		}
		cmd := exec.Command(bin, c[1:]...)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			last = err
			continue
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}
	if last == nil {
		last = errors.New("no browser found")
	}
	return last
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
