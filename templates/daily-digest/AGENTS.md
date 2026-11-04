# daily-digest

This is a DAP watcher. One card each morning covers yesterday's mail.

- Stdout is JSON-RPC only. Logs go to stderr.
- `mail.read` is the only permission. Do not send mail.
- The schedule is `0 8 * * *` in the machine's local zone.
- The card kind stays `change` so it waits in the inbox.
