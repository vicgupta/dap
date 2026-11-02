# Inbox follow-ups

`inbox-followups` watches connected mailboxes for threads you owe a reply to and threads that are waiting on someone else. It proposes a draft as a decision card. Approving the card is what sends it. Edit, reject, and snooze work in the plugin's own page and in the core decision inbox. The plugin page uses the same keys as the core inbox: `j` / `k` move, `a` approves, `e` edits, `r` rejects, and `s` snoozes for one hour.

Nothing sends unless a person approves a card. Tokens stay in the `da` process. Plugin processes and `da` commands never receive an access token or a refresh token. They call `mail.read` and `mail.send` on the host.

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
| `mail-gmail` | capability | Gmail. The host holds the OAuth token. |
| `mail-graph` | capability | Outlook for a personal Microsoft account. The host holds the token. |
| `inbox-followups` | watcher | Every hour, proposes follow-up drafts. `mail.send` is ask-first. |

`inbox-followups` config: `wait_hours` (default 24), `signature` (default `Vic`), `skip_domains` (comma-separated domains that never get a draft), and `preview_rule`.

Open **Connections**. **Demo mailbox** connects with one click. Gmail and Outlook need a public client id, which you paste there. DAP stores tokens with the secret keeper under service `dap.mail` (OS keychain, or `secrets.json` mode `0600` when the keychain does not answer). Disconnect deletes that token and keeps the client id.

`mail.send` still requires an approved card whose effects include `sends`. The send scope is requested from Google or Microsoft only on the first approved send.

## Host API

Plugins call these on their scoped API (`api_base` from `initialize`). The bearer token is the plugin token, not the admin token.

| Method | Path | Rule |
| --- | --- | --- |
| GET | `/mail/status` | Connection rows. No tokens. |
| GET | `/mail/threads?provider=gmail\|graph\|fake` | Requires declared `mail.read`. Allowed reads are not written to the ledger, because the follow-ups page polls. |
| POST | `/mail/send` | Requires declared `mail.send`, effects `sends`, and `card_id` of an approved card that covers that permission. |
| POST | `/mail/undo` | Demo mailbox only. Gmail and Outlook refuse. |

The follow-ups page can also list and approve its own cards (`GET /inbox`, `POST /inbox/{id}/approve|edit|reject|snooze`). Those calls use the same guardrails as the core inbox. The sandbox does not have the admin token.

The admin UI uses `GET /api/connections` and `POST /api/connections/{provider}/setup|connect|disconnect`, where `{provider}` is `gmail`, `graph`, or `fake`.

## Doctor

For each installed mailbox, `da doctor` adds `mail:gmail`, `mail:graph`, or `mail:fake`. The detail says whether a token is present, whether refresh worked, whether send scope is granted, and any throttle window. A connected Gmail or Outlook mailbox whose refresh fails is an error and makes `da doctor` exit `1`. The demo mailbox does not.

Ledger rows for this feature use kinds `mail.connect`, `mail.disconnect`, `mail.sent`, and `mail.undo`.

## Files

| Path | Role |
| --- | --- |
| `mail/public.json` | Gmail and Outlook public client ids, and the optional Outlook calendar flag. Mode `0600`. Not a token. |
| `mail/fake.json` | Demo threads and whether the demo mailbox is connected. |

## Gmail

1. In Google Cloud Console, create a project and enable the Gmail API.
2. Configure the OAuth consent screen. Scopes used here: `gmail.readonly`, `gmail.compose`, and later `gmail.send`.
3. Create an OAuth client of type **Desktop app**. Copy the client id. Do not put a client secret in the repo or in `plugin.yaml`. This flow is PKCE and does not use a secret.
4. Desktop clients accept a loopback redirect (`http://127.0.0.1` with an ephemeral port). If the console asks for a redirect URI, add `http://127.0.0.1`.
5. On Connections, save the client id and press Connect. Sign in inside Google's window. DAP never sees the password.
6. The first time you approve a follow-up, Google is asked for `gmail.send` as well. Until that consent succeeds, DAP will not call send. Google's `gmail.compose` scope is also allowed to send at the API level. DAP still waits for the incremental `gmail.send` grant.

Google restricted-scope verification is not part of this setup.

## Outlook (personal Microsoft accounts)

1. In Microsoft Entra, register an application.
2. Supported account types: **Personal Microsoft accounts only**.
3. Add a **Mobile and desktop applications** platform. Set the redirect URI to `http://localhost`. Turn on **Allow public client flows**.
4. Do not create a client secret. This is a public client with PKCE.
5. Permissions at sign-in: `User.Read`, `Mail.ReadWrite`, `offline_access`. `Mail.Send` is requested only when the first send is approved. `Calendars.Read` stays off unless you check it on Connections.
6. Paste the application (client) id into Connections and press Connect. Sign-in, including MFA, happens in Microsoft's window. DAP does not read Outlook's stored credentials or tokens.
7. On Windows, macOS, and Linux the sign-in uses the system browser and a localhost redirect, which is the MSAL public-client pattern. A native Web Account Manager broker is not linked in this build, so Windows uses that same browser flow.

Polling uses Graph delta queries. Threads are grouped by `conversationId`. A 429 or 503 honors `Retry-After`. A wait of two seconds or less is retried once. A longer wait is recorded and shown by doctor instead of blocking the laptop.
