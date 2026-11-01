// Package version holds the DAP core version and the plugin interface id.
package version

const (
	// Version is the core release. Plugin manifests declare the minimum they need.
	// Release tags such as v0.1.0-alpha.1 stay in Release so semver checks keep working.
	Version = "0.1.0"
	// InterfaceV1 is the first frozen plugin contract.
	InterfaceV1 = "dap.plugin.v1"
)

// Release is the build label, set with -ldflags "-X github.com/vicgupta/dap/internal/version.Release=v0.1.0-alpha.1".
// Empty means a local build of Version.
var Release = ""

// Display is what `da version` prints.
func Display() string {
	if Release != "" {
		return Release
	}
	return Version
}

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
