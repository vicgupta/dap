// Package rundir keeps DAP's temporary files and tool caches inside the data directory.
package rundir

import (
	"os"
	"path/filepath"
	"strings"
)

// Temp is <dataDir>/tmp.
func Temp(dataDir string) string { return filepath.Join(dataDir, "tmp") }

// NpmCache is the npm cache DAP points child processes at.
func NpmCache(dataDir string) string { return filepath.Join(dataDir, "cache", "npm") }

// PipCache is the pip cache DAP points child processes at.
func PipCache(dataDir string) string { return filepath.Join(dataDir, "cache", "pip") }

// Ensure creates the temp directory and the npm and pip caches.
func Ensure(dataDir string) error {
	if dataDir == "" {
		return nil
	}
	for _, dir := range []string{Temp(dataDir), NpmCache(dataDir), PipCache(dataDir)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// Apply points temp and cache variables at dataDir.
// HOME is left unchanged. An empty dataDir returns env as it was.
func Apply(env []string, dataDir string) []string {
	if dataDir == "" {
		return env
	}
	set := map[string]string{
		"TMPDIR":           Temp(dataDir),
		"TEMP":             Temp(dataDir),
		"TMP":              Temp(dataDir),
		"npm_config_cache": NpmCache(dataDir),
		"NPM_CONFIG_CACHE": NpmCache(dataDir),
		"PIP_CACHE_DIR":    PipCache(dataDir),
	}
	order := []string{"TMPDIR", "TEMP", "TMP", "npm_config_cache", "NPM_CONFIG_CACHE", "PIP_CACHE_DIR"}
	seen := map[string]bool{}
	out := make([]string, 0, len(env)+len(order))
	for _, e := range env {
		key, _, ok := strings.Cut(e, "=")
		if !ok {
			out = append(out, e)
			continue
		}
		if v, repl := set[key]; repl {
			if seen[key] {
				continue
			}
			out = append(out, key+"="+v)
			seen[key] = true
			continue
		}
		out = append(out, e)
	}
	for _, key := range order {
		if seen[key] {
			continue
		}
		out = append(out, key+"="+set[key])
	}
	return out
}
