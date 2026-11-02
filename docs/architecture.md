# Architecture

DAP core `0.1.0` is one Go binary, `da`, built from `cmd/da`. The module requires Go `1.22.0`. It runs on macOS, Windows, and Linux. There is no CGO and no bundled webview.

## Window

The UI is HTML, CSS, and JavaScript embedded with `go:embed` (`internal/web`). `da serve` listens on loopback and prints a URL. `da run` does that and opens a window.

`internal/desktop` searches for a browser and starts the first one it finds:

- macOS: `/Applications/Google Chrome.app` with `--app=<url> --new-window`, then Microsoft Edge the same way, then `open <url>`.
- Windows: `msedge --app=<url>`, then `chrome --app=<url>`, then `cmd /c start "" <url>`.
- Linux: `google-chrome`, `google-chrome-stable`, `chromium`, and `chromium-browser`, each with `--app=<url> --new-window`, then `xdg-open`.

`--app` is a chromeless window. The fallback is a normal browser tab. If none of those exist, `da run` still serves and writes an error to stderr. The server keeps running until **Quit** in the sidebar, `da quit`, or Ctrl+C (`SIGINT`, and `SIGTERM` on Unix). Those three use the same shutdown: stop the scheduler, wait up to 10 seconds for in-flight `act` calls, call `shutdown` on each plugin, flush the ledger and state, remove `run.json`, release `dap.lock`, stop HTTP, and exit 0.

`da run` gives Chrome, Edge, or Chromium a private `--user-data-dir` under the data directory (`browser-profile/`). That process is the only browser DAP will stop on quit. `open` and `xdg-open` are normal tabs; the page asks the window to close itself, and those fallbacks are left alone.

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
| GET, PUT | `/api/settings` with `theme` (`system`, `light`, `dark`) and `layout` (view, density, order, pinned, collapsed, sizes). Either key may be omitted. |
| GET | `/api/context` and `/api/context?plugin=<name>` |
| POST | `/api/inbox/{id}/approve`, `edit`, `reject`, `snooze` |
| POST | `/api/guardrails` with `plugin`, `permission`, `decision` |
| POST | `/api/plugins/install` with `path` and `approve` |
| POST | `/api/plugins/{name}/remove`, `approve`, `run` |
| POST | `/api/propose`, `/api/check`, `/api/log`, `/api/outcome`, `/api/tile` |
| POST | `/api/connections/{provider}/setup`, `connect`, `disconnect` (`gmail`, `graph`, or `fake`) |
| POST | `/api/create/scaffold`, `/api/create/run`, `/api/create/validate`, `/api/create/propose` |
| GET | `/api/create/run/{id}` |
| POST | `/api/create/run/{id}/cancel` |
| GET | `/api/quit-status` — whether plugins are running and whether an action is in flight |
| POST | `/api/quit` — admin token, loopback peer, `X-DAP-Request: quit`, POST only. Starts the shutdown above |
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
| GET | `/mail/threads` | Requires declared `mail.read` |
| POST | `/mail/send` | Requires declared `mail.send`, effect `sends`, and a covering approved card |
| POST | `/mail/undo` | Demo mailbox only |

Mail tokens live in the host, service `dap.mail`. See [inbox-followups.md](inbox-followups.md).

A missing or wrong bearer token is HTTP 401. Bodies are limited to 1 MiB.

## Plugin processes

The host starts one process per plugin. The working directory is the plugin directory. The environment keeps `PATH`, `HOME`, `USER`, `LOGNAME`, `LANG`, `LC_ALL`, `LC_CTYPE`, `TZ`, `SYSTEMROOT`, `WINDIR`, `TEMP`, `TMP`, `LOCALAPPDATA`, and `USERPROFILE` when they are set, plus `PYTHONUNBUFFERED=1`, `PYTHONIOENCODING=utf-8`, `DAP_PLUGIN`, `DAP_API`, and `DAP_TOKEN`. Other parent variables are not passed in.

On Unix the process is in its own group so stop can signal the group. The host writes one JSON-RPC request at a time and reads responses by `id`. Non-JSON stdout is kept with stderr. The stderr tail is capped around 8 KiB. `initialize` waits up to 20 seconds. A run's `watch` and `propose` share a 60 second budget. `report` waits 15 seconds. `act` waits 45 seconds. `shutdown` waits 3 seconds and is sent only when that plugin has no call in progress.

`POST /api/quit` is not on the plugin API. A plugin token is rejected with 401. The plugin origin proxies `/api/...` to `/v1/<name>/...`, so a sandbox cannot reach the admin quit route. The handler also requires a loopback peer, `POST`, and the header `X-DAP-Request: quit`. A cross-site `Origin` or `Sec-Fetch-Site: cross-site` is rejected. The admin token is a bearer header, not a cookie.

Dependencies start first (topological order, names sorted for a stable tie). A plugin whose dependency failed to start is not started. A crash or a failed `initialize` marks the plugin unhealthy. The next manual or scheduled run tries to start it again. Cards left in `acting` because the process stopped are returned to `pending` on the next start.

Inter is bundled for the UI under the SIL Open Font License (`internal/web/static/fonts/OFL.txt`).

## What the core does not do

DAP does not call Claude, Codex, or OpenCode. Doctor only reports whether those names are on `PATH`. DAP does not send operating-system notifications. It sends mail only when a plugin calls `/mail/send` and an approved card covers that send. It does not ship a native window toolkit.
