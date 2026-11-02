package chat

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const maxProposalBytes = 256 << 10

// StripProposals removes ```dap-proposal fences and returns the cards they describe.
// Malformed fences are left in the text so the person can see them.
func StripProposals(text string) (string, []Proposal) {
	const open = "```dap-proposal"
	var out []Proposal
	var b strings.Builder
	rest := text
	for {
		i := strings.Index(rest, open)
		if i < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:i])
		rest = rest[i+len(open):]
		rest = strings.TrimPrefix(rest, "\r\n")
		rest = strings.TrimPrefix(rest, "\n")
		j := strings.Index(rest, "```")
		if j < 0 {
			b.WriteString(open)
			b.WriteString(rest)
			break
		}
		raw := strings.TrimSpace(rest[:j])
		rest = rest[j+3:]
		if strings.HasPrefix(rest, "\r\n") {
			rest = rest[2:]
		} else if strings.HasPrefix(rest, "\n") {
			rest = rest[1:]
		}
		p, err := parseProposal(raw)
		if err != nil {
			b.WriteString(open)
			b.WriteString("\n")
			b.WriteString(raw)
			b.WriteString("\n```\n")
			continue
		}
		out = append(out, p)
	}
	return strings.TrimSpace(b.String()), out
}

func parseProposal(raw string) (Proposal, error) {
	var p Proposal
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Proposal{}, err
	}
	p = normalizeProposal(p)
	if strings.TrimSpace(p.Title) == "" {
		return Proposal{}, fmtError("proposal needs a title")
	}
	if len(raw) > maxProposalBytes {
		return Proposal{}, fmtError("proposal is too large")
	}
	return p, nil
}

func normalizeProposal(p Proposal) Proposal {
	p.Title = oneLine(p.Title, 140)
	p.Summary = oneLine(p.Summary, 240)
	p.Body = strings.TrimSpace(p.Body)
	p.Op = strings.ToLower(strings.TrimSpace(p.Op))
	p.Kind = strings.ToLower(strings.TrimSpace(p.Kind))
	p.Path = strings.TrimSpace(p.Path)
	p.Target = oneLine(p.Target, 160)
	switch p.Op {
	case "write", "delete", "send", "record":
	default:
		if len(p.Effects) > 0 && containsEffect(p.Effects, "sends") {
			p.Op = "send"
		} else if p.Path != "" && p.Content != "" {
			p.Op = "write"
		} else {
			p.Op = "record"
		}
	}
	switch p.Op {
	case "send":
		p.Kind = "action"
		p.Effects = []string{"sends"}
		p.Perm = PermSend
		if p.Title == "" {
			p.Title = "Send"
		}
	case "delete":
		p.Kind = "change"
		p.Effects = []string{"deletes"}
		p.Perm = PermDelete
		p.Content = ""
		if p.Title == "" {
			p.Title = "Delete a file"
		}
	case "write":
		p.Kind = "change"
		p.Effects = nil
		p.Perm = PermWrite
		if p.Title == "" {
			p.Title = "Write a file"
		}
	default:
		p.Kind = "change"
		p.Effects = nil
		p.Perm = PermWrite
		p.Op = "record"
	}
	if p.Op == "write" && len(p.Content) > maxProposalBytes {
		p.Op = "record"
		p.Kind = "change"
		p.Effects = nil
		p.Perm = PermWrite
		p.Content = ""
		p.Summary = "The proposed file is too large for DAP to write from chat. Approving records the attempt."
	}
	return p
}

func containsEffect(effects []string, want string) bool {
	for _, e := range effects {
		if strings.EqualFold(strings.TrimSpace(e), want) {
			return true
		}
	}
	return false
}

