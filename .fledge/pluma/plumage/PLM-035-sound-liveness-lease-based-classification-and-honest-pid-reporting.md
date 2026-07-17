---
id: PLM-035
title: "Sound liveness: lease-based classification with declared quiet periods"
status: hatched
priority: P1
authored: 2026-07-17T07:36:18Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.7
---

# PLM-035: Sound liveness: lease-based classification with declared quiet periods

## Context

PLM-030's liveness classification (FC-4, shipped and unit-tested by FTHR-072) **reports every healthy worker as stalled, always.** Not in a corner case — in the normal case, for every worker, from the moment its first heartbeat lands.

**The defect, measured.** `fledge heartbeat` records the PID of the *`fledge` CLI process itself*, which exits the instant the command returns. It cannot record anything else: it is a different, shorter-lived process than the worker on whose behalf it writes. A real heartbeat run in a scratch repo recorded pid `284015`; `kill -0 284015` reported it dead immediately afterward. Feeding that recorded PID and a **two-second-old** lease to the real classifier returns:

```
ClassifyLiveness(284015, lease 2s old, now)
  → stalled=true  reason="pid 284015 is not alive"
```

And it is *decisive*: the classifier checks PID liveness first and returns immediately — "a dead PID is decisive — the worker is gone regardless of how fresh its lease is." Lease freshness is never consulted. A worker that heartbeat two seconds ago is classified stalled, on the strength of a signal that was never about the worker at all.

**Why it is structural rather than a bug.** PLM-030 FC-4 says a worker is stalled when "**its recorded PID** is no longer a running process." That presumes something records *the worker's* PID. Nothing does, and nothing can. `fledge heartbeat` only ever sees its own PID, and on the harnesses this workflow targets a worker is frequently an in-process agent with **no operating-system PID at all**. There is no implementation of FC-4 that is correct, which is why this is a plumage and not a bug report.

**The root cause: an advisory promoted to a verdict.** This pattern was inherited. `internal/lock` has recorded `os.Getpid()` since it shipped, so `fledge broods` annotates locks with their PID's liveness — and a brood created *seconds* earlier already displays `(pid not alive)`:

```
$ fledge brood FTHR-001 --owner probe     → brooding FTHR-001 (branch master)
$ fledge broods
  FTHR-001  probe  since 2026-07-17T07:29:21Z  branch master  (pid not alive)  (worktree gone)
```

In `lock` the dead PID is **advisory** — a printed annotation and a `pid_alive` JSON field. It is wrong, but it decides nothing: the one dangerous path, `fledge broods --stale` (which feeds force-release during recovery), keys on **worktree existence**, not the PID. That path is sound and stays sound. PLM-030 copied the PID-recording pattern into the ledger and, in classification, **promoted it from an annotation into a decisive check**. The same mistake: cosmetic in one layer, a false verdict in the next.

This is PLM-034's own diagnosis recurring one layer down — *the correct signal is in hand and used for display while detection runs on the wrong one*. There, `await` printed a record's timestamp while comparing payload bytes. Here, the lease is recorded, displayed, and then never consulted, because a meaningless PID short-circuits ahead of it.

**The second defect: the contract promises something it cannot deliver.** PLM-030's user story asks, verbatim:

> *"As a worker about to start a long-running operation (a build, a test suite, a large shell command), I want to record that I'm still working before I go quiet, so that whoever is waiting on me has an authoritative reason not to conclude I've stalled."*

And FC-3 says heartbeat is *"intended to be called immediately before a long-running operation."* But the lease TTL is a fixed five minutes, and `fledge heartbeat` accepts only `--note` and `--json`. **A heartbeat written immediately before a ten-minute operation expires at minute five, while the worker is healthily mid-operation.** The command can say *"I am alive now"*; it cannot say *"expect me to be quiet for a while"* — and the second is the entire reason to call it before going quiet. PLM-030's user story is structurally unsatisfiable by what PLM-030 built.

Crucially, **no heartbeat discipline fixes this.** Multi-step work has seams to heartbeat at. A worker blocked inside a single long call has none — there is no moment at which it could heartbeat, so only a *pre-declared* expectation can cover it.

**Why the obvious fix is not the fix.** "Raise the TTL" was considered and ruled out. One fixed value must serve two irreconcilable masters: long enough never to false-stall the longest legitimate quiet period, short enough to detect a genuine stall promptly. Any value is simultaneously too short for the twenty-minute operation and too long for the worker that died thirty seconds in. Raising it relocates the defect rather than removing it, and leaves the user story still unsatisfiable at a different threshold. A globally configurable TTL was also declined: PLM-034 deliberately out-of-scoped config-file support for `await`'s timeout, and a global value has the same one-size-fits-nobody problem with more machinery. **The worker about to do the work is the only party that knows how long it will take** — so the declaration belongs there, per-lease.

