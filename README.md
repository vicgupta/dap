# DAP

DAP (Dashboard Agent Platform) is a local-first desktop app for building, running, approving, and measuring personal agents. Your own coding agent (Claude Code, Codex, or OpenCode) does the thinking. DAP is the interface, the contract, and the safety rails. Everything runs on your laptop.

This repository is the **core and the plugin system**. It does not include the inbox follow-ups plugin or any Gmail or Microsoft Graph integration.

## Architecture

```
browser or app window
        │  http://127.0.0.1  (admin token)
        ▼
┌──────────────────────────────────────────────┐
│  da                                          │
│  scheduler · decision inbox · guardrails     │
│  append-only ledger · plugin host · secrets  │
└───────────┬────────────────────┬─────────────┘
            │ JSON-RPC over stdio │ scoped HTTP + bearer token
            ▼                     ▼
     plugin process          plugin process
     (any language)          (any language)
            │
            ▼
     /plugins/<name>/  → its own loopback origin
```

One plugin or many run at the same time. Each is its own OS process. A plugin that is waiting on you does not block the others.

### Why a single Go binary, not Wails

`da` is one Go binary. It embeds the web UI and serves it on `127.0.0.1`. `da run` opens that UI in a chromeless window (Chrome or Edge `--app`, then the system browser). The same binary is `da serve` if you want a normal browser tab.

Wails (and other native webview bindings) need CGO and a platform webview SDK at compile time: WebKitGTK on Linux, WebView2 on Windows, WKWebView on macOS. That makes `go build` fail on a machine that does not have those SDKs, and it makes cross-compilation painful. The product is the local API, the contract, and the guardrails. The window is a shell. Packaging a native webview later does not require changing plugins.

### Why plugins are processes speaking JSON-RPC

Plugins are separate processes. The core sends one JSON-RPC 2.0 message per line on the plugin's stdin and reads one response per line from stdout. Logs go to stderr.

That isolation is real: a plugin cannot read another plugin's memory, and a crash stops only that plugin. Any language that can read and write a line of JSON can implement the contract. The two examples are Python; a Go or JavaScript plugin is the same protocol.

