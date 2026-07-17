---
id: PLM-042
title: "Run state machine, typed artifacts, and event model (single persistence substrate)"
status: hatched
priority: P1
authored: 2026-07-17T21:55:00Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.10
---

# PLM-042: Run state machine, typed artifacts, and event model (single persistence substrate)

## Context
Third plumage of the multi-harness migration program, sitting on **PLM-040
(Process Runner)** and **PLM-041 (Provider adapters)**. Those give fledge the
ability to *run* provider work; this plumage gives fledge the ability to *drive and
remember* a run — a persisted state machine, typed artifacts, and a normalized
event log — as the **single go-forward coordination substrate**.

### The endgame decision: this substrate replaces the ledger
This is the plumage where the migration's persistence endgame is fixed. Today
fledge coordinates **interactive teammate workers** (brooder/skua/forager) through
the `internal/ledger` records (`status`/`verdict`/`escalation`, subject-keyed,
5-minute lease, `fledge await`) — a design whose entire purpose is coordinating
agents that can only act when the harness grants them a turn. Under the subprocess
model fledge **owns each process**, so state is a deterministic fact of the run,
not something awaited or inferred. The user's decision: the **new run-state
persistence is the single substrate**, and the ledger is **replaced**.

Replacement is **sequenced, not immediate** (settled in interrogation):
- The **interactive driver** (layer 1 — the human-facing Claude Code/Codex session
  that issues `fledge` commands, e.g. this session) **stays**; it is not a ledger
  consumer.
- The **interactive teammate workers** (brooder/skua) — the ledger's actual
  consumers — are what the subprocess model replaces (`claude -p` / `codex exec`
  work subprocesses instead of spawned teammates). The user **explicitly confirms
  this path is being replaced, not maintained indefinitely.**
- **This plumage marks the ledger superseded but does NOT delete it.** The ledger
  keeps functioning until the subprocess plan→implement→review workflow (P4/P5) can
  actually do the work, so there is **no coordination gap**. Physical removal of the
  ledger + the interactive teammate-worker prose happens as P4/P5 land the
  replacement.

This is where the supersession of **PLM-038** (deterministic, harness-owned
coordination — realized here natively) and **PLM-039** (per-run isolation — realized
by the per-run directory + run identity below) fully resolves. PLM-038's proposed
`signal` ledger kind is **moot**: any handoff the interactive driver still needs is
expressed in the new run-state model, not a ledger kind.

### What this plumage builds
1. **Run state machine (engine + concrete graph).** A persisted, harness-controlled
   state machine. The engine enforces legal transitions; the concrete graph adopts
   the brief's states with **plain descriptive names** (not bird-themed, for
   legibility): `created → planning → plan_review → awaiting_implementation_approval
   → implementing → validating → reviewing → fixing → final_validation →
   awaiting_human_approval → completed`, plus the terminal/exceptional states
   `cancelled`, `timed_out`, `blocked`, `failed`, `needs_input`. Transitions are
   **controlled by the harness, never inferred from model prose**. Two transitions
   are **human-gated**: `plan_review → awaiting_implementation_approval →
   implementing` (approve an expensive/risky plan before implementation) and
   `final_validation → awaiting_human_approval → completed` (approve before any
   commit/push/merge/deploy/destructive operation). All other transitions are
   automatic. The **per-transition workflow ACTIONS** (what actually runs during
   `implementing`, etc.) are wired by P4/P5; this plumage owns the graph, validation,
   and gate *markers*, not the actions. The full human-approval MECHANISM is P6; the
   gate markers live in the graph here.
2. **Typed Artifact Protocol (envelope + versioning).** A common artifact envelope —
   `schema_version`, `run_id`, `artifact_id`, `artifact_type`, `created_at`,
   `producer`, `consumer`, `status`, `payload` — with **large outputs referenced by
   file path** rather than copied inline, and a **schema-versioning + migration
   strategy** (how `schema_version` is bumped and how older artifacts are read). This
   plumage defines the **envelope and the versioning strategy**; the **payload schema
   of each artifact type is defined by its producing phase** — `plan.json` → P4,
   `review.json`/`fix-report.json` → P5, `validation-results.json` → P5/P6,
   `approval.json` → P6. `request.json` and `repository-context.json` inputs are
   defined where first produced/consumed.
3. **Normalized event model + `events.jsonl`.** A normalized event envelope
   (`schema_version`, `event_id`, `run_id`, `task_id`, `provider`, `role`,
   `timestamp`, `type`, `payload`) representing events from any provider, **fed by
   PLM-041's streaming hook over PLM-040's framed output**. Raw provider events may
   be preserved in the payload when useful, but nothing outside the adapter depends
   on provider-specific JSON. Event types cover at least: process start/exit,
   assistant messages, tool calls, commands, file changes, plans, reviews,
   validation, usage, warnings, errors, cancellation, human approval.
