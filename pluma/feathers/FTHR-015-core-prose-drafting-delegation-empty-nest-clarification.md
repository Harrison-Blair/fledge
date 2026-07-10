---
id: FTHR-015
title: "Core prose: drafting delegation + empty-nest clarification"
plumage: PLM-010
status: pipping
priority: P2
depends_on: []
oversight: merge
authored: 2026-07-10T21:21:28Z
agent: fledge-orchestrate/planning
fledge_version: 0.3.0
---

# FTHR-015: Core prose: drafting delegation + empty-nest clarification

## Description
Teach the agent-neutral planning phase to delegate spec-body drafting to an incubator where the
harness provides it, and document the normal empty-nest state so agents stop misreading it as a
failure. This is prose-only, in the shared `core/` skills (no harness names — must stay
neutral). It widens the FTHR-014 tracer: 014 makes the incubator *exist* in the Claude adapter;
015 makes the shared workflow *use* it (capability-conditionally) and fixes the foraging
false-alarm. Runs in parallel with 014 (disjoint files).

## Affected Modules
- **`internal/bootstrap/core/skills/fledge-orchestrate/planning.md`** — steps 3.4 (plumage
  draft) and 4.6 (feather draft) gain a capability-conditional delegation branch mirroring
  step 2's foraging pattern; step 2 (or its neighborhood) gains a one-line pointer to the
  empty-nest note. See `.fledge/nest/entry-points.md` → orchestration entry points.
- **`internal/bootstrap/core/skills/fledge-orchestrate/foraging.md`** — add the empty-nest
  clarification to the forager protocol.
- **`cmd/fledge/testdata/<new>.txtar`** (new fixture, e.g. `plan_delegation.txtar`) — scaffold
  core and grep the two markers. A *new* file, deliberately not `init.txtar`/`agents.txtar`,
  so it never collides with FTHR-014's edits to those fixtures.

## Approach
- **Delegation branch (AC-3 → PLM-010 FC-3).** In 3.4 and 4.6, after "draft the full file",
  add: *"If you provide `spawn-worker`, delegate the draft to an incubator worker — pass it the
  resolved decisions and pointers (prospective ID, template, concern docs to cite, and for
  feathers the plumage link / `depends_on` / `oversight`); it reads the template and concern
  docs itself and returns the full body. If you do not provide `spawn-worker`, draft inline.
  Either way you (the orchestrator) run the confirm-gate and, on Accept, create the file with
  `fledge new …` and write the returned body — the incubator never runs `fledge new` or mutates
  specs, so no un-gated file is ever written."* Keep it agent-neutral: reference the
  `spawn-worker` primitive and the `incubator` role (both allowed in core — `TestCoreNeutral`
  only forbids harness names). Preserve the existing author-to-draft-then-gate ground rule.
- **Empty-nest note (AC-4 → PLM-010 FC-4).** In `foraging.md`, state that a forager's first act
  (`fledge nest scaffold`) clears and recreates `.fledge/nest/` as **empty template stubs**
  (placeholder concern docs, unfilled `raw/*.md`, `index.md` frontmatter stamped to HEAD), and
  that this empty state is the **expected intermediate** after scaffolding — scouts and
  synthesis fill it next; it is **not** a failure and must not be flagged as one. Add a
  one-line pointer from planning.md step 2 so an orchestrator watching a forager sees it too.
- Choose **stable grep markers** in the prose (a distinctive phrase in each) that the test pins
  without being brittle to surrounding wording.

## Tests
Written test-first: (1) write the fixture; (2) `fledge init` in it against unchanged embedded
core, observe the greps FAIL (markers absent); (3) add the prose, observe PASS.
- **`cmd/fledge/testdata/plan_delegation.txtar`** (new)
  - `fledge init`; `exists .fledge/skills/fledge-orchestrate/planning.md`.
  - `grep '<delegation marker>' .fledge/skills/fledge-orchestrate/planning.md` — pins the
    capability-conditional delegation branch.
  - `grep '<empty-nest marker>' .fledge/skills/fledge-orchestrate/foraging.md` — pins the
    empty-nest clarification.
- `TestCoreNeutral` (existing) stays green — the added prose names no harness.
- Whole `go test ./...` green.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `planning.md` steps 3.4 and 4.6 delegate drafting capability-conditionally on
      `spawn-worker`, and explicitly retain the confirm-gate and `fledge new` commit with the
      orchestrator while barring the incubator from mutating specs (satisfies PLM-010 FC-3,
      AC-3).
- [x] AC-3: `foraging.md` (with a pointer from planning.md step 2) documents the
      empty-post-`fledge nest scaffold` nest as the expected intermediate state, not a failure
      (satisfies PLM-010 FC-4, AC-4).
- [x] AC-4: The added core prose keeps the skills agent-neutral (`TestCoreNeutral` passes) and
      the new grep fixture passes; `go test ./...` is green.
