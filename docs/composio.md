# Composio connectivity

DAP uses Composio as its connectivity layer. Gmail and Outlook are the first two connections. Both are fully working for reading threads, creating drafts, and sending. A send still waits for an approved card. New toolkits are added as configuration, or as a thin mail adapter, without changing the plugin protocol. How to add one is in [plugin-authoring.md](plugin-authoring.md).

This note is the decision record for that design. It was checked against Composio's docs on 1 November 2026.

## Decision

Two shapes were on the table.

**(a) Call Composio's hosted service from inside `da`.** OAuth, token refresh, and provider API calls happen on Composio's servers. DAP keeps an API key and one connection id per toolkit.

**(b) Run Composio as a sidecar** that DAP talks to on localhost.

**(a) is the one DAP ships.** It is a small Go HTTP client in `internal/composio`, compiled into the single binary. It speaks Composio REST API v3.1 (`https://backend.composio.dev/api/v3.1`). There is no new module in `go.mod`.

### Which SDKs actually exist

Composio's repository and docs list these official clients:

| Client | Package | Status |
| --- | --- | --- |
| TypeScript | `@composio/core` (and `@composio/slim`) | Official and current. Docs and the SDK use a session API. The package targets Node 22+. |
| Python | `composio` | Official. Python 3.10+. |
| CLI | `composio` | A shell client (`curl -fsSL https://composio.dev/install \| sh`). It is not a long-running daemon. |
| Go | none | No official Go SDK. `github.com/composiohq/composio` on pkg.go.dev is the polyglot monorepo (TypeScript, Python, CLI). It is not a Go API. |

Marketing that mentions a Go SDK is not backed by a published Go module. The TypeScript SDK does not belong in the inbox plugin: that plugin is Python (`inbox_followups.py`), and mail is a host capability. Putting `@composio/core` or the Python SDK in a plugin would put the API key in a plugin process. The key can read and send mail, so it stays in the `da` process, in the same secret keeper as other DAP secrets.

A sidecar was rejected for the same architecture reasons:

- Composio is a hosted API. A local process would still send every message through Composio, and it would not work offline. Gmail and Outlook do not work offline either way.
- Shipping a CLI or a Node process means per-OS packaging, signing, and `install.sh` changes. Those files belong to the release work. This change does not touch them.
- DAP is one Go binary plus plugin processes. A third process to supervise does not buy a permission boundary the host does not already have.

## What the layer does

`internal/composio` is the layer. The mail broker is a client of it.

| Piece | Where | Role |
| --- | --- | --- |
| Toolkit registry | `internal/composio/registry.go` plus `~/.dap/composio/toolkits.json` | Id, Composio slug, label, kind, actions, and the tools a plugin may call |
| Secrets | service `dap.composio` | Account `api_key` is the API key. Account `conn.<id>` is `{connection_id, auth_config_id, account, status}` |
| Connect | `Service.Connect` | Hosted Google or Microsoft sign-in. Composio returns a URL. DAP opens it and polls until the connection is `ACTIVE` |
| List | `GET /api/connections` | One row per toolkit, plus the demo mailbox. No key, no connection id, no provider token |
| Revoke | Disconnect | Deletes the remote connected account when Composio is reachable, then deletes `conn.<id>`. The API key stays |
| Execute | `POST /connect/execute` | A plugin names a toolkit and a tool slug. The registry says which manifest permission and effects that tool requires |

Plugins do not talk to Composio. They call the host with a bearer token and a declared permission:

- `GET /mail/threads?provider=gmail|outlook` for any toolkit whose kind is `mail`
- `POST /mail/draft` to create a draft
- `POST /mail/send` with `card_id` to send
- `POST /connect/execute` for a tool that is not one of those three mail actions

`graph` is still accepted as an alias of `outlook`, so older callers keep working. The Connections screen says Outlook.

Gmail and Outlook are interchangeable to `inbox-followups`. It watches every connected mailbox, drafts through the host, and sends only from `act` after approval. A remote draft is a copy in the mailbox. The approved card body is what is sent, so an edit on the card is not lost.

## First two connections

