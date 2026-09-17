# herdr API: CLI ↔ socket method mapping

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](README.md). Wire format: [protocol.md](protocol.md).

The `herdr` CLI is a thin client over the same Unix-domain-socket protocol this reference
documents. Most subcommands parse their flags, issue exactly one socket request, and print
the raw `result` object as JSON on stdout. This file is the complete cross-reference: a row
for every one of the 91 socket methods (mapped to its CLI subcommand or marked `API-only`),
a section for CLI subcommands that have no single backing method, and the CLI's I/O
conventions. Sources: the 90-page `--help` sweep, live read-only probes, and mutating
scratch-server probes (including `.err` usage strings). Where a CLI subcommand does not
exist, invoking the reserved verb prints the group's usage list (e.g. `herdr tab move`) or,
for an unknown top-level group, `unknown command: layout`.

## Mapping table (all 91 socket methods)

Methods are listed in the schema's method-name order. "API-only" means no CLI subcommand
reaches the method — a client must speak the socket directly (see `raw/` probes). CLI flag
lists below are abbreviated; consult each subcommand's `--help` for the exhaustive set.

| socket method | CLI equivalent | notes |
| --- | --- | --- |
| `agent.explain` | `herdr agent explain [TARGET] [--file PATH] [--agent LABEL] [--json] [--format text\|json] [-v]` | TARGET optional; can explain detection for a file via `--file` instead of a live pane |
| `agent.focus` | `herdr agent focus <target>` | target is a pane id or agent name |
| `agent.get` | `herdr agent get <target>` | |
| `agent.list` | `herdr agent list` | |
| `agent.prompt` | `herdr agent prompt <target> <text> [--wait] [--until STATE]… [--timeout MS]` | `--wait`/`--until`/`--timeout` are CLI-side settle-state polling layered on the method response; STATE ∈ idle, working, blocked, done, unknown |
| `agent.read` | `herdr agent read <target> [--source SRC] [--lines N] [--format text\|ansi] [--ansi]` | SRC ∈ visible, recent, recent-unwrapped, detection |
| `agent.rename` | `herdr agent rename <target> <name>\|--clear` | positional NAME or `--clear` |
| `agent.send_keys` | `herdr agent send-keys <target> <key>…` | `esc` is the canonical Escape name (`escape` also accepted) |
| `agent.start` | `herdr agent start <name> --kind KIND --pane ID [--timeout MS] [-- AGENT_ARG…]` | KIND is a supported agent kind; pane must be at an interactive shell prompt |
| `agent.view.clear` | API-only (no CLI subcommand) | raw socket only (`raw/agent-view-clear.json`) |
| `agent.view.set` | API-only (no CLI subcommand) | raw socket only (`raw/agent-view-set.json`) |
| `agent.wait` | `herdr agent wait <target> [--until STATE]… [--timeout MS]` | without `--until`, matches idle/done/blocked |
| `client.window_title.clear` | API-only (no CLI subcommand) | |
| `client.window_title.set` | API-only (no CLI subcommand) | raw socket only (`raw/client-window-title-set.json`) |
| `events.subscribe` | API-only (no CLI subcommand) | connection stays open and receives pushed `{"event":…,"data":…}` lines |
| `events.wait` | API-only (no CLI subcommand) | raw socket only (`raw/events-wait.json`) |
| `integration.install` | `herdr integration install <target>` | target ∈ pi, omp, claude, codex, copilot, devin, droid, kimi, opencode, kilo, hermes, qodercli, qwen, cursor, mastracode, antigravity-cli, grok |
| `integration.uninstall` | `herdr integration uninstall <target>` | same target set |
| `layout.apply` | API-only (no CLI subcommand) | `herdr layout …` → `unknown command: layout` |
| `layout.export` | API-only (no CLI subcommand) | raw socket only (`raw/layout-export.json`) |
| `layout.set_split_ratio` | API-only (no CLI subcommand) | raw socket only (`raw/layout-set-split-ratio.json`) |
| `notification.show` | `herdr notification show <title> [--body TEXT] [--position POS] [--sound SOUND]` | POS ∈ top-left, top-right, bottom-left, bottom-right; SOUND ∈ none, done, request |
| `pane.clear_agent_authority` | API-only (no CLI subcommand) | distinct from `pane release-agent`; raw socket only (`raw/pane-clear-agent-authority.json`) |
| `pane.close` | `herdr pane close <pane_id>` | |
| `pane.current` | `herdr pane current [--pane ID \| --current]` | |
| `pane.edges` | `herdr pane edges [--pane ID \| --current]` | |
| `pane.focus` | API-only (no CLI subcommand) | takes a PaneTarget (focus a specific pane); CLI `pane focus` is directional and maps to `pane.focus_direction` |
| `pane.focus_direction` | `herdr pane focus --direction DIR [--pane ID \| --current]` | DIR ∈ left, right, up, down |
| `pane.get` | `herdr pane get <pane_id>` | positional id (not `--pane`) |
| `pane.graphics.clear` | API-only (no CLI subcommand) | |
| `pane.graphics.info` | API-only (no CLI subcommand) | raw socket only (`raw/pane-graphics-info.json`) |
| `pane.graphics.set` | API-only (no CLI subcommand) | |
| `pane.input.set` | `herdr pane input --right-click TARGET [PANE_ID] [--pane ID \| --current]` | CLI subcommand is `pane input`; TARGET ∈ herdr, pane (right-click routing) |
| `pane.layout` | `herdr pane layout [--pane ID \| --current]` | |
| `pane.list` | `herdr pane list [--workspace ID]` | |
| `pane.move` | `herdr pane move <pane_id> [--tab ID\|--new-tab\|--workspace ID\|--new-workspace] [--split right\|down] [--target-pane ID] [--ratio F] [--label] [--tab-label] [--focus\|--no-focus]` | positional pane id, targeting via flags |
| `pane.neighbor` | `herdr pane neighbor --direction DIR [--pane ID \| --current]` | DIR ∈ left, right, up, down |
| `pane.process_info` | `herdr pane process-info [--pane ID \| --current]` | |
| `pane.read` | `herdr pane read <pane_id> [--source SRC] [--lines N] [--format text\|ansi] [--ansi] [--raw]` | positional id; SRC as for agent.read |
| `pane.release_agent` | `herdr pane release-agent <pane_id> --source ID --agent LABEL [--seq N]` | |
| `pane.rename` | `herdr pane rename <pane_id> [LABEL…] \| --clear` | |
| `pane.report_agent` | `herdr pane report-agent <pane_id> --source ID --agent LABEL --state STATUS [--message TEXT] [--seq N] [--agent-session-id ID] [--agent-session-path PATH]` | STATUS ∈ idle, working, blocked, unknown |
| `pane.report_agent_session` | `herdr pane report-agent-session <pane_id> --source ID --agent LABEL [--seq N] [--agent-session-id ID] [--agent-session-path PATH] [--session-start-source SOURCE]` | |
| `pane.report_metadata` | `herdr pane report-metadata <pane_id> --source ID [--agent] [--applies-to-source] [--title\|--clear-title] [--display-agent\|--clear-display-agent] [--state-label STATUS=TEXT\|--clear-state-labels] [--token NAME=VALUE\|--clear-token NAME] [--seq N] [--ttl-ms N]` | display-only overlay metadata |
| `pane.resize` | `herdr pane resize --direction DIR [--amount F] [--pane ID \| --current]` | DIR ∈ left, right, up, down |
| `pane.send_input` | API-only (no CLI subcommand) | raw socket only (`raw/pane-send-input.json`); `pane run` composes `send-text` rather than calling this |
| `pane.send_keys` | `herdr pane send-keys <pane_id> <key>…` | `esc` canonical Escape |
| `pane.send_text` | `herdr pane send-text <pane_id> <text>` | literal text, no trailing Enter |
| `pane.split` | `herdr pane split [PANE_ID] [--pane ID\|--current] [--direction right\|down] [--ratio F] [--cwd] [--env KEY=VALUE] [--right-click herdr\|pane] [--focus\|--no-focus]` | |
| `pane.swap` | `herdr pane swap [--direction DIR \| --source-pane ID --target-pane ID] [--pane ID\|--current]` | directional or explicit source/target pair |
| `pane.wait_for_output` | `herdr pane wait-output <pane_id> <--match TEXT\|--regex PATTERN> [--source SRC] [--lines N] [--timeout MS] [--raw]` | SRC ∈ visible, recent, recent-unwrapped; regex is Rust syntax |
| `pane.zoom` | `herdr pane zoom [PANE_ID] [--pane ID\|--current] [--toggle\|--on\|--off]` | |
| `ping` | API-only (no CLI subcommand) | `herdr status` performs the ping/pong handshake internally but exposes no `ping` verb |
| `plugin.action.invoke` | API-only (no CLI subcommand) | |
| `plugin.action.list` | API-only (no CLI subcommand) | raw socket only (`raw/plugin-action-list.json`) |
| `plugin.disable` | API-only (no CLI subcommand) | |
| `plugin.enable` | API-only (no CLI subcommand) | |
| `plugin.link` | API-only (no CLI subcommand) | |
| `plugin.list` | API-only (no CLI subcommand) | raw socket only (`raw/plugin-list.json`) |
| `plugin.log.list` | API-only (no CLI subcommand) | |
| `plugin.pane.close` | API-only (no CLI subcommand) | |
| `plugin.pane.focus` | API-only (no CLI subcommand) | |
| `plugin.pane.open` | API-only (no CLI subcommand) | |
| `plugin.unlink` | API-only (no CLI subcommand) | |
| `popup.close` | API-only (no CLI subcommand) | raw socket only (`raw/popup-close.json`) |
| `server.agent_manifests` | `herdr server agent-manifests [--json]` | thin 1:1 wrapper |
| `server.live_handoff` | API-only (no CLI subcommand) | invoked indirectly by `herdr update --handoff` and `--remote`/`--handoff` attach; not a standalone verb |
| `server.reload_agent_manifests` | `herdr server reload-agent-manifests` | reloads local manifest overrides only |
| `server.reload_config` | `herdr server reload-config` | |
| `server.stop` | `herdr server stop` | |
| `session.snapshot` | `herdr api snapshot` | CLI request id is `cli:api:snapshot` |
| `tab.close` | `herdr tab close <tab_id>` | |
| `tab.create` | `herdr tab create [--workspace ID] [--cwd] [--label] [--env KEY=VALUE] [--focus\|--no-focus]` | |
| `tab.focus` | `herdr tab focus <tab_id>` | |
| `tab.get` | `herdr tab get <tab_id>` | |
| `tab.list` | `herdr tab list [--workspace ID]` | |
| `tab.move` | API-only (no CLI subcommand) | `herdr tab move` prints the `tab` usage list, exit 2 |
| `tab.rename` | `herdr tab rename <tab_id> <label>…` | LABEL is variadic (joined) |
| `workspace.close` | `herdr workspace close <workspace_id>` | |
| `workspace.create` | `herdr workspace create [--cwd] [--label] [--env KEY=VALUE] [--focus\|--no-focus]` | |
| `workspace.focus` | `herdr workspace focus <workspace_id>` | |
| `workspace.get` | `herdr workspace get <workspace_id>` | |
| `workspace.list` | `herdr workspace list` | |
| `workspace.move` | API-only (no CLI subcommand) | `herdr workspace move` prints the `workspace` usage list, exit 2 |
| `workspace.move_block` | API-only (no CLI subcommand) | raw socket only (`raw/workspace-move-block.json`) |
| `workspace.rename` | `herdr workspace rename <workspace_id> <label>…` | LABEL variadic |
| `workspace.report_metadata` | `herdr workspace report-metadata <workspace_id> --source ID [--token NAME=VALUE] [--clear-token NAME] [--seq N] [--ttl-ms N]` | |
| `worktree.create` | `herdr worktree create [--workspace ID] [--cwd] [--branch] [--base] [--path] [--label] [--focus\|--no-focus]` | |
| `worktree.list` | `herdr worktree list [--workspace ID] [--cwd PATH]` | |
| `worktree.open` | `herdr worktree open [--workspace ID] [--cwd] [--path] [--branch] [--label] [--focus\|--no-focus]` | |
| `worktree.remove` | `herdr worktree remove [--workspace ID] [--force]` | note: `remove` accepts only `--workspace`/`--force`; passing `--path` fails with `unknown option: --path` (exit 2) even though `create`/`open` accept `--path` |

