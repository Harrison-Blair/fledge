---
id: FTHR-075
title: Orchestration prose rewrite for ledger-based handoffs
plumage: PLM-030
status: fledged
priority: P1
depends_on: [FTHR-073, FTHR-074, FTHR-088, FTHR-089, FTHR-090, FTHR-092]
authored: 2026-07-16T22:24:20Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-075: Orchestration prose rewrite for ledger-based handoffs

## Description
Rewrites the agent-neutral orchestration prose so every state-bearing handoff it describes uses the PLM-030 ledger (`fledge heartbeat`/`await`/`verdict`/`escalate`/`pulse`/`ledger read`) instead of a `message-peer` message carrying that state. `message-peer` itself is not removed — it's re-scoped in this prose to a stateless wake-up nudge ("I wrote a record, go check"), consistent with PLM-030's decision that the 6-primitive/tier model stays untouched. Kept as a single feather across all seven files (not split per-doc) to guarantee one consistent handoff vocabulary throughout, rather than risking seven independently-drifted descriptions of the same mechanism.

**This feather is `fledge await`'s and `fledge pulse`'s first real consumer.** Every command shape it teaches must match the contracts as they actually ship, because these seven files are where every future agent learns how handoffs work — a wrong shape here propagates into brooder, skua, foraging, implementation and planning behavior indefinitely.

**Provenance of the command shapes below.** This feather does not invent contracts; it restates already-gated ones. But the relevant set is **not** just FTHR-072/073/074, as originally authored — two later plumages superseded parts of that:
- **PLM-034** (shipped by **FTHR-088**) gave `await` an opt-in existence-wait (`--exists`) and made `--timeout` **mandatory on both paths**, superseding PLM-030 FC-5's indefinite-blocking clause.
- **PLM-035** (shipped by **FTHR-089**/**FTHR-090**/**FTHR-092**) made liveness lease-only (deleting a PID check that classified every healthy worker as stalled), gave `heartbeat` a `--expect <duration>` declaration, and added `fledge pulse` as the CLI surface for the classification — superseding PLM-030 FC-3 and FC-4.

Writing this prose against the pre-PLM-034/035 contracts would have produced text that deadlocks on write-once records, is a usage error against the shipped binary, and teaches a recovery path that escalates to the user on every timeout while never once taking the branch it exists for. The `depends_on` edges exist so this is written **once**, against the contracts that actually ship.

## Affected Modules
Source of truth only — never the scaffolded copies at this repo's own `.fledge/skills/` (root `CLAUDE.md`; `.fledge/nest/architecture.md` — the two-layer split):