4. **On-disk layout + run identity.** Each run gets its own directory
   **`.fledge/runs/<run-id>/`** containing `run.json` (the persisted state-machine
   record), `events.jsonl` (the normalized event log), and the run's typed artifacts.
   A run has its **own fledge-generated id namespace** and **records the
   feather/plumage it implements** in `run.json`, so every run is traceable to the
   spec it fulfills. The existing `brood` claim is **reconciled at feather time**
   (worktree ownership likely subsumes/wraps the brood lock — worktrees are P4); this
   plumage only records the run→spec linkage and does not redesign `brood`.
5. **Crash recovery (write-ahead + reconcile, no auto-resume).** Because runs advance
   through per-step CLI invocations with no daemon (P1), recovery is deterministic:
   the harness **write-aheads intent** (marks a task in-flight with pid/started-at
   before launch) and, on the next invocation, **reconciles** — an in-flight task
   whose process is gone is classified to **`failed`/`needs_input`** and **never
   silently auto-resumed**. A `fledge run recover`/reconcile command surfaces
   interrupted runs for a human/explicit-retry decision. Worktree cleanup is P4; this
   plumage owns the state-level classification.

The change is net-new Go: a run/state-machine package, the artifact + event
envelope types with a versioning strategy, the `.fledge/runs/<run-id>/` persistence,
and the `fledge run` command surface (create/advance/show/recover, `--json`). It
consumes PLM-040/041 and marks — but does not yet delete — `internal/ledger`.

## User Stories
- As the fledge harness controller, I want a persisted run state machine whose
  transitions I control, so that a run's progress is a deterministic fact I advance
  step by step — never inferred from a model's prose.
- As a fledge user, I want each run's plan, review, validation, and approval recorded
  as typed artifacts under a per-run directory, with large outputs referenced by
  path, so that agents hand off structured data instead of re-pasting big blobs and I
  can inspect exactly what happened.
- As a fledge user, I want a normalized event log per run that reads the same
  regardless of which provider produced it, so that tooling and my own inspection
  don't depend on Codex-vs-Claude JSON quirks.
- As a fledge user, I want an interrupted run to be reconciled safely on the next
  command — surfaced as failed/needs-input rather than silently resumed — so that a
  crash never causes duplicate side effects.
- As a fledge maintainer, I want this to be the single go-forward persistence that
  replaces the lease/await ledger as the subprocess workflow lands, so that fledge
  has one coordination substrate rather than two.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: A persisted **run state machine** exists with the concrete state graph:
   `created, planning, plan_review, awaiting_implementation_approval, implementing,
   validating, reviewing, fixing, final_validation, awaiting_human_approval,
   completed`, plus `cancelled, timed_out, blocked, failed, needs_input`. State names
   are plain/descriptive.
2. FC-2: The engine **validates transitions** against the graph and **rejects illegal
   transitions**; state is advanced only by explicit harness action, never inferred
   from provider output text.
3. FC-3: Exactly two transitions are **human-gated** — plan approval
   (`plan_review → awaiting_implementation_approval → implementing`) and final
   approval (`final_validation → awaiting_human_approval → completed`, before any
   commit/push/merge/deploy/destructive op) — and are marked as such in the graph.
   All other transitions are automatic. This plumage owns the gate markers; the
   approval mechanism is P6; per-transition actions are P4/P5.
4. FC-4: A **typed artifact envelope** is defined with `schema_version`, `run_id`,
   `artifact_id`, `artifact_type`, `created_at`, `producer`, `consumer`, `status`,
   `payload`; artifacts may reference **large outputs by file path** instead of
   inlining them.
5. FC-5: A **schema-versioning + migration strategy** is defined: `schema_version` is
   present on every artifact and event, its bump policy is documented, and reading an
   artifact whose version is older than current is handled deterministically
   (migrate-on-read or reject-with-clear-error — decided at feather time).
6. FC-6: Payload schemas for specific artifact types are **out of scope here** and
   defined by their producing phase; this plumage defines only the envelope and
   versioning. It provides a place for a payload without constraining its shape.
7. FC-7: A **normalized event model** is defined (`schema_version`, `event_id`,
   `run_id`, `task_id`, `provider`, `role`, `timestamp`, `type`, `payload`) and a
   per-run **`events.jsonl`** is appended from **PLM-041's streaming hook**. Raw
   provider event data may be preserved in `payload`; no consumer outside the adapter
   depends on provider-specific JSON.
8. FC-8: Each run persists under **`.fledge/runs/<run-id>/`** containing `run.json`
   (state record), `events.jsonl`, and typed artifacts. Run ids are fledge-generated
   in their own namespace.
