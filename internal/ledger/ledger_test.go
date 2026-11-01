package ledger

import (
	"os"
	"sync"
	"testing"
)

func TestAppendOnlyAndConcurrent(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(l.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("ledger mode %o is too open", info.Mode().Perm())
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := l.Append(Entry{Kind: KindLog, Message: "row", Plugin: "p"}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	all, err := l.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 20 {
		t.Fatalf("got %d rows", len(all))
	}
	recent, err := l.Recent(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 3 {
		t.Fatalf("recent %d", len(recent))
	}
}
