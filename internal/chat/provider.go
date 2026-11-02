// Package chat runs the user's installed coding CLIs as managed subprocesses.
//
// DAP does not host a model and does not read CLI credentials. Claude Code and
// Codex each implement Provider. A later OpenCode adapter implements the same
// interface and is registered from Providers; the picker stays closed until then.
package chat

import "time"

const (
	// ProviderClaude is the Claude Code CLI (binary name "claude").
	ProviderClaude = "claude-code"
	// ProviderCodex is the Codex CLI (binary name "codex").
	ProviderCodex = "codex"

	// PluginName is the ledger and inbox owner for cards proposed from chat.
	// It is not an installed plugin.
	PluginName = "chat"

	// PermWrite, PermDelete, and PermSend are the ask-first permissions
	// chat cards declare. The CLI never holds these; approval applies them.
	PermWrite  = "chat.write"
	PermDelete = "chat.delete"
	PermSend   = "chat.send"
)

// Provider is one coding CLI. OpenCode can be added by implementing this
// and listing it from Providers. The manager does not import the CLI's code.
type Provider interface {
	// ID is the stable settings value: claude-code or codex.
	ID() string
	// Label is the name shown in the picker.
	Label() string
	// Binary is the executable LookPath resolves: claude or codex.
	Binary() string
	// Args is the argument vector after the binary for one read-only turn.
	Args(turn Turn) ([]string, error)
	// ParseLine turns one stdout JSON line into zero or more events.
	// A non-JSON line returns no events and a nil error.
	ParseLine(line []byte) ([]Event, error)
}

// Turn is one user message against a saved provider session.
// Resume is true when ProviderSessionID already exists at the CLI.
// A Claude session id minted for the first turn uses --session-id, not --resume.
type Turn struct {
	Prompt            string
	ProviderSessionID string
	Resume            bool
	WorkDir           string
	RulesFile         string
}

// Event is a normalized stream item. Providers with different JSON shapes
// all land here so the UI and the card path stay provider-agnostic.
type Event struct {
	Kind      string
	Text      string
	Delta     bool
	Tool      *ToolUse
	SessionID string
	Proposal  *Proposal
	Done      bool
}

const (
	EventText     = "text"
	EventTool     = "tool"
	EventSession  = "session"
	EventProposal = "proposal"
	EventDone     = "done"
	EventError    = "error"
)

// ToolUse is one compact activity line. Blocked means the CLI was not
// allowed to perform it; DAP may still propose a card.
type ToolUse struct {
	Name    string `json:"name"`
	Detail  string `json:"detail,omitempty"`
	Blocked bool   `json:"blocked,omitempty"`
}

// Proposal is a data change or a send the person must approve.
// Op is write, delete, send, or record. record saves the blocked action
// as a file after approval and does not run it.
type Proposal struct {
	Title   string   `json:"title"`
	Summary string   `json:"summary,omitempty"`
	Body    string   `json:"body,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Effects []string `json:"effects,omitempty"`
	Op      string   `json:"op,omitempty"`
	Path    string   `json:"path,omitempty"`
	Content string   `json:"content,omitempty"`
	Target  string   `json:"target,omitempty"`
	Perm    string   `json:"-"`
}

// Providers returns the CLIs the picker offers, in display order.
// OpenCode is intentionally absent until an adapter exists.
func Providers() []Provider {
	return []Provider{Claude{}, Codex{}}
}

// ByID returns a provider. Unknown ids, including opencode, are refused.
func ByID(id string) (Provider, bool) {
	norm, err := NormalizeProvider(id)
	if err != nil {
		return nil, false
	}
	switch norm {
	case ProviderClaude:
		return Claude{}, true
	case ProviderCodex:
		return Codex{}, true
	default:
		return nil, false
	}
}

// Probe is what the picker shows for one CLI. It never includes a credential.
type Probe struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Installed   bool   `json:"installed"`
	Path        string `json:"path,omitempty"`
	Version     string `json:"version,omitempty"`
	SignedIn    bool   `json:"signed_in"`
	AuthUnknown bool   `json:"auth_unknown,omitempty"`
	Hint        string `json:"hint,omitempty"`
	Login       string `json:"login,omitempty"`
}

// Session is one saved chat. ProviderSessionID is the CLI's own resume id.
type Session struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Provider          string `json:"provider"`
	ProviderSessionID string `json:"provider_session_id,omitempty"`
	// Bound is true after a turn has been handed to the CLI, so the next
	// turn uses --resume / codex exec resume instead of starting over.
	Bound     bool      `json:"bound,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages,omitempty"`
}

// Message is one bubble. Cards are inbox ids proposed from this message.
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Text      string    `json:"text"`
	At        time.Time `json:"at"`
	Tools     []ToolUse `json:"tools,omitempty"`
	Cards     []string  `json:"cards,omitempty"`
	Cancelled bool      `json:"cancelled,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// Info is a history row. The list endpoint omits message bodies.
type Info struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Provider  string    `json:"provider"`
	UpdatedAt time.Time `json:"updated_at"`
}
