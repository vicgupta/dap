//go:build windows

package lockfile

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// Lock is an exclusive non-blocking lock on a file.
type Lock struct {
	f  *os.File
	ol windows.Overlapped
}

// Acquire takes an exclusive lock, creating the file if needed.
//
// LockFileEx requires a live OVERLAPPED pointer. Passing NULL makes the
// kernel read address 0x10 and the process dies with 0xc0000005 before
// `da run` can listen. The struct stays on Lock so it outlives the call.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	l := &Lock{f: f}
	err = windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&l.ol,
	)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("data directory is in use: %w", err)
	}
	return l, nil
}

// Release unlocks and closes the file.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, 1, 0, &l.ol)
	err := l.f.Close()
	l.f = nil
	return err
}
