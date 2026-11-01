# DAP

This repository is the Dashboard Agent Platform core. DAP does not host a model. When a person creates a plugin, their own coding agent (Claude Code, Codex, or OpenCode) writes it.

## Build a plugin

Read [docs/plugin-authoring.md](docs/plugin-authoring.md) before editing a plugin. The machine-readable contract is [schemas/plugin.schema.json](schemas/plugin.schema.json). `da plugin validate <path>` checks the schema, the manifest rules, and the JSON-RPC lifecycle.

A scaffolded plugin also contains its own `AGENTS.md`. Follow that file when the working directory is the plugin, not this repo.

## Rules

- Never bypass guardrails. Sends, spends, and deletes wait for an approved card. Do not do them in `watch` or `propose`.
- Declare the smallest permission list that does the job. Anything undeclared is denied. Do not mark a sensitive permission `allowed` to skip the inbox.
- Do not handle raw OAuth tokens, API keys, or another plugin's secrets.
- Do not add network calls the user did not ask for. There is no general network permission.
- Plugin stdout is JSON-RPC only. Logs go to stderr.
- Installation is never automatic. A change card is approved by the person.

## Check

```bash
go test ./...
da plugin validate <plugin-directory>
```

`examples/note-watch` is the tutorial plugin. It must pass `da plugin validate`.
