// Package templates is the embedded plugin studio catalog.
// Each directory is a real plugin plus a template.yaml the Create page reads.
package templates

import "embed"

// Files holds the six studio templates. template.yaml is metadata;
// the other files are the plugin the studio copies into a draft.
//
//go:embed inbox-triage follow-up-nudger daily-digest reply-drafter scheduled-checklist competitor-research
var Files embed.FS
