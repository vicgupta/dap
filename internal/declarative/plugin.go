// Package declarative is the no-code plugin language.
// A plugin.yaml with apiVersion dap/v1 and kind declarative is executed by
// the core. It has no subprocess and no user code.
package declarative

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vicgupta/dap/internal/semver"
	"github.com/vicgupta/dap/internal/version"
)

const (
	APIVersion = "dap/v1"
	Kind       = "declarative"

	ServiceEmail = "email"
	EventEmail   = "email.received"

	TypeWatcher    = "watcher"
	TypeCapability = "capability"
	TypeWorkflow   = "workflow"

	ModeAllowed  = "allowed"
	ModeAskFirst = "ask_first"
)

// Plugin is one declarative document.
type Plugin struct {
	APIVersion    string    `json:"apiVersion" yaml:"apiVersion"`
	Kind          string    `json:"kind" yaml:"kind"`
	Name          string    `json:"name" yaml:"name"`
	Version       string    `json:"version" yaml:"version"`
	MinDAPVersion string    `json:"min_dap_version" yaml:"min_dap_version"`
	Type          string    `json:"type,omitempty" yaml:"type,omitempty"`
	Description   string    `json:"description,omitempty" yaml:"description,omitempty"`
	Settings      []Setting `json:"settings,omitempty" yaml:"settings,omitempty"`
	Triggers      []Trigger `json:"triggers" yaml:"triggers"`
	Inputs        []Input   `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Rules         []Rule    `json:"rules,omitempty" yaml:"rules,omitempty"`
	Output        Output    `json:"output" yaml:"output"`
	LLM           *LLM      `json:"llm,omitempty" yaml:"llm,omitempty"`
	Tile          *Tile     `json:"tile,omitempty" yaml:"tile,omitempty"`
}

// Setting is one typed field. The Create page renders it.
type Setting struct {
	Key         string `json:"key" yaml:"key"`
	Type        string `json:"type" yaml:"type"`
	Label       string `json:"label,omitempty" yaml:"label,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
}