func oneLine(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// ProposalFromTool builds a card from a tool call the CLI must not perform.
// Read tools return a nil proposal.
func ProposalFromTool(name string, input map[string]any) (ToolUse, *Proposal) {
	name = strings.TrimSpace(name)
	detail := toolDetail(input)
	tool := ToolUse{Name: displayTool(name), Detail: oneLine(detail, 80)}
	lower := strings.ToLower(name)
	switch lower {
	case "read", "glob", "grep", "ls":
		return tool, nil
	case "write", "file_change":
		tool.Blocked = true
		path := strField(input, "file_path", "path", "file")
		content := strField(input, "content", "contents", "text")
		p := normalizeProposal(Proposal{
			Title:   "Write " + fallback(path, "a file"),
			Summary: "Save this file in the chat workspace after you approve it.",
			Op:      "write",
			Path:    path,
			Content: content,
		})
		if content == "" {
			p.Op = "record"
			p.Body = "The assistant tried to write " + fallback(path, "a file") + " without the full contents. Approving saves that attempt. It does not apply a partial edit."
			p = normalizeProposal(p)
		}
		return tool, &p
	case "edit", "multiedit", "notebookedit":
		tool.Blocked = true
		path := strField(input, "file_path", "path", "notebook_path")
		p := normalizeProposal(Proposal{
			Title:   "Edit " + fallback(path, "a file"),
			Summary: "The assistant tried to edit a file. Approving records the attempt. DAP does not apply a patch from chat.",
			Body:    prettyInput(input),
			Op:      "record",
		})
		return tool, &p
	case "webfetch", "websearch":
		tool.Blocked = true
		p := normalizeProposal(Proposal{
			Title:   "Network request was not sent",
			Summary: "The assistant asked to reach the network. Approving records it. DAP does not send it.",
			Body:    prettyInput(input),
			Op:      "send",
			Target:  strField(input, "url", "query"),
		})
		return tool, &p
	}
	if lower == "bash" || lower == "command_execution" || strings.Contains(lower, "shell") {
		cmd := strField(input, "command", "cmd")
		if cmd == "" {
			cmd = detail
		}
		if readOnlyCommand(cmd) {
			tool.Detail = oneLine(cmd, 80)
			return tool, nil
		}
		tool.Blocked = true
		tool.Detail = oneLine(cmd, 80)
		p := proposalFromCommand(cmd)
		return tool, &p
	}
	if input == nil {
		return tool, nil
	}
	tool.Blocked = true
	p := normalizeProposal(Proposal{
		Title:   displayTool(name) + " was not run",
		Summary: "The assistant tried to use a tool chat does not run. Approving records it.",
		Body:    prettyInput(input),
		Op:      "record",
	})
	return tool, &p
}

func proposalFromCommand(cmd string) Proposal {
	first := strings.ToLower(strings.Fields(cmd)[0])
	switch first {
	case "rm", "rmdir", "del", "erase":
		path := ""
		fields := strings.Fields(cmd)
		if len(fields) > 1 {
			path = fields[len(fields)-1]
		}
		return normalizeProposal(Proposal{
			Title:   "Delete " + fallback(path, "a file"),
			Summary: "The assistant tried to delete a file. Approving deletes it inside this chat workspace only.",
			Op:      "delete",
			Path:    strings.Trim(path, `"'`),
		})
	case "curl", "wget", "mail", "sendmail", "nc", "ssh", "scp", "sftp":
		return normalizeProposal(Proposal{
			Title:   "Send was blocked",
			Summary: "The assistant tried to send or connect out. Approving records it. DAP does not send it.",
			Body:    cmd,
			Op:      "send",
		})
	default:
		return normalizeProposal(Proposal{
			Title:   "Command was not run",
			Summary: "The assistant tried to run a command. Approving records it in the chat workspace.",
			Body:    cmd,
			Op:      "record",
		})
	}
}

func readOnlyCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return true
	}
	lower := strings.ToLower(cmd)
	if strings.ContainsAny(lower, ">&|") || strings.Contains(lower, ">>") || strings.Contains(lower, "`") || strings.Contains(lower, "$(") {
		return false
	}
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return true
	}
	switch fields[0] {
	case "ls", "cat", "pwd", "head", "tail", "wc", "find", "rg", "grep", "which", "echo", "stat", "du", "diff", "nl", "true", "false", "cd":
		return true
	case "git":
		if len(fields) < 2 {
			return false
		}
		switch fields[1] {
		case "status", "diff", "log", "show", "rev-parse", "ls-files":
			return true
		}
	}
	return false
}

func displayTool(name string) string {
	switch strings.ToLower(name) {
	case "command_execution":
		return "Command"
	case "file_change":
		return "File"
	case "":
		return "Tool"
	default:
		return name
	}
}

func toolDetail(input map[string]any) string {
	if input == nil {
		return ""
	}
	if s := strField(input, "file_path", "path", "file", "pattern", "command", "cmd", "query", "url", "notebook_path"); s != "" {
		return s
	}
	return ""
}

func strField(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func prettyInput(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	b, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return ""
	}
	s := string(b)
	if len(s) > 4000 {
		s = s[:4000] + "…"
	}
	return s
}

func fallback(s, or string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return or
	}
	return s
}

type fmtError string

func (e fmtError) Error() string { return string(e) }
