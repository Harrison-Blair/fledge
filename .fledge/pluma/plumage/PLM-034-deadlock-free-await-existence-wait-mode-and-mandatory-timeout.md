---
id: PLM-034
title: "Deadlock-free await: existence-wait mode and mandatory timeout"
status: fledged
priority: P1
authored: 2026-07-17T04:00:43Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# PLM-034: Deadlock-free await: existence-wait mode and mandatory timeout

## Context

`fledge await` (PLM-030 FC-5, shipped by FTHR-073) answers an **existence question with a change-detector**. That single sentence is the root cause, and everything below follows from it.

The three ledger kinds want two different waits:

| kind | written | what a waiter actually asks |
|---|---|---|
| `status` | repeatedly (heartbeats) | "tell me when it **changes**" |
| `verdict` | **once** | "has it **landed** yet?" |
| `escalation` | **once** | "has it **landed** yet?" |

`await` implements only change-wait. For the two write-once kinds, "it was already there when I asked" is a **success** — but `await` treats it as a reason to keep waiting, forever. This produces two distinct deadlocks:

1. **Baseline race.** `await` samples a baseline at call time, then waits for the record to appear or change. If the record was already written before that sample, and is never rewritten, `await` blocks indefinitely.
2. **Identical-payload rewrite.** Change is detected by comparing the record's *payload bytes* only. `Record.Timestamp` is a sibling field, ignored by the comparison. `StatusRecord` carries `UpdatedAt`, so its payload always differs — it is immune. `VerdictRecord{Result, Note}` and `EscalationRecord{Message}` carry no timestamp and no pid, so re-writing the same verdict produces a **byte-identical payload** and the write is undetectable. A genuinely new record lands and no waiter ever sees it.

Both hit `verdict` and `escalation`; neither hits `status`. That is not coincidence — it is the same mistake wearing two hats.

The command already reads `record.Timestamp` to *print* `"await %s: record updated at %s"` while comparing payload bytes to *detect*. It has the correct signal in hand and uses it for display while detecting on the wrong one.

**Why the apparently obvious fix is not the fix.** "Compare `Timestamp` instead of `Payload`" was proposed and ruled out during interrogation: the ledger writes `time.Now().UTC().Format(time.RFC3339)` — **second granularity**. Two writes inside the same second produce byte-identical timestamps, so timestamp-comparison has the same blind spot as payload-comparison, merely narrower — and sub-second rewrites are the normal case for fast agent handoffs, not an exotic one. Making it sound would require moving the ledger to `RFC3339Nano` or a sequence number, i.e. changing `internal/ledger`, which FTHR-073's sibling FTHR-072 already fledged. This plumage takes a route that needs no such change: **existence-wait consults neither payload nor timestamp**, so it sidesteps the precision problem entirely.

**Why this matters now.** The motivating principle for the whole handoff-ledger area is that **silent wrongness burns tokens**. A deadlocked `await` is the purest instance available: no output, no exit code, no diagnosis — just a process that never returns while the clock runs. This is not hypothetical. The defect surfaced as a flaky acceptance test on main: **~1 run in 3 hangs, burning the full test timeout (measured at 216s) per hang**, with 4 passed / 2 hung across 6 consecutive runs. The test is unsound, but it is the messenger, not the disease.

**Why now specifically.** `fledge await` has **zero consumers today** — no orchestration prose calls it. FTHR-075 ("Orchestration prose rewrite for ledger-based handoffs", PLM-030, `egg`, unwritten) is the feather that will make the prose use it, and it is the one that could bake the deadlock in by telling agents to change-wait on a write-once `verdict`. This is the last window to fix the contract before its first real consumer exists, and before that prose has to be written twice.

**Superseded decisions.** This plumage revises two decisions settled during PLM-030's interrogation. Both are named here rather than left to contradict silently:

