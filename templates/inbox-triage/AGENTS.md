# inbox-triage

This is a DAP watcher. Read `plugin.yaml` before editing.

- Stdout is JSON-RPC only. Logs go to stderr.
- `mail.read` is the only permission. Do not add `mail.send`. Nothing is sent.
- `watch` reads the configured mailbox. `propose` returns one change card that summarizes the newest threads.
- Keep the card kind `change` so it waits in the inbox. Do not add a `sends`, `spends`, or `deletes` effect.
