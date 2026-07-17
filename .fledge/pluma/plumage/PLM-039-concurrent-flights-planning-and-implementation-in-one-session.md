---
id: PLM-039
title: "Concurrent flights: planning and implementation in one session"
status: hatched
priority: P2
authored: 2026-07-17T21:15:22Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.10
---

# PLM-039: Concurrent flights: planning and implementation in one session

> **SUPERSEDED (2026-07-17) — parked, not to be implemented as written.** The
> multi-harness migration program makes each run a subprocess-backed **Run State
> Machine** the harness controller owns; N concurrent runs are then N controllers
> with per-run worktrees and per-run persisted state — concurrency and isolation
> fall out of that model rather than needing a bespoke "flight" grouping over
> interactive teammates. This plumage's durable ideas are absorbed: run identity /
> per-run namespacing and multiplexed watching → the **Process Runner** (P1) and
> **Run State Machine** (P3); harness-owned per-run worktree isolation → the
> **Plan→Implement workflow** (P4). Left hatched-and-parked for provenance; the
> body below reflects the pre-reframe design (note: "flight" was the name chosen
> for that grouping; "effort" remains reserved for the user's separate future
> feature).

## Context
Today one orchestrator (the main session) runs **one phase at a time** — planning
(delegated to an incubator) or implementation (dispatching brooder/skua pairs).
The workflow's routing (`SKILL.md`), digests, and scope all assume a single active
phase. To author plumages while an implementation run proceeds, the user has to
spawn a **separate Claude Code instance** — a real friction the user wants gone.

This plumage lets multiple runs proceed **concurrently in one session**. The
enabling groundwork is PLM-C: with the orchestrator reduced to a pure relay plus
deterministic, ledger-driven coordination, a single orchestrator can interleave
several runs without doing any run's thinking itself.

The organizing concept is a **flight**: one planning or implementation run,
identified by a flight id that namespaces its workers, its declared
feather/plumage scope, and its digest. Multiple flights coexist; the orchestrator
relays for and coordinates all of them. The design is **general** — N independent
flights of any mix — with the two headline cases explicitly verified: a planning
flight concurrent with an implementation flight, and two concurrent implementation
flights.

Two mechanisms make concurrency safe:

- **Deterministic multiplexed watching.** `fledge await` blocks on one subject,
  and PLM-C forbids inferring state from idle/messages — so juggling flights needs
  a multiplexed, deterministic way to detect "the next thing that needs the
  orchestrator across all active flights." Its concrete implementation may be
  delivered by PLM-E's comms subsystem.
- **Per-flight scope isolation.** Each flight declares its feather/plumage scope so
  two flights never fight over the same feather; the per-feather `fledge brood` lock
  remains the hard backstop.

The changes touch the workflow prose
(`internal/bootstrap/core/skills/fledge-orchestrate/`, especially SKILL.md routing,
planning.md, implementation.md), the Claude adapter piping (`team-loop.md`), and
the CLI/ledger (a flight concept + the multiplexed-await capability). This plumage
**depends on PLM-C** (deterministic coordination + `signal` kind) and **may depend
on PLM-E** for the multiplexed-await implementation.

## User Stories
- As a fledge user, I want to run a planning session while an implementation run is
  in progress — in the **same** Claude Code session — so that I no longer have to
  spawn a separate instance to make progress on both at once.
- As a fledge user, I want to run multiple implementation flights at the same time,
  each scoped to its own feathers, so that independent work streams proceed in
  parallel without colliding.
- As a fledge user, I want to direct an instruction at a specific flight and have the
  orchestrator route it correctly, so that concurrent runs don't get my feedback
  crossed.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …

1. FC-1: A **flight** is an explicit grouping of one planning or implementation run,
   identified by a flight id that namespaces the run's workers, its declared
   feather/plumage scope, and its digest. Flight membership is recorded (not held in
   orchestrator context alone).
2. FC-2: Multiple flights run concurrently within one Claude Code session, in any mix
   (a planning flight alongside an implementation flight; two or more implementation
   flights). The single orchestrator relays for and coordinates all active flights.
