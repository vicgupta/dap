# `da` CLI

`da` is the only binary. Every successful command writes indented JSON to stdout. Failures write `{"ok":false,"error":"..."}` to stderr and exit `1`, except an unknown command, which exits `2`. `da`, `da help`, `da --help`, and `da -h` print usage and exit `0`.

Global flag, anywhere in the arguments:

```text
--data <dir>
--data=<dir>
```

If both are absent, the directory is `$DAP_HOME`, then the home directory plus `.dap` (`%USERPROFILE%\.dap` on Windows).

While `da serve` or `da run` is up, it writes `run.json` and holds `dap.lock`. Other commands call that process over HTTP. If `run.json` names a dead PID, the file is removed and the command opens the directory itself. Two processes cannot open the same directory: the second fails with "data directory is in use".

`da doctor` and `da plugin approve` start plugins when they open the directory themselves, then stop them on exit. The other commands do not start plugin processes unless a server is already running.

## `da version`

`da version`, `da --version`, and `da -V` print JSON and exit `0`.

```json
{ "ok": true, "version": "0.1.0" }
```

A release build also includes `release` (the tag without a leading `v`), `commit`, and `date`. A local `go build` omits those. Plugin compatibility uses `version`. A pre-release tag is not written into that field, because plugin semver rejects it.

## `da init`

Creates the data directory (mode `0700`) and `config.json` if it is missing.

```bash
da init
da --data /tmp/dap-demo init
```

```json
{ "ok": true, "data_dir": "/home/you/.dap", "addr": "127.0.0.1:7391" }
```

`init` does not install plugins and does not start a server.

## `da serve`

Starts plugins, the scheduler, and the admin HTTP server. Blocks until Ctrl+C.

```text
da serve [--addr host:port] [--open]
```

`--addr` overrides `config.json` for this process. `--open` also opens a window (same search as `da run`). Unknown extra arguments are ignored.

Stdout, once listening:

```json
{
  "ok": true,
  "addr": "127.0.0.1:7391",
  "url": "http://127.0.0.1:7391/",
  "data_dir": "/home/you/.dap"
}
```

If the window cannot be opened, that error is a second JSON object on stderr. The server still runs.

## `da run`

`da serve` with the window opened. Same flags: `--addr`, and `--open` (already implied).

```bash
da run
da run --addr 127.0.0.1:8787
```

## `da quit`

Stops a DAP that is already running. `da stop` is the same command. It reads `run.json` and `POST`s `/api/quit` with the admin token and `X-DAP-Request: quit`. That is the same shutdown as the Quit button and as Ctrl+C: the scheduler stops, in-flight `act` calls get up to 10 seconds, plugins receive `shutdown`, the ledger and state are flushed, `run.json` is removed, `dap.lock` is released, and the process exits 0.

```bash
da quit
da stop
```

```json
{ "ok": true }
```

If nothing is listening, the command exits `1` with `DAP is not running`. It does not start a server.

## `da doctor`

Checks the data directory, ledger, secret backend, dependency order, each plugin's schedule and health, permission approval, and whether `claude`, `codex`, and `opencode` are on `PATH`.

```bash
da doctor
```

The body is `version`, `ok`, and `checks`. Each check has `name`, `ok`, `detail`, and `level` (`info` or `error`).

| Check name | Fails the command when |
| --- | --- |
| `data_dir` | never (informational) |
| `ledger` | the JSONL file cannot be read |
| `secrets` | never (reports `file`, `keychain`, `credential-manager`, or `secret-service`) |
| `dependencies` | install order could not be resolved |
| `scheduler:<plugin>` | a scheduled plugin has no next run |
| `plugin:<plugin>` | not running, health is not `ok`, permissions need approval, or start failed |
| `permissions:<plugin>` | the manifest asked for a permission that is not approved |
| `agents` | never. Missing agents are listed and `ok` stays true |
| `mail:<provider>` | an installed mailbox is connected and its connection check failed. `provider` is `gmail`, `outlook`, `fake`, or a toolkit added in `composio/toolkits.json`. Composio rows fail when the connection is no longer active. Direct Gmail and Microsoft Graph fail when token refresh fails. Detail says that sends wait for an approved card, or that a direct send scope is missing. The demo mailbox does not fail doctor |

Exit code `1` when the top-level `ok` is false. The report is still on stdout.

