package agentrun

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// UseHostPath prepends the directories a macOS app launched outside a shell
// would otherwise miss (Homebrew, nvm, Volta, ~/.local/bin). It is safe to call
// more than once. Other operating systems keep the process PATH as it is.
func UseHostPath() {
	pathOnce.Do(func() {
		next := augmentPath(os.Getenv("PATH"), homeDir(), runtime.GOOS)
		if next != "" && next != os.Getenv("PATH") {
			_ = os.Setenv("PATH", next)
		}
	})
}

var pathOnce sync.Once

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

func augmentPath(current, home, goos string) string {
	return joinPath(current, extraBinDirs(home, goos, dirExists))
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// extraBinDirs is the macOS lookup list, newest nvm first. exists reports
// whether a directory should be added. linux and windows return nothing.
func extraBinDirs(home, goos string, exists func(string) bool) []string {
	if goos != "darwin" || exists == nil {
		return nil
	}
	candidates := []string{"/opt/homebrew/bin", "/usr/local/bin"}
	if home != "" {
		candidates = append(candidates, nvmBins(home)...)
		candidates = append(candidates,
			filepath.Join(home, ".volta", "bin"),
			filepath.Join(home, ".local", "bin"),
		)
	}
	var out []string
	seen := map[string]bool{}
	for _, dir := range candidates {
		if dir == "" || seen[dir] || !exists(dir) {
			continue
		}
		seen[dir] = true
		out = append(out, dir)
	}
	return out
}

func nvmBins(home string) []string {
	root := filepath.Join(home, ".nvm", "versions", "node")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool { return versionNewer(names[i], names[j]) })
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, filepath.Join(root, name, "bin"))
	}
	return out
}

// versionNewer reports whether a should be searched before b. Names look like v22.2.0.
func versionNewer(a, b string) bool {
	as := versionParts(a)
	bs := versionParts(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av != bv {
			return av > bv
		}
	}
	return a > b
}

func versionParts(s string) []int {
	s = strings.TrimPrefix(strings.ToLower(s), "v")
	var out []int
	n := 0
	saw := false
	flush := func() {
		if saw {
			out = append(out, n)
			n = 0
			saw = false
		}
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
			saw = true
			continue
		}
		flush()
	}
	flush()
	return out
}

func joinPath(current string, front []string) string {
	sep := string(os.PathListSeparator)
	seen := map[string]bool{}
	var parts []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		parts = append(parts, p)
	}
	for _, dir := range front {
		add(dir)
	}
	for _, dir := range strings.Split(current, sep) {
		add(dir)
	}
	return strings.Join(parts, sep)
}

func commandEnv(agent string) []string {
	path := augmentPath(os.Getenv("PATH"), homeDir(), runtime.GOOS)
	out := make([]string, 0, len(os.Environ())+2)
	sawPath := false
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "PATH=") {
			out = append(out, "PATH="+path)
			sawPath = true
			continue
		}
		if agent == "opencode" && strings.HasPrefix(e, "OPENCODE_CONFIG_CONTENT=") {
			continue
		}
		out = append(out, e)
	}
	if !sawPath {
		out = append(out, "PATH="+path)
	}
	if agent == "opencode" {
		out = append(out, "OPENCODE_CONFIG_CONTENT="+openCodePermissions)
	}
	return out
}

func lookAugmented(name string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		return exec.LookPath(name)
	}
	path := augmentPath(os.Getenv("PATH"), homeDir(), runtime.GOOS)
	if found, err := lookIn(path, name); err == nil {
		return found, nil
	}
	return exec.LookPath(name)
}

func lookIn(path, name string) (string, error) {
	for _, dir := range strings.Split(path, string(os.PathListSeparator)) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		fi, err := os.Stat(candidate)
		if err != nil || fi.IsDir() || !isExec(fi) {
			continue
		}
		return candidate, nil
	}
	return "", os.ErrNotExist
}
