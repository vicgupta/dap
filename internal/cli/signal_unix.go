//go:build !windows

package cli

import (
	"os"
	"os/signal"
	"syscall"
)

func waitForStop(extra <-chan struct{}) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	select {
	case <-ch:
	case <-extra:
	}
	signal.Stop(ch)
}
