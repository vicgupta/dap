//go:build windows

package agentrun

import "os"

func isExec(fi os.FileInfo) bool {
	return !fi.IsDir()
}
