# Agents

[README](../../README.md)

Run agent commands inside Herdr, with `HERDR_ENV=1` and `HERDR_SOCKET_PATH` set.
Fledge talks directly to that session's Unix socket.

```sh
fledge agent spawn --name reviewer --harness claude --model sonnet
fledge agent spawn --name builder --harness codex --workspace backend --tab builds
fledge agent spawn --name task --harness codex --workspace backend --worktree new --branch feature/task
fledge agent spawn --name existing --harness claude --pane w2:p3
fledge agent spawn --name review-1 --harness pi --model openai-codex/gpt-6-astra --profile reviewer --file brief.md
fledge agent profiles
fledge agent profiles reviewer --json
fledge agent adopt --name helper
fledge agent adopt --name builder --json
fledge agent rename --to orchestrator
fledge agent rename --name builder --to reviewer --json
fledge agent list --json
fledge agent list --mine
fledge agent list --parent 3f9a0c2e --json
fledge agent list --state idle --state blocked --harness codex
fledge agent list --registered --profile reviewer --ids
fledge agent list --registered --profile reviewer --ids | xargs -I{} fledge agent message --id {} --body 'Rebase on dev'
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
fledge agent usage --name reviewer
fledge agent usage --mine --json
```

Spawn requires a unique live `--name` and a `--harness`.
A [profile](#profiles) never supplies either.
Every spawn that does not reuse a pane creates a **new tab**, by default in the caller's workspace.
`--tab NAME` is only that new tab's label (default: the agent name).

It never selects an existing tab, and labels need not be unique, so two spawns with the same `--tab` get two tabs.
Workspace names match exactly and case-sensitively.
Fledge creates a workspace for a missing `--workspace` name. An ambiguous name fails.
`--workspace-id` selects an existing workspace by ID.
A newly created workspace (or newly opened worktree workspace) uses its initial tab, labeled as above.

An existing workspace receives a new tab from `tab.create`.
`--pane` is the only way to reuse an existing shell, and it excludes `--workspace`, `--workspace-id`, `--tab`, `--worktree`, `--cwd`, and `--env`.
`--label` and `--focus` still apply to that pane.
The pane label defaults to `--name`.

Spawn has no interactive prompts, wizard, or pane splits.
It no longer accepts `--tab-id`, `--direction`, or `--ratio` (see [Migrating from earlier versions](migration.md#migrating-from-earlier-versions)).
Fledge rejects those spellings as unknown flags before any Herdr call.
Fledge passes those tokens after `--` to the harness as native arguments.

Ordinary spawn honors Herdr's configured cwd policy unless you supply `--cwd`.
A relative `--cwd` resolves against the caller's working directory.
Fledge uses an absolute path as given.
`--cwd` only places the shell (and selects the source for `--worktree new`).

Fledge registers the agent in the invoking repository (see [Identity](#identity)).
Use repeatable `--env KEY=VALUE` for new ordinary shells.
`--focus` defaults to false and focuses the destination before start.
`--timeout` is a duration, default `30s`.

Its millisecond value must be greater than 3000 and at most 300000.

By default, spawn waits for startup to settle before returning.
Success reports the settled status, such as `idle`, rather than `unknown`.
Settled means ready for input.
Herdr must report `interactive_ready`, no pending startup, and `idle` or `done`.

A lifecycle status alone does not suffice: pi can report `idle` seconds before accepting a prompt.
Spawn polls for readiness whether or not you supply a first prompt.
If a different terminal, name, or harness answers, it fails `unknown` without registration or prompting.

`--timeout` covers startup through the first prompt.
Its deadline bounds Herdr requests for startup, lifecycle waits, readiness polls, registration, sender lookup, and prompt submission.
Fledge rejects replies that arrive after the deadline.
The local registration step takes the state lock and writes the record.

Fledge cannot interrupt this step.
Spawn can overrun `--timeout` while another Fledge process holds the lock.
Fledge sends no first prompt after the deadline either way.
If the budget expires before prompt submission, spawn exits `partial` with `timeout`.
It reports that it did not submit the prompt.

If the budget expires during submission, the outcome is `unknown`.
Inspect with `fledge agent read --pane P` before resending.
A spawn that times out before the agent is ready does not register it.
Only that agent can register itself later, by running `fledge agent adopt` in its own pane; another pane cannot register it.
Fledge keeps and reports any record it already wrote.
A registration interrupted by the deadline reports `registration_error`.
Herdr reserves the startup name for the larger of `--timeout` and 30s.

Herdr drops the name if that reservation expires during startup.
A short `--timeout` bounds only spawn itself.
After a `partial` timeout, startup can still finish under its name.

A start unfinished when the reservation expires can still lose its name.
Address it by pane.
`--no-wait` restores the old behavior: return once the start begins, without waiting for readiness.
If the agent settles on `blocked` (its own startup dialog, e.g.

An update prompt), spawn fails with `agent_blocked` and a `partial` outcome (exit 1).
Fledge leaves the agent running and keeps its record.
A wait timeout is also `partial`, like a startup timeout.
Human output then gives recovery hints addressed by pane, since the name may not resolve: `fledge agent read --pane P` to inspect the dialog, then `fledge agent send --pane P --key <key>` to answer it.

Spawn never answers a dialog itself.

Pass `--prompt TEXT` or `--file PATH|-` (mutually exclusive, unlike `message`, inline text on spawn is `--prompt`, not `--body`) to deliver a first prompt once the agent is ready.
Fledge reads and checks the prompt before changing Herdr state.
A missing file never leaves a tab or agent behind.
Spawn does not wait for the prompted turn to finish.
`--no-wait` cannot be combined with `--prompt` or `--file`, since there would be no settled agent to prompt.

`prompt_requested` reports whether you supplied a first prompt.
This distinguishes "none requested" from "not submitted" when `prompted=false`.
A spawn that stops before prompting (blocked, timed out, or unconfirmed startup) says the first prompt was not submitted.
It is not queued or replayed, so resend it after resolving the dialog with `fledge agent message --pane P --file <brief>` (or `--body`).
An uncertain `agent.prompt` failure makes no such claim, since the prompt may have arrived.

With `--worktree new`, workspace selectors identify an **existing source** repository workspace.
That source takes precedence over `--cwd`.
Without either, the source is Fledge's working directory.
`--branch` defaults to the agent name, and `--base` selects the starting ref.

Without `--base`, spawn passes the branch checked out in the primary checkout as the base (even when invoked from a linked worktree).
With a detached primary checkout it passes none and Herdr chooses.
Fledge creates checkouts at `.fledge/worktrees/<branch>` beneath the primary checkout, even when you invoke it from a linked worktree.
Branch slashes create nested directories.

Existing branches or paths fail.
Fledge does not invent suffixes.
`.fledge/.gitignore` excludes the managed directory, except [profile](#profiles) files, without changing the root ignore file.

`--worktree PATH` opens an existing checkout.
Without an explicit source, the absolute checkout path determines its repository.
A newly opened workspace uses its initial pane.
An already-open workspace receives a new tab.

`--tab NAME` labels that tab.
Every checkout gets its own workspace.
`--workspace` and `--workspace-id` never place a worktree agent, they only select the source.
Worktree shells use the checkout directory.

Worktree mode excludes `--env` and `--pane`.
`--branch` and `--base` apply only to creation.

Fledge accepts all 24 documented Herdr harness kinds.
Fledge's optional `--model` translates to verified native flags.
It is currently unavailable for `kiro`, `amp`, `muse`, `mastracode`, and `qodercli`.
Those harnesses can still be started and receive native arguments.

Model values remain opaque.
Hermes uses its `chat` subcommand.
Fledge rejects documented native model conflicts when you set `--model`.
Harness resume behavior can still affect model selection.

Codex spawns default to `--yolo`, disabling approval prompts and sandboxing.
Claude spawns default to `--permission-mode bypassPermissions`.
These defaults apply with or without a profile.
Other harnesses receive no permission defaults.

Codex spawns also default to `--no-daemon`, with or without a profile.
Codex's shared app-server daemon runs every tool command with the environment of the pane that first started the daemon, so a worker's Fledge commands would identify another pane.
With `--no-daemon`, tool commands run under the worker's own Codex process and get its pane's `HERDR_*` values.
Spawn does not add a second `--no-daemon` when the native arguments already contain it, and `--no-permission-bypass` does not remove it.
There is no opt-out.

Explicit native permission choices override these defaults:

- Claude: `--permission-mode` or `--dangerously-skip-permissions`.
- Codex: bypass, sandbox, approval, `--approve-for-me`, or `--full-auto` options.
- Codex configuration through `-c`/`--config`: `approval_policy`, `sandbox_mode`, `default_permissions`, or `permissions` and its nested keys.
  Use `--no-permission-bypass` to suppress injection and use harness configuration or native arguments instead.
  It does not remove explicit bypass arguments or force a restricted mode.
  Fledge does not edit harness settings or answer startup confirmation dialogs.

Pass native tokens with repeatable `--args=TOKEN`, then optional trailing `-- TOKEN...`.
Tokens retain whitespace and commas.
Fledge does not parse a shell command string.
For example:

```sh
fledge agent spawn --name reviewer --harness claude --args=--permission-mode --args=plan
```

List includes unnamed agents and agents started outside Fledge.
Message takes one or more targets and exactly one of `--body`/`--file`.
It preserves newlines and rejects empty or invalid UTF-8 content.
Success acknowledges **submission**, without waiting for the agent to begin or finish.
A registered recipient (one with a Fledge record) gets the body stored in its [inbox](#inbox) and only a one-line doorbell typed into its pane.
An unregistered recipient gets the sender header and the body typed directly, with `inbox_id: null` and `state: "direct"`, and human output says so.

Blocked agents require the user to handle their approval dialog.

Targets use repeatable `--name`, `--pane`, and `--id` flags, which you may mix.
Alternatively, use the `agent list` filters: `--state`, `--harness`, `--profile`, `--task`, `--worktree`, `--registered`, `--mine`, and `--parent`.
Do not combine explicit targets with filters.
Filter flags AND together and repeating one ORs its values.

Matches never include the caller, and an empty match fails with `no_agents_matched`.
An explicit target naming the caller is still messaged.

Fledge rejects duplicate targets.
One resolved target gives the single result described here.
Several targets are all resolved first, so a lookup failure is `rejected` with nothing sent.
Then each is delivered in turn, with `--confirm` and `--timeout` applied to each.
Each registered target gets its own inbox record with its own id.
Unregistered targets share one sender header and message ID.

The result is `{"mode": "fan-out", "targets": [...]}`.
Each target has one row, in target order, with `target`, `outcome`, `message_id`, `inbox_id`, `state`, `agent`, and `error`.
`target` holds the explicit flag value, a filter match's record ID, or its pane if unregistered.
For a registered target, `message_id` and `inbox_id` are the row's own inbox record id, and `state` is `queued` or `rung`.
For an unregistered target, `inbox_id` is `null`, `state` is `direct`, and `message_id` is the shared header id.
All three are `null` when nothing was stored or sent.

Outcomes are `submitted`, `confirmed`, `already_working`, `unconfirmed` (submitted, but activity was not acknowledged), `unknown` (may have been submitted), `deferred` (stored, but the doorbell did not ring), and `rejected` (nothing stored or sent).
For a registered recipient, `submitted` and its neighbors describe the doorbell.
`rejected` is only for validation and target-resolution failures.
Delivery continues past failures.
Any failed row makes the outcome `partial` with exit status 1, and human output prints one line per target.

For example, to tell every worker on a task:

```sh
fledge agent message --task 3f9a0c2e --body 'The API now returns 404 for missing tasks'
```

`--confirm` also waits, up to `--timeout` (default 10s, valid only with `--confirm`), for Herdr to observe the agent become active after submission.
A reply of `working`, or a quickly finished `done`/`idle`, reports `confirmed: true`.
An agent already working still receives the message.
The result reports `already_working: true` and `confirmed: false`.

Herdr tracks lifecycle state, not individual prompts, so it cannot acknowledge this prompt's start.
An agent that turns `blocked` after submission, or Herdr's `agent_prompt_stalled` (no activity seen within five seconds), is `partial` with a `submitted` effect.

A wait `timeout` or lost acknowledgement is `unknown`.
This command does not retry these cases.
The message was, or may have been, delivered, so read the pane instead of resending.
For an unregistered recipient, refusals made before input (`agent_blocked`, `agent_not_ready`) stay `rejected`.
For a registered recipient, any doorbell failure leaves the record stored: the outcome is `partial` with `deferred`, or `unknown` when the doorbell may have rung.
The command then starts the [supervision daemon](#supervision) if none runs, and the daemon rings the doorbell when the agent is ready.

Every prompt Fledge types with a body, from `message` to an unregistered recipient or from spawn's `--prompt`/`--file`, starts with one header line naming the sender and a correlation ID, e.g.
`ᛉ fledge message from reviewer (w1:p2) · id m-0a1b2c · reply: fledge agent message --name reviewer`.
Herdr resolves the sender from the caller's `HERDR_PANE_ID`.
An unnamed agent appears as `unnamed agent (PANE)`.

A pane without an agent appears as `pane PANE`.
An unresolvable caller appears as `unknown sender`.
Only named agents get the reply command.

Attribution failures never block delivery.
JSON results include `message_id`, `inbox_id`, `state`, and `sender` (`name`, `pane`, `kind`, `error`).

`fledge agent send` types raw input into a live agent's terminal with **no** sender header, so the recipient sees no sender and has no reply channel.
Use it for input a harness must see verbatim, such as a slash command, or to answer a dialog that blocks an agent.
It takes exactly one of `--name`, `--pane`, or `--id`, and at least one of `--text` and a repeatable `--key`.
Send pastes the text first and does **not** press Enter.

Send types any newline inside the text literally.
Send presses keys (`enter`, `esc`, `down`, `ctrl+c`, ...) after the text, in order.
Add `--key enter` to submit.
Both go to the resolved pane in one Herdr `pane.send_input` call.
Herdr checks every key name before writing.
An unknown key rejects the send with `invalid_key`, without typing anything.

Send works in any agent state (`idle`, `working`, `blocked`, `done`, or `unknown`) without `--force`.
The output reports the status observed before sending, for example `Sent input to reviewer (claude) in w2:p3; it was blocked before sending.` JSON uses operation `agent.send`, the standard agent fields (with that earlier status), and `submitted`.
A lost acknowledgement is `unknown`.
Sending does not wait for or check the effect.

Read the pane with `fledge agent read` to check it.
Claude Code's `/model` also saves the chosen model as the global default for new Claude sessions (it rewrites `~/.claude/settings.json`), not only for the target agent.

`fledge agent get` inspects one live agent with exactly one nonempty `--name` or `--pane` target and no positional arguments.
It makes a single read request, without focusing the pane or marking output seen.
Labeled text includes the list fields, foreground working directory, interactive readiness, pending startup, and focus state.
It also includes the resolved title and native session source, harness, reference kind, and value.

Title selection uses the agent-reported title, then the stripped terminal title, then the raw terminal title.
Unavailable values appear as `-`.

Known booleans appear as `true` or `false`.
JSON returns a single result object with these details under `foreground_cwd`, `interactive_ready`, `launch_pending`, `focused`, `title`, and `agent_session` (with `source`, `harness`, `kind`, and `value`).
Unavailable details are `null`.
`interactive_ready` and `launch_pending` are optional and appear as `null` when absent, while `focused` is always present on a successful read.

`effects` is empty.
Failed reads use `rejected` with the error code and phase.

`fledge agent read` prints a plain-text terminal snapshot of one live agent's pane, selected by exactly one of `--name`/`--pane`.
It resolves the agent, then reads that pane without focusing it or marking output seen.
`--source` is `visible`, `recent`, `recent-unwrapped` (default, soft wraps joined), or `detection`.
`--lines N` requests the bottom N rows (0 to 4294967295).

Herdr returns at most 1000 rows, and `--lines 0` returns an empty, truncated snapshot.
Text output is a label, `Terminal snapshot of <name or pane> (<source>, <N> rows, truncated: yes|no)`, followed by the text.
Human output ends with a newline even when the snapshot does not.
`--json` preserves the bytes exactly.

JSON adds `source`, `lines` (rows returned), `text`, `revision`, and `truncated` to the standard agent fields.
A snapshot is the terminal's current contents, not a conversation transcript.

`fledge agent wait` blocks until agents reach a lifecycle state.
Without `--until`, it matches `idle`, `done`, or `blocked`.
Repeat `--until` to choose states from `idle`, `working`, `blocked`, `done`, and `unknown`.
Without `--timeout`, it waits indefinitely with no transport deadline.

Ctrl-C cancels it.
Fledge passes a finite `--timeout` to Herdr.
Expiry fails with `timeout`.
A settled state means the agent's turn ended, **not** that its assigned work succeeded.
`--name`, `--pane`, and `--id` are repeatable and may be mixed.

Fledge rejects duplicate targets.
An `--id` target follows its record's terminal and fails closed with `agent_identity_stale` if a different terminal answers.
Instead of explicit targets, the filter flags `--state`, `--harness`, `--profile`, `--task`, `--worktree`, `--registered`, `--mine`, and `--parent` select live agents (mixing them with explicit targets is rejected).
Filters AND together, repeated values of one flag OR together, and the caller is never selected.

An empty match fails with `no_agents_matched`.
Fledge reports filter matches by record ID, or by pane when unregistered.
Registered matches get the same terminal check as `--id`.
One target, explicit or matched, prints `<name> is <status>.` and its JSON result is the agent row.
Two or more targets, including several filter matches, need `--all` or `--any`, and each target gets its own Herdr wait.

`--all` waits for every target.
The first target that fails ends the wait with that failure and cancels the rest.
Near-simultaneous failures may report `operation_failed`.
A shared `--timeout` can leave a mix of timed-out and cancelled rows.

`--any` succeeds on the first match, records targets that fail (for example `agent_not_running` when an agent exits) while others remain, then cancels the rest.
It fails only if every target fails.

In human mode, a target that fails while others remain is reported on stderr at once, for example `b failed: agent_not_running: ... (still waiting on 1 target).`, and the wait continues.
Multi-target output prints one line per target.
JSON writes no progress lines and returns `mode`, `winner`, and `targets`.
Each target row has `target`, `outcome` (`matched`, `errored`, or `cancelled`), `agent`, and `error`.

`fledge agent pause` interrupts the current foreground turn while preserving the pane and conversation.
It accepts exactly one nonempty `--name` or `--pane`, no positionals, a positive `--timeout` (default `10s`), and `--no-wait`.
It resolves the agent, then submits one key sequence to the resolved pane, without retries, escalation, focusing, closing, or submitting a prompt.
Already `idle` or `done` agents succeed without keys.

Pause refuses `blocked`, `unknown`, or startup-pending agents without sending keys.
It also refuses agents with missing or unknown harnesses.

The bindings assume harness defaults: double Escape for `amp`, `copilot`, `opencode`, and `kilo`.
Single Ctrl+C for `droid`, `grok`, `hermes`, `mastracode`, and `qodercli`.
Single Escape for every other currently supported harness.
Run `fledge agent capabilities --harness <kind>` to see a harness's interrupt binding and its evidence.

The evidence column does not mark every binding as checked or unchecked.
The [dogfooding record](../dogfood/friction.md) keeps the live pause observations.
OpenCode's keys can visibly interrupt output while the outcome remains `partial` with `agent_blocked`.
Inspect the terminal before retrying.

Mappings remain best effort.
Custom keybindings can change their effect.

Interruption does not freeze a process, undo completed work, drain queued prompts, or establish a persistent paused state.
Queued work can start again.
Resume or redirect the conversation with `fledge agent message`.

By default, pause waits within the remaining timeout for `idle` or `done` on the same resolved terminal.
These states indicate settlement, not successful task completion.
Becoming blocked fails immediately.
A changed terminal, disappearance, malformed response, or timeout also fails.

`--no-wait` acknowledges only key delivery.
Text reports `Pause requested`, `Paused`, or `Already idle or done`.
JSON uses operation `agent.pause`, the standard agent fields plus `submitted` and `settled`, and a `submitted` interrupt effect after acknowledgement.
Lost or malformed send acknowledgements produce `unknown`.

Failures after acknowledgement are `partial`.
Inspect the agent before retrying an uncertain interruption.
Resolving a pane cannot prevent its occupant changing before Herdr receives the keys.

`fledge agent stop` stops live agents by closing their panes.
It resolves each agent first and never closes a pane that does not host a known agent.
Stop refuses `working`, `blocked`, or `unknown` agents with exit status 2 unless you pass `--force`.
`idle` and `done` agents stop without it.

Stop first gives a `working` agent up to `--grace` to finish its turn.
The default is 5s, with a range of 0s through 60s.
A worker often reports just before its turn ends.
Stop proceeds if the agent settles `idle` or `done` in the same terminal.

It refuses if the agent remains working, becomes `blocked`, or the wait fails.
`--grace 0` refuses at once.
`blocked` and `unknown` agents get no grace, and `--force` never waits, so `--grace` with `--force` is invalid.
Stop sets `ended_at` on the agent's live record, if it has one, just before closing the pane, so an agent can stop its own pane.

If the pane fails to close, stop reopens the record it ended, returning it from the archive.
It leaves a record another command ended alone and reports a reopen refused because the terminal was registered again.

Stop takes the same targets as `agent message`: repeatable `--name`, `--pane`, and `--id` flags, which may be mixed, or the `agent list` filter flags, but not both.
Filter flags AND together and repeating one ORs its values.
Matches never include the caller, and an empty match fails with `no_agents_matched`.
An explicit target naming the caller is still stopped, as a single stop would, but it closes the caller's own pane, so name it last.

Fledge rejects duplicate targets.
One target gives the single result described above.
Stop handles targets one after another and resolves each again when its turn comes.
It follows a registered filter match by record ID.

An unregistered match must still have the same terminal.
The rules above apply, including `--force` and `--grace` for every target.
A working agent may therefore wait up to `--grace` each, so the worst-case wall time is `--grace` times the number of targets.

The listing a filter matched is a snapshot, and the per-target guard still refuses an agent that started working before stop reached it.
The result is `{"mode": "fan-out", "targets": [...]}`.
Each target has one row, in target order, with `target`, `outcome`, `agent`, and `error`.
`target` holds the explicit flag value, a filter match's record ID, or its pane if unregistered.

`outcome` is `stopped`, `refused` without `--force`, or `failed`.
Lookup failures also use `failed`.
Stopping continues past refusals and failures.
Any row that is not stopped, or whose record could not be ended, makes the outcome `partial` with exit status 1.

Effects are those of each stop.
Human output prints `Stopped N of M agents.` and one line per target.

`--dry-run` changes nothing: it looks up explicit targets, or lists a filter's matches, and makes no closing, waiting, or record call.
It always returns `{"mode": "dry-run", "targets": [...]}`, even for one target.
Row outcomes are `stop`, `refuse`, or `error`.
For `refuse`, the `error` gives the reason, such as an agent state requiring `--force`.

An `error` outcome means the lookup failed.
A `working` agent is planned as `refuse`, since stop would first give it `--grace` to finish its turn and it may still settle.
Refusals exit 0.

Any `error` row makes the outcome `rejected` with exit status 1.
Human output prints `Dry run: would stop N of M agents.` and one line per target with its state.

`fledge agent models` lists coding-agent models discovered locally and does not need a Herdr session.
It reads the `pi`, `codex`, and `claude` caches under the home directory and runs `opencode models` and `cursor-agent --list-models`.
Other harness kinds are not yet supported.
Fledge sorts rows by harness, then model.
Pass the `MODEL` value to `--model`.

`--harness` limits output to one documented kind.
An unsupported or uninstalled kind yields an empty list, and a missing file, unreadable cache, or failing command silently contributes no rows.

## Inbox

Fledge stores the body of every `agent message` to a registered recipient, every `task assign` brief, and every `task complete` notice to a registered creator as an inbox record.
The recipient's pane gets only a one-line doorbell, typed through Herdr `agent.prompt`:

```text
ᛉ fledge inbox · message from orchestrator (w1:p1) · id 0a1b2c3d · read: fledge inbox read --id 0a1b2c3d
```

The sender is `<name> (<pane>)`, `unnamed agent (<pane>)`, `pane <pane>`, or `fledge supervisor`.
An assignment's doorbell adds ` · task: <id> · <title>`.
The doorbell never contains the body.

The delivery guarantee: a body is typed at most once, and never for an inbox recipient; a doorbell may be rung again.
The [supervision daemon](#supervision) rings a doorbell that did not ring, and rings an unread one once more after `ack_timeout`.
A spawn's first prompt (its profile brief and `--prompt`/`--file`) is still typed with a header, and `agent send` is unchanged.

Each record has `id`, `kind` (`message`, `assignment`, or `wake`), `from` (`id`, `name`, `pane`), `to` (the recipient's record id), `task`, `body`, `created_at`, `rings`, `read_at`, `reply_to`, `answered_at`, and `answered_by`.
`from.id` is the sender's record id when it has one.
`from.name` is empty for an unnamed agent and `null` for a pane without an agent; a wake has only `name`, `fledge supervisor`.
Each entry in `rings` is one doorbell attempt: `at`, `pane`, `ok`, and `error`.
A failed attempt has `ok: false` and does not count as rung.
The derived `state` is `queued` (no successful ring), `rung` (rung and unread), `read`, or `answered`.

- `fledge inbox [--all] [--json]` lists the caller's records: unread first, each group oldest first, as `ID STATE KIND FROM TASK AGE` with the first body line beneath, cut to the terminal width.
  `--all` adds read records.
- `fledge inbox read [--id ID]... [--json]` prints each record's header, body, and a ready-made reply command, then sets `read_at`.
  Without `--id` it reads every unread record, oldest first.
  Reading again keeps the first `read_at`.
  Only the recipient may read a record (`inbox_not_recipient`), and a caller without a live record fails with `caller_unregistered`.
- The reply command is `fledge agent message --id <sender-id> --reply-to <inbox-id> --body "..."` for a registered sender and `--pane <pane>` for an unregistered one.
  A wake has no reply command.
- `agent message --reply-to <inbox-id>` links the reply to that record and sets its `answered_at` and `answered_by` (the reply's message id).
  Only the record's recipient may use it (`inbox_not_recipient`).

Bodies and names are printed through a terminal-control filter.
Records live in the primary checkout's `.fledge/state/inbox/`, one file each, so every linked worktree sees the same inbox.
`agent cleanup` moves the records to or from the workers it retires into `.fledge/state/inbox/archive/`, where `fledge inbox read --id` still finds them.
Nothing else deletes records.

## Supervision

One supervision daemon per Herdr session, `fledge supervise run`, watches workers and writes **wakes**: inbox records of kind `wake` from `fledge supervisor`, rung at once.
`agent spawn`, `task assign`, and any message stored but not rung start it in the background when none runs.
A `task complete` notice whose creator's doorbell does not ring also starts it.
The child runs detached in the primary checkout with only the Herdr socket and session from the environment, so it never acts as the starting pane.
A starter whose daemon reports another Fledge version stops it and starts a new one.

- `fledge supervise status [--json]` reports whether the daemon runs, with its pid, version, start time, heartbeat, and the agent records it watches.
  The heartbeat is rewritten every 10 seconds; one older than 30 seconds is reported as stale, which means the daemon may be hung.
- `fledge supervise stop [--json]` sends SIGTERM and waits up to 10 seconds.
- Liveness is an exclusive lock on `.fledge/state/supervise/<session>/lock`, held for the daemon's lifetime; the file also holds the status.
  A second daemon for the session exits at once.
  The daemon logs one line per event, wake, ring, reconnect, start, and exit to `.fledge/state/supervise/<session>/log`.
  It types nothing into any pane except doorbells.

Every 30 seconds the daemon rebuilds its watched set from `agent.list`: live agents whose record has a parent, agents owning an `assigned` task, and recipients of unread records.
It subscribes to each watched pane's agent status, and to pane exits, closes, moves, and agent detection.
After a lost socket connection it reconnects with a backoff from 1 second doubling to 30 seconds, then rebuilds the watched set.

Wakes go to the worker's parent, else to the creator of a task it owns; with neither, the daemon only logs the event.

| Wake | When | Body names |
| --- | --- | --- |
| `blocked` | The worker's status becomes `blocked`. One per blocked episode. | `fledge agent read --pane P` and `fledge agent send --pane P --key ...` |
| `exited` | The worker's pane exits or closes while its record is live. Once per worker. Not sent when `agent stop` or `agent cleanup` ended the record first. | Its tasks and `fledge task list --owner <id>` |
| `quiet` | 30 seconds after the worker becomes `idle` or `done`, it is still idle, still owns an `assigned` task, and has sent its parent nothing since its last doorbell. One per idle episode. | The task and `fledge agent read --name N` |
| `unacknowledged` | A rung record is still unread after `ack_timeout`: the daemon rings it once more. After another `ack_timeout`, it tells a registered sender. A wake or an unregistered sender is only logged. | The record and the recipient's pane |

`ack_timeout` comes from `[inbox] ack_timeout` in `.fledge/config.toml` (10s to 1h, default 5m).
At the end of the `quiet` grace period the daemon asks Herdr for the worker's live status and wakes only if it is still `idle` or `done`, so a missed status event cannot cause a false `quiet` wake.
A worker waiting on a background job can look idle, so some `quiet` wakes are benign: read the worker's pane before acting.
The daemon stores which wakes it sent under `.fledge/state/supervise/<session>/`, so a restarted daemon does not repeat them.
The daemon leaves ending records to the lookups that already do it.

The daemon exits after 60 seconds with nothing watched and no unread record.
`agent cleanup` stops it when every agent it watches was just retired.

`fledge attention [--json]` is a read-only summary for the caller: unread inbox records, the live workers it spawned grouped as `blocked`, `quiet` (shown only while the daemon runs), `working`, `idle`, `done`, and `unknown`, the tasks it created that are completed but not verified, and the daemon's state.

## Capabilities

`fledge agent capabilities` reports, for every documented harness kind in registry order, which operations Fledge can rely on.
`--harness` limits it to one kind.
The report comes from Fledge's harness registry and needs no Herdr session:

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

Levels: `fledge` means Fledge implements or uses it.
`native` means the harness or Herdr provides it natively.
`none` means it is not available.
`unknown` means it has not been checked.

`--live` makes one `integration.list` call and adds two facts per kind as Herdr reports them: `available` (the harness binary is on `PATH`) and `hook_state` (`current`, `outdated`, or `not_installed`).
A kind Herdr has no integration target for shows `-` in the table and `"live": null` in JSON.
`--json` emits `{"harnesses":[{"kind","capabilities":[{"name","level","evidence"}],"live"}]}` as the outcome's result.

## Usage

`fledge agent usage` reports each selected agent's whole-session usage, read from the harness's own session store.
It uses the same selection as `stop` and `message`.
Use repeatable `--name`, `--pane`, or `--id` targets, or filters, never both.
The filters are `--mine`, `--parent`, `--state`, `--harness`, `--profile`, `--task`, `--worktree`, and `--registered`.

A filter that matches nothing fails with `no_agents_matched`.

```text
NAME      HARNESS  MODELS           TURNS  INPUT  OUTPUT  CACHE-R  CACHE-W  COST         ELAPSED  BASIS
reviewer  claude   claude-opus-5-5  20     40     28.6k   1.7M     105.1k   -            4m20s    measured
builder   pi       model-one        14     1.2k   34k     2.5M     10       $0.61 (est)  1h2m5s   measured
helper    cursor   -                -      -      -       -        -        -            12m0s    unavailable (no native session ref observed)
```

Usage reads the harness and native session ref from the live agent when available.
Otherwise, it reads the record's `native_session`.
This lets `--id` report an agent that ended.
`ELAPSED` runs from registration to the record's end, or to now.

An agent without a record shows `-`.
Counts use `k` and `M` suffixes and absent values show `-`.

- `measured`: Fledge counted tokens from the harness's session store.
  The totals include sessions that the harness starts.
- estimate: `COST` is only a cost the harness itself recorded, such as pi's price table or opencode, shown as `(est)`.
  Fledge has no price table, so harnesses that record no cost show `-`.
- `unavailable`: Fledge could not read usage.
  The reason follows, such as a missing session ref, unsupported usage reader, or unreadable store.

Only commands that already write state capture session refs: spawn, adopt, `agent usage`, and `task assign`, `complete`, and `verify`.
An agent observed live here has its ref recorded (a failed write is a `warning` `native_session` effect).
An ended agent spawned before the ref capture shipped reports `unavailable`.
Usage covers the whole session, including several tasks or human conversations in the same pane.

`--json` emits `{"agents":[...]}` as the outcome's result, with one object per agent.
Each object contains `agent_id`, `name`, `pane`, `elapsed_seconds`, `harness`, `session_kind`, `session_value`, `models`, and `turns`.
It also contains `tokens`, `cost`, `first`, `last`, `subagents`, `basis`, `reason`, and `sources`.
`tokens` has `input`, `output`, `cache_read`, `cache_write`, and `reasoning`.

`cost` is null or contains `amount`, `currency`, `basis`, and `source`.
`subagents` holds the child sessions' share of `tokens`, or null.

## Profiles

A profile is a named **role brief**: plain Markdown instructions that `fledge agent spawn --profile NAME` sends ahead of the task.
A profile carries no start settings.
It never chooses the harness, model, or native arguments, so every spawn passes `--harness` itself, `--model` when wanted, and any additional native arguments explicitly.
Spawn supplies the Claude and Codex permission defaults independently of the profile:

```sh
fledge agent spawn --name impl --harness claude --model claude-opus-5-5 --profile implementer \
  --file brief.md
fledge agent spawn --name review --harness pi --model openai-codex/gpt-6-astra --profile reviewer --file brief.md
```

Eight built-ins ship inside the binary as Markdown and update with it.
They are never copied into a repository: `orchestrator`, `planner`, `researcher`, `implementer`, `debugger`, `integrator`, `reviewer`, and `verifier`.
Each is role guidance only.
The role prose names files to read first, such as `AGENTS.md`, with instructions such as "read if present".

There is no schema that checks or filters these files.

A brief is an ordinary first-prompt instruction, subordinate to the task and repository instructions.
It grants no permissions and changes no task state.

A repository profile is `.fledge/profiles/NAME.md` at the Git top level of the checkout Fledge is invoked from, so a linked worktree uses its own branch's profiles.
This differs from agent and task state, which the primary checkout shares, and from `--cwd`, which only places the agent.
A file named after a built-in replaces that built-in's role text entirely.
Any other name adds a custom profile.

There is no inheritance, section merging, or partial override: copy a built-in's text (`fledge agent profiles NAME`) and edit it to adapt one.
Names match `[a-z][a-z0-9_-]{0,31}`.
The file must be a regular file holding nonblank UTF-8 without NUL.
Invalid files fail before Fledge contacts Herdr.
An invalid override never falls back to its built-in.

Fledge never rewrites these files.

A profile spawn sends these blocks in order:

1. The role text.
2. One `## Fledge protocol` block, shared by every profile.
3. One `## Project memory` block, read at spawn time from the agent's working directory.

For `--worktree`, the working directory is the new or opened checkout.
The memory block contains the repository's [memory](memory.md#memory) index from its primary checkout, with a pointer to `fledge memory get`.
It contains `No project memories yet.` if no memories exist or the directory is outside any repository.
An unreadable memory file puts the error in the block instead.

It never fails the spawn.
Fledge adds the protocol and memory blocks only when you select a profile.

A spawn without `--profile` sends just its prompt, as before.
Fledge sends the brief before the task from `--prompt`/`--file`, separated by a blank line.
It uses one first prompt with one sender header, after readiness.
A profile without a task still sends its brief.
Fledge rejects `--profile` with `--no-wait` before any Herdr call.
Human output names the profile and its source, and JSON adds `profile` (`name`, `source`, `path`).

`fledge agent profiles` lists effective profiles with `NAME` and `SOURCE`.
`fledge agent profiles NAME` shows the source and brief for one profile.
The brief includes role text and the shared protocol, without the spawn-time memory block.
With `--json` a profile is `name`, `source` (`builtin` or `repo`), `path` (null for a built-in), and `brief`.

Both need no Herdr session, work outside Git (built-ins only), and write nothing.

Legacy TOML profiles are not loaded or migrated.
Spawning with a profile whose `.fledge/profiles/NAME.toml` exists fails before start, and any `.toml` file in `.fledge/profiles/` fails `agent profiles`.
The error names the TOML file.
It adds `and conflicts with .../NAME.md` if a same-name Markdown file exists.

It gives these migration instructions:

1. Move the brief text into `NAME.md`.
2. Pass settings to `agent spawn` with `--harness`, `--model`, and native arguments after `--`.
3. Delete the TOML file.

`.fledge/.gitignore` keeps everything else in `.fledge` ignored and ends with `*`, `!/profiles/`, `!/profiles/*.md`, so profile files can be committed.
Commands that prepare `.fledge`, such as agent registration or managed worktree creation, append missing rules to an existing ignore file.
They preserve its contents.
A file from an older Fledge that ends with `!/profiles/*.toml` gets the whole Markdown block appended after it.

Fledge-created checkouts (`agent spawn --worktree new` or `worktree create`) receive their own `.fledge/.gitignore` with those rules.
Scratch files such as `.fledge/tmp/` stay outside `git status`, including under an allowlist-style root `.gitignore`.

If that write fails, the checkout stays and the outcome is partial.
Opening an existing checkout writes nothing, so a linked worktree Fledge did not create has no `.fledge/.gitignore` and root ignore rules apply there.
`.fledge/tmp/` is the agents' scratch directory: the leading `*` in `.fledge/.gitignore` already ignores it, and proposals conventionally go under `.fledge/tmp/plans/` (see [Proposals](tasks.md#proposals)).

## Identity

Fledge keeps a durable record for each agent it starts or adopts, in `.fledge/state/agents/` under the repository's primary checkout (ignored by Git, shared by linked worktrees).
A repository created with `--separate-git-dir` records no path back to its primary checkout, so commands run from its linked worktrees fail until you run `git config core.worktree <primary checkout path>` once.
Bare repositories, including their linked worktrees, have no primary checkout and are not supported.
A record holds an 8-hex `id`, the agent's name, pane, workspace, harness, Herdr session (`HERDR_SESSION`), and Herdr `terminal_id`.

It also holds `parent`, `profile`, `registered_at`, and `registered_by` (`spawn` or `adopt`).
Checkout fields are `worktree_path`, `worktree_created`, `worktree_base`, `worktree_branch`, and `worktree_marker`.
Other fields are `ended_at`, `native_session`, and `native_session_history`.
`profile` names the profile selected by `agent spawn --profile`.

It is null for adopted agents, spawns without a profile, and records written before that field existed.

`worktree_created` is true only when the agent's spawn created its checkout (`--worktree new`), not when it opened an existing one.
`worktree_base` records the ref spawn passed to Herdr when creating the checkout.
It holds the explicit `--base`, otherwise the primary checkout's branch at spawn time.
For a detached primary checkout, it is null and spawn passes no base.

`worktree_branch` and `worktree_marker` identify the created checkout.
Spawn writes a random marker into `<git dir>/worktrees/<name>/fledge-checkout-id`.
Git deletes this marker when it removes or prunes the checkout.
A checkout later recreated at the same path never carries that marker, even on the same branch.

The marker is null if it could not be written.

Records written before these fields existed read as not created or unidentified.
The parent is the caller's live record when the caller's pane hosts a registered terminal.
Otherwise it is null.
Fledge never deletes records.

It moves ended records to `.fledge/state/agents/archive/`.
ID lookups still find them, but live-agent scans exclude them.

`.fledge/state/agents/index/` holds empty marker files that index each record by its parent, which is fixed when the record is created, so `agent cleanup` reads only the caller's children.
A record without a marker, such as one an older Fledge wrote, is still read, and the next registration indexes it.

`native_session` holds the harness's session ref as Herdr reports it in `agent_session`.
Its fields are `source`, `harness`, `kind`, `value`, and `observed_at`.
Later commands can use the ref after the agent's pane closes.
A command that already writes state stores the ref when it first observes it.

When an observation has a different `value`, Fledge moves the old ref to `native_session_history` and makes the new ref current.
The history keeps at most eight refs, newest last.
An equal value changes nothing.
Spawn (once startup settles, or at start with `--no-wait`), adopt, and `task assign`, `complete`, and `verify` (for the owner or verifier they touch) capture it.

A failed write leaves the command successful with a `warning` `native_session` effect, and a capture that changed the record adds an `updated` `native_session` effect.
`agent get`, `list`, and `wait` never capture it.
They only show what Herdr or the record already holds.
Both fields are null until a capturing command observes a ref, including for records written before these fields existed.

An agent spawned before ref capture shipped has no persisted ref until such a command observes one.
The same applies if the harness reports its session only after spawn readiness.

Herdr reports Claude or Codex session refs only at the harness's `SessionStart`.
A restart or resume in the same pane can leave a stale ref until that hook fires again.
The history records a change but cannot force one.

The terminal is a record's identity and the pane only locates it.
Herdr changes a pane's ID when it moves across workspaces, but keeps its terminal.
A lookup may find the recorded pane gone or hosting another terminal.
Fledge then searches Herdr's agent list for the recorded terminal.

If found, Fledge updates the record's pane and workspace while keeping its ID.
Fledge sets `ended_at` when `agent stop` closes the agent's pane or a lookup finds its terminal absent from all Herdr panes.
An ended record is no longer live.

A terminal that still exists but hosts no agent (its harness exited) only makes lookups fail.
The record stays live.
A record belongs to the harness that registered it.
A later lookup compares the record's `harness` with Herdr's live agent kind.

If both are known and differ, Fledge ends the record and `--id` fails with `agent_identity_stale`.
The terminal then counts as unregistered.
Its new agent has no parent record and can adopt itself.
Listings never attribute such a record to the live agent.

A Herdr server handoff reissues every `terminal_id`, so records from before it end on their next lookup and each agent must run `fledge agent adopt` again in its own pane.

Spawn registers the agent once startup settles (or once start begins with `--no-wait`) and before any first prompt.
Its result adds `id`, `registered`, and `registration_error`.
Outside Git, or when Fledge cannot write the store, the agent still runs and spawn succeeds.
`registered` is false.

Text output shows `id: - (not registered: <reason>)`.

The invoking repository owns coordination state.
`--cwd` only changes the shell's starting directory and, with `--worktree new`, the source repository.
Spawning from repository A with `--cwd` in repository B records the agent in A.
A caller outside any Git repository gets an unregistered spawn even when `--cwd` names one.

`--name` and `--pane` are live Herdr selectors that work from anywhere, but `--id` and task records are read from the invoking repository.

A worker started into B that should use A's records must run those commands from A, for example `cd /abs/path/to/A && fledge task complete --id <task> --summary "..."`.

`fledge agent adopt` registers the caller's own running agent: the agent in the caller's pane (`HERDR_PANE_ID`).
Run it inside that agent, for example from its shell tool.
It has no flag to select another pane, and `--pane` is refused as an unknown flag.
To register a different agent, run adopt in that agent's own pane, or start the agent with `fledge agent spawn`.

Adopt verifies the caller before it writes anything.
It asks Herdr (`pane.process_info`) for the foreground processes of the pane where the caller's terminal now runs, which follows a moved pane.
One of them, other than PID 1, must be this `fledge` process or one of its ancestors, read from Linux `/proc`.
The pane's shell PID, working directories, process names, the environment, and native session IDs prove nothing.
Missing or malformed evidence, an unreadable or reused ancestor, and other platforms fail with `caller_identity_unverified` (phase `identity`, exit 1) before Fledge creates any state.
A caller outside a Herdr pane fails with `invalid_input` (exit 2).
Just before its first change, adopt looks up the caller again and verifies the same terminal, the same harness, and the same ownership.
A failed recheck reports only the `.fledge` setup it created; it registers and names nothing.
Herdr and the operating system have no transaction, so a change after the last check is not detected.

Tool commands must run as descendants of the agent's process.
Claude and Pi tool commands do.
Codex tools run by its default shared daemon do not, so adopt refuses them: start Codex with `codex --no-daemon`. `fledge agent spawn` adds `--no-daemon` for Codex.
Codex workers need a Codex version that supports `--no-daemon` (0.162.1 does); an older Codex cannot adopt itself and fails to start under `fledge agent spawn`.

An unnamed agent needs `--name`, which adopt sets through Herdr (`agent_name_taken` and `agent_launch_pending` are reported as is).
A named agent keeps its name.

A different `--name` is refused.
A named agent whose terminal already has a live record is refused with `agent_already_registered` and its existing ID.
The check and the record creation share one store lock, so concurrent adopts or spawn registrations of one terminal produce exactly one record.
An unnamed agent can already have a live record, for example after losing its Herdr name.

Adopt names it through Herdr and keeps its ID, parent, profile, registration, and checkout.
It stores the new name and current pane, with an `updated` `agent_record` effect.

If the record ends after the rename, the outcome is `partial`: the agent is named but the record is unchanged.
Naming an agent also labels its pane, and its tab when the pane is the only one there, as `agent rename` does.
A new record has no parent.
Success prints `Adopted <name> (<pane>) as <id>.`

`fledge agent rename --to NAME` renames a live agent: the caller's own without `--name`, `--pane`, or `--id`.
Herdr renames the agent, reporting `agent_name_taken` and `agent_launch_pending` unchanged.
If a live record exists, Fledge stores the new name under the same ID.
Task ownership and `--id` lookups remain valid.

The pane label becomes the new name, and so does the tab label when the pane is alone in its tab (`pane_label` and `tab` effects).
A tab shared with other panes keeps its label.

An agent already named `NAME` is only relabeled.
Messages addressed to the old name, including earlier reply headers, no longer reach the agent.
Success prints `Renamed <old> to <new> (<pane>).`

`get`, `read`, `send`, and `pause` accept `--id` in place of `--name` or `--pane`.
Exactly one of the three is required.
`message`, `wait`, and `stop` accept repeatable `--id` alongside `--name` and `--pane`.
An `--id` lookup follows a moved terminal as described above.

It fails closed with `agent_identity_stale` if the terminal hosts no agent, the record belongs to another Herdr session, or the record ended.
An absent agent ends the record only if the terminal itself is gone.

An unknown ID fails with `agent_record_not_found`.
`agent get` shows the live agent's record when one exists.
This includes the Fledge ID, parent, profile, persisted native session, registration time, and source.
The native session appears as `Native session: <kind> <value>` or `-`.

JSON adds `record`.
`agent list` adds `ID` and `PARENT` columns and a `PROFILE` column after `HARNESS` (`-` when unregistered, parentless, or spawned without a profile) and `id`, `parent`, and `profile` fields.
It fails with `protocol_error` rather than list partially when any entry of Herdr's agent list lacks a required field, such as its terminal ID.

Session names come from the environment, so these checks are a workflow guard, not a security boundary.

`agent list` filters take effect together: different flags AND, and repeating one flag ORs its values.
`--state` matches the live Herdr status (`idle`, `working`, `blocked`, `done`, or `unknown`) and `--harness` the live harness kind.
Other filters match this repository's live records:

- `--profile`: the recorded profile name.
- `--task <id>`: the task's owner.
  An unowned task matches nothing.
  An unknown task fails with `task_not_found`.
- `--worktree <path>`: the recorded worktree path, resolved against the current directory.
- `--registered`: any live record.
  Herdr's agent list spans every repository, so these record filters never match agents registered elsewhere, which show `-` in the ID column.

`--parent <id>` keeps only live agents whose record's parent is that ID.
`--mine` does the same for the caller's own live record.
The two are mutually exclusive and list only direct children.
With any filter an empty result prints `No agents match.`.

An unfiltered empty listing prints `No live agents.` A record filter fails with the store error when the repository's records cannot be read.
Without a filter, Fledge leaves ID, PARENT, and PROFILE empty instead.
`--ids` prints one record ID per line for the registered matches, skipping unregistered ones, for piping into other commands.
Fledge rejects `--ids` together with `--json`.

`fledge agent current` shows the caller's own live record: its ID, name, pane, workspace, harness, worktree, profile, parent, and the tasks it owns in the `assigned` state, oldest first.
The parent's name is the name recorded on the parent's record, shown while that record exists.
It can differ from the parent's live Herdr name.
JSON flattens the record into `result` and adds `parent_name` and `tasks` (`id` and `title`).

`agent current` and `agent list --mine` fail with `caller_unregistered` when the caller's pane hosts no registered agent, including outside a Herdr agent pane.
The agent registers itself with `fledge agent adopt` in its own pane.

## Cleanup

`fledge agent cleanup` stops the caller's finished workers and removes the checkouts their spawns created.
It acts only on direct workers the caller spawned: records whose parent is the caller's live record and whose `registered_by` is `spawn`.
The caller itself, siblings, grandchildren, adopted agents, and unregistered agents are never touched.
The caller must have a live record (`caller_unregistered` otherwise).
After stopping workers, cleanup archives their inbox records (to or from them) and stops the supervision daemon when nothing it watches remains (see [Supervision](#supervision)).

Cleanup stops a worker with a live record only when all these conditions hold:

- Its agent is `idle` or `done`.
- No live record names it as parent.
- Every task it owns is `verified` or `cancelled`, regardless of who created the task.

`idle` or `done` alone never counts as accepted results.
Cleanup holds a worker without tasks unless you pass `--results-collected`.
That flag asserts that you read and kept all its reports.
Fledge cannot detect that itself.

Cleanup holds every other worker and reports the reasons.

Cleanup removes a checkout only when all these conditions hold:

- A worker's spawn created it (`worktree_created`).
- Its Git marker and checked-out branch match `worktree_marker` and `worktree_branch`.
- It is managed under `.fledge/worktrees` and is not the primary checkout.
- A base branch was recorded.
- Its worker ended or cleanup is stopping it now.
- It is clean.
- It is merged into its recorded base, such as `dev`, rather than the [integration branch](worktrees.md#integration-branch).
- No other live agent uses it, except workers cleanup is stopping, under the same rules as `worktree remove`.

Cleanup always keeps branches.
It reports and keeps borrowed checkouts and checkouts recorded before creation or identity tracking.
It also keeps replaced checkouts, with reason `replaced since the worker's spawn`, and any checkout that fails a check.
When several workers recorded the same path, only the one whose recorded marker matches the checkout there can own it.

Registration order never decides.
Once the created checkout is gone, its record can never authorize a removal again.
Remove kept checkouts yourself with `fledge worktree remove` once they are no longer needed.
Cleanup also reads archived worker records.

This lets a later run remove a stopped worker's checkout once it meets the removal conditions.

A checkout no longer listed by Git is not reported.

`--dry-run` changes nothing, including the caller's recorded pane, and requires no assertion.
It lists planned stops, removals, and held resources, with reasons.
Cleanup reads each worker's tasks again, then stops eligible workers by record ID using `agent stop --id` without `--force`.
It removes eligible checkouts through `worktree remove` without `--force`.

Removal rechecks live agents, cleanliness, and the merge into the recorded base.
Immediately before removal, cleanup also checks the planned path under `.fledge/worktrees`, the worker's marker, and the branch.
Cleanup keeps a checkout moved meanwhile, even with a symlink at its old path, as `moved since cleanup planned it`.
These identity and location checks apply only to cleanup.

An explicit `worktree remove` is unchanged.
A task assigned, a live worker of the worker registered, or a worker busy again after planning holds it.
Cleanup keeps a checkout replaced meanwhile as `replaced since the worker's spawn`.
Nothing is ever forced.

Text output starts with `Stopped N of M workers and removed N of M checkouts.` or `Dry run: would stop ...`.
It lists workers as `stop`/`stopped`, `hold`, or `failed`.
It lists checkouts as `remove`/`removed`, `keep`, or `failed`.
Each entry includes reasons.

JSON uses operation `agent.cleanup`.
`result` contains `dry_run`, `caller`, `workers`, and `checkouts`.
Worker fields are `id`, `name`, `pane_id`, and `agent_status`.
Checkout fields are `path`, `branch`, `base`, and `worker`.

Each entry has an `outcome` (`planned`, `done`, `skipped`, or `failed`) and a `reason`.
Effects are those of the stops and removals.

Held and skipped resources still exit 0.
Failed actions do not stop later actions, and cleanup never reverses completed actions.
Failures produce `partial`, or `rejected` when nothing changed.
The outcome is `unknown` when a stop or removal may have applied.

These outcomes use exit status 1.

Cleanup rechecks its guards before each action.
Task assignment, Herdr, and Git changes cannot form one transaction.
Run cleanup when you are not dispatching new work to those workers.
A worktree whose spawn failed before registration has no record and needs `worktree remove`.

Clean means Git's view of tracked and untracked, nonignored files.
Ignored build artifacts in a removed checkout are lost.

Squash-merged branches count as unmerged.

## Outcomes and recovery

Each command supports `--json`, emitting one Fledge object with `operation`, `status`, `result`, `effects`, and `error`.
Results use stable Fledge fields rather than raw Herdr responses.
Unavailable values are null.
Effects report known resources and IDs or paths.

Errors include a code, message, and failing phase.
Exit codes are 0 for success, 2 for invalid input, and 1 for runtime failures.

- `success`: the requested operation completed.
- `rejected`: failure is known to precede mutation, without uncertainty.
- `partial`: acknowledged mutations preceded failure, or Herdr acknowledged a failed startup.
  A startup timeout can leave a process running, even in an existing pane.
- `unknown`: a mutation may have applied, but its response was lost or unusable.

Fledge retains created resources and never automatically retries mutations or cleans up after failure.
Inspect the reported pane and resources in Herdr before retrying.
For an unknown message outcome, inspect the conversation first to avoid submitting the same prompt twice.
A started process is not proof of readiness.

A fresh tab can briefly return `agent_pane_busy` while its shell reaches a prompt.
Fledge retries only the start step for that code, up to six more times, without recreating resources.
Delays start at 50ms and double to a cap of 800ms, totaling about 2.4 seconds.
Other errors are never retried.

If startup still fails, inspect the preserved pane.
Check that it is an idle shell without a started agent.
Retry into that specific pane:

```sh
fledge agent spawn --name reviewer --harness claude --pane w2:p3
```

Use the actual pane ID from the partial outcome and retain your intended harness, model, and native arguments.
Fledge makes no further automatic retries.
