# fledge

Fledge manages a project-local [Herdr](https://herdr.dev/) session from the
command line.

## Build and install

Build and test Fledge from the repository root:

```sh
scripts/build.sh
```

Install the built binary to `~/go/bin/fledge`:

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
files themselves are not rewritten. At `fledge start`, Fledge snapshots the
orchestrator profile and Fledge policy, plus Codex escalation instructions when
applicable, into durable harness-level instructions. Conversation clearing,
compaction, harness restarts, and mode changes retain that snapshot. Profile
edits take effect only after a full orchestrator restart with `fledge stop` and
`fledge start`. Fledge does not submit an initial user prompt, so a newly
launched orchestrator starts idle with those durable instructions already
loaded.

Workers continue to receive their instructions in each per-spawn prompt. For
OpenCode, Fledge preserves the original inline configuration and applies the
coordinator policy only to the coordinator, not to control or worker panes.

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

Stop and permanently delete the nearest Fledge session in the current directory
or one of its parents:

```sh
fledge stop
```

Stopping requires confirmation from an interactive terminal. Fledge stores the
runtime session pointer and active-session message audit in ignored files under
`.fledge/`. Removing a session also removes its message audit, while leaving the
tracked config, profiles, and any other `.fledge/` content untouched.
