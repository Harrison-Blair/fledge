# Fledge

Fledge is a Linux CLI for running project-scoped AI agents in a deterministic
[Herdr](https://herdr.dev/) session. It provides a small lifecycle harness:
discover the project, start named agents, prompt or wait for them atomically,
read their output, and dispose of the session when the work is done.

Phase one requires Go 1.26 and an independently installed Herdr 0.7.5 or newer
(socket protocol 17+). Fledge never installs Herdr or changes its integrations
or configuration.

## Install from source

For day-to-day source development, rebuild and install Fledge in one step, then
verify which executable the shell will run:

```sh
./scripts/reinstall.sh
command -v fledge
```

The scripts have separate responsibilities:

- `scripts/build.sh` runs race tests, `go vet ./...`, validates an exact
  release tag against `internal/buildinfo/VERSION`, and creates `bin/fledge`
  with reproducible VCS metadata.
- `scripts/install.sh` atomically copies an existing `bin/fledge` into the Go
  binary directory.
- `scripts/reinstall.sh` runs both scripts and is the normal workflow after
  changing source.

The install destination is `go env GOBIN` when configured. Otherwise it is the
`bin` directory under the first entry in `go env GOPATH`. Installation is
user-global and does not require `sudo`.

If `command -v fledge` prints nothing or points at an older binary, inspect the
destination and your `PATH`:

```sh
go env GOBIN GOPATH
printf '%s\n' "$PATH"
```

Add the effective Go binary directory to `PATH`, start a new shell (or refresh
the shell command cache), and run `command -v fledge` again.

To install the latest released package instead:

```sh
go install github.com/Harrison-Blair/fledge/cmd/fledge@latest
```

Fledge checks `herdr api schema --json` before operating. A running server must
use the same protocol as the installed `herdr` executable. After updating
Herdr, stop and restart an affected session if Fledge reports a mismatch. Use
`--herdr-bin | -H` to select another executable.

## Initialize and start a session

Every managed project has a `.fledge/config.json` marker. Initialize the
project once:

```sh
cd /path/to/project
fledge init
```

`fledge init [path]` writes schema version 1 and is idempotent when the marker
is already valid. It will not overwrite malformed or unsupported
configuration.

Start the project's Herdr server and open its UI in the current terminal.
Before completing either an interactive or detached start, Fledge renames the
workspace's initial tab to `orchestrator` and splits its named primary pane
beside an ordinary right shell. Both panes start in the directory where the
command was invoked:

```sh
fledge start
```

Closing the attached Herdr client normally leaves the persistent server
running. Use detached mode when a script, agent, or subsequent command needs
to continue in the same terminal:

```sh
fledge start --detach
fledge start --detach --json
fledge agent spawn --name implementer --harness codex
```

`fledge start --json` is rejected because interactive attachment and
machine-readable output cannot share the terminal; combine `--json` with
`--detach`.

The initial orchestrator split is created only once while the Herdr server
remains running. Fledge reuses the first tab returned by workspace creation, so
there is no unused bootstrap tab before `orchestrator`. Later starts focus its
primary pane without resetting renamed or resized panes, extra panes, or other
layout edits. If the primary pane was closed, Fledge uses another pane in the
tab; if the whole initialized tab was closed, it creates a replacement without
repurposing unrelated user tabs.

A stopped deterministic session is disposable. The next `fledge start`
deletes its saved Herdr namespace and clears stale workspace, orchestrator,
socket, and agent mappings before launching a clean server. The coordinated
stop generation counter and the last picker-selected harness/model are retained
so an attached client can recognize an intentional shutdown and the next
interactive spawn can reuse the prior choice.

Fledge searches upward from the invocation directory for the closest marker,
so commands work in any project subdirectory:

```sh
mkdir -p src/component
cd src/component
fledge status
```

From a project subdirectory in another terminal:

```sh
cd /path/to/project/src/component
fledge status
fledge agent spawn --name implementer --harness codex --model gpt-5
fledge agent prompt implementer "Run the tests and fix the failure." --wait
fledge agent wait implementer --until idle,done,blocked --timeout 10m
fledge agent read implementer --source recent-unwrapped --lines 120
fledge agent attach implementer
fledge agent stop implementer
fledge stop
```

## Durable agent messages

Each server lifecycle has one private, append-only audit stream under
`.fledge/logs/agents/<run-id>.jsonl`. Fledge commits and syncs a message before
asking Herdr to inject it. An accepted injection is only `awaiting_ack`; the
recipient must acknowledge it or send a linked reply before it becomes
`acknowledged`.

```text
queued ──inject──▶ awaiting_ack ──ack/reply──▶ acknowledged
   ▲                    │
   │                    └─agent exits──▶ failed ──new activation──┐
   └────definite delivery failure─────────────────────────────────┘

ambiguous transport ──▶ uncertain     sender/user ──▶ cancelled
```

Send from the user mailbox or from a managed agent pane:

```sh
fledge agent message send implementer "Please review the parser."
fledge agent message send reviewer --file request.md
fledge agent message ack msg_example
fledge agent message reply msg_example "Done; see the linked changes."
```

The sender is inferred from `HERDR_PANE_ID` only when both the saved mapping
and live Herdr snapshot prove it; otherwise the sender is `user`. Agent names
are portable identifiers, and `user` is reserved for the owner mailbox.
Messages to stopped agents remain queued and replay oldest-first on their next
activation. Messages do not replay into a later server run.

Audit commands remain available while Herdr is stopped:

```sh
fledge agent message inbox
fledge agent message history implementer --with reviewer --all-runs
fledge agent message show msg_example
fledge agent message runs
```

Use `retry --force` only when deliberately reinjecting a message already
awaiting acknowledgement. Cancellation prevents future replay but cannot
retract text already injected into an agent prompt.

`fledge status` and `fledge start` report the selected session and retain
`session_source: "derived"` in human and JSON output for compatibility.

## Deterministic sessions and durable mappings

Every project is assigned exactly one session name:

```text
fledge-<project-slug>-<8-character-path-hash>
```

Fledge always manages that session and ignores every other Herdr session, even
when another session contains the same repository. There is no `--session`
override. Legacy `associations.json` files are neither read nor rewritten.

Fledge keeps logical-agent-to-pane mappings in `$XDG_STATE_HOME/fledge`, or
`~/.local/state/fledge` when `XDG_STATE_HOME` is unset. Updates use file locks,
`fsync`, and atomic replacement. Existing Fledge-labeled panes are validated
against the deterministic session's live snapshot and reused. A new logical
agent gets one tab and pane labeled with its raw name in the project workspace.
Inside Herdr, `agent spawn` replaces the current pane by default and leaves its
tab label untouched; use `--new-tab | -N` to request a dedicated tab.

Running `fledge agent spawn` interactively opens fuzzy-searchable pickers for
the missing installed harness and model, then asks for the missing agent name
last. After a harness or model picker contributes to a successful launch, its
resolved harness/model pair is saved for that project. The next harness picker
shows `Last used — <harness> · <model>` first; selecting it reuses both values
and skips the model picker (`default model` means the harness default). The
shortcut is hidden when that harness is no longer installed. Explicit flags
still win, and prompting only for an agent name does not update the saved
choice. Pi models are grouped under collapsible provider headers; OpenCode Go
and OpenCode Zen providers contain a second level of collapsible model-creator
headers. The supported harnesses are Claude Code, Codex, Pi, and OpenCode.
Non-interactive use requires `--name | -n` and `--harness | -k`; omitting
`--model | -m` uses the harness default. Native harness arguments follow `--`:

```sh
fledge agent spawn -n reviewer -k claude -m sonnet -- --permission-mode plan
```

On the first attached `fledge start`, the picker is opened automatically in
the left orchestrator pane with the name `fledge-orchestrator`. The right pane
remains an ordinary control shell. Detached starts and attachments to an
existing session do not open it.

The home directory itself cannot be a Fledge project root. Searches made below
the home directory stop before inspecting it; searches elsewhere continue to
the filesystem root.

## Output and prompt input

Every non-interactive command supports `--json | -j`. JSON output has
`schema_version: 1`, an `ok` boolean, and either `data` or a stable `error`
object. Usage failures exit 2; runtime, safety, transport, and Herdr failures
exit 1. Interactive `fledge start` and `agent attach` reject JSON mode;
`fledge start --detach --json` retains the standard start JSON envelope.
`fledge version` requires neither a Fledge project nor Herdr.

Prompts accept exactly one source:

```sh
fledge agent prompt reviewer "Review the staged diff."
fledge agent prompt reviewer --file review.md
cat review.md | fledge agent prompt reviewer --file -
```

`prompt --wait` and `agent wait` leave Herdr's settled-state default intact:
`idle`, `done`, or `blocked`. The `unknown` state matches only when explicitly
requested with `--until unknown`.

## Lifecycle states and stopping

- `idle`: ready for input
- `working`: actively processing
- `blocked`: waiting for user action or permission
- `done`: the current task completed
- `unknown`: Herdr cannot establish a supported lifecycle state
- `stopped`: Fledge's retained pane is at its shell with no active agent

`fledge agent stop` first sends `Ctrl+D`. If necessary it interrupts with
`Ctrl+C`, waits for a settled state, and retries `Ctrl+D`. On timeout the pane
is preserved and the command returns `agent_stop_timeout`. `--force | -f`
explicitly closes the pane instead.
The saved `fledge-orchestrator` pane cannot be force-closed through
`agent stop`; use `fledge stop --force` for a coordinated shutdown.

In an interactive terminal, `fledge stop` shows every live agent and asks for
confirmation before changing the session. After confirmation, Fledge asks all
live agents to exit gracefully under one shared 10-second budget. It then
completes coordinated server shutdown even if some agents remain; Herdr
terminates those remaining pane processes as part of stopping the session.

Non-interactive and JSON invocations do not prompt. Without `--force | -f`,
they refuse to terminate the server while agents are live. `--force | -f`
bypasses interactive confirmation and the non-interactive live-agent guard,
but still attempts graceful agent shutdown before forced completion.

After coordinated shutdown, Fledge waits for Herdr to report the session
stopped and permanently deletes its saved namespace. A session that was
already stopped is also deleted, and a missing session still causes stale
Fledge mappings to be cleared. Interactive cleanup of either case is confirmed
first. Project files, branches, and Git worktrees are not deleted.

Stopped session backlogs can be reviewed and removed outside an initialized
project:

```sh
fledge sessions prune --dry-run
fledge sessions prune --yes
fledge sessions prune --all --yes
```

By default prune selects only stopped, non-default sessions in the `fledge-`
namespace. `--all | -a` includes all stopped, non-default named sessions.
Interactive use asks once for confirmation; non-interactive and JSON use
requires either `--yes | -y` or `--dry-run | -n`.

## Development

```sh
./scripts/build.sh
```

Set `FLEDGE_INTEGRATION=1` to enable the isolated local Herdr integration test.
It requires Herdr 0.7.5+, creates a temporary initialized project and its
deterministic session, and verifies that the session is deleted at the end.
