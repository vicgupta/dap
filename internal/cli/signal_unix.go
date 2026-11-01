//go:build !windows

package cli

import (
	"os"
	"os/signal"
	"syscall"
)

func waitForStop() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	signal.Stop(ch)
}
