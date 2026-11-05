// Package manifest reads and validates plugin.yaml.
package manifest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"

	"github.com/vicgupta/dap/internal/declarative"
	"github.com/vicgupta/dap/internal/layout"
	"github.com/vicgupta/dap/internal/semver"
	"github.com/vicgupta/dap/internal/version"
)

const (
	TypeWatcher    = "watcher"
	TypeCapability = "capability"
	TypeWorkflow   = "workflow"

	ModeAllowed  = "allowed"
	ModeAskFirst = "ask_first"
)

// Manifest is the drop-in description every plugin ships.
type Manifest struct {
	Name          string         `json:"name" yaml:"name"`
	Version       string         `json:"version" yaml:"version"`
	MinDAPVersion string         `json:"min_dap_version" yaml:"min_dap_version"`
	Type          string         `json:"type" yaml:"type"`
	Interface     string         `json:"interface" yaml:"interface"`
	Description   string         `json:"description,omitempty" yaml:"description,omitempty"`
	Entrypoint    Entrypoint     `json:"entrypoint" yaml:"entrypoint"`
	Runtime       string         `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Dependencies  []Dependency   `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Permissions   PermissionList `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Provides      []string       `json:"provides,omitempty" yaml:"provides,omitempty"`
	Config        []ConfigField  `json:"config,omitempty" yaml:"config,omitempty"`
	Schedule      Schedule       `json:"schedule" yaml:"schedule"`
	UI            *UI            `json:"ui,omitempty" yaml:"ui,omitempty"`
	Tile          *TileSpec      `json:"tile,omitempty" yaml:"tile,omitempty"`
	// Declarative is set when kind is declarative. It is not read from YAML
	// and it is not stored in state.json. start reloads plugin.yaml.
	Declarative *declarative.Plugin `json:"-" yaml:"-"`
}

// TileSpec is the dashboard tile's preferred span and the smallest span
// the core should still offer. size and min are small, medium, or large.
type TileSpec struct {
	Size string `json:"size,omitempty" yaml:"size,omitempty"`
	Min  string `json:"min,omitempty" yaml:"min,omitempty"`
}

// Dependency names another plugin and the versions that satisfy it.
type Dependency struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
}

// UnmarshalYAML accepts either "name: web-watch / version: ^0.3" or the short "web-watch ^0.3".
func (d *Dependency) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		parts := strings.Fields(value.Value)
		if len(parts) == 0 || len(parts) > 2 {
			return fmt.Errorf("dependency %q should be 'name' or 'name range'", value.Value)
		}
		d.Name = parts[0]
		d.Version = "*"
		if len(parts) == 2 {
			d.Version = parts[1]
		}
		return nil
	}
	var raw struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	d.Name = raw.Name
	d.Version = raw.Version
	if d.Version == "" {
		d.Version = "*"
	}
	return nil
}

// Permission is one capability the plugin may ask the core to exercise.
type Permission struct {
	Name        string `json:"name" yaml:"name"`
	Mode        string `json:"mode" yaml:"mode"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// PermissionList accepts a list or the compact map from the design sketches.
type PermissionList []Permission

// UnmarshalYAML accepts:
//
//   - name: web.read
//     mode: allowed
//     description: Read public pages
//
// or
//
//	web.read: allowed
//	share.team: ask first
//	web.read: public pages   # description, mode defaults to ask first
func (p *PermissionList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		var list []Permission
		if err := value.Decode(&list); err != nil {
			return err
		}
		for i := range list {
			list[i].Mode = NormalizeMode(list[i].Mode)
		}
		*p = list
		return nil
	}
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("permissions must be a list or a map")
	}
	var out []Permission
	for i := 0; i < len(value.Content); i += 2 {
		key := value.Content[i]
		val := value.Content[i+1]
		perm := Permission{Name: key.Value}
		switch val.Kind {
		case yaml.ScalarNode:
			mode := NormalizeMode(val.Value)
			if mode == ModeAllowed || mode == ModeAskFirst {
				perm.Mode = mode
			} else {
				perm.Mode = ModeAskFirst
				perm.Description = val.Value
			}
		case yaml.MappingNode:
			var raw struct {
				Mode        string `yaml:"mode"`
				Description string `yaml:"description"`
			}
			if err := val.Decode(&raw); err != nil {
				return err
			}
			perm.Mode = NormalizeMode(raw.Mode)
			perm.Description = raw.Description
		default:
			return fmt.Errorf("permission %s has an unsupported value", key.Value)
		}
		out = append(out, perm)
	}
	*p = out
	return nil
}

