//go:build windows

package desktop

import "os/exec"

func prepareBrowser(cmd *exec.Cmd) {}

func termBrowser(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func killBrowser(cmd *exec.Cmd) { termBrowser(cmd) }
