---
id: PLM-030
title: Deterministic agent handoff ledger
status: hatched
priority: P1
authored: 2026-07-16T22:14:29Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# PLM-030: Deterministic agent handoff ledger

## Context
fledge's orchestration workflow coordinates its planning/implementation phases entirely through spawned, named agent workers (incubator, forager, brooder, skua) that communicate over the harness's native messaging primitive (`message-peer`, e.g. Claude Code's SendMessage). That channel is stateless and best-effort from fledge's point of view: nothing stops a message from being missed, and — critically — the harness fires "idle" notifications whenever a worker's turn ends, including harmlessly *between* steps of its own work (e.g. mid a long shell command). The orchestrator (or an incubator acting as a forager's commissioner) has no independent way to tell "genuinely stalled" from "just busy," so it either misreads spurious idles as stalls, or the prose workaround ("idle is not completion, don't act on it") has to be manually re-learned and re-applied at every wait point. This has produced real, observed indeterminism in this very repo's own orchestration runs (see the digest from this planning phase's own foraging wait).

fledge already has a proven building block for exactly this class of problem: `internal/lock`'s brood-claim files (`.fledge/broods/*.brood`) are atomic, pid-stamped, JSON, disk-resident, and `--json`-queryable — a deterministic alternative to "ask the agent and hope." This plumage extends that same pattern from exclusive claims to a general **handoff ledger**: every state-bearing signal agents currently pass to each other over messages (a skua's review verdict, a worker's done/ready/blocked status, an escalation, and worker liveness) becomes a typed, schema-validated record written and read through new `fledge` CLI subcommands, with a blocking `fledge await`-style command replacing ad hoc waiting with a deterministic poll-with-timeout. Messages continue to exist as wake-up nudges, but carry no state of their own — the ledger is the single source of truth for "what happened" and "what's the current state," removing the ambiguity the current message-only approach cannot resolve.

