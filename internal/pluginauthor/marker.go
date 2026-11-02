package pluginauthor

import (
	"bytes"
	"os"
	"path/filepath"
)

// ScaffoldMarker is the starter line the agent must remove. A draft that still
// contains it has not replaced the idle scaffold.
const ScaffoldMarker = "DAP_SCAFFOLD_STARTER"

// MarkerPresent reports whether a plugin source still has the starter line.
func MarkerPresent(dir string) bool {
	for _, rel := range []string{"src/index.ts", "plugin.py", "plugin.js"} {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			continue
		}
		if bytes.Contains(b, []byte(ScaffoldMarker)) {
			return true
		}
	}
	return false
}