With no server running, doctor launches the plugins, reports, and shuts them down.

## `da plugin`

```text
da plugin install [--approve] <path>
da plugin list
da plugin remove <name>
da plugin approve <name>
```

`install` reads `<path>/plugin.yaml`. Without `--approve` it validates and returns the permissions. Nothing is written.

```json
{
  "ok": true,
  "status": "approval_required",
  "name": "hello-watch",
  "permissions": [
    { "name": "demo.greet", "mode": "ask_first", "plain": "Record a hello in the decision inbox — ask first" }
  ]
}
```

`--approve` registers the absolute path (the tree is not copied), fills config defaults, and computes the next run. If a server is already running, it also starts the process. A start failure still leaves the plugin installed; `message` says so and the command exits `0`.

`list` is `{"ok":true,"plugins":[...]}` with the same summaries as Home.

`remove` stops the process, drops it from the registry, and leaves the ledger and old cards in place.

`approve` accepts the current manifest's permission list and starts the plugin. Use it after you edit `plugin.yaml` to add a permission or change a mode from `ask_first` to `allowed`. A tighter mode does not need a new approval.

## `da context`

```text
da context [--plugin <name>]
```

No `--plugin`: the Home payload (version, pending count, plugins, secret backend, detected agents).

`--plugin`: that plugin's config, schedule, scratch state, open cards, and health.

```bash
da context --plugin workflow
```

## `da propose`

Reads one JSON object from stdin. A terminal with no pipe is an error.

```bash
echo '{"plugin":"hello-watch","title":"Note","summary":"From the CLI","permission":"demo.greet","kind":"action"}' | da propose
```

Fields are the card draft (`title`, `summary`, `body`, `kind`, `permission`, `effects`, `evidence`, `payload`, `editable`) plus `plugin`. `actor` defaults to `external`. An external card stays `pending` until a person approves it. Approval sets `approved` and does **not** call the plugin. Post the result with `da outcome`.

`kind` is `action` (default) or `change`. Effects may only be `sends`, `spends`, or `deletes`. A `change` card's payload is `{"config":{...}}` and optionally `{"schedule":{...}}`.

Exit `1` if the plugin is missing, the permission is `never`, or the JSON is invalid. The card is not stored when the decision is `never`.

## `da approved`

Lists up to 50 cards whose status is `approved`, newest first.

```bash
da approved
```

Those are external cards a person has approved and that still need `da outcome`. A plugin card that the core has already acted on is `done` and is not in this list.

## `da check`

Reads one JSON object from stdin.

```bash
echo '{"plugin":"hello-watch","permission":"mail.send","effects":["sends"]}' | da check
```

```json
{
  "ok": false,
  "decision": "never",
  "reason": "Send mail was not declared, so it is denied."
}
```

Fields: `plugin`, `permission`, `effects`, `card_id`. `ok` is true only when `decision` is `allowed`. `ask_first` and `never` still exit `0` and print that object. Exit `1` only when the request itself is bad (unknown plugin, bad effect name, JSON that does not parse).

On the plugin HTTP API, `/check` and `/call` return **403** for any decision other than `allowed`. The JSON body still contains `decision` and `reason`.

`card_id` counts as approval only when that card belongs to the plugin, its status is `acting`, `approved`, or `done`, its permission matches when both sides set one, and every requested effect is on the card.

## `da log`

A terminal, or empty stdin, lists the latest 200 ledger rows, newest first: `{"ok":true,"entries":[...]}`.

A JSON object on stdin appends one `plugin.log` row:

```bash
echo '{"plugin":"hello-watch","message":"noted","evidence":[{"label":"Tick","ref":"tick-1"}]}' | da log
```

## `da outcome`

Completes an external card. Status must already be `approved`, `done`, or `acting`. A `pending` card is refused.

```bash
echo '{"plugin":"hello-watch","card_id":"c_0123456789abcdef","summary":"Recorded the note"}' | da outcome
```

Optional `evidence` is a list of `{label, ref}`. The card becomes `done` and an `outcome` ledger row is appended.

## `da tile write`

Replaces one plugin's home tile. Requires JSON on stdin.

```bash
echo '{"plugin":"hello-watch","title":"Hello watch","status":"ok","lines":[{"label":"Ticks","value":"1"}]}' | da tile write
```

`lines` entries are `label` and `value`. This also appends a `tile.write` ledger row.
