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

Resolved 2026-09-23 for pi as well: see the resolution of "`spawn --file` on
the pi harness fails the prompt with `agent_not_ready`" below. Spawn now also
waits for `interactive_ready` and a cleared `launch_pending`, not only a
lifecycle status.

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

Resolved 2026-09-23: measured on `dev` `b833d6f`, the lag is sub-second, not
seconds. `agent.prompt` acknowledges once text and Enter are written, and
Herdr's screen detection sees `working` 0.1-0.3 s after `message` returns; until
then `agent get` shows the previous settled state, `idle` or `done` (12/13
Claude immediate gets; pi was already `working` 5/5). `agent message --confirm`
(`790c0f0`, `8dc6b66`, `1cb3596`) makes Herdr wait for observed activity after
submission: live, 10/10 Claude and 5/5 pi returned `working` at command return
in 0.5-0.8 s, while a wait-less mutation returned the stale `done` 15/15. Plain
`agent message` is unchanged and still confirms submission only, so loops of the
form `message; wait --until idle` should use `--confirm`.

**Reproduction steps:**
1. Message an idle agent with `fledge agent message --name w --body "..."`.
2. Immediately run `fledge agent get --name w`.
3. Observe the status still reads the previous settled state (`idle` or
   `done`) for about 0.1-0.3 s.

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

Resolved 2026-09-23: the cause is Herdr's `agent.start` reservation, which
Fledge set to the local `--timeout`; when it expired mid-launch, Herdr's
teardown dropped the name. Spawn now sends a reservation of the larger of
`--timeout` and 30s (`ff3ce2d`), while its own budget stays `--timeout`. Live pi
runs at `--timeout 3001ms` (file, bare, and `--no-wait`, 3 each) lost the name
9/9 on `dev` `b833d6f` at 3.06-3.47 s, 6 of them after reporting success, and
0/9 on the branch through 36 s, in both the implementer's and the verifier's
runs. A short timeout can now return `partial` while Herdr still finishes the
launch under its name. A launch still unfinished when the 30s reservation
expires can still lose its name.

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

Observed again 2026-09-23 with a role-only first prompt: `agent spawn --profile
verifier` (pi, openai-codex/gpt-6-astra) built from feat/agent-profiles returned
partial `agent_not_ready` at agent.prompt, so the profile's role text was not
delivered. The orchestrator resent it with `agent message`. Profiles that
resolve to pi are therefore affected by this entry too.

Observed again 2026-09-23 at `dev` `b833d6f`: `agent spawn --profile planner
--name backlog-astra --timeout 90s` (no `--file`) returned partial
`agent_not_ready` at agent.prompt, and an `agent message --file` sent right
after it was also rejected with `agent_not_ready`, while `agent get` already
showed the agent named and `idle` with `Interactive ready: true`. A second
`agent message` a few seconds later was delivered. So the immediate resend is
not a reliable workaround either.

Resolved 2026-09-23: pi reaches lifecycle `idle` at about 0.7 s while
`launch_pending` is still true and `interactive_ready` is unset; launch clears
at 3.1-3.5 s. Spawn now polls the started pane after the lifecycle wait until
`interactive_ready` is true, `launch_pending` is clear, and the status is
`idle` or `done`, with or without a first prompt (`124d096`). It fails closed if
a different terminal, name, or harness answers. `--timeout` is now one budget
from launch through the first prompt, and no prompt is sent after its deadline
(`f985675`, documented in `3e211d3` and `7a20b17`). Live on `dev` `b833d6f`,
10/10 pi `--file` spawns and 5/5 role-only profile spawns failed
`agent_not_ready` at 0.6-0.8 s. On the branch, with no resend, the
implementer's runs delivered 10/10 file and 5/5 role prompts with one exact
reply each, and the verifier's delivered 10/10 file prompts, 10/10 more with an
exact reply line each, and 5/5 role prompts; spawn takes about 3.3-3.6 s. Claude spawns were unchanged at about 4.3 s. No
prompt retry was added.

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