## User Stories
- As an orchestrator (or an incubator acting as a forager's commissioner), I want to classify a worker as genuinely stalled vs. merely busy from an objective, on-disk signal, so that I stop misreading harness idle notifications as stalls and stop churning workers that are actually still working.
- As a spawned worker (brooder, skua, incubator, forager) waiting on my counterpart's next handoff, I want to block on a single deterministic command until that handoff's record appears or changes (or a timeout elapses), so that I don't have to interpret ambiguous message traffic to know when to act.
- As a skua, I want to record my review verdict as a typed, schema-validated record rather than free-text in a message, so that the brooder and orchestrator can read and act on it programmatically without misparsing.
- As a worker about to start a long-running operation (a build, a test suite, a large shell command), I want to record that I'm still working before I go quiet, so that whoever is waiting on me has an authoritative reason not to conclude I've stalled.
- As a developer of fledge itself, I want the ledger's record shapes and CLI surface to be self-contained and independently testable (like `internal/lock`), so that the deterministic-handoff guarantee doesn't depend on any particular harness's messaging behavior.

## Functional Criteria
1. FC-1: A new ledger record type exists, addressed by `(subject, kind)` — `subject` is a feather ID or a worker name — with three kinds in scope: `status` (current activity/lease, doubling as done/ready/blocked terminal signals and as the liveness heartbeat), `verdict` (a skua's pass/fail review outcome), and `escalation` (a worker surfacing a blocker/decision to its commissioner). Each `(subject, kind)` is a single file holding the latest value only (no history/append log).
2. FC-2: Writing a ledger record is atomic (a reader never observes a partial or torn record) and concurrency-safe, using the same proven pattern (temp-file + `os.Link`/atomic rename, `flock` where exclusivity is needed) already used by `internal/lock` and `internal/spec`'s ID allocation — never re-implemented from scratch.
3. FC-3: A `fledge heartbeat <name> [--note "<what I'm doing>"]` command lets a worker refresh its own `status` record's lease timestamp, intended to be called immediately before a long-running operation.
4. FC-4: A worker is classified "stalled" only when its recorded PID is no longer a running process, OR its `status` record's lease timestamp has exceeded a fixed staleness threshold (5 minutes) with the PID still alive but no other explanation — never from a harness idle notification alone.
5. FC-5: A `fledge await <subject> --kind <kind> [--timeout <duration>]` command blocks until the named record either first appears or its content changes from what it was at call time, printing the resulting record as JSON on stdout. On change, it exits `0`. On timeout with no change, it exits a new dedicated exit code (`ExitTimeout`, additive to the existing `ExitOK/Fail/Usage/Env` set) and still prints the last-known record (or `null`) as JSON, with `--json` output additionally carrying an explicit `timed_out: true` field.
6. FC-6: Every new ledger command supports `--json` output, consistent with the existing convention that every `fledge` command does.
7. FC-7: The orchestration prose (`worker-protocols.md`, `incubator.md`, `brooder.md`, `skua.md`, `foraging.md`, `implementation.md`, `planning.md`) is updated so that every state-bearing handoff currently described as "send a message with X" instead describes "write/read the corresponding ledger record," with `message-peer` re-scoped in that prose to a stateless wake-up nudge only. This applies to every relationship between a persistent, individually-addressed worker and its commissioner/counterpart — brooder↔skua, incubator↔orchestrator, and forager↔commissioner (whether the commissioner is the orchestrator or a delegated incubator) — but not to scouts, which remain unnamed, one-shot, and untracked by the ledger.
8. FC-8: `fledge preen` gains no new validation obligations toward ledger records in this plumage (the ledger has no cross-record consistency invariant analogous to brood/hatching-status bidirectionality) — but a feather may add basic corruption tolerance (skip-and-report, mirroring `lock.List`'s handling of an unparseable `.brood` file) if the interrogation for that feather surfaces the need.

## Acceptance Criteria
- [ ] AC-1: `internal/ledger` (or equivalent new package) exists with `status`, `verdict`, and `escalation` record types, atomic write/read functions, and unit tests covering concurrent-write safety and corrupt-file handling, independent of any CLI command.
- [ ] AC-2: `fledge heartbeat`, `fledge await`, and the read/write commands needed to record a `verdict` and an `escalation` are implemented, each supporting `--json`, exercised by CLI acceptance tests (testscript/txtar) covering the happy path, the timeout path, and at least one malformed-input path per command.
- [ ] AC-3: `fledge await`'s timeout path is proven with a real elapsed-time test (not mocked away) exiting the new dedicated timeout exit code, distinct from `ExitFail`.
- [ ] AC-4: The stalled-vs-busy classification (PID check OR stale lease, not idle notifications) is implemented and unit-tested against both failure directions: a dead-PID worker with a fresh lease, and a live-PID worker with a stale lease.
- [ ] AC-5: `worker-protocols.md`, `incubator.md`, `brooder.md`, `skua.md`, `foraging.md`, `implementation.md`, and `planning.md` are updated to describe handoffs in terms of ledger records and `fledge await`/`fledge heartbeat`, with `message-peer` reduced to a stateless nudge role in that prose, and this repo's own `.fledge/skills/` scaffold refreshed to match (`fledge init --refresh`) with `fledge preen` passing.
- [ ] AC-6: `go test ./...` is green and `fledge preen` passes on the branch that closes this plumage.

## Out of Scope
- An append-only history/audit log for ledger records — latest-value-only records, no replay of past handoff state.
- Value-predicate waiting (e.g. `fledge await --equals status=done`) — `fledge await` only waits for "the record appeared or changed since I started waiting," never a specific value.
- Any change to the 6-primitive/tier model (`internal/bootstrap/primitives.go`) or any adapter manifest — `message-peer` and `spawn-worker` remain exactly what they are today; this plumage changes what travels over `message-peer`, not whether a harness needs it.
- Migrating existing `internal/lock` brood-claim records into the new ledger, or merging the two packages — brood locks (exclusive claims) and the ledger (handoff state/history) remain separate systems that happen to reuse the same atomic-write technique.
- A remote or networked ledger — stays local-filesystem, single-repo-checkout scope, same as the existing brood-lock system.
- A configurable heartbeat/lease TTL (flag or config) — the 5-minute threshold is a fixed constant in this plumage; a future feather can add an override if a concrete need for a different value shows up.
- Extending the ledger or heartbeat/await commands to scouts, or to Tier A harnesses (no `spawn-worker` at all).

## Open Questions
None outstanding — every branch raised during interrogation was resolved with the user (see `.fledge/scratch/PLM-handoff-ledger-questions.md` for the batched leaf-decision record).
