# Adding a Composio toolkit

The plugin contract itself is in [plugins.md](plugins.md). This page is for a coding agent that needs to connect another service through Composio. Gmail and Outlook are already built in. Teams, calendars, contacts, files, and tasks are added the same way, without a new OAuth implementation and without changing `inbox-followups`.

Read [composio.md](composio.md) for the layer, the privacy rules, and why Teams is not connected yet.

## What you do not change

- `internal/composio/client.go`. It already speaks Composio's REST API.
- The secret keeper. The API key and `conn.<toolkit id>` are stored for you when someone presses Connect.
- The approval rule. A tool whose effects include `sends`, `spends`, or `deletes` does not run until an approved card covers it.
- Do not put the API key, a connection id, or a provider token in `plugin.yaml`, `toolkits.json`, the ledger, or a log line.

## Add a toolkit with configuration

Create `composio/toolkits.json` in the data directory (`~/.dap`, or `$DAP_HOME`). Mode `0600`. The next Connections refresh loads it. A restart is not required.

```json
{
  "toolkits": [
    {
      "id": "tasks",
      "slug": "todoist",
      "label": "Tasks",
      "tools": [
        {"slug": "TODOIST_GET_TASKS", "permission": "tasks.read"},
        {"slug": "TODOIST_CREATE_TASK", "permission": "tasks.write", "effects": ["sends"]}
      ]
    }
  ]
}
```

Rules the loader enforces:

- `id` is a short lowercase name. `slug` is the Composio toolkit slug.
- `label` is the Connections card title. The card appears next to Gmail and Outlook, with **Connect Tasks**.
- Each tool slug is the Composio tool name. `permission` looks like `tasks.read`.
- `effects` may only be `sends`, `spends`, or `deletes`. Leave it off for a read.
- Builtin ids (`gmail`, `outlook`) cannot be redefined. You cannot strip the send effect off `GMAIL_SEND_EMAIL`.
- The file is refused if it contains an `api_key` or a `connection_id`.

Find the real slug and tool names in Composio's toolkit docs, not by guessing. Confirm whether the toolkit accepts personal accounts or only work accounts before you tell someone to connect it.

## Call it from a plugin

Declare the same permissions in `plugin.yaml`. A permission the manifest does not declare is denied.

```yaml
permissions:
  - name: tasks.read
    mode: allowed
    description: Read tasks
  - name: tasks.write
    mode: ask_first
    description: Create a task as you
```

From the plugin, after `initialize` has stored `api_base` and `token`:

```http
POST {api_base}/connect/execute
Authorization: Bearer {token}
Content-Type: application/json

{
  "toolkit": "tasks",
  "tool": "TODOIST_GET_TASKS",
  "arguments": {}
}
```

A write that sends, spends, or deletes also needs the approved card:

```json
{
  "toolkit": "tasks",
  "tool": "TODOIST_CREATE_TASK",
  "arguments": {"content": "Reply to Dana"},
  "card_id": "c_..."
}
```

Propose that card the usual way (`kind: action`, `permission: tasks.write`, `effects: ["sends"]`). `act` calls `connect/execute` with the card id. The host checks the registry, the manifest, and the card. Unknown tools return an error and are not forwarded to Composio. Arguments are not written to the ledger. A sensitive tool records `connect.exec` and the tool slug only.

The user still has to save the Composio API key and press Connect on the new card. The plugin does not see the key.

## Add a mailbox the inbox plugin will watch

`inbox-followups` treats every connected toolkit with `"kind": "mail"` as another mailbox. It calls `/mail/threads`, `/mail/draft`, and `/mail/send`. It does not grow a provider branch.

For a mailbox whose Composio tools accept the generic argument names (`user_id`, `max_results`, `query`, `body`, `subject`, `recipient_email`, `to`, `thread_id`, `message_id`), add `kind` and `actions`:

```json
{
  "id": "fastmail",
  "slug": "fastmail",
  "label": "Fastmail",
  "kind": "mail",
  "actions": {
    "threads": "FASTMAIL_LIST_THREADS",
    "draft": "FASTMAIL_CREATE_DRAFT",
    "send": "FASTMAIL_SEND",
    "reply": "FASTMAIL_REPLY",
    "profile": "FASTMAIL_GET_PROFILE"
  },
  "tools": [
    {"slug": "FASTMAIL_LIST_THREADS", "permission": "mail.read"},
    {"slug": "FASTMAIL_GET_PROFILE", "permission": "mail.read"},
    {"slug": "FASTMAIL_CREATE_DRAFT", "permission": "mail.draft"},
    {"slug": "FASTMAIL_SEND", "permission": "mail.send", "effects": ["sends"]},
    {"slug": "FASTMAIL_REPLY", "permission": "mail.send", "effects": ["sends"]}
  ]
}
```

Action names the loader accepts are `threads`, `draft`, `draft_reply`, `send`, `reply`, and `profile`. The shared parser reads message lists shaped like Gmail (`messages`, `messageId`, `threadId`, `sender`) or Microsoft Graph (`value`, `id`, `conversationId`, `from.emailAddress`). If the tool's arguments or payload do not match, add a thin adapter instead of teaching the plugin about the provider.

## Thin adapter

Gmail and Outlook argument names differ, so they have dedicated builders in `internal/mail/broker_composio.go` (`draftCall` and `sendCall`). A new mailbox that needs the same treatment gets a branch there, plus a `Toolkit` in `Builtin()` in `internal/composio/registry.go` if it should ship in the binary rather than in `toolkits.json`.

That is the whole adapter. Do not add a second HTTP client, a sidecar, or a copy of the OAuth flow. Connect, list, revoke, and the secret keeper already cover the new id.

After the branch exists, `go test ./internal/mail/ ./internal/composio/` with Composio mocked. Do not call the live API from tests, and do not commit a real API key.