| | Gmail | Outlook |
| --- | --- | --- |
| Composio slug | `gmail` | `outlook` |
| Read | `GMAIL_FETCH_EMAILS` | `OUTLOOK_LIST_MESSAGES` (inbox and sent) |
| Draft | `GMAIL_CREATE_EMAIL_DRAFT` | `OUTLOOK_CREATE_DRAFT` or `OUTLOOK_CREATE_DRAFT_REPLY` |
| Send | `GMAIL_REPLY_TO_THREAD` when there is a thread, otherwise `GMAIL_SEND_EMAIL` | `OUTLOOK_REPLY_EMAIL` when there is a message id, otherwise `OUTLOOK_SEND_EMAIL` |
| Profile | `GMAIL_GET_PROFILE` | `OUTLOOK_GET_PROFILE` |
| Account types | Google accounts | Personal Microsoft accounts (Outlook.com, Hotmail, Live) and work or school accounts |
| Fallback | Legacy direct OAuth, `DAP_GMAIL_DIRECT=1` | **Direct Microsoft Graph** on the Outlook card. `DAP_OUTLOOK_DIRECT=1` forces it |

Composio-managed OAuth is available for both, so DAP does not register a Google Cloud client or an Entra app for the default path. Composio's consent screen lists the scopes. DAP still refuses to send until a card with effect `sends` is approved, even if the grant includes send.

The Graph fallback is unchanged: public client, PKCE, no client secret, personal Microsoft accounts, `Mail.Send` only on the first approved send. Direct Gmail is the same PKCE desktop flow as before, hidden unless `DAP_GMAIL_DIRECT=1` is set. Both direct paths keep their tokens in `dap.mail`, not in Composio.

## Teams, calendar, and what comes next

Teams is the next toolkit. It is not connected in this version, because a useful Teams connection is more than a registry row: posting needs an approved card, and personal Microsoft accounts cannot connect to Teams at all. The same layer can carry it later by the steps in [plugin-authoring.md](plugin-authoring.md). Calendar and contacts are the same kind of addition. Outlook's Graph fallback still has the optional `Calendars.Read` checkbox. Composio Outlook calendar tools are not called yet.

Checked against Composio's toolkit pages on 1 November 2026.

| Capability | Composio | Tool slugs | Personal Microsoft account | Work or school |
| --- | --- | --- | --- | --- |
| Outlook mail read | Supported. Wired here | `OUTLOOK_LIST_MESSAGES`, `OUTLOOK_GET_MESSAGE`, `OUTLOOK_SEARCH_MESSAGES` | Yes | Yes |
| Outlook draft | Supported. Wired here | `OUTLOOK_CREATE_DRAFT`, `OUTLOOK_CREATE_DRAFT_REPLY` | Yes | Yes |
| Outlook send | Supported. Wired here, behind a card | `OUTLOOK_SEND_EMAIL`, `OUTLOOK_REPLY_EMAIL`, `OUTLOOK_SEND_DRAFT` | Yes | Yes |
| Outlook calendar read | Supported by Composio. Not called by DAP | `OUTLOOK_LIST_EVENTS`, `OUTLOOK_SEARCH_EVENTS` (personal accounts fall back to a local calendar-view search) | Yes | Yes |
| Outlook calendar write | Supported by Composio. Not wired | `OUTLOOK_CALENDAR_CREATE_EVENT` | Yes | Yes |
| Teams chats, read | Supported by Composio. Not wired | `MICROSOFT_TEAMS_CHATS_GET_ALL_CHATS`, `MICROSOFT_TEAMS_TEAMS_LIST_CHAT_MESSAGES` | No. Composio's own guide says a personal account cannot connect to Teams | Yes |
| Teams channels, read | Supported by Composio. Not wired | `MICROSOFT_TEAMS_LIST_USER_JOINED_TEAMS`, `MICROSOFT_TEAMS_TEAMS_LIST_CHANNELS`, `MICROSOFT_TEAMS_TEAMS_LIST_CHANNEL_MESSAGES` | No | Yes |
| Teams post to a chat or channel | Supported by Composio. Would be effect `sends` | `MICROSOFT_TEAMS_TEAMS_POST_CHAT_MESSAGE`, `MICROSOFT_TEAMS_TEAMS_POST_CHANNEL_MESSAGE` | No | Yes |

The Outlook toolkit listed about 329 tools and the Teams toolkit about 238, both version `20261029_00`, with Composio-managed OAuth. Gmail listed about 63 tools. DAP declares only the tools in the table and in the Gmail registry, so a plugin cannot call an undeclared tool such as delete.

