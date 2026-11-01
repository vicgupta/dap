//go:build !windows

package pluginhost

import (
	"os/exec"
	"syscall"
)

func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalProc(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}

func termProc(cmd *exec.Cmd) { signalProc(cmd, syscall.SIGTERM) }

func killProc(cmd *exec.Cmd) { signalProc(cmd, syscall.SIGKILL) }
