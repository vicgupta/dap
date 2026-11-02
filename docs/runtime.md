# Runtime

How the running core behaves: schedule, inbox, guardrails, ledger, secrets, doctor, files on disk, and the limits of this version.

## Scheduler

The loop wakes every second. For each plugin with a schedule it computes whether `next_run` is due.

- Before `next_run`: do nothing.
- Due, and less than or equal to 2 minutes late (`scheduler.Grace`): run with trigger `schedule`. On success, clear the missed banner.
- Due, and more than 2 minutes late: append `schedule.missed`, run **once** with trigger `catchup`, then set `next_run` to the next slot after now.

The missed line on Home is built from the first overdue time and the count of slots in `[first, now]`:

```text
Missed 7:00 AM run — DAP was not running or the laptop was asleep
Missed 3 runs since 7:00 AM — DAP was not running or the laptop was asleep. Caught up once.
```

The reason is that sentence because the core cannot tell sleep from the process having been stopped. It does not invent the skipped runs as if they had executed. A second tick after catch-up does not fire again until the new `next_run`.

If a run is already in progress, the tick skips that plugin and leaves `next_run` where it is, so the slot is tried again. **Run now** waits for the in-progress run, then runs with trigger `manual`. A manual success does not clear a missed banner.

Install sets `next_run` to the next future time. Nothing runs at install time unless a server is already up and the schedule is already due.

## Decision inbox

One queue for every plugin. Cards are stored in `state.json`.

| Status | Meaning |
| --- | --- |
| `pending` | Waiting for a person |
| `snoozed` | Hidden until `snooze_until`, then returned to `pending` by the scheduler |
| `acting` | `act` is in progress. A crash returns this to `pending` on the next start |
| `approved` | A person approved an external card. `da approved` lists these. `da outcome` finishes them |
| `done` | `act` or `outcome` succeeded |
| `rejected` | A person rejected it. A reason is required |
| `failed` | `act` or applying a change failed |
| `blocked` | Approval-time guardrail said `never` |

The inbox screen lists `pending`, then `snoozed`, unless you have reordered or pinned cards.

## Card layouts

Home and the decision inbox each have their own layout. The switcher sits in the page header: icon buttons, a radiogroup, arrow keys to move, Enter is not required because moving selects. A second group sets density.

| View | Home | Inbox |
| --- | --- | --- |
| Grid | Default. Small tiles span a third of the row, medium half, large the whole row. | Two cards across. |
| List | One column. | Default. One card under the next. |
| Board | Columns are Needs you, Attention, Down, and Healthy. Empty columns are hidden. | One column per plugin. |
| Split | — | List of titles plus Approve and Reject, with the open card beside them. Narrow windows show one pane at a time. |

Comfortable is the default density. Compact tightens padding and type. Approve, Edit, Reject, and Snooze stay labeled in every layout, including a collapsed card. Collapse hides the body. Pin keeps a card at the top of its view. Drag the handle to reorder, or focus the handle and press Up or Down.

The choice is stored per view in `localStorage["dap.layout"]` and in `config.json` under `layout`, through `GET` and `PUT /api/settings`. The browser value wins, then it is written back so the next browser on this machine starts there. `layout-boot.js` applies density and the view before the stylesheet loads, so the first paint matches. Defaults are Home grid and inbox list, both comfortable.

A plugin page follows the Home density. The contract is in [layout.md](layout.md). Tile span comes from `plugin.yaml` (`tile.size`, `tile.min`) until the person changes it on the card. See [plugins.md](plugins.md).

Keyboard, when the inbox is open and focus is not in a field or on a layout control:

| Key | Action |
| --- | --- |
| `j` or Down | Next card |
| `k` or Up | Previous card |
| `a` | Approve |
| `e` | Edit title, summary, and body |
| `r` | Ask for a reject reason |
| `s` | Snooze choices: 1 hour, 4 hours, tomorrow 08:00 local |

The API also accepts `{"until":"<RFC3339>"}` on `POST /api/inbox/{id}/snooze`. The time must be in the future.

Approve re-checks guardrails with the card marked approved. `never` blocks the card. Otherwise:

- `kind: change` merges `payload.config` and optional `payload.schedule`, re-runs `initialize`, and on failure restores the previous config and schedule.
- `actor: external` (what `da propose` creates) stays `approved` and does not call the plugin.
- Any other action card calls `act`, then becomes `done` or `failed`.

A plugin `action` card produced inside `watch`/`propose` is acted immediately when the decision is `allowed` and the effects do not include `sends`, `spends`, or `deletes`. Change cards are never auto-acted. Cards from `POST /propose` or `da propose` are never auto-acted.

Pending and snoozed cards do not stop the scheduler or **Run now**.

## Guardrails

`da check` and `POST /check` answer `allowed`, `ask_first`, or `never`.

Order:

