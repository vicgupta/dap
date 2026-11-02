# DAP

DAP (Dashboard Agent Platform) is a local-first app for running personal agents on your laptop. Your coding agent (Claude Code, Codex, or OpenCode) can do the thinking later. This repository is the core (the interface, the plugin contract, and the safety rails) plus inbox follow-ups and the mail connections those follow-ups use.

Core version: `0.1.0` (`internal/version`). Go `1.22` or newer.

| Read | For |
| --- | --- |
| [docs/architecture.md](docs/architecture.md) | Process model, window, tokens, scoped API |
| [docs/cli.md](docs/cli.md) | Every `da` command, flag, exit code, and example |
| [docs/plugin-authoring.md](docs/plugin-authoring.md) | Write a plugin, or point a coding agent at it |
| [docs/plugins.md](docs/plugins.md) | `plugin.yaml`, the five-step interface, authoring in any language |
| [AGENTS.md](AGENTS.md) | Rules for a coding agent working in this repo |
| [docs/runtime.md](docs/runtime.md) | Scheduler, inbox, guardrails, ledger, secrets, doctor, data directory, limitations |
| [docs/layout.md](docs/layout.md) | Home and inbox card layouts, density, and the plugin contract |
| [docs/theme.md](docs/theme.md) | System, Light, and Dark, and how plugin pages follow them |
| [docs/inbox-followups.md](docs/inbox-followups.md) | Follow-ups watcher, Connections, Gmail and Outlook |
| [docs/composio.md](docs/composio.md) | Composio connectivity layer, setup, privacy, releases |
| [docs/plugin-authoring.md](docs/plugin-authoring.md) | How to add another Composio toolkit |
| [docs/release.md](docs/release.md) | Release install, Gatekeeper, SmartScreen, checksums |

## Install the macOS pre-release

