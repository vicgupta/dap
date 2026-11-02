# Plugin authoring

A plugin is a directory with `plugin.yaml` and a program that speaks JSON-RPC 2.0 on stdin and stdout. DAP does not host a model. You, or your coding agent, write the program. The core installs it only after a person approves a change card.

The machine-readable contract is [schemas/plugin.schema.json](../schemas/plugin.schema.json). `da plugin validate <path>` checks that schema, then the semantic rules in `internal/manifest`, then starts the process and calls `initialize`, `watch`, `propose`, `act`, `report`, and an unknown method. Quit sends `shutdown` separately. A missing `shutdown` does not fail that quit.

This page is enough to write a plugin without reading the Go source. [plugins.md](plugins.md) is the shorter reference. [runtime.md](runtime.md) is how the scheduler, inbox, guardrails, and ledger behave. [cli.md](cli.md) is every `da` command.

Core version: `0.1.0`. Interface: `dap.plugin.v1`.

## Concepts

| Type | Use it when | Schedule | What the core does differently |
| --- | --- | --- | --- |
| `watcher` | Something should be noticed on a timer, and a person may need to approve a card. `hello-watch`, `examples/note-watch`. | Usually `every` or `cron`. | Nothing. The type is a label. |
| `capability` | Other plugins will depend on this one. It offers a service. | Usually none. Other plugins call it by being started after it. | Started before plugins that depend on it. |
| `workflow` | The work is an ordered list of steps, and some steps must pause. `plugins/workflow`. | Usually `every`. | Nothing besides the label. Pausing is your card with effects. |

All three types implement the same methods: `initialize`, `watch`, `propose`, `act`, `report`, and `shutdown`. Pick the label that matches the job so a person reading the Plugins page understands it.

## plugin.yaml

Unknown keys are rejected. `da plugin validate` reports the schema path and the message.

| Field | Required | Default | Rule |
| --- | --- | --- | --- |
| `name` | yes | | `^[a-z][a-z0-9-]{0,63}$`. Starts with a letter. |
| `version` | yes | | `MAJOR.MINOR.PATCH`. No pre-release tag. |
| `min_dap_version` | yes | | Same shape. Must be less than or equal to this core (`0.1.0`). |
| `type` | yes | | `watcher`, `capability`, or `workflow`. |
| `interface` | no | `dap.plugin.v1` | Only `dap.plugin.v1`. |
| `description` | no | empty | One sentence shown on the install card. |
| `entrypoint` | yes | | A string, or a list of strings. See below. |
| `dependencies` | no | none | A name plus a version range. The dependency must already be installed. |
| `permissions` | no | none | List or compact map. Undeclared names are denied. |
| `provides` | no | none | Labels, non-empty strings. |
| `config` | no | none | Keys the core will store and pass to `initialize`. |
| `schedule` | no | manual | One of `every`, `cron`, or `at`. |
| `tile` | no | medium | Home span: `size` and optional `min`, each `small`, `medium`, or `large`. `size` cannot be smaller than `min`. |
| `ui` | no | none | Static files served on their own loopback origin. |

### Entrypoint

- `runtime: node` runs the entry file with Node.js 20 or newer. The usual entry is the compiled `dist/index.js`.
- `runtime: bun` runs the entry with Bun, which can execute `src/index.ts` directly.
- `plugin.py` is run with `python3` on macOS and Linux, and `python` or `py` on Windows.
- Any other relative file without a runtime is executed directly.
- A list runs that argv. A bare first word such as `node` is looked up on `PATH`. A path with a slash stays inside the plugin directory. `..` that escapes the directory is rejected.

```yaml
runtime: node
entrypoint: dist/index.js
```

```yaml
runtime: bun
entrypoint: src/index.ts
```

```yaml
entrypoint: plugin.py
```

### Dependencies

Short form or object:

```yaml
dependencies:
  - web-watch ^0.3
  - name: other-plugin
    version: ">=1.0.0 <2.0.0"
```

| Range | Meaning |
| --- | --- |
| empty or `*` | any version |
| `1.2.3` or `=1.2.3` | exact |
| `1.2` or `1` | padded to `1.2.0` or `1.0.0` |
| `>=`, `>`, `<=`, `<` | comparison |
| `^1.2.3` | `>=1.2.3 <2.0.0`. `^0.2.3` is `<0.3.0`. `^0.0.3` is `<0.0.4`. |
| `~1.2.3` | `>=1.2.3 <1.3.0` |
| `>=1.0.0 <2.0.0` | space or comma means AND |

