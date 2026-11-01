package secrets

import (
	"os"
	"testing"
)

func TestFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	k, err := NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if k.Backend() != "file" {
		t.Fatal(k.Backend())
	}
	if _, err := k.Get("dap.plugin.workflow", "token"); err != ErrNotFound {
		t.Fatalf("got %v", err)
	}
	if err := k.Set("dap.plugin.workflow", "token", "s3cret"); err != nil {
		t.Fatal(err)
	}
	got, err := k.Get("dap.plugin.workflow", "token")
	if err != nil {
		t.Fatal(err)
	}
	if got != "s3cret" {
		t.Fatal(got)
	}
	// A different service does not see it.
	if _, err := k.Get("dap.plugin.other", "token"); err != ErrNotFound {
		t.Fatal("services must be isolated")
	}
	if err := k.Delete("dap.plugin.workflow", "token"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get("dap.plugin.workflow", "token"); err != ErrNotFound {
		t.Fatal("expected delete")
	}
	info, err := os.Stat(dir + "/secrets.json")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
}
