# Inbox follow-ups

`inbox-followups` watches connected mailboxes for threads you owe a reply to and threads that are waiting on someone else. It proposes a draft as a decision card. Approving the card is what sends it. Edit, reject, and snooze work in the plugin's own page and in the core decision inbox. The plugin page uses the same keys as the core inbox: `j` / `k` move, `a` approves, `e` edits, `r` rejects, and `s` snoozes for one hour.

Nothing sends unless a person approves a card. Gmail and Outlook are the same kind of mailbox: the plugin asks the host for threads, drafts, and sends, and does not care which provider a thread came from. Composio holds the Google and Microsoft tokens. The host stores the Composio API key and the connection id for each toolkit. Plugin processes never receive those values. They call `mail.read`, `mail.draft`, and `mail.send` on the host.

## Install

```bash
./da plugin install ./plugins/mail-fake --approve
./da plugin install ./plugins/mail-gmail --approve
./da plugin install ./plugins/mail-graph --approve
./da plugin install ./plugins/inbox-followups --approve
./da run
```

| Plugin | Type | What it does |
| --- | --- | --- |
| `mail-fake` | capability | Demo mailbox. Example threads, no account, no token. |
| `mail-gmail` | capability | Gmail through Composio. |
| `mail-graph` | capability | Outlook through Composio. Microsoft Graph is the direct fallback. |
| `inbox-followups` | watcher | Every hour, proposes follow-up drafts for every connected mailbox. `mail.send` is ask-first. |

`inbox-followups` config: `wait_hours` (default 24), `signature` (default `Vic`), `skip_domains` (comma-separated domains that never get a draft), and `preview_rule`.

Open **Connections**. Save a Composio API key, then **Connect Gmail** and **Connect Outlook**. Sign-in happens in the browser. The demo mailbox still connects with one click and does not use Composio. The API key and connection ids are stored in `./.dap/secrets.json` under service `dap.composio` (mode `0600`). Disconnect revokes that toolkit's connection and keeps the API key.

`mail.send` still requires an approved card whose effects include `sends`. A draft created while proposing is not a send. The approved card body is what goes out.

Outlook can use **Direct Microsoft Graph** instead of Composio. That path keeps a public client id in `mail/public.json` and the Microsoft token in `dap.mail`. Direct Gmail is legacy: set `DAP_GMAIL_DIRECT=1` before `da run`. `DAP_OUTLOOK_DIRECT=1` forces the Graph path the same way. Setup for the Composio account is in [composio.md](composio.md).

## Host API

Plugins call these on their scoped API (`api_base` from `initialize`). The bearer token is the plugin token, not the admin token.

| Method | Path | Rule |
| --- | --- | --- |
| GET | `/mail/status` | Connection rows. No tokens. |
| GET | `/mail/threads?provider=gmail\|outlook\|graph\|fake` | Requires declared `mail.read`. `graph` is an alias of `outlook`. Allowed reads are not written to the ledger, because the follow-ups page polls. |
| POST | `/mail/draft` | Requires declared `mail.draft`. On Composio this creates a Gmail or Outlook draft and returns `draft_id`. It does not send. Direct paths keep the draft on the card (`local: true`). |
| POST | `/mail/send` | Requires declared `mail.send`, effects `sends`, and `card_id` of an approved card that covers that permission. |
| POST | `/mail/undo` | Demo mailbox only. Gmail and Outlook refuse. |

The follow-ups page can also list and approve its own cards (`GET /inbox`, `POST /inbox/{id}/approve|edit|reject|snooze`). Those calls use the same guardrails as the core inbox. The sandbox does not have the admin token.

The admin UI uses `GET /api/connections` and `POST /api/connections/{provider}/setup|connect|disconnect`. `{provider}` is `gmail`, `outlook` (`graph` still works), `fake`, or `composio` when the body only saves the API key. The response includes `key_saved` and never includes the key or a connection id.

## Doctor

For each installed mailbox, `da doctor` adds `mail:gmail`, `mail:graph`, or `mail:fake`. The detail says whether a token is present, whether refresh worked, whether send scope is granted, and any throttle window. A connected Gmail or Outlook mailbox whose refresh fails is an error and makes `da doctor` exit `1`. The demo mailbox does not.

Ledger rows for this feature use kinds `mail.connect`, `mail.disconnect`, `mail.sent`, and `mail.undo`.

## Files

| Path | Role |
| --- | --- |
| `mail/public.json` | Direct-path client ids, the Outlook calendar flag, and the composio/direct choice. Mode `0600`. Not a token and not the Composio API key. |
| `mail/fake.json` | Demo threads and whether the demo mailbox is connected. |

## Gmail and Outlook

Use Composio. The steps are in [composio.md](composio.md). DAP does not ask you to create a Google Cloud project.

While proposing, the watcher calls `POST /mail/draft` for Gmail and Outlook. That saves a copy in the mailbox. Approving the card sends the card body, not a stale draft, so an edit on the card is what the recipient gets. The remote draft is left in the mailbox.

## Legacy direct Gmail

Set `DAP_GMAIL_DIRECT=1` and restart `da`. Connections then asks for a Desktop OAuth client id. The flow is PKCE, with no client secret. Scopes are `gmail.readonly` and `gmail.compose` at connect, and `gmail.send` the first time a send is approved. Google restricted-scope verification is not part of this setup. This path stores the Google token in `dap.mail`. Leave it unset unless you are keeping an existing client id.

## Outlook direct fallback (Microsoft Graph)

1. In Microsoft Entra, register an application.
2. Supported account types: **Personal Microsoft accounts only**.
3. Add a **Mobile and desktop applications** platform. Set the redirect URI to `http://localhost`. Turn on **Allow public client flows**.
4. Do not create a client secret. This is a public client with PKCE.
5. Permissions at sign-in: `User.Read`, `Mail.ReadWrite`, `offline_access`. `Mail.Send` is requested only when the first send is approved. `Calendars.Read` stays off unless you check it on Connections.
6. On the Outlook card, press **Microsoft Graph instead**, paste the application (client) id, and press **Save client id**. Then press **Connect**. Sign-in, including MFA, happens in Microsoft's window. DAP does not read Outlook's stored credentials or tokens. **Use Composio** switches back.
7. On Windows, macOS, and Linux the sign-in uses the system browser and a localhost redirect, which is the MSAL public-client pattern. A native Web Account Manager broker is not linked in this build, so Windows uses that same browser flow.

Polling uses Graph delta queries. Threads are grouped by `conversationId`. A 429 or 503 honors `Retry-After`. A wait of two seconds or less is retried once. A longer wait is recorded and shown by doctor instead of blocking the laptop.
