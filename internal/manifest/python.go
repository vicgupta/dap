package manifest

import (
	"os/exec"
	"runtime"
)

// PythonBin is the interpreter used for .py entrypoints.
func PythonBin() string {
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("python"); err == nil {
			return p
		}
		if p, err := exec.LookPath("py"); err == nil {
			return p
		}
		return "python"
	}
	if p, err := exec.LookPath("python3"); err == nil {
		return p
	}
	return "python3"
}
