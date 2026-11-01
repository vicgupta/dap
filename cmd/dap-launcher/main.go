// Command dap-launcher is the DAP.app executable. Double-click runs `da run`.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	da := filepath.Join(filepath.Dir(exe), "da")
	cmd := exec.Command(da, "run")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
