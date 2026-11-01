// Package permissions turns a manifest declaration into a plain-language decision.
// Anything that was not declared is denied. Modes are allowed or ask first.
package permissions

import (
	"strings"

	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
)

// Decision is the result of looking a permission up in a plugin's manifest.
type Decision struct {
	// Mode is allowed, ask_first, or never.
	Mode string
	// Plain is one sentence a person can approve.
	Plain string
	// Declared is false when the plugin never asked for this permission.
	Declared bool
}

// Lookup decides what a plugin may do with permission name.
// An empty name means the call does not exercise a declared capability
// (for example writing the plugin's own tile). Those are allowed.
func Lookup(declared []manifest.Permission, name string) Decision {
	if strings.TrimSpace(name) == "" {
		return Decision{Mode: model.ModeAllowed, Plain: "No extra permission.", Declared: true}
	}
	for _, p := range declared {
		if p.Name == name {
			mode := manifest.NormalizeMode(p.Mode)
			if mode != model.ModeAllowed && mode != model.ModeAskFirst {
				mode = model.ModeAskFirst
			}
			return Decision{
				Mode:     mode,
				Plain:    Plain(p),
				Declared: true,
			}
		}
	}
	return Decision{
		Mode:     model.ModeNever,
		Plain:    humanize(name) + " was not declared, so it is denied.",
		Declared: false,
	}
}

// Plain renders a permission the way the install screen shows it.
func Plain(p manifest.Permission) string {
	mode := "ask first"
	if manifest.NormalizeMode(p.Mode) == model.ModeAllowed {
		mode = "allowed"
	}
	desc := strings.TrimSpace(p.Description)
	if desc == "" {
		desc = humanize(p.Name)
	}
	return desc + " — " + mode
}

func humanize(name string) string {
	switch name {
	case "mail.read":
		return "Read mail"
	case "mail.draft":
		return "Draft mail, without sending"
	case "mail.send":
		return "Send mail"
	case "web.read":
		return "Read public web pages"
	case "files.write":
		return "Write local files"
	case "share.team":
		return "Share with the team"
	case "secrets.read":
		return "Read this plugin's saved secrets"
	case "secrets.write":
		return "Save secrets for this plugin"
	default:
		return name
	}
}
