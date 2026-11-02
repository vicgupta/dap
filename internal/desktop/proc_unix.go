//go:build !windows

package desktop

import (
	"os/exec"
	"syscall"
)

func prepareBrowser(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalBrowser(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}

func termBrowser(cmd *exec.Cmd) { signalBrowser(cmd, syscall.SIGTERM) }

func killBrowser(cmd *exec.Cmd) { signalBrowser(cmd, syscall.SIGKILL) }