// Trigger is one schedule or one event. A trigger object has exactly one.
type Trigger struct {
	Schedule *Schedule `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	Event    string    `json:"event,omitempty" yaml:"event,omitempty"`
}

// Input is an abstract service. email is the only service this core runs.
type Input struct {
	Service       string `json:"service" yaml:"service"`
	Window        string `json:"window,omitempty" yaml:"window,omitempty"`
	WindowSetting string `json:"window_setting,omitempty" yaml:"window_setting,omitempty"`
}

// Rule is one named filter. Clauses are a fixed vocabulary.
type Rule struct {
	ID   string `json:"id" yaml:"id"`
	When When   `json:"when" yaml:"when"`
}

// When is all (every clause) or any (one clause). Not both.
type When struct {
	All []Clause `json:"all,omitempty" yaml:"all,omitempty"`
	Any []Clause `json:"any,omitempty" yaml:"any,omitempty"`
}

// Clause is one comparison.
type Clause struct {
	Key     string
	Strings []string
	Bool    *bool
	Setting string
}

// UnmarshalYAML reads a single-key comparison.
func (c *Clause) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode || len(value.Content) != 2 {
		return fmt.Errorf("a comparison must be one field")
	}
	key := value.Content[0].Value
	if !clauseKeys[key] {
		return fmt.Errorf("unknown comparison %q", key)
	}
	c.Key = key
	return c.readValue(value.Content[1])
}

func (c *Clause) readValue(val *yaml.Node) error {
	if val.Kind == yaml.MappingNode {
		if len(val.Content) != 2 || val.Content[0].Value != "setting" {
			return fmt.Errorf("%s: a setting reference is { setting: key }", c.Key)
		}
		c.Setting = strings.TrimSpace(val.Content[1].Value)
		if c.Setting == "" {
			return fmt.Errorf("%s: setting name is empty", c.Key)
		}
		return nil
	}
	if val.Kind == yaml.ScalarNode {
		switch c.Key {
		case "last_from_me":
			var b bool
			if err := val.Decode(&b); err != nil {
				return fmt.Errorf("last_from_me: expected true or false")
			}
			c.Bool = &b
			return nil
		default:
			if strings.TrimSpace(val.Value) == "" {
				return fmt.Errorf("%s is empty", c.Key)
			}
			c.Strings = []string{val.Value}
			return nil
		}
	}
	if val.Kind == yaml.SequenceNode {
		if c.Key == "last_from_me" || c.Key == "older_than" || c.Key == "newer_than" || c.Key == "no_reply_for" {
			return fmt.Errorf("%s does not take a list", c.Key)
		}
		var list []string
		if err := val.Decode(&list); err != nil {
			return fmt.Errorf("%s: expected a list of strings", c.Key)
		}
		if len(list) == 0 {
			return fmt.Errorf("%s is empty", c.Key)
		}
		c.Strings = list
		return nil
	}
	return fmt.Errorf("%s has an unsupported value", c.Key)
}

// Output is a dashboard report, proposed actions, or both.
type Output struct {
	Limit          int      `json:"limit,omitempty" yaml:"limit,omitempty"`
	LimitSetting   string   `json:"limit_setting,omitempty" yaml:"limit_setting,omitempty"`
	UnmatchedLabel string   `json:"unmatched_label,omitempty" yaml:"unmatched_label,omitempty"`
	Report         *Report  `json:"report,omitempty" yaml:"report,omitempty"`
	Actions        []Action `json:"actions,omitempty" yaml:"actions,omitempty"`
}

// Report is the inbox summary and the home tile title.
type Report struct {
	Title string `json:"title" yaml:"title"`
	Line  string `json:"line,omitempty" yaml:"line,omitempty"`
}

// Action is one proposed card. It never runs until a person approves it.
type Action struct {
	Type             string `json:"type" yaml:"type"`
	Rule             string `json:"rule,omitempty" yaml:"rule,omitempty"`
	Label            string `json:"label,omitempty" yaml:"label,omitempty"`
	Title            string `json:"title,omitempty" yaml:"title,omitempty"`
	Body             string `json:"body,omitempty" yaml:"body,omitempty"`
	Subject          string `json:"subject,omitempty" yaml:"subject,omitempty"`
	SignatureSetting string `json:"signature_setting,omitempty" yaml:"signature_setting,omitempty"`
	StepsSetting     string `json:"steps_setting,omitempty" yaml:"steps_setting,omitempty"`
}

// LLM is reserved. This core does not call a model.
type LLM struct {
	Task  string `json:"task" yaml:"task"`
	Input string `json:"input,omitempty" yaml:"input,omitempty"`
}

// Tile is the home span, same as a code plugin.
type Tile struct {
	Size string `json:"size,omitempty" yaml:"size,omitempty"`
	Min  string `json:"min,omitempty" yaml:"min,omitempty"`
}

// Permission is derived from the services and actions the YAML uses.
type Permission struct {
	Name        string
	Mode        string
	Description string
}

var (
	nameRe     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	settingRe  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	clauseKeys = map[string]bool{
		"sender": true, "sender_domain": true,
		"subject_contains": true, "snippet_contains": true, "label": true,
		"older_than": true, "newer_than": true, "no_reply_for": true,
		"last_from_me": true,
	}
	settingTypes = map[string]bool{
		"string": true, "number": true, "bool": true, "duration": true, "list": true,
	}
	bodyStyles = map[string]bool{"nudge": true, "acknowledge": true, "snippet": true}
	subjects   = map[string]bool{"reply": true, "same": true}
	lineKinds  = map[string]bool{"subject": true, "sender": true, "snippet": true, "label": true}
)

// LooksLike reports whether the YAML should be read as a declarative plugin.
// A code plugin has neither apiVersion nor kind.
func LooksLike(b []byte) bool {
	var head struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(b, &head); err != nil {
		return false
	}
	return head.Kind == Kind || head.APIVersion != ""
}

// Compile checks the schema and the semantic rules.
func Compile(b []byte) (Plugin, error) {
	errs := SchemaErrors(b)
	if len(errs) > 0 {
		return Plugin{}, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	p, err := Decode(b)
	if err != nil {
		return Plugin{}, err
	}
	if err := p.Validate(); err != nil {
		return Plugin{}, err
	}
	return p, nil
}

// Decode parses YAML. Prefer Compile, which also validates.
func Decode(b []byte) (Plugin, error) {
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	var p Plugin
	if err := dec.Decode(&p); err != nil {
		return Plugin{}, fmt.Errorf("parse declarative plugin: %w", err)
	}
	return p, nil
}

// Validate checks references, durations, and the unsupported llm step.
func (p *Plugin) Validate() error {
	if p.APIVersion != APIVersion {
		return fmt.Errorf("apiVersion must be %s", APIVersion)
	}
	if p.Kind != Kind {
		return fmt.Errorf("kind must be declarative")
	}
	if !nameRe.MatchString(p.Name) {
		return fmt.Errorf("name must be lowercase letters, numbers, and hyphens")
	}
	if _, err := semver.Parse(p.Version); err != nil {
		return fmt.Errorf("version: %w", err)
	}
	if p.MinDAPVersion == "" {
		return fmt.Errorf("min_dap_version is required")
	}
	min, err := semver.Parse(p.MinDAPVersion)
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
	if p.Type == "" {
		p.Type = TypeWatcher
	}
	switch p.Type {
	case TypeWatcher, TypeCapability, TypeWorkflow:
	default:
		return fmt.Errorf("type must be watcher, capability, or workflow")
	}
	if p.LLM != nil {
		return fmt.Errorf("llm is not supported yet. This core does not call a model. Remove the llm block")
	}
	settings := map[string]Setting{}
	for _, f := range p.Settings {
		if !settingRe.MatchString(f.Key) {
			return fmt.Errorf("setting %q must be lowercase letters, numbers, and underscores", f.Key)
		}
		if !settingTypes[f.Type] {
			return fmt.Errorf("setting %s type must be string, number, bool, duration, or list", f.Key)
		}
		if _, ok := settings[f.Key]; ok {
			return fmt.Errorf("setting %s is declared twice", f.Key)
		}
		settings[f.Key] = f
	}
	if len(p.Triggers) == 0 {
		return fmt.Errorf("at least one trigger is required")
	}
	schedules := 0
	events := 0
	for i := range p.Triggers {
		t := &p.Triggers[i]
		hasSched := t.Schedule != nil && !t.Schedule.Empty()
		hasEvent := t.Event != ""
		if hasSched == hasEvent {
			return fmt.Errorf("trigger %d must set schedule or event, not both", i+1)
		}
		if hasSched {
			schedules++
			if err := t.Schedule.Validate(); err != nil {
				return err
			}
		}
		if hasEvent {
			events++
			if t.Event != EventEmail {
				return fmt.Errorf("unknown event %q (this core supports %s)", t.Event, EventEmail)
			}
		}
	}
	if schedules > 1 {
		return fmt.Errorf("only one schedule trigger is allowed")
	}
	if schedules == 0 && events == 0 {
		return fmt.Errorf("at least one trigger is required")
	}
	seenSvc := map[string]bool{}
	for i, in := range p.Inputs {
		if in.Service != ServiceEmail {
			return fmt.Errorf("unknown service %q (this core provides %s)", in.Service, ServiceEmail)
		}
		if seenSvc[in.Service] {
			return fmt.Errorf("service %s is listed twice", in.Service)
		}
		seenSvc[in.Service] = true
		if in.Window != "" && in.WindowSetting != "" {
			return fmt.Errorf("input %d sets both window and window_setting", i+1)
		}
		if in.Window != "" {
			if _, err := parseAge(in.Window); err != nil {
				return fmt.Errorf("input window: %w", err)
			}
		}
		if in.WindowSetting != "" {
			f, ok := settings[in.WindowSetting]
			if !ok {
				return fmt.Errorf("window_setting %s is not a setting", in.WindowSetting)
			}
			if f.Type != "string" && f.Type != "duration" {
				return fmt.Errorf("window_setting %s must be a string or duration", in.WindowSetting)
			}
			if s, ok := f.Default.(string); ok && s != "" {
				if _, err := parseAge(s); err != nil {
					return fmt.Errorf("window_setting %s default: %w", in.WindowSetting, err)
				}
			}
		}
	}
	if events > 0 && !seenSvc[ServiceEmail] {
		return fmt.Errorf("event %s needs an email input", EventEmail)
	}
	seenRule := map[string]bool{}
	for _, r := range p.Rules {
		if !nameRe.MatchString(r.ID) {
			return fmt.Errorf("rule id %q must be lowercase letters, numbers, and hyphens", r.ID)
		}
		if seenRule[r.ID] {
			return fmt.Errorf("rule %s is declared twice", r.ID)
		}
		seenRule[r.ID] = true
		if len(r.When.All) > 0 && len(r.When.Any) > 0 {
			return fmt.Errorf("rule %s must set all or any, not both", r.ID)
		}
		clauses := r.When.All
		if len(clauses) == 0 {
			clauses = r.When.Any
		}
		if len(clauses) == 0 {
			return fmt.Errorf("rule %s needs at least one comparison", r.ID)
		}
		for _, c := range clauses {
			if err := c.validate(settings); err != nil {
				return fmt.Errorf("rule %s: %w", r.ID, err)
			}
		}
	}
	if p.Output.Report == nil && len(p.Output.Actions) == 0 {
		return fmt.Errorf("output needs a report or at least one action")
	}
	if p.Output.Limit != 0 && (p.Output.Limit < 1 || p.Output.Limit > 50) {
		return fmt.Errorf("output.limit must be from 1 to 50")
	}
	if p.Output.LimitSetting != "" {
		f, ok := settings[p.Output.LimitSetting]
		if !ok {
			return fmt.Errorf("limit_setting %s is not a setting", p.Output.LimitSetting)
		}
		if f.Type != "number" {
			return fmt.Errorf("limit_setting %s must be a number", p.Output.LimitSetting)
		}
	}
	if p.Output.Report != nil {
		if strings.TrimSpace(p.Output.Report.Title) == "" {
			return fmt.Errorf("output.report.title is required")
		}
		if p.Output.Report.Line == "" {
			p.Output.Report.Line = "subject"
		}
		if !lineKinds[p.Output.Report.Line] {
			return fmt.Errorf("output.report.line must be subject, sender, snippet, or label")
		}
	}
	needsEmail := false
	for i, a := range p.Output.Actions {
		if a.Rule != "" && !seenRule[a.Rule] {
			return fmt.Errorf("action %d refers to unknown rule %s", i+1, a.Rule)
		}
		switch a.Type {
		case "label":
			needsEmail = true
			if strings.TrimSpace(a.Label) == "" {
				return fmt.Errorf("action %d label is empty", i+1)
			}
		case "draft", "send":
			needsEmail = true
			if !bodyStyles[a.Body] {
				return fmt.Errorf("action %d body must be nudge, acknowledge, or snippet", i+1)
			}
			if a.Subject == "" {
				p.Output.Actions[i].Subject = "reply"
			} else if !subjects[a.Subject] {
				return fmt.Errorf("action %d subject must be reply or same", i+1)
			}
			if a.SignatureSetting != "" {
				f, ok := settings[a.SignatureSetting]
				if !ok {
					return fmt.Errorf("signature_setting %s is not a setting", a.SignatureSetting)
				}
				if f.Type != "string" {
					return fmt.Errorf("signature_setting %s must be a string", a.SignatureSetting)
				}
			}
		case "reminder":
			needsEmail = true
		case "checklist":
			if a.StepsSetting == "" {
				return fmt.Errorf("action %d needs steps_setting", i+1)
			}
			f, ok := settings[a.StepsSetting]
			if !ok {
				return fmt.Errorf("steps_setting %s is not a setting", a.StepsSetting)
			}
			if f.Type != "list" {
				return fmt.Errorf("steps_setting %s must be a list", a.StepsSetting)
			}
			if err := checkSteps(f.Default); err != nil {
				return fmt.Errorf("steps_setting %s default: %w", a.StepsSetting, err)
			}
		default:
			return fmt.Errorf("unknown action type %q", a.Type)
		}
	}
	if needsEmail && !seenSvc[ServiceEmail] {
		return fmt.Errorf("label, draft, reminder, and send need an email input")
	}
	return nil
}

func (c Clause) validate(settings map[string]Setting) error {
	if !clauseKeys[c.Key] {
		return fmt.Errorf("unknown comparison %q", c.Key)
	}
	if c.Setting != "" {
		f, ok := settings[c.Setting]
		if !ok {
			return fmt.Errorf("%s refers to unknown setting %s", c.Key, c.Setting)
		}
		switch c.Key {
		case "last_from_me":
			if f.Type != "bool" {
				return fmt.Errorf("setting %s must be a bool", c.Setting)
			}
		case "older_than", "newer_than", "no_reply_for":
			if f.Type != "number" && f.Type != "string" && f.Type != "duration" {
				return fmt.Errorf("setting %s must be a number (hours), string, or duration", c.Setting)
			}
			if f.Type != "number" && f.Default != nil {
				s, ok := f.Default.(string)
				if ok {
					if _, err := parseAge(s); err != nil {
						return fmt.Errorf("setting %s default: %w", c.Setting, err)
					}
				}
			}
		default:
			if f.Type != "string" && f.Type != "list" {
				return fmt.Errorf("setting %s must be a string or list", c.Setting)
			}
		}
		return nil
	}
	switch c.Key {
	case "last_from_me":
		if c.Bool == nil {
			return fmt.Errorf("last_from_me needs true or false")
		}
	case "older_than", "newer_than", "no_reply_for":
		if len(c.Strings) != 1 {
			return fmt.Errorf("%s needs one duration", c.Key)
		}
		if _, err := parseAge(c.Strings[0]); err != nil {
			return fmt.Errorf("%s: %w", c.Key, err)
		}
	default:
		if len(c.Strings) == 0 {
			return fmt.Errorf("%s is empty", c.Key)
		}
	}
	return nil
}

func checkSteps(v any) error {
	if v == nil {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		return fmt.Errorf("expected a list of steps")
	}
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("step %d must be an object with a title", i+1)
		}
		title, _ := m["title"].(string)
		if strings.TrimSpace(title) == "" {
			return fmt.Errorf("step %d needs a title", i+1)
		}
		if err := checkEffects(m["effects"]); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	return nil
}

func checkEffects(v any) error {
	if v == nil {
		return nil
	}
	switch list := v.(type) {
	case []any:
		for _, item := range list {
			s, ok := item.(string)
			if !ok || !effectOK(s) {
				return fmt.Errorf("effects must be sends, spends, or deletes")
			}
		}
	case []string:
		for _, s := range list {
			if !effectOK(s) {
				return fmt.Errorf("effects must be sends, spends, or deletes")
			}
		}
	default:
		return fmt.Errorf("effects must be a list")
	}
	return nil
}

func effectOK(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sends", "spends", "deletes":
		return true
	default:
		return false
	}
}

// Permissions is the install list. The YAML does not declare them.
// mail.read is allowed. Every action waits (ask_first). A report card
// uses report.review so it also waits.
func (p Plugin) Permissions() []Permission {
	var out []Permission
	add := func(name, mode, desc string) {
		for _, have := range out {
			if have.Name == name {
				return
			}
		}
		out = append(out, Permission{Name: name, Mode: mode, Description: desc})
	}
	if p.usesEmail() {
		add("mail.read", ModeAllowed, "Read mail through the connected mailbox")
	}
	if p.Output.Report != nil {
		add("report.review", ModeAskFirst, "Show a report in the inbox. It waits for approval and changes nothing")
	}
	for _, a := range p.Output.Actions {
		switch a.Type {
		case "label":
			add("mail.label", ModeAskFirst, "Record a label on a thread. Waits for approval")
		case "draft":
			add("mail.draft", ModeAskFirst, "Prepare a draft. Nothing is sent until you approve")
		case "send":
			add("mail.send", ModeAskFirst, "Send mail. Always waits for approval")
		case "reminder":
			add("reminder.create", ModeAskFirst, "Record a reminder. Waits for approval")
		case "checklist":
			add("workflow.run", ModeAskFirst, "Run a checklist step. Waits for approval")
		}
	}
	return out
}

func (p Plugin) usesEmail() bool {
	for _, in := range p.Inputs {
		if in.Service == ServiceEmail {
			return true
		}
	}
	return false
}

// HasEvent reports whether the plugin listens for name.
func (p Plugin) HasEvent(name string) bool {
	for _, t := range p.Triggers {
		if t.Event == name {
			return true
		}
	}
	return false
}

// EffectiveSchedule is the schedule trigger, or the core's 15m poll when
// the plugin only has an event.
func (p Plugin) EffectiveSchedule() Schedule {
	for _, t := range p.Triggers {
		if t.Schedule != nil && !t.Schedule.Empty() {
			s := *t.Schedule
			return s
		}
	}
	return Schedule{Every: EventPoll}
}

// PollsEvent is true when the core invented the 15m poll.
func (p Plugin) PollsEvent() bool {
	for _, t := range p.Triggers {
		if t.Schedule != nil && !t.Schedule.Empty() {
			return false
		}
	}
	return p.HasEvent(EventEmail)
}

// Resolve fills setting defaults. Unknown keys are rejected.
func (p Plugin) Resolve(given map[string]any) (map[string]any, error) {
	out := map[string]any{}
	known := map[string]Setting{}
	for _, f := range p.Settings {
		known[f.Key] = f
		if given != nil {
			if v, ok := given[f.Key]; ok {
				if err := checkSetting(f, v); err != nil {
					return nil, fmt.Errorf("setting %s: %w", f.Key, err)
				}
				out[f.Key] = v
				continue
			}
		}
		if f.Required && f.Default == nil {
			return nil, fmt.Errorf("setting %s is required", f.Key)
		}
		if f.Default != nil {
			out[f.Key] = f.Default
		}
	}
	for k := range given {
		if _, ok := known[k]; !ok {
			return nil, fmt.Errorf("unknown setting %s", k)
		}
	}
	return out, nil
}

func checkSetting(f Setting, v any) error {
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
		if err := durationAtLeast5s(s); err != nil {
			return err
		}
	case "bool":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("expected a bool")
		}
	case "number":
		if _, ok := asInt(v); !ok {
			return fmt.Errorf("expected a number")
		}
	case "list":
		switch v.(type) {
		case []any, []string:
		default:
			return fmt.Errorf("expected a list")
		}
	default:
		return fmt.Errorf("unknown type %s", f.Type)
	}
	return nil
}
