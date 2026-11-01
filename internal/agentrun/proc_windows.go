//go:build windows

package agentrun

import "os/exec"

func setProcGroup(cmd *exec.Cmd) {}

func killProc(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
