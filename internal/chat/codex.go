package chat

import (
	"encoding/json"
	"strings"
)

// Codex is the Codex CLI adapter.
//
// One turn, verified against the non-interactive mode page and the exec
// reference (https://learn.chatgpt.com/docs/non-interactive-mode,
// https://learn.chatgpt.com/docs/developer-commands):
//
//	codex exec --json --sandbox read-only --skip-git-repo-check
//	  --ignore-user-config --ignore-rules -C <workdir> <prompt>
//
//	codex exec --json --sandbox read-only --skip-git-repo-check
//	  --ignore-user-config --ignore-rules -C <workdir> resume <thread_id> <prompt>
//
// exec defaults to a read-only sandbox; the flag makes that explicit so a
// user config cannot widen it. --ignore-user-config skips config.toml and
// still uses CODEX_HOME for the existing login. DAP does not pass an API key.
// --ephemeral is not used, because those sessions cannot be resumed.
type Codex struct{}

func (Codex) ID() string     { return ProviderCodex }
func (Codex) Label() string  { return "Codex" }
func (Codex) Binary() string { return "codex" }

func (Codex) Args(turn Turn) ([]string, error) {
	if strings.TrimSpace(turn.Prompt) == "" {
		return nil, fmtError("empty prompt")
	}
	if strings.TrimSpace(turn.WorkDir) == "" {
		return nil, fmtError("codex needs a working directory")
	}
	args := []string{
		"exec",
		"--json",
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"--ignore-user-config",
		"--ignore-rules",
		"-C", turn.WorkDir,
	}
	if turn.Resume {
		id := strings.TrimSpace(turn.ProviderSessionID)
		if id == "" {
			return nil, fmtError("resume needs a session id")
		}
		args = append(args, "resume", id)
	}
	args = append(args, CodexPrompt(turn.Prompt))
	return args, nil
}

func (Codex) ParseLine(line []byte) ([]Event, error) {
	line = bytesTrim(line)
	if len(line) == 0 || line[0] != '{' {
		return nil, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, nil
	}
	switch jsonString(raw, "type") {
	case "thread.started":
		if id := jsonString(raw, "thread_id"); id != "" {
			return []Event{{Kind: EventSession, SessionID: id}}, nil
		}
	case "turn.completed":
		return []Event{{Kind: EventDone, Done: true}}, nil
	case "turn.failed":
		msg := "Codex turn failed"
		if m := nestedString(raw["error"], "message"); m != "" {
			msg = m
		}
		return []Event{{Kind: EventError, Text: msg}, {Kind: EventDone, Done: true}}, nil
	case "error":
		msg := jsonString(raw, "message")
		if msg == "" {
			msg = "Codex reported an error"
		}
		return []Event{{Kind: EventError, Text: msg}}, nil
	case "item.started", "item.completed":
		return codexItem(jsonString(raw, "type"), raw["item"]), nil
	}
	return nil, nil
}

func codexItem(event string, raw json.RawMessage) []Event {
	if len(raw) == 0 {
		return nil
	}
	var item struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Command string `json:"command"`
		Path    string `json:"path"`
		Changes []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil
	}
	switch item.Type {
	case "agent_message":
		if event != "item.completed" || strings.TrimSpace(item.Text) == "" {
			return nil
		}
		return textEvents(item.Text)
	case "command_execution":
		// item.completed repeats the command. The activity line is the start.
		if event != "item.started" {
			return nil
		}
		tool, prop := ProposalFromTool("command_execution", map[string]any{"command": item.Command})
		ev := []Event{{Kind: EventTool, Tool: &tool}}
		if prop != nil {
			ev = append(ev, Event{Kind: EventProposal, Proposal: prop})
		}
		return ev
	case "file_change":
		if event != "item.completed" {
			return nil
		}
		path := item.Path
		if path == "" && len(item.Changes) > 0 {
			path = item.Changes[0].Path
		}
		tool, prop := ProposalFromTool("file_change", map[string]any{"path": path})
		ev := []Event{{Kind: EventTool, Tool: &tool}}
		if prop != nil {
			ev = append(ev, Event{Kind: EventProposal, Proposal: prop})
		}
		return ev
	case "reasoning":
		return nil
	default:
		if event != "item.completed" && event != "item.started" {
			return nil
		}
		if event == "item.completed" {
			return nil
		}
		tool := ToolUse{Name: displayTool(item.Type), Detail: oneLine(item.Command, 80)}
		return []Event{{Kind: EventTool, Tool: &tool}}
	}
}

func nestedString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	return jsonString(m, key)
}
