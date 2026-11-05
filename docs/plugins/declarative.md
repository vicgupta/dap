# Declarative plugins

A declarative plugin is one `plugin.yaml`. DAP runs it inside the `da` process. There is no entrypoint, no subprocess, and no user code. Code plugins (`dap.plugin.v1` with an entrypoint) are unchanged.

The schema is [schemas/declarative.schema.json](../../schemas/declarative.schema.json). `da plugin validate <dir>` checks it, then the rules below, then a dry run. Install still waits for a change card. The card lists the derived permissions and a sample run against the demo mailbox.

```yaml
apiVersion: dap/v1
kind: declarative
name: inbox-triage
version: 0.1.0
min_dap_version: 0.1.0
description: Labels and summarizes new threads into Inbox cards. Nothing is sent.
settings:
  - key: top
    type: number
    label: Threads to show
    default: 5
  - key: urgent_words
    type: list
    label: Urgent words
    default: [urgent, asap, invoice, payment, deadline]
triggers:
  - schedule: every 1h
  - event: email.received
inputs:
  - service: email
    window: 24h
rules:
  - id: urgent
    when:
      any:
        - subject_contains: { setting: urgent_words }
        - subject_contains: action required
output:
  limit_setting: top
  unmatched_label: Review
  report:
    title: Inbox triage
    line: subject
  actions:
    - type: label
      rule: urgent
      label: Needs a reply
```

Unknown keys are rejected. Do not set `entrypoint`, `runtime`, `permissions`, or `llm`.

## When to write one

Write declarative YAML when the work is all of:

- a schedule, or the `email.received` event
- the `email` service
- comparisons from the list below (`all` or `any`, one level)
- a report, or the actions `label`, `draft`, `reminder`, `send`, or `checklist`

Write a TypeScript plugin when the work needs `web.fetch`, a loop, a provider-specific call, or anything outside that list. Competitor research stays a code plugin for that reason.

## Triggers

Set at least one. A trigger object is either `schedule` or `event`, not both. Only one schedule is allowed.

`schedule` uses the same language as a code plugin: `every 1h`, `cron 0 8 * * *`, `at 08:00`, `weekdays at 08:00`, `weekly at 08:00`, or the structured form (`every`, `cron`, `at`, `recurrence`, `day`).

`event` is only `email.received`. On each run, threads older than the previous successful run are dropped. Combine it with a window, not with `no_reply_for`: a thread that has been waiting is not "new".

If the plugin has an event and no schedule, the core polls it every 15 minutes. That poll is removed on uninstall.

## Services

`email` is the only service this core provides. It reads every connected mailbox (Gmail, Outlook, or the demo mailbox). The plugin does not name a provider, so the same YAML works for Gmail and Outlook.

`window` is `30m`, `24h`, `7d`, or a Go duration of at least one minute. `window_setting` names a string or duration setting instead. Set only one of them.

`calendar` and `web.fetch` are not services. Adding a service means extending the `service` enum in the schema and teaching the runtime to fill it. Do not invent a service name.

## Rules

No loops, no scripts, and no expression language. A rule is `all` (every comparison) or `any` (one comparison).

| Comparison | Value | Matches when |
| --- | --- | --- |
| `sender` | email, list, or `{ setting: key }` | `with_email` equals one value |
| `sender_domain` | domain, list, or a setting | the email domain equals one value |
| `subject_contains` | text, list, or a setting | subject contains one value, ignoring case |
| `snippet_contains` | text, list, or a setting | snippet contains one value, ignoring case |
| `label` | text, list, or a setting | a mailbox label equals one value. The demo mailbox tags one thread `flagged`. Composio Gmail label ids are copied through. A direct Gmail read includes `labelIds` when the API returns them |
| `older_than` | `30m`, `24h`, `7d`, or a setting | age is at least that long |
| `newer_than` | same | age is within that long |
| `no_reply_for` | same | you sent the last message and the age is at least that long |
| `last_from_me` | `true`, `false`, or a bool setting | the last message is or is not from you |

A number setting used as a duration counts as hours. A string setting must be a duration. With no rules, every thread in the window matches. `unmatched_label` puts non-matching threads on the report only.

## Output

`report` updates the home tile and proposes one inbox card. `line` is `subject` (default), `sender`, `snippet`, or `label`.

`actions` propose one card per matching thread:

| `type` | After approval | Effect |
| --- | --- | --- |
| `label` | Records the label in this plugin's scratch. It does not call the provider. | none |
| `draft` | Saves a draft. `body` is `nudge`, `acknowledge`, or `snippet`. `subject` is `reply` (default) or `same`. | none |
| `reminder` | Records a reminder in scratch. The inbox card is the reminder. | none |
| `send` | Sends through the connected mailbox. | `sends` |
| `checklist` | One card per step in `steps_setting`. A step is `{ title, detail, effects }`. | the step's effects |

`nudge` is "Checking in on {subject}." `acknowledge` thanks them for the subject. `snippet` adds up to 180 characters of the snippet. `{subject}` here is filled by the core. It is not a template language. A `signature_setting` is appended to a nudge.

`limit` or `limit_setting` (a number setting) caps threads at 1 to 50. The default cap is 20.

A declarative plugin never acts in `watch` or `propose`, and it never auto-acts when a permission is `allowed`. Every card waits until a person approves it.

## Settings

Fields are `string`, `number`, `bool`, `duration`, or `list`, with a `default`. Keys are lowercase letters, digits, and underscores. The Create page builds the form from the template's `settings`, which should match these fields. A `list` of strings is one item per line. Object lists, such as checklist steps, stay in `plugin.yaml`.

## Permissions

Do not declare them. The core derives:

| YAML uses | Permission | Mode |
| --- | --- | --- |
| `email` | `mail.read` | allowed |
| `report` | `report.review` | ask first |
| `label` | `mail.label` | ask first |
| `draft` | `mail.draft` | ask first |
| `send` | `mail.send` | ask first |
| `reminder` | `reminder.create` | ask first |
| `checklist` | `workflow.run` | ask first |

The install card shows that list. A person can tighten a mode. They cannot loosen it.

## llm

The schema allows an `llm` object (`task`: `classify` or `summarize`) so the shape is reserved. This core does not call a model. Validation fails with `llm is not supported yet` when the block is present. Delete it.

## Unload

The handler, the schedule (including a 15 minute event poll), the scratch document, and `plugin-data/<name>/` belong to the core. `da plugin remove` and the Plugins page Remove stop the handler and delete that state. Quit stops the handler and leaves the plugin installed for the next start.