9. FC-9: `run.json` records the **feather/plumage the run implements**, making every
   run traceable to its target spec. The `brood` claim is not redesigned here; the
   linkage is recorded and reconciliation with brood/worktrees is deferred to P4.
10. FC-10: **Crash recovery is write-ahead + reconcile**: a task is marked in-flight
    (pid/started-at) before launch; on the next invocation an in-flight task whose
    process is absent is reconciled to `failed`/`needs_input` and **never silently
    auto-resumed**; a `fledge run recover`/reconcile surface lists interrupted runs.
11. FC-11: The existing `internal/ledger` is **marked superseded but not deleted** by
    this plumage — it keeps functioning for the legacy interactive teammate-worker
    path until P4/P5 land the subprocess workflow that replaces it. No new code is
    added that depends on the ledger; the new substrate does not write ledger records.
12. FC-12: A `fledge run` command surface exists (at least create / advance / show /
    recover) with `--json` output consistent with existing fledge commands, exercised
    without any real provider CLI.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [ ] AC-1: A run-state-machine package exists with the concrete state graph; unit tests assert legal transitions succeed, illegal transitions are rejected, and state is only advanced by explicit action (no inference from text).
- [ ] AC-2: A test asserts the two human-gated transitions are marked as gated in the graph and that reaching a gated boundary does not auto-advance (it parks in `awaiting_*` until an explicit approve action), while non-gated transitions advance automatically.
- [ ] AC-3: The artifact envelope type exists with all required fields; a test asserts an artifact can reference a large output by path (not inlined) and round-trips through persistence.
- [ ] AC-4: A schema-versioning/migration test asserts `schema_version` is present and that an older-versioned artifact is handled per the chosen deterministic policy (migrated or rejected with a clear error), not silently misread.
- [ ] AC-5: The normalized event model exists; a test drives PLM-041's streaming hook (over PLM-040's framed output, via a fake command) and asserts normalized events are appended to `events.jsonl` with the envelope fields, provider-agnostically.
- [ ] AC-6: A test asserts a run persists under `.fledge/runs/<run-id>/` with `run.json`, `events.jsonl`, and artifacts; run ids are unique and fledge-generated; `run.json` records the target feather/plumage id.
- [ ] AC-7: A crash-recovery test simulates an in-flight task whose process is gone and asserts reconcile classifies it to `failed`/`needs_input` (never auto-resumed) and that `fledge run recover` lists it.
- [ ] AC-8: A test/assertion confirms the new substrate writes no ledger records and adds no new dependency on `internal/ledger`, while the ledger package remains present and functional (superseded, not deleted).
- [ ] AC-9: A txtar acceptance test exercises the `fledge run` surface (create/advance/show/recover) end-to-end with `--json`, using fake commands only (no real provider CLI, no network).
- [ ] AC-10: `fledge preen` passes and `go test ./...` (new run/state/artifact/event tests plus any updated fixtures) is green after the changes.

## Out of Scope
- **Per-artifact payload schemas** — `plan.json` (P4), `review.json`/`fix-report.json`
  (P5), `validation-results.json` (P5/P6), `approval.json` (P6). This plumage defines
  only the envelope, versioning strategy, and event model.
- **The workflow ACTIONS** wired to transitions (what runs during planning/
  implementing/reviewing/fixing/validating) — P4/P5. This plumage owns the graph,
  validation, gate markers, persistence, and recovery, not the actions.
- **The human-approval mechanism / UX** (how a human is prompted and how the approval
  is recorded) — P6; only the gate *markers* live in the graph here.
- **Git worktrees, branch naming, diff capture, and worktree cleanup** — P4; this
  plumage records the run→spec linkage but does not create worktrees or redesign the
  `brood` lock.
- **Physical deletion of `internal/ledger` and the interactive teammate-worker
  prose** — sequenced to P4/P5 as the subprocess workflow replaces them; here the
  ledger is only marked superseded.
- **Permission policy, redaction ruleset, validation pipeline definitions** — P6 /
  P5. Only mechanisms/markers relevant to state and artifacts are here.
- **Team/hosted/production concerns** (shared run stores, multi-user run isolation
  beyond a single-user filesystem) — flagged, not built.

## Open Questions
- The exact `schema_version` bump policy and whether older artifacts are
  migrated-on-read or rejected-with-error — chosen at feather time (FC-5 leaves both
  open with a deterministic requirement).
- The concrete `run.json` field set and the run-id format (fledge-generated;
  human-readable vs opaque) — settled at feather time.
- Precise reconciliation of run identity/worktree ownership with the existing `brood`
  lock — deferred to P4, which owns worktrees.
- Whether `needs_input` and `blocked` are distinct in practice or one of them is
  redundant given the human gates — refined at feather time.
