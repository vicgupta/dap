// Package version holds the DAP core version and the plugin interface id.
package version

const (
	// Version is the core release. Plugin manifests declare the minimum they need.
	// Release builds keep this at 0.1.0. Pre-release tags are not valid plugin semver.
	Version = "0.1.0"
	// InterfaceV1 is the first frozen plugin contract.
	InterfaceV1 = "dap.plugin.v1"
)

// Release, Commit, and Date are stamped with -ldflags on a release build.
// A local `go build` leaves them empty. `da version` omits empty fields.
var (
	Release = ""
	Commit  = ""
	Date    = ""
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
