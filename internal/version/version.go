// Package version holds the DAP core version and the plugin interface id.
package version

const (
	// Version is the core release. Plugin manifests declare the minimum they need.
	Version = "0.1.0"
	// InterfaceV1 is the first frozen plugin contract.
	InterfaceV1 = "dap.plugin.v1"
)

// SupportedInterfaces lists contracts this core can host.
// Older entries stay here so an older plugin keeps loading.
func SupportedInterfaces() []string {
	return []string{InterfaceV1}
}

// SupportsInterface reports whether the core can speak with a plugin
// that negotiated this interface id.
func SupportsInterface(id string) bool {
	if id == "" {
		id = InterfaceV1
	}
	for _, s := range SupportedInterfaces() {
		if s == id {
			return true
		}
	}
	return false
}
