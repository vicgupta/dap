# competitor-research

Watches the public URLs in `urls`. It does not log in and it does not send anything.

This stays a code plugin. Declarative plugins have no `web.fetch` service and no page-parsing step. Do not rewrite it as `kind: declarative`.

- Stdout is JSON-RPC only. Logs go to stderr.
- `web.fetch` is the only permission. Keep it `ask_first`.
- Accept only `http` and `https` URLs. Skip everything else.
- An empty list watches nothing and proposes no cards.
