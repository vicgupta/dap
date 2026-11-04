//go:build !windows

package secrets

import "os"

// tightenSecret leaves secrets.json readable and writable by the owner only.
func tightenSecret(path string) error {
	return os.Chmod(path, 0o600)
}