// NormalizeMode accepts the sketch spelling "ask first".
func NormalizeMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	m = strings.ReplaceAll(m, " ", "_")
	m = strings.ReplaceAll(m, "-", "_")
	return m
}

// ConfigField describes one piece of plugin configuration.
type ConfigField struct {
	Key         string `json:"key" yaml:"key"`
	Type        string `json:"type" yaml:"type"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
}

// Schedule is when the core wakes the plugin. Empty means manual runs only.
type Schedule struct {
	Every      string `json:"every,omitempty" yaml:"every,omitempty"`
	Cron       string `json:"cron,omitempty" yaml:"cron,omitempty"`
	At         string `json:"at,omitempty" yaml:"at,omitempty"`
	Recurrence string `json:"recurrence,omitempty" yaml:"recurrence,omitempty"`
	Day        string `json:"day,omitempty" yaml:"day,omitempty"`
}

// UnmarshalYAML accepts a structured schedule or the shorthand "every 2h".
func (s *Schedule) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		parsed, err := ParseSchedule(value.Value)
		if err != nil {
			return err
		}
		*s = parsed
		return nil
	}
	type plain Schedule
	var raw plain
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*s = Schedule(raw)
	return nil
}

// Empty reports whether the plugin has no schedule.
func (s Schedule) Empty() bool {
	return s.Every == "" && s.Cron == "" && s.At == ""
}

// ParseSchedule reads "every 2h", "cron 0 8 * * 1", "at 08:00", "weekdays at 08:00".
func ParseSchedule(s string) (Schedule, error) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "every "):
		return Schedule{Every: strings.TrimSpace(strings.TrimPrefix(s, "every "))}, nil
	case strings.HasPrefix(s, "cron "):
		return Schedule{Cron: strings.TrimSpace(strings.TrimPrefix(s, "cron "))}, nil
	case strings.HasPrefix(s, "weekdays at "):
		return Schedule{At: strings.TrimSpace(strings.TrimPrefix(s, "weekdays at ")), Recurrence: "weekdays"}, nil
	case strings.HasPrefix(s, "weekly at "):
		return Schedule{At: strings.TrimSpace(strings.TrimPrefix(s, "weekly at ")), Recurrence: "weekly"}, nil
	case strings.HasPrefix(s, "at "):
		return Schedule{At: strings.TrimSpace(strings.TrimPrefix(s, "at ")), Recurrence: "daily"}, nil
	default:
		return Schedule{}, fmt.Errorf("unrecognized schedule %q", s)
	}
}

func normalizeTile(t *TileSpec) error {
	if t.Size == "" {
		t.Size = layout.Medium
	} else {
		size, err := layout.NormalizeSize(t.Size)
		if err != nil {
			return fmt.Errorf("tile.size: %w", err)
		}
		t.Size = size
	}
	if t.Min == "" {
		t.Min = layout.Small
	} else {
		min, err := layout.NormalizeSize(t.Min)
		if err != nil {
			return fmt.Errorf("tile.min: %w", err)
		}
		t.Min = min
	}
	if layout.Rank(t.Size) < layout.Rank(t.Min) {
		return fmt.Errorf("tile.size %s is smaller than tile.min %s", t.Size, t.Min)
	}
	return nil
}

// UI is the optional per-plugin web entry.
type UI struct {
	Entry string `json:"entry" yaml:"entry"`
	Route string `json:"route,omitempty" yaml:"route,omitempty"`
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
}

// Entrypoint is a command. A string is one token; a list is argv.
type Entrypoint []string

// UnmarshalYAML accepts "hello.py" or ["python3", "hello.py"].
func (e *Entrypoint) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		if strings.TrimSpace(value.Value) == "" {
			return fmt.Errorf("entrypoint is empty")
		}
		*e = []string{value.Value}
		return nil
	}
	var list []string
	if err := value.Decode(&list); err != nil {
		return err
	}
	*e = list
	return nil
}

var (
	nameRe       = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	permNameRe   = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)+$`)
	atRe         = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	cronParser   = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	configTypes  = map[string]bool{"string": true, "number": true, "bool": true, "duration": true, "list": true}
	weekdayNames = map[string]time.Weekday{
		"sun": time.Sunday, "sunday": time.Sunday,
		"mon": time.Monday, "monday": time.Monday,
		"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
		"wed": time.Wednesday, "wednesday": time.Wednesday,
		"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
		"fri": time.Friday, "friday": time.Friday,
		"sat": time.Saturday, "saturday": time.Saturday,
	}
)

