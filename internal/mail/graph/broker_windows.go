//go:build windows

package graph

// BrokerName reports the sign-in broker. This build uses the system browser
// with the same public-client PKCE flow MSAL uses. The native Web Account
// Manager broker is not linked; DAP never reads Outlook's stored tokens.
func BrokerName() string { return "system-browser" }
