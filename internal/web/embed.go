// Package web holds the embedded DAP interface.
package web

import "embed"

// Static is the Home, inbox, activity, guardrails, and plugins UI.
//
//go:embed static
var Static embed.FS
