package rundir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyKeepsHomeAndRedirectsTemp(t *testing.T) {
	dir := t.TempDir()
	if err := Ensure(dir); err != nil {
		t.Fatal(err)
	}
	env := Apply([]string{"HOME=/keep/login", "TMPDIR=/system/tmp", "PATH=/usr/bin"}, dir)
	got := map[string]string{}
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if ok {
			got[k] = v
		}
	}
	if got["HOME"] != "/keep/login" {
		t.Fatalf("HOME %q", got["HOME"])
	}
	if got["PATH"] != "/usr/bin" {
		t.Fatalf("PATH %q", got["PATH"])
	}
	if got["TMPDIR"] != filepath.Join(dir, "tmp") || got["TEMP"] != got["TMPDIR"] || got["TMP"] != got["TMPDIR"] {
		t.Fatalf("temp %#v", got)
	}
	if got["npm_config_cache"] != filepath.Join(dir, "cache", "npm") || got["NPM_CONFIG_CACHE"] != got["npm_config_cache"] {
		t.Fatalf("npm %#v", got)
	}
	if got["PIP_CACHE_DIR"] != filepath.Join(dir, "cache", "pip") {
		t.Fatalf("pip %q", got["PIP_CACHE_DIR"])
	}
	if Jobs(dir) != filepath.Join(dir, "create") || strings.HasPrefix(Jobs(dir), Temp(dir)) {
		t.Fatalf("jobs %s temp %s", Jobs(dir), Temp(dir))
	}
	if _, err := os.Stat(got["TMPDIR"]); err != nil {
		t.Fatal(err)
	}
}

func TestApplyEmptyIsIdentity(t *testing.T) {
	in := []string{"HOME=/x", "TMP=/y"}
	out := Apply(in, "")
	if strings.Join(out, "\n") != strings.Join(in, "\n") {
		t.Fatalf("%v", out)
	}
}
