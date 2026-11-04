package core

import (
	"context"
	"sync"
	"time"

	"github.com/vicgupta/dap/internal/ledger"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginhost"
	"github.com/vicgupta/dap/internal/runinfo"
)

const (
	// ShutdownActTimeout is how long quit waits for an in-flight act,
	// and for any other plugin call still holding that process.
	ShutdownActTimeout = 10 * time.Second
	// ShutdownPluginTimeout is how long one plugin may take to answer shutdown.
	ShutdownPluginTimeout = 3 * time.Second
	// After processes are signaled, in-flight calls unwind and write the ledger.
	shutdownSettleTimeout = 2 * time.Second
)

// QuitStatus tells the UI whether quit needs a confirm, and why.
type QuitStatus struct {
	Running         bool `json:"running"`
	PluginsRunning  int  `json:"plugins_running"`
	InflightActions int  `json:"inflight_actions"`
	Acting          int  `json:"acting"`
	ChatTurns       int  `json:"chat_turns"`
}

// QuitStatus reports running plugins and work that quit should mention.
func (a *App) QuitStatus() QuitStatus {
	a.mu.Lock()
	ops := a.inflightOps
	acts := a.inflightActs
	chatTurns := 0
	if a.chatSys != nil && a.chatSys.runner != nil && a.chatSys.runner.Running() {
		chatTurns = 1
	}
	a.mu.Unlock()
	running := 0
	for _, name := range a.host.Names() {
		if a.host.Running(name) {
			running++
		}
	}
	acting := 0
	for _, card := range a.store.Snapshot().Cards {
		if card.Status == model.StatusActing {
			acting++
		}
	}
	if acts > acting {
		acting = acts
	}
	return QuitStatus{
		Running:         running > 0 || ops > 0 || acting > 0 || chatTurns > 0,
		PluginsRunning:  running,
		InflightActions: ops,
		Acting:          acting,
		ChatTurns:       chatTurns,
	}
}

// Shutdown stops new work, drains in-flight act calls, asks plugins to exit,
// flushes the ledger and state, removes run.json, and releases dap.lock.
// It is safe to call more than once. The admin HTTP server is stopped by the caller.
func (a *App) Shutdown() error {
	a.shutOnce.Do(func() {
		a.shutErr = a.shutdown()
	})
	return a.shutErr
}

func (a *App) shutdown() error {
	a.mu.Lock()
	a.stopping = true
	a.mu.Unlock()
	a.cancelCreateJobs()
	a.StopScheduler()
	a.stopChat()
	a.waitIdle(ShutdownActTimeout)
	a.shutdownPlugins()
	a.waitIdle(shutdownSettleTimeout)
	if a.apiSrv != nil {
		_ = a.apiSrv.Close()
		a.apiSrv = nil
	}
	var err error
	if a.ledger != nil {
		if e := a.ledger.Close(); e != nil {
			err = e
		}
	}
	if a.store != nil {
		if e := a.store.Flush(); e != nil && err == nil {
			err = e
		}
	}
	if e := runinfo.Remove(a.DataDir); e != nil && err == nil {
		err = e
	}
	if a.lock != nil {
		if e := a.lock.Release(); e != nil && err == nil {
			err = e
		}
		a.lock = nil
	}
	return err
}

func (a *App) shutdownPlugins() {
	names := a.host.Names()
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			a.shutdownOne(name)
		}(name)
	}
	wg.Wait()
}

func (a *App) shutdownOne(name string) {
	if !a.host.Running(name) {
		a.host.Stop(name)
		return
	}
	if !a.pluginBusy(name) {
		ctx, cancel := context.WithTimeout(context.Background(), ShutdownPluginTimeout)
		_ = a.host.Call(ctx, name, pluginhost.MethodShutdown, pluginhost.ShutdownParams{Reason: "quit"}, nil)
		cancel()
	}
	a.host.Stop(name)
	a.log(ledger.Entry{
		Plugin:  name,
		Kind:    ledger.KindStop,
		Message: "Stopped " + name,
	})
}

// beginCall marks one plugin busy until the returned function runs.
// Calls fail once shutdown has started, so quit does not race a new act.
func (a *App) beginCall(name string, act bool) (func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopping {
		return func() {}, ErrStopping
	}
	if a.busy == nil {
		a.busy = map[string]int{}
	}
	a.inflightOps++
	if act {
		a.inflightActs++
	}
	a.busy[name]++
	return func() {
		a.mu.Lock()
		a.inflightOps--
		if act && a.inflightActs > 0 {
			a.inflightActs--
		}
		if a.busy[name] > 0 {
			a.busy[name]--
			if a.busy[name] == 0 {
				delete(a.busy, name)
			}
		}
		a.mu.Unlock()
	}, nil
}

func (a *App) pluginBusy(name string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.busy[name] > 0
}

func (a *App) isStopping() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stopping
}

func (a *App) waitIdle(d time.Duration) {
	deadline := time.Now().Add(d)
	for {
		a.mu.Lock()
		n := a.inflightOps
		a.mu.Unlock()
		if n == 0 || !time.Now().Before(deadline) {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
}