Callbacks (log, tile, propose, check, outcome, the plugin's own scratch state) go to a scoped HTTP API on `127.0.0.1`, authenticated with a bearer token minted for that plugin. The token cannot call another plugin's API or the admin API. Keeping callbacks off the stdio stream means the protocol stays request/response, which is easy to implement correctly.

In-process Go plugins would lock the language and share the core's address space. Containers are a lot of machinery for a personal agent on a laptop.

### The five-step contract

Interface id: `dap.plugin.v1`. The core tells the plugin which interface it is speaking. A future core keeps accepting `dap.plugin.v1` so today's plugins keep loading.

| Step | Who calls | What it does |
| --- | --- | --- |
| `initialize` | core, at start and after an approved config change | Load config. Return `ready` and a health status. |
| `watch` | core, each run | Return observations since the last run, and the home tile. |
| `propose` | core, after watch | Return decision cards. |
| `act` | core, only after guardrails pass and the card is approved | Do the thing. Return a result and evidence. |
| `report` | core, end of a run | Return metrics. |

`act` is never called for a card that sends, spends, or deletes until you approve it. A change to a plugin's own config or rules is a change card and is applied only after approval. Pending cards do not block the scheduler.

### Guardrails

Every action is `allowed`, `ask first`, or `never`.

- A permission the manifest did not declare is `never`.
- A declared permission is `allowed` or `ask first`, as the manifest says.
- **Sends, spends, and deletes always wait for an approved card**, even when the permission itself is `allowed`.
- You can tighten a permission (down to `never`). You cannot loosen one.
- Change cards always wait, even when their permission is `allowed`.

### Scheduler

A plugin schedule is an interval (`every: 15m`), a five-field cron expression, or a clock time (`at: 08:00` with `daily`, `weekdays`, or `weekly`). If the laptop slept, or DAP was not running, the missed slot is labeled in the ledger and on Home. DAP catches up **once**. It does not replay every skipped interval and pretend they ran on time.

### Ledger and secrets

`ledger.jsonl` in the data directory is append-only. There is no update and no delete. Evidence refs live on the row.

Secrets use a `Keeper` interface. On macOS it uses the login keychain (`security`). On Windows it uses Credential Manager (`CredWrite`). On Linux it uses Secret Service (`secret-tool`) when that answers. Otherwise secrets sit in `secrets.json` with mode `0600`. Set `DAP_SECRETS=file` to force the file, or `DAP_SECRETS=os` to require the OS store. Plugin secrets are namespaced `dap.plugin.<name>`, and the API only allows them when the manifest declares `secrets.read` or `secrets.write`.

### Plugin web UI

A plugin may ship an `ui` block. DAP serves it on its own loopback port and links it as `/plugins/<name>/`. That origin is not the admin origin, so the page cannot read the admin token. Its JavaScript receives only the plugin token, and the only API it can call is that plugin's.

## Run it

Requires Go 1.22+ and, for the example plugins, Python 3.

### Linux, macOS, and Windows

```bash
go build -o da ./cmd/da          # Windows: go build -o da.exe ./cmd/da
./da init                        # creates ~/.dap  (or %USERPROFILE%\.dap)
./da plugin install ./plugins/hello-watch --approve
./da plugin install ./plugins/workflow --approve
./da doctor
./da run                         # serves the UI and opens a window
```

`da serve` is the same server without opening a window. Open the printed URL.

Data directory: `~/.dap`, or `$DAP_HOME`, or `--data <dir>`.

The UI:

- **Home** — every plugin, health, last run, next run, pending count, missed-run banner.
- **Decision inbox** — `j`/`k` move, `a` approve, `e` edit, `r` reject, `s` snooze.
- **Activity** — the ledger.
- **Guardrails** — the baked-in rules, each permission, and recent checks. You can tighten a permission to never.
- **Plugins** — install (permissions are shown in plain language before anything is installed), health, and the plugin's web UI.

`workflow` includes a step editor. Saving it creates a change card. The new steps apply only after you approve that card. A step marked sends, spends, or deletes pauses in the inbox; the steps before it run.

## CLI

Every command prints JSON. Send JSON on stdin where noted.

```bash
da init
da serve --addr 127.0.0.1:7391
da run
da doctor
da plugin install --approve ./plugins/hello-watch
da plugin list
da plugin remove hello-watch
da plugin approve hello-watch          # after a manifest asks for a new permission
da context --plugin workflow
echo '{"plugin":"hello-watch","title":"Note","permission":"demo.greet","kind":"action"}' | da propose
da approved
echo '{"plugin":"hello-watch","permission":"mail.send","effects":["sends"]}' | da check
da log
echo '{"plugin":"hello-watch","message":"noted"}' | da log
echo '{"plugin":"hello-watch","card_id":"c_…","summary":"sent the note"}' | da outcome
echo '{"plugin":"hello-watch","title":"Hello","status":"ok","lines":[{"label":"Ticks","value":"1"}]}' | da tile write
```

`da propose` from the CLI creates an external card. Approving it does not call the plugin's `act`; the caller records the result with `da outcome`. Cards proposed by a plugin are acted on by the core after approval.

`da check` returns `allowed`, `ask_first`, or `never`. A `never` is also an error on the plugin HTTP API (403). `ask_first` means "not yet" — propose a card.

While `da serve` is running, other commands talk to it. Otherwise they open the data directory directly.

## Plugin authoring

A plugin is a folder with a `plugin.yaml` and an entrypoint. Drop it anywhere and install it; DAP does not need to be rebuilt.

### Manifest

```yaml
name: competitor-research          # lowercase, digits, hyphens
version: 0.1.0                     # semver
min_dap_version: 0.1.0             # this core is 0.1.0
type: watcher                      # watcher | capability | workflow
interface: dap.plugin.v1
description: Compare public pages.
entrypoint: plugin.py              # a .py file is run with python3; or an argv list
dependencies:
  - web-watch ^0.3                 # or {name, version}. Loaded first. Ranges: ^ ~ >= exact *
permissions:
  - name: web.read
    mode: allowed                  # allowed | ask_first  ("ask first" is accepted)
    description: Read public pages
  - name: share.team
    mode: ask_first
    description: Share with the team
provides: [alerts, brief]
config:
  - key: every
    type: duration                 # string | number | bool | duration | list
    default: 1h
schedule: every 2h                 # or cron / at. See below.
ui:                                # optional
  entry: ui/index.html
  route: /plugins/competitor-research/
  title: Comparison board
```

A compact permission map is also accepted: `web.read: allowed`, `share.team: ask first`. Any other text is treated as the description and the mode defaults to ask first, which is the safer direction.

Schedule forms:

- `every: 15m` — Go duration, minimum 5s (`30s`, `15m`, `2h`)
- `cron: "0 8 * * 1"` — five fields, minute hour day month weekday
- `at: "08:00"` plus `recurrence: daily | weekdays | weekly` and, for weekly, `day: mon`

On install, DAP validates the manifest, checks dependency versions, shows each permission in plain language, and installs nothing until you approve. A later edit that adds a permission or loosens a mode will not start until you approve it again (`da plugin approve` or the Plugins page).

### Protocol `dap.plugin.v1`

Newline-delimited JSON-RPC 2.0 on stdin/stdout. Unknown methods should return a JSON-RPC error. Do not print anything else on stdout.

`initialize` params:

```json
{
  "interface": "dap.plugin.v1",
  "dap_version": "0.1.0",
  "config": {},
  "plugin_dir": "/path/to/plugin",
  "data_dir": "/path/to/plugin-data",
  "api_base": "http://127.0.0.1:12345/v1/your-plugin",
  "token": "…"
}
```

Result: `{"ready": true, "interface": "dap.plugin.v1", "health": {"status": "ok", "message": "Ready"}}`.

`watch` params: `{"since": "<RFC3339 or null>"}`. Result: `{"observations": [{"id","at","summary"}], "tile": {"title","status","lines":[{"label","value"}]}}`.

`propose` params: `{"observations": [...]}`. Result: `{"cards": [CardDraft]}`. A card draft:

```json
{
  "title": "Draft the team note",
  "summary": "This step sends.",
  "body": "The note.",
  "kind": "action",
  "permission": "share.team",
  "effects": ["sends"],
  "editable": true,
  "payload": {"step_id": "draft"},
  "evidence": [{"label": "Step", "ref": "draft"}]
}
```

`kind` is `action` or `change`. A change card's payload is `{"config": {...}}` and, optionally, `{"schedule": {...}}`. Effects are only `sends`, `spends`, and `deletes`.

`act` params: `{"card": {...}}` including the id, the edited text, and `status: "acting"`. Result: `{"status": "done", "summary": "…", "evidence": [...]}`.

`report` params: `{"period": "week", "from": "…", "to": "…"}`. Result: `{"metrics": [{"key","label","value"}]}`.

### Scoped API

Base URL and token come from `initialize`. Send `Authorization: Bearer <token>`.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/context` | This plugin's config, schedule, scratch state, open cards |
| GET | `/cards` | This plugin's cards |
| GET/PUT | `/state` | Scratch JSON. Not a permission; it is not a side effect |
| POST | `/log` | Append a ledger row |
| POST | `/tile` | Replace the home tile |
| POST | `/propose` | Enqueue a card. Does not act |
| POST | `/check` | `{"permission","effects","card_id"}` → `allowed` / `ask_first` / `never` |
| POST | `/call` | Same as check. 403 unless the decision is `allowed` |
| POST | `/outcome` | Record a result for a card you own |
| POST | `/secret/set` `/secret/get` `/secret/delete` | Only if `secrets.write` / `secrets.read` is declared |

A call the manifest did not declare is denied. Pass `card_id` only for a card of this plugin that is approved (or currently being acted on) and whose effects cover the call.

The optional web UI is a folder of static files. `ui.entry` is the HTML file. DAP injects `<meta name="dap-token">`, `dap-api`, `dap-inbox`, and `dap-plugin`. Talk to `dap-api` (`/api/...`) with the token. See `plugins/workflow/ui` for a working editor.

## Examples

- `plugins/hello-watch` — a watcher. Each run proposes "Say hello". Nothing is sent. Approving writes the result to the ledger.
- `plugins/workflow` — a workflow. "Pull the digest" runs on its own. "Draft the team note" sends, so it waits. "Save the summary" runs after you approve. Both plugins run as separate processes at the same time.

## Inbox follow-ups

`inbox-followups` watches connected mailboxes for threads you owe a reply to and threads that are waiting on someone else. It proposes a draft as a decision card. Approving the card is what sends it. Dismiss, edit, and snooze stay in the plugin's own view and in the core inbox.

```bash
./da plugin install ./plugins/mail-fake --approve
./da plugin install ./plugins/mail-gmail --approve
./da plugin install ./plugins/mail-graph --approve
./da plugin install ./plugins/inbox-followups --approve
./da run
```

Open Connections. **Demo mailbox** connects with one click and needs no account. Gmail and Outlook need a public client id, which you paste there. DAP stores tokens in the OS keychain (or the 0600 file when the keychain is not available). The plugin process and `da` commands never receive the access token or the refresh token. They call `mail.read` and `mail.send` on the host. `mail.send` still requires an approved card, and the send scope is requested from Google or Microsoft only on the first approved send.

`da doctor` reports each installed mailbox: token present or not, whether send scope is granted, whether refresh works, and any throttle window.

### Gmail

1. In Google Cloud Console, create a project and enable the Gmail API.
2. Configure the OAuth consent screen. Scopes you will use: `gmail.readonly`, `gmail.compose`, and later `gmail.send`.
3. Create an OAuth client of type **Desktop app**. Copy the client id. Do not put a client secret in the repo or in `plugin.yaml`. This flow is PKCE and does not use a secret.
4. Desktop clients accept a loopback redirect (`http://127.0.0.1` with an ephemeral port). If the console asks for a redirect URI, add `http://127.0.0.1`.
5. On the Connections page, save the client id and press Connect. Sign in inside Google's window. DAP never sees the password.
6. The first time you approve a follow-up, Google is asked for `gmail.send` as well. Until that consent succeeds, DAP will not call send. Google's `gmail.compose` scope is also allowed to send at the API level; DAP still waits for the incremental `gmail.send` grant.

Google restricted-scope verification is not part of this setup. It is parked.

### Outlook (personal Microsoft accounts)

1. In Microsoft Entra, register an application.
2. Supported account types: **Personal Microsoft accounts only**.
3. Add a **Mobile and desktop applications** platform. Set the redirect URI to `http://localhost`. Turn on **Allow public client flows**.
4. Do not create a client secret. This is a public client with PKCE.
5. API permissions to add over time: `User.Read`, `Mail.ReadWrite`, `offline_access` at sign-in. `Mail.Send` is requested only when the first send is approved. `Calendars.Read` stays off unless you check it on the Connections page.
6. Paste the application (client) id into Connections and press Connect. Sign-in, including MFA, happens in Microsoft's window. DAP does not read Outlook's stored credentials or tokens.
7. On Windows, macOS, and Linux the sign-in uses the system browser and a localhost redirect, which is the MSAL public-client pattern. A native Web Account Manager broker is not linked in this build, so Windows uses that same browser flow.

Polling uses Graph delta queries. Threads are grouped by `conversationId`. A 429 or 503 honors `Retry-After`. A short wait is retried once. A long wait is recorded and shown by doctor instead of blocking the laptop. Disconnect on the Connections page deletes the token from the keychain.

## Development

```bash
go test ./...
go build -o da ./cmd/da
```

Tests cover manifest validation, permission decisions, guardrails, the scheduler, the ledger, plugin lifecycle, Gmail and Graph clients against mocked HTTP, the PKCE sign-in and incremental send scope, and the approval-gated demo send.

## Layout

```
cmd/da                  CLI entry
internal/core           scheduler, inbox, guardrails, host wiring
internal/pluginhost     process isolation and dap.plugin.v1
internal/mail           token custody and Gmail, Graph, and demo clients
internal/manifest       plugin.yaml
internal/api            localhost UI and plugin sandbox
internal/web            embedded UI
plugins/hello-watch
plugins/workflow
plugins/inbox-followups
plugins/mail-gmail
plugins/mail-graph
plugins/mail-fake
```

Inter is bundled under the SIL Open Font License (`internal/web/static/fonts/OFL.txt`).