- `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` — the shared "spawn prompt is entire context, one-shot lifecycle, final message" contract. **Home of the heartbeat instruction**, since it is role-agnostic: heartbeat **before and periodically during** long operations, at an interval comfortably under the lease default; and where an operation is a **single blocking call with no seam to heartbeat at**, declare it up front with `fledge heartbeat <name> --expect <duration>`. Role files reference this rather than repeating it.
- `internal/bootstrap/core/skills/fledge-orchestrate/incubator.md` — the relay envelope routes `GATE`/`QUESTION`/`SPAWN-REQUEST`/`PHASE-CLOSE` as `message-peer`; PHASE-CLOSE-equivalent state can stay in the message body since it's a one-shot report, not an ongoing handoff. As a persistent named worker being tracked, the incubator gains liveness (`fledge heartbeat`) plus a note that forwarded idle notifications are never to be trusted over ledger state.
- `internal/bootstrap/core/skills/fledge-orchestrate/brooder.md`, `skua.md` — the skua's verdict currently travels as a `message-peer` pass/fail message. Rewrite to: skua writes `fledge verdict FTHR-### --result pass|fail`; whoever blocks on it calls **`fledge await FTHR-### --kind verdict --exists --timeout <duration>`**. `--exists` is mandatory here, not stylistic: `verdict` is **write-once**, so a change-wait deadlocks whenever the verdict lands before the waiter asks — and omitting `--timeout` is now a usage error. Brooder escalations become `fledge escalate <brooder-name> --message "…"`; anyone waiting on one uses `--kind escalation --exists --timeout <duration>`, for the same write-once reason.
- `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` — the Commissioner section's "wait as a two-input state machine" apparatus is the biggest rewrite target: replace "wait for the by-name final message" with **`fledge await <forager-name> --kind status --timeout <duration>`** for a `status: done` terminal value. **Change-wait is correct here** — `status` is repeatedly written, so "has it changed?" is the right question, and `--exists` would return on the forager's first heartbeat. The **"(or block indefinitely)" clause must go**: unbounded waits no longer exist. The forager still sends a final message for the coverage summary (which doesn't fit a terse record), but the *decision of when to act* stops depending on interpreting message-arrival versus a bare idle.
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` — §3.1's dispatch loop and §3.2's on-approval path reference the skua's pass message and brooder idle; update to the ledger commands above. §4's escalations use the same `fledge escalate` handoff. **FTHR-090 lands first and removes this file's pid-alive clause; do not re-add a PID reference — that field no longer exists.**
- `internal/bootstrap/core/skills/fledge-orchestrate/planning.md` — step 0's lifecycle-notification-forwarding language and step 2's Commissioner-wait reference point at the rewritten `foraging.md`/`incubator.md` sections rather than duplicating them.
- `.fledge/skills/fledge-orchestrate/*` (this repo's scaffolded copy) — refreshed in FTHR-076, not here; do not hand-edit (root `CLAUDE.md`).

## Approach
- Work file by file in the order above — `worker-protocols.md` first, since it's the shared contract the others reference.
- **Every `fledge await` in the prose must carry a concrete `--timeout` and the correct per-kind mode.** The rule, from the shipped contract (`fledge await`'s own usage text states it): **`verdict` and `escalation` are write-once → `--exists`; `status` is repeatedly written → change-wait (no `--exists`)**. Never write a bare `fledge await X --kind verdict`: it deadlocks *and* is a usage error.
- **Every wait site needs an exit-4 recovery path, and it must be liveness-based rather than a retry loop.** `--timeout` is mandatory, so `ExitTimeout` (4) is a normal, reachable outcome — a waiter that hits it has learned nothing and must not guess. The pattern to write:
  1. `fledge await …` returns 4.
  2. Run **`fledge pulse <peer-name>`**.
  3. **Not stalled** → the peer is working; it reports the declared quiet period and elapsed time, so **re-await for the remainder** rather than a blind retry.
  4. **Stalled** → *that* is the stall signal; escalate to the user. Never abandon a worker unilaterally — the existing rule stands.
  5. **No status record** → the peer hasn't heartbeat yet (starting up); it is **not** stalled. Re-await.
  Do **not** tell agents to compare timestamps by hand: `pulse` exists precisely so one tested procedure has one home.
- **The heartbeat instruction goes once, in `worker-protocols.md`.** Before *and periodically during* long operations. **This distinction is load-bearing, with a measured example worth citing in the prose:** during this feather's own planning phase, a forager's synthesis ran as a single unbroken **5m25s** silence. A heartbeat written only *before* that stretch goes stale at 5:00 — **25 seconds before the work succeeded** — so a before-only discipline would have raised a false stall on a healthy worker. Multi-step work (synthesis writes eight documents) has seams to heartbeat at; a **single blocking call has none**, which is what `--expect` is for.
- **Preserve every invariant that isn't about a handoff's transport** — the skua's 3rd-rejection escalation rule, the brooder's test-first discipline, green-teardown sequencing. Only "how does the other party learn this happened" changes, from message content to ledger record + optional nudge.
- **State explicitly that `fledge await` is not a return to polling.** `foraging.md` currently says "you are re-invoked by events, never poll", and that prohibition stays. `await` *is* the poll — bounded interval, mandatory timeout, run inside the waiting agent's own turn — a deterministic replacement for "wait for the harness to re-invoke me", not a hand-rolled sleep-loop. Say so, or the rewrite reads as contradicting the rule it's honoring.
- **Don't invent decisions.** Every mechanism here restates an already-gated command shape from PLM-030/034/035. If a genuine new decision surfaces while drafting (e.g. the exact wording of the heartbeat instruction), escalate rather than deciding unilaterally.

## Tests
Prose-content assertions following the established guard-test pattern (`.fledge/nest/testing.md` → `internal/bootstrap`, ~15 existing guards like `TestBrooderFixLoopInvariant`/`TestSkuaRedTeamPass`/`TestIncubatorDocDescribesScratchpadBatching`), plus the existing `TestCoreNeutral`.

- **Positive guards** — new tests in `internal/bootstrap`, asserting by substring (not exact text, per the doctest/hooktest convention) that:
  - `worker-protocols.md` names `fledge heartbeat`, the before-**and-during** instruction, and `--expect` for no-seam operations.
  - `skua.md` describes `fledge verdict` (not a bare pass/fail message) as how a verdict reaches its reader.
  - `brooder.md` describes `fledge escalate` for blockers.
  - `foraging.md`'s Commissioner section describes `fledge await … --kind status --timeout` and a `status` terminal value as the done-signal.
  - Each of the seven files describing a wait also names `fledge pulse` as the exit-4 recovery.
  - `implementation.md`/`planning.md` reference the rewritten sections rather than duplicating stale message-based wording.
- **Negative guards — the ones that actually prevent the deadlock** (mirroring FTHR-088's AC-8 grep-for-absence style, which is what makes a "don't do X" criterion checkable):
  - **No file under `core/skills/` contains `--kind verdict` or `--kind escalation` without `--exists`** on the same command.
  - **No `fledge await` appears without a `--timeout`.**
  - **The string "block indefinitely" (and equivalents) appears nowhere** — unbounded waits are impossible.
  - No file references PID liveness (FTHR-090 establishes this; this feather must not regress it).
- `TestCoreNeutral` (existing) still passes — no harness-specific paths leak into `core/`.
- Order: write/extend the guards first (they fail against current prose — the negative guards fail *because the current text contains exactly the deadlocking form*), confirm each failure is for the expected reason, then rewrite until green.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: Every `fledge await` invocation in the rewritten prose uses the **correct per-kind wait mode** — `--exists` for the write-once `verdict` and `escalation` kinds; change-wait (no `--exists`) for the repeatedly-written `status` kind — proven by a guard test asserting no `--kind verdict`/`--kind escalation` appears without `--exists`. A literal-but-deadlocking reading must fail this criterion (satisfies PLM-034 FC-1, FC-2).
- [x] AC-3: Every `fledge await` invocation in the prose carries a concrete `--timeout`, and no prose describes an unbounded or indefinite wait, proven by guard tests including a grep-for-absence of "block indefinitely" (satisfies PLM-034 FC-3).
- [x] AC-4: Every wait site in the prose specifies an `ExitTimeout` (4) recovery path in terms of `fledge pulse`: not stalled → re-await for the declared remainder; stalled → escalate to the user; no status record → not stalled, re-await. No wait site instructs agents to compare timestamps by hand (satisfies PLM-035 FC-7, FC-8).
- [x] AC-5: `worker-protocols.md` instructs workers to heartbeat **before and periodically during** long operations at an interval under the lease default, and to declare `--expect` for a single blocking call with no seam — proven by a guard test. A before-only instruction fails this criterion (satisfies PLM-035 FC-3).
- [x] AC-6: `worker-protocols.md`, `incubator.md`, `brooder.md`, `skua.md`, `foraging.md`, `implementation.md`, and `planning.md` all describe their state-bearing handoffs in terms of ledger reads/writes (`fledge heartbeat`/`await`/`verdict`/`escalate`/`pulse`/`ledger read`), satisfying PLM-030 FC-7.
- [x] AC-7: `message-peer` is described in the rewritten prose as a stateless wake-up nudge only, never as the carrier of verdict/status/escalation content.
- [x] AC-8: The rewritten `foraging.md` states explicitly that `fledge await` is a deterministic replacement for event re-invocation and **not** a return to hand-rolled polling, so the existing "never `sleep`-poll" prohibition reads as honored rather than contradicted.
- [x] AC-9: No file under `core/skills/` references PID liveness — this feather does not regress FTHR-090 (satisfies PLM-035 FC-11).
- [x] AC-10: `TestCoreNeutral` and every pre-existing guard test not intentionally superseded by this rewrite still passes.
- [x] AC-11: `go test ./internal/bootstrap/...` passes; `go test ./...` is green, `go vet ./...` and `gofmt -l .` are clean, and `fledge preen` reports no errors on the branch.
