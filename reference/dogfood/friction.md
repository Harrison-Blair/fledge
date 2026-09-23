# Dogfooding friction log

This file tracks friction agents have hit while using Fledge itself for agent
coordination: bugs, missing capabilities, and workarounds found during real
work in this repository. New entries are appended at the bottom, using the
template below.

## Entry template

**Issue:** Short title

**Summary:** What happened and how often/reliably it was observed.

**Reproduction steps:**
1. ...
2. ...

## Entries

**Issue:** First message after spawn is rejected as not ready

**Summary:** `agent spawn` exits 0, but an immediate `agent message` fails
with `rejected: agent_not_ready: agent <name> is not an active named agent
(agent.prompt)`. Reproduced on 11 of 11 un-retried first messages (4
orchestrator spawns + 7 probe trials). `agent spawn` returns in ~6 ms with
`agent_status: unknown` and `launch_pending: true`. All 7 probe trials
left `unknown` at 3922-3926 ms; the 5 Claude trials then flipped to
`idle` / `interactive_ready: true` with `launch_pending` cleared, and the
first accepted prompt was at ~4326 ms, while the 2 Codex trials flipped to
`blocked` with `launch_pending` still `true` and never accepted a prompt
in the 30 s window (see the startup-dialog entry below). Herdr's reference
doc for `agent.start` (`reference/herdr/api/agent.md`) says it returns
only once the agent is ready; on 0.9.1 it does not. Observed 2026-09-18,
Fledge 0.0.3 built from `dev` (`fc4538b`), Herdr 0.9.1, binary
`/tmp/fledge-dev`. Resolved 2026-09-19: `agent spawn` now waits on
`agent.wait` before returning, so a successful spawn reports the settled
status and an immediate `agent message`/`--prompt` no longer races
`agent_not_ready`; `--no-wait` restores the old return-immediately
behavior.

**Reproduction steps:**
1. Run `fledge agent spawn --name w --harness claude --tab w`.
2. Immediately run `fledge agent message --name w --body "hi"` (or
   `--file`).
3. Observe the `agent_not_ready` rejection; a retry ~5 s later succeeds.
4. Poll `fledge agent get --name w --json` and watch `launch_pending` /
   `interactive_ready`.

---

**Issue:** `agent get` still reports `idle` just after a successful message

**Summary:** For a few seconds after "Message submitted", `agent get` still
reports status `idle`, so a loop that waits for `idle` exits before work
starts. Observed 2026-09-18, Fledge 0.0.3 built from `dev` (`fc4538b`),
Herdr 0.9.1, binary `/tmp/fledge-dev`.

**Reproduction steps:**
1. Message an idle agent with `fledge agent message --name w --body "..."`.
2. Immediately run `fledge agent get --name w`.
3. Observe the status still reads `idle` for a few seconds.

---

**Issue:** No command to wait for an agent

**Summary:** There is no Fledge command that blocks until an agent finishes
its work; orchestrators are left writing their own shell poll loops over
`agent get`. Observed 2026-09-18, Fledge 0.0.3 built from `dev`
(`fc4538b`), Herdr 0.9.1, binary `/tmp/fledge-dev`.

Resolved 2026-09-22: `agent wait` blocks until agents reach a lifecycle
state, with `--until`, `--timeout`, and repeatable `--name`/`--pane`
targets combined with `--all`/`--any`.

**Reproduction steps:**
1. Spawn and message a worker agent.
2. Try to block until it finishes using only the commands listed in
   `fledge agent --help`.
3. Observe there is no wait/block command; only manual polling works.

---

**Issue:** No command to read an agent's output

**Summary:** The only way to see a worker's reply is
`herdr pane read <pane> --source recent-unwrapped`; Fledge itself has no
command for it. Observed 2026-09-18, Fledge 0.0.3 built from `dev`
(`fc4538b`), Herdr 0.9.1, binary `/tmp/fledge-dev`.

Resolved 2026-09-22: `agent read` prints a plain-text terminal snapshot of
one live agent's pane, with `--source` and `--lines` options.

**Reproduction steps:**
1. Message a worker agent and wait for it to go idle.
2. Try to obtain its reply using only `fledge agent --help` commands.
3. Observe no such command exists; fall back to `herdr pane read`.

---

**Issue:** No command to answer a blocked agent

**Summary:** A worker waiting on a permission prompt or question can only
be answered with `herdr pane send-text` / `herdr pane send-keys`, not with
any Fledge command. Observed 2026-09-18, Fledge 0.0.3 built from `dev`
(`fc4538b`), Herdr 0.9.1, binary `/tmp/fledge-dev`. Since 2026-09-22
(`1bb9b98`), `fledge agent send --name w --key down --key enter` (or `--text`)
is the Fledge workaround; it types raw input without detecting the dialog or
its choices.