1. Empty permission and no sensitive effect: `allowed`.
2. Named permission not in the approved manifest: `never`.
3. A stored override for that plugin (or `*`) and permission (or `*`) may raise the rank `allowed` < `ask_first` < `never`. It cannot lower it.
4. If effects include `sends`, `spends`, or `deletes`: `never` stays `never`. Otherwise the answer is `ask_first` until a covering card is approved, then `allowed`. This applies even when the manifest says `allowed`.
5. Otherwise the manifest mode is the answer. `ask_first` becomes `allowed` when a covering card is approved.

The Guardrails page can set an override to a tighter decision. The select offers the current mode and stricter ones (`never`, and `ask_first` when the current mode is `allowed`).

There is no notification channel. The inbox is the only place a decision waits.

## Ledger

`<data>/ledger.jsonl` is opened `O_APPEND`, mode `0600`. Each record is one JSON object and a newline, then `fsync`. There is no API to edit or delete a row. Concurrent appends are serialized. A corrupt line fails the read instead of being skipped.

Fields: `id`, `ts` (UTC), `plugin`, `kind`, `message`, optional `card_id`, `evidence` (`label`, `ref`), and `data`.

Kinds the core writes today:

| Kind | When |
| --- | --- |
| `plugin.install` / `plugin.remove` | Registry changes |
| `plugin.start` | Process started, or `initialize` failed |
| `schedule.run` | A run finished or failed |
| `schedule.missed` | Catch-up after a gap longer than 2 minutes |
| `card.proposed` / `card.approved` / `card.edited` / `card.rejected` / `card.snoozed` | Inbox |
| `card.acted` / `card.failed` / `card.blocked` | After approve |
| `config.applied` | A change card was applied |
| `guardrail.check` | A check that was not a hard denial |
| `permission.denied` | Decision `never` |
| `plugin.log` | `da log` or `POST /log` |
| `tile.write` | `da tile write` or `POST /tile` |
| `outcome` | `da outcome` |
| `mail.connect`, `mail.disconnect`, `mail.sent`, `mail.undo` | Mailbox connection changes and sends. Allowed mail reads and drafts are not logged. Messages do not include the API key or a connection id |
| `connect.exec` | A generic Composio tool ran with a send, spend, or delete effect. The tool slug is recorded. Arguments are not |
| `plugin.stop` | Quit asked the plugin to `shutdown`, or stopped it after the drain timeout |

`plugin.stop` is written when DAP quits. Removing one plugin with `da plugin remove` still does not append a row.

`da log` with no JSON lists the newest 200 rows. The Activity page asks for the same window.

## Secrets

`secrets.Keeper` is `Set`, `Get`, and `Delete` by service and account. Plugin HTTP uses service `dap.plugin.<plugin-name>`, so one plugin cannot read another's account names. The Composio API key is service `dap.composio`, account `api_key`. Each toolkit's connection id is service `dap.composio`, account `conn.<toolkit>`. Direct Gmail and Microsoft Graph tokens, when those paths are selected, stay in service `dap.mail`. None of these values are returned on the plugin API, written to the ledger, or written to `mail/public.json`.

`Open` builds the store like this:

- `DAP_SECRETS=file`: `secrets.json` only.
- `DAP_SECRETS=os`: the OS store, or an error if it cannot be probed.
- Unset or `auto`: probe the OS store (write and delete `dap.probe` / `probe`). On failure, use the file.

| OS | Backend name | Mechanism |
| --- | --- | --- |
| macOS | `keychain` | `security add-generic-password` / `find-generic-password` / `delete-generic-password`. The secret is passed with `-w`, so it is on that process's argv until the command exits. |
| Windows | `credential-manager` | `CredWriteW`, `CredReadW`, `CredDeleteW` for a generic credential `DAP:<service>:<account>`. |
| Linux | `secret-service` | `secret-tool` store, lookup, and clear. Each call times out after 3 seconds. |

The file is `secrets.json`, mode `0600`, in the data directory. Doctor reports the backend name and does not fail on `file`.

`POST /secret/get` returns the secret to that plugin only after `secrets.read` is declared and the decision is `allowed`. `set` and `delete` require `secrets.write`. An ask-first mode without an approved card is denied. The example plugins do not declare these permissions.

## `da doctor`

See [cli.md](cli.md) for the command. The check list is fixed:

1. `data_dir` is the absolute directory.
2. `ledger` opens.
3. `secrets` is the backend name.
4. `dependencies` is `resolved`, or the order error from the last start.
5. For each installed plugin, `scheduler:<name>` when it has a schedule, then `plugin:<name>`, then `permissions:<name>`.
6. `agents` looks up `claude`, `codex`, and `opencode` with `LookPath`.
7. `mail:<provider>` for each installed mailbox. See [inbox-followups.md](inbox-followups.md).

Info-level failures do not clear `ok`. Error-level failures do. A plugin that was installed while the server was stopped shows as not running until `da serve` or a doctor run starts it.

## Quitting

Quit in the sidebar, `da quit`, `da stop`, `SIGINT`, and `SIGTERM` share one path:

