# Architecture

DAP core `0.1.0` is one Go binary, `da`, built from `cmd/da`. The module requires Go `1.22.0`. It runs on macOS, Windows, and Linux. There is no CGO and no bundled webview.

## Window

The UI is HTML, CSS, and JavaScript embedded with `go:embed` (`internal/web`). `da serve` and `da run` listen on loopback and print a URL. `da run` also opens that URL in the default browser. `--no-browser` on either command skips the launch and still prints the URL.

`internal/desktop` does not search for a particular browser:

- macOS: `open <url>`
- Windows: `rundll32 url.dll,FileProtocolHandler <url>` (no shell, so `&` in the URL is preserved, and no elevation)
- Linux and other Unix: `xdg-open <url>`

The URL is one argument. There is no `--app` flag, no `--user-data-dir`, and no Chrome DevTools port. Chrome, Edge, Firefox, Brave, and Safari all work, including a machine with no Chrome installed. If the opener is missing, `da run` still serves and writes the error, plus the URL, to stderr. The server keeps running until **Quit** in the sidebar, `da quit`, or Ctrl+C (`SIGINT`, and `SIGTERM` on Unix). Those three use the same shutdown: stop the scheduler, wait up to 10 seconds for in-flight `act` calls, call `shutdown` on each plugin, flush the ledger and state, remove `run.json`, release `dap.lock`, stop HTTP, and exit 0.

DAP does not own the browser process, so quit does not close other windows. The page says DAP has quit and asks the tab to close itself. A `browser-profile/` directory left by an older build is unused.

Mail sign-in uses that same opener: Composio’s hosted page, direct Gmail, and direct Microsoft Graph. The loopback callback listens on `127.0.0.1` and `[::1]` for one port. Graph’s redirect host stays `localhost` (the Microsoft public-client form); Gmail uses `127.0.0.1`. The code comes back in the query string. The callback does not set a cookie, so Brave Shields and Safari ITP are not in the path. Firefox HTTPS-Only mode exempts `http://127.0.0.1` and `http://localhost`.

Wails was not used. A Wails (or webview) build needs CGO and a platform SDK: WebKitGTK on Linux, WebView2 on Windows, WKWebView on macOS. `go build` would fail without that SDK, and cross-compilation gets harder. Plugin traffic already goes through HTTP, so a native shell can wrap this binary later without a plugin change.

## Bind address

The default is `127.0.0.1:7391`, stored in `config.json` by `da init`. `--addr` overrides it for one process. `internal/api` rewrites an empty host, `0.0.0.0`, `::`, or `[::]` to `127.0.0.1`. A host that is already loopback is left alone. A malformed `--addr` falls back to `127.0.0.1:7391`.

## Two HTTP origins

**Admin origin.** The page you open with `da run`. Its HTML includes `<meta name="dap-token">`. The browser sends that token as `Authorization: Bearer` to `/api/...`. The same token is written to `run.json` (mode `0600`) so the CLI can call the server. Anyone who can read that file or load the page on the machine can call the admin API. DAP does not add a second login. Binding to loopback is the network boundary.

**Plugin origin.** One extra listener per plugin that declares `ui`, on `127.0.0.1:<ephemeral>`. `GET /plugins/<name>/` on the admin origin redirects there. The plugin page receives:

- `dap-token`, the plugin bearer token
- `dap-api`, `/api`
- `dap-plugin`, the plugin name
- `dap-inbox`, the admin origin's `/#/inbox`
- `dap-theme`, `system`, `light`, or `dark`
- `dap-density`, `comfortable` or `compact` (the Home density)

The redirect to a plugin UI adds `?theme=` and `?density=`. The sandbox sets `<html data-theme>` and `<html data-density>`, plus the matching meta tags, and serves `/theme.css`, `/theme-boot.js`, `/layout.css`, and `/layout-boot.js`. See [theme.md](theme.md) and [layout.md](layout.md).

The sandbox proxies `/api/...` to the core's plugin API at `/v1/<name>/...` and forwards the bearer token. It serves the plugin's UI directory and the shared Inter fonts. Content-Security-Policy includes `frame-ancestors 'none'` and `connect-src 'self'`. The plugin origin cannot read the admin token.

The admin API used by the UI and the CLI, all under the admin token:

| Method | Path |
| --- | --- |
| GET | `/api/home`, `/api/inbox`, `/api/activity`, `/api/guardrails`, `/api/plugins`, `/api/doctor`, `/api/approved`, `/api/connections` |
| GET, PUT | `/api/settings` with `theme` (`system`, `light`, `dark`), `layout` (view, density, order, pinned, collapsed, sizes), and `chat` (`provider`: `claude-code` or `codex`). Any key may be omitted. |
| GET | `/api/context` and `/api/context?plugin=<name>` |
| POST | `/api/inbox/{id}/approve`, `edit`, `reject`, `snooze` |
| POST | `/api/guardrails` with `plugin`, `permission`, `decision` |
| POST | `/api/plugins/install` with `path` and `approve` |
| POST | `/api/plugins/{name}/remove`, `approve`, `run` |
| POST | `/api/propose`, `/api/check`, `/api/log`, `/api/outcome`, `/api/tile` |
| POST | `/api/connections/{provider}/setup`, `connect`, `disconnect`. `provider` is `gmail`, `outlook` (alias `graph`), `fake`, `composio` (API key only), or a toolkit id from `composio/toolkits.json` |
| GET | `/api/templates` |
| POST | `/api/create` with `description`, `agent`, optional `name`, `autoInstall`, `template`, `settings`, and `plugin` (an edit) |
| GET | `/api/create/current`, `/api/create/{id}` — status, step, log tail, card, failed checks, error |
| POST | `/api/create/{id}/retry`, `/api/create/{id}/cancel`, `/api/create/{id}/dismiss` |
| GET | `/api/quit-status` — whether plugins are running and whether an action is in flight |
| POST | `/api/quit` — admin token, loopback peer, `X-DAP-Request: quit`, POST only. Starts the shutdown above |
| GET | `/api/chat/providers`, `/api/chat/sessions`, `/api/chat/sessions/{id}` |
| POST | `/api/chat/sessions`, `/api/chat/sessions/{id}/turn` (newline-delimited JSON), `/api/chat/sessions/{id}/cancel` |
| GET | `/plugins/{name}/` redirects to that plugin's loopback origin |

