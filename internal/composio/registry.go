package composio

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
)

// Tool is one Composio tool a plugin may call, and the DAP permission it requires.
type Tool struct {
	Slug       string   `json:"slug"`
	Permission string   `json:"permission"`
	Effects    []string `json:"effects,omitempty"`
}

// Toolkit is one connectable service. Builtin rows ship in the binary.
// More rows load from composio/toolkits.json. See docs/plugin-authoring.md.
type Toolkit struct {
	ID       string            `json:"id"`
	Slug     string            `json:"slug"`
	Label    string            `json:"label"`
	Kind     string            `json:"kind,omitempty"`
	Alias    string            `json:"alias,omitempty"`
	Summary  string            `json:"summary,omitempty"`
	Fallback bool              `json:"fallback,omitempty"`
	Actions  map[string]string `json:"actions,omitempty"`
	Tools    []Tool            `json:"tools"`
}

var (
	idRe   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	slugRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	toolRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`)
	permRe = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)+$`)
)

// Builtin is the connectivity DAP ships. Gmail and Outlook are the mail pair.
// Teams is intentionally not here; it is the next toolkit, documented in docs/composio.md.
func Builtin() []Toolkit {
	return []Toolkit{
		{
			ID:      "gmail",
			Slug:    "gmail",
			Label:   "Gmail",
			Kind:    "mail",
			Summary: "Sign in with Google. Composio holds the Gmail token.",
			Actions: map[string]string{
				"threads": "GMAIL_FETCH_EMAILS",
				"draft":   "GMAIL_CREATE_EMAIL_DRAFT",
				"send":    "GMAIL_SEND_EMAIL",
				"reply":   "GMAIL_REPLY_TO_THREAD",
				"profile": "GMAIL_GET_PROFILE",
			},
			Tools: []Tool{
				{Slug: "GMAIL_FETCH_EMAILS", Permission: "mail.read"},
				{Slug: "GMAIL_GET_PROFILE", Permission: "mail.read"},
				{Slug: "GMAIL_CREATE_EMAIL_DRAFT", Permission: "mail.draft"},
				{Slug: "GMAIL_SEND_EMAIL", Permission: "mail.send", Effects: []string{"sends"}},
				{Slug: "GMAIL_REPLY_TO_THREAD", Permission: "mail.send", Effects: []string{"sends"}},
				{Slug: "GMAIL_SEND_DRAFT", Permission: "mail.send", Effects: []string{"sends"}},
			},
		},
		{
			ID:       "outlook",
			Slug:     "outlook",
			Label:    "Outlook",
			Kind:     "mail",
			Alias:    "graph",
			Fallback: true,
			Summary:  "Sign in with Microsoft. Composio holds the Outlook token. Personal and work mailboxes both work.",
			Actions: map[string]string{
				"threads":     "OUTLOOK_LIST_MESSAGES",
				"draft":       "OUTLOOK_CREATE_DRAFT",
				"draft_reply": "OUTLOOK_CREATE_DRAFT_REPLY",
				"send":        "OUTLOOK_SEND_EMAIL",
				"reply":       "OUTLOOK_REPLY_EMAIL",
				"profile":     "OUTLOOK_GET_PROFILE",
			},
			Tools: []Tool{
				{Slug: "OUTLOOK_LIST_MESSAGES", Permission: "mail.read"},
				{Slug: "OUTLOOK_GET_PROFILE", Permission: "mail.read"},
				{Slug: "OUTLOOK_CREATE_DRAFT", Permission: "mail.draft"},
				{Slug: "OUTLOOK_CREATE_DRAFT_REPLY", Permission: "mail.draft"},
				{Slug: "OUTLOOK_SEND_EMAIL", Permission: "mail.send", Effects: []string{"sends"}},
				{Slug: "OUTLOOK_REPLY_EMAIL", Permission: "mail.send", Effects: []string{"sends"}},
				{Slug: "OUTLOOK_SEND_DRAFT", Permission: "mail.send", Effects: []string{"sends"}},
			},
		},
	}
}

