---
id: PLM-043
title: "Plan to implement workflow: Codex planner, human approval, Claude implementer in a harness-owned worktree"
status: hatched
priority: P1
authored: 2026-07-17T22:03:51Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.10
---

# PLM-043: Plan to implement workflow: Codex planner, human approval, Claude implementer in a harness-owned worktree

## Context
Fourth plumage of the multi-harness migration program. It is the first that wires
**actual workflow behavior** onto the substrate built by PLM-040 (Process Runner),
PLM-041 (Provider adapters), and PLM-042 (run state machine + artifacts + events).
It delivers the front half of the brief's initial workflow: **Codex planner
(read-only) → `plan.json` → human plan-approval gate → Claude implementer in a
harness-owned git worktree → validation trigger**. The review/fix half is P5.

### fledge stays spec-driven: run targets a feather, `plan.json` is the "how"
Two distinct "planning" activities are kept separate (settled in interrogation):
- **Spec-authoring** — the existing planning phase produces plumages/feathers: the
  durable *what* + acceptance criteria. Unchanged by this plumage.
- **Implementation-planning** — this workflow's first step: the Codex planner reads a
  **target feather** and the repository context and produces **`plan.json`, the
  run-scoped *how*** (approach, steps, files, risks, AC coverage). Feathers remain
  the source of truth; `plan.json` is ephemeral per-run detail, validated downstream
  against the feather's acceptance criteria.

