# Plugins

A plugin is a directory with `plugin.yaml` and an entrypoint. `da plugin install` records that directory's absolute path. It does not copy the files. Rebuild `da` only when the core changes.

To write one, follow [plugin-authoring.md](plugin-authoring.md). The schema is [schemas/plugin.schema.json](../schemas/plugin.schema.json). `da plugin validate <path>` checks it.

The shipped reference watcher, `plugins/hello-watch`, is TypeScript and uses `@dap/plugin-sdk`. `plugins/workflow` and the mail connectors stay Python. Any language that can speak the JSON-RPC contract still works. New plugins should be TypeScript unless you pass `--lang python`.

## `plugin.yaml`

```yaml
name: competitor-research       # ^[a-z][a-z0-9-]{0,63}$
version: 0.1.0                  # MAJOR.MINOR.PATCH. No pre-release tag.
min_dap_version: 0.1.0          # required. Must be <= this core (0.1.0).
type: watcher                   # watcher | capability | workflow
interface: dap.plugin.v1        # optional. Defaults to dap.plugin.v1.
description: Compare public pages.
entrypoint: plugin.py           # or [python3, plugin.py]
dependencies:
  - web-watch ^0.3              # short form, or {name, version}
permissions:
  - name: web.read              # dotted, like mail.read or share.team
    mode: allowed               # allowed | ask_first. "ask first" is accepted.
    description: Read public pages
  - name: share.team
    mode: ask_first
    description: Share with the team
provides: [alerts, brief]       # optional labels
config:
  - key: every
    type: duration              # string | number | bool | duration | list
    description: How often to run
    default: 1h
    required: false
schedule: every 2h              # optional. See below.
ui:                             # optional
  entry: ui/index.html
  route: /plugins/competitor-research/
  title: Comparison board
```

Unknown top-level keys are rejected. The core version this binary reports is `0.1.0`. Only interface `dap.plugin.v1` is accepted. If the plugin answers `initialize` with another interface id, start fails.

### Types

The three types share one contract. The type is a label for people and for how you write the plugin:

| Type | Role in this core |
| --- | --- |
| `watcher` | Watches something and proposes cards. `hello-watch`. |
| `capability` | Meant to be depended on by other plugins. Same five calls. |
| `workflow` | Runs an ordered list. A schedule is its watch. `workflow`. |

The host does not treat the types differently except to store and display them.

### Dependencies

Each dependency is a name plus a version range. The dependency must already be installed. Ranges:

| Form | Meaning |
| --- | --- |
| empty or `*` | any version |
| `1.2.3` or `=1.2.3` | exact |
| `1.2` or `1` | padded to `1.2.0` or `1.0.0` |
| `>=`, `>`, `<=`, `<` | comparison |
| `^1.2.3` | `>=1.2.3 <2.0.0`. A leading zero narrows it: `^0.2.3` is `<0.3.0`, `^0.0.3` is `<0.0.4` |
| `~1.2.3` | `>=1.2.3 <1.3.0` |
| `>=1.0.0 <2.0.0` | space or comma means AND |

Start order is dependencies first. Cycles and duplicate names are errors. A missing dependency fails `install`, not a later surprise.

### Permissions

Names match `^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)+$` (`web.read`, `share.team`). Duplicate names are rejected. Modes are only `allowed` and `ask_first`. `never` is not a manifest mode. It is the decision for an undeclared permission, or for a guardrail override.

A compact map is also accepted:

```yaml
permissions:
  web.read: allowed
  share.team: ask first
  files.write: local only    # not a mode, so this becomes ask_first
```

`allowed` and `ask first` / `ask_first` set the mode. Any other scalar is the description and the mode defaults to `ask_first`.

The install preview shows `description — allowed` or `description — ask first`. With an empty description, the core fills a short phrase for `mail.read`, `mail.draft`, `mail.send`, `web.read`, `files.write`, `share.team`, `secrets.read`, and `secrets.write`. Those names are not special-cased beyond the sentence. Declaring `mail.send` does not connect a mailbox. This core has no mail client.

Anything not declared is denied. A later manifest that adds a name, or that changes `ask_first` to `allowed`, will not start until `da plugin approve <name>`. Changing `allowed` to `ask_first` is covered by the old approval.

### Schedule

Set only one of `every`, `cron`, or `at`. A string shorthand is accepted: `every 2h`, `cron 0 8 * * 1`, `at 08:00`, `weekdays at 08:00`, `weekly at 08:00`.