Still open 2026-09-23, cause unknown. Wave-3 sightings at `dev` `b833d6f` spawn
code, Herdr 0.9.1: the planners saw 0 of 9 pi spawns lose their name through
36 s. `ws-b` saw it once: `b5-p1`, a default spawn without a prompt, was named
and `idle` about 5 s after spawn and unnamed by about 80 s, while its pane, pi,
and its Fledge record survived. `ws-d` saw it once: `d6-p1`, idle and never
prompted, had lost its name about 3 minutes after spawn; `agent get --pane`
showed a live idle pi with `name: null`. `verify-a` saw 0 of 30 in the branch
runs, each observed through 37 s. Losing the name at default timeout is not
fixed by the readiness gate or the 30s start reservation. Workaround: target the
agent by `--pane` or `--id`.

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

Resolved 2026-09-23: `fledge task verify` now accepts a `verified` task and
applies the same checks again; the repeat replaces the verifier, note, forced
flag, and time, keeping only the latest verification.

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

Resolved 2026-09-23: two effects were measured on `dev` `b833d6f`. The dominant
trigger is a report sent from a tool call before the worker's turn ends: the
worker keeps generating, and settles `done` 1.2-1.5 s later, so a stop fired on
the report was refused 5/5 (4 Claude, 1 pi). The second, smaller effect is
detection lag at turn end, at most 231 ms (5/5). A non-forced stop now gives a
`working` agent up to `--grace` (default 5s, 0s through 60s; `0` refuses at
once) to settle `idle` or `done` in the same terminal before refusing
(`99d8558`, `de26175`); `blocked` and `unknown` get no grace, and `--force`
never waits, so `--grace` with `--force` is invalid. The stop client's transport
limit is sized to the grace (`9be0e00`), so a grace above 15s is not cut short.
`agent cleanup`'s stop recheck inherits the default 5s. Live, the verifier's
report-then-stop cycles stopped 10/10 without `--force` in 1.5-4.0 s (baseline
refused 5/5); the implementer's runs stopped 14/15, the miss being a turn that
was really still working at 5 s ("Cogitated for 8s"). A genuinely busy agent is still refused after
about 5.1 s (3/3), and `--grace 60s` stopped a real agent that settled after
18.2 s.

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

Resolved 2026-09-23: `agent adopt --name <name>` on a terminal with a live
record whose live Herdr agent is unnamed now names it through Herdr and keeps
the same record (ID, parent, registration, worktree), storing the new name and
current pane. Whether the agent is unnamed is decided from Herdr, not from the
record's stored name. Renaming a named agent is still refused.

**Reproduction steps:**
1. Register an agent while it is named (`fledge agent adopt --name worker`, or `fledge agent spawn --name worker`), then clear its Herdr name so the live agent is unnamed while its record stays live. (`fledge agent adopt` without `--name` on an unnamed agent is itself refused, so it cannot create such a record.)
2. Run `fledge agent adopt --name orchestrator` in the same pane.
3. Before the fix: observe the refusal naming the existing record id, and that no command renames it.

---

**Issue:** `agent wait` keeps waiting after its target agents are stopped

**Summary:** On 2026-09-22 the orchestrator ran `fledge agent wait --name
impl-worktree --name impl-state --name impl-commands --any --until
blocked --until done --until idle --timeout 3h` in the background, then stopped
`impl-worktree` and `impl-commands` with `agent stop --force`. The wait
neither returned nor reported the closed panes; it had to be killed by hand.
A later `--any` wait over respawned agents of the same names reported
`impl-state was cancelled` and `impl-commands is done (first match)`.

Corrected 2026-09-23: on `dev` `b833d6f` with Herdr 0.9.1, stopping *all*
targets ends the wait promptly: a single target returned `agent_not_running`
0.08-0.7 s after its stop, and `--any` over targets that were all stopped
returned 0.07-0.24 s after the last stop. The historical hang did not reproduce
and its cause is unproven. The real symptom is `--any` with some targets stopped
and others alive: it correctly keeps waiting on the survivors, but printed
nothing (0 bytes for 8 s and more) until the whole wait ended, so the stopped
targets were invisible.