Plugin API routes (suffixes of `api_base`, which is `http://127.0.0.1:<port>/v1/<plugin>`):

| Method | Path | Behavior |
| --- | --- | --- |
| GET | `/context` | Config, schedule, scratch state, open cards, health |
| GET | `/cards` | Card summaries for this plugin |
| GET | `/state` | Scratch JSON (`null` if unset) |
| PUT | `/state` | Replace scratch JSON |
| POST | `/log` | Append a `plugin.log` row. Body: `message`, optional `evidence` |
| POST | `/tile` | Replace the home tile |
| POST | `/propose` | Enqueue a card. Does not call `act` |
| POST | `/check` | Guardrail decision. HTTP 403 unless the decision is `allowed` |
| POST | `/call` | Same handler as `/check` |
| POST | `/outcome` | Complete a card this plugin owns |
| POST | `/secret/set` | Requires declared `secrets.write` and decision `allowed` |
| POST | `/secret/get` | Requires declared `secrets.read` and decision `allowed` |
| POST | `/secret/delete` | Requires declared `secrets.write` and decision `allowed` |
| GET | `/inbox` | This plugin's cards, including bodies |
| POST | `/inbox/{id}/approve`, `edit`, `reject`, `snooze` | Same guardrails as the core inbox. The plugin token cannot touch another plugin's cards |
| GET | `/mail/status` | Mailbox connection rows. No tokens |
| GET | `/mail/threads` | Requires declared `mail.read`. Gmail and Outlook are interchangeable. |
| POST | `/mail/draft` | Requires declared `mail.draft`. Creates a provider draft and does not send. |
| POST | `/mail/send` | Requires declared `mail.send`, effect `sends`, and a covering approved card |
| POST | `/mail/undo` | Demo mailbox only |
| POST | `/connect/execute` | Runs one tool declared on a toolkit. The plugin must declare that tool's permission. A send, spend, or delete also needs `card_id` of an approved card. Arguments are not written to the ledger. |

Composio connection ids and the API key live in the host, service `dap.composio`. Direct Gmail and Microsoft Graph tokens, when those paths are selected, stay in service `dap.mail`. See [composio.md](composio.md) and [inbox-followups.md](inbox-followups.md).

A missing or wrong bearer token is HTTP 401. Bodies are limited to 1 MiB.

## Plugin processes

A declarative plugin (`kind: declarative`) does not get a process. The core runs it and drops that registration on Remove and on quit. Code plugins still get one process each.

The host starts one process per code plugin. The working directory is the plugin directory. The environment keeps `PATH`, `HOME`, `USER`, `LOGNAME`, `LANG`, `LC_ALL`, `LC_CTYPE`, `TZ`, `SYSTEMROOT`, and `WINDIR` when they are set, plus `PYTHONUNBUFFERED=1`, `PYTHONIOENCODING=utf-8`, `DAP_PLUGIN`, `DAP_API`, and `DAP_TOKEN`. `TMPDIR`, `TEMP`, and `TMP` point at `./.dap/tmp`. `npm_config_cache` and `PIP_CACHE_DIR` point at `./.dap/cache`. Other parent variables are not passed in.

On Unix the process is in its own group so stop can signal the group. The host writes one JSON-RPC request at a time and reads responses by `id`. Non-JSON stdout is kept with stderr. The stderr tail is capped around 8 KiB. `initialize` waits up to 20 seconds. A run's `watch` and `propose` share a 60 second budget. `report` waits 15 seconds. `act` waits 45 seconds. `shutdown` waits 3 seconds and is sent only when that plugin has no call in progress.

`POST /api/quit` is not on the plugin API. A plugin token is rejected with 401. The plugin origin proxies `/api/...` to `/v1/<name>/...`, so a sandbox cannot reach the admin quit route. The handler also requires a loopback peer, `POST`, and the header `X-DAP-Request: quit`. A cross-site `Origin` or `Sec-Fetch-Site: cross-site` is rejected. The admin token is a bearer header, not a cookie.

Dependencies start first (topological order, names sorted for a stable tie). A plugin whose dependency failed to start is not started. A crash or a failed `initialize` marks the plugin unhealthy. The next manual or scheduled run tries to start it again. Cards left in `acting` because the process stopped are returned to `pending` on the next start.

Inter is bundled for the UI under the SIL Open Font License (`internal/web/static/fonts/OFL.txt`).

## What the core does not do

Chat can launch the Claude Code or Codex CLI you already installed. It does not host a model, does not read those CLIs' credentials, and does not run OpenCode. Doctor still only reports whether `claude`, `codex`, and `opencode` are on `PATH`. See [chat.md](chat.md). DAP does not send operating-system notifications. It sends mail only when a plugin calls `/mail/send` and an approved card covers that send. A chat "send" card does not transmit anything. It does not ship a native window toolkit.
