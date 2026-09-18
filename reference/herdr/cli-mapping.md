# herdr API: CLI ↔ socket method mapping

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](README.md). Wire format: [protocol.md](protocol.md).

The `herdr` CLI is a thin client over the same Unix-domain-socket protocol this reference
documents. Most subcommands parse their flags, issue exactly one socket request, and print
the raw `result` object as JSON on stdout. This file is the complete cross-reference: a row
for every one of the 103 socket methods (mapped to its CLI subcommand or marked `API-only`),
a section for CLI subcommands that have no single backing method, and the CLI's I/O
conventions. Sources: the 90-page `--help` sweep, live read-only probes, and mutating
scratch-server probes (including `.err` usage strings). Where a CLI subcommand does not
exist, invoking the reserved verb prints the group's usage list (e.g. `herdr tab move`) or,
for an unknown top-level group, `unknown command: layout`.

## Mapping table (all 103 socket methods)

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
| `client_shell.surface.set` | API-only (no CLI subcommand) | toggles whether the requesting client shell endpoint receives/controls pane presentation; returns `connection_local_only` outside a client-shell endpoint |
| `command.invoke` | API-only (no CLI subcommand) | invokes an opaque `command_id` from the client-shell command projection; used by `herdr command`-less integrations, not the CLI |
| `events.subscribe` | API-only (no CLI subcommand) | connection stays open and receives pushed `{"event":…,"data":…}` lines |
| `events.wait` | API-only (no CLI subcommand) | raw socket only (`raw/events-wait.json`) |
| `integration.install` | `herdr integration install <target>` | target ∈ pi, omp, claude, codex, copilot, devin, droid, kimi, opencode, kilo, hermes, qodercli, qwen, cursor, mastracode, antigravity-cli, grok, letta; note: `letta` is accepted by the CLI's usage text but is absent from the schema's `IntegrationTarget` enum and from a live `integration.list` response (0.9.1 drift) |
| `integration.list` | API-only (no CLI subcommand) | returns one `IntegrationInfo`/`IntegrationState` entry per built-in integration (17 in 0.9.1, `letta` not among them); `herdr integration status` (CLI-only, see below) is a separate report |
| `integration.uninstall` | `herdr integration uninstall <target>` | same target set |
| `layout.apply` | API-only (no CLI subcommand) | `herdr layout …` → `unknown command: layout` |
| `layout.export` | API-only (no CLI subcommand) | raw socket only (`raw/layout-export.json`) |
| `layout.set_split_ratio` | API-only (no CLI subcommand) | raw socket only (`raw/layout-set-split-ratio.json`) |
| `notification.show` | `herdr notification show <title> [--body TEXT] [--position POS] [--sound SOUND]` | POS ∈ top-left, top-right, bottom-left, bottom-right; SOUND ∈ none, done, request |
| `pane.clear_agent_authority` | API-only (no CLI subcommand) | distinct from `pane release-agent`; raw socket only (`raw/pane-clear-agent-authority.json`) |
| `pane.close` | `herdr pane close <pane_id>` | |
| `pane.copy_motion` | API-only (no CLI subcommand) | moves a copy-mode cursor by a `PaneCopyMotion` (word/line/paragraph); no CLI copy-mode exists |
| `pane.copy_search` | API-only (no CLI subcommand) | searches scrollback from a copy-mode cursor in a `PaneCopySearchDirection`; no CLI copy-mode exists |
| `pane.current` | `herdr pane current [--pane ID \| --current]` | |
| `pane.edges` | `herdr pane edges [--pane ID \| --current]` | |
| `pane.edit_scrollback` | API-only (no CLI subcommand) | takes a bare `PaneTarget`; opens the pane's scrollback for external editing |
| `pane.focus` | API-only (no CLI subcommand) | takes a PaneTarget (focus a specific pane); CLI `pane focus` is directional and maps to `pane.focus_direction` |
| `pane.focus_direction` | `herdr pane focus --direction DIR [--pane ID \| --current]` | DIR ∈ left, right, up, down |
| `pane.get` | `herdr pane get <pane_id>` | positional id (not `--pane`) |
| `pane.graphics.clear` | API-only (no CLI subcommand) | |
| `pane.graphics.info` | API-only (no CLI subcommand) | raw socket only (`raw/pane-graphics-info.json`) |
| `pane.graphics.set` | API-only (no CLI subcommand) | |
| `pane.input.set` | `herdr pane input --right-click TARGET [PANE_ID] [--pane ID \| --current]` | CLI subcommand is `pane input`; TARGET ∈ herdr, pane (right-click routing) |
| `pane.layout` | `herdr pane layout [--pane ID \| --current]` | |
| `pane.link.activate` | API-only (no CLI subcommand) | activates a detected link region at a viewport row/col; returns `{"handled":false}` when nothing is there |
| `pane.link.resolve` | API-only (no CLI subcommand) | resolves detected `PaneLinkRegion`s at a viewport row/col without activating them |
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
| `pane.scroll` | API-only (no CLI subcommand) | sets absolute scrollback `offset_from_bottom`; no CLI scroll command exists |
| `pane.selection.read` | API-only (no CLI subcommand) | reads text between an anchor and cursor `PaneTextPoint`; no CLI equivalent |
| `pane.send_input` | API-only (no CLI subcommand) | raw socket only (`raw/pane-send-input.json`); `pane run` composes `send-text` rather than calling this |
| `pane.send_keys` | `herdr pane send-keys <pane_id> <key>…` | `esc` canonical Escape |
| `pane.send_text` | `herdr pane send-text <pane_id> <text>` | literal text, no trailing Enter |
| `pane.split` | `herdr pane split [PANE_ID] [--pane ID\|--current] [--direction right\|down] [--ratio F] [--cwd] [--env KEY=VALUE] [--right-click herdr\|pane] [--focus\|--no-focus]` | |
| `pane.swap` | `herdr pane swap [--direction DIR \| --source-pane ID --target-pane ID] [--pane ID\|--current]` | directional or explicit source/target pair |
| `pane.wait_for_output` | `herdr pane wait-output <pane_id> <--match TEXT\|--regex PATTERN> [--source SRC] [--lines N] [--timeout MS] [--raw]` | SRC ∈ visible, recent, recent-unwrapped; regex is Rust syntax |
| `pane.zoom` | `herdr pane zoom [PANE_ID] [--pane ID\|--current] [--toggle\|--on\|--off]` | |
| `ping` | API-only (no CLI subcommand) | `herdr status` performs the ping/pong handshake internally but exposes no `ping` verb |
| `plugin.action.invoke` | `herdr plugin action invoke <ACTION_ID> [--plugin <ID>]` | new CLI group in 0.9.1 |
| `plugin.action.list` | `herdr plugin action list [--plugin <ID>]` |  |
| `plugin.disable` | `herdr plugin disable <PLUGIN_ID>` | new CLI group in 0.9.1 |
| `plugin.enable` | `herdr plugin enable <PLUGIN_ID>` | new CLI group in 0.9.1 |
| `plugin.link` | `herdr plugin link <PATH> [--enabled \| --disabled]` | new CLI group in 0.9.1 |
| `plugin.list` | `herdr plugin list [--plugin <ID>] [--json]` |  |
| `plugin.log.list` | `herdr plugin log list [--plugin <ID>] [--limit <N>]` | new CLI group in 0.9.1 |
| `plugin.pane.close` | `herdr plugin pane close <PANE_ID>` | new CLI group in 0.9.1 |
| `plugin.pane.focus` | `herdr plugin pane focus <PANE_ID>` | new CLI group in 0.9.1 |
| `plugin.pane.open` | `herdr plugin pane open --plugin <ID> --entrypoint <ID> [--placement <overlay\|split\|tab\|zoomed>] [--workspace <ID>] [--target-pane <PANE>] [--direction <right\|down>] [--cwd <PATH>] [--env <KEY=VALUE>]... [--focus \| --no-focus]` | CLI `--placement` omits `popup`, which remains valid on the wire |
| `plugin.unlink` | `herdr plugin unlink <PLUGIN_ID>` | new CLI group in 0.9.1 |
| `popup.close` | API-only (no CLI subcommand) | raw socket only (`raw/popup-close.json`) |
| `product_announcement.dismiss` | API-only (no CLI subcommand) | dismisses an in-app product announcement by `id`/`version`; errors `stale_announcement` once it is no longer current |
| `release_notes.dismiss` | API-only (no CLI subcommand) | dismisses the in-app release-notes prompt for a given `version` |
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

