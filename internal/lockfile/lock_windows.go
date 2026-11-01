//go:build windows

package lockfile

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Lock is an exclusive non-blocking lock on a file.
type Lock struct {
	f *os.File
}

var (
	kernel32     = windows.NewLazySystemDLL("kernel32.dll")
	procLockFile = kernel32.NewProc("LockFileEx")
	procUnlock   = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock   = 0x0002
	lockfileFailImmediately = 0x0001
)

// Acquire takes an exclusive lock, creating the file if needed.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	var ol windows.Overlapped
	r, _, callErr := procLockFile.Call(
		f.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r == 0 {
		_ = f.Close()
		return nil, fmt.Errorf("data directory is in use: %w", callErr)
	}
	return &Lock{f: f}, nil
}

// Release unlocks and closes the file.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	var ol windows.Overlapped
	_, _, _ = procUnlock.Call(l.f.Fd(), 0, 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
	err := l.f.Close()
	l.f = nil
	return err
}
