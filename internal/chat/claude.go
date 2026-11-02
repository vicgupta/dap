package chat

import (
	"encoding/json"
	"strings"
)

// Claude is the Claude Code adapter.
//
// One turn, verified against the Claude Code CLI reference and the headless
// page (https://code.claude.com/docs/en/cli-reference, https://code.claude.com/docs/en/headless):
//
//	claude -p --output-format stream-json --verbose --include-partial-messages
//	  --permission-mode plan --permission-prompts none
//	  --tools Read,Glob,Grep
//	  --disallowedTools Edit,Write,NotebookEdit,MultiEdit,Bash,WebFetch,WebSearch
//	  --append-system-prompt-file <rules>
//	  --session-id <uuid>            # first turn
//	  --resume <session-id>          # later turns
//	  <prompt>
//
// plan mode reads and runs read-only shell commands but does not edit source.
// --permission-prompts none denies anything that would still ask, so a turn
// cannot block on a permission dialog. --bare is not used: bare mode ignores
// the signed-in Claude account. DAP never passes an API key.
type Claude struct{}

func (Claude) ID() string     { return ProviderClaude }
func (Claude) Label() string  { return "Claude Code" }
func (Claude) Binary() string { return "claude" }

func (Claude) Args(turn Turn) ([]string, error) {
	if strings.TrimSpace(turn.Prompt) == "" {
		return nil, fmtError("empty prompt")
	}
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--permission-mode", "plan",
		"--permission-prompts", "none",
		"--tools", "Read,Glob,Grep",
		"--disallowedTools", "Edit,Write,NotebookEdit,MultiEdit,Bash,WebFetch,WebSearch",
		"--max-turns", "6",
	}
	if turn.RulesFile != "" {
		args = append(args, "--append-system-prompt-file", turn.RulesFile)
	}
	id := strings.TrimSpace(turn.ProviderSessionID)
	if turn.Resume {
		if id == "" {
			return nil, fmtError("resume needs a session id")
		}
		args = append(args, "--resume", id)
	} else {
		if id == "" {
			id = newUUID()
		}
		args = append(args, "--session-id", id)
	}
	args = append(args, turn.Prompt)
	return args, nil
}

func (Claude) ParseLine(line []byte) ([]Event, error) {
	line = bytesTrim(line)
	if len(line) == 0 || line[0] != '{' {
		return nil, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, nil
	}
	typ := jsonString(raw, "type")
	switch typ {
	case "system":
		if jsonString(raw, "subtype") == "init" {
			if id := jsonString(raw, "session_id"); id != "" {
				return []Event{{Kind: EventSession, SessionID: id}}, nil
			}
		}
	case "stream_event":
		if text := claudeDelta(raw["event"]); text != "" {
			return []Event{{Kind: EventText, Text: text, Delta: true}}, nil
		}
	case "assistant":
		return claudeAssistant(raw), nil
	case "result":
		ev := Event{Kind: EventDone, Done: true, SessionID: jsonString(raw, "session_id")}
		if jsonBool(raw, "is_error") || jsonString(raw, "subtype") == "error" {
			msg := jsonString(raw, "result")
			if msg == "" {
				msg = "Claude Code reported an error"
			}
			return []Event{{Kind: EventError, Text: msg}, ev}, nil
		}
		if result := jsonString(raw, "result"); result != "" {
			return []Event{{Kind: EventText, Text: result}, ev}, nil
		}
		return []Event{ev}, nil
	}
	return nil, nil
}

func claudeDelta(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var event map[string]json.RawMessage
	if err := json.Unmarshal(raw, &event); err != nil {
		return ""
	}
	// Headless docs filter on event.delta.type == text_delta. The stream also
	// nests that delta under content_block_delta. Accept both.
	if delta := event["delta"]; len(delta) > 0 {
		var d map[string]json.RawMessage
		if json.Unmarshal(delta, &d) == nil && jsonString(d, "type") == "text_delta" {
			return jsonString(d, "text")
		}
	}
	return ""
}

func claudeAssistant(raw map[string]json.RawMessage) []Event {
	msgRaw := raw["message"]
	if len(msgRaw) == 0 {
		return nil
	}
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return nil
	}
	var events []Event
	if len(msg.Content) > 0 && msg.Content[0] == '"' {
		var text string
		if json.Unmarshal(msg.Content, &text) == nil && text != "" {
			events = append(events, textEvents(text)...)
		}
		return events
	}
	var blocks []struct {
		Type  string         `json:"type"`
		Text  string         `json:"text"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			events = append(events, textEvents(b.Text)...)
		case "tool_use":
			tool, prop := ProposalFromTool(b.Name, b.Input)
			events = append(events, Event{Kind: EventTool, Tool: &tool})
			if prop != nil {
				events = append(events, Event{Kind: EventProposal, Proposal: prop})
			}
		}
	}
	if id := jsonString(raw, "session_id"); id != "" {
		events = append(events, Event{Kind: EventSession, SessionID: id})
	}
	return events
}

func textEvents(text string) []Event {
	clean, props := StripProposals(text)
	var events []Event
	if clean != "" {
		events = append(events, Event{Kind: EventText, Text: clean})
	}
	for i := range props {
		p := props[i]
		events = append(events, Event{Kind: EventProposal, Proposal: &p})
	}
	return events
}