33 of the 103 methods have no CLI subcommand: all 3 `layout.*`, all 3
`pane.graphics.*`, both `pane.link.*`, both `agent.view.*`, both `client.window_title.*`, both `events.*`,
`pane.focus`, `pane.send_input`, `pane.clear_agent_authority`, `pane.copy_motion`,
`pane.copy_search`, `pane.edit_scrollback`, `pane.scroll`, `pane.selection.read`,
`popup.close`, `ping`, `client_shell.surface.set`, `command.invoke`, `integration.list`,
`product_announcement.dismiss`, `release_notes.dismiss`, `server.live_handoff`,
`tab.move`, `workspace.move`, and `workspace.move_block`.

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
| `herdr integration status [--outdated-only]` | Shows install status of built-in integrations. | No `integration.status` method exists; status is computed CLI-side, distinct from the `integration.list` socket method. |
| `herdr machine list [--json]` / `add --label LABEL [--remote-session NAME] <SSH_TARGET>` / `rename --label LABEL <PROFILE_ID>` / `remove\|enable\|disable <PROFILE_ID>` | Manages saved SSH machine profiles used by `--machine`/`--remote`. | Local config file operation; `machine add` also SSHes out to prepare the remote Herdr server. No socket method. |

Top-level launch/attach invocations (`herdr`, `herdr --session <name>`, `herdr --machine
<label-or-id>`, `herdr --remote <target>`, `herdr --remote-keybindings <local\|server>`,
`herdr --default-config`, `herdr --skill`, `herdr --version`) are client entry points, not
socket methods.

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
