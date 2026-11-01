// Package schemas holds the machine-readable plugin.yaml contract.
package schemas

import _ "embed"

// Plugin is the JSON Schema for plugin.yaml.
//
//go:embed plugin.schema.json
var Plugin string
