package pluginauthor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vicgupta/dap/internal/rundir"
)

func prepareBuild(dir, dataDir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return "", fmt.Errorf("package.json: %w", err)
	}
	if strings.TrimSpace(pkg.Scripts["build"]) == "" {
		return "", nil
	}
	if !needsBuild(dir) {
		return "dist is up to date", nil
	}
	if _, err := exec.LookPath("node"); err != nil {
		return "", fmt.Errorf("node is not on PATH. Install Node.js 20 or newer from https://nodejs.org, or install Bun from https://bun.sh and set runtime: bun")
	}
	var env []string
	if dataDir != "" {
		if err := rundir.Ensure(dataDir); err != nil {
			return "", err
		}
		env = rundir.Apply(os.Environ(), dataDir)
	}
	if st, err := os.Stat(filepath.Join(dir, "node_modules", "typescript")); err != nil || !st.IsDir() {
		cmd := exec.Command("npm", "install", "--silent")
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("npm install: %w\n%s", err, trimOut(out))
		}
	}
	cmd := exec.Command("npm", "run", "build", "--silent")
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("npm run build: %w\n%s", err, trimOut(out))
	}
	return "built with npm run build", nil
}

func needsBuild(dir string) bool {
	dist := filepath.Join(dir, "dist", "index.js")
	info, err := os.Stat(dist)
	if err != nil {
		return true
	}
	newest := info.ModTime()
	src := filepath.Join(dir, "src")
	_ = filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err == nil && fi != nil && !fi.IsDir() && fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
		return nil
	})
	return newest.After(info.ModTime())
}

func trimOut(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 800 {
		return s[len(s)-800:]
	}
	return s
}