### API-only method count

32 of the 91 methods have no CLI subcommand: all 3 `layout.*`, all 11 `plugin.*`, all 3
`pane.graphics.*`, both `agent.view.*`, both `client.window_title.*`, both `events.*`,
`pane.focus`, `pane.send_input`, `pane.clear_agent_authority`, `popup.close`, `ping`,
`server.live_handoff`, `tab.move`, `workspace.move`, and `workspace.move_block`.

## CLI-only commands

These CLI subcommands do not correspond to a single socket method: they operate on local
files/state, orchestrate several socket calls, run an interactive attach loop, or perform
work (network fetch, binary download) outside the socket protocol entirely.

| CLI command | what it does | relation to the socket API |
| --- | --- | --- |
| `herdr status [server\|client] [--json]` | Prints local client and running-server status. | Talks to the server via the ping/pong handshake and version negotiation; not a single method, and its default output is a plain-text report (`--json` for machine form). |
| `herdr update [--handoff]` | Downloads and installs the latest binary. | Local/network operation, not a socket method. `--handoff` opts into `server.live_handoff` after install. |
| `herdr channel show` / `herdr channel set <stable\|preview>` | Reads/writes the configured update channel. | Local config only; no socket call. |
| `herdr config check` | Validates `config.toml` and prints diagnostics. | Local config file operation; distinct from `server reload-config`, which applies config in the running server. |
| `herdr config reset-keys` | Backs up `config.toml` and removes custom keybindings. | Local config file operation. |
| `herdr completion <bash\|elvish\|fish\|powershell\|zsh>` | Emits a shell-completion script. | Purely local; no server. |
| `herdr session list [--json]` | Lists named persistent sessions. | Enumerates session directories/sockets locally; not a socket method. |
| `herdr session attach <name>` | Attaches the terminal to a session. | Interactive attach loop, not a request/response method. |
| `herdr session stop <name> [--json]` | Stops a named session. | Targets that session's server (may start/stop the process); not `server.stop` against the ambient socket. |
| `herdr session delete <name> [--json]` | Deletes a stopped session. | Local session-directory removal. |
| `herdr api schema [--json] [--output PATH]` | Prints or writes the schema bundled into the binary. | Returns the compiled-in schema; no live server needed (contrast `api snapshot` → `session.snapshot`). |
| `herdr server agent-manifests [--json]` | Shows active agent-detection manifests. | Thin 1:1 wrapper over `server.agent_manifests` (listed here only for the `server` group's completeness). |
| `herdr server update-agent-manifests [--json]` | Fetches manifests from the network, then reloads them. | The network fetch has no socket method; the reload step reuses the manifest-reload path. Genuinely composite/CLI-only. |
| `herdr server reload-agent-manifests` | Reloads local manifest overrides. | Thin 1:1 wrapper over `server.reload_agent_manifests`. |
| `herdr agent attach <target> [--takeover]` | Attaches the terminal directly to an agent pane. | Interactive attach loop, not a request/response method. |
| `herdr pane run <pane_id> <command>…` | Sends text plus Enter in one call. | CLI convenience composite over `pane send-text` (documented in `send-text`'s help: "herdr pane run … sends text and Enter in one call"); no dedicated `pane.run` method. |
| `herdr integration status [--outdated-only]` | Shows install status of built-in integrations. | No `integration.status` method exists; status is computed CLI-side (only `integration.install`/`.uninstall` are socket methods). |

Top-level launch/attach invocations (`herdr`, `herdr --session <name>`, `herdr --remote
<target>`, `herdr --no-session`, `herdr --default-config`, `herdr --skill`,
`herdr --version`) are client entry points, not socket methods.

## CLI conventions

Observed from the help sweep and probe captures (`probes/`, `probes/scratch/*.err`):

- **JSON result on stdout.** A method-backed subcommand prints the socket `result` object
  verbatim as one JSON line on stdout, with the envelope `id` set to
  `cli:<group>:<cmd>` — e.g. `cli:pane:get`, `cli:workspace:create`, `cli:agent:list`,
  `cli:api:snapshot`. Some result objects carry a `type` discriminator (`{"type":"ok"}`,
  `{"type":"tab_info"}`, `{"type":"workspace_info"}`, `{"type":"pane_zoom"}`), others are a
  bare named object (`{"pane":{…}}`, `{"agents":[…]}`).
- **Server errors → JSON on stderr, exit 1.** A socket error is printed as the same
  `{"error":{"code":…,"message":…},"id":"cli:<group>:<cmd>"}` envelope on stderr with exit
  status 1. Example: `{"error":{"code":"pane_not_found","message":"pane w1:p99 not found"},"id":"cli:pane:read"}`;
  `{"error":{"code":"workspace_not_found","message":"workspace w99 not found"},"id":"cli:workspace:get"}`;
  `{"error":{"code":"agent_pane_not_found",…},"id":"cli:agent:start"}`. Error `code` is
  snake_case; `message` is human text.
- **Syntax errors → usage on stderr, exit 2.** Bad flags/args or a nonexistent subcommand
  print a usage string (not JSON) to stderr with exit status 2 — e.g. `unknown option:
  --path` for `worktree remove --path`, or the group's usage list for `tab move` /
  `workspace move`. An unknown top-level group prints `unknown command: layout` plus
  `run 'herdr --help' for usage`.
- **Non-JSON subcommands.** `status` (default), `channel show`, `config check`, and
  `completion` emit human-readable text, not the JSON envelope. Most add `--json` for a
  machine-readable form.
- **Mixed positional-vs-flag targeting.** Read-by-id commands take the id positionally:
  `pane get <pane_id>`, `pane read <pane_id>`, `tab get <tab_id>`, `workspace get
  <workspace_id>`, `pane close/rename/move/send-text/send-keys/wait-output <pane_id>`.
  Directional and split-relative commands target with flags instead: `pane resize
  --direction … [--pane <id>]`, `pane neighbor/focus/edges/layout/process-info/current`,
  which accept `--pane <id>` or `--current` to name the subject pane. Several commands
  (`pane current/split/zoom/input`) accept both a positional `[PANE_ID]` and `--pane`/
  `--current`. `pane swap` targets either directionally (`--direction`) or by an explicit
  `--source-pane`/`--target-pane` pair.
- **Focus flags.** Creation commands (`workspace/tab/worktree create`, `pane split`,
  `pane move`) accept mutually exclusive `--focus` / `--no-focus` to control post-create
  focus.
- **Variadic labels.** `tab rename`, `workspace rename`, and `pane rename` take the new
  label as trailing variadic args (joined), and rename commands accept `--clear` to drop
  the label.
- **Key names.** `send-keys` uses `esc` as the canonical Escape name; `escape` is also
  accepted.
