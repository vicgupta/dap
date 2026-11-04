# Chat

Chat is a panel in the local DAP window. It talks to the coding CLI you already installed: Claude Code or Codex. DAP does not host a model, does not read CLI credentials, and does not send anything on its own.

Open it from **Chat** in the sidebar, or press **Ctrl+Shift+.** (Command+Shift+. on a Mac) from any screen. That combo is free in Firefox, Edge, Chrome, Brave, and Safari. Ctrl+Shift+C and Command+Shift+C are the inspector in Firefox and Edge, so DAP does not use them. Enter sends. Shift+Enter adds a line. Escape stops the turn that is running.

## What a turn does

Each turn starts a new process and waits for it to exit. The working directory is `./.dap/chat/work/<session>/` (or `$DAP_HOME/chat/work/<session>/`). That subdirectory keeps the process away from `run.json`, `secrets.json`, and the ledger. Temporary files for the process are under `./.dap/tmp`.

The process is read-only:

| CLI | Invocation |
| --- | --- |
| Claude Code | `claude -p --output-format stream-json --verbose --include-partial-messages --permission-mode plan --permission-prompts none --tools Read,Glob,Grep --disallowedTools Edit,Write,NotebookEdit,MultiEdit,Bash,WebFetch,WebSearch --resume <session-id>` |
| Codex | `codex exec --json --sandbox read-only --skip-git-repo-check --ignore-user-config --ignore-rules -C <workdir> resume <thread-id> <prompt>` |

The first Claude turn uses `--session-id` with a UUID instead of `--resume`. Later turns pass the id Claude reported. Codex resume uses the `thread_id` from `thread.started`. Flags were checked against the [Claude Code CLI reference](https://code.claude.com/docs/en/cli-reference) and [headless](https://code.claude.com/docs/en/headless) pages, and against [Codex non-interactive mode](https://developers.openai.com/codex/cli) (`codex exec --json`, `codex exec resume`).

`--bare` is not used for Claude Code. Bare mode ignores the signed-in account. DAP also does not put `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` into the child environment. The CLI uses the login you created with `claude auth login` or `codex login`.

Replies stream into the thread. Tool use shows as one line (`Read · AGENTS.md`). A tool that would write, delete, or send is marked **Not run** and becomes a card.

## Approval

The model is told to end a change with a `dap-proposal` fence. DAP turns that into a decision-inbox card. The same card is drawn in the thread. Approving it uses the existing inbox path and the ledger (`card.proposed`, `card.approved`, then `config.applied` / `card.acted`).

| Proposal | After you approve |
| --- | --- |
| `write` | DAP writes the file inside that session's workspace. Mode `0600`. |
| `delete` | DAP deletes that file, if it is a file inside the workspace. `..` and absolute paths are refused. |
| `send` | The card is marked approved. **Nothing is sent.** The summary says so. |
| a blocked tool, with no full file body | DAP records the attempt under `proposals/` in the workspace. It does not run the command. |

Chat is not a plugin. Cards are owned by `chat` so they show in the decision inbox and follow guardrails (`chat.write`, `chat.delete`, `chat.send`, all ask-first). A send or a delete still waits even if a rule were loosened, because those effects always ask first until the card is approved.

## Provider picker

The picker lists Claude Code and Codex only. DAP checks `claude` / `codex` on `PATH`, runs `claude --version` or `codex --version`, and asks the CLI whether it is signed in:

- `claude auth status` (exit 0 means signed in)
- `codex login status` (exit 0 means signed in)

The reply is a yes or no. DAP does not open Claude Code or Codex credential files. Those CLIs keep their own logins, and DAP does not move them. If the CLI is missing or signed out, the panel shows the command to run in a terminal (`claude auth login` or `codex login`) and disables Send.

The choice is stored in `config.json` as `"chat": {"provider": "claude-code"}` or `"codex"`. `PUT /api/settings` with only `chat` leaves `theme` and `layout` alone. A body with only `theme` or only `layout` leaves `chat` alone.

## History

Sessions live in `./.dap/chat/sessions/<id>.json` (mode `0600`). **New chat** starts an empty one. Opening a past row resumes that CLI session. The ledger records `chat.session` when a chat starts, `chat.turn` when a turn finishes, and `chat.cancel` when you stop one. The row is a short summary (provider and the first line of what you asked). The transcript stays in the session file, not the ledger.

Quit, `da quit`, and Ctrl+C kill the running CLI through the same shutdown path that stops plugins. A turn also stops itself after 10 minutes.

## API

Admin token, loopback only, same as the other `/api` routes.

| Method | Path |
| --- | --- |
| GET | `/api/chat/providers` |
| GET | `/api/chat/sessions` |
| POST | `/api/chat/sessions` with `provider` |
| GET | `/api/chat/sessions/{id}` |
| POST | `/api/chat/sessions/{id}/turn` with `text`. The body is newline-delimited JSON: `text`, `tool`, `proposal`, `error`, `done`. |
| POST | `/api/chat/sessions/{id}/cancel` |

## Adding OpenCode later

OpenCode is detected by `da doctor` and is not in the picker. The seam is `chat.Provider` in `internal/chat`:

- `ID`, `Label`, `Binary`
- `Args` for one read-only turn, including resume
- `ParseLine` to turn that CLI's JSON into text, tool, session, proposal, and done events

Add the implementation next to `claude.go` and `codex.go`, return it from `Providers`, and allow the id in `NormalizeProvider`. Keep the working directory on the session workspace, keep writes and sends on the card path, and do not teach DAP the OpenCode credential. Until that adapter exists, `PUT /api/settings` rejects `opencode`.