- `every` is a Go duration: `30s`, `15m`, `2h`, `1h30m`. Minimum `5s`. `2d` is not a Go duration. Use cron for a daily slot.
- `cron` is five fields: minute, hour, day of month, month, day of week. `@daily` and seconds-level cron are not accepted. In this parser, Sunday is `0`.
- `at` is `HH:MM` in the machine's local zone. `recurrence` is `daily` (the default), `weekdays`, or `weekly`. Weekly requires `day` (`mon`, `monday`, `tue`, …).

Empty schedule means the plugin runs only from **Run now** or a direct host call.

On install the next run is the next future slot, not "now". The plugin does not fire during install.

### Config

Keys are unique and cannot contain spaces. Types:

| Type | JSON / YAML shape |
| --- | --- |
| `string` | string |
| `number` | number |
| `bool` | bool |
| `duration` | Go duration string, at least `5s` |
| `list` | array |

Unknown keys in a stored config or a change card are rejected. Defaults are copied at install. `required: true` without a default fails install when the value is missing.

### Tile size

Optional. This is the Home dashboard tile, not the plugin's own page.

```yaml
tile:
  size: medium    # small | medium | large. Default medium.
  min: small      # smallest size the Home control may use. Default small.
```

`size` is the span until the person picks another. `min` is a floor: a plugin that needs room can set `min: medium` and Home will not offer small. `size` cannot be smaller than `min`. On the grid, small is one third of the row, medium is half, and large is the full row. List and board keep the card readable; the span applies when Home is in the grid.

`hello-watch` asks for small. `workflow` stays medium. `inbox-followups` asks for large and will not go below medium.

### UI

`entry` is a file inside the plugin directory. `..` is rejected. `route` must start with `/plugins/`. It defaults to `/plugins/<name>/`. `title` defaults to the plugin name. The core serves the entry file's directory on a private loopback port. See [architecture.md](architecture.md).

Plugin pages do not share the core origin, so they cannot read the core page's `localStorage`. DAP passes the Home density the same way it passes theme. See [layout.md](layout.md).

### Entrypoint

A string or an argv list. Paths stay inside the plugin directory.

- `plugin.py` is run with `python3` on macOS and Linux, or `python` / `py` on Windows.
- Any other relative file is executed directly.
- An argv list whose first element contains a slash is resolved inside the plugin directory. A bare name such as `python3` is left for `PATH`.

## Interface `dap.plugin.v1`

Newline-delimited JSON-RPC 2.0. The core writes a request and waits for the matching `id`. Do not print logs on stdout. Use stderr.

A run calls `initialize`, `watch`, `propose`, `act`, and `report`. When DAP quits it also calls `shutdown`. An unknown method should return an error. The core treats that as a failed call, except `shutdown`: if that method is missing or slow, DAP still stops the process.

