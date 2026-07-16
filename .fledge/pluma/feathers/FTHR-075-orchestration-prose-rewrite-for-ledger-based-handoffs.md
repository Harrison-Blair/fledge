---
id: FTHR-075
title: Orchestration prose rewrite for ledger-based handoffs
plumage: PLM-030
status: egg
priority: P1
depends_on: [FTHR-073, FTHR-074]
authored: 2026-07-16T22:24:20Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-075: Orchestration prose rewrite for ledger-based handoffs

## Description
Rewrites the agent-neutral orchestration prose so every state-bearing handoff it describes uses the PLM-030 ledger (FTHR-072/073/074's `fledge heartbeat`/`await`/`verdict`/`escalate`/`ledger read`) instead of a `message-peer` message carrying that state. `message-peer` itself is not removed — it's re-scoped in this prose to a stateless wake-up nudge ("I wrote a record, go check"), consistent with PLM-030's decision that the 6-primitive/tier model stays untouched. Kept as a single feather across all seven files (not split per-doc) to guarantee one consistent handoff vocabulary throughout, rather than risking seven independently-drifted descriptions of the same mechanism.

## Affected Modules
Source of truth only — never the scaffolded copies at this repo's own `.fledge/skills/` (root `CLAUDE.md`; `.fledge/nest/architecture.md` — Cross-module relationships):
- `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` — the shared "spawn prompt is entire context, one-shot lifecycle, final message" contract; add the heartbeat-before-long-operation instruction here since it's role-agnostic.
- `internal/bootstrap/core/skills/fledge-orchestrate/incubator.md` — relay envelope currently routes `GATE`/`QUESTION`/`SPAWN-REQUEST`/`PHASE-CLOSE` all as `message-peer`; PHASE-CLOSE-equivalent state (e.g. hatched plumage/feather IDs) can stay in the message body since it's a one-shot report, not an ongoing handoff — but per FTHR-072/074, the incubator↔orchestrator relationship as a "persistent named worker being tracked" (per the batched-questions Q3 answer) gains liveness (`fledge heartbeat`) and can use `fledge await` if/where a blocking wait already exists in prose (e.g. none currently — incubator's waits are all on the user via the orchestrator relay, so this file's main change is the heartbeat instruction plus a note that idle notifications forwarded to it are not to be trusted over ledger state, if any wait point uses one).
- `internal/bootstrap/core/skills/fledge-orchestrate/brooder.md`, `skua.md` — the skua's verdict currently travels as a `message-peer` pass/fail message to the orchestrator (per `.fledge/nest/architecture.md`/`conventions.md` on the team loop); rewrite to: skua writes `fledge verdict FTHR-### --result pass|fail`, orchestrator (or the brooder, if it's the one blocking) calls `fledge await FTHR-### --kind verdict` instead of waiting on a message. Escalations (brooder → orchestrator) become `fledge escalate <brooder-name> --message "..."`.
- `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` — the Commissioner section's whole "wait as a two-input state machine" apparatus (idle vs final message, `fledge nest status` as the one-time disambiguator) is the single biggest rewrite target: replace "wait for the by-name final message" with "call `fledge await <forager-name> --kind status --timeout <duration>` (or block indefinitely) for a `status: done` terminal value, written by the forager once `fledge nest status` itself reports complete" — collapsing the whole idle-notification-interpretation apparatus into one deterministic command. The forager still sends a final message too (for the coverage-summary content, which doesn't fit a terse ledger record), but the *decision of when to act* no longer depends on interpreting that message's arrival vs. a bare idle.
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` — §3.1 dispatch loop and §3.2 on-approval currently describe the skua's pass message and brooder idle-while-implementing; update to reference the ledger commands from brooder.md/skua.md above. §4 escalations references the same `fledge escalate` handoff.
- `internal/bootstrap/core/skills/fledge-orchestrate/planning.md` — step 0's incubator-lifecycle-notification-forwarding language, and step 2's forager Commissioner-wait reference, get updated to point at the rewritten `foraging.md`/`incubator.md` sections rather than duplicating the old message-based description.
- `.fledge/skills/fledge-orchestrate/*` in this repo (the scaffolded copy) — refreshed in FTHR-076, not this feather; do not hand-edit it here (root `CLAUDE.md`).

## Approach
- Work file by file in the list above, in the order given (worker-protocols.md first since it's the shared contract other files reference).
- Preserve every existing invariant these files assert that isn't about the *transport* of a handoff — e.g. skua's 3rd-rejection escalation rule, brooder's test-first discipline, green-teardown sequencing — only the "how does the other party learn this happened" mechanism changes from message content to ledger record + (optional) `fledge await`/nudge message.
- Introduce the heartbeat-before-long-operation instruction once, in `worker-protocols.md` (shared contract), rather than repeating it in each role file — role files reference it.
- Where a wait today is described as "you are re-invoked by events, never poll" (foraging.md's Commissioner section) — `fledge await` itself is the poll (with a bounded interval and timeout), run inside the *waiting* agent's own turn, not a manual sleep-loop the agent authors itself; this is a deterministic replacement for "wait for the harness to re-invoke me," not a return to manual polling. State this distinction explicitly in the rewritten foraging.md so it isn't confused with the "never `sleep`-poll" prohibition that the prose already establishes for other reasons.
- Don't invent new decisions while rewriting — every mechanism change here is a direct restatement of FTHR-072/073/074's already-gated command shapes; if a genuine new decision surfaces while drafting (e.g. exact wording of the heartbeat-before-long-operation instruction), gate it rather than deciding unilaterally, per this feather's oversight (unset — fully autonomous, but escalate to the commissioning orchestrator/user through normal facts-vs-decisions triage if something doesn't reduce cleanly).

## Tests
This feather's tests are prose-content assertions, following the established "guard test" pattern (`.fledge/nest/testing.md` → Unit test coverage by package → `internal/bootstrap`, ~15 existing guard tests like `TestBrooderFixLoopInvariant`/`TestSkuaRedTeamPass`/`TestIncubatorDocDescribesScratchpadBatching`), plus the existing `TestCoreNeutral` guard that must keep passing (no harness-specific paths leak into `core/`):
- `internal/bootstrap/registry_test.go` (or a new `_test.go` beside the existing guard tests) — new guard tests, one or more per rewritten file, asserting by substring (not exact text, matching the doctest/hooktest convention) that:
  - `worker-protocols.md` mentions `fledge heartbeat` and the before-long-operation instruction.
  - `skua.md` describes `fledge verdict` (not a bare pass/fail message) as how its verdict reaches the orchestrator/brooder.
  - `brooder.md` describes `fledge escalate` for blockers.
  - `foraging.md`'s Commissioner section describes `fledge await`/a `status` terminal value as the done-signal, not solely "wait for the final message."
  - `implementation.md` and `planning.md` reference the rewritten sections rather than duplicating stale message-based wording.
- `TestCoreNeutral` (existing) still passes unmodified — confirms no harness-specific (`.claude/`, `.codex/`, `.pi/`) paths were introduced by this rewrite.
- Implementation order fixed: write/extend the guard tests first (they'll fail against the current prose), confirm the failure is for the expected reason (old wording still present / new command not mentioned), then rewrite the prose until green.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after.
- [ ] AC-2: `worker-protocols.md`, `incubator.md`, `brooder.md`, `skua.md`, `foraging.md`, `implementation.md`, and `planning.md` all describe their state-bearing handoffs in terms of ledger reads/writes (`fledge heartbeat`/`await`/`verdict`/`escalate`/`ledger read`), satisfying PLM-030 FC-7.
- [ ] AC-3: `message-peer` is described in the rewritten prose as a stateless wake-up nudge only, never as the carrier of verdict/status/escalation content.
- [ ] AC-4: `TestCoreNeutral` and every pre-existing guard test that isn't intentionally superseded by this rewrite still passes.
- [ ] AC-5: `go test ./internal/bootstrap/...` passes.