1. **PLM-030's Out of Scope** states `fledge await` "only waits for 'the record appeared or changed since I started waiting.'" This plumage adds an existence-wait mode. That is *not* a reversal of the accompanying no-value-predicates rule (`--equals status=done` remains out of scope, permanently): asking *whether a record exists* is not asking *what it says*.
2. **PLM-030 FC-5 and FTHR-073's AC-2** specify that omitting `--timeout` blocks indefinitely, and FTHR-073's evidence records that behavior as verified. This plumage makes `--timeout` **mandatory on both paths**, so omitting it becomes a usage error. That is a **breaking change to a shipped contract**, taken deliberately as defence in depth: it does not fix the semantics, it bounds the blast radius of any wait we have not thought of.

FTHR-073 and its `.fledge/molt/FTHR-073.md` evidence are deliberately **left untouched**. A fledged feather is an immutable record of what shipped and what was actually verified on the day; retroactively editing it would make its evidence misrepresent that. This plumage is the forward pointer.

## User Stories

- As a worker waiting on a peer's write-once handoff (a skua's `verdict`, a peer's `escalation`), I want to ask "has it landed?" and get an answer whether or not it landed before I asked, so that the ordering of two agents' turns cannot deadlock the handoff.
- As a worker waiting on a peer's liveness, I want to keep asking "has it changed?" for `status` records, so that the existing heartbeat/stalled-vs-busy classification keeps working exactly as it does today.
- As any agent calling `fledge await`, I want it to be impossible to accidentally block forever, so that a mistake costs me a bounded, diagnosable timeout instead of a silent hang that burns the full test or session budget.
- As a developer of fledge, I want the acceptance suite to prove `await`'s contract without racing two processes, so that a green run means the code is correct rather than that a race happened to fall the right way.
- As whoever writes FTHR-075's prose, I want the correct wait mode per record kind stated in the contract, so that the first real consumer of `await` cannot reintroduce the deadlock.

## Functional Criteria

1. FC-1: `fledge await` gains an explicit opt-in existence-wait mode, `--exists`. In this mode the command returns successfully as soon as the named `(subject, kind)` record **exists** — immediately, if it already existed at call time — and otherwise blocks until it first appears. Existence-wait consults neither the record's payload nor its timestamp.
2. FC-2: `await`'s default behavior is unchanged: without `--exists` it remains change-wait, returning when the record first appears or its payload differs from the baseline sampled at call time (PLM-030 FC-5's semantics, minus the indefinite-blocking clause superseded by FC-3).
3. FC-3: `--timeout` is **mandatory on both paths**. Invoking `fledge await` without `--timeout` is a usage error (`ExitUsage`, 2) carrying a message that names the flag, on both the existence-wait and change-wait paths. It is no longer possible to request an unbounded wait.
4. FC-4: The timeout path's existing behavior is preserved for both modes: on elapse, exit `ExitTimeout` (4, distinct from `ExitFail`), print the last-known record (or `null`), and under `--json` carry an explicit `timed_out: true`. On success `timed_out` remains omitted.
5. FC-5: Both modes support `--json`, consistent with the convention that every `fledge` command does.
6. FC-6: `cmd/fledge/testdata/await.txtar`'s happy-path scenario is reworked to be **race-free by construction**: no backgrounded process, no `wait`, no two processes racing. The record is written first, then `await --exists` returns immediately against it. The appearance-while-blocking path remains covered deterministically at unit level (`TestAwaitReturnsOnAppearance`), which is where it was always actually proven.
7. FC-7: `fledge await`'s own usage/help text states the correct wait mode per record kind — `verdict` and `escalation` use existence-wait; `status` uses change-wait — so that the constraint ships with the command itself and is discoverable by running it. This is where FTHR-075's author will encounter it: the orchestration prose does not yet discuss the ledger at all (that prose is precisely what FTHR-075 adds), so the command's own contract is the only shipped home available without pre-empting that feather. The constraint is additionally enforced structurally: FTHR-075 gains a `depends_on` edge on this plumage's feather, so it cannot be dispatched until the new contract has fledged. FTHR-075 remains PLM-030's feather and this plumage does not write its prose — the edge orders the two, it does not claim the work. The edge exists because a note is not a guardrail: FTHR-075 was dispatchable the moment its own dependencies fledged, so without it, whoever picked it up next would write ledger prose against a contract this plumage is mid-flight to change, and then rewrite it — precisely the double-work the "why now" argument above exists to avoid. That risk is not hypothetical: a concurrent session is active in this repository with no visibility into this reasoning.