// Registry is builtin toolkits plus any file additions.
type Registry struct {
	items []Toolkit
}

// Load reads an optional toolkit file. Builtin ids cannot be replaced.
// The file must not carry secrets. A key or connection id in it is refused.
func Load(path string) (Registry, error) {
	reg := Registry{items: append([]Toolkit(nil), Builtin()...)}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return reg, nil
		}
		return reg, err
	}
	if strings.Contains(string(b), `"api_key"`) || strings.Contains(string(b), `"connection_id"`) {
		return reg, errors.New("composio/toolkits.json must not contain an api key or connection id")
	}
	var file struct {
		Toolkits []Toolkit `json:"toolkits"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		return reg, err
	}
	for _, tk := range file.Toolkits {
		if err := validate(tk); err != nil {
			return reg, err
		}
		if _, ok := reg.lookup(tk.ID); ok {
			return reg, errors.New("toolkit " + tk.ID + " is built in and cannot be redefined")
		}
		reg.items = append(reg.items, tk)
	}
	return reg, nil
}

func validate(tk Toolkit) error {
	if !idRe.MatchString(tk.ID) {
		return errors.New("toolkit id " + tk.ID + " must be a short lowercase name")
	}
	if !slugRe.MatchString(tk.Slug) {
		return errors.New("toolkit slug " + tk.Slug + " is invalid")
	}
	if strings.TrimSpace(tk.Label) == "" {
		return errors.New("toolkit " + tk.ID + " needs a label")
	}
	if tk.Alias != "" && !idRe.MatchString(tk.Alias) {
		return errors.New("toolkit alias is invalid")
	}
	if len(tk.Tools) == 0 && len(tk.Actions) == 0 {
		return errors.New("toolkit " + tk.ID + " needs tools or actions")
	}
	for _, tool := range tk.Tools {
		if !toolRe.MatchString(tool.Slug) {
			return errors.New("tool slug " + tool.Slug + " is invalid")
		}
		if !permRe.MatchString(tool.Permission) {
			return errors.New("permission " + tool.Permission + " must look like mail.read")
		}
		for _, eff := range tool.Effects {
			switch eff {
			case "sends", "spends", "deletes":
			default:
				return errors.New("effect " + eff + " is unknown")
			}
		}
	}
	for name, slug := range tk.Actions {
		switch name {
		case "threads", "draft", "draft_reply", "send", "reply", "profile":
		default:
			return errors.New("action " + name + " is unknown")
		}
		if !toolRe.MatchString(slug) {
			return errors.New("action " + name + " tool slug is invalid")
		}
	}
	return nil
}

// List returns toolkits in display order.
func (r Registry) List() []Toolkit {
	return append([]Toolkit(nil), r.items...)
}

// ByID finds a toolkit by id or alias.
func (r Registry) ByID(id string) (Toolkit, bool) {
	return r.lookup(strings.ToLower(strings.TrimSpace(id)))
}

func (r Registry) lookup(id string) (Toolkit, bool) {
	for _, tk := range r.items {
		if tk.ID == id || tk.Alias == id {
			return tk, true
		}
	}
	return Toolkit{}, false
}

// Tool finds a declared tool. Unknown tools are denied so a plugin cannot
// call a send under a read permission.
func (r Registry) Tool(toolkit, slug string) (Toolkit, Tool, bool) {
	tk, ok := r.ByID(toolkit)
	if !ok {
		return Toolkit{}, Tool{}, false
	}
	for _, tool := range tk.Tools {
		if tool.Slug == slug {
			return tk, tool, true
		}
	}
	return tk, Tool{}, false
}

// Canonical returns the stored id for a builtin alias (outlook, not graph).
func Canonical(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, tk := range Builtin() {
		if tk.ID == id || tk.Alias == id {
			return tk.ID
		}
	}
	return id
}
