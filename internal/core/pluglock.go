package core

import (
	goruntime "runtime"
	"sync"
)

// pluginLock is a mutex one goroutine may lock again.
// Approving an install card already holds the plugin lock, and install
// starts and runs that same plugin.
type pluginLock struct {
	mu    sync.Mutex
	cond  *sync.Cond
	owner int64
	depth int
}

func newPluginLock() *pluginLock {
	p := &pluginLock{}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *pluginLock) Lock() {
	gid := currentGID()
	p.mu.Lock()
	for p.depth > 0 && p.owner != gid {
		p.cond.Wait()
	}
	if p.depth == 0 {
		p.owner = gid
	}
	p.depth++
	p.mu.Unlock()
}

func (p *pluginLock) TryLock() bool {
	gid := currentGID()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.depth > 0 && p.owner != gid {
		return false
	}
	if p.depth == 0 {
		p.owner = gid
	}
	p.depth++
	return true
}

func (p *pluginLock) Unlock() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.depth == 0 || p.owner != currentGID() {
		panic("plugin lock unlock")
	}
	p.depth--
	if p.depth == 0 {
		p.owner = 0
		p.cond.Broadcast()
	}
}

func currentGID() int64 {
	var buf [64]byte
	n := goruntime.Stack(buf[:], false)
	const prefix = "goroutine "
	if n < len(prefix) {
		return 0
	}
	var id int64
	for i := len(prefix); i < n; i++ {
		c := buf[i]
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + int64(c-'0')
	}
	return id
}
