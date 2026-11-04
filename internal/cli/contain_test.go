package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWritesNothingOutsideWorkDir(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	home := filepath.Join(outside, "home")
	tmp := filepath.Join(outside, "tmp")
	xdg := filepath.Join(outside, "xdg")
	appData := filepath.Join(outside, "appdata")
	for _, dir := range []string{filepath.Join(home, ".dap"), tmp, xdg, appData} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(home, ".dap", "marker")
	if err := os.WriteFile(marker, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "marker"), []byte("tmp"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, outside)

	if runtime.GOOS != "windows" {
		if err := chmodTree(outside, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = chmodTree(outside, 0o755) })
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_CACHE_HOME", xdg)
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("XDG_STATE_HOME", xdg)
	t.Setenv("XDG_RUNTIME_DIR", xdg)
	t.Setenv("APPDATA", appData)
	t.Setenv("LOCALAPPDATA", appData)
	t.Setenv("DAP_HOME", "")
	t.Setenv("DAP_SECRETS", "os")
	t.Setenv("PATH", tmp)

	code, out, errOut := runInPlace(t, "init")
	if code != 0 {
		t.Fatalf("init %d %s %s", code, out, errOut)
	}
	if !strings.Contains(errOut, "left the old one in place") {
		t.Fatalf("missing legacy hint: %s", errOut)
	}
	var initBody map[string]any
	if err := json.Unmarshal([]byte(out), &initBody); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	wantData := filepath.Join(cwd, ".dap")
	if initBody["data_dir"] != wantData {
		t.Fatalf("data_dir %v want %s", initBody["data_dir"], wantData)
	}

	code, out, errOut = runInPlace(t, "doctor")
	if code != 0 {
		t.Fatalf("doctor %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, `"detail": "file"`) && !strings.Contains(out, `"Detail": "file"`) && !strings.Contains(out, `file`) {
		t.Fatalf("doctor should report the file secret store: %s", out)
	}

	if runtime.GOOS != "windows" {
		if err := chmodTree(outside, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	after := snapshotTree(t, outside)
	if diff := treeDiff(before, after); diff != "" {
		t.Fatalf("wrote outside the work directory:\n%s", diff)
	}
	body, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "old" {
		t.Fatalf("legacy marker changed: %s", body)
	}
	if _, err := os.Stat(filepath.Join(wantData, "config.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wantData, "tmp")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wantData, "secrets.json")); err != nil {
		t.Fatal(err)
	}
	walkErr := filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if rel != ".dap" && !strings.HasPrefix(rel, ".dap"+string(filepath.Separator)) {
			t.Errorf("path outside .dap: %s", rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
}

func TestRelativeDAPHomeStaysInWorkDir(t *testing.T) {
	work := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DAP_HOME", "nested")
	code, out, errOut := runInPlace(t, "init")
	if code != 0 {
		t.Fatalf("init %d %s %s", code, out, errOut)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if body["data_dir"] != filepath.Join(cwd, "nested") {
		t.Fatalf("data_dir %v", body["data_dir"])
	}
}

func runInPlace(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := Run(args, Options{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr})
	return code, stdout.String(), stderr.String()
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			out[rel+"/"] = "dir"
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func treeDiff(before, after map[string]string) string {
	var b strings.Builder
	for k, v := range after {
		prev, ok := before[k]
		if !ok {
			b.WriteString("added " + k + "\n")
			continue
		}
		if prev != v {
			b.WriteString("changed " + k + "\n")
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			b.WriteString("removed " + k + "\n")
		}
	}
	return b.String()
}

func chmodTree(root string, mode os.FileMode) error {
	writable := mode&0o200 != 0
	fileMode := os.FileMode(0o444)
	if writable {
		fileMode = 0o644
	}
	var dirs, files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return err
	}
	if writable {
		for _, dir := range dirs {
			if err := os.Chmod(dir, mode); err != nil {
				return err
			}
		}
		for _, file := range files {
			if err := os.Chmod(file, fileMode); err != nil {
				return err
			}
		}
		return nil
	}
	for _, file := range files {
		if err := os.Chmod(file, fileMode); err != nil {
			return err
		}
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i], mode); err != nil {
			return err
		}
	}
	return nil
}