## Acceptance Criteria

- [x] AC-1: Tests for every criterion below are written first, run against the unchanged code, and observed FAILING for the expected reason, with the failing output captured verbatim in `.fledge/molt/` evidence before any implementation is written.
- [x] AC-2: `fledge await <subject> --kind <kind> --exists --timeout <d>` returns successfully and immediately when the record already exists at call time — the exact condition that deadlocks today — proven by a test that fails against the current code.
- [x] AC-3: `--exists` returns successfully when the record does not exist at call time and appears while blocking.
- [x] AC-4: `--exists` is immune to the identical-payload rewrite defect: a record rewritten with a byte-identical payload does not affect the existence-wait result, and existence-wait never consults the payload.
- [x] AC-5: Change-wait remains the default and its behavior is unchanged from FTHR-073 for the `status` kind, including detection of a payload change from the call-time baseline.
- [x] AC-6: Omitting `--timeout` is a usage error exiting `ExitUsage` (2) with a message naming the flag, proven **separately on both the existence-wait and change-wait paths**.
- [x] AC-7: The timeout path exits `ExitTimeout` (4) with `timed_out: true` and the last-known record (or `null`) under `--json`, proven for **both** modes with a real elapsed-time test, not mocked away.
- [x] AC-8: `cmd/fledge/testdata/await.txtar` contains no `&` backgrounding and no `wait`, restoring the property that no file in the txtar suite races two processes.
- [x] AC-9: The flake is demonstrated gone by **20 consecutive green runs** of the reworked `await.txtar`, captured in the evidence file, **accompanied by a structural argument** naming why the race is eliminated by construction rather than merely unlikely. The argument carries the claim; the runs are the backstop. (A single green run proves nothing: at the measured ~1-in-3 hang rate it would pass ~67% of the time with the bug fully intact.)
- [x] AC-10: `fledge await`'s usage/help text states the correct wait mode per record kind (FC-7), asserted by a test so the guidance cannot silently drift from the behavior it describes.
- [x] AC-11: `go test ./...` is green and `fledge preen` passes on the branch that closes this plumage.

## Out of Scope

- Moving the ledger to `RFC3339Nano` or sequence-numbered records. Named explicitly because it was seriously considered and **ruled out on cost**, not overlooked: existence-wait avoids consulting timestamps for detection, so the precision problem does not need solving to fix these deadlocks. A future need for sub-second change-detection on `status` would reopen it.
- `--since <timestamp>` and value-predicate waiting (e.g. `--equals status=done`). Still out, consistent with PLM-030's original decision. This plumage supersedes only the *appeared-or-changed* framing, not the no-predicates rule.
- Any change to `internal/ledger`'s record shapes (`StatusRecord`, `VerdictRecord`, `EscalationRecord`), to `ledger.Read`/`ledger.Write`, or to `internal/lock`.
- A configurable or default `--timeout` value, or config-file support for one — the flag is explicit per call, with no global default.
- Migrating `status`-kind semantics, the heartbeat protocol, or the 5-minute stale-lease threshold.
- Editing FTHR-073, its acceptance criteria, or its molt evidence to match the new contract — deliberately left as the historical record of what shipped.
- Writing FTHR-075's prose. The feather stays PLM-030's and this plumage does not author it; FC-7 only orders it behind the new contract via a `depends_on` edge, so that it is written once, against the contract that will actually ship.

## Open Questions

None outstanding — every branch raised during interrogation was resolved with the user (see `.fledge/scratch/PLM-await-contract-questions.md` for the batched leaf-decision record, and the individually-gated decisions on scope, contract shape, and mandatory `--timeout`).
