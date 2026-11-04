package agentrun

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vicgupta/dap/internal/rundir"
)

// UseHostPath does not change PATH. DAP does not add Homebrew, nvm, Volta,
// or directories under the home directory.
func UseHostPath() {}

func augmentPath(current, home, goos string) string {
	return joinPath(current, extraBinDirs(home, goos, dirExists))
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// extraBinDirs adds nothing. Homebrew, nvm, Volta, and home bin directories
// sit outside the folder DAP is run from.
func extraBinDirs(string, string, func(string) bool) []string {
	return nil
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

func commandEnv(agent, dataDir string) []string {
	out := make([]string, 0, len(os.Environ())+8)
	sawPath := false
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "PATH=") {
			out = append(out, e)
			sawPath = true
			continue
		}
		if agent == "opencode" && strings.HasPrefix(e, "OPENCODE_CONFIG_CONTENT=") {
			continue
		}
		out = append(out, e)
	}
	if !sawPath {
		out = append(out, "PATH="+os.Getenv("PATH"))
	}
	if agent == "opencode" {
		out = append(out, "OPENCODE_CONFIG_CONTENT="+openCodePermissions)
	}
	if dataDir != "" {
		_ = rundir.Ensure(dataDir)
		out = rundir.Apply(out, dataDir)
	}
	return out
}

func lookAugmented(name string) (string, error) {
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
