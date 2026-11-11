package pluginauthor

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const sdkMissingDetail = "missing vendor/plugin-sdk/dist/index.js. This plugin imports the DAP SDK. Run da plugin validate so DAP can stage the SDK under the data directory, or copy the SDK into vendor/plugin-sdk."

func missingSDK(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "vendor", "plugin-sdk", "dist", "index.js")); err == nil {
		return false
	}
	return importsSDK(dir)
}

func importsSDK(dir string) bool {
	hit := false
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".js", ".mjs", ".ts":
		default:
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if containsSDKImport(b) {
			hit = true
			return fs.SkipAll
		}
		return nil
	})
	return hit
}

func containsSDKImport(b []byte) bool {
	return bytes.Contains(b, []byte("vendor/plugin-sdk"))
}

// stageForValidate copies the plugin under dataDir and vendors the SDK there.
// The repo copy is left unchanged. dataDir is the DAP data directory.
func stageForValidate(dir, dataDir string) (string, error) {
	if dataDir == "" {
		return "", fmt.Errorf("%s", sdkMissingDetail)
	}
	stage := filepath.Join(dataDir, "validate", filepath.Base(dir))
	if err := os.RemoveAll(stage); err != nil {
		return "", err
	}
	if err := copyTree(dir, stage); err != nil {
		return "", err
	}
	if err := copySDK(filepath.Join(stage, "vendor", "plugin-sdk")); err != nil {
		return "", err
	}
	dist := filepath.Join(stage, "dist", "index.js")
	if _, err := os.Stat(dist); err == nil {
		// The copied dist can run. Skip npm unless the source is newer.
		stamp := time.Now().Add(2 * time.Second)
		if err := os.Chtimes(dist, stamp, stamp); err != nil {
			return "", err
		}
	}
	return stage, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git") {
			return fs.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	})
}