A missing dependency fails install. Cycles fail install. Start order is dependencies first.

### Permissions

Names match `^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)+$` (`notes.record`, `mail.read`). Duplicate names are rejected. Modes are only `allowed` and `ask_first`. The spellings `ask first` and `ask-first` are accepted and stored as `ask_first`. `never` is not a manifest mode.

```yaml
permissions:
  - name: notes.record
    mode: ask_first
    description: Record a local note
```

Compact map: a value of `allowed` or `ask first` sets the mode. Any other string is the description and the mode defaults to `ask_first`.

What a declaration allows:

| You declare | The core will |
| --- | --- |
| nothing for that name | deny the check (`never`) |
| `allowed`, and the effects are empty | let a plugin-originated action card run without waiting, when it is proposed inside a run |
| `ask_first`, or any effect in `sends`, `spends`, `deletes` | keep the card pending until a person approves |
| `secrets.read` or `secrets.write` | allow `POST /secret/get`, `/secret/set`, and `/secret/delete` on this plugin's own accounts when the decision is `allowed` |

A permission does not grant network, shell, or another plugin's files by itself. Those are your process. Do not take them unless the description says so.

`mail.read` allows `GET /mail/threads?provider=gmail|graph|fake`. `mail.send` allows `POST /mail/send`, and only when the body includes `card_id` for an approved card whose effects include `sends`. The SDK helpers are `mailThreads` and `mailSend` in TypeScript, and `mail_threads` and `mail_send` in Python. They never return an access token or a refresh token. Declare `mail.draft` when you only prepare text. A send still waits for a person even if you also declare the permission as `allowed`.

A person can tighten a permission (`allowed` to `ask_first`, or either to `never`). They cannot loosen one. Your manifest cannot override that.

### Schedule

Set only one of `every`, `cron`, or `at`. A string shorthand is also valid: `every 1h`, `cron 0 8 * * 1`, `at 08:00`, `weekdays at 08:00`, `weekly at 08:00`.

- `every` is a Go duration: `30s`, `15m`, `1h`, `1h30m`. Minimum `5s`. `2d` is not a Go duration. Use cron for a daily slot.
- `cron` is five fields: minute, hour, day of month, month, day of week. Sunday is `0`. No seconds and no `@daily`.
- `at` is `HH:MM` in the machine's local zone. `recurrence` is `daily` (default), `weekdays`, or `weekly`. Weekly requires `day` (`mon`, `monday`, `tue`, …).

Empty schedule means the plugin runs only from **Run now**.

The scheduler ticks once a second. A run up to 2 minutes late still counts as on time. Later than that, DAP records one missed run and catches up once. It does not replay skipped slots. Pending cards do not block the next run.

`watch` and `propose` share a 60 second budget. `initialize` waits 20 seconds. `act` waits 45 seconds. `report` waits 15 seconds. A failed `report` does not fail the run. `shutdown` waits 3 seconds when DAP quits. A missing `shutdown` does not stop the quit.

### Config

```yaml
config:
  - key: every
    type: duration
    description: How often to look
    default: 1h
    required: false
```

Types are `string`, `number`, `bool`, `duration`, and `list`. Keys cannot contain spaces. Unknown keys in stored config or a change card are rejected. `required: true` without a default fails install when the value is missing. The core passes the resolved map to `initialize` as `config`.

### UI

```yaml
ui:
  entry: ui/index.html
  route: /plugins/note-watch/
  title: Notes
```

`entry` must stay inside the plugin directory. `route` must start with `/plugins/` and defaults to `/plugins/<name>/`. The core serves the entry file's directory on a separate `127.0.0.1` port. `GET /plugins/<name>/` on the admin origin redirects there. The page receives `dap-token`, `dap-api`, `dap-plugin`, `dap-inbox`, `dap-theme`, and `dap-density`. It does not receive the admin token. Content-Security-Policy includes `frame-ancestors 'none'`.

The sandbox sets `<html data-theme>` and `<html data-density>` and serves `/theme.css`, `/theme-boot.js`, `/layout.css`, and `/layout-boot.js`. Link those before your own stylesheet. Style with the theme tokens (`--bg`, `--surface`, `--ink`, `--muted`, `--line`, `--accent`, and the rest) and the density tokens (`--card-pad`, `--card-gap`, `--stack-gap`, `--layout-gap`). Do not hardcode a light palette. See [theme.md](theme.md) and [layout.md](layout.md).