1. The scheduler stops, so it will not start a new run.
2. In-flight `act` calls, and any other plugin call still in progress, are given 10 seconds to finish.
3. Each idle plugin is sent `shutdown` (`{"reason":"quit"}`) and given 3 seconds to answer. A plugin that is still inside a call is stopped without that request.
4. The process is stopped. A `plugin.stop` row is appended.
5. The ledger is fsynced and closed. `state.json` is written and fsynced.
6. `run.json` is removed and `dap.lock` is released.
7. The HTTP server stops and the process exits 0. The page shows "DAP has quit. You can close this window." and tries to close itself. The chromeless window's browser process is stopped when DAP started it.

The confirm reads "Quit DAP? Scheduled agents stop running until you open DAP again." It is skipped when no plugin is running and nothing is in flight. If an approved action is executing, or a plugin call is in progress, the confirm says so.

`POST /api/quit` needs the admin token, a loopback client, and `X-DAP-Request: quit`. Plugin tokens cannot call it.

## Data directory

Mode `0700` on the directory. JSON state files are mode `0600`.

| Path | Role |
| --- | --- |
| `config.json` | `addr`, and `layout` once a view is changed. Created by `da init`. Default address `127.0.0.1:7391`. Other keys in the file are kept. |
| `state.json` | Installed plugins, config, schedule cursor, tiles, metrics, cards, guardrail rules, plugin scratch state. |
| `ledger.jsonl` | Append-only activity. |
| `secrets.json` | File backend only. |
| `dap.lock` | Exclusive lock held by `da serve` / `da run` and by a one-shot command. |
| `run.json` | While the server is up: `pid`, `addr`, `token`, `started`. Removed on a clean shutdown, before the lock is released. Mode `0600`. |
| `plugin-data/<name>/` | The `data_dir` passed to `initialize`. |
| `browser-profile/` | Private Chrome, Edge, or Chromium profile for the `da run` window. Quit can close that process without closing other browser windows. |
| `mail/public.json` | Direct-path client ids and the composio/direct choice. Not a token and not the Composio API key. Mode `0600`. |
| `mail/fake.json` | Demo mailbox threads and connection flag. |
| `composio/toolkits.json` | Optional extra Composio toolkits. Not a secret. Refused if it contains an API key or connection id. Mode `0600`. |

`state.json` keeps cards after `da plugin remove`. The ledger is never trimmed. There is no export command.

## Testing

```bash
go test ./...
```

`make test` is the same. Tests do not need a display, a keychain, or a network. They use the file secret store, temporary data directories, and the Python example plugins (so Python 3 has to be on `PATH` for `internal/core` and `internal/pluginhost`).

What they cover:

- Manifest validation, shorthand permissions and schedules, dependency order, semver ranges.
- Permission lookup and the guardrail order, including send/spend/delete and tighten-only overrides.
- Scheduler intervals, Monday cron, weekday skips, missed-run catch-up that does not replay.
- Ledger append under concurrency, and mode `0600`.
- File secret isolation between services.
- JSON-RPC method order and process stop.
- Both example plugins in one process: concurrent runs, approve then `act`, a second run while a card is pending, a missed run, a change card, a `never` override.
- Admin HTTP rejects a missing token and serves the UI with the session token.
- Quit drains an in-flight `act`, calls `shutdown`, releases `dap.lock`, removes `run.json`, and flushes the ledger. A plugin token cannot call `POST /api/quit`.
- CLI init, install preview, install, list, and `da check` of an undeclared permission.
- Gmail and Graph clients against mocked HTTP, PKCE sign-in with an incremental send scope, and an approval-gated demo send.

## Limitations

- The window is a browser, not a native webview. With no Chrome, Edge, or Chromium, Linux and macOS fall back to the system browser. Closing that window does not stop `da`. Quit, `da quit`, and Ctrl+C do. The Quit page says DAP has quit and asks the window to close; a chromeless app window is also stopped with the process when DAP launched it.
- The admin token is in the HTML of the loopback page and in `run.json`. That is the whole of admin authentication.
- Plugin UI isolation is a separate loopback origin, a bearer token, and Content-Security-Policy. It is not an OS sandbox.
- macOS keychain writes pass the secret on `security`'s argv. Use `DAP_SECRETS=file` if that matters. Linux `secret-tool` can block on a keyring prompt until the 3 second timeout, and then the file backend is used.
- Semver in manifests has no pre-release or build metadata.
- `every` cannot express days. Cron has no seconds field and no descriptors such as `@daily`.
- The core detects coding agents and does not launch them.
- `da version` prints the core version. A release build also prints `release`, `commit`, and `date`. Home and `da doctor` still report `version`.
- `da serve` ignores arguments it does not recognize.
- `da check` exits `0` when the decision is `never` or `ask_first`. Read `ok` and `decision`. The plugin HTTP API uses status 403 for those decisions.
- `plugin.stop` is written when DAP quits, not when a single plugin is removed.
- Stderr from a plugin is kept in memory for that process and is not a doctor field.
- One data directory, one server. A second `da serve` on the same directory fails the lock.
