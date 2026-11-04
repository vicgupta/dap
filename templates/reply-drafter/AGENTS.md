# reply-drafter

Drafts replies for flagged threads. This plugin has no `mail.send` permission and must not gain one.

- Stdout is JSON-RPC only. Logs go to stderr.
- A flagged thread is one with `flagged`, `starred`, or a subject that contains "flag".
- Saving a draft uses `mail.draft`. The card kind is `change`, so it waits in the inbox.
- Do not add a `sends` effect.
