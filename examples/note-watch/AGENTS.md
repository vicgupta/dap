# Build this DAP plugin

You are editing the plugin in this directory (note-watch, type watcher, language python). DAP does not host a model. You are the user's coding agent. Follow this file. The long form is docs/plugin-authoring.md in the DAP repo, and the machine-readable contract is schemas/plugin.schema.json.

## Rules

- Speak JSON-RPC 2.0, one JSON object per line, on stdout. Logs go to stderr. Never print a prompt, a log line, or a stack trace on stdout.
- Implement initialize, watch, propose, act, and report. Unknown methods return a JSON-RPC error.
- Declare the smallest permission list that does the job. Modes are only allowed or ask_first. An undeclared permission is denied.
- Anything that sends, spends, or deletes must be a decision card with that effect. Do not perform it in watch or propose. act runs only after the person approves.
- A change to this plugin's own config or schedule is a change card (kind change). Do not write it yourself.
- Do not read or write raw OAuth tokens, API keys, or another plugin's secrets.
- Do not add network calls unless the user asked and a permission describes them. This core does not grant a general network permission.
- Do not weaken guardrails. You cannot set a permission to never in the manifest, and you cannot make an ask_first permission allowed in order to skip the inbox.
- Keep entrypoints inside this directory. No path may contain .. that escapes the folder.

## Self-check

From the DAP repo, or with da on PATH:

```
da plugin validate .
```

Fix every check that is not ok. The validator output is the bug list. Do not claim success until it passes.

## Protocol reminder

initialize result: {"ready": true, "interface": "dap.plugin.v1", "health": {"status": "ok", "message": "ready"}}

watch result: {"observations": [], "tile": {"title": "...", "status": "ok", "lines": []}}

propose result: {"cards": [{"title": "...", "summary": "...", "kind": "action", "permission": "...", "effects": []}]}

act result: {"status": "done", "summary": "..."}

report result: {"metrics": [{"key": "runs", "label": "Runs", "value": 1}]}

Effects may only be sends, spends, or deletes. kind is action or change.