So a run (PLM-042's run→spec linkage) **targets a feather**, and the brief's "user
request → planner" maps to "**feather → run → Codex plans its implementation**." A
feather-less ad-hoc planning mode is a possible later extension, explicitly not
built here.

### The workflow steps this plumage wires
1. **Codex planner (read-only).** The harness starts a Codex work task
   (PLM-041 provider adapter, PLM-040 runner) in **read-only** mode (P2 supplies the
   flag translation; P6 owns the policy) with the target feather + repository context
   as input. Its schema-constrained output is captured and wrapped as the
   **`plan.json`** artifact (PLM-042 envelope). Run state: `planning → plan_review`.
2. **`plan.json` payload schema** (this plumage defines it — PLM-042 deferred
   per-artifact payloads to their producer): `target_feather` (the FTHR-### plus its
   acceptance criteria pulled in for reference), `summary` (approach in prose),
   `steps[]` (ordered implementation steps), `files_touched[]` (files likely
   created/modified), `validation_commands[]` (**suggested** — the model proposes; the
   harness decides what actually runs, in P5/P6), `risks[]` (risks/open concerns for
   the human), and `ac_coverage` (how the plan addresses each feather AC).
3. **Human plan-approval gate.** At `plan_review`, the human reviews `plan.json` and
   **approves** (→ `awaiting_implementation_approval` → `implementing`) or **rejects
   with feedback** (→ re-plan: the Codex planner revises given the feedback). This is
   PLM-042's first human-gated transition; the approval *mechanism/UX* is P6, but the
   gate lives here. Plan re-planning is **uncapped** for now (distinct from P5's
   capped review/fix loop).
4. **Harness-owned git worktree.** Before implementation, **fledge creates and owns
   the worktree + branch** for the run: branch name **`fledge/<FTHR-id>-<short-run-id>`**
   (e.g. `fledge/FTHR-061-a1b2c3`) — traceable to both the spec and the specific run,
   collision-free across retries. fledge records the **baseline commit**, and on
   completion **captures the diff**. This replaces today's model where an agent/human
   creates the worktree and `brood` merely records it.
5. **Brood reused as the run's feather lock.** The run's implementation step
   **acquires the `brood` claim** for its target feather, and fledge now **sets the
   `Branch`/`Worktree` fields and actually creates them** (today those are recorded
   but externally created). Brood's file mechanism and feather status-flip
   (`hatching`) are unchanged; only brood's *interactive-teammate usage* is the thing
   being retired. This is the reconciliation deferred from PLM-042.
6. **Claude implementer — one task per run, in the worktree.** The harness starts a
   single Claude work task (`claude -p`) with the whole `plan.json` + target feather +
   the worktree as its working directory; **`steps[]` are guidance to the
   implementer, not separate harness launches**. The implementer has **write access
   only inside the worktree** (P2 cwd + scrubbed-env mechanism). **Agents never push,
   merge, rewrite history, or delete branches** — commit/merge is harness-performed
   and human-gated (PLM-042's final-approval gate, in P5's completion). Run state:
   `implementing → validating` (this plumage only **wires PLM-042's transition** into
   `validating`; the validation *pipeline* is P5/P6).
7. **Non-destructive worktree recovery.** Building on PLM-042's state reconcile
   (in-flight → `failed`/`needs_input`), `fledge run recover` **surfaces orphaned
   worktrees/branches** for a crashed run; **removal is an explicit action, never
   automatic**, because a worktree may hold uncommitted work. Reuses the existing
   `fledge broods --stale` vanished-worktree detection.

### Retirement: prepared here, executed at P5
This plumage **builds the subprocess plan→implement half alongside the still-intact
interactive teammate-worker path** and **marks** the interactive implementation path
(the brooder role + implementation.md's interactive-implementation prose) for
retirement. **No physical removal happens here** — the interactive loop
(brooder + skua + ledger) is retired in a **single clean cutover at the end of P5**,
once the whole subprocess loop (implement + review + fix) works end-to-end, so there
is never a broken half-loop.

The change is net-new Go/prose: the plan→implement workflow wiring over the run state
machine, the `plan.json` payload schema + validation, harness-owned worktree
creation/branch-naming/baseline/diff, brood integration driven by the run, and the
`fledge run` surface extensions for the plan-approval gate and worktree recovery.

## User Stories
- As a fledge user, I want Codex to read a feather and produce a concrete
  implementation plan I approve before any code is written, so that I control whether
  an expensive or risky approach proceeds.
- As a fledge user, I want the plan expressed as a structured `plan.json` — approach,
  steps, files, risks, and how it covers each acceptance criterion — so that my
  approval decision is informed rather than a wall of prose.
- As a fledge user, I want Claude to implement the approved plan inside a git worktree
  that fledge creates and owns, writing nowhere else and never pushing or merging, so
  that implementation is isolated and every integration stays under my control.
- As a fledge user, I want a crashed run's worktree surfaced rather than silently
  deleted, so that I never lose uncommitted work to automatic cleanup.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: A run **targets a feather**; the workflow's planning step runs a **read-only
   Codex** work task (PLM-041 adapter over PLM-040 runner) with the target feather +
   repository context as input, capturing its output as the `plan.json` artifact
   (PLM-042 envelope). State advances `planning → plan_review`.
2. FC-2: The **`plan.json` payload schema** is defined with at least: `target_feather`
   (id + referenced ACs), `summary`, `steps[]`, `files_touched[]`,
   `validation_commands[]` (suggested), `risks[]`, and `ac_coverage`.
   `validation_commands` are advisory — the harness, not the plan, decides what runs.
3. FC-3: The **human plan-approval gate** at `plan_review` accepts **approve**
   (advance toward `implementing`) or **reject-with-feedback** (re-plan via the Codex
   planner). Re-planning is uncapped in this plumage. State is advanced only by the
   explicit human action, never inferred (per PLM-042).
4. FC-4: **fledge creates and owns the git worktree + branch** for the run before
   implementation, using branch name `fledge/<FTHR-id>-<short-run-id>`, and records
   the **baseline commit**; on implementation completion it **captures the diff**.
5. FC-5: The run's implementation step **acquires the `brood` claim** for the target
   feather with fledge **creating and recording** the `Branch`/`Worktree`; brood's
   existing file mechanism and `hatching` status-flip are unchanged. A feather already
   brooded by another run/holder is refused (existing brood semantics).
6. FC-6: The **Claude implementer runs as one work task per run** (`claude -p`) with
   the whole `plan.json` + target feather, its working directory set to the worktree
   and **write access confined to it** (P2 mechanism); `steps[]` are guidance, not
   separate launches.
7. FC-7: **Agents never push, merge, rewrite history, or delete branches.** No
   workflow step grants the implementer those operations; commit/merge is
   harness-performed and deferred to the human-gated completion (P5). This plumage
   asserts the constraint in prose and in the implementer's permission translation.
8. FC-8: On implementation completion the run advances `implementing → validating`
   (wiring PLM-042's transition only). The **validation pipeline is not built here**
   (P5/P6); reaching `validating` is the handoff point.
9. FC-9: **Non-destructive crash recovery**: `fledge run recover` surfaces a crashed
   run's orphaned worktree/branch; **removal is explicit, never automatic**. Reuses
   `fledge broods --stale` detection. Uncommitted work is never auto-destroyed.
10. FC-10: This plumage **builds the subprocess plan→implement half alongside the
    intact interactive path** and **marks** the interactive implementation path
    (brooder + implementation.md interactive prose) for retirement **without removing
    it**; physical cutover is a P5-completion event.
11. FC-11: The Codex planner task is **read-only** (P2 flag translation; P6 policy);
    no planning step is granted write or network-mutating access.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [ ] AC-1: A test drives the planning step against a **fake Codex command** (no real CLI) and asserts a `plan.json` artifact is produced under the run directory, wrapped in the PLM-042 envelope, with the run advancing `planning → plan_review`.
- [ ] AC-2: A schema test asserts `plan.json` carries `target_feather`(+ACs), `summary`, `steps[]`, `files_touched[]`, `validation_commands[]`, `risks[]`, `ac_coverage`, and that an invalid/missing-field plan is rejected.
- [ ] AC-3: A test asserts the plan-approval gate parks at `plan_review`, that **approve** advances toward `implementing` and **reject-with-feedback** triggers a re-plan (a second fake-Codex planning task), and that state never advances without the explicit action.
- [ ] AC-4: A test asserts fledge **creates a git worktree + branch** named `fledge/<FTHR-id>-<short-run-id>`, records the baseline commit, and captures a diff after a simulated change (exercised on a scratch git repo in the test).
- [ ] AC-5: A test asserts the implementation step **acquires the brood** for the target feather (fledge setting Branch/Worktree), that a second run targeting the same feather is refused, and that brood's `hatching` status-flip still occurs.
- [ ] AC-6: A test drives the implementer step against a **fake Claude command** and asserts it runs one task per run with cwd = the worktree and write confined there (a fake command writing outside the worktree is prevented/does not affect the main tree), advancing `implementing → validating`.
- [ ] AC-7: A test/assertion confirms **no workflow step grants push/merge/rewrite/delete**; the implementer's permission translation excludes them.
- [ ] AC-8: A crash-recovery test asserts `fledge run recover` surfaces an orphaned worktree and that removal requires an explicit action (never automatic), reusing stale-worktree detection; uncommitted content is preserved.
- [ ] AC-9: An invariant/prose test asserts the interactive implementation path is **marked** for retirement but **still present and functional** in this plumage (no physical removal of brooder/implementation prose or the ledger here).
- [ ] AC-10: A txtar acceptance test exercises the plan→implement flow end-to-end (fake Codex + fake Claude, scratch git repo): plan produced, approved, implemented in a worktree, reaching `validating`, with `--json`.
- [ ] AC-11: `fledge preen` passes and `go test ./...` (new workflow/worktree/plan-schema tests plus updated fixtures) is green after the changes.

## Out of Scope
- **The review/fix loop** (independent Codex review → Claude fix, iteration cap,
  re-review) — P5. This plumage ends at reaching `validating`.
- **The validation pipeline itself** (what commands run, formatting/lint/test/build
  gates) — P5/P6; this plumage only wires the transition into `validating`.
- **The commit/merge action and the final human-approval gate mechanics** — the
  merge is harness-performed at the P5-completion human gate (PLM-042 marker; P6
  mechanism). No committing/merging happens in this plumage.
- **Physical retirement of the interactive teammate-worker path and the ledger** —
  single cutover at end of P5; here it is only marked.
- **Payload schemas of other artifacts** (`review.json`, `fix-report.json`,
  `validation-results.json`, `approval.json`) — their producing phases (P5/P6).
- **The human-approval UX/mechanism and per-role permission policy** — P6; this
  plumage consumes P2's flag-translation mechanism and PLM-042's gate markers.
- **Feather-less ad-hoc planning** (a run with no target feather) — a possible later
  extension, not built here.
- **Team/hosted/production concerns** — flagged, not built (single-user for now).

## Open Questions
- Whether plan re-planning should gain a configurable cap later (uncapped now) — noted
  for feather/P6 config time; not imposed here.
- Exact `short-run-id` derivation and whether the worktree lives under `.fledge/` or a
  configurable worktrees directory (the brief mentions a worktree directory config,
  owned by P6) — settled at feather time.
- Precise shape of `ac_coverage` (per-AC status vs prose) and how strictly `plan.json`
  is validated against the feather's current ACs — settled at feather time.
- How repository context is supplied to the read-only planner (existing `.fledge/nest`
  vs a fresh per-run `repository-context.json`) — reconciled at feather time with the
  artifact set.
