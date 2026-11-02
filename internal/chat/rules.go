package chat

import "strings"

// Rules is appended to Claude Code and written into each session workspace
// as AGENTS.md so Codex, which has no append-system-prompt flag on exec,
// still sees the same boundary.
const Rules = `You are chatting inside DAP, a local app. This turn is read-only.

- Do not edit files, delete files, send messages, spend money, or run commands that write or transmit anything.
- Read tools may look at this chat workspace only.
- When the person asks for a change or a send, do not attempt it. End your reply with one fenced block exactly like this:

` + "```dap-proposal" + `
{"title":"Short title","summary":"One sentence","body":"What will happen after approval","kind":"change","effects":[],"op":"write","path":"notes.md","content":"file body"}
` + "```" + `

- op is write, delete, or send.
- write and delete use kind "change". path is relative to this workspace, with no .. and no absolute path. write includes the full file content.
- delete sets effects to ["deletes"] and omits content.
- send sets kind to "action", effects to ["sends"], and puts the message in content. Use target for who would receive it. You still do not send it.
- DAP turns that block into an approval card. Nothing changes and nothing is sent until the person approves the card in DAP.
- Do not wrap the block in another fence. One proposal per block. You may include more than one block.
`

// CodexPrompt prefixes the user message. Codex exec takes a single prompt and
// does not have Claude's --append-system-prompt flag. The stored transcript
// keeps the person's text; only the subprocess sees this prefix.
func CodexPrompt(user string) string {
	return strings.TrimSpace(Rules) + "\n\nUser message:\n" + user
}