Resolved 2026-09-23: without `--json`, `agent wait --any` now writes one stderr
line per target that fails while others are still waited on (`2fc07cd`,
`d16dec9`), for example `c4-k1 failed: agent_not_running: agent is no longer
running in the target pane (still waiting on 2 targets).`. Live, each line
appeared 0.08-0.23 s after its `agent stop` (3/3 runs, and 3/3 in the
verifier's runs); JSON output stays a single final outcome.

**Reproduction steps:**
1. Spawn three agents and start `fledge agent wait --name a --name b --name c --any --until working` while all are idle.
2. Run `fledge agent stop --name a --force` and `fledge agent stop --name b --force`.
3. Before the fix: observe that the wait keeps running with no output at all until `c` is also stopped. Stopping all three ends it promptly.

---

**Issue:** `agent spawn --cwd <other repo>` registers the agent in the invoking repository

**Summary:** On 2026-09-22 a verifier working in a `fledge` worktree ran
`agent spawn --no-wait --cwd <throwaway repo>` to probe spawn behavior. Both
probe records (`cc5cf5f8`, `c27617b2`) were written to the primary checkout's
real `.fledge/state/agents/`, not to the throwaway repo named by `--cwd`.
The probes then blocked at Claude's trust dialog and had to be stopped by the
orchestrator. Workaround: run any spawn that registers from a working directory
inside the repository whose state should hold the record.

Resolved 2026-09-23: documented as intended. The invoking repository owns
coordination state; `--cwd` only places the shell. See README "Identity" and
`fledge agent spawn --help`.

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
Update: every Claude built-in profile now ships this flag, so `--profile` spawns
of Claude roles run in bypass mode without the extra `--` arguments.

**Reproduction steps:**
1. Spawn a Claude agent in auto mode and have it run `fledge agent stop --name <another agent> --force`.
2. Observe the classifier denial, then denials of unrelated follow-up commands.

---

**Issue:** An interrupted `agent spawn` can leave a live agent with no Fledge record

**Summary:** On 2026-09-23 an orchestrator ran a loop of three
`fledge agent spawn --harness claude --worktree new --branch <b> --base dev ...`
commands. The user interrupted the tool call. All three agents still started in
new worktrees. The third, `impl-d`, had a live Herdr name but no Fledge record:
`fledge agent list` showed ID `-` and `worktree list` showed OWNER `-`. The
likely cause is that the interruption landed between agent start and
registration; this is not confirmed. Workaround: `fledge agent adopt --pane
<pane> --name <name>` registered it (as `dbf053f3`). That record has no
worktree association, because adopt does not record one. A later `spawn` with
the same names was rejected with "agent name ... is already in use
(preflight)", which showed that the agents existed.

**Reproduction steps:**
1. Run `fledge agent spawn --harness claude --worktree new --branch <b> --base dev ...`.
2. Interrupt the fledge process (SIGINT) after the agent starts but before spawn prints its result.
3. Run `fledge agent list` and look for the missing ID.

---

**Issue:** `worktree remove` reports branches merged into dev as unmerged.

**Summary:** on 2026-09-23 the orchestrator removed three wave-1 checkouts whose
branches were already merged into `dev` (confirmed with
`git merge-base --is-ancestor <branch> dev`). `fledge worktree remove --branch
<b>` refused each one with "is not known to be clean and merged (merged: no)"
because its merged check uses fledge.baseBranch, then origin/HEAD, then main,
and this repository integrates into dev before main. The workaround was --force
after checking ancestry by hand. `fledge agent cleanup` (feature #12) avoids
this for checkouts its workers created, by using the base recorded at spawn, but
plain `worktree remove` and `worktree list`'s MERGED column still use the
repository-wide policy. Setting `git config fledge.baseBranch dev` would be the
manual fix.

**Reproduction steps:**
1. Merge a feature branch into dev only.
2. Run `fledge worktree list` and see MERGED no.
3. Run `fledge worktree remove --branch <b>` and see the refusal.

---

**Issue:** A verification task gated with `--after` on the task it verifies can never be assigned without `--force`

**Summary:** On 2026-09-23 (`dev` `b833d6f`) the orchestrator created each
wave-3 verify task with `task create --after <impl task>`, to show that
verification follows implementation. `task assign` treats a prerequisite as
satisfied only when it is `verified` or `cancelled`, but the verify task's whole
job is to verify that implementation task, so `assign` refused with
`task_dependencies_unmet` until `--force` was passed. There is no "after
completed" dependency kind.

**Reproduction steps:**
1. `fledge task create --title impl --body x` → I; `fledge task create --title verify --after I --body y` → V.
2. Assign I and complete it (`task complete --id I`).
3. `fledge task assign --id V --name <verifier>` → `task_dependencies_unmet`; only `--force` works.

---

**Issue:** `agent cleanup` from an isolated probe repository has no registered caller

**Summary:** On 2026-09-23 verifier `verify-c`, registered in the project
repository, spawned and stopped 32 named probes from a throwaway repository, as
required so that probe records stay out of real state. `agent cleanup --dry-run
--json` run there returned `caller_unregistered`, because records belong to one
repository and the verifier's record is in the project repository. `verify-d`
hit the same refusal with `--dry-run --results-collected`. Bulk cleanup of
probes therefore needs a list of probe names plus explicit
`agent stop --name ... --force` calls.

**Reproduction steps:**
1. As an agent registered in repository A, create throwaway Git repository B.
2. `cd B` and spawn a named probe with absolute `--cwd B`.
3. `cd B` and run `fledge agent cleanup --dry-run --json`; observe `caller_unregistered`.

---

**Issue:** Claude's trust dialog defaults to "No, exit", and `agent send` once reported a dialog-blocked probe `idle`

**Summary:** On 2026-09-23, during wave-3 planning and verification, every
Claude probe spawned into a fresh folder stopped at Claude's folder-trust
dialog with the cursor on "No, exit". The workaround documented earlier,
`agent send --key enter`, would therefore exit Claude; `agent send --key down
--key enter` is needed, after reading the screen with `agent read`. Separately,
once during planning, `agent send` reported probe `o2-b` as `idle` before
sending while its trust dialog was on screen; the sibling probe `o2-a`, on the
same dialog, reported `blocked`. Seen once and not pursued.

**Reproduction steps:**
1. Spawn a Claude agent with `--cwd` pointing at a new Git repository Claude has never trusted.
2. Run `fledge agent read --name <agent>` and observe the trust dialog with "No, exit" selected.
3. Run `fledge agent send --name <agent> --key down --key enter` to trust the folder; `--key enter` alone exits Claude.

---

**Issue:** Spawn can overrun `--timeout` while another Fledge process holds the state lock

**Summary:** Known limitation, accepted 2026-09-23. Spawn's `--timeout` bounds
every Herdr request from launch through the first prompt, but registration's
local step, taking the state lock and writing the record, uses a blocking file
lock and cannot be interrupted. `verify-a` held only a throwaway repository's
state lock for 5 s, and a ready `--timeout 3001ms --prompt ...` spawn against
a fake Herdr socket returned
at 5.0 s: `partial` timeout, prompt not sent, record kept. No prompt is sent
after the deadline either way. The state lock is normally held for
milliseconds, so the orchestrator accepted this without a code change; README
and `agent spawn --help` document it (`7a20b17`).

**Reproduction steps:**
1. In a throwaway repository, hold an exclusive `flock` on `.fledge/state/lock` from another process for 5 s.
2. Spawn a ready agent with `--timeout 3001ms --prompt "..."`.
3. Observe spawn return after about 5 s with a `partial` timeout and the prompt not sent.

---

**Issue:** `agent read` with a small `--lines` can miss a Claude reply

**Summary:** On 2026-09-23 two wave-3 implementers checked probe replies with
`fledge agent read --lines 40` and missed replies that had been delivered.
`impl-a` found that Claude repaints its screen, so the default
`recent-unwrapped` source with 40 lines could miss the reply;
`--source recent --lines 200` found it. `impl-d` found that on a fresh Claude
pane the startup banner and earlier turns pushed replies out of a 40-row
window; a 1000-row read showed each reply exactly once. Use a larger `--lines`
(and `--source recent` for Claude) when checking whether a reply arrived.

**Reproduction steps:**
1. Spawn a Claude agent and have it answer a few short prompts, each with a unique token.
2. Run `fledge agent read --name <agent> --lines 40` and look for the first token's reply line.
3. Run `fledge agent read --name <agent> --source recent --lines 200` and find it.

---

**Issue:** `task complete` loses the creator notification while the creator's pane is blocked

**Summary:** On 2026-09-23, during a planning session, the orchestrator (agent
`c3c07b6b`, pane `wA:p1`, Claude Code) created planning tasks `5d6ab497`,
`9af44645`, and `537c1916` and assigned them to planners `plan-a`, `plan-c`, and
`plan-d`. Each planner ran `fledge task complete` while the orchestrator's Claude
pane was showing an interactive question dialog (Claude Code's AskUserQuestion
UI), which Herdr reports as `blocked`. In all three cases `task complete`
recorded the completion but returned `partial`: the completion notification to
the creator failed with `agent_blocked`, and the command states it is never
retried. Workaround: each planner ran `fledge agent wait` and re-sent the notice
by hand with `fledge agent message --name orchestrator`. An orchestrator that
uses a question dialog therefore routinely misses completions unless workers
resend them.

**Reproduction steps:**
1. Register a creator pane, then create a task and assign it to a worker.
2. Put the creator's harness into a blocking dialog, such as Claude Code's AskUserQuestion UI.
3. From the worker, run `fledge task complete --id <task> --summary "..."`.
4. Observe `partial` with `agent_blocked` and no later delivery of the notification.

---

**Issue:** The orchestrator pane lost its Herdr agent name mid-session

**Summary:** On 2026-09-23, `fledge agent current` in pane `wA:p1` showed `Name:
orchestrator` (record `c3c07b6b`). Shortly after, while spawning and messaging
planners, `fledge agent list` showed the NAME column for `c3c07b6b` as `-`, and
messages sent from that pane carried the header `unnamed agent (wA:p1)` with no
reply command. The cause is unknown; it was not investigated. Workaround:
`fledge agent adopt --name orchestrator` in that pane restored the Herdr name and
kept the same record ID (output: `Adopted orchestrator (wA:p1) as c3c07b6b.`);
later messages showed `from orchestrator (wA:p1)`. The reproduction below records
what was observed; the exact trigger is not known.

**Reproduction steps:**
1. Register an agent with a name and confirm it with `fledge agent current`.
2. During normal spawn, assign, and message use, run `fledge agent list`.
3. Observe the NAME column as `-` and messages sent with an unnamed sender header.
4. Run `fledge agent adopt --name <name>` in that pane to restore the name.

---

**Issue:** Every agent's identity is lost after a machine restart

**Summary:** On 2026-09-23 at about 23:29 EDT the user restarted their machine.
Herdr came back (process start 23:29:14) with the same workspace and pane IDs
(for example `wA:p1` and `w19:p1`), and the running agents' panes survived.
Afterwards `fledge agent list` showed `-` for ID and PARENT for every agent,
including agents spawned and registered minutes earlier (`impl-b1`, record
`7bf1f804`, not ended, worktree set). `fledge agent current` in the orchestrator
pane failed with `caller_unregistered`. The orchestrator's Herdr name was also
gone, while the `impl-*` panes kept their names. Agent records match live agents
by terminal (records store `terminal_id`); after the restart none matched.
`fledge agent adopt --name orchestrator` registered the orchestrator under a new
record ID (`73f2ff4a`; the old one was `c3c07b6b`). There is no way to reattach
an existing record, so task records keep stale owner and creator IDs: an owner
must use `task complete --force`, and completion notifications to the creator's
old record cannot be delivered. Workaround: workers also report to the
orchestrator with `fledge agent message`. Related backlog idea:
`enhancement-ideas.md` #25 "Recover after coordinator interruption".

**Reproduction steps:**
1. Spawn and register agents, then create and assign tasks.
2. Restart the machine (or Herdr) so the panes are restored.
3. Run `fledge agent list` and `fledge agent current`.
4. Observe no attribution: ID and PARENT are `-`, and `current` fails with `caller_unregistered`.
5. Run `fledge agent adopt --name <name>` and observe a new record ID, so task ownership no longer matches.

---

**Issue:** Message reply hint omits --body

**Summary:** The header on every received message ends with
`reply: fledge agent message --name <name>`, which does not show that the text
must go in `--body` or `--file`. `impl-d4` copied the hint and appended the text
as a positional argument, and `fledge agent message` failed with `unknown
command`. Observed once. Workaround: pass the text with `--body "..."` or
`--file <path>`.

**Reproduction steps:**
1. Receive a message from a named agent and copy the reply command from its header.
2. Append the reply text as a positional argument, for example
   `fledge agent message --name orchestrator "done"`.
3. Observe the `unknown command` error; the same command with `--body "done"` succeeds.

---

**Issue:** Primary checkout .fledge/tmp/ vanished (cause unknown)

**Summary:** On 2026-09-23 between about 23:30 and 23:41 EDT, the orchestrator's
`.fledge/tmp/` (plans and working files) disappeared from
`/home/penguin/source/fledge` while wave-2 agents ran. An investigation found no
Fledge code or test that removes it: no non-test code calls `RemoveAll` or
`git clean`, and the tests that touch `.fledge` use per-test roots or
`t.Chdir(t.TempDir())`. The directory was later recreated, and the plans were
recovered from agent transcripts. Not reproduced; recorded as unexplained data
loss. Workaround: keep durable planning artifacts in task records, or commit them.

**Reproduction steps:**
1. Not reproduced. The directory was present before wave-2 work began and gone
   by about 23:41 on 2026-09-23, with no known trigger.

---

**Issue:** fledgedir.Ensure append race can duplicate the managed .fledge/.gitignore block

**Summary:** `fledgedir.Ensure` reads the ignore file
(`internal/lib/fledgedir/fledgedir.go:78-87`) and later appends the missing
rules with `O_APPEND` (`appendIgnoreRule`, about line 150) without a lock or a
re-check. Two concurrent calls on a root with no managed `.fledge/.gitignore`
can both read it as unmanaged and both append the full block. The primary
checkout's `.fledge/.gitignore` (mtime 2026-09-23 22:31, during parallel spawns)
contains the 3-line managed block twice. Plausible but not proven as the cause
of that file. A repeated `Ensure` on an already-managed file does not duplicate
the block, so the duplicate is harmless to Git but untidy.

**Reproduction steps:**
1. Use a root with no `.fledge/.gitignore`.
2. Run two `fledgedir.Ensure` calls on it concurrently.
3. Observe the managed block appended twice (plausible, not proven).

---

**Issue:** Agent status reports done after provider failure

**Summary:** Two `pi` verifiers running `opencode-go/kimi-k3` hit `402 Insufficient
account funds` (the 5-hour usage window was exhausted) and stopped with `Retry
failed after 3 attempts`. `fledge agent list` and `fledge agent wait` reported
both as done, indistinguishable from a finished verification. The orchestrator
only noticed by reading the pane. Observed twice. Workaround: read the pane
(`fledge agent read`) before trusting a done status from a `pi` agent on a
quota-limited provider.

**Reproduction steps:**
1. Spawn a `pi` agent on a provider with no remaining quota.
2. Send it a prompt and let it fail with the provider error.
3. Run `fledge agent list` or `fledge agent wait` and observe the agent reported as done.

---

**Issue:** Markdown scratch files under `.fledge/tmp/` appear untracked in some worktrees

**Summary:** In a linked worktree with no `.fledge/.gitignore` (one created before
managed worktrees received that file), the root allowlist `.gitignore` applies to
`.fledge/tmp/`. Its `!*.md` rule re-admits Markdown files there, so
`.fledge/tmp/plan-brief.md` showed as `??` in `git status`. `.toml` proposals stayed
ignored (`.gitignore:2:*`). Observed once, 2026-09-24, while dogfooding the planner
(task A5) in `feat/planner-docs`. Workaround: write scratch Markdown elsewhere, or
delete it before committing.

**Reproduction steps:**
1. Use a linked worktree of this repository that has no `.fledge/.gitignore`.
2. Write `.fledge/tmp/note.md`.
3. Run `git status --short -uall` and observe `?? .fledge/tmp/note.md`.

---

**Issue:** Spawned agents use the installed `fledge`, not the checkout build

**Summary:** When the installed binary lacks commands from this checkout (here
`task template` and `task import`), AGENTS.md says to build a temporary binary and use
it consistently. A spawned agent still runs `fledge` from `PATH`, and `--env` applies
only to ordinary shells, so the planner could not reach the new commands by default.
Workaround: name the temporary binary's absolute path in the brief. Observed
2026-09-24 in the A5 planner dogfood.

**Reproduction steps:**
1. Build the checkout to `.fledge/tmp/bin/fledge` while an older `fledge` is on `PATH`.
2. Spawn a `pi` agent with that binary and `--profile planner`.
3. The agent's `fledge task template --proposal` resolves to the older binary and fails.

---

**Issue:** Planner role conflicts with the completion report file

**Summary:** The built-in planner role allows only the proposal file as a write, but
its Fledge protocol section says to finish with `fledge task complete --file
<report>`. The planner resolved this by passing the proposal TOML itself as the
report, prefixed with a comment block. The completion notification therefore carried
the whole proposal. Observed once, 2026-09-24, A5 planner dogfood (task 6adf25eb).

**Reproduction steps:**
1. Spawn `--profile planner` and assign it a planning task.
2. Let it finish.
3. Run `fledge task get --id <task>` and observe that the result is the proposal file.
