//go:build windows

package chat

import "os/exec"

func setProcGroup(cmd *exec.Cmd) {}

func termProc(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func killProc(cmd *exec.Cmd) { termProc(cmd) }
