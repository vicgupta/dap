package desktop

import (
	"strings"
	"testing"
)

func TestCommandPerOS(t *testing.T) {
	url := "http://127.0.0.1:7391/?theme=dark&next=1"
	cases := []struct {
		goos string
		bin  string
		args []string
	}{
		{"darwin", "open", []string{url}},
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", url}},
		{"linux", "xdg-open", []string{url}},
		{"freebsd", "xdg-open", []string{url}},
	}
	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			bin, args, err := Command(tc.goos, url)
			if err != nil {
				t.Fatal(err)
			}
			if bin != tc.bin {
				t.Fatalf("bin %q, want %q", bin, tc.bin)
			}
			if len(args) != len(tc.args) {
				t.Fatalf("args %#v", args)
			}
			for i := range tc.args {
				if args[i] != tc.args[i] {
					t.Fatalf("arg %d = %q, want %q (full %#v)", i, args[i], tc.args[i], args)
				}
			}
			joined := bin + " " + strings.Join(args, " ")
			for _, ban := range []string{
				"chrome", "Chrome", "chromium", "Chromium", "msedge", "Edge",
				"--app", "--user-data-dir", "--remote-debugging", "headless",
			} {
				if strings.Contains(joined, ban) {
					t.Fatalf("command mentions %q: %s", ban, joined)
				}
			}
		})
	}
}

func TestWindowsURLIsOneArgument(t *testing.T) {
	url := `http://127.0.0.1:9/callback?code=a&state=b^c|d`
	bin, args, err := Command("windows", url)
	if err != nil {
		t.Fatal(err)
	}
	if bin != "rundll32" {
		t.Fatal(bin)
	}
	if len(args) != 2 || args[0] != "url.dll,FileProtocolHandler" || args[1] != url {
		t.Fatalf("args %#v", args)
	}
	if strings.Contains(strings.Join(args, " "), "cmd") || args[0] == "/c" {
		t.Fatalf("URL must not be handed to cmd.exe: %#v", args)
	}
}

func TestCommandRejectsEmptyAndBrokenURL(t *testing.T) {
	if _, _, err := Command("darwin", "  "); err == nil {
		t.Fatal("empty url")
	}
	if _, _, err := Command("linux", "http://127.0.0.1/\n/etc/passwd"); err == nil {
		t.Fatal("newline url")
	}
}

func TestCloseDoesNotPanic(t *testing.T) {
	Close()
	Close()
}
