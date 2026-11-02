package pluginauthor

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/vicgupta/dap/sdk/typescript"
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
		lang = "ts"
	}
	switch lang {
	case "ts", "typescript":
		lang = "ts"
	case "python":
	default:
		return "", fmt.Errorf("lang must be ts or python")
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
		files["dap_plugin.py"] = pythonHelper()
		files["test_protocol.py"] = pythonTest()
	default:
		files["package.json"] = tsPackage(name)
		files["tsconfig.json"] = tsConfig()
		files["src/index.ts"] = tsSrc(name, kind, perm)
		files["test/protocol.test.mjs"] = tsTest()
	}
	if opt.UI {
		files["ui/index.html"] = uiHTML(name)
		files["ui/app.js"] = uiJS()
		files["ui/app.css"] = uiCSS()
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
	if lang == "ts" {
		if err := copySDK(filepath.Join(abs, "vendor", "plugin-sdk")); err != nil {
			return "", err
		}
	}
	return abs, nil
}

func copySDK(dst string) error {
	return fs.WalkDir(typescript.Files, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := typescript.Files.ReadFile(rel)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
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
	entry := "dist/index.js"
	runtime := "runtime: node\n"
	if lang == "python" {
		entry = "plugin.py"
		runtime = ""
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
		runtime +
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
