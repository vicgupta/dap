//go:build !windows

package agentrun

import (
	"context"
	"os/exec"
	"syscall"
)

func command(ctx context.Context, bin string, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, bin, args...)
}

func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// attachJob is a no-op. setProcGroup already puts the process in its own group,
// and killProc signals that group so children such as npm exit with the agent.
func attachJob(cmd *exec.Cmd) {}

func killProc(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
