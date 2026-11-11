package core

import (
	"testing"
	"time"
)

func TestPluginLockReenters(t *testing.T) {
	p := newPluginLock()
	p.Lock()
	p.Lock()
	p.Unlock()
	p.Unlock()

	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		p.Lock()
		close(held)
		<-release
		p.Unlock()
	}()
	select {
	case <-held:
	case <-time.After(time.Second):
		t.Fatal("lock was not free")
	}
	if p.TryLock() {
		t.Fatal("try should fail while another goroutine holds the lock")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if p.TryLock() {
			p.Unlock()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("lock was not released")
}
