# fledge

Fledge manages a project-local [Herdr](https://herdr.dev/) session from the
command line.

## Prerequisites

- Go 1.26 or newer, matching the version required by `go.mod`.
- [Herdr](https://herdr.dev/) installed and on `PATH`. Fledge drives every
  session through the `herdr` command.
- Herdr protocol 16 or newer for the watcher's event-stream mode, matching
  `min_protocol` in `.fledge/watch.json`. On an older Herdr, the watcher
  degrades to snapshot polling.

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

This creates tracked `.fledge/config.json`, `.fledge/watch.json`, and
`.fledge/profiles/orchestrator.toml` files. Edit the profile to change the
instructions sent to the project's coordinator and `watch.json` to tune
supervision.

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

Workers continue to receive their instructions in each per-spawn prompt. For
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
  --prompt "Review the current diff"
```

Commands invoked by agents or scripts must provide `--name` and `--harness`;
omitting `--model` uses that harness's default. Harness-native arguments can be
placed after `--`. Model flags must use Fledge's `--model` option.

`--cwd` may select the owning Fledge project root or one of its descendants.
Fledge resolves symlinks before checking this boundary and rejects external
directories and directories owned by a nested or otherwise different Fledge
project.

Every spawned agent receives harness-neutral Fledge messaging instructions,
even when `--prompt` is omitted. These require progress updates and a completion
summary to `orchestrator`, plus correlated replies to incoming messages. They
also prohibit polling the Fledge inbox and direct Herdr agent communication or
inspection. When provided, the caller's `--prompt` task follows those
instructions in the same single prompt submission.

Workers also receive the absolute path of their append-only watcher status
file. A status line starts with one of these exact lower-case verbs followed by
a colon:

```text
working: concise progress
done: concise result
needs-decision: decision the orchestrator must make
blocked: concrete blocker
failed: concrete failure
paused: reason work is paused
```

`working` and `paused` are recorded without waking the orchestrator. `blocked`,
`needs-decision`, and `failed` are actionable. `done` is held briefly so the
worker's ordinary Fledge completion message can arrive; a missing completion is
then escalated. Status reporting supplements Fledge messaging and never replaces
the required progress and completion messages.

After assigning the initial task through `fledge agent spawn --prompt`,
orchestrators coordinate with `fledge agent message send` and `reply`. An
injected Fledge completion message is the completion signal; the orchestrator
then stops that worker with `fledge agent stop`. Managed agents never poll the
Fledge inbox or use direct Herdr commands to inspect or collect agent output.

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

Fledge persists each message before immediately injecting it through Herdr. A
`delivered` status means Herdr accepted the injection; Fledge does not wait for
the agent to finish processing the message. Recipients can send a correlated
reply using the incoming message ID:

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

## Watcher

Fledge automatically launches one detached watcher for each active project
session when starting, reattaching, or successfully spawning a worker. The
watcher combines worker status files with Herdr agent-status events. If Herdr's
event protocol or socket is unavailable, it continues with snapshot polling.
Set `"enabled": false` in `.fledge/watch.json` to disable both attached and
detached watcher modes cleanly.

Attach a live decision-log monitor with:

```sh
fledge watch
```

If the background watcher is running, this prints roughly the last 50 complete
lines and follows new lines until canceled or the watcher exits. If no watcher
owns the session lock, the command runs the watcher in the foreground and
writes decisions to both the terminal and the log. The hidden
`fledge watch --daemon` form is reserved for Fledge's lifecycle launcher; it
writes only to the log and exits successfully when another watcher already owns
the session lock.

Watcher decisions are appended to the owner-only
`.fledge/logs/<session>/watch.log`. Queued lines include durable `w-...` wake
IDs. Delivery lines include the injected message ID and every retired wake ID,
allowing a queued decision to be traced through delivery even after ledger
compaction.

The tracked `.fledge/watch.json` accepts these settings:

```json
{
  "version": 1,
  "enabled": true,
  "poll_interval_seconds": 15,
  "idle_poll_interval_seconds": 60,
  "signal_grace_seconds": 2,
  "heartbeat_seconds": 600,
  "heartbeat_max_seconds": 7200,
  "wake_min_interval_seconds": 30,
  "done_message_grace_seconds": 90,
  "event_stream": true,
  "min_protocol": 16
}
```

Unknown fields and status/event values are ignored for forward compatibility.
Actionable observations are appended to a durable wake ledger before their
suppression markers advance, then batched into watcher messages sent from
`watcher` to `orchestrator`. A normal ledger write failure leaves markers in
place so the observation retries. If the ledger is corrupt, the watcher sends
one explicit warning and continues with in-memory deduplication; supervision
continues, but crash-safe replay is unavailable until restart.

Stop and permanently delete the nearest Fledge session in the current directory
or one of its parents:

```sh
fledge stop
```

Stopping requires confirmation from an interactive terminal. Fledge's project
storage is divided by purpose:

- `.fledge/config.json`, `.fledge/watch.json`, and
  `.fledge/profiles/orchestrator.toml` are tracked. Edit the TOML profile to
  customize coordinator instructions and `watch.json` to tune supervision.
- `.fledge/profiles/generated/orchestrator.md` is an ignored, reusable rendered
  prompt owned by Fledge. It is refreshed on fresh startup when the profile or
  mandatory policy changes and is preserved across stop and cleanup.
- `.fledge/tmp/<session>/` is ignored ephemeral state, including messaging and
  watcher locks, watcher PID files, worker status files, the durable wake
  ledger and OpenCode's original configuration snapshot. It is removed after a
  successful stop, stale-session cleanup, or completed failed-start rollback,
  but retained when Herdr session deletion fails so cleanup can be retried.
- `.fledge/logs/<session>/` contains `fledge.log`, `messages.jsonl`, and
  `watch.log`.
  Successful stop and stale-session cleanup preserve these audit/debug logs;
  a completed failed-start rollback removes logs for the unusable session.

The ignored `.fledge/session.json` remains the active runtime session pointer.
