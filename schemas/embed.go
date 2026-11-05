// Package schemas holds the machine-readable plugin.yaml contract.
package schemas

import _ "embed"

// Plugin is the JSON Schema for a code plugin.yaml.
//
//go:embed plugin.schema.json
var Plugin string

// Declarative is the JSON Schema for a kind: declarative plugin.yaml.
//
//go:embed declarative.schema.json
var Declarative string