**The measured false-stall we actually have.** During this plumage's own planning phase, a forager's synthesis ran as a single unbroken **5m25s** silence (07:04:05 → 07:09:30) before it reported success. A heartbeat written before that stretch goes stale at 5:00 — **25 seconds before the work completed.** That is the real, observed case. The build-or-test-suite framing above is PLM-030's own stated intent for FC-3, not a claim about this repository's suite, which currently completes well inside the TTL.

**Why now.** The PID field has **no stored corpus to migrate**: ledger records are latest-value-only with no history, the broods directory is gitignored, and no ledger directory exists on disk in this repository yet. Deleting a field costs nothing today and will not stay free. More pressingly, `fledge await`'s first real consumer is imminent: FTHR-075 rewrites seven prose files that teach every future agent how handoffs work, and its timeout-recovery path is specified in terms of exactly this classification. Shipping that prose against a classifier that always says "stalled" would teach seven files a recovery path that escalates to the user on every single timeout and never once takes the branch it exists for. This is the last window in which the contract can be fixed before the prose bakes it in — the same argument, and the same timing, that motivated PLM-034.

**Superseded decisions.** This plumage revises three decisions settled during PLM-030's interrogation. Each is named here rather than left to contradict silently:

1. **PLM-030 FC-4** defines stalled as "recorded PID is no longer a running process, OR lease past a fixed five minutes." The PID half is deleted outright, not weakened: it is unusable rather than merely unused, and a permanently-false advisory is worse than none. Liveness becomes lease-freshness only.
2. **PLM-030 FC-3** specifies `fledge heartbeat <name> [--note "<text>"]`. It gains the ability to declare an expected quiet period, without which its own stated intent is unreachable.
3. **PLM-030 AC-4** requires the stalled classification "unit-tested against both failure directions: a dead-PID worker with a fresh lease, and a live-PID worker with a stale lease." The first direction ceases to exist. That criterion was satisfied honestly against the contract as written; it is the contract that was wrong.

FTHR-072/073/074, FTHR-088, and their `.fledge/molt/` evidence are **left untouched**. A fledged feather is an immutable record of what shipped and what was verified on the day; retroactively editing it would make its evidence misrepresent that. This plumage is the forward pointer — the same discipline PLM-034 applied to FTHR-073.

**On evidence quality.** This plumage changes the classifier's signature twice (a parameter deleted, another added). The previous run recorded a lesson from FTHR-088: changing a Go signature test-first breaks the package build, so every test in the package fails to *compile* rather than fails *behaviorally* — making the "observed failing" step vacuous. That caveat was surfaced, accepted, and written down. This plumage walks into the identical hazard by design, so its criteria require at least one **behavioral** failing-test observation per criterion, sourced from the CLI acceptance layer, which drives the built binary and therefore fails on behavior rather than on arity. The requirement exists because a prior feather in this area produced vacuous test-first evidence; it is not ceremony.

## User Stories

- As a worker about to go quiet for a known stretch, I want to declare how long that will be, so that whoever waits on me has the authoritative reason not to conclude I stalled that PLM-030 promised and could not deliver.
- As a worker blocked inside a single long-running call with no seam to heartbeat at, I want my pre-declared expectation to cover the whole operation, so that liveness works for the case no heartbeat discipline can reach.
- As an agent waiting on a peer's handoff, I want a liveness verdict I can act on, so that "fresh, keep waiting" and "stale, escalate" are decisions about the worker rather than about an artifact of how a CLI process is spawned.
- As an agent that has just learned a peer is not stalled, I want to see how long it declared and how much of that has elapsed, so that I can wait out the remaining time instead of guessing and timing out repeatedly against a worker I know is working.
- As a user, I want a stall escalation to mean something, so that being asked to intervene is a signal rather than the constant background state of every wait.
- As a developer of fledge, I want no field in a record whose only possible value is misleading, so that a future reader cannot rebuild this defect by trusting it — which is precisely how it arrived here.

## Functional Criteria

1. FC-1: Liveness classification consults **only** lease freshness. The recorded-PID input is removed from the classification entirely — not demoted, not made advisory.
2. FC-2: The PID field is deleted from both the status-record and the feather-claim record shapes, and from every command's output that exposes it — human-readable display and machine-readable JSON alike. No field remains whose only possible value is a falsehood. Supersedes PLM-030 FC-4.
3. FC-3: `fledge heartbeat` accepts a declared expected quiet period. When omitted it defaults to the current five minutes, so every existing call site keeps its present behavior unchanged. Supersedes PLM-030 FC-3.
4. FC-4: The declared period is stored as a **duration** anchored to the record's existing update timestamp, not as an absolute deadline — so the record stays self-describing (what was claimed, and when) rather than stating only their sum.
5. FC-5: A worker is stalled when the present moment is past its lease's update timestamp plus its declared period. A worker that never declares one is stalled after five minutes, exactly as today.
6. FC-6: The declared period is **not capped**. The legitimate long operation is the case FC-3 exists for, and a cap would reinstate the fixed-TTL defect at a different threshold.
7. FC-7: A command reports a named worker's liveness — whether it is stalled, why, **and its declared period with elapsed time against it**. The declaration must be visible: with no cap (FC-6), visibility is the only control on an implausible declaration, and it is what lets a waiter wait out the remaining period rather than re-timing-out blindly.
8. FC-8: Absence of a status record is reported as a **distinct state**, not as stalled. A worker that has not yet reached its first heartbeat is starting up, not dead; conflating the two rebuilds the false-stall this plumage exists to remove.
9. FC-9: A liveness report is not a command failure. The classification is carried in the output and a successful read exits zero, so that "your peer is stalled" and "the command broke" remain distinguishable.
10. FC-10: The ledger directory joins the set of per-run intermediates the tool ignores in version control. It is the only member of that class omitted from a block whose own comment describes exactly it: "per-run intermediates — regenerable, not shared".
11. FC-11: Orchestration prose that directs agents to consult PID liveness is corrected in step with the field's deletion, so that no shipped prose points at a field that no longer exists.