Request:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
```

Success:

```json
{"jsonrpc":"2.0","id":1,"result":{}}
```

Failure:

```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"what broke"}}
```

### `initialize`

Called at start, and again after an approved change card.

Params:

```json
{
  "interface": "dap.plugin.v1",
  "dap_version": "0.1.0",
  "config": {},
  "plugin_dir": "/abs/path/to/plugin",
  "data_dir": "/abs/path/to/data/plugin-data/<name>",
  "api_base": "http://127.0.0.1:12345/v1/<name>",
  "token": "hex token"
}
```

`data_dir` is created mode `0700`. `token` is also in `DAP_TOKEN`. It changes every time the process starts.

Result:

```json
{
  "ready": true,
  "interface": "dap.plugin.v1",
  "health": { "status": "ok", "message": "Ready" }
}
```

`health.status` is `ok`, `degraded`, or `error`. `ready: false` fails the start. Health is what Home and doctor show.

### `watch`

Params: `{"since":"<RFC3339>"}` or `{}` before the first success. `since` is the previous successful run.

Result:

```json
{
  "observations": [
    { "id": "tick-1", "at": "2026-11-01T18:00:00Z", "summary": "Watch ticked", "data": {} }
  ],
  "tile": {
    "title": "Hello watch",
    "status": "ok",
    "lines": [{ "label": "Ticks", "value": "1" }]
  }
}
```

The core stores the tile when `title` or `lines` is set.

### `propose`

Params: `{"observations":[...]}` using the list `watch` just returned.

Result: `{"cards":[ ... ]}`. Each card:

```json
{
  "title": "Say hello",
  "summary": "The watch ticked. Nothing is sent.",
  "body": "Hello from hello-watch.",
  "kind": "action",
  "permission": "demo.greet",
  "effects": [],
  "editable": true,
  "payload": {},
  "evidence": [{ "label": "Watch", "ref": "tick-1" }]
}
```

`kind` is `action` or `change`. `effects` is any of `sends`, `spends`, `deletes`. The core assigns the id, the risk (`allowed`, `ask_first`, or `never`), and the status.

A `never` card is not stored. A `change` card always waits. Payload for a change:

```json
{ "config": { "every": "30m", "steps": [] }, "schedule": { "every": "30m" } }
```

If `schedule` is omitted and `config.every` is set, and the current schedule is interval-based, the core updates that interval.

Inside a run, an `action` card from the plugin with decision `allowed` and no sensitive effect is acted on immediately. Every other card waits. Proposing over `POST /propose` always waits, so a plugin can enqueue the next card from inside `act` without deadlocking.

### `act`

Called only after the guardrail check passes and a person, or the auto-act rule above, has approved the card. Not called for `change` cards and not called for external cards.

Params: `{"card":{...}}`. `status` is `acting` while this call runs. `body` and `title` may have been edited.

Result:

```json
{
  "status": "done",
  "summary": "Hello. Recorded “Say hello”.",
  "evidence": [{ "label": "Hello", "ref": "hello-watch" }],
  "output": {}
}
```

A JSON-RPC error marks the card `failed`.

### `report`

Params:

```json
{ "period": "week", "from": "2026-10-26T00:00:00-07:00", "to": "2026-11-01T18:00:00-07:00" }
```

`from` is Monday 00:00 in the machine's local zone. `to` is the run time.

Result:

```json
{ "metrics": [{ "key": "ticks", "label": "Ticks", "value": 1 }] }
```

Metrics are stored on the plugin and shown on Home when the UI has them. A failed `report` does not fail the run.

### `shutdown`

Sent once when the person quits DAP, runs `da quit` / `da stop`, or the process receives `SIGINT` or `SIGTERM`. The core has already stopped the scheduler from starting new runs and has waited up to 10 seconds for in-flight `act` calls.

Params:

```json
{ "reason": "quit" }
```

Result:

```json
{ "ok": true }
```

Finish any private work, then return. DAP waits up to 3 seconds for this reply, appends a `plugin.stop` ledger row, and then stops the process. Do not start a new watch from `shutdown`.

## Minimal loop

```python
#!/usr/bin/env python3
import json, sys

def reply(i, result=None, error=None):
    msg = {"jsonrpc": "2.0", "id": i}
    if error:
        msg["error"] = {"code": -32000, "message": str(error)}
    else:
        msg["result"] = result
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()

def handle(method, params):
    if method == "initialize":
        return {"ready": True, "interface": "dap.plugin.v1",
                "health": {"status": "ok", "message": "ready"}}
    if method == "watch":
        return {"observations": [], "tile": {"title": "Example", "status": "ok", "lines": []}}
    if method == "propose":
        return {"cards": []}
    if method == "act":
        return {"status": "done", "summary": "done"}
    if method == "report":
        return {"metrics": []}
    if method == "shutdown":
        return {"ok": True}
    raise RuntimeError("unknown method " + method)

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    req = json.loads(line)
    try:
        reply(req.get("id"), result=handle(req.get("method"), req.get("params") or {}))
    except Exception as exc:
        reply(req.get("id"), error=str(exc))
```

Call the scoped API with `Authorization: Bearer <token>` and `api_base` from `initialize`. Flush stdout after every response. The same program in another language is a loop that reads a line, parses JSON, and writes one line.

## The examples

`hello-watch` has no config and no UI. `demo.greet` is `ask_first`, so each run leaves a card in the inbox. `act` returns a hello and an evidence ref. State is in memory, so the tick counter resets when the process restarts.

`workflow` stores its cursor with `PUT /state`. Safe steps call `POST /check` with `workflow.run` and no effects, then `POST /log`. A step whose effects include `sends`, `spends`, or `deletes` becomes one card with permission `share.team` and waits. `act` calls `POST /call` with that card id before it records the step, then runs any following safe steps. The step editor (`plugins/workflow/ui`) saves by `POST /propose` with `kind: change` and permission `workflow.rules`. The new steps and `every` apply only after that card is approved.

Both can be installed together. They do not depend on each other, and a pending card in one does not stop the other.

`mail-fake`, `mail-gmail`, and `mail-graph` are capability plugins. `inbox-followups` is a watcher that proposes drafts and sends only after you approve a card. Gmail and Outlook are interchangeable mail providers on the Composio layer. The host stores the API key and connection ids. The setup is in [composio.md](composio.md) and [inbox-followups.md](inbox-followups.md). Adding another toolkit is in [plugin-authoring.md](plugin-authoring.md).
