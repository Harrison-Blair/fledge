# Fledge

A Go CLI built with Cobra, with agent commands for Herdr.

```sh
go run . --help
go run . --version
go install .
```

## Install and update

Install with Go, or download a Linux amd64/arm64 archive and `checksums.txt` from
[Releases](https://github.com/Harrison-Blair/fledge/releases/latest). Check the
archive with `sha256sum --check --ignore-missing checksums.txt` before extracting.
Release archives contain `fledge`, `LICENSE`, and `README.md`.

```sh
go install github.com/Harrison-Blair/fledge@latest
fledge --version
fledge update --check
fledge update
fledge update --yes
```

Existing binaries without the `update` command need the Go install command or a
manual archive installation once. The updater installs the latest stable release
only, verifies the archive's SHA-256, and replaces the running executable while
preserving permissions and following symlinks. It needs write access to the
installation directory. Download, verification, and replacement failures leave
the installed executable intact. Updating affects subsequent invocations;
already-running agents and commands continue with their existing processes.

`--check` only reports availability. Ordinary updates ask for confirmation with a
default of no; noninteractive use requires `--yes`. Equal or newer stable versions
are left alone. Development builds cannot be compared reliably, so installing a
stable release over one requires confirmation or `--yes`. There are no background
checks. The updater currently supports Linux amd64 and arm64.

## Releases

Git tags own the version; there is no version file to edit. Merge reviewed changes
into `main`, then select **Actions → Release → Run workflow** on `main` and choose
a bump. The choice authorizes publication once checks and packaging pass:

| Bump | Example from `v0.4.7` |
| --- | --- |
| `patch` (default) | `v0.4.8` |
| `minor` | `v0.5.0` |
| `major` | `v1.0.0` |

You or an agent with authenticated GitHub write access can also run:

```sh
gh workflow run release.yml --ref main -f bump=minor
# Find the dispatched run, then watch it:
gh run list --workflow release.yml --event workflow_dispatch
gh run watch RUN_ID --exit-status
```

The workflow releases the exact selected `main` commit, runs all checks, builds
both Linux archives, and generates notes since the previous stable tag. It uses
a temporary draft to upload and verify every asset, then publishes automatically
and marks the release latest. Merging alone does not release.

If a run fails after creating its draft, rerun that original workflow run using
the same bump and commit. Existing matching assets are reused; mismatched assets
are refused. A published commit is a successful no-op. A draft targeting another
commit blocks new releases until you finish it or deliberately remove the draft.
Existing tags are never moved; deleting a draft does not authorize moving its tag.
Concurrent release requests are serialized; GitHub may replace an older pending
run with a newer request, so inspect the run status rather than assuming all
queued requests will publish.

Stable tags use `vMAJOR.MINOR.PATCH`. A `v2` or later release requires migrating
`go.mod` and imports to the corresponding `/vN` module path first. Release binaries
report their exact tag. Local builds use Go's embedded tag or commit-derived
version, including `+dirty` when appropriate; metadata-free builds report `dev`.

## Agents

Run agent commands inside Herdr, with `HERDR_ENV=1` and `HERDR_SOCKET_PATH` set.
Fledge talks directly to that session's Unix socket.

```sh
fledge agent spawn --name reviewer --harness claude --model sonnet
fledge agent spawn --name builder --harness codex --workspace backend --tab builds
fledge agent spawn --name task --harness codex --workspace backend --worktree new --branch feature/task
fledge agent spawn --name existing --harness claude --pane w2:p3
fledge agent spawn --name review-1 --profile reviewer --file brief.md
fledge agent profiles
fledge agent profiles reviewer --json
fledge agent adopt --name helper
fledge agent adopt --pane w2:p3 --name builder --json
fledge agent list --json
fledge agent list --mine
fledge agent list --parent 3f9a0c2e --json
fledge agent list --state idle --state blocked --harness codex
fledge agent list --registered --profile reviewer --ids
fledge agent list --task 5b21e0f4 --worktree .fledge/worktrees/feature/task
fledge agent current
fledge agent get --name reviewer
fledge agent get --id 3f9a0c2e
fledge agent get --pane w2:p3 --json
fledge agent read --name reviewer
fledge agent read --pane w2:p3 --source visible --lines 40 --json
fledge agent wait --name reviewer --timeout 10m
fledge agent wait --name reviewer --name builder --all --json
fledge agent wait --name reviewer --pane w2:p3 --any --until done
fledge agent wait --mine --state working --all --timeout 30m
fledge agent message --name reviewer --body 'Review the current diff'
fledge agent message --pane w2:p3 --file task.md
cat task.md | fledge agent message --name reviewer --file -
fledge agent message --name reviewer --name builder --body 'Rebase on dev'
fledge agent message --mine --state idle --file update.md --json
fledge agent send --name reviewer --text '/model claude-haiku-4-5-20251001' --key enter
fledge agent send --name reviewer --key down --key enter
fledge agent pause --name reviewer
fledge agent pause --pane w2:p3 --timeout 20s --json
fledge agent pause --name reviewer --no-wait
fledge agent stop --name reviewer
fledge agent stop --pane w2:p3 --force --json
fledge agent stop --name reviewer --name builder
fledge agent stop --mine --state idle --dry-run
fledge agent cleanup --dry-run
fledge agent cleanup --results-collected --json
fledge agent models --harness codex --json
fledge agent capabilities --harness claude --live
```

Spawn requires a unique live `--name` and a `--harness`, which a
[profile](#profiles) can supply. By default it creates a
new tab in the caller's workspace. Workspace and tab names match exactly and
case-sensitively: missing destination names are created, ambiguous names fail,
and an existing tab receives a fresh split. New containers reuse their initial
children. `--workspace-id` and `--tab-id` select existing IDs instead of names.
`--pane` uses an existing shell and excludes other placement, cwd, env, and split
flags. `--label` and `--focus` still apply to that pane.

Running `fledge agent spawn` with no flags or native arguments, with stdin and
stdout both on a terminal, prompts for the harness, model (from local discovery,
or the harness default), name, and placement: a new tab, a split right or down
of the caller's tab, or a new worktree. Empty answers take the shown default;
EOF or Ctrl-C cancels with exit status 2 before any Herdr call. The equivalent
flag form is printed before launch. Any flag, or a non-terminal stream, keeps the
ordinary required-flag validation.

Ordinary spawn honors Herdr's configured cwd policy unless `--cwd` is supplied.
A relative `--cwd` resolves against the caller's working directory; an
absolute path is used as given. `--cwd` only places the shell (and selects the
source for `--worktree new`); the agent is still registered in the repository
Fledge was invoked from (see [Identity](#identity)). Use repeatable
`--env KEY=VALUE` for new ordinary shells. `--direction` defaults to `right`;
`--ratio` delegates to Herdr when omitted. These flags only affect splits. `--focus` defaults to
false and focuses the destination before launch. `--timeout` is a duration,
default `30s`; its millisecond value must be greater than 3000 and at most
300000.

By default, spawn waits for the launch to settle before returning, so a
successful spawn reports the settled status (e.g. `idle`) rather than `unknown`.
Settled means ready for input, not just a lifecycle status: Herdr must report
`interactive_ready`, no pending launch, and `idle` or `done`, since an agent
such as pi reports `idle` seconds before it accepts a prompt. Spawn polls the
pane for this with or without a first prompt, and fails `unknown` without
registering or prompting if a different terminal, name, or harness answers.
`--timeout` is one budget from launch through the first prompt: Herdr requests
for the launch, lifecycle wait, readiness polls, registration, sender lookup,
and prompt submission stop at its deadline, and a reply that arrives after it
is not accepted. Registration's local step, taking the state lock and writing
the record, cannot be interrupted, so spawn can overrun `--timeout` while
another Fledge process holds the state lock; no first prompt is sent after the
deadline either way. Out of
budget before the prompt is submitted, spawn exits `partial` with `timeout` and
says the prompt was not submitted; out of budget while it is being submitted,
the outcome is `unknown`, so inspect with `fledge agent read --pane P` before
resending. A spawn that times out before the agent is ready does not register
it (register it later with `fledge agent adopt --pane P`); a record already
written is kept and reported, and a registration the deadline cuts off is
reported as `registration_error`. Herdr's own startup
reservation is the larger of `--timeout` and 30s: Herdr drops the name of an
agent whose reservation expires mid-launch, so a short `--timeout` bounds only
spawn itself, and a `partial` timeout can still finish launching under its
name. A launch unfinished when the reservation expires can still lose its name;
address it by pane. `--no-wait` restores the old
behavior: return once the launch begins, without waiting for readiness. If the
agent settles on `blocked` (its own startup dialog, e.g. an update prompt),
spawn fails with `agent_blocked` and a `partial` outcome (exit 1); the agent
and its record are left running. A wait timeout is also `partial`, like a
startup timeout. Human output then gives recovery hints addressed by pane,
since the name may not resolve: `fledge agent read --pane P` to inspect the
dialog, then `fledge agent send --pane P --key <key>` to answer it. Spawn never
answers a dialog itself.

Pass `--prompt TEXT` or `--file PATH|-` (mutually exclusive; unlike `message`,
inline text on spawn is `--prompt`, not `--body`) to deliver a first prompt once
the agent is ready. The prompt is read and validated before any Herdr mutation,
so a missing file never leaves a tab or agent behind. Spawn does not wait for
the prompted turn to finish. `--no-wait` cannot be combined with `--prompt` or
`--file`, since there would be no settled agent to prompt. The result's
`prompt_requested` reports whether a first prompt was given, so
`prompted=false` separates "none requested" from "not submitted". A spawn that
stops before prompting (blocked, timed out, or unconfirmed startup) says the
first prompt was not submitted; it is not queued or replayed, so resend it after
resolving the dialog with `fledge agent message --pane P --file <brief>` (or
`--body`). An uncertain `agent.prompt` failure makes no such claim, since the
prompt may have arrived.

With `--worktree new`, workspace selectors identify an **existing source**
repository workspace. That source takes precedence over `--cwd`; without either,
the source is Fledge's working directory. `--branch` defaults to the agent name,
and `--base` selects the starting ref. Without `--base`, spawn passes the branch
checked out in the primary checkout as the base (even when invoked from a linked
worktree); with a detached primary checkout it passes none and Herdr chooses.
Checkouts are created beneath the
primary checkout at `.fledge/worktrees/<branch>`, even when invoked from a linked
worktree. Branch slashes create nested directories. Existing branches or paths
fail; Fledge does not invent suffixes. `.fledge/.gitignore` excludes the
managed directory, except [profile](#profiles) files, without changing the root
ignore file.

`--worktree PATH` opens an existing checkout. Without an explicit source, the
absolute checkout path determines its repository. A newly opened workspace uses
its initial pane; an already-open workspace receives a new tab. `--tab NAME`
selects or creates a tab there. Worktree shells use the checkout directory.
Worktree mode excludes `--env`, `--pane`, and, in this version, `--tab-id`.
`--branch` and `--base` apply only to creation.

All 24 documented Herdr harness kinds are accepted. Fledge's optional `--model`
translates to verified native flags; it is currently unavailable for `kiro`,
`amp`, `muse`, `mastracode`, and `qodercli`. Those harnesses can still be launched
and receive native arguments. Model values remain opaque. Hermes uses its `chat`
subcommand. Documented native model conflicts are rejected when `--model` is set;
harness resume behavior can still affect model selection.

Pass native tokens with repeatable `--args=TOKEN`, then optional trailing
`-- TOKEN...`. Tokens retain whitespace and commas; Fledge does not parse a shell
command string. For example:

```sh
fledge agent spawn --name reviewer --harness claude --args=--permission-mode --args=plan
```

List includes unnamed agents and agents launched outside Fledge. Message takes
one or more targets and exactly one of `--body`/`--file`; it preserves newlines
and rejects empty or invalid UTF-8 content. Success acknowledges **submission**,
without waiting for the agent to begin or finish. Blocked agents require the
user to handle their approval dialog.

Targets are repeatable `--name`, `--pane`, and `--id` flags, which may be mixed,
or the filter flags `agent list` uses (`--state`, `--harness`, `--profile`,
`--task`, `--worktree`, `--registered`, `--mine`, `--parent`), but not both.
Filter flags AND together and repeating one ORs its values; matches never include
the caller, and an empty match fails with `no_agents_matched`. An explicit
target naming the caller is still messaged. Duplicate targets are rejected. One
resolved target gives the single result described here. Several targets are all
resolved first, so a lookup failure is `rejected` with nothing sent; then each
gets the same sender header and message ID, one after another, with `--confirm`
and `--timeout` applied to each. The result is
`{"mode": "fan-out", "targets": [...]}` with one row per target, in target
order: `target` (the flag value, or the record ID for a filter match, or the
pane for an unregistered one), `outcome`, `message_id` (`null` when not sent),
`agent`, and `error`. Outcomes are `submitted`, `confirmed`, `already_working`,
`unconfirmed` (submitted, but activity was not confirmed), `unknown` (may have
been submitted), and `rejected` (not sent). Delivery continues past failures;
any failed row makes the outcome `partial` with exit status 1, and human output
prints one line per target. For example, to tell every worker on a task:

```sh
fledge agent message --task 3f9a0c2e --body 'The API now returns 404 for missing tasks'
```

`--confirm` also waits, up to `--timeout` (default 10s; valid only with
`--confirm`), for Herdr to observe the agent become active after submission. A
reply of `working`, or a quickly finished `done`/`idle`, reports
`confirmed: true`. An agent already working when messaged still receives the
message, but reports `already_working: true` and `confirmed: false`: Herdr
tracks lifecycle state, not individual prompts, so this prompt's start is not
confirmed. An agent that turns `blocked` after submission, or Herdr's
`agent_prompt_stalled` (no activity seen within five seconds), is `partial`
with a `submitted` effect; a wait `timeout` or lost acknowledgement is
`unknown`. None of these is retried; the message was, or may have been,
delivered, so read the pane instead of resending. Refusals made before input
(`agent_blocked`, `agent_not_ready`) stay `rejected`.

Every prompt Fledge delivers, from `message` or from spawn's `--prompt`/`--file`,
starts with one header line naming the sender and a correlation ID, e.g.
`ᛉ fledge message from reviewer (w1:p2) · id m-0a1b2c · reply: fledge agent message --name reviewer`.
The sender is the caller's `HERDR_PANE_ID`, resolved through Herdr: an unnamed
agent appears as `unnamed agent (PANE)`, a pane without an agent as `pane PANE`,
and an unresolvable caller as `unknown sender`; only named agents get the reply
command. Attribution failures never block delivery. JSON results include
`message_id` and `sender` (`name`, `pane`, `kind`, `error`).

`fledge agent send` types raw input into a live agent's terminal with **no**
sender header, so the recipient sees no sender and has no reply channel. Use it
for input a harness must see verbatim, such as a slash command, or to answer a
dialog that blocks an agent. It takes exactly one of `--name`, `--pane`, or
`--id`, and at least one of `--text` and a repeatable `--key`. Text is delivered
first, as a paste, and does **not** press Enter; a newline inside it is typed
literally. Keys (`enter`, `esc`, `down`, `ctrl+c`, ...) are pressed after it, in
order, so add `--key enter` to submit. Both go to the resolved pane in one Herdr
`pane.send_input` call. Herdr validates every key name before writing, so an
unknown key rejects the send (`invalid_key`) and nothing is typed. Send works in
any agent state (`idle`, `working`, `blocked`, `done`, or `unknown`) without `--force`;
the output reports the status observed before sending, for example
`Sent input to reviewer (claude) in w2:p3; it was blocked before sending.` JSON
uses operation `agent.send`, the standard agent fields (with that earlier status),
and `submitted`. A lost acknowledgement is `unknown`. Sending does not wait for
or check the effect. Read the pane with `fledge agent read` to confirm it.
Claude Code's `/model` also saves the chosen model as the global default for new
Claude sessions (it rewrites `~/.claude/settings.json`), not only for the target agent.

`fledge agent get` inspects one live agent with exactly one nonempty `--name` or
`--pane` target and no positional arguments. It makes a single read request,
without focusing the pane or marking output seen. Labeled text includes the list
fields plus foreground working directory, interactive readiness, launch-pending
status, focus state, resolved title (the agent-reported title, else the stripped
terminal title, else the raw terminal title), and native session source, harness,
reference kind, and value. Unavailable values appear as `-`; known booleans appear
as `true` or `false`. JSON returns a single result object with these details under
`foreground_cwd`, `interactive_ready`, `launch_pending`, `focused`, `title`, and
`agent_session` (with `source`, `harness`, `kind`, and `value`). Unavailable
details are `null`; `interactive_ready` and `launch_pending` are optional and
appear as `null` when absent, while `focused` is always present on a successful
read. `effects` is empty. Failed reads use `rejected` with the error code and
phase.

`fledge agent read` prints a plain-text terminal snapshot of one live agent's
pane, selected by exactly one of `--name`/`--pane`. It resolves the agent, then
reads that pane without focusing it or marking output seen. `--source` is
`visible`, `recent`, `recent-unwrapped` (default; soft wraps joined), or
`detection`. `--lines N` requests the bottom N rows (0 to 4294967295); Herdr
returns at most 1000 rows, and `--lines 0` returns an empty, truncated snapshot.
Text output is a label, `Terminal snapshot of <name or pane> (<source>, <N> rows,
truncated: yes|no)`, followed by the text. Human output ends with a newline even
when the snapshot does not; `--json` preserves the bytes exactly. JSON adds `source`, `lines` (rows
returned), `text`, `revision`, and `truncated` to the standard agent fields. A
snapshot is the terminal's current contents, not a conversation transcript.

`fledge agent wait` blocks until agents reach a lifecycle state. Without
`--until`, it matches `idle`, `done`, or `blocked`; repeat `--until` to choose
states from `idle`, `working`, `blocked`, `done`, and `unknown`. Without
`--timeout`, it waits indefinitely with no transport deadline; Ctrl-C cancels
it. A finite `--timeout` is passed to Herdr and fails with `timeout`. A settled
state means the agent's turn ended, **not** that its assigned work succeeded.
`--name`, `--pane`, and `--id` are repeatable and may be mixed; duplicate
targets are rejected. An `--id` target follows its record's terminal and fails
closed with `agent_identity_stale` if a different terminal answers. Instead of
explicit targets, the filter flags `--state`, `--harness`, `--profile`,
`--task`, `--worktree`, `--registered`, `--mine`, and `--parent` select live
agents (mixing them with explicit targets is rejected). Filters AND together,
repeated values of one flag OR together, and the caller is never selected; an
empty match fails with `no_agents_matched`. Filter matches are reported by
record ID (by pane when unregistered), and registered matches get the same
terminal check as `--id`. One target, explicit or matched, prints
`<name> is <status>.` and its JSON result is the agent row. Two or more
targets, including several filter matches, need `--all` or `--any`, and each
target gets its own Herdr wait. `--all` waits for every target; the first target that fails
ends the wait with that failure and cancels the rest (near-simultaneous
failures may report `operation_failed`, and a shared `--timeout` can leave a
mix of timed-out and cancelled rows).
`--any` succeeds on the first match, records targets that fail (for example
`agent_not_running` when an agent exits) while others remain, then cancels the
rest; it fails only if every target fails. In human mode, a target that fails
while others remain is reported on stderr at once, for example
`b failed: agent_not_running: ... (still waiting on 1 target).`, and the wait
continues. Multi-target output is one line per target, and JSON (which writes
no progress lines) returns `mode`, `winner`, and `targets` rows with `target`,
`outcome` (`matched`, `errored`, or `cancelled`), `agent`, and `error`.

`fledge agent pause` interrupts the current foreground turn while preserving the
pane and conversation. It accepts exactly one nonempty `--name` or `--pane`, no
positionals, a positive `--timeout` (default `10s`), and `--no-wait`. It resolves
the agent, then submits one key sequence to the resolved pane, without retries,
escalation, focusing, closing, or submitting a prompt. Already `idle` or `done`
agents succeed without keys; `blocked`, `unknown`, launch-pending agents, and
missing or unknown harnesses are refused without keys.

The bindings assume harness defaults: double Escape for `amp`, `copilot`,
`opencode`, and `kilo`; single Ctrl+C for `droid`, `grok`, `hermes`, `mastracode`,
and `qodercli`; single Escape for every other currently supported harness.
Prior keybinding experiments covered Claude and Codex; live smoke tests of the
implemented `fledge agent pause` command covered Codex and OpenCode. Codex interruption settled
successfully and a subsequent message resumed the same session and pane.
OpenCode 1.18.25 visibly interrupted active output with double Escape, but the
immediate wait reported `blocked` (`partial`, `submitted=true`, `settled=false`,
`agent_blocked`); a subsequent message resumed the same session and pane and
reached `done`. See the [integration observations](reference/dogfood/friction.md).
Other mappings remain best effort; custom keybindings can change their effect. Interruption does not freeze a
process, undo completed work, drain queued prompts, or establish a persistent
paused state. Queued work can start again. Resume or redirect the conversation
with `fledge agent message`.

By default, pause waits within the remaining timeout for `idle` or `done` on the
same resolved terminal. These states indicate settlement, not successful task
completion. Becoming blocked fails immediately. A changed terminal, disappearance,
malformed response, or timeout also fails. `--no-wait` confirms only key delivery.
Text reports `Pause requested`, `Paused`, or `Already idle or done`. JSON uses
operation `agent.pause`, the standard agent fields plus `submitted` and `settled`,
and a `submitted` interrupt effect after acknowledgement. Lost or malformed send
acknowledgements produce `unknown`; failures after acknowledgement are `partial`.
Inspect the agent before retrying an uncertain interruption. Resolving a pane
cannot prevent its occupant changing before Herdr receives the keys.

`fledge agent stop` stops live agents by closing their panes. It resolves each
agent first and never closes a pane that does not host a known agent. Agents whose status is `working`, `blocked`, or `unknown`
are refused with exit status 2 unless `--force` is passed; `idle` and `done` agents
stop without it. A `working` agent is first given up to `--grace` (default 5s,
0s through 60s) to finish its turn, since a worker often reports just before its
turn ends: it is stopped if it settles `idle` or `done` in the same terminal, and
refused as before if it is still working, becomes `blocked`, or the wait fails.
`--grace 0` refuses at once. `blocked` and `unknown` agents get no grace, and
`--force` never waits, so `--grace` with `--force` is invalid. Stop sets `ended_at` on the agent's live record, if it has one,
just before closing the pane, so an agent can stop its own pane. If the pane
fails to close, stop reopens the record it ended, returning it from the archive;
it leaves a record another command ended alone and reports a reopen refused
because the terminal was registered again.

Stop takes the same targets as `agent message`: repeatable `--name`, `--pane`,
and `--id` flags, which may be mixed, or the `agent list` filter flags, but not
both. Filter flags AND together and repeating one ORs its values; matches never
include the caller, and an empty match fails with `no_agents_matched`. An
explicit target naming the caller is still stopped, as a single stop would, but
it closes the caller's own pane, so name it last. Duplicate targets are
rejected. One target gives the single result described above. Several targets
are handled one after another: each is looked up again when its turn comes (a
filter match with a record is followed by its record ID; one without must
still be the same terminal) and stopped under the rules above, with `--force`
and `--grace` applied to every target. A working agent may therefore wait up to
`--grace` each, so the worst-case wall time is `--grace` times the number of
targets. The listing a filter matched is a snapshot, and the per-target guard
still refuses an agent that started working before stop reached it. The result
is `{"mode": "fan-out", "targets": [...]}` with one row per target, in target
order: `target` (the flag value, or the record ID for a filter match, or the
pane for an unregistered one), `outcome` (`stopped`, `refused` without
`--force`, or `failed`, including a target that could not be looked up),
`agent`, and `error`. Stopping continues past refusals and failures; any row
that is not stopped, or whose record could not be ended, makes the outcome
`partial` with exit status 1. Effects are those of each stop. Human output
prints `Stopped N of M agents.` and one line per target.

`--dry-run` changes nothing: it looks up explicit targets, or lists a filter's
matches, and makes no closing, waiting, or record call. It always returns
`{"mode": "dry-run", "targets": [...]}`, even for one target, with rows whose
`outcome` is `stop`, `refuse` (the `error` gives the reason, such as the agent
state that needs `--force`), or `error` (the target could not be looked up).
A `working` agent is planned as `refuse`, since stop would first give it
`--grace` to finish its turn and it may still settle. Refusals exit 0; any
`error` row makes the outcome `rejected` with exit status 1. Human output
prints `Dry run: would stop N of M agents.` and one line per target with its
state.

`fledge agent models` lists coding-agent models discovered locally and does not
need a Herdr session. It reads the `pi`, `codex`, and `claude` caches under the
home directory and runs `opencode models` and `cursor-agent --list-models`; other
harness kinds are not yet supported. Rows are sorted by harness then model, and
`MODEL` is the value to pass to `--model`. `--harness` limits output to one
documented kind; an unsupported or uninstalled kind yields an empty list, and a
missing file, unreadable cache, or failing command silently contributes no rows.

### Capabilities

`fledge agent capabilities` reports, for every documented harness kind in
registry order, which operations Fledge can rely on. `--harness` limits it to one
kind. The report comes from Fledge's harness registry and needs no Herdr session:

```text
HARNESS  CAPABILITY       LEVEL   EVIDENCE
claude   interrupt        fledge  esc
claude   model_select     fledge  spawn passes --model
claude   model_discovery  fledge  cache: ~/.claude/cache/model-catalog
claude   resume           native  --resume <session-id>
claude   session_ref      fledge  Herdr hook reports id and transcript path at SessionStart
claude   lifecycle_hooks  native  Herdr integration target exists
claude   usage_tokens     fledge  transcript
claude   usage_cost       none    transcript records no cost
```

Levels: `fledge` means Fledge implements or uses it; `native` means the harness
or Herdr provides it natively; `none` means it is not available; `unknown` means
it has not been checked. `--live` makes one `integration.list` call and adds two
facts per kind as Herdr reports them: `available` (the harness binary is on
`PATH`) and `hook_state` (`current`, `outdated`, or `not_installed`). A kind Herdr
has no integration target for shows `-` in the table and `"live": null` in JSON.
`--json` emits `{"harnesses":[{"kind","capabilities":[{"name","level","evidence"}],"live"}]}`
as the outcome's result.

### Profiles

A profile is a named launch configuration: harness, model, native arguments, and
a role brief. `fledge agent spawn --profile NAME` applies one. Five built-ins
ship inside the binary and update with it; they are never copied into a
repository:

| Profile | Harness | Model | Args |
| --- | --- | --- | --- |
| `orchestrator` | `claude` | `claude-opus-5-5` | none |
| `implementer` | `claude` | `claude-opus-5-5` | none |
| `planner` | `pi` | `openai-codex/gpt-6-astra` | `--thinking xhigh` |
| `reviewer` | `pi` | `openai-codex/gpt-6-astra` | none |
| `verifier` | `pi` | `openai-codex/gpt-6-astra` | none |

Codex models run through the `pi` harness; no built-in uses `codex`. Built-ins
set no permission-mode arguments. Their roles are ordinary first-prompt
instructions, subordinate to the task and repository instructions; they grant no
permissions and change no task state.

`fledge agent profiles` lists the effective profiles with their source, and
`fledge agent profiles NAME` shows one resolved profile: source, built-in base,
harness, model, args, and full role. Both accept `--json`, need no Herdr session,
work outside Git (built-ins only), and write nothing.

Repository profiles live in `.fledge/profiles/NAME.toml` at the Git top level of
the checkout Fledge is invoked from, so a linked worktree uses its own branch's
profiles. This differs from agent and task state, which the primary checkout
shares, and from `--cwd`, which only places the agent. A file named after a
built-in overrides and extends it; any other name is a custom profile, which
may extend a built-in explicitly or stand alone:

```toml
schema_version = 1                # required
extends = "builtin:reviewer"      # optional; only built-in bases
harness = "claude"
model = "sonnet"
args = ["--permission-mode", "plan"]   # exact tokens, never shell text
role_append = "Focus on this repository's Go conventions."
# role = "Full replacement brief"       # instead of role_append
```

Only fields present in the file apply. Scalars replace the base, `args` replaces
the whole list (`[]` clears it), `role` replaces the role (`""` clears it), and
`role_append` adds a paragraph to the inherited role; `role` and `role_append`
cannot both be set. Unknown keys, wrong types, unknown bases or schema versions,
and invalid names fail before Herdr is contacted; an invalid override never falls
back to its built-in. Fledge never rewrites these files. Inherited built-in role
text follows binary upgrades; set `role` to pin it.

On spawn, explicit flags win. `--harness` matching the profile keeps everything;
a different `--harness` drops the profile's model and args, which belong to its
harness, and keeps the role. `--model` replaces the model, and `--args` or
tokens after `--` replace the args (a native model option in the effective args
still conflicts with `--model`). The role is sent before the task from
`--prompt`/`--file`, separated by a blank line, in one first prompt with one
sender header; a role alone is sent by itself, so `--no-wait` is rejected when
the profile has a role. Human output names the profile and its source, and JSON
adds `profile` (`name`, `source`, `path`, `base`).

`.fledge/.gitignore` keeps everything else in `.fledge` ignored and ends with
`*`, `!/profiles/`, `!/profiles/*.toml`, so profile files can be committed. The
next command that prepares `.fledge`, such as agent registration or managed
worktree creation, appends any missing rules to an existing file and preserves
its contents. A checkout Fledge creates (`agent spawn --worktree new` or
`worktree create`) gets its own `.fledge/.gitignore` with the same rules, so
scratch files such as `.fledge/tmp/` stay out of its `git status` even under an
allowlist-style root `.gitignore`; if that write fails, the checkout stays and
the outcome is partial. Opening an existing checkout writes nothing, so a linked
worktree Fledge did not create has no `.fledge/.gitignore` and root ignore rules
apply there. `.fledge/tmp/` is the agents' scratch directory: the leading `*`
in `.fledge/.gitignore` already ignores it, and proposals conventionally go
under `.fledge/tmp/plans/` (see [Proposals](#proposals)).

### Identity

Fledge keeps a durable record for each agent it launches or adopts, in
`.fledge/state/agents/` under the repository's primary checkout (ignored by Git,
shared by linked worktrees). A repository created with `--separate-git-dir`
records no path back to its primary checkout, so commands run from its linked
worktrees fail until you run `git config core.worktree <primary checkout path>`
once. Bare repositories, including their linked worktrees, have no primary
checkout and are not supported. A record holds an 8-hex `id`, the agent's name,
pane, workspace, harness, Herdr session (`HERDR_SESSION`), Herdr `terminal_id`,
`parent`, `profile` (the name of the profile `agent spawn --profile` used; null
for adopted agents, spawns without a profile, and records written before the
field existed), `registered_at`, `registered_by` (`spawn` or `adopt`),
`worktree_path`, `worktree_created`, `worktree_base`, `worktree_branch`,
`worktree_marker`, and `ended_at`.
`worktree_created` is true only when the agent's spawn created its checkout
(`--worktree new`), not when it opened an existing one; `worktree_base` is the
ref that checkout was created from, as spawn passed it to Herdr: `--base` as
given, otherwise the primary checkout's branch at spawn time (null when that
checkout was detached, so no base was passed). `worktree_branch` and
`worktree_marker` identify the created checkout itself: spawn writes a random
marker into that checkout's Git admin directory
(`<git dir>/worktrees/<name>/fledge-checkout-id`), which Git deletes when the
checkout is removed or pruned, so a checkout later recreated at the same path,
even on the same branch, never carries it. The marker is null if it could not be
written. Records written before these fields existed read as not created or
unidentified. The parent is the caller's live record when the
caller's pane hosts a registered terminal; otherwise it is null. Records are
never deleted: an ended record moves to `.fledge/state/agents/archive/`, where
lookups by ID still find it but scans for live agents no longer read it.

The terminal is a record's identity and the pane only locates it. Herdr gives a
pane moved across workspaces a new ID but keeps its terminal, so when a lookup
finds the recorded pane gone or hosting another terminal, Fledge searches
Herdr's agent list for the recorded terminal and, if found, updates the record's
pane and workspace and keeps its ID. `ended_at` is set when `agent stop` closes
the agent's pane, or when a lookup finds the terminal in no Herdr pane at all;
an ended record is no longer live. A terminal that still exists but hosts no
agent (its harness exited) only makes lookups fail; the record stays live. A
record belongs to the harness that registered it: when its terminal now runs a
different harness (the record's `harness` differs from Herdr's live agent kind,
both known), a lookup of that agent ends the record, `--id` fails with
`agent_identity_stale`, and the terminal counts as unregistered, so its new
agent has no parent record and can be adopted. Listings never attribute such a
record to the live agent. A
Herdr server handoff reissues every `terminal_id`, so records from before it
end on their next lookup and their agents need `fledge agent adopt`.

Spawn registers the agent once startup settles (or once launch begins with
`--no-wait`) and before any first prompt. Its result adds `id`, `registered`, and
`registration_error`. Outside a Git repository, or if the store cannot be
written, the agent still runs and spawn still succeeds: `registered` is false
and the text output shows `id: - (not registered: <reason>)`.

The repository Fledge is invoked from owns this coordination state, not the one
named by `--cwd`, which only changes where the agent's shell starts and, with
`--worktree new`, the source repository. Spawning from repository A with
`--cwd` in repository B records the agent in A; a caller outside any Git
repository gets an unregistered spawn even when `--cwd` names one. `--name` and
`--pane` are live Herdr selectors that work from anywhere, but `--id` and task
records are read from the invoking repository. A worker launched into B that
should use A's records must run those commands from A, for example
`cd /abs/path/to/A && fledge task complete --id <task> --summary "..."`.

`fledge agent adopt` registers an agent that is already running. Without
`--pane` it targets the caller's own pane. An unnamed agent needs `--name`,
which adopt sets through Herdr (`agent_name_taken` and `agent_launch_pending`
are reported as is). A named agent keeps its name; a different `--name` is
refused. A named agent whose terminal already has a live record is refused
with `agent_already_registered` and its existing ID; the check and the record
creation share one store lock, so concurrent adopts or spawn registrations of
one terminal produce exactly one record. An unnamed agent whose terminal
already has a live record, such as one whose Herdr name was lost, is named
through Herdr and keeps that record: the same ID, parent, and registration,
with the new name and its current pane stored and an `updated` `agent_record`
effect. If the record ends after the rename, the outcome is `partial`: the
agent is named but the record is unchanged. Success prints
`Adopted <name> (<pane>) as <id>.`

`get`, `read`, and `pause` accept `--id` in place of `--name` or `--pane`;
exactly one of the three is required. `message`, `wait`, and `stop` accept
repeatable `--id` alongside `--name` and `--pane`. An `--id` lookup follows a moved terminal as above, and fails closed
with `agent_identity_stale` when the terminal no longer hosts an agent (ending
the record only if the terminal itself is gone), the record belongs to another
Herdr session, or the record has ended; an unknown ID fails with
`agent_record_not_found`. `agent get` shows the record (Fledge ID, parent,
profile, registration time and source) whenever the live agent has one, and JSON adds
`record`. `agent list` adds `ID` and `PARENT` columns and a `PROFILE` column
after `HARNESS` (`-` when unregistered, parentless, or spawned without a
profile) and `id`, `parent`, and `profile` fields; it fails with `protocol_error`
rather than list partially when any entry of Herdr's agent list lacks a
required field, such as its terminal ID. Session names come from the
environment, so these checks are a workflow guard, not a security boundary.

`agent list` filters take effect together: different flags AND, and repeating
one flag ORs its values. `--state` matches the live Herdr status (`idle`,
`working`, `blocked`, `done`, or `unknown`) and `--harness` the live harness
kind. The rest match record fields of this repository's live agent records:
`--profile` the recorded profile name, `--task <id>` the task's owner (an
unowned task matches nothing; an unknown one fails with `task_not_found`),
`--worktree <path>` the recorded worktree path, resolved against the current
directory, and `--registered` any live record. Herdr's agent list spans every
repository, so these record filters never match agents registered elsewhere,
which show `-` in the ID column. `--parent <id>` keeps only live agents whose
record's parent is that ID; `--mine` does the same for the caller's own live
record. The two are mutually exclusive and only direct children are listed.
With any filter an empty result prints `No agents match.`; an unfiltered empty
listing prints `No live agents.` A record filter fails with the store error when
the repository's records cannot be read; without a filter the ID, PARENT, and
PROFILE columns are left empty instead. `--ids` prints one record ID per line
for the registered matches, skipping unregistered ones, for piping into other
commands; it is rejected together with `--json`.

`fledge agent current` shows the caller's own live record: its ID, name, pane,
workspace, harness, worktree, profile, parent, and the tasks it owns in the `assigned`
state, oldest first. The parent's name is the name recorded on the parent's
record, shown while that record exists; it can differ from the parent's live
Herdr name. JSON flattens the record into `result` and adds `parent_name` and
`tasks` (`id` and `title`). `agent current` and `agent list --mine` fail with
`caller_unregistered` when the caller's pane hosts no registered agent,
including outside a Herdr agent pane; register it with `fledge agent adopt`.

### Cleanup

`fledge agent cleanup` retires the caller's finished workers and removes the
checkouts their spawns created. It acts only on direct workers the caller
spawned: records whose parent is the caller's live record and whose
`registered_by` is `spawn`. The caller itself, siblings, grandchildren, adopted
agents, and unregistered agents are never touched; the caller must have a live
record (`caller_unregistered` otherwise).

A worker with a live record is stopped only when its agent is `idle` or `done`,
no live record names it as parent, and every task it owns, whoever created it,
is `verified` or `cancelled`. `idle` or `done` alone never counts as accepted
results. A worker that owns no task is held unless you pass
`--results-collected`, which asserts that you have read and kept all of its
reports; Fledge cannot detect that itself. Every other worker is held and
reported with its reasons.

A checkout is removed only when all of these hold: a worker's spawn created it
(`worktree_created`) and it is still that same checkout (its Git marker and
checked-out branch match `worktree_marker` and `worktree_branch`), it is a managed checkout under `.fledge/worktrees` and not
the primary checkout, a base branch was recorded, its worker has ended or is
being stopped now, it is clean, it is merged into that recorded base (for
example `dev`), not into the [integration branch](#integration-branch), and no
live agent other than the workers being stopped uses it, by the same rules as
`worktree remove`. Branches are always kept. Checkouts a worker only opened,
checkouts recorded before creation or identity was tracked, checkouts replaced
since the worker's spawn (reason `replaced since the worker's spawn`), and any
that fail a check are reported and kept. When several workers recorded the same
path, only the one whose recorded marker matches the checkout there can own it;
registration order never decides. Once the created checkout is gone, its record
can never authorize a removal again. Remove kept checkouts yourself with
`fledge worktree remove` once they are no longer needed. Because a stopped worker's record moves to the archive,
cleanup also reads archived records, so a later run removes a checkout whose
worker it already stopped once the checkout is safe. A checkout no longer
listed by Git is not reported.

`--dry-run` makes no changes, not even updating the caller's recorded pane,
and needs no assertion: it lists what would be stopped and removed and what is
held, with reasons. Otherwise cleanup stops each eligible worker by its record
ID, as `agent stop --id` without `--force`, after reading its tasks again, and
then removes each eligible checkout through `worktree remove` without
`--force`, which rechecks live agents, cleanliness, and the merge into the
recorded base, and last, immediately before deleting, that the checkout is
still at the resolved path it was planned at under `.fledge/worktrees` and
still carries the worker's marker and branch. A checkout moved meanwhile, even
with a symlink left at its old path, is kept as `moved since cleanup planned
it`. These identity and location checks apply only to cleanup; an explicit
`worktree remove` is unchanged. A task assigned, a live worker of the
worker registered, or a worker busy again after planning holds it; a checkout
replaced meanwhile is kept as `replaced since the worker's spawn`; nothing is
ever forced. The text output starts with
`Stopped N of M workers and removed N of M checkouts.` (`Dry run: would stop
...` for a dry run) and lists each worker as `stop`/`stopped`, `hold`, or
`failed`, and each checkout as `remove`/`removed`, `keep`, or `failed`, with
reasons. JSON uses operation `agent.cleanup`; `result` has `dry_run`, `caller`,
`workers` (`id`, `name`, `pane_id`, `agent_status`), and `checkouts` (`path`,
`branch`, `base`, `worker`), each entry with an `outcome` (`planned`, `done`,
`skipped`, or `failed`) and a `reason`. Effects are those of the stops and
removals. Held and skipped resources still exit 0. Failed actions do not stop
the others and are never rolled back: the outcome is then `partial` (or
`rejected` when nothing changed), or `unknown` when a stop or removal may have
applied, with exit status 1.

Cleanup rechecks its guards right before each action but cannot make task
assignment, Herdr, and Git changes one transaction, so run it when you are not
dispatching new work to those workers. A worktree whose spawn failed before
registration has no record and needs `worktree remove`. Clean means Git's view
of tracked and untracked, nonignored files; ignored build artifacts in a
removed checkout are lost. Squash-merged branches count as unmerged.

### Outcomes and recovery

Each command supports `--json`, emitting one Fledge object with `operation`,
`status`, `result`, `effects`, and `error`. Results use stable Fledge fields rather
than raw Herdr responses; unavailable values are null. Effects report known
resources and IDs or paths. Errors include a code, message, and failing phase.
Exit codes are 0 for success, 2 for invalid input, and 1 for runtime failures.

- `success`: the requested operation completed.
- `rejected`: failure is known to precede mutation, without uncertainty.
- `partial`: confirmed mutations preceded failure, or Herdr acknowledged a failed
  startup. A startup timeout can leave a process running, even in an existing pane.
- `unknown`: a mutation may have applied, but its response was lost or unusable.

Fledge retains created resources and never automatically retries mutations or
cleans up after failure. Inspect the reported pane and resources in Herdr before
retrying. For an unknown message outcome, inspect the conversation first to avoid
submitting the same prompt twice. A launched process is not proof of readiness.

A fresh split can briefly return `agent_pane_busy` while its shell reaches a
prompt. Fledge retries only the launch step for that code, up to six more times
with delays of 50ms doubling to a cap of 800ms (about 2.4 seconds in total),
without recreating any resource; other errors are never retried. If it still
fails, inspect the preserved pane, confirm it is an idle shell with no launched
agent, and retry into that specific pane instead of creating another one:

```sh
fledge agent spawn --name reviewer --harness claude --pane w2:p3
```

Use the actual pane ID from the partial outcome and retain your intended harness,
model, and native arguments. Fledge does not retry automatically.

## Tasks

A task is a durable brief with an owner and an outcome. Records live in
`.fledge/state/tasks/` beside the agent records, so they survive the owner's
pane closing. Owners and verifiers are Fledge agent record IDs (see
[Identity](#identity)). Task status changes only through these commands;
completion is never derived from Herdr idle or done.

```sh
fledge task template > brief.md                               # fill in, then create
fledge task create --title "Fix the parser" --file brief.md   # prints the task ID
fledge task create --title "Add tests" --file tests.md --parent 1a2b3c4d --after 5e6f7a8b
fledge task create --title "Try a probe" --body "one line" --freeform
fledge task import --file .fledge/tmp/plans/5d6ab497.toml --dry-run   # see Proposals
fledge task depend --id 9c0d1e2f --after 1a2b3c4d --remove 5e6f7a8b
fledge task list --ready                                        # created and unblocked
fledge task assign --id 1a2b3c4d --name worker
fledge task complete --id 1a2b3c4d --summary "Fixed in abc123"  # run by the owner
fledge task verify --id 1a2b3c4d --summary "Tests pass"         # run by another agent
fledge task list --status completed
fledge task get --id 1a2b3c4d
```

### Briefs

`create` requires the brief to follow the **brief template**: six `## `
headings, exactly once each and in this order, each followed by content:

- `## Objective` – the outcome wanted, in one paragraph.
- `## Acceptance criteria` – concrete, testable checks that decide completion.
- `## Scope` – allowed files or areas, and what is explicitly out.
- `## Known facts` – established facts the worker would otherwise rediscover,
  with `file:line` where known.
- `## Deliverables` – the return contract: evidence to report, findings or
  decisions to return, what may remain undone.
- `## Constraints` – rules: authorization, commit policy, who to report to,
  what to escalate.

Headings match exactly (case-sensitive, trailing whitespace ignored). Text
before the first heading and `###` sub-headings inside a section are allowed;
any other `## ` heading is an unknown section. A section holding only blank
lines or HTML comments on their own lines (`<!-- hint -->`) is empty. A brief
off the template fails with `task_brief_incomplete` (exit 1, phase
`validation`) naming the missing, empty, duplicated, unknown, or misordered
section, before any state is created. `--freeform` skips the check and stores
the brief as given. Existing records are never re-checked, and `assign`
delivers the brief text unchanged.

Run `fledge task template` for the brief skeleton and fill it in, rather than
writing the headings from memory: the command is the source of truth for the
template. It prints the six headings, each followed by an HTML comment hint,
so an unfilled skeleton fails `create` with `task_brief_incomplete`.
`--proposal` prints a proposal skeleton instead: `schema_version`, a `[parent]`,
and one example `[[tasks]]` entry, each brief being the brief skeleton.
`--json` returns `kind` (`brief` or `proposal`) and `text` in the outcome
envelope. `task template` never contacts Herdr or reads task state, so it
works outside a repository.

### Lifecycle

A task moves `created` → `assigned` → `completed` → `verified`; `task cancel
[--reason TEXT]` ends a `created`, `assigned`, or `completed` task as
`cancelled`. A `created` or `assigned` parent can also go straight to
`verified` once its subtasks are finished (see `verify` below). A verified task
can only be verified again, cancelled tasks are final, and other out-of-order
transitions fail with `task_invalid_state`. There are no progress updates within
a task and no recorded checks.

A task can be a **subtask** of a parent and can run **after** prerequisite tasks.
The two are independent: a parent keeps its own lifecycle and does not wait for
its subtasks, and a subtask is not a prerequisite of its parent.

- **Subtasks.** `create --parent TASK` fixes the parent at creation; it cannot
  change later. The parent must exist and be neither verified nor cancelled
  (`task_not_found` or `task_invalid_state`); nesting depth is unlimited and
  cycles cannot form because a parent always predates its subtasks. `list` and
  `get` show the progress of direct subtasks as `2/3 verified`, adding
  `, 1 cancelled` when any were cancelled: cancelled subtasks are shown but not
  counted toward the total. `verify` refuses a parent whose direct subtasks are
  not all verified or cancelled with `task_open_subtasks`; `--force` verifies
  a completed parent anyway, records `forced: true`, and lists the open
  subtasks. Cancelling a
  parent leaves its subtasks unchanged.
- **Dependencies.** A prerequisite is satisfied once it is `verified`. A
  `cancelled` prerequisite also counts as satisfied but stays on its dependents
  and is shown with its state and reason, never dropped. Declare prerequisites
  with repeatable `create --after TASK`, or change them later with
  `depend --id TASK --after PREREQ --remove PREREQ` (both repeatable; removals
  apply first; adding a present prerequisite or removing an absent one changes
  nothing). `depend` refuses a verified or cancelled task (`task_invalid_state`),
  an unknown prerequisite (`task_not_found`), and any addition that would form
  a cycle, including a task after itself (`task_dependency_cycle`, naming the
  chain). `assign` refuses a task with unmet prerequisites with
  `task_dependencies_unmet`, naming them, before it contacts the agent;
  `--force` assigns it anyway and records the unmet prerequisites as
  `unmet_at_assign` (null otherwise), separately from verify's `forced`.
  `cancel` names the created tasks it left ready. These checks and the changes
  they guard run under the state store lock, so concurrent commands cannot
  form a cycle or lose an edit.

- `create` requires a single-line `--title` and a brief (`--body`, or `--file`
  with `-` for stdin), and takes optional `--parent` and repeatable `--after`.
  `created_by` is the caller's agent record, or null when the caller is
  unregistered.
- `assign --id TASK` takes exactly one of `--name`, `--pane`, or `--agent-id`.
  The agent must be registered; otherwise it fails with `agent_unregistered` and
  suggests `fledge agent adopt`. A `created` or `assigned` task is first stored
  as assigned to that agent, then the brief is submitted exactly like
  `agent message`: the sender header, then
  `task: <id> · title: <title> · complete with: fledge task complete --id <id> --summary "..."`,
  then the brief. The delivery (`message_id`, `pane`, `delivered_at`, `error`,
  `uncertain`) is recorded in a second step. A failed delivery leaves the task
  assigned with `delivery.error` set and a `partial` outcome. When Herdr cannot
  confirm whether the brief arrived, `delivery.uncertain` is true, the outcome
  is `unknown`, and the output says the delivery outcome is unknown. Deliveries
  are never retried. If the task
  changes between the lookup and the locked update, for example because another
  caller assigned it first, assign fails with `task_state_changed`.
- `complete --id TASK` (`--summary` or `--file`) requires an `assigned` task and
  a caller whose live agent record is the owner (`task_not_owner` otherwise);
  `--force` overrides the owner check. After recording completion, it sends the
  task ID, title, result, and verification command to the distinct registered
  creator. An unregistered creator or a creator completing its own task needs no
  notification. A stale creator or confirmed delivery failure returns `partial`;
  an uncertain delivery returns `unknown`. The task remains completed, the
  notification outcome is recorded, and delivery is never retried automatically.
- `verify --id TASK [--summary TEXT]` requires a `completed` or `verified` task
  and a registered caller other than the owner. The owner is refused with
  `task_self_verification` and an unregistered caller with
  `caller_unregistered`; `--force` overrides both and records `forced: true`.
  This is a workflow guard, not a security boundary. Verifying an
  already-verified task, for example after repairs, applies the same checks and
  replaces `verifier` (null for an unregistered `--force`), `verification_note`
  (null when omitted), `forced`, and `verified_at`; only the latest
  verification is kept, and the result, completion, and owner are unchanged.
  Verification is not tied to a Git revision, so name the checked commit in
  `--summary` when it matters.
  A `created` or `assigned` task that groups subtasks can be verified without
  being completed once every direct subtask is `verified` or `cancelled` and at
  least one is `verified`, with the same caller checks; its owner, if any, is
  unchanged and not notified. Such a task with open subtasks is refused with
  `task_open_subtasks`, naming them; one whose subtasks were all cancelled, or
  that has no subtasks, is refused with `task_invalid_state` (cancel the
  former instead). `--force` never overrides these state checks.
- `list [--status STATE] [--owner AGENT_ID] [--parent TASK] [--ready]` prints
  ID, status, owner (the agent's name while its record is live, otherwise its
  ID), parent, waiting (unmet prerequisites), subtask progress, and title,
  oldest first. `--parent` keeps direct subtasks only; `--ready` keeps created
  tasks whose prerequisites are all satisfied. Filters combine. `get --id TASK`
  prints the full record with its parent, subtask progress, and each
  prerequisite's state, for example
  `after: 1a2b3c4d (verified), 5e6f7a8b (cancelled: superseded), 9c0d1e2f (assigned, waiting)`.

Each record holds `id`, `title`, `brief`, `parent`, `after`, `owner`, `status`,
`result`, `verifier`, `verification_note`, `forced`, `cancel_reason`,
`created_at`, `created_by`, `assigned_at`, `unmet_at_assign`, `completed_at`,
`completion_notification`, `verified_at`, `cancelled_at`, and `delivery`.
Records written before subtasks and dependencies load with a null `parent`,
`after`, and `unmet_at_assign`. A completion notification records
its `recipient`, `message_id`, optional `pane`, `delivered_at`, `error`, and
`uncertain` state. Every command supports `--json` with the same outcome envelope
as the agent commands. JSON results add derived fields: list rows `progress`
(`verified`, `total`, `cancelled`, or null) and `waiting`; get `progress` and
`dependencies` (`id`, `title`, `status`, `cancel_reason`, `satisfied`); verify
`open_subtasks`; cancel `unblocked`; and template `kind` and `text`. `task
list`, `task get`, `task depend`, and `task template` never contact Herdr.

### Proposals

A **proposal** is a TOML file describing several tasks at once, typically
written by a planner and reviewed before dispatch. `fledge task template
--proposal` prints a skeleton.

```toml
schema_version = 1

[parent]                     # optional: the task the others are created under
title = "Task brief templates and decomposition"
brief = '''
## Objective
...
'''

[[tasks]]
key = "brief-lib"            # names the task within this file
title = "Add brief template validation"
brief = '''
## Objective
...
'''
after = []                   # local keys or existing 8-hex task ids

[[tasks]]
key = "import"
title = "Add task import"
brief = '''...'''
after = ["brief-lib"]
```

`schema_version` must be `1`, and unknown keys anywhere are rejected. There
must be at least one `[[tasks]]` entry. Each `key` is nonempty, single-line,
unique, and never an 8-hex task id; each `title` is nonempty and single-line;
every brief, including the parent's, must follow the brief template (there is
no freeform import); and each `after` entry names a key in the file or an
existing task id. Local dependencies must not form a cycle.

```sh
fledge task import --file plan.toml --dry-run            # validate and preview
fledge task import --file plan.toml                      # create the tasks
fledge task import --file - --parent 1a2b3c4d < plan.toml
```

`task import --file PATH` (`-` for stdin) validates the whole file before it
opens the state store. Then, under one store lock, it creates the `[parent]`
(if any) and the tasks in dependency order (file order where dependencies
allow), resolving each `after` key to the new task's id and setting every task's
`parent`. `--parent TASK` places the tasks under an existing task instead; it
must be neither verified nor cancelled, and it conflicts with a `[parent]` in
the file. `created_by` is the caller, as for `create`. Import never assigns.

`--dry-run` runs the same checks, including that `--parent` and any existing
`after` ids exist, and prints the tasks in creation order without creating
anything:

```text
Would create 2 tasks under 1a2b3c4d:
  brief-lib  Add brief template validation
  import     Add task import  (after: brief-lib)
```

A real run prints `Created task 9c0d1e2f (brief-lib): Add brief template
validation` per task, preceded by the created parent's own line and followed by
`Created 2 tasks under parent <id>` when the file had a `[parent]`. The JSON
result is `{"parent": <id or null>, "tasks": [{"key", "id", "title", "after"}],
"dry_run": <bool>}`; `id` is null on a dry run, and `after` holds the file's
entries on a dry run and resolved ids otherwise. Effects list one `created
task <id>` per record.

A file or schema problem, a `[parent]` with `--parent`, or a missing `--file`
is invalid input (exit 2, phase `validation`); a brief off the template fails
with `task_brief_incomplete` (exit 1) naming the task key, as in
`task "import": ...`. An unknown `--parent` or `after` id fails with
`task_not_found`, and a verified or cancelled `--parent` with
`task_invalid_state` (phase `task`); none of these create anything. The store
keeps one file per record, so a failure partway through a real import leaves
the earlier records in place; the outcome is then `partial` and its effects
name them. A dry run never contacts Herdr.

By convention a planner writes its proposal to
`.fledge/tmp/plans/<task-id>.toml`, named for the planning task it was
assigned. `.fledge/tmp/` is the agents' scratch directory and is already
ignored; no Fledge command creates it, and `task import` accepts any path.

## Worktrees

Worktree commands run inside Herdr like agent commands, use the same `--json`
outcome envelope, and act on the repository containing `--cwd` (default: the
current directory, which may be a linked checkout).

```sh
fledge worktree list
fledge worktree list --cwd ~/src/project --json
fledge worktree create --branch feature/task --base dev
fledge worktree remove --branch feature/task
fledge worktree remove --path .fledge/worktrees/feature/task --force --json
```

`list` shows every checkout, primary first, with its branch (or detached), the
open Herdr workspace, whether it is dirty (including untracked files), whether it
is merged, and whether it is managed under `.fledge/worktrees`. Merged means the
branch head (or detached HEAD) is an ancestor of the
[integration branch](#integration-branch). Squash-merged branches therefore count
as unmerged. Either check reports `unknown` when git cannot answer. The `OWNER`
column names the live registered agent whose spawn created or opened that
checkout as `name (id)`, with `+N` when N more live agents share it, or `-`.
JSON rows carry `owner` (`{id, name, pane}` of the earliest registered such
agent, or null) and `owner_count`. Records of agents no longer in Herdr do not
count, and a repository without a state store shows no owners. Owners are best
effort: when Herdr's agent list is unavailable, or any of its entries lacks a
required field such as its terminal ID, every row shows no owner.

`create` makes a managed checkout at `.fledge/worktrees/<branch>` on a new branch
and opens it as a workspace without starting an agent, then writes the checkout's
managed [`.fledge/.gitignore`](#profiles). An existing branch is refused.

`remove` takes exactly one of `--path` or `--branch` and keeps the branch. An open
checkout is removed through Herdr, which also closes its workspace; a closed one
through `git worktree remove`. Its guards:

- The primary checkout is never removed.
- A checkout in use by a live agent is refused, checked again immediately before
  removal, whether or not the checkout is open as a workspace. An agent uses it
  when its pane is in the checkout's workspace, when its working directory is the
  checkout or inside it (from any workspace, so `fledge agent spawn --cwd
  .fledge/worktrees/feat` in another tab counts), or when it is a registered
  agent whose recorded worktree is the checkout or inside it. Paths are compared after
  resolving symlinks, and a sibling such as `feature` is not inside `feat`.
  `--force` does not override this. The guard sees only agents in the connected
  Herdr session; other sessions and direct Herdr actions are outside it. Records
  of agents no longer in Herdr do not count, but an unreadable state store
  refuses removal until the bad record under `.fledge/state` is repaired or
  removed. An agent list with any entry missing a required field, such as its
  terminal ID, refuses removal with `protocol_error`.
- A dirty or unmerged checkout, or one where either check is `unknown`, is
  refused unless `--force` is passed. Unmerged is judged against the
  [integration branch](#integration-branch).

### Integration branch

`worktree list` (the `MERGED` column) and the `worktree remove` merged guard
compare each checkout against one integration branch per repository, chosen in
this order (`agent cleanup` instead uses the base each checkout was created
from; see [Cleanup](#cleanup)):

1. The local branch named by the repository's git config `fledge.baseBranch`,
   resolved as `refs/heads/<name>`.
2. Otherwise, the branch `origin/HEAD` points to (usually the remote default,
   such as `origin/main`).
3. Otherwise, the local `main` branch.
4. Otherwise, none: every checkout is merged `unknown`.

Set it when work integrates into a branch other than the remote default. Fledge
itself develops on `dev` and reaches `main` only through pull requests, so its
checkouts set:

```sh
git config fledge.baseBranch dev
```

The setting is ordinary git config, read like `git config fledge.baseBranch`
from every scope: the repository value, shared by every linked checkout, wins,
but a `git config --global fledge.baseBranch dev` applies to every repository
that does not set its own, including one that lacks that branch, where it gives
the `does not exist` unknown described below rather than a fallback. Unset it
with `git config --unset fledge.baseBranch` (add `--global` for the global
value). If git cannot read its config, merged is `unknown` with git's error as
the reason. Give a local branch name
(`dev`, not `origin/dev` or `refs/heads/dev`). If it names a branch that does not
exist, Fledge does not fall back to `origin/HEAD` or `main`: every checkout is
merged `unknown`, `worktree list` ends with a `MERGED is unknown: ...` line
giving the reason (JSON: `default_branch` is null and `default_branch_error`
holds the reason), and `worktree remove` refuses without `--force`, naming the
reason. JSON `default_branch` is the chosen branch's short name, such as `dev`
or `origin/main`.

## Doctor

`fledge doctor` diagnoses the local Fledge and Herdr environment with read-only
checks and prints a per-check report. It never mutates Herdr state, writes files,
or reaches the network.

```sh
fledge doctor
fledge doctor --verbose
fledge doctor --json
```

It runs five checks, each independently:

- `herdr_connectivity` — `ping` reaches the Herdr socket (`fail` if not).
- `herdr_compatibility` — the pong `protocol` matches the pinned protocol (`22`);
  a mismatch is `warn`, not `fail`, and the detail reports `version`, `protocol`,
  and capabilities.
- `harness_installations` — the integration targets Herdr knows about with their
  `command`, `available`, and `state`; `warn` when none are available.
- `model_discovery` — local model discovery gated on availability, restricted to
  the harness kinds whose models come from local cache files (`pi`, `codex`,
  `claude`). To stay strictly read-only, doctor never executes a harness command,
  so command-only kinds (`opencode`, `cursor`) are not checked here; use `fledge
  agent models` for those. A missing harness is `warn` (nothing to diagnose); an
  installed harness whose cache is unreadable is `fail`; an installed harness with
  no models, including one that has not created its cache yet, is `warn`.
- `configuration` — `HERDR_ENV`, `HERDR_SOCKET_PATH` and its file mode,
  `HERDR_PANE_ID`, `HERDR_SESSION`, and the working directory.

Each check reports exactly one status: `ok`, `warn`, or `fail`. Human output is a
grouped report: one block per check with the padded check name and its status on
the first line, its detail indented beneath, a blank line between checks, and a
`N ok · N warn · N fail` summary at the end. In human output only, a path under
your home directory is shown with a leading `~` (for example
`~/.config/herdr/herdr.sock`); `--json` keeps absolute paths.

By default the long detail is hidden to keep the report scannable:

- `herdr_compatibility` shows `version … · protocol … (expected …)`.
- `harness_installations` shows `N targets, M available`.
- `model_discovery` shows the short per-harness counts (`pi N · codex N · claude N`).
- `configuration` always shows its labeled `HERDR_ENV` / `socket` / `pane` /
  `session` / `cwd` block with an aligned key column.

`--verbose` reveals the long detail: `herdr_compatibility` adds a `capabilities:`
line (also shown on a compatibility `warn`/`fail` when the capabilities are
known) and `harness_installations` adds an `available:` line listing the target
names. These lists wrap at their `, ` separators to stay within 80 columns, with
continuation lines indented and tokens never split. `warn` and `fail` checks
always show their diagnostic detail, with or without `--verbose`, because it is
actionable. `--verbose` never affects `--json`.

`--json` emits one document with stable per-check names, per-check `data`, and a
summary. The exit code is `1` if any check is `fail`, else `0`; warnings never
fail. Running outside Herdr (no `HERDR_ENV=1` or no reachable socket) is reported
as a `fail`, not a crash.

## Layout

- `main.go` delegates to `cmd.Execute()` and handles the exit status.
- `cmd/` constructs fresh command trees with `NewRootCmd()` and provides
  `ExecuteWithArgs()` for tests.
- `cmd/<name>/` and `cmd/<parent>/<subcommand>/` contain thin Cobra wiring;
  `internal/` mirrors that command nesting as `internal/<name>/` and
  `internal/<parent>/<subcommand>/`.
- Each internal command leaf owns its options, orchestration, result types,
  human rendering, and tests. Internal parent packages may coordinate nested
  components, as `doctor` does with `checks`/`report` and `update` with
  `release`/`archive`/`install`/`confirm`; child packages do not import their
  parents.
- `internal/lib/<capability>/` contains focused shared code used by multiple
  commands or packages; do not create one flat grab-bag lib package and do not
  extract speculative utilities.
- `internal/lib/version` reports the release tag or Go build metadata through
  `--version` and `-V`, without a maintained version file.
- `internal/lib/harness` is the single source of per-harness facts: the kinds,
  one typed profile per kind that `agent pause`, `agent spawn`, and `agent models`
  read, and the fixed capability rows derived from it. It imports nothing from Fledge.
- `internal/lib/usage` reads a harness session's measured tokens and
  harness-recorded cost estimate from claude, codex, and pi session files and
  `opencode export`, filtered to a time window; missing or unreadable data is
  `unavailable` with a reason. Windows attribute by time on one session, so
  concurrent tasks or human chat in the same pane double-count.

New subcommands export `New() *cobra.Command` and are registered by their parent.
Internal packages do not import Cobra or `cmd/`.

## Development

Check formatting from the repository root using Bash. This includes tracked and
new nonignored Go files, skips deleted files, and excludes ignored worktrees:

```bash
set -euo pipefail
git ls-files --cached --others --exclude-standard --deduplicate -z -- '*.go' |
  (
    status=0
    while IFS= read -r -d '' file; do
      [[ -f "$file" ]] || continue
      unformatted="$(gofmt -l -- "$file")" || exit "$?"
      if [[ -n "$unformatted" ]]; then
        printf '%s\n' "$unformatted" >&2
        status=1
      fi
    done
    exit "$status"
  )
```

Then run:

```sh
go vet ./...
go test -race ./...
go build -o /tmp/fledge .
```

The GitHub workflows lint, test, and build for Linux amd64 and arm64. Release
workflow and script changes also trigger these checks. Test release automation
locally without contacting GitHub:

```sh
python3 -B -m unittest discover -s .github/scripts -p '*_test.py'
bash .github/scripts/package.sh v0.0.0 /tmp/fledge-package-smoke
```

The packaging command uses a synthetic version for a local smoke test only; it
neither creates tags nor publishes releases.

## License

[MIT](LICENSE).