**Reproduction steps:**
1. Give a Claude worker a task that needs a command outside its allowlist.
2. Wait for its status to become `blocked`.
3. Try to respond to it using only Fledge; observe no such command exists.

---

**Issue:** `blocked` status carries no reason

**Summary:** `agent get` shows `Status: blocked` but not what the agent is
waiting on; the underlying pane must be read separately to find out.
Observed 2026-09-18, Fledge 0.0.3 built from `dev` (`fc4538b`), Herdr
0.9.1, binary `/tmp/fledge-dev`.

**Reproduction steps:**
1. Reproduce a blocked agent as in the "No command to answer a blocked
   agent" entry above.
2. Run `fledge agent get --name w`.
3. Observe `Status: blocked` with no indication of what it is waiting on.

---

**Issue:** Spawn reports success for an agent stuck on a startup dialog

**Summary:** Codex 0.154.0 opens on an "Update available" menu (default
choice runs a curl|sh installer). `agent spawn` prints success, status is
`blocked`, and `agent message` fails with `agent_blocked: ... requires
interactive input`. In 2 probe trials the agent went `unknown` -> `blocked`
at ~3.9 s with `launch_pending` still `true`, and stayed `agent_blocked`
for the full 30 s observation. Workaround used: `herdr pane send-text
<pane> 2` (Skip); `herdr pane send-keys <pane> Down` had no effect.
Observed 2026-09-18, Fledge 0.0.3 built from `dev` (`fc4538b`), Herdr
0.9.1, binary `/tmp/fledge-dev`. Since 2026-09-22 (`1bb9b98`), the Fledge
workaround is `fledge agent send --name x --text 2`; spawn still reports
success and gives no reason for the block.

Resolved 2026-09-23: spawn waits by default and a `blocked` wait fails as
`partial` `agent_blocked` (exit 1), keeping the agent and its record. Human
output now points at `fledge agent read --pane P` and
`fledge agent send --pane P --key <key>` instead of raw Herdr commands. It
still cannot say what the dialog is (see the entry above).

**Reproduction steps:**
1. With a Codex update pending, run
   `fledge agent spawn --name x --harness codex --tab x`.
2. Note spawn reports success even though the agent is stuck on the
   update dialog.
3. Run `fledge agent message --name x --body "hi"` and observe the
   `agent_blocked: ... requires interactive input` failure.

---

**Issue:** Delivered messages carry no sender

**Summary:** `agent message` text arrives at the worker bare, with no
indication of who sent it. A careful worker treated an orchestrator brief
as a possible mis-paste and stopped to ask whether to execute it. Observed
2026-09-18, Fledge 0.0.3 built from `dev` (`fc4538b`), Herdr 0.9.1, binary
`/tmp/fledge-dev`.

Resolved 2026-09-22: every delivered prompt now starts with one header
line naming the sender and a correlation ID.

**Reproduction steps:**
1. Write a brief file whose text opens "You are a worker...".
2. Run `fledge agent message --name w --file brief.md`.
3. Observe the worker receives the text with no sender attribution and
   asks who sent it.

---

**Issue:** `fledge --version` does not identify the build

**Summary:** A binary built one commit behind `dev` reported the same
`0.0.3` as the current `dev` build, so a stale binary went unnoticed until
a command was missing. Observed 2026-09-18, Fledge 0.0.3 built from `dev`
(`fc4538b`), Herdr 0.9.1, binary `/tmp/fledge-dev`.

Resolved 2026-09-22: release binaries report their exact tag, and local
builds report Go's embedded tag or commit-derived version (`+dirty` when
appropriate); metadata-free builds report `dev`.

**Reproduction steps:**
1. Build the binary at commit A.
2. Check out commit B (a different commit, same `VERSION` file).
3. Run `fledge --version` on the commit-A binary and observe it reports
   the same version as commit B, with no way to tell them apart.

---

**Issue:** Session fields rendered `-` for a Codex agent in `agent get`

**Summary:** The `Session ...` lines in `fledge agent get` output showed
`-` instead of real values for a Codex agent. Observed at commit
`62d9fa5`; not re-checked since.

**Reproduction steps:**
1. Spawn a Codex agent with `fledge agent spawn --harness codex ...`.
2. Run `fledge agent get --name x`.
3. Observe the `Session ...` lines render `-` instead of values.

---

**Issue:** Workers cannot reply to the agent that messaged them

**Summary:** `agent message` gives the receiver no sender identity or
reply path, so a worker told to "report to the orchestrator" guessed from
the session list and sent its report to a sibling worker (`race-prober`)
instead; the orchestrator had to read the pane with `herdr pane read`.
Observed 2026-09-18, Fledge 0.0.3 built from `dev` (`fc4538b`), Herdr
0.9.1, binary `/tmp/fledge-dev`.

