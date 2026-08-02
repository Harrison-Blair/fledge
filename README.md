# Fledge

[![release](https://img.shields.io/github/v/release/Harrison-Blair/fledge?color=brightgreen)](https://github.com/Harrison-Blair/fledge/releases)

Fledge is a Linux CLI for running project-scoped AI agents in a deterministic
[Herdr](https://herdr.dev/) session. It provides a small lifecycle harness:
discover the project, start named agents, exchange durable asynchronous
messages, read their output, and dispose of the session when the work is done.

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
fledge agent message send implementer "Run the tests and fix the failure."
fledge agent message inbox
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
   ▲                    │                         │
   │                    └─agent exits──▶ failed ──┼─new activation─┐
   │                                              └─system notice──▶ sender
   └────definite delivery failure──────────────────────────────────┘

ambiguous transport ──▶ uncertain     sender/user ──▶ cancelled
```

Send from the user mailbox or from a managed agent pane:

```sh
fledge agent message send implementer "Please review the parser."
fledge agent message send reviewer --file request.md
fledge agent message ack msg_example
fledge agent message reply msg_example "Done; see the linked changes."
```

Delegated work uses `message send` and `message reply`. Sending returns after
the durable write and a bounded delivery handshake; it never waits for task
completion. A reply acknowledges the original task and is injected into the
live sender pane as soon as it arrives. If a worker activation ends before
replying, Fledge sends the original sender a correlated system notification;
system notifications are acknowledged rather than replied to.

The sender is inferred from `HERDR_PANE_ID` only when both the saved mapping
and live Herdr snapshot prove it; otherwise the sender is `user`. Agent names
are portable identifiers; `user` is reserved for the owner mailbox and
`fledge` for durable system notifications.
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
retract text already injected into a recipient harness.

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

## Managed agent profiles

Managed profiles keep an agent's launch settings and provenance in the project at
`.fledge/profiles/<name>.toml`. The filename supplies the profile name; `name`
is therefore not allowed inside the TOML document. A complete schema-version 1
profile looks like this:

```toml
schema_version = 1
description = "Reviews changes against project conventions"
harness = "codex"
model = "gpt-5.6"
effort = "high"
native_args = ["--image=architecture.png"]
instructions = """
Review for correctness, determinism, and missing tests.
Report concrete findings before suggesting refinements.
"""
```

Create a profile from fields or from a strict TOML file. Explicit field flags
overlay the file, which makes scripted customization deterministic. `-` reads
the TOML from stdin:

```sh
fledge agent profile create reviewer \
  --harness codex --model gpt-5.6 --effort high \
  --instructions "Review correctness and tests."
fledge agent profile create reviewer --file reviewer.toml --model gpt-5.6
cat reviewer.toml | fledge agent profile create reviewer --file -
```

Updates preserve unspecified stored fields when no file is supplied. With
`--file | -f`, the file becomes the new base and explicit field flags overlay
it. `--native-arg | -a` is repeatable and replaces the native argument list
when supplied.

```sh
fledge agent profile update reviewer --effort xhigh
fledge agent profile list
fledge agent profile show reviewer
fledge agent profile validate reviewer
fledge agent profile validate candidate --file candidate.toml
fledge agent profile delete reviewer
```

Launch a profile by passing its name to `spawn`. The logical agent name
defaults to the profile name and can be changed per launch. Profile agents
always start at the discovered project root, even when invoked from a nested
directory:

```sh
fledge agent spawn reviewer
fledge agent spawn reviewer --name review-auth --new-tab --timeout 1m
fledge agent spawn reviewer --json
```

Profiles may omit both `harness` and `model` to leave them as launch-time
selections. Interactive launches reuse the normal installed-harness/model
pickers and project-local last-used shortcut, filtered to harnesses that can
transport the profile's managed settings. Non-interactive and JSON launches
of a profile without a harness require `--harness | -k`; an omitted model uses
that harness's default. Explicit harness/model flags can fill only fields the
profile omits. Fields present in the profile remain locked. Profile agents
also reject `--cwd | -C` and extra arguments after `--`; put native arguments
in the profile instead. `--name | -n`, `--new-tab | -N`, and `--timeout | -t`
remain per-launch controls.

Except for the reserved `orchestrator` profile described below, managed effort
and instructions use each harness's reliable interactive transport:

- Claude Code supports both effort and instructions.
- Codex supports both through native configuration arguments.
- OpenCode profiles may select a model and safe native arguments, but its
  interactive TUI cannot reliably transport managed effort or instructions;
  profiles containing either are rejected before launch.
- Pi supports managed effort and instructions. Fledge writes instructions to a
  private content-addressed file under `.fledge/tmp/profile-instructions/` and
  passes its absolute path through Pi's `--append-system-prompt` option.

Every profile command supports `--json | -j`. Profile spawn JSON includes the
profile provenance and the exact final native argument vector. Agent list,
agent status, and project status JSON use the same provenance projection; human
agent tables add a `PROFILE` column whenever at least one displayed agent came
from a profile.

On the first attached `fledge start`, Fledge launches the project-local
`orchestrator` profile in the left orchestrator pane with the reserved name
`fledge-orchestrator`. The supplied profile contains only managed instructions,
so its compatible harness/model are selected at launch. A missing profile
silently opens the ad-hoc picker; an invalid, unreadable, or incompatible
profile warns before doing the same. Cancelling either picker leaves the pane
as a shell. The right pane remains an ordinary control shell. Detached starts
and attachments to an existing session do not launch a picker or profile.

The reserved profile keeps `.fledge/profiles/orchestrator.toml` as its canonical
instruction source but delivers those instructions through repository context,
not native launch arguments. Immediately before every `agent spawn
orchestrator`, Fledge synchronizes this block in root `AGENTS.md`:

```md
<!-- <fledge-managed-orchestrator> -->
## Fledge Orchestrator (managed)

<instructions from orchestrator.toml>
<!-- </fledge-managed-orchestrator> -->
```

It also maintains a Claude bridge in root `CLAUDE.md`:

```md
<!-- <fledge-managed-orchestrator> -->
@AGENTS.md
<!-- </fledge-managed-orchestrator> -->
```

An existing `@AGENTS.md` import or a `CLAUDE.md` symlink to `AGENTS.md` needs no
bridge. Synchronization is serialized with a Fledge lock and uses atomic file
replacement. Existing line endings and permissions are retained, and every
byte outside a valid managed block remains project-owned. Content inside the
markers is Fledge-owned and is refreshed from the profile. Missing, duplicate,
reordered, inline, or partially edited markers fail with
`profile_launch_invalid`; Fledge does not guess at the overwrite boundary or
launch the orchestrator. Profile instructions containing the reserved marker
text are rejected for the same reason.

Clearing the profile's instructions removes only the Fledge-owned blocks. A
generated context file is deleted only when the managed block is its complete
contents. Because Codex, Claude Code, Pi, and OpenCode reload this repository
context, the orchestrator policy survives their native clear/new-context
workflow without restarting the harness. Other project agents can see the root
block, but its wording applies specifically when coordinating delegated work.

The orchestrator profile requires durable asynchronous delegation. Tasks use
`fledge agent message send <name> <task>` and results return through correlated
`message reply` operations. Sending returns after a bounded delivery handshake;
the coordinator continues useful work or returns control instead of waiting
for task completion. Fledge injects replies and system failure notifications
into the sender pane as they arrive. Coordinators never use status/read polling
or background waits to detect completion.

The home directory itself cannot be a Fledge project root. Searches made below
the home directory stop before inspecting it; searches elsewhere continue to
the filesystem root.

## Output and message input

Every non-interactive command supports `--json | -j`. JSON output has
`schema_version: 1`, an `ok` boolean, and either `data` or a stable `error`
object. Usage failures exit 2; runtime, safety, transport, and Herdr failures
exit 1. Interactive `fledge start` and `agent attach` reject JSON mode;
`fledge start --detach --json` retains the standard start JSON envelope.
`fledge version` requires neither a Fledge project nor Herdr. Message sends and
replies accept exactly one source:

```sh
fledge agent message send reviewer "Review the staged diff."
fledge agent message send reviewer --file review.md
cat review.md | fledge agent message send reviewer --file -
```

## Lifecycle states and stopping

- `idle`: ready for input
- `working`: actively processing
- `blocked`: waiting for user action or permission
- `done`: the current task completed
- `unknown`: Herdr cannot establish a supported lifecycle state
- `stopped`: Fledge's retained pane is at its shell with no active agent

`fledge agent stop` first sends `Ctrl+D`. If necessary it interrupts with
`Ctrl+C`, waits for a settled state, and retries `Ctrl+D`. After a successful
stop, a one-pane dedicated agent tab is closed by default so stopped-agent tabs
do not accumulate. In-pane placements and tabs that have become shared are
left intact. JSON reports this as `tab_closed`. On timeout the pane is preserved
and the command returns `agent_stop_timeout`. `--force | -f` forcibly stops the
agent and also closes its safe dedicated tab.
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
