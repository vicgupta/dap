package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SafeJoin resolves rel inside work. Absolute paths and ".." are refused
// so an approved card cannot write outside the session workspace.
func SafeJoin(work, rel string) (string, error) {
	work = strings.TrimSpace(work)
	rel = strings.TrimSpace(rel)
	if work == "" {
		return "", fmt.Errorf("chat workspace is empty")
	}
	if rel == "" || strings.ContainsAny(rel, "\x00\r\n") {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path must stay inside the chat workspace")
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path must stay inside the chat workspace")
	}
	absWork, err := filepath.Abs(work)
	if err != nil {
		return "", err
	}
	full := filepath.Join(absWork, clean)
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if absFull != absWork && !strings.HasPrefix(absFull, absWork+string(os.PathSeparator)) {
		return "", fmt.Errorf("path must stay inside the chat workspace")
	}
	return absFull, nil
}
