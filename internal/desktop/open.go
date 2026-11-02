// Package desktop opens a URL in the operating system's default browser.
//
// DAP does not look for Chrome, Edge, Chromium, or Brave, and it does not
// pass --app, a private user-data-dir, or a remote-debugging port. Any
// browser the person has set as the default can show the UI: Chrome, Edge,
// Firefox, Brave, or Safari. Quit does not stop that browser.
package desktop

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Open shows url in the default browser. The process is not owned by DAP.
func Open(url string) error {
	return openOS(runtime.GOOS, url)
}

// OpenApp shows url in the default browser. dataDir is ignored.
// Older builds stored a private Chromium profile there. This build does not
// create one and does not close the browser on quit.
func OpenApp(url, dataDir string) error {
	_ = dataDir
	return Open(url)
}

// Close does nothing. The default browser belongs to the person, so quit
// must not kill it. The page asks them to close the tab.
func Close() {}

// Command is the default-browser invocation for goos.
// The URL is one argument. It is not parsed by a shell.
//
// Windows uses rundll32 url.dll,FileProtocolHandler. That handler does not
// need elevation, and it does not pass the URL through cmd.exe, so an OAuth
// query string that contains & stays intact. cmd /c start would treat & as
// a command separator and would treat the first quoted token as a window title.
func Command(goos, url string) (string, []string, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", nil, errors.New("url is empty")
	}
	if strings.ContainsAny(url, "\r\n\x00") {
		return "", nil, errors.New("url is invalid")
	}
	switch goos {
	case "darwin":
		return "open", []string{url}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	default:
		return "xdg-open", []string{url}, nil
	}
}

func openOS(goos, url string) error {
	bin, args, err := Command(goos, url)
	if err != nil {
		return err
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return fmt.Errorf("no default browser command %q: %w", bin, err)
	}
	cmd := exec.Command(path, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
