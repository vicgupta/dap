//go:build windows

package cli

import (
	"os"
	"os/signal"
)

func waitForStop(extra <-chan struct{}) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	select {
	case <-ch:
	case <-extra:
	}
	signal.Stop(ch)
}
