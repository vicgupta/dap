//go:build windows

package cli

import (
	"os"
	"os/signal"
)

func waitForStop() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	signal.Stop(ch)
}