## Acceptance Criteria

- [ ] AC-1: Tests for every criterion below are written first, run against the unchanged code, and observed FAILING for the expected reason, with the failing output captured verbatim in `.fledge/molt/` evidence before any implementation is written.
- [ ] AC-2: At least one failing-test observation per criterion is **behavioral** — an assertion about observable command behavior, not a compilation or arity error — sourced from the CLI acceptance layer. Evidence consisting solely of build breakage does not satisfy AC-1: the classifier's signature changes twice here, and a build break would otherwise make every test in the package vacuously "fail".
- [ ] AC-3: A worker whose lease is fresh classifies as not stalled, proven by a test that **fails against the current code** — the exact condition misclassified today (satisfies FC-1).
- [ ] AC-4: No PID field remains in either record shape or in any command's human-readable or JSON output, proven by a search asserting its absence (satisfies FC-2).
- [ ] AC-5: A heartbeat declaring an expected quiet period longer than the default keeps its worker classified not-stalled past the old five-minute threshold, and stalled once the declared period elapses — both directions proven (satisfies FC-3, FC-5).
- [ ] AC-6: A heartbeat with no declared period stalls after five minutes exactly as today, proving the default preserves existing behavior for every current call site (satisfies FC-3).
- [ ] AC-7: The declared period is stored as a duration alongside the update timestamp, and both are readable from the record (satisfies FC-4).
- [ ] AC-8: The liveness report includes the declared period and the elapsed time against it, proven by a test asserting both appear (satisfies FC-7).
- [ ] AC-9: A worker with no status record reports as a distinct state, is **not** reported as stalled, and exits zero (satisfies FC-8, FC-9).
- [ ] AC-10: `fledge broods --stale` continues to key on worktree existence and is unaffected by the PID deletion, proven by a test — the force-release path was sound before this plumage and must remain so (satisfies FC-2).
- [ ] AC-11: A freshly initialized repository ignores the ledger directory in version control (satisfies FC-10).
- [ ] AC-12: No shipped prose references PID liveness, proven by a search returning nothing (satisfies FC-11).
- [ ] AC-13: `go test ./...` is green and `fledge preen` passes on the branch that closes this plumage.

## Out of Scope

- **Writing FTHR-075's orchestration prose.** FTHR-075 remains PLM-030's feather. This plumage corrects only the prose that points at the deleted PID field (FC-11); the ledger-handoff rewrite stays where it is, ordered behind this plumage's feathers so it is written once, against the contract that actually ships.
- **Renaming the classifier function to match its CLI surface.** The CLI verb and the internal package name are allowed to differ — the root `CLAUDE.md` documents this as the design ("`check` (validation = `preen`)", "`graph` (dependency graph = `vee`)", "`lock` (feather claims = `brood`)"). Renaming tested code for surface coherence is churn with no behavioral payoff.
- **Editing FTHR-072/073/074, FTHR-088, or their `.fledge/molt/` evidence.** Deliberately left as the historical record of what shipped and what was verified. This plumage is the forward pointer.
- **A configurable or default declared-period value, or config-file support for one.** Declared explicitly per call, consistent with PLM-034's identical decision for `await`'s timeout. This plumage narrows where it could have widened.
- **Sub-second or higher-precision lease timestamps.** PLM-034 ruled this out on cost and nothing here reopens it: lease staleness is measured in minutes, where second granularity is ample.
- **Any change to `fledge await`'s wait contract**, to the record kinds themselves, or to the atomic write/read machinery.
- **The tracked `.gitkeep` inside the ignored broods directory** — a tracked file overrides its ignore rule, and it appears to be a leftover from an earlier gitkeep removal. Observed during this interrogation; noted rather than chased.

## Open Questions

None outstanding — every branch raised during interrogation was resolved with the user (see `.fledge/scratch/PLM-035-questions.md` for the batched leaf-decision record, and the individually-gated decisions on the PID's fate and the TTL contract).
