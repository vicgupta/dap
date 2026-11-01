//go:build !windows

package graph

// BrokerName is how this OS signs in. Personal accounts use the system browser.
func BrokerName() string { return "system-browser" }
