# competitor-research

Watches the public URLs in `urls`. It does not log in and it does not send anything.

- Stdout is JSON-RPC only. Logs go to stderr.
- `web.fetch` is the only permission. Keep it `ask_first`.
- Accept only `http` and `https` URLs. Skip everything else.
- An empty list watches nothing and proposes no cards.
