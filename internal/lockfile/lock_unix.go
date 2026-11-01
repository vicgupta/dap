//go:build !windows

package lockfile

import (
	"fmt"
	"os"
	"syscall"
)

// Lock is an exclusive non-blocking lock on a file.
type Lock struct {
	f *os.File
}

// Acquire takes an exclusive lock, creating the file if needed.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("data directory is in use")
	}
	return &Lock{f: f}, nil
}

// Release unlocks and closes the file.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}
