// Package python embeds the thin DAP plugin helper.
package python

import _ "embed"

// Helper is dap_plugin.py, copied into Python scaffolds.
//
//go:embed dap_plugin.py
var Helper string