Resolved 2026-09-22: the sender header now includes a `reply:` command
naming the sender for a named sender, so a worker can address its reply
directly.

**Reproduction steps:**
1. Spawn workers `a` and `b` from an orchestrator.
2. Message `a` with a brief that says "report back to the orchestrator".
3. Observe `a` has no Fledge way to address its sender.

---

**Issue:** Herdr reference docs may be outdated

**Summary:** The repo's `reference/herdr/` docs drove a wrong assumption; the
`agent.start` section was marked validated against 0.8.2 and promised a
blocking start, but the installed herdr is 0.9.1 and returns immediately.
The section was corrected on 2026-09-18, but other sections still carry
0.8.2 validation stamps and may be stale too, and
`reference/herdr/raw/skill.md` (upstream text) still makes the blocking
claim. Observed 2026-09-18, Fledge 0.0.3 built from `dev` (`fc4538b`),
Herdr 0.9.1, binary `/tmp/fledge-dev`.

**Reproduction steps:**
1. Run `herdr --version` and compare it against the doc stamps with
   `grep -rn "Validated" reference/herdr`.
2. Time `fledge agent spawn --name <n> --harness <kind> --tab <n>`.
3. Immediately poll `fledge agent get --name <n> --json` and inspect
   `launch_pending`.
4. Observe stamps reading `0.8.2` (and `reference/herdr/raw/skill.md`
   still describing a blocking start) alongside a spawn that returns
   immediately with `launch_pending: true`, confirming the docs no longer
   matched measured behavior until corrected.

---

**Issue:** Short spawn timeout loses the agent name

**Summary:** With `--timeout 3001ms`, `agent.wait` returned `agent_not_running`
(not `timeout`) at ~3011 ms; Herdr dropped the agent's name and left the
harness running unnamed in its pane. The printed partial-outcome hint
(`herdr agent get <name>`) then fails with `agent_not_found`, and `fledge
agent stop --name <name>` cannot reach the agent either (`--pane` still
works). Also reproduced with `--no-wait --timeout 3001ms` (the name is gone
~4 s later), so this predates spawn waiting for readiness; `agent.start`
gets the full budget and `agent.wait` the remainder, so both deadlines fire
together and Herdr's start-side teardown wins. Only measured at 3001 ms.
Observed 2026-09-19, Fledge `dev` @ `180abb1`, Herdr 0.9.1.

**Reproduction steps:**
1. Run `fledge agent spawn --name vfy-f --harness claude --tab vfy-f --timeout 3001ms`.
2. Observe a `partial` outcome with error code `agent_not_running`, phase
   `agent.wait`, at ~3011 ms.
3. Run `herdr agent get vfy-f` (or `fledge agent get --name vfy-f`) and
   observe `agent_not_found`.