3. FC-3: The orchestrator watches all active flights **deterministically** via a
   multiplexed await — a CLI capability that returns the next actionable ledger
   transition across all active flights' subjects (e.g. `fledge await --any` or a
   `fledge pending`/inbox query) — never blocking on one flight and never inferring
   from idle/messages (per PLM-C). The concrete implementation may be provided by
   PLM-E's comms subsystem.
4. FC-4: Each flight declares its feather/plumage scope; a feather belongs to at most
   one active flight at a time; the orchestrator refuses to dispatch a feather already
   in another active flight's scope; the per-feather `fledge brood` lock remains the
   hard backstop against double-claim.
5. FC-5: Digests are **per-flight**, namespaced by flight id — no singleton
   `digest-*.md` collision. Each flight's phase-close writes its own digest.
6. FC-6: The roster allocates non-colliding species across all flights' workers
   (already global), and flight membership records which workers belong to which
   flight, so teardown and recovery stay scoped to the right flight.
7. FC-7: The user can direct an instruction at a specific flight; the orchestrator
   tags relays by flight, and when a user instruction is ambiguous across active
   flights it asks which flight it targets (a relay/disambiguation action, consistent
   with the pure-relay role — no run cognition).
8. FC-8: The routing prose (`SKILL.md`, planning.md, implementation.md, team-loop.md)
   is updated so both phases can be active simultaneously under distinct flights,
   replacing the current one-phase-at-a-time assumption.
9. FC-9: The design builds on PLM-C's deterministic coordination + `signal` kind, and
   the multiplexed-await capability's implementation may depend on PLM-E; the
   dependency is stated and reconciled during PLM-E interrogation.

## Acceptance Criteria
- [ ] AC-1: A flight concept exists — a flight id namespacing workers, declared scope, and digest — represented via the CLI/ledger and documented; unit and txtar tests cover flight creation and worker→flight membership.
- [ ] AC-2: A multiplexed, deterministic await capability exists (e.g. `fledge await --any` or `fledge pending`) that returns actionable ledger transitions across multiple subjects at once, with unit and txtar tests. If this capability is delivered by PLM-E, PLM-D's corresponding feather declares the dependency.
- [ ] AC-3: Per-flight scope is declared and enforced: dispatch refuses a feather already in another active flight's scope, and the per-feather `fledge brood` lock remains the backstop; tests cover both the scope refusal and the lock race.
- [ ] AC-4: Digests are per-flight (namespaced by flight id) with no singleton collision; a txtar/unit test asserts two concurrent flights write distinct digests.
- [ ] AC-5: The workflow prose permits concurrent flights of different phases, documents user-directed flight routing and ambiguity resolution, and drops the one-phase-at-a-time assumption; a bootstrap invariant test asserts the concurrency-and-routing language is present.
- [ ] AC-6: The two headline cases are verified — a planning flight concurrent with an implementation flight, and two concurrent implementation flights — via end-to-end or representative acceptance tests.
- [ ] AC-7: `fledge preen` passes, and `go test ./...` (new flight/await tests plus updated invariant and txtar fixtures) is green after the changes.

## Out of Scope
- The CLI-backed content-messaging transport and primitive/tier re-architecture —
  **PLM-E**; it may *implement* the multiplexed-await capability this plumage designs,
  but the transport work itself is not owned here.
- The future, separate "effort" feature the user has reserved that name for — unrelated
  to the flight concept introduced here.
- The coordination discipline itself (deterministic state, no nudges, ignore idle,
  pure relay, forager lifecycle) — owned by **PLM-C**; this plumage consumes it.
- The adversarial shrike (PLM-A) and editor-free interrogation (PLM-B) internals,
  except where flight namespacing touches their digests/scratchpads.

## Open Questions
- Exact surface of the multiplexed await (`fledge await --any` vs a `fledge
  pending`/inbox query) and whether PLM-E provides it — settled during PLM-E
  interrogation.
- How a flight is created and named at the CLI (verb/flags) and whether flights persist
  across a session resume (likely reusing the existing roster + `fledge broods`
  recovery, scoped per flight) — refined at feather-authoring time.