## JSON-RPC

One JSON object per line. The core writes a request and waits for the matching `id`. Do not print logs on stdout.

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
```

```json
{"jsonrpc":"2.0","id":1,"result":{}}
```

```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"unknown method nope"}}
```

The process environment keeps `PATH`, `HOME`, `USER`, `LOGNAME`, `LANG`, `LC_ALL`, `LC_CTYPE`, `TZ`, `SYSTEMROOT`, `WINDIR`, `TEMP`, `TMP`, `LOCALAPPDATA`, and `USERPROFILE` when set, plus `PYTHONUNBUFFERED=1`, `PYTHONIOENCODING=utf-8`, `DAP_PLUGIN`, `DAP_API`, and `DAP_TOKEN`. Other parent variables are not passed in. `DAP_TOKEN` is the plugin bearer token. It changes every start. It is not an OAuth token.

### initialize

```json
{
  "interface": "dap.plugin.v1",
  "dap_version": "0.1.0",
  "config": {},
  "plugin_dir": "/abs/plugin",
  "data_dir": "/abs/data/plugin-data/note-watch",
  "api_base": "http://127.0.0.1:12345/v1/note-watch",
  "token": "hex"
}
```

`data_dir` is created mode `0700`. It is this plugin's scratch directory. It is not the whole DAP data directory.

```json
{"ready": true, "interface": "dap.plugin.v1", "health": {"status": "ok", "message": "ready"}}
```

`health.status` is `ok`, `degraded`, or `error`. `ready: false` fails the start. The validator fails the check when `ready` is false.

### watch

Params: `{"since":"<RFC3339>"}` or `{}` before the first success. `since` is the previous successful run.

```json
{"observations": [{"id": "n1", "at": "2026-11-01T18:00:00Z", "summary": "A note appeared"}], "tile": {"title": "Notes", "status": "ok", "lines": [{"label": "Waiting", "value": "1"}]}}
```

Return a valid result even if a host call fails. `da plugin validate` uses a dummy API base and no server.

### propose

Params: `{"observations":[...]}`.

```json
{"cards": [{
  "title": "Save a note",
  "summary": "Nothing is sent.",
  "body": "The note text.",
  "kind": "action",
  "permission": "notes.record",
  "effects": [],
  "editable": true,
  "payload": {},
  "evidence": [{"label": "File", "ref": "note.txt"}]
}]}
```

`kind` is `action` or `change`. `effects` may only be `sends`, `spends`, and `deletes`. A card without a title fails validation. The core assigns the id and the risk.

A `change` card's payload is `{"config": {...}}` and optionally `{"schedule": {...}}`. The core applies it only after approval, then calls `initialize` again. If that call fails, the previous config is restored.

Inside a run, an `action` card from the plugin with decision `allowed` and no sensitive effect is acted on immediately. Every other card waits. `POST /propose` always waits.

Do not send, spend, or delete in `propose`. Propose a card and wait for `act`.

### act

Called only after guardrails pass and the card is approved. Not called for `change` cards and not called for external cards created with `da propose`.

Params: `{"card":{...}}` with `status` `acting`. The person may have edited the title, summary, and body.

```json
{"status": "done", "summary": "Recorded the note.", "evidence": [{"label": "Note", "ref": "n1"}]}
```

`status` is `done` or `failed`. `summary` is required. A JSON-RPC error marks the card `failed`.

### report

```json
{"period": "week", "from": "2026-10-26T00:00:00-07:00", "to": "2026-11-01T18:00:00-07:00"}
```

`from` is Monday 00:00 in the local zone.

```json
{"metrics": [{"key": "notes", "label": "Notes", "value": 1}]}
```

## Host calls

Base URL and token come from `initialize`. Send `Authorization: Bearer <token>`. A missing or wrong token is HTTP 401. Bodies are limited to 1 MiB.

| Method | Path | Gate |
| --- | --- | --- |
| GET | `/context` | This plugin's config, schedule, scratch state, open cards, health |
| GET | `/cards` | Card summaries |
| GET | `/state` | Scratch JSON. `null` if unset. Not a permission. |
| PUT | `/state` | Replace scratch JSON |
| POST | `/log` | Append a `plugin.log` ledger row. Body: `message`, optional `evidence` |
| POST | `/tile` | Replace the home tile |
| POST | `/propose` | Enqueue a card. Does not call `act` |
| POST | `/check` | `{"permission","effects","card_id"}`. HTTP 403 unless the decision is `allowed` |
| POST | `/call` | Same handler as `/check` |
| POST | `/outcome` | Finish a card this plugin owns that is `approved`, `done`, or `acting` |
| POST | `/secret/set` | Declared `secrets.write`, decision `allowed` |
| POST | `/secret/get` | Declared `secrets.read`, decision `allowed` |
| POST | `/secret/delete` | Declared `secrets.write`, decision `allowed` |

`card_id` counts as approval only when that card belongs to this plugin, its status is `acting`, `approved`, or `done`, the permission matches when both sides set one, and every requested effect is on the card.

Secrets are stored as service `dap.plugin.<name>`. You cannot read another plugin's accounts. You never see the admin token.

`POST /check` of an undeclared permission returns `never` and HTTP 403. `ask_first` is also 403 until a covering card is approved. The CLI `da check` prints the same decision and exits 0. Read `ok` and `decision`.

## Guardrails a plugin must respect

- Do the sensitive work in `act`, not in `watch` or `propose`.
- Put `sends`, `spends`, or `deletes` on the card when the action does that. The core will wait even if the permission mode is `allowed`.
- Use `kind: change` to edit your own config or schedule. Do not write `state.json` yourself.
- If a check returns `ask_first`, propose a card. If it returns `never`, stop. Do not retry with a different permission name to get around a denial.
- A person can tighten your permission later. Handle `never` on the next run.

## Plugin UI

Call `dap-api` (`/api/...` on the plugin origin) with the plugin token. The sandbox proxies that to `/v1/<name>/...`.

A plugin page can approve, edit, reject, or snooze its own cards: `GET /inbox` and `POST /inbox/{id}/approve|edit|reject|snooze`. Those calls use the plugin token and the same guardrails as the core inbox. The page can also send the person to `dap-inbox`. The core inbox keys are `j`/`k` move, `a` approves, `e` edits, `r` rejects, `s` snoozes.

Theme and density: link `/theme-boot.js`, `/layout-boot.js`, `/theme.css`, and `/layout.css` before your stylesheet. The sandbox sets `data-theme` (`system`, `light`, or `dark`) and `data-density` (`comfortable` or `compact`) from the query, then from saved settings. Use the tokens in those stylesheets. Compact density should tighten spacing, not hide actions. See [theme.md](theme.md) and [layout.md](layout.md).

## Data

| Path | Role |
| --- | --- |
| `~/.dap` or `$DAP_HOME` or `--data` | The DAP data directory, mode `0700` |
| `plugin-data/<name>/` | `data_dir` in `initialize`. Yours. |
| `drafts/<name>/` | Where **Create plugin** scaffolds a new plugin |
| `ledger.jsonl` | Append-only. You can add rows with `POST /log`. You cannot edit or delete rows. |
| `state.json` | The core's copy of config, cards, and your scratch JSON |

`da doctor` checks the data directory, the ledger, secrets, dependency order, each plugin's next run, health, permission approval, and whether `claude`, `codex`, and `opencode` are on `PATH`. Missing agents are informational. A plugin that is down fails the command. The report is JSON and the exit code is 1 when `ok` is false.

`da log` with no stdin lists the latest 200 ledger rows.

## Install, approve, remove

```bash
da plugin new note-watch --type watcher
da plugin validate ./note-watch
da plugin dev ./note-watch
da plugin install ./note-watch          # previews permissions, installs nothing
da plugin install ./note-watch --approve
da plugin list
da plugin approve note-watch            # after plugin.yaml gains a permission or loosens a mode
da plugin remove note-watch
```

`install` records the absolute path. It does not copy the folder. Adding a permission, or changing `ask_first` to `allowed`, will not start until `da plugin approve` or the Plugins page.

**Create plugin** in the UI scaffolds into `drafts/`, runs your coding agent there, validates, and puts a change card in the inbox. Approving the card is what installs or reloads. The UI never installs by itself.

`da plugin dev` prints one JSON object per line (`ready`, `stderr`, `reload`, `stop`) and restarts the process when a file in the directory changes. Ctrl+C stops it.

`da plugin validate` prints one JSON object. On failure the exit code is 1 and stdout still has the checks. Fix every check whose `ok` is false.

```json
{"ok": false, "name": "note-watch", "checks": [{"name": "propose", "ok": false, "detail": "card 0 is missing a title"}]}
```

## Security

Do:

- Keep tokens, if you must store one, in `POST /secret/set` after declaring `secrets.write`.
- Treat `DAP_TOKEN` as a bearer for your own API only.
- Stay inside `plugin_dir` and `data_dir`.
- Return a JSON-RPC error for methods you do not understand.

Do not:

- Print secrets, the admin token, or OAuth tokens on stdout or into a card body.
- Read `run.json`, `secrets.json`, or another plugin's `plugin-data`.
- Bypass an `ask_first` or `never` result.
- Shell out to `curl` the admin API.
- Add a permission you do not use.

## Tutorial: note-watch

The TypeScript reference is [plugins/hello-watch](../plugins/hello-watch). `da plugin validate plugins/hello-watch` passes. [examples/note-watch](../examples/note-watch) is the same contract in Python.

1. `da plugin new note-watch --type watcher` writes TypeScript. `runtime: node` and `entrypoint: dist/index.js`. The source imports `@dap/plugin-sdk` from `vendor/plugin-sdk`.
2. The manifest declares `note.watch` as `ask_first` and runs every hour. Nothing is sent, so `effects` is empty. The card still waits because the mode is ask-first.
3. `src/index.ts` answers `initialize`, `watch`, `propose`, `act`, `report`, and `shutdown` through `serve()`. Unknown methods return a JSON-RPC error. `npm test` builds and checks `initialize` with `node:test`.
4. `da plugin validate .` builds when `dist` is stale, then must print `"ok": true`.
5. Install only after you have read the permission sentence, with `da plugin install . --approve`, or by approving the change card from **Create plugin**.

Python is the other supported language: `da plugin new notes-py --lang python`. It copies `dap_plugin.py` and a `plugin.py` that calls `dap_plugin.serve`. `plugins/workflow` is a larger Python example. Go is the language of the `da` binary, not of plugins.

## Minimal TypeScript plugin

`plugins/hello-watch/src/index.ts` is the reference. A new plugin looks like this:

```ts
import { serve, type PluginHandler } from "../vendor/plugin-sdk/dist/index.js";

