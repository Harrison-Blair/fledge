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
`/tmp/fledge-dev`.

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

**Reproduction steps:**
1. Message a worker agent and wait for it to go idle.
2. Try to obtain its reply using only `fledge agent --help` commands.
3. Observe no such command exists; fall back to `herdr pane read`.

---

**Issue:** No command to answer a blocked agent

**Summary:** A worker waiting on a permission prompt or question can only
be answered with `herdr pane send-text` / `herdr pane send-keys`, not with
any Fledge command. Observed 2026-09-18, Fledge 0.0.3 built from `dev`
(`fc4538b`), Herdr 0.9.1, binary `/tmp/fledge-dev`.

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
0.9.1, binary `/tmp/fledge-dev`.

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
