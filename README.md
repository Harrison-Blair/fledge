# fledge

Fledge manages a project-local [Herdr](https://herdr.dev/) session from the
command line.

## Prerequisites

- Go 1.26 or newer, matching the version required by `go.mod`.
- [Herdr](https://herdr.dev/) installed and on `PATH`. Fledge drives every
  session through the `herdr` command.
- Herdr protocol 19 or newer. Coordination is event-driven end to end and has
  no polling fallback, so `fledge start` refuses an older Herdr and asks you to
  upgrade it and then run `fledge stop` and `fledge start` yourself.

## Build and install

Build and test Fledge from the repository root:

```sh
scripts/build.sh
```

Install the built binary to Go's install directory (`go env GOBIN`, otherwise
`$(go env GOPATH)/bin`, otherwise `~/go/bin`):

```sh
scripts/install.sh
```

To build and install in one step, including when replacing an existing
installation:

```sh
scripts/reinstall.sh
```

Set `FLEDGE_INSTALL_DIR` to install somewhere else:

```sh
FLEDGE_INSTALL_DIR=/path/to/bin scripts/install.sh
```

If another `fledge` executable appears earlier in `PATH`, the installer reports
its location and the `FLEDGE_INSTALL_DIR` command needed to replace it.

## Usage

Initialize a project once. The optional path defaults to the current directory:

```sh
fledge init [path]
```

This creates tracked `.fledge/config.json` and
`.fledge/profiles/orchestrator.toml` files. Edit the profile to change the
instructions sent to the project's coordinator.

Initialization also creates the managed
[Codex command policy](https://learn.chatgpt.com/docs/agent-configuration/rules)
`.codex/rules/fledge.rules`. In repositories trusted by Codex, this policy
allows `fledge` commands to run outside the sandbox, but explicitly forbids
`fledge start` and `fledge stop`; users must run those lifecycle commands
directly in their own terminal. It also forbids direct Herdr agent communication
and inspection commands, including session-selector forms that could otherwise
bypass a command-prefix rule. Non-communication Herdr commands without those
global selectors, and shell commands, continue to use normal Codex permissions.

Running `fledge init` again updates exact prior Fledge-generated policies, but
does not overwrite other differing files. Before launching a Codex coordinator
or worker, `fledge start` and `fledge agent spawn` backfill a missing policy and
accept the current policy byte-for-byte; they do not migrate legacy or
conflicting policies. Run `fledge init` to migrate a legacy policy, or move or
remove a conflicting policy and rerun `fledge init`.
Codex loads project rules only from trusted repositories and only when a Codex
session starts. After upgrading an already-running Fledge project, run one
user-controlled `fledge stop` and `fledge start` so its Codex sessions load the
new policy.

Start the project orchestrator, or reattach to its existing session:

```sh
fledge start
```

On first start, Fledge prompts for an installed agent harness and model when it
is running in a user terminal. The same choices can be supplied explicitly:

```sh
fledge start --harness codex --model gpt-5.4
```

The orchestrator occupies the first pane of the `orchestrator` tab, with a
control shell beside it. Reattaching preserves that layout and all running
processes. Fledge appends a mandatory communication policy to every
orchestrator profile at launch, for every harness. This runtime policy requires
Fledge messages even when an existing custom profile says otherwise; profile
files themselves are not rewritten. On a fresh `fledge start`, Fledge renders
the editable profile and mandatory policy to the ignored, owner-only
`.fledge/profiles/generated/orchestrator.md`. Claude, Pi, and OpenCode reuse that
file; Codex retains an escaped inline developer-instructions override that also
includes its Codex-specific escalation guidance. Conversation clearing,
compaction, harness restarts, and mode changes retain the rendered instructions.
Profile edits take effect only after a full orchestrator restart with
`fledge stop` and `fledge start`. Fledge does not submit an initial user prompt,
so a newly launched orchestrator starts idle with those durable instructions
already loaded.

The mandatory policy also tells coordinators to inspect model catalogs with
`fledge agent models [harness]` and pass the selected exact value to
`fledge agent spawn --model`. Because the policy is appended at launch,
existing custom coordinator profiles receive this guidance after the same full
restart without being rewritten.

Workers continue to receive their coordination instructions in each per-spawn
prompt. For
OpenCode, Fledge preserves the original inline configuration and applies the
coordinator policy only to the coordinator, not to control or worker panes.

List the advisory model catalogs for every installed supported harness, in
Fledge's supported harness order:

```sh
fledge agent models
```

Pass a harness ID, display name, or executable name to list only that installed
harness:

```sh
fledge agent models "Claude Code"
```

The stable table shows the harness, provider or integration group, exact model
value, friendly name, and description. `(default)` means omit `--model` and use
the harness's configured default. Discovery warnings do not hide the default or
any models that were available. Claude's built-in catalog includes its current
moving aliases, canonical IDs, and active legacy IDs; it intentionally omits
dated snapshots, cloud-platform spellings, and deprecated or retired models.
Catalogs guide selection but do not validate launches, so an explicit custom
model value remains valid even when it is not listed.

Spawn another agent in a dedicated, matching-name tab:

```sh
fledge agent spawn \
  --name reviewer \
  --harness claude \
  --model opus \
  --task "Review the current diff"
```

Commands invoked by agents or scripts must provide `--name` and `--harness`;
omitting `--model` uses that harness's default. Harness-native arguments can be
placed after `--`. Model flags must use Fledge's `--model` option.

`--cwd` may select the owning Fledge project root or one of its descendants.
Fledge resolves symlinks before checking this boundary and rejects external
directories and directories owned by a nested or otherwise different Fledge
project.

Every spawned agent receives harness-neutral Fledge coordination instructions,
even when `--task` is omitted. These require progress updates and a completion
summary to `orchestrator`, plus correlated replies to incoming messages. They
also prohibit polling the Fledge inbox and direct Herdr agent communication or
inspection.

`--task` is atomic with registration: the agent's registry entry, its first
assignment, and the wake that delivers that assignment are appended to the
session ledger as one fsynced transaction, so a worker never exists with a task
that was never delivered, or the reverse.

`--can-delegate` lets a worker create child tasks of its own. A delegating
worker must name the assignment it is delegating from with `--parent-task`,
which is what keeps the task tree connected and lets a cancellation reach every
descendant. Callers without an active parent task, and agents without
`--can-delegate`, are refused. The names `user` and `orchestrator` are reserved
coordination identities and cannot be spawned.

## Tasks

The durable registry and its task tree are managed with `fledge agent list` and
the `fledge agent task` verbs:

```sh
fledge agent list
fledge agent task assign <agent> [task] [--parent-task <id>] [--file <path>]
fledge agent task progress <task-id> <text>
fledge agent task blocked <task-id> <reason>
fledge agent task needs-decision <task-id> <question>
fledge agent task resume <task-id> [detail]
fledge agent task complete <task-id> [detail]
fledge agent task fail <task-id> <reason>
fledge agent task cancel <task-id> [detail]
fledge agent task list
fledge agent task show <task-id>
```

Every verb accepts `--file` (`-F`, or `-` for stdin) in place of an inline
argument, for text that shell quoting cannot carry. `progress`, `blocked`,
`needs-decision`, and `fail` require their detail; the rest accept one
optionally.

`fledge agent list` shows each registered pane's live state, the harness it
runs, whether it may delegate, and its parent task. An agent with no assignment
is unassigned, one holding an active task is assigned, and a stopped pane's
unfinished work becomes orphaned.

An agent holds at most one active task at a time. A parent task cannot be
completed while any child of it is unfinished, and cancelling or failing a task
cancels every descendant. Losing the agent an update was destined for never
blocks the update itself: the durable transition is recorded and the wake is
simply dropped, so a worker whose coordinator has gone can still record that it
finished, failed, or is blocked.

Each command only validates its inputs, appends the resulting events to the
fsynced session ledger, and makes sure a dispatcher is running. It never waits
on a Herdr delivery, list, or snapshot, so a wedged agent pane cannot stall an
unrelated command.

The project session record is bound to the ledger's random session ID. A
missing ledger, an unbound legacy record, or a mismatched ID is rejected instead
of being silently initialized as current state; reattaching with `fledge start`
validates and upgrades a legacy record. This durable check does not prove that
the Herdr process is still live—doing that would require a Herdr call—so
lifecycle commands remain responsible for live-session checks.

New managed panes also receive a random authority token, while only its hash is
stored in the ledger. Coordination commands require the token and pane ID to
agree, and can recover the registered identity when `HERDR_PANE_ID` alone was
cleared. A process that deliberately clears both inherited variables is locally
indistinguishable from the user's unmanaged control shell without a Herdr query
or OS-specific process attestation; this is an explicit protocol limitation,
not a hostile-process security boundary.

Wakes are routed to exactly one participant. Assignment and resumption wake the
assignee; blocked, needs-decision, completion, failure, and orphaning wake the
agent that assigned the work; cancellation wakes the assignee that has to stop.
Progress is recorded durably and wakes nobody. Ordinary messages always wake
their recipient.

Orchestrators stop a worker with `fledge agent stop` once its task is terminal.
Managed agents never poll the Fledge inbox or use direct Herdr commands to
inspect or collect agent output.

Stop a named agent and close its pane:

```sh
fledge agent stop reviewer
```

The orchestrator is protected from this command; use `fledge stop` to tear down
the complete Fledge session.

Send a message to a live named agent in the current project session:

```sh
fledge agent message send reviewer "Please check the authentication changes"
```

Fledge appends each message and its wake to the session ledger and returns; the
session's dispatcher performs the injection. A `delivered` status means Herdr
accepted the injection; Fledge does not wait for the agent to finish processing
the message. Recipients can send a correlated reply using the incoming message
ID:

```sh
fledge agent message reply <message-id> "The changes look correct"
```

A direct user or control-shell terminal can inspect the active session's
oldest-first transcript. It defaults to `user`; pass an optional identity to
include every message sent or received by that identity:

```sh
fledge agent message inbox [identity]
```

Transcript entries are labeled sent or received and retain delivery status and
failure details. Unknown identities have an empty transcript, and messages from
stopped agents remain visible. Managed agents receive messages through
injection and cannot query transcripts. New sends and replies require a live
recipient; messages are not queued or replayed after an agent or session
restart.

Detach from Herdr with `Ctrl+B`, then `Q`. The session and its processes keep
running in the background.

## Dispatcher

Every active project session has exactly one detached dispatcher, launched when
Fledge starts, reattaches, or successfully spawns a worker, and held to a single
instance by a session lock. It is mandatory: it is the only thing that turns a
durable coordination event into an agent wake.

The dispatcher waits on two event sources and no clock. Filesystem
notifications tell it the session ledger has grown; a Herdr protocol 19 event
subscription tells it that a registered pane changed agent status or closed. It
never polls either one, and it has no configuration to tune.

Each requested wake carries a stable delivery ID and is replayed until a
terminal outcome is durably recorded, so a dispatcher killed mid-delivery
resumes exactly where it stopped. An agent that cannot be reached has its
failure recorded and the remaining wakes still go out. A pane Herdr reports
closed is retired from the registry, and its unfinished work is orphaned. A
session whose last agent has stopped is a resting state, not a failure: the
dispatcher stays up with no subscription and delivers the next spawn's wakes.

Attach a live monitor with:

```sh
fledge watch
```

If a dispatcher is already running, this prints roughly the last 50 complete
lines and follows new lines until canceled or that dispatcher exits. Otherwise
the command runs the dispatcher in the foreground and writes to both the
terminal and the log. The hidden `fledge watch --daemon` form is reserved for
Fledge's lifecycle launcher; it writes only to the log and exits successfully
when another dispatcher already owns the session lock.

Dispatcher activity is appended to the owner-only
`.fledge/logs/<session>/dispatcher.log`.

Stop and permanently delete the nearest Fledge session in the current directory
or one of its parents:

```sh
fledge stop
```

Stopping requires confirmation from an interactive terminal. Fledge's project
storage is divided by purpose:

- `.fledge/config.json` and `.fledge/profiles/orchestrator.toml` are tracked.
  Edit the TOML profile to customize coordinator instructions.
- `.fledge/profiles/generated/orchestrator.md` is an ignored, reusable rendered
  prompt owned by Fledge. It is refreshed on fresh startup when the profile or
  mandatory policy changes and is preserved across stop and cleanup.
- `.fledge/tmp/<session>/` is ignored ephemeral state, including the messaging
  and dispatcher locks, the dispatcher PID and readiness files, and OpenCode's
  original configuration snapshot. It is removed after a successful stop,
  stale-session cleanup, or completed failed-start rollback, but retained when
  Herdr session deletion fails so cleanup can be retried.
- `.fledge/logs/<session>/` contains `fledge.log`, `events.jsonl`, and
  `dispatcher.log`. `events.jsonl` is the append-only session ledger holding
  every message, registry, task, and wake event.
  Successful stop and stale-session cleanup preserve these audit/debug logs;
  a completed failed-start rollback removes logs for the unusable session.

The ignored `.fledge/session.json` remains the active runtime session pointer.
