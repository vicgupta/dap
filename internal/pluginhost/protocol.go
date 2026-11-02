// Package pluginhost runs each plugin as its own process.
// The core speaks JSON-RPC 2.0, one message per line, on stdin/stdout.
// Plugins call back through the scoped HTTP API, not on the same stream,
// so a plugin can be any language that can read and write a line of JSON.
package pluginhost

import (
	"time"

	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/version"
)

// Methods of interface dap.plugin.v1. The names are part of the contract.
const (
	MethodInitialize = "initialize"
	MethodWatch      = "watch"
	MethodPropose    = "propose"
	MethodAct        = "act"
	MethodReport     = "report"
	MethodShutdown   = "shutdown"
)

// InitParams is the argument to initialize.
type InitParams struct {
	Interface  string         `json:"interface"`
	DAPVersion string         `json:"dap_version"`
	Config     map[string]any `json:"config"`
	PluginDir  string         `json:"plugin_dir"`
	DataDir    string         `json:"data_dir"`
	APIBase    string         `json:"api_base"`
	Token      string         `json:"token"`
}

// InitResult is what a plugin returns once it has loaded config and checked its sources.
type InitResult struct {
	Ready     bool         `json:"ready"`
	Interface string       `json:"interface"`
	Health    model.Health `json:"health"`
}

// WatchParams asks for observations since the previous successful run.
type WatchParams struct {
	Since *time.Time `json:"since,omitempty"`
}

// WatchResult is observations plus the home tile.
type WatchResult struct {
	Observations []model.Observation `json:"observations"`
	Tile         model.Tile          `json:"tile"`
}

// ProposeParams feeds the observations that watch just returned.
type ProposeParams struct {
	Observations []model.Observation `json:"observations"`
}

// ProposeResult is the cards the plugin wants in the inbox.
type ProposeResult struct {
	Cards []model.CardDraft `json:"cards"`
}

// ActParams is an approved card. The core only sends this after guardrails pass.
type ActParams struct {
	Card model.Card `json:"card"`
}

// ReportParams asks for metrics over a period.
type ReportParams struct {
	Period string    `json:"period"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
}

// ReportResult is the plugin's metrics.
type ReportResult struct {
	Metrics []model.Metric `json:"metrics"`
}

// ShutdownParams tells a plugin that DAP is quitting.
// The plugin should finish its own in-flight work and return.
// A missing method is not fatal: the core still stops the process.
type ShutdownParams struct {
	Reason string `json:"reason,omitempty"`
}

// ShutdownResult is the optional reply to shutdown.
type ShutdownResult struct {
	OK bool `json:"ok"`
}

// Negotiated is the interface id a plugin agreed to, defaulting to v1.
func Negotiated(result string, requested string) (string, error) {
	if result == "" {
		result = requested
	}
	if requested == "" {
		requested = version.InterfaceV1
	}
	if !version.SupportsInterface(result) {
		return "", errUnsupported(result)
	}
	return result, nil
}

type unsupported string

func errUnsupported(id string) error { return unsupported(id) }

func (u unsupported) Error() string { return "plugin negotiated unsupported interface " + string(u) }
