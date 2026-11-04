//go:build windows

package agentrun

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobs maps a process id to the job object that owns its tree.
// Closing or terminating the job kills every child, including node and npm,
// without an administrator. A standard user can assign their own processes.
var jobs sync.Map

func command(ctx context.Context, bin string, args []string) *exec.Cmd {
	lower := strings.ToLower(bin)
	if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") {
		// CreateProcess cannot run a batch shim. cmd.exe /c can, and the prompt
		// stays on stdin so the shim's %* does not split it.
		comspec := os.Getenv("ComSpec")
		if comspec == "" {
			comspec = "cmd.exe"
		}
		all := append([]string{"/d", "/s", "/c", bin}, args...)
		return exec.CommandContext(ctx, comspec, all...)
	}
	return exec.CommandContext(ctx, bin, args...)
}

func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
}

func attachJob(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	assignErr := windows.AssignProcessToJobObject(job, proc)
	_ = windows.CloseHandle(proc)
	if assignErr != nil {
		_ = windows.CloseHandle(job)
		return
	}
	jobs.Store(cmd.Process.Pid, job)
}

func killProc(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if v, ok := jobs.LoadAndDelete(pid); ok {
		h := v.(windows.Handle)
		_ = windows.TerminateJobObject(h, 1)
		_ = windows.CloseHandle(h)
	}
	// taskkill /T /F covers a child that escaped the job. It does not need admin
	// for processes this user started.
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