// Load reads plugin.yaml from a plugin directory.
func Load(dir string) (Manifest, error) {
	path := filepath.Join(dir, "plugin.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	return Parse(b, dir)
}

// Parse decodes YAML and validates it. dir may be empty to skip filesystem checks.
func Parse(b []byte, dir string) (Manifest, error) {
	if declarative.LooksLike(b) {
		p, err := declarative.Compile(b)
		if err != nil {
			return Manifest{}, err
		}
		return projectDeclarative(p, dir)
	}
	var m Manifest
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.Validate(dir); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Validate checks the manifest against the core rules.
// dir is the plugin package directory; pass "" to skip entrypoint and ui file checks.
func (m *Manifest) Validate(dir string) error {
	if !nameRe.MatchString(m.Name) {
		return fmt.Errorf("name must be lowercase letters, numbers, and hyphens")
	}
	if _, err := semver.Parse(m.Version); err != nil {
		return fmt.Errorf("version: %w", err)
	}
	if m.MinDAPVersion == "" {
		return fmt.Errorf("min_dap_version is required")
	}
	min, err := semver.Parse(m.MinDAPVersion)
	if err != nil {
		return fmt.Errorf("min_dap_version: %w", err)
	}
	core, err := semver.Parse(version.Version)
	if err != nil {
		return err
	}
	if min.Compare(core) > 0 {
		return fmt.Errorf("plugin requires DAP %s, this core is %s", min, core)
	}
	switch m.Type {
	case TypeWatcher, TypeCapability, TypeWorkflow:
	default:
		return fmt.Errorf("type must be watcher, capability, or workflow")
	}
	if m.Interface == "" {
		m.Interface = version.InterfaceV1
	}
	if !version.SupportsInterface(m.Interface) {
		return fmt.Errorf("unsupported interface %q", m.Interface)
	}
	if m.Runtime == "declarative" {
		if m.Declarative == nil {
			return fmt.Errorf("declarative plugin spec is missing")
		}
	} else if len(m.Entrypoint) == 0 {
		return fmt.Errorf("entrypoint is required")
	}
	switch m.Runtime {
	case "", "node", "bun", "declarative":
	default:
		return fmt.Errorf("runtime must be node or bun")
	}
	if err := m.Schedule.Validate(); err != nil {
		return err
	}
	seenPerm := map[string]bool{}
	for i := range m.Permissions {
		p := &m.Permissions[i]
		p.Mode = NormalizeMode(p.Mode)
		if !permNameRe.MatchString(p.Name) {
			return fmt.Errorf("permission %q must look like mail.read", p.Name)
		}
		if p.Mode != ModeAllowed && p.Mode != ModeAskFirst {
			return fmt.Errorf("permission %s mode must be allowed or ask_first", p.Name)
		}
		if seenPerm[p.Name] {
			return fmt.Errorf("permission %s is declared twice", p.Name)
		}
		seenPerm[p.Name] = true
	}
	seenDep := map[string]bool{}
	for _, d := range m.Dependencies {
		if !nameRe.MatchString(d.Name) {
			return fmt.Errorf("dependency name %q is invalid", d.Name)
		}
		if d.Name == m.Name {
			return fmt.Errorf("plugin cannot depend on itself")
		}
		if seenDep[d.Name] {
			return fmt.Errorf("dependency %s is listed twice", d.Name)
		}
		seenDep[d.Name] = true
		if err := semver.ValidRange(d.Version); err != nil {
			return fmt.Errorf("dependency %s: %w", d.Name, err)
		}
	}
	seenKey := map[string]bool{}
	for _, f := range m.Config {
		if f.Key == "" || strings.Contains(f.Key, " ") {
			return fmt.Errorf("config key %q is invalid", f.Key)
		}
		if !configTypes[f.Type] {
			return fmt.Errorf("config %s type must be string, number, bool, duration, or list", f.Key)
		}
		if seenKey[f.Key] {
			return fmt.Errorf("config key %s is declared twice", f.Key)
		}
		seenKey[f.Key] = true
		if f.Default != nil {
			if err := checkType(f, f.Default); err != nil {
				return fmt.Errorf("config %s default: %w", f.Key, err)
			}
		}
	}
	for _, p := range m.Provides {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("provides contains an empty value")
		}
	}
	if m.Tile != nil {
		if err := normalizeTile(m.Tile); err != nil {
			return err
		}
	}
	if m.UI != nil {
		if m.UI.Entry == "" {
			return fmt.Errorf("ui.entry is required when ui is set")
		}
		if m.UI.Route == "" {
			m.UI.Route = "/plugins/" + m.Name + "/"
		}
		if !strings.HasPrefix(m.UI.Route, "/plugins/") {
			return fmt.Errorf("ui.route must start with /plugins/")
		}
		if m.UI.Title == "" {
			m.UI.Title = m.Name
		}
	}
	if dir != "" && m.Runtime != "declarative" {
		if _, err := m.Entrypoint.Command(dir); err != nil {
			return err
		}
		if m.UI != nil {
			if _, err := SafePath(dir, m.UI.Entry); err != nil {
				return fmt.Errorf("ui.entry: %w", err)
			}
		}
	}
	return nil
}

// Validate checks a schedule's shape and, for cron, its expression.
func (s *Schedule) Validate() error {
	n := 0
	if s.Every != "" {
		n++
	}
	if s.Cron != "" {
		n++
	}
	if s.At != "" {
		n++
	}
	if n > 1 {
		return fmt.Errorf("schedule must set only one of every, cron, or at")
	}
	if s.Every != "" {
		d, err := time.ParseDuration(s.Every)
		if err != nil {
			return fmt.Errorf("schedule.every: %w", err)
		}
		if d < 5*time.Second {
			return fmt.Errorf("schedule.every must be at least 5s")
		}
	}
	if s.Cron != "" {
		if _, err := cronParser.Parse(s.Cron); err != nil {
			return fmt.Errorf("schedule.cron: %w", err)
		}
	}
	if s.At != "" {
		if !atRe.MatchString(s.At) {
			return fmt.Errorf("schedule.at must be HH:MM")
		}
		if s.Recurrence == "" {
			s.Recurrence = "daily"
		}
		switch s.Recurrence {
		case "daily", "weekdays":
		case "weekly":
			if _, ok := weekdayNames[strings.ToLower(s.Day)]; !ok {
				return fmt.Errorf("schedule.day is required for a weekly recurrence (mon, tue, ...)")
			}
		default:
			return fmt.Errorf("schedule.recurrence must be daily, weekdays, or weekly")
		}
	} else if s.Recurrence != "" || s.Day != "" {
		return fmt.Errorf("schedule.recurrence and day belong with schedule.at")
	}
	return nil
}

// Weekday returns the weekday for a weekly schedule.
func (s Schedule) Weekday() (time.Weekday, bool) {
	d, ok := weekdayNames[strings.ToLower(s.Day)]
	return d, ok
}

// Label is a short human description of the schedule.
func (s Schedule) Label() string {
	switch {
	case s.Every != "":
		return "every " + s.Every
	case s.Cron != "":
		return "cron " + s.Cron
	case s.At != "":
		rec := s.Recurrence
		if rec == "" {
			rec = "daily"
		}
		if rec == "weekly" && s.Day != "" {
			return rec + " " + s.Day + " at " + s.At
		}
		return rec + " at " + s.At
	default:
		return "manual"
	}
}

// Launch resolves the entrypoint and, when runtime is set, the Node or Bun binary.
func (m Manifest) Launch(dir string) ([]string, error) {
	if m.Runtime == "declarative" {
		return nil, fmt.Errorf("declarative plugins run inside DAP and have no entrypoint")
	}
	argv, err := m.Entrypoint.Command(dir)
	if err != nil {
		return nil, err
	}
	switch m.Runtime {
	case "":
		return argv, nil
	case "node":
		return withRuntime(NodeBin(), argv), nil
	case "bun":
		return withRuntime(BunBin(), argv), nil
	default:
		return nil, fmt.Errorf("runtime must be node or bun")
	}
}

func withRuntime(bin string, argv []string) []string {
	if len(argv) == 0 {
		return []string{bin}
	}
	base := filepath.Base(argv[0])
	if base == "node" || base == "bun" || base == "node.exe" || base == "bun.exe" {
		return argv
	}
	return append([]string{bin}, argv...)
}

// NodeBin is the Node.js binary used for runtime: node.
func NodeBin() string {
	if p, err := exec.LookPath("node"); err == nil {
		return p
	}
	return "node"
}

// BunBin is the Bun binary used for runtime: bun.
func BunBin() string {
	if p, err := exec.LookPath("bun"); err == nil {
		return p
	}
	return "bun"
}

// Command resolves the entrypoint to an argv whose paths stay inside dir.
func (e Entrypoint) Command(dir string) ([]string, error) {
	if len(e) == 0 {
		return nil, fmt.Errorf("entrypoint is required")
	}
	argv := append([]string{}, e...)
	if len(argv) == 1 {
		return resolveOne(dir, argv[0])
	}
	bin, err := resolveBin(dir, argv[0])
	if err != nil {
		return nil, err
	}
	argv[0] = bin
	for i := 1; i < len(argv); i++ {
		if looksRelative(argv[i]) {
			abs, err := SafePath(dir, argv[i])
			if err != nil {
				return nil, err
			}
			if st, err := os.Stat(abs); err == nil && !st.IsDir() {
				argv[i] = abs
			}
		}
	}
	return argv, nil
}

func resolveOne(dir, ep string) ([]string, error) {
	abs, err := SafePath(dir, ep)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("entrypoint: %w", err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("entrypoint is a directory")
	}
	if strings.HasSuffix(strings.ToLower(abs), ".py") {
		return []string{PythonBin(), abs}, nil
	}
	return []string{abs}, nil
}

func resolveBin(dir, bin string) (string, error) {
	if looksRelative(bin) || strings.HasSuffix(strings.ToLower(bin), ".py") {
		return SafePath(dir, bin)
	}
	return bin, nil
}

func looksRelative(s string) bool {
	return strings.Contains(s, "/") || strings.Contains(s, `\`) || strings.HasPrefix(s, ".")
}

// SafePath joins rel onto dir and rejects anything that escapes the package.
func SafePath(dir, rel string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("plugin directory is unknown")
	}
	dir = filepath.Clean(dir)
	var full string
	if filepath.IsAbs(rel) {
		full = filepath.Clean(rel)
	} else {
		full = filepath.Clean(filepath.Join(dir, rel))
	}
	relTo, err := filepath.Rel(dir, full)
	if err != nil {
		return "", fmt.Errorf("path escapes the plugin directory")
	}
	if relTo == ".." || strings.HasPrefix(relTo, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes the plugin directory")
	}
	return full, nil
}

// Find returns the declared permission or false.
func (m Manifest) FindPermission(name string) (Permission, bool) {
	for _, p := range m.Permissions {
		if p.Name == name {
			return p, true
		}
	}
	return Permission{}, false
}

// ResolveConfig fills defaults and checks types. Unknown keys are rejected.
func ResolveConfig(m Manifest, given map[string]any) (map[string]any, error) {
	out := map[string]any{}
	known := map[string]ConfigField{}
	for _, f := range m.Config {
		known[f.Key] = f
		if given != nil {
			if v, ok := given[f.Key]; ok {
				if err := checkType(f, v); err != nil {
					return nil, fmt.Errorf("config %s: %w", f.Key, err)
				}
				out[f.Key] = v
				continue
			}
		}
		if f.Required && f.Default == nil {
			return nil, fmt.Errorf("config %s is required", f.Key)
		}
		if f.Default != nil {
			out[f.Key] = f.Default
		}
	}
	for k := range given {
		if _, ok := known[k]; !ok {
			return nil, fmt.Errorf("unknown config key %s", k)
		}
	}
	return out, nil
}

func checkType(f ConfigField, v any) error {
	switch f.Type {
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("expected a string")
		}
	case "duration":
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("expected a duration string")
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		if d < 5*time.Second {
			return fmt.Errorf("must be at least 5s")
		}
	case "bool":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("expected a bool")
		}
	case "number":
		switch v.(type) {
		case int, int64, float64, float32, uint64:
		default:
			return fmt.Errorf("expected a number")
		}
	case "list":
		switch v.(type) {
		case []any, []string, []map[string]any:
		default:
			return fmt.Errorf("expected a list")
		}
	default:
		return fmt.Errorf("unknown type %s", f.Type)
	}
	return nil
}

// SamePermissions reports whether the approved set still covers the manifest.
// A new name or a looser mode requires a fresh approval.
func SamePermissions(approved, current []Permission) bool {
	have := map[string]Permission{}
	for _, p := range approved {
		have[p.Name] = p
	}
	for _, p := range current {
		old, ok := have[p.Name]
		if !ok {
			return false
		}
		if looser(old.Mode, p.Mode) {
			return false
		}
	}
	return true
}

func looser(approved, next string) bool {
	rank := map[string]int{ModeAskFirst: 1, ModeAllowed: 0}
	return rank[next] < rank[approved]
}

// Order resolves plugin start order so dependencies start first.
// Versions that fall outside a dependency range are an error.
func Order(manifests []Manifest) ([]string, error) {
	byName := map[string]Manifest{}
	for _, m := range manifests {
		if _, ok := byName[m.Name]; ok {
			return nil, fmt.Errorf("duplicate plugin %s", m.Name)
		}
		byName[m.Name] = m
	}
	for _, m := range manifests {
		for _, d := range m.Dependencies {
			dep, ok := byName[d.Name]
			if !ok {
				return nil, fmt.Errorf("%s depends on %s, which is not installed", m.Name, d.Name)
			}
			ok, err := semver.Satisfies(dep.Version, d.Version)
			if err != nil {
				return nil, fmt.Errorf("%s depends on %s: %w", m.Name, d.Name, err)
			}
			if !ok {
				return nil, fmt.Errorf("%s needs %s %s, installed version is %s", m.Name, d.Name, d.Version, dep.Version)
			}
		}
	}
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var out []string
	var walk func(string) error
	walk = func(name string) error {
		switch state[name] {
		case done:
			return nil
		case visiting:
			return fmt.Errorf("dependency cycle at %s", name)
		}
		state[name] = visiting
		for _, d := range byName[name].Dependencies {
			if err := walk(d.Name); err != nil {
				return err
			}
		}
		state[name] = done
		out = append(out, name)
		return nil
	}
	// Stable: walk in name order so ties don't flicker.
	names := make([]string, 0, len(manifests))
	for n := range byName {
		names = append(names, n)
	}
	sortStrings(names)
	for _, n := range names {
		if err := walk(n); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j] < s[j-1] {
			s[j], s[j-1] = s[j-1], s[j]
			j--
		}
	}
}