const plugin: PluginHandler = {
  async initialize() {
    return { ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "ready" } };
  },
  async watch() {
    return { observations: [], tile: { title: "Notes", status: "ok", lines: [] } };
  },
  async propose() {
    return { cards: [{ title: "Save a note", summary: "Nothing is sent.", kind: "action", permission: "note.watch", effects: [] }] };
  },
  async act() {
    return { status: "done", summary: "Recorded the note." };
  },
  async report() {
    return { metrics: [{ key: "notes", label: "Notes", value: 1 }] };
  },
  async shutdown() {
    return { ok: true };
  },
};

serve(plugin);
```

## Minimal Python loop

`da plugin new name --lang python` writes this shape:

```python
import dap_plugin

def initialize(params, api):
    return {"ready": True, "interface": "dap.plugin.v1", "health": {"status": "ok", "message": "ready"}}

def watch(params, api):
    return {"observations": [], "tile": {"title": "Notes", "status": "ok", "lines": []}}

def propose(params, api):
    return {"cards": [{"title": "Save a note", "summary": "Nothing is sent.", "kind": "action", "permission": "note.watch", "effects": []}]}

def act(params, api):
    return {"status": "done", "summary": "Recorded the note."}

def report(params, api):
    return {"metrics": [{"key": "notes", "label": "Notes", "value": 1}]}

def shutdown(params, api):
    return {"ok": True}

if __name__ == "__main__":
    dap_plugin.serve({
        "initialize": initialize, "watch": watch, "propose": propose, "act": act, "report": report, "shutdown": shutdown,
    })
```

`dap_plugin.Host` wraps config, scratch state, secrets, and `mail_threads` / `mail_send`. `mail_send` still requires an approved card. The helper never returns an OAuth token.

## Checklist

- [ ] `name`, `version`, `min_dap_version`, and `type` match the schema.
- [ ] Permissions are the smallest set. Sensitive work uses `effects`.
- [ ] `watch` and `propose` do not send, spend, or delete.
- [ ] `act` is safe to run only after approval, and it returns `status` and `summary`.
- [ ] Unknown methods return a JSON-RPC error. Stdout has nothing else.
- [ ] `da plugin validate .` prints `"ok": true`.
- [ ] A person approves the install card before the plugin is enabled.