4. Run `fledge agent stop --name vfy-f` and observe it cannot resolve the
   agent; `fledge agent stop --pane <pane-id>` (from the outcome's effects)
   still works.
5. Repeat with `--no-wait --timeout 3001ms` and poll `agent get` a few
   seconds later: the name still disappears (~4 s), confirming this
   predates spawn waiting for readiness.

---

**Issue:** Spawn-time prompt is lost when a harness shows a startup dialog

**Summary:** `fledge agent spawn --harness cursor --file <brief>` reported
`Message submitted`, but cursor-agent stopped on its "Workspace Trust Required"
dialog and the brief never reached the model; the pane showed an empty prompt input
after trust was granted. A follow-up `fledge agent message --file <brief>` to the same
agent delivered it. Separately, cursor named models were plan-gated: the model
`claude-opus-4-8-medium` started but immediately returned "Named models unavailable.
Free plans can only use Auto," with no preflight warning from Fledge. Workaround: send
the brief again after the agent settles, and use the `claude` harness instead.
Observed 2026-09-20, Fledge `dev` @ `d81468d`, Herdr 0.9.1.

**Reproduction steps:**
1. `fledge agent spawn --name w --harness cursor --model claude-opus-4-8-medium --tab w --file brief.md`.
2. Observe `Message submitted`, then cursor-agent's workspace-trust dialog.
3. Grant trust and observe the prompt input is empty; the model never saw the brief.
4. Re-send with `fledge agent message --name w --file brief.md`; it is delivered.

---

**Issue:** `fledge doctor` model discovery causes file writes and network activity

**Summary:** A live `fledge doctor` run launches `opencode models` and
`cursor-agent --list-models`. A syscall trace showed the child processes opening
`~/.local/share/opencode/log/opencode.log`, `opencode.db` and its WAL files, and
`.git/opencode` for writing. Cursor created a session log under
`/tmp/cursor-agent-logs-1000` and a `.running` marker. The trace also showed
Internet-address socket connections from cursor. This violates doctor's stated
read-only, no-file-writes, no-network contract. Observed 2026-09-20 with the
current `dev` working tree and Herdr protocol 22.

Resolved 2026-09-20: `model_discovery` now reads only local cache files, for
the harness kinds whose models come from a cache (`pi`, `codex`, `claude`);
doctor never executes a harness command, so `opencode` and `cursor` are no
longer checked there (use `fledge agent models` for those).

**Reproduction steps:**
1. Build the current checkout with `go build -o /tmp/fledge-verify .`.
2. Run `strace -f -e trace=connect,openat,creat,rename,unlink,mkdir -o /tmp/fledge-doctor-strace.log /tmp/fledge-verify doctor` in a Herdr pane with opencode and cursor available.
3. Search the trace for `O_WRONLY`, `O_RDWR`, `O_CREAT`, and `AF_INET`; observe the child process writes and network socket connections.

---

**Issue:** Fake Herdr socket regression tests need sandbox approval

**Summary:** While implementing `agent pause`, the restricted Codex sandbox refused
Unix socket listeners used by CLI integration tests (`setsockopt: operation not
permitted`). This is an environment restriction, not a pause defect. Workaround:
run the Go test command with approved sandbox escalation. The approved run reached
the expected pre-implementation missing-command failures. The full `go vet`
check also needed escalation because the existing Go build cache was read-only
in the sandbox.

**Reproduction steps:**
1. In the restricted Codex sandbox, run `go test ./internal/agent ./cmd`.
2. Observe fake socket listener failures in the CLI integration tests.
3. Re-run `go test ./cmd -run '^TestPause' -count=1` with sandbox escalation to exercise the fake Herdr protocol.

---

**Issue:** OpenCode interruption can report transient blocked settlement

**Summary:** During live pause smoke testing on 2026-09-21, OpenCode 1.18.25
interrupted active arithmetic output at item 62 after the double-Escape sequence;
the terminal explicitly showed interruption. The immediate `agent.wait` returned
`blocked`, so Fledge correctly reported `partial`, `submitted=true`,
`settled=false`, and `agent_blocked`. A subsequent message produced the exact
response `RESUMED_OPENCODE_9137` in the same session and pane, then reached `done`.
The observed interruption worked, but immediate settlement was not confirmed.
Inspect the terminal before deciding how to resume; do not treat this partial
outcome as proof that the keys failed or automatically retry them.

**Reproduction steps:**
1. Start an OpenCode 1.18.25 agent and request a long arithmetic response.
2. While output is active, run `fledge agent pause --name <agent> --json`.
3. Observe interruption in the terminal (item 62 in this trial), while the pause outcome reports `agent_blocked` with acknowledged delivery and unconfirmed settlement.
4. Send a follow-up using `fledge agent message --name <agent> --body 'Respond with exactly RESUMED_OPENCODE_9137'`.
5. Verify the exact response, unchanged session and pane, and subsequent `done` state.

---

**Issue:** Initial Codex message acknowledged without visible prompt or session

**Summary:** During live smoke testing on 2026-09-21, a message sent immediately
after a fresh Codex spawn was acknowledged, but no prompt or session appeared
in the terminal. A later message worked. A startup readiness race is a possible
explanation, not an established cause. Subsequent active-turn pause testing
succeeded with `submitted=true` and `settled=true`; a follow-up produced the exact
response `RESUMED_CODEX_9137` in the same session and pane. This observation does
not establish that pause caused or fixed the initial delivery issue.

**Reproduction steps:**
1. Spawn a fresh Codex agent, then immediately submit a message with `fledge agent message`.
2. Compare the delivery acknowledgement with the terminal; in this trial no prompt or session appeared.
3. Later submit another message and inspect the terminal; in this trial that message worked.
4. Treat the timing and cause as unconfirmed until independently reproduced.

---

**Issue:** `spawn --file` on the pi harness fails the prompt with `agent_not_ready` after reporting the agent idle

**Summary:** On 2026-09-21, `fledge agent spawn --name plan-reviewer --harness pi
--model openai-codex/gpt-6-astra --tab plan-reviewer --file brief.md --timeout 90s`
returned status `partial` with `agent_status` `idle`, effects tab/pane created and
agent started, and error code `agent_not_ready`, message "agent plan-reviewer is
not an active named agent", phase `agent.prompt`. An immediate
`fledge agent message --name plan-reviewer --file brief.md` succeeded. Fledge
built from `dev` at `dccfdf1`, Herdr 0.9.1. Observed on both of two pi spawns on
2026-09-21/22. The second, `fledge agent spawn --name verify-plan-doc --harness pi
--model openai-codex/gpt-6-astra --cwd /home/penguin/source/fledge --worktree
/home/penguin/source/fledge/.fledge/worktrees/wave1/plan-doc --tab
verify-plan-doc --file brief.md --timeout 90s`, gave the same `partial` outcome,
the same `agent_not_ready` at phase `agent.prompt`, and the same immediate
success with `agent message --file`. It appears reliable for pi, not
intermittent. This is a recurrence of
the entry marked resolved 2026-09-19 ("First message after spawn is rejected as
not ready"), now on the pi harness.

**Reproduction steps:**
1. Spawn a pi agent with `--file`, for example `fledge agent spawn --name plan-reviewer --harness pi --model openai-codex/gpt-6-astra --tab plan-reviewer --file brief.md --timeout 90s`.
2. Observe the `partial` outcome with `agent_status` `idle` and `agent_not_ready` in phase `agent.prompt`.
3. Immediately run `fledge agent message --name plan-reviewer --file brief.md` and observe success.

---

**Issue:** Spawn from a linked worktree sends the linked checkout as the `worktree.create` source

**Summary:** `internal/agent/spawn/worktree.go` resolves the primary root from
`worktree.list` for the destination path but keeps the caller's linked `cwd` as
the `cwd` param of the following `worktree.create`/`worktree.open` call.
`reference/herdr/api/worktree.md` states those methods reject a linked source
with `linked_worktree_source`. The unit test
`TestNewWorktreeFromLinkedCheckoutUsesPrimaryRoot` scripts a successful response
for that request, so it does not catch it. Suspected from code and docs on
2026-09-21 at `dccfdf1`; not reproduced live. A fix is planned in the worktree
library extraction ([coordination plan](../plan/coordination-plan.md)).
Confirmed live on 2026-09-22 for the `worktree.open` path; see the next entry.

Resolved 2026-09-22: `worktree.Source.changeParams` (in
`internal/lib/worktree/request.go`, landed in `b76bcd0` "refactor: extract
worktree and .fledge handling into shared libraries") now addresses
`worktree.create`/`worktree.open` to the listed primary checkout
(`listing.Source.RepoRoot`) instead of the caller's linked `cwd`.

**Reproduction steps:**
1. From inside a linked worktree, run `fledge agent spawn --worktree new ...` without `--workspace`.
2. Observe the `cwd` param of the `worktree.create` call: it is the linked checkout, not the primary root.

---

**Issue:** `agent spawn --worktree PATH` without an explicit source fails with `linked_worktree_source`

**Summary:** On 2026-09-22, at `dev` `1aed95d` with Herdr 0.9.1, running from the
primary checkout `/home/penguin/source/fledge`:
`fledge agent spawn --name verify-plan-doc --harness pi --model openai-codex/gpt-6-astra --worktree /home/penguin/source/fledge/.fledge/worktrees/wave1/plan-doc --tab verify-plan-doc --file brief.md --timeout 90s`
returned status `rejected`, no effects, error code `linked_worktree_source`,
message "New and open worktree actions start from the repo parent workspace.",
phase `worktree.open`. Fledge sent the linked checkout path as the `cwd` source
because no `--cwd` or workspace selector was given (README: "Without an explicit
source, the absolute checkout path determines its repository"). Adding
`--cwd /home/penguin/source/fledge` let `worktree.open` succeed and cleared
`linked_worktree_source` (the agent and tab were created), but the spawn then
returned `partial` at `agent.prompt`, as recorded in the pi entry above. This confirms
live, for the open path, the suspected create-path issue recorded in the previous
entry; both stem from the same source handling in
`internal/agent/spawn/worktree.go`.

Resolved 2026-09-22: see the previous entry — the same `changeParams` fix
addresses `worktree.open` to the primary checkout as well.

**Reproduction steps:**
1. Create a managed worktree with `--worktree new`.
2. From the primary checkout, run `fledge agent spawn --worktree <that path> ...` with no `--cwd`/`--workspace`.
3. Observe `linked_worktree_source` at phase `worktree.open`.
4. Add `--cwd <primary checkout>` and observe that `worktree.open` succeeds and the failure, if any, moves to `agent.prompt`.

---

**Issue:** Stopping the last agent in a worktree workspace closes the workspace and orphans the checkout

**Summary:** On 2026-09-22 (`dev` at `1aed95d` plus branch `wave1/worktree-lib`
`a09f509`, Herdr 0.9.1), an independent verifier spawned a probe agent with
`fledge agent spawn --name wt-probe2 --harness pi --worktree new --branch wt-probe2 --no-wait --json`,
which created worktree `.fledge/worktrees/wt-probe2`, workspace `wN`, tab
`wN:t1`, and pane `wN:p1`. `fledge agent stop --name wt-probe2 --force --json`
succeeded and closed pane `wN:p1`. Because that was the workspace's only pane,
Herdr closed workspace `wN`. The prescribed cleanup
`herdr worktree remove --workspace wN --force` then failed with
`workspace_not_found` ("workspace wN not found"), and Herdr's `worktree.remove`
accepts only a workspace id, so the checkout directory and the `wt-probe2`
branch were left behind with no Herdr handle to remove them. The orchestrator
removed them with `git worktree remove --force` and `git branch -D`. Observed
once, reliably explained by Herdr's documented behaviour
(`reference/herdr/api/worktree.md`: remove requires `workspace_id`; a closed
worktree has no `open_workspace_id`). This is why the planned
`fledge worktree remove` ([coordination plan](../plan/coordination-plan.md))
must handle closed checkouts through git.

Resolved 2026-09-22: `fledge worktree remove` (`f116a62`, guarded against
live-agent use in `d96e91c`) removes an open checkout through Herdr and a
closed one through `git worktree remove`, keeping the branch.

**Reproduction steps:**
1. Spawn an agent with `fledge agent spawn --name probe --harness pi --worktree new --branch probe` so it is the only pane in a new worktree workspace.
2. Run `fledge agent stop --name probe --force`; observe the pane closes and the workspace disappears from `herdr workspace list`.
3. Run `herdr worktree remove --workspace <that id> --force`; observe `workspace_not_found`.
4. Observe `git worktree list` still shows the checkout and `git branch --list probe` still shows the branch.

---

**Issue:** pi agent loses its name shortly after a successful spawn

**Summary:** On 2026-09-22, `fledge agent spawn` (interactive picker, equivalent to
`fledge agent spawn --harness pi --name picker-probe`, default `30s` timeout)
reported success: "Spawned picker-probe (pi) in wS / wS:t3 / wS:p3". Seconds
later `fledge agent get --name picker-probe` and `fledge agent stop --name
picker-probe --force` both failed with `agent_not_found`, while `fledge agent get
--pane wS:p3` showed the pi harness running `idle` with `Name: -`. Unlike the
earlier short-timeout entry, the spawn did not time out. Observed once, Fledge
`wave2/picker` branch built from `dev` at `495f4db`, pi reporting an available
update to 0.87.0.

**Reproduction steps:**
1. Run `fledge agent spawn --name picker-probe --harness pi` in a Herdr pane and observe the success line.
2. Within a few seconds run `fledge agent get --name picker-probe`; observe `agent_not_found`.
3. Run `fledge agent list`; observe the pi agent in the new pane with name `-`.

---

**Issue:** Task completion does not notify the dispatcher

**Summary:** On 2026-09-22 at `dev` `d68e0c1`, a task created by registered
agent `pr14-dispatcher` was assigned to `pr14-worker-a`. The worker completed it
successfully, and the durable task record moved to `completed`, but the creator
received no Herdr message or other completion notification. The dispatcher had
to poll `fledge task list --status completed` or `fledge task get --id ...`.
`internal/task/complete` records only the state transition; unlike assignment,
it does not deliver a message. This leaves dispatchers without a built-in way to
be awakened when another agent finishes a task.

Resolved 2026-09-22: `task complete` now sends the result and verification
command to a distinct live `created_by` agent, records the delivery outcome, and
reports failed or uncertain notifications without rolling completion back.

**Reproduction steps:**
1. Adopt or spawn a registered dispatcher and worker in the same repository.
2. As the dispatcher, create a task and assign it to the worker.
3. As the worker, run `fledge task complete --id <task-id> --summary done`.
4. Observe that the task record is `completed` but no message is delivered to the dispatcher.

---

**Issue:** `agent spawn --cwd` with a relative path starts the agent in the wrong directory

**Summary:** On 2026-09-22 with Fledge built from `dev` `a61f7ae`, running
`fledge agent spawn --name review-agent --harness claude --cwd
.fledge/worktrees/fix/agent-cmds --tab review-agent --file brief.md` from
`/home/penguin/source/fledge` launched Claude in `/home/penguin` instead of the
worktree. Claude showed its folder-trust dialog for `/home/penguin`, and spawn
reported "Agent is waiting on a startup prompt". An absolute `--cwd` works. The
relative path appears to be resolved against something other than the caller's
working directory (likely the Herdr server's or the pane shell's home). Spawn
should resolve a relative `--cwd` against the caller's working directory or
reject relative paths.

Resolved 2026-09-22: `agent spawn` now resolves a relative `--cwd` against the
caller's working directory before sending it to Herdr; an absolute `--cwd` is
unchanged.

**Reproduction steps:**
1. From the repository root, create a worktree such as `.fledge/worktrees/fix/agent-cmds`.
2. Run `fledge agent spawn --name review-agent --harness claude --cwd .fledge/worktrees/fix/agent-cmds --tab review-agent --file brief.md`.
3. Observe that spawn reports the agent waiting on a startup prompt and that Claude's folder-trust dialog names `/home/penguin`, not the worktree.
4. Repeat with the absolute worktree path as `--cwd`; observe that the agent starts in the worktree.

---

**Issue:** No way to change a live agent's model

**Summary:** On 2026-09-22 with Fledge built from `dev` `a61f7ae`, switching a
running Claude worker from Sonnet 5 to Opus 5.5 required `agent stop`, a
respawn with `--worktree <path> --model claude-opus-5-5`, and `task assign`
again, which creates a new agent record ID. `agent message` cannot carry a
harness slash command such as `/model claude-opus-5-5`, because every delivered
message is prefixed with the sender header line, so the text no longer begins
with `/`. Possible capabilities: a raw, unheaded send option, or
`agent set-model`. Resolved 2026-09-22: `agent send` (`1bb9b98`) types raw
text and keys with no header, so
`fledge agent send --name worker --text "/model claude-haiku-4-5-20251001" --key enter`
switched a live Claude worker from Sonnet 5 to Haiku 4.5 in place. Side effect:
Claude Code's `/model` also saves the model as the global default for new Claude
sessions, rewriting `~/.claude/settings.json`.

**Reproduction steps:**
1. Spawn a Claude worker with `fledge agent spawn --name worker --harness claude --model claude-sonnet-5`.
2. Run `fledge agent message --name worker --body "/model claude-opus-5-5"`.
3. Observe that the delivered text begins with the Fledge sender header, so Claude treats it as a prompt rather than a slash command and the model is unchanged.
4. Observe that the only way to switch models is `fledge agent stop --name worker`, a respawn with `--model claude-opus-5-5`, and reassigning its task, which yields a new record ID.

---

**Issue:** A new harness in a reused terminal inherits the previous agent record

**Summary:** On 2026-09-22, Fledge `dev` at `d535618` (`agent current` added in
`ff7c96c`), Herdr 0.9.1, record `b2934d28`
(`.fledge/state/agents/b2934d28.json`) was adopted at 2026-09-22T14:56:33Z for a
Codex agent named `pr14-dispatcher` in terminal `term_65c12d047118e1` (pane
`wZ:p1`). That Codex agent later exited and a Claude Code agent was started in
the same terminal; it has no live Herdr name. `fledge agent current` from that
Claude agent reports Fledge ID `b2934d28`, Name `pr14-dispatcher`, Harness
`codex`; children spawned by the Claude agent record parent `b2934d28`, and
`agent get --pane wZ:p1` showed Name `-` but the record fields. Cause: identity
is keyed by `terminal_id`, and per README Identity a terminal whose harness
exits keeps its record live, so the next harness launched in it is treated as
the same agent. Possible fixes: end or invalidate the record when the harness
kind or Herdr session reference changes, or compare the recorded harness
against the live agent on lookup. Resolved 2026-09-22: a lookup that finds a
record's terminal running a different known harness ends the record and treats
the terminal as unregistered; listings skip such records.

**Reproduction steps:**
1. Adopt an agent in a pane (`fledge agent adopt --name a`).
2. Exit that harness, leaving the shell.
3. Start a different harness in the same pane.
4. Run `fledge agent current` there and observe the old name and harness.

---

**Issue:** A verified task cannot be reopened for re-verification after repairs

**Summary:** On 2026-09-22, task `d1ba4881` (`agent send`) was verified by
`send-verify` on the first pass while verification findings F1-F3 were still
open. After the fix commit `48f5817`, the verifier's second
`fledge task verify --id d1ba4881` was rejected with `task_invalid_state`,
because `verified` is terminal and no command reopens or re-completes a task.
The task record therefore reflects the earlier commit, not the re-verified one.
Workaround: rely on the verifier's message for the final result.

**Reproduction steps:**
1. Assign a task, complete it, and run `fledge task verify --id <task>`.
2. Commit follow-up repairs for findings from that verification.
3. Run `fledge task verify --id <task>` again.
4. Observe `task_invalid_state`, and that neither `task complete` nor any other
   command can move the task back to a verifiable state.

---

**Issue:** `agent stop` refuses a finished agent still reported as working

**Summary:** At about 6:45 PM on 2026-09-22 the Claude verifier `docs-verify`
(pane `w1J:p2`) had sent its final report, and its terminal showed an empty
prompt with `Worked for 51s · done 6:45 PM`. Right afterwards,
`fledge agent stop --name docs-verify` was rejected with
`agent docs-verify is working; pass --force to stop it anyway (guard)`.
`fledge worktree remove` then also refused because the live agent was in the
workspace, which its `--force` does not override, so cleanup needed
`agent stop --force`. This is the inverse of "`agent get` still reports `idle`
just after a successful message" above: the reported status lags the
terminal in both directions. Workaround: check `fledge agent read` for an idle
prompt, then stop the agent with `--force`.

**Reproduction steps:**
1. Let a spawned Claude agent finish its turn right after sending a message.
2. Confirm with `fledge agent read --name <agent>` that it shows an idle prompt.
3. Immediately run `fledge agent stop --name <agent>`.
4. Observe the `is working; pass --force` guard rejection.

---

**Issue:** A registered but unnamed agent cannot be given a name

**Summary:** On 2026-09-22 the orchestrator's Claude pane (`wZ:p1`) had a live
record (`27392de9`, registered by `adopt`) but no Herdr name. `agent adopt
--name <name>` refuses a terminal that already has a live record, and no other
Fledge command sets a name, so the orchestrator stayed unnamed all session.
Workers could only reply with `agent message --pane wZ:p1`, and every message
it sent was headed `unnamed agent (wZ:p1)` with no reply command.

**Reproduction steps:**
1. Run `fledge agent adopt` in a pane without `--name` while the agent is unnamed, or register it some other way without a name.
2. Run `fledge agent adopt --name orchestrator` in the same pane.
3. Observe the refusal naming the existing record id, and that no command renames it.

---

**Issue:** `agent wait` keeps waiting after its target agents are stopped

**Summary:** On 2026-09-22 the orchestrator ran `fledge agent wait --name
impl-worktree --name impl-state --name impl-commands --any --until
blocked --until done --until idle --timeout 3h` in the background, then stopped
`impl-worktree` and `impl-commands` with `agent stop --force`. The wait
neither returned nor reported the closed panes; it had to be killed by hand.
A later `--any` wait over respawned agents of the same names reported
`impl-state was cancelled` and `impl-commands is done (first match)`.

**Reproduction steps:**
1. Spawn two agents and start `fledge agent wait --name a --name b --any --until idle` while both are working.
2. Run `fledge agent stop --name a --force` and `fledge agent stop --name b --force`.
3. Observe that the wait keeps running instead of returning a gone or failed result for each target.

---

**Issue:** `agent spawn --cwd <other repo>` registers the agent in the invoking repository

**Summary:** On 2026-09-22 a verifier working in a `fledge` worktree ran
`agent spawn --no-wait --cwd <throwaway repo>` to probe spawn behavior. Both
probe records (`cc5cf5f8`, `c27617b2`) were written to the primary checkout's
real `.fledge/state/agents/`, not to the throwaway repo named by `--cwd`.
The probes then blocked at Claude's trust dialog and had to be stopped by the
orchestrator. Workaround: run any spawn that registers from a working directory
inside the repository whose state should hold the record.

**Reproduction steps:**
1. From inside repository A, run `fledge agent spawn --harness claude --name probe --no-wait --cwd <repository B>`.
2. Look in A's `.fledge/state/agents/` and in B's.
3. Observe the record in A, and nothing in B.

---

**Issue:** Spawn with `--prompt` into a new folder stops at Claude's trust dialog and drops the prompt

**Summary:** On 2026-09-23 a verifier spawned a Claude probe with `--prompt`
and `--cwd` pointing at a new throwaway repository. Spawn returned with the
agent `blocked` on Claude's folder-trust dialog, and the first prompt was never
delivered. The verifier accepted the dialog with `agent send --key down --key
enter` and resent the command with `agent send`. See also the folder-trust
entry above for spawns through a worktree.

Resolved 2026-09-23: the result now carries `prompt_requested`, so
`prompted=false` with `prompt_requested=true` marks an unsent first prompt. The
human output says the first prompt was not submitted and gives
`fledge agent read`/`send`/`message --pane P` hints to resolve the dialog and
resend it. Spawn deliberately does not queue, replay, or answer the dialog.

**Reproduction steps:**
1. Create a new git repository Claude has never trusted.
2. Run `fledge agent spawn --harness claude --name probe --cwd <repo> --prompt "echo hi"`.
3. Observe spawn report the agent blocked on a startup prompt, and that after granting trust the prompt was never submitted.

---

**Issue:** Claude workers in auto mode stall on permission denials

**Summary:** On 2026-09-22 a Claude verifier spawned in the default auto
permission mode had one `agent stop --force` denied by the auto-mode
classifier. It then had `rm` and, finally, a read-only `git status` denied as
"auto-mode bypass" attempts, so it stopped mid-verification without running
`task verify` or cleaning up its probe agent. The orchestrator hit the same
classifier ("Create Unsafe Agents") when it prepared bypass-mode spawns.
Workaround: spawn Claude workers with
`fledge agent spawn ... -- --permission-mode bypassPermissions` (with
`skipDangerousModePermissionPrompt` set, no dialog appears). Fledge has no
first-class permission-mode option.

**Reproduction steps:**
1. Spawn a Claude agent in auto mode and have it run `fledge agent stop --name <another agent> --force`.
2. Observe the classifier denial, then denials of unrelated follow-up commands.
