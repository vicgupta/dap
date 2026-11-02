// Command fakecli is a stand-in for claude and codex in tests and screenshots.
// It records argv and cwd, prints a recorded JSON event stream, and can hold
// so cancel and shutdown can be observed. It has no network and no credentials.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	logInvocation()
	args := os.Args[1:]
	if isVersion(args) {
		if b, err := os.ReadFile(siblingPath("version")); err == nil && strings.TrimSpace(string(b)) != "" {
			fmt.Println(strings.TrimSpace(string(b)))
			return
		}
		fmt.Println(envOr("DAP_FAKE_VERSION", "fake 9.9.9"))
		return
	}
	if isAuth(args) {
		if os.Getenv("DAP_FAKE_AUTH") == "no" || sibling("auth.no") {
			fmt.Println(`{"authMethod":"none"}`)
			fmt.Println("Authenticated: No")
			os.Exit(1)
		}
		fmt.Println(`{"authMethod":"claude.ai"}`)
		fmt.Println("Authenticated: Yes")
		return
	}
	if d := os.Getenv("DAP_FAKE_DELAY"); d != "" {
		if n, err := time.ParseDuration(d); err == nil {
			time.Sleep(n)
		}
	}
	body := events()
	pace := paceDelay()
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fmt.Println(line)
		if pace > 0 {
			time.Sleep(pace)
		}
	}
	if os.Getenv("DAP_FAKE_HOLD") == "1" || sibling("hold") {
		time.Sleep(30 * time.Second)
	}
	if code, err := strconv.Atoi(os.Getenv("DAP_FAKE_EXIT")); err == nil {
		os.Exit(code)
	}
}

func isVersion(args []string) bool {
	for _, a := range args {
		if a == "--version" || a == "-V" {
			return true
		}
		if a == "-v" && !has(args, "-p") && !has(args, "exec") {
			return true
		}
	}
	return false
}

func isAuth(args []string) bool {
	joined := strings.Join(args, " ")
	return strings.Contains(joined, "auth status") || strings.Contains(joined, "login status")
}

func has(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func events() []byte {
	if p := os.Getenv("DAP_FAKE_EVENTS"); p != "" {
		b, err := os.ReadFile(p)
		if err == nil {
			return b
		}
	}
	if b, err := os.ReadFile(siblingPath("events.ndjson")); err == nil {
		return b
	}
	return []byte(`{"type":"result","subtype":"success","result":"ok","session_id":"11111111-1111-4111-8111-111111111111"}` + "\n")
}

func paceDelay() time.Duration {
	if v := os.Getenv("DAP_FAKE_PACE"); v != "" {
		if n, err := time.ParseDuration(v); err == nil {
			return n
		}
	}
	b, err := os.ReadFile(siblingPath("pace"))
	if err != nil {
		return 0
	}
	n, err := time.ParseDuration(strings.TrimSpace(string(b)))
	if err != nil {
		return 0
	}
	return n
}

func logInvocation() {
	path := os.Getenv("DAP_FAKE_LOG")
	if path == "" {
		path = siblingPath("invoke.log")
		if _, err := os.Stat(filepathDir(path)); err != nil {
			return
		}
	}
	cwd, _ := os.Getwd()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "cwd=%s\nargv=%s\n", cwd, strings.Join(os.Args, "\t"))
}

func sibling(name string) bool {
	_, err := os.Stat(siblingPath(name))
	return err == nil
}

func siblingPath(name string) string {
	exe, err := os.Executable()
	if err != nil {
		return name
	}
	return filepathDir(exe) + string(os.PathSeparator) + name
}

func filepathDir(path string) string {
	i := strings.LastIndex(path, string(os.PathSeparator))
	if i < 0 {
		return "."
	}
	return path[:i]
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
