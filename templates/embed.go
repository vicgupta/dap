// Package templates is the embedded plugin studio catalog.
// Each directory is a real plugin plus a template.yaml the Create page reads.
package templates

import "embed"

// Files holds the six studio templates. template.yaml is metadata.
// Five of them are declarative plugin.yaml files. competitor-research
// stays a code plugin. The studio copies everything except template.yaml.
//
//go:embed inbox-triage follow-up-nudger daily-digest reply-drafter scheduled-checklist competitor-research
var Files embed.FS