A personal Outlook consent can fail if the managed app asks for an organization-only scope such as `Chat.ReadWrite`. If Microsoft shows that error, use **Direct Microsoft Graph**, which requests `User.Read`, `Mail.ReadWrite`, `offline_access`, and `Mail.Send` only when a send is approved.

## Which key?

DAP sends the key as `x-api-key` to `backend.composio.dev`. That call needs a **project API key**.

| Prefix | What it is | What DAP does |
| --- | --- | --- |
| `ak_` | Project API key from the developer dashboard, **Settings → API Keys** | Saved after Composio accepts a short check, then shown as **Key verified** |
| `ck_` | Connect / MCP consumer key (`x-consumer-api-key`) | Refused on Save key. It is not stored |
| Anything else | Not a known DAP key | Soft warning. DAP still asks Composio; a rejection is not stored |

The consumer key and the project key are different credentials. Pasting a `ck_` key used to surface Composio’s raw 401, including a masked tail of the key. Save key now stops that on the Composio card, in plain language. A later failure (rejected key, sign-in setup, or the network) stays on that card. The request id, when Composio sends one, is under **Details**. The key itself is not repeated.

## Setup

1. Create a Composio account at [dashboard.composio.dev](https://dashboard.composio.dev/). The Hobby plan does not ask for a card.
2. Open **Settings → API Keys** and create a project API key. It starts with `ak_`. Do not paste a Connect consumer key (`ck_`).
3. `./da run`, open **Connections**, paste the key, and press **Save key**. The field is a password field and is cleared after a successful save.
4. Press **Connect Gmail** or **Connect Outlook**. Finish sign-in in the browser. DAP never asks for the Google or Microsoft password.
5. Install `mail-gmail`, `mail-graph`, and `inbox-followups` if they are not installed. Approving a follow-up card is what sends.

Disconnect removes that one connection. The key remains so the other toolkit stays connected. To remove the key, save an empty key only by deleting it from the secret keeper (`dap.composio` / `api_key`). The Connections page replaces a key when you paste a new one.

## Privacy and data flow

```
this laptop                         Composio                         Google or Microsoft
Connections UI                      holds the OAuth token            mailbox
API key + connection id    HTTPS    refreshes it            HTTPS    threads, drafts, sends
in the OS secret keeper             sees message contents            the account itself
```

Message contents, drafts, and sends pass through Composio's servers. That is the trade for not running DAP's own Google OAuth client. Composio's pricing page describes the company as SOC 2 Type II. Review their trust center before connecting a mailbox you would not send through a third party.

DAP does not read or reuse tokens stored by other apps. The key and connection ids are not written to `mail/public.json`, the ledger, plugin logs, or the status JSON the UI and plugins receive. The secret keeper is the OS store when it answers:

| OS | Store |
| --- | --- |
| macOS | Login keychain via `security` |
| Windows | Credential Manager |
| Linux | Secret Service via `secret-tool` |

If the OS store does not answer, DAP uses `secrets.json` mode `0600`. `DAP_SECRETS=file` forces that file. `DAP_SECRETS=os` refuses to start without the OS store. On macOS the `security -w` flag puts the secret on that short-lived process's argv. That is the existing keeper, documented in [runtime.md](runtime.md).

## Cost

Checked on 1 November 2026 at [composio.dev/pricing](https://composio.dev/pricing).

| Plan | Price | Tool calls | Connected accounts |
| --- | --- | --- | --- |
| Hobby | $0, no card | 100,000 / month, then the plan stops | Unlimited |
| Pro | $29 / month, includes a $29 usage credit | then $0.0003 per tool call | Unlimited |
| Enterprise | Custom | Custom | Custom |

A tool execution counts once. Trigger events are a separate meter (50,000 / month on Hobby, then $0.003) and DAP does not subscribe to triggers. New pricing applies to signups on or after 15 August 2026. A Composio article about shipping Gmail also describes a managed-app allowance of 20,000 calls and then $0.0005. That sentence does not match the pricing page. Treat the dashboard's usage panel as the source of truth for Vic's project, and expect a single mailbox's follow-up polling to stay inside Hobby.

## Releases

This change does not add a dependency, a sidecar, or an installer change. `go build` still produces one binary. Binary size does not grow by a bundled SDK. There is nothing extra to sign per OS, and `install.sh` is untouched. Private GitHub releases stay Mac zips, tarballs, and `install.sh`, with no Homebrew and no public hosting. Do not put the API key in a release artifact or in the repo.
