package pluginauthor

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vicgupta/dap/templates"
	"gopkg.in/yaml.v3"
)

const (
	// ModeInstall copies a finished plugin. Settings are enough; no agent runs.
	ModeInstall = "install"
	// ModeCustomize copies a starting plugin and then runs the coding agent.
	ModeCustomize = "customize"
)

// Setting is one field the Create form shows for a template.
type Setting struct {
	Key         string `json:"key" yaml:"key"`
	Type        string `json:"type" yaml:"type"`
	Label       string `json:"label" yaml:"label"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
}

// Template is one studio card.
type Template struct {
	ID          string    `json:"id" yaml:"id"`
	Title       string    `json:"title" yaml:"title"`
	Blurb       string    `json:"blurb" yaml:"blurb"`
	Mode        string    `json:"mode" yaml:"mode"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	Permissions []string  `json:"permissions" yaml:"permissions"`
	Connections []string  `json:"connections,omitempty" yaml:"connections,omitempty"`
	Settings    []Setting `json:"settings,omitempty" yaml:"settings,omitempty"`
}

// templateOrder is the Create page order.
var templateOrder = []string{
	"inbox-triage",
	"follow-up-nudger",
	"daily-digest",
	"reply-drafter",
	"scheduled-checklist",
	"competitor-research",
}

// ListTemplates reads every embedded template.yaml.
func ListTemplates() ([]Template, error) {
	out := make([]Template, 0, len(templateOrder))
	seen := map[string]bool{}
	for _, id := range templateOrder {
		t, err := readTemplate(id)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		seen[id] = true
	}
	entries, err := fs.ReadDir(templates.Files, ".")
	if err != nil {
		return nil, err
	}
	var extra []string
	for _, e := range entries {
		if e.IsDir() && !seen[e.Name()] {
			extra = append(extra, e.Name())
		}
	}
	sort.Strings(extra)
	for _, id := range extra {
		t, err := readTemplate(id)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// Lookup returns one template by id.
func Lookup(id string) (Template, error) {
	return readTemplate(id)
}

func readTemplate(id string) (Template, error) {
	raw, err := templates.Files.ReadFile(filepath.ToSlash(filepath.Join(id, "template.yaml")))
	if err != nil {
		return Template{}, fmt.Errorf("unknown template %s", id)
	}
	var t Template
	if err := yaml.Unmarshal(raw, &t); err != nil {
		return Template{}, fmt.Errorf("template %s: %w", id, err)
	}
	if t.ID == "" {
		t.ID = id
	}
	if t.ID != id {
		return Template{}, fmt.Errorf("template %s: id is %s", id, t.ID)
	}
	if t.Mode != ModeInstall && t.Mode != ModeCustomize {
		return Template{}, fmt.Errorf("template %s: mode must be install or customize", id)
	}
	if t.Title == "" || t.Blurb == "" {
		return Template{}, fmt.Errorf("template %s: title and blurb are required", id)
	}
	return t, nil
}

// CopyTemplate writes the plugin files for id into dst and vendors the TypeScript SDK.
// template.yaml stays in the catalog; it is not part of the plugin.
func CopyTemplate(id, dst string) error {
	if _, err := readTemplate(id); err != nil {
		return err
	}
	abs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if st, err := os.Stat(abs); err == nil {
		entries, rerr := os.ReadDir(abs)
		if rerr != nil {
			return rerr
		}
		if st.IsDir() && len(entries) > 0 {
			return fmt.Errorf("%s already exists and is not empty", abs)
		}
	}
	root := filepath.ToSlash(id)
	err = fs.WalkDir(templates.Files, root, func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if name == root {
			return nil
		}
		clipped := name[len(root)+1:]
		if clipped == "template.yaml" || d.Name() == "template.yaml" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		target := filepath.Join(abs, filepath.FromSlash(clipped))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := templates.Files.ReadFile(name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(abs, "package.json")); err == nil {
		if err := copySDK(filepath.Join(abs, "vendor", "plugin-sdk")); err != nil {
			return err
		}
	}
	dist := filepath.Join(abs, "dist", "index.js")
	if _, err := os.Stat(dist); err == nil {
		// Copying gives src and dist the same timestamp. A newer dist skips npm.
		stamp := time.Now().Add(2 * time.Second)
		if err := os.Chtimes(dist, stamp, stamp); err != nil {
			return err
		}
	}
	return nil
}