[v0.1.0-alpha.1](https://github.com/vicgupta/dap/releases/tag/v0.1.0-alpha.1) is a private GitHub pre-release: Apple Silicon, Intel, and a universal `DAP.app`, plus `da` CLI tarballs. The repo stays private, so these links open only when you are logged into GitHub.

One-time:

```bash
gh auth login
```

Then:

```bash
gh release download v0.1.0-alpha.1 --repo vicgupta/dap --pattern install.sh && sh install.sh
```

`install.sh` uses that `gh` login. It checks SHA256, installs `DAP.app` and `da`, and clears the quarantine flag. Double-click `DAP.app` to run `da run`. Data stays in `~/.dap` (or `$DAP_HOME`).

Direct downloads:

| File | Use |
| --- | --- |
| [DAP-0.1.0-alpha.1-macos-arm64.zip](https://github.com/vicgupta/dap/releases/download/v0.1.0-alpha.1/DAP-0.1.0-alpha.1-macos-arm64.zip) | Apple Silicon app |
| [DAP-0.1.0-alpha.1-macos-amd64.zip](https://github.com/vicgupta/dap/releases/download/v0.1.0-alpha.1/DAP-0.1.0-alpha.1-macos-amd64.zip) | Intel app |
| [DAP-0.1.0-alpha.1-macos-universal.zip](https://github.com/vicgupta/dap/releases/download/v0.1.0-alpha.1/DAP-0.1.0-alpha.1-macos-universal.zip) | Both CPUs |
| [da-0.1.0-alpha.1-darwin-arm64.tar.gz](https://github.com/vicgupta/dap/releases/download/v0.1.0-alpha.1/da-0.1.0-alpha.1-darwin-arm64.tar.gz) | Apple Silicon CLI |
| [da-0.1.0-alpha.1-darwin-amd64.tar.gz](https://github.com/vicgupta/dap/releases/download/v0.1.0-alpha.1/da-0.1.0-alpha.1-darwin-amd64.tar.gz) | Intel CLI |
| [da-0.1.0-alpha.1-darwin-universal.tar.gz](https://github.com/vicgupta/dap/releases/download/v0.1.0-alpha.1/da-0.1.0-alpha.1-darwin-universal.tar.gz) | Universal CLI |
| [checksums.txt](https://github.com/vicgupta/dap/releases/download/v0.1.0-alpha.1/checksums.txt) | SHA256 |

Gatekeeper, checksum checks, and the example plugins: [docs/release.md](docs/release.md).

The same `gh release download` command works for any single file:

```bash
gh release download v0.1.0-alpha.1 --repo vicgupta/dap --pattern DAP-0.1.0-alpha.1-macos-arm64.zip
```

## Linux and Windows

v0.1.0-alpha.1 does not include Linux or Windows archives. After this packaging change is on `main`, the next `v*` tag builds them. `install.sh` on Linux and `install.ps1` on Windows use `gh release download` and check SHA256. Neither script embeds a token.

```bash
gh release download <tag> --repo vicgupta/dap --pattern install.sh && sh install.sh
```

```powershell
gh release download <tag> --repo vicgupta/dap --pattern install.ps1
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

Those tags publish `da-<version>-linux-amd64.tar.gz`, `da-<version>-linux-arm64.tar.gz`, `dap_<version>_amd64.deb` and the arm64 deb (Debian version uses `~` in place of the pre-release hyphen), and `DAP-<version>-windows-amd64.zip`. Names and SmartScreen steps are in [docs/release.md](docs/release.md).

## Run

Supported operating systems: macOS, Windows, and Linux. The binary is pure Go. TypeScript plugins need Node.js 20 or newer (Bun is optional). Python plugins, including workflow and the mail connectors, need Python 3 (`python3` on macOS and Linux; `python` or `py` on Windows).

```bash
go build -o da ./cmd/da          # Windows: go build -o da.exe ./cmd/da
./da init                        # ~/.dap, or %USERPROFILE%\.dap
./da plugin install ./plugins/hello-watch --approve
./da plugin install ./plugins/workflow --approve
./da doctor
./da run                         # serve the UI and open it in the default browser
./da run --no-browser           # print the URL and do not open a browser
```

`make build`, `make test`, and `make run` do the same build, `go test ./...`, and `./da run`.

`da serve` is the same server without opening a window. Open the `url` it prints. Default address is `127.0.0.1:7391`. The listener is forced onto loopback (`0.0.0.0` and `::` become `127.0.0.1`).

Data directory, first match: `--data <dir>`, `--data=<dir>`, `$DAP_HOME`, then `~/.dap`.

## Architecture

`da` is one Go binary. It embeds the web UI and serves it on `127.0.0.1`. `da run` prints that URL and opens it in the operating system's default browser. Chrome is not required. Edge, Firefox, Brave, and Safari work the same way, including on a machine that has none of the Chromium browsers installed.

| OS | Command |
| --- | --- |
| macOS | `open <url>` |
| Windows | `rundll32 url.dll,FileProtocolHandler <url>` |
| Linux | `xdg-open <url>` |

`--no-browser` skips that launch. The URL is still printed, so it can be pasted into any browser. DAP does not pass `--app`, a private browser profile, or a remote-debugging port, and quit does not close the browser.

Wails and other native webviews need CGO and a platform SDK at compile time (WebKitGTK, WebView2, or WKWebView). `go build` would then fail on a machine without that SDK. The window is a shell. Plugins talk to the local API, so a native webview can be added later without changing them.

```
window or browser tab
        │  http://127.0.0.1   admin token
        ▼
┌─────────────────────────────────────────────┐
│ da                                          │
│ scheduler · inbox · guardrails · ledger     │
│ plugin host · secret store                  │
└───────┬──────────────────────┬──────────────┘
        │ JSON-RPC lines       │ scoped HTTP
        ▼                      ▼
  plugin process         plugin process
```

Several plugins run at once. Each is its own OS process. A card waiting for you does not stop the others.

Plugins speak JSON-RPC 2.0, one JSON object per line, on stdin and stdout. Logs go to stderr. The core is the client and calls `initialize`, `watch`, `propose`, `act`, and `report`. Plugins call back on a loopback HTTP API with a bearer token minted for that plugin. That token cannot call another plugin or the admin API.

A plugin with a `ui` block is served on its own `127.0.0.1` port. The core route `/plugins/<name>/` redirects there. The page gets only that plugin's token (`dap-token`, `dap-api`, `dap-plugin`, `dap-inbox`) plus `dap-theme` and `dap-density`. Its content security policy sets `frame-ancestors 'none'`.

## UI

- **Appearance.** The top bar has System, Light, and Dark. System follows the OS. The choice is in `localStorage` (`dap.theme`) and `config.json` (`theme`). See [docs/theme.md](docs/theme.md).
- **Home.** Every plugin, health, last run, next run, pending count, missed-run line. Grid, list, or board. Tile size is small, medium, or large.
- **Chat.** Claude Code or Codex, read-only, with approval cards for anything that would change data or send. Ctrl+Shift+. opens it (Command+Shift+. on a Mac). See [docs/chat.md](docs/chat.md).
- **Decision inbox.** `j` / `k` or the arrow keys move. `a` approves, `e` edits, `r` rejects, `s` snoozes. Shortcuts are ignored while a field is focused. Snooze offers 1 hour, 4 hours, or tomorrow at 08:00 local. Reject asks for a reason. Edit changes the title, summary, and body. List, grid, board (one column per plugin), or a split list and detail. Approve and Reject stay on the card in every layout.
- **Activity.** The ledger, newest first.
- **Guardrails.** The baked-in rules, each permission's effective mode, and recent checks. A control can tighten a permission. It cannot loosen one.
- **Plugins.** Review permissions in plain language, approve the install, see health, open a plugin UI, or remove a plugin.
- **Connections.** One Composio API key, then Connect Gmail and Connect Outlook. More toolkits show up here when they are added. Setup is in [docs/composio.md](docs/composio.md).
- **Create plugin.** Describe the job in plain words, pick a type and a coding agent that is already on `PATH` (Claude Code, Codex, or OpenCode). DAP scaffolds a draft, runs that agent with your login, validates, and puts a change card in the inbox. The plugin is installed only after you approve the card.
- **Quit.** Bottom of the sidebar. Stops scheduled agents, shuts plugins down, and exits. `da quit` does the same.

## Safety rules

Every check is `allowed`, `ask_first`, or `never`.

- A permission the manifest does not declare is `never`.
- A declared permission is `allowed` or `ask_first`.
- Anything that **sends, spends, or deletes** waits for an approved card, even when the permission mode is `allowed`.
- A plugin changes its own config or rules with a `change` card. The core applies it only after approval, then calls `initialize` again. If that call fails, the previous config is restored.
- You can tighten a rule (`allowed` → `ask_first` → `never`). An override cannot make a rule looser than the manifest.
- During a run, the core may act immediately on a plugin card that is `allowed` and has no send, spend, or delete. Cards submitted over HTTP or `da propose` wait in the inbox. `da propose` creates an external card: approval does not call the plugin's `act`. Finish it with `da outcome`.

There are no operating-system notifications. Gmail and Outlook go through the host, which calls Composio, and only after an approved card when the action sends. Composio holds the provider tokens. The API key and connection ids stay in the secret keeper. The direct Microsoft Graph path, and the legacy direct Gmail path, talk to Microsoft or Google from this laptop instead. Pending cards never block a plugin.

## Scheduler

A schedule is one of `every` (Go duration, at least `5s`), a five-field cron expression, or `at: "HH:MM"` with `daily`, `weekdays`, or `weekly`. The scheduler ticks once a second. A run up to 2 minutes late still counts as on time. Later than that, DAP records a missed run ("DAP was not running or the laptop was asleep"), runs **once**, and schedules the next future slot. It does not replay the skipped slots. An on-time success clears the missed banner. A manual run does not.

## Ledger, secrets, doctor

`ledger.jsonl` is append-only. Rows are JSON lines. There is no update and no delete. Kinds the core writes include `plugin.install`, `plugin.remove`, `plugin.start`, `schedule.run`, `schedule.missed`, `card.proposed`, `card.approved`, `card.edited`, `card.rejected`, `card.snoozed`, `card.acted`, `card.blocked`, `card.failed`, `config.applied`, `guardrail.check`, `permission.denied`, `plugin.log`, `tile.write`, and `outcome`.

Secrets use a `Keeper`. `DAP_SECRETS=file` forces `secrets.json` (mode `0600`). `DAP_SECRETS=os` requires the OS store. The default, `auto`, uses the OS store when a probe succeeds:

| OS | Store | Tool |
| --- | --- | --- |
| macOS | login keychain | `security` (`-w` puts the secret on the process argv for that call) |
| Windows | Credential Manager | `CredWrite` / `CredRead` / `CredDelete` |
| Linux | Secret Service | `secret-tool`, 3 second timeout |

Plugin secrets are stored as service `dap.plugin.<name>` and the HTTP API allows them only when the manifest declares `secrets.read` or `secrets.write`.

`da doctor` checks the data directory, the ledger, the secret backend, dependency order, each plugin's next run, health, and permission approval, and whether `claude`, `codex`, and `opencode` are on `PATH`. Missing agents are informational and do not fail the command. A plugin that is down or waiting on permission approval does. The command prints JSON and exits `1` when `ok` is false. If no server is running, doctor starts plugins for the check and then stops them.

## Plugins in this repo

- `plugins/hello-watch` is a watcher. Every 15 minutes it proposes "Say hello". The permission `demo.greet` is ask-first, and the card does not send. Approving it writes the result to the ledger.
- `plugins/workflow` is a workflow with a step editor at `/plugins/workflow/`. "Pull the digest" is allowed and runs. "Draft the team note" sends, so it waits. "Save the summary" runs after that approval. Saving the step list creates a change card.
- `plugins/mail-fake`, `plugins/mail-gmail`, and `plugins/mail-graph` are mailbox capabilities. `plugins/inbox-followups` proposes drafts for Gmail and Outlook the same way. Approving a card is what sends it. See [docs/composio.md](docs/composio.md) and [docs/inbox-followups.md](docs/inbox-followups.md).

Install does not copy the folder. DAP records the absolute path and runs the package in place. A dependency must already be installed, and its version must satisfy the range. Adding a permission, or loosening a mode, requires `da plugin approve <name>` or the Plugins page before that plugin starts again.

## Test

```bash
go test ./...
```

The tests cover manifest validation, dependency order, permissions, guardrails, the scheduler (including missed-run catch-up), the append-only ledger, the file secret store, JSON-RPC lifecycle, both example plugins running together, approve-and-act, the CLI, Gmail and Graph clients against mocked HTTP, PKCE sign-in with an incremental send scope, and the approval-gated demo send.
