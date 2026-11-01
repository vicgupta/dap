package pluginauthor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Options is a new plugin skeleton.
type Options struct {
	Name        string
	Type        string
	Lang        string
	UI          bool
	Description string
	Dir         string
}

// Scaffold writes a plugin that passes `da plugin validate`.
func Scaffold(opt Options) (string, error) {
	name := strings.TrimSpace(opt.Name)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if !nameOK.MatchString(name) {
		return "", fmt.Errorf("name must be lowercase letters, numbers, and hyphens, and start with a letter")
	}
	kind := opt.Type
	if kind == "" {
		kind = "watcher"
	}
	switch kind {
	case "watcher", "capability", "workflow":
	default:
		return "", fmt.Errorf("type must be watcher, capability, or workflow")
	}
	lang := opt.Lang
	if lang == "" {
		lang = "python"
	}
	switch lang {
	case "python", "go", "node":
	default:
		return "", fmt.Errorf("lang must be python, go, or node")
	}
	dir := opt.Dir
	if dir == "" {
		dir = name
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err == nil {
		entries, rerr := os.ReadDir(abs)
		if rerr != nil {
			return "", rerr
		}
		if st.IsDir() && len(entries) > 0 {
			return "", fmt.Errorf("%s already exists and is not empty", abs)
		}
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	desc := strings.TrimSpace(opt.Description)
	if desc == "" {
		desc = "A " + kind + " plugin named " + name + "."
	}
	perm := permissionName(name)
	files := map[string]string{
		"plugin.yaml": manifestYAML(name, kind, lang, desc, perm, opt.UI),
		"AGENTS.md":   agentsMD(name, kind, lang),
		"CLAUDE.md":   "Follow AGENTS.md in this directory. It is the spec for this DAP plugin.\n",
	}
	switch lang {
	case "python":
		files["plugin.py"] = pythonSrc(name, kind, perm)
		files["test_protocol.py"] = pythonTest()
	case "node":
		files["plugin.js"] = nodeSrc(name, kind, perm)
		files["test_protocol.mjs"] = nodeTest()
	case "go":
		files["go.mod"] = "module " + name + "\n\ngo 1.22\n"
		files["main.go"] = goSrc(name, kind, perm)
		files["main_test.go"] = goTest()
	}
	if opt.UI {
		files["ui/index.html"] = uiHTML(name)
		files["ui/app.js"] = uiJS()
		files["ui/app.css"] = uiCSS()
		files["ui/theme-boot.js"] = themeBoot()
	}
	for rel, body := range files {
		path := filepath.Join(abs, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		mode := os.FileMode(0o644)
		if rel == "plugin.py" || rel == "plugin.js" || rel == "test_protocol.py" {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			return "", err
		}
	}
	return abs, nil
}

// SuggestType picks a starting type from a plain-language description.
func SuggestType(description string) string {
	d := strings.ToLower(description)
	switch {
	case strings.Contains(d, "workflow"), strings.Contains(d, "step by step"), strings.Contains(d, "steps"), strings.Contains(d, "sequence"):
		return "workflow"
	case strings.Contains(d, "capability"), strings.Contains(d, "other plugins"), strings.Contains(d, "for other plugins"):
		return "capability"
	default:
		return "watcher"
	}
}

func permissionName(name string) string {
	parts := strings.Split(name, "-")
	if len(parts) == 1 {
		return "demo." + parts[0]
	}
	return parts[0] + "." + strings.Join(parts[1:], "")
}

func manifestYAML(name, kind, lang, desc, perm string, ui bool) string {
	entry := "plugin.py"
	switch lang {
	case "node":
		entry = "plugin.js"
	case "go":
		entry = "\n  - go\n  - run\n  - ."
	}
	sched := "schedule:\n  every: 1h\n"
	mode := "ask_first"
	if kind == "capability" {
		sched = ""
	}
	uiBlock := ""
	if ui {
		uiBlock = "ui:\n  entry: ui/index.html\n  route: /plugins/" + name + "/\n  title: " + name + "\n"
	}
	return "name: " + name + "\n" +
		"version: 0.1.0\n" +
		"min_dap_version: 0.1.0\n" +
		"type: " + kind + "\n" +
		"interface: dap.plugin.v1\n" +
		"description: " + yamlString(desc) + "\n" +
		"entrypoint: " + entry + "\n" +
		"permissions:\n" +
		"  - name: " + perm + "\n" +
		"    mode: " + mode + "\n" +
		"    description: Record a local result for " + name + "\n" +
		sched + uiBlock
}

func yamlString(s string) string {
	if strings.ContainsAny(s, ":#\n\"'") || strings.TrimSpace(s) != s {
		return strconvQuote(s)
	}
	return s
}

func strconvQuote(s string) string {
	b, _ := jsonMarshalString(s)
	return b
}
