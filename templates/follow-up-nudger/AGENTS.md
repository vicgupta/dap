# follow-up-nudger

Starting point built on the inbox-followups idea. Customize the voice and the wait rule. Do not send mail.

- Delete the DAP_SCAFFOLD_STARTER line once this starter is no longer the idle copy.
- Update plugin.yaml so the description, permissions, and name match the plugin you were asked to build.
- Stdout is JSON-RPC only. Logs go to stderr.
- `mail.read` and `mail.draft` are enough. Do not add `mail.send`.
- A step that would send must be a card with the `sends` effect so it waits for approval.
