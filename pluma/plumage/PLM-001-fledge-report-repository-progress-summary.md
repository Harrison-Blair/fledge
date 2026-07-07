---
id: PLM-001
title: "fledge colony: repository progress summary"
status: fledged
priority: P2
authored: 2026-07-06T23:40:05Z
agent: fledge-orchestrate/planning
fledge_version: 0.1.0
---

# PLM-001: fledge colony: repository progress summary

## Context
Fledge repositories accumulate REQ/TASK specs, but no single command answers "where does this project stand?" Today that requires mentally combining `status`, `ready`, `graph`, and `locks` output. `fledge colony` provides one repo-wide progress summary for both humans running the CLI and agents orchestrating implementation. Both consumers are first-class: the human text view and the `--json` view carry the same information, consistent with every existing command's dual-output convention.

## User Stories
- As a developer working in a fledge repo, I want a one-command progress summary, so that I can see project health without running four commands and correlating their output.
- As an orchestrating agent, I want the same summary as machine-readable JSON, so that I can decide dispatch/merge actions from a single stable contract.

## Functional Criteria
1. FC-1: `fledge colony` prints task counts by status: totals for `blocked`, `ready`, `in-progress`, `done`, and overall.
2. FC-2: The report shows per-requirement completion: each requirement's ID, title, status, and done-tasks/total-tasks for the tasks referencing it.
3. FC-3: Tasks whose `requirement` references a REQ ID that does not exist are surfaced separately (not silently dropped and not folded into any requirement's counts).
4. FC-4: The report lists each blocked task together with the specific `depends_on` IDs that are not yet `done` (including dangling dependency IDs, which count as never-done).
5. FC-5: The report lists active locks: each locked task with the lock's owner, from `.fledge/broods/`.
6. FC-6: `--json` emits the same information as a single JSON document; the text and JSON views are generated from the same computed report.
7. FC-7: Degraded spec data does not abort the report: unparseable spec files and dangling references are summarized in a dedicated issues section (present in both text and JSON) while everything that parses is reported normally. The command exits 0 in this case.
8. FC-8: An empty or spec-less repository produces a valid report (zero counts, empty sections), exit 0.
9. FC-9: Exit codes follow the existing taxonomy: 0 success (including degraded-data reports), 2 usage error, 3 environment error (e.g. not a fledge repo). No report condition produces exit 1.

## Acceptance Criteria
- [x] AC-1: Tests written first and observed failing against the unchanged code, then passing after implementation (per-repo test-first convention).
- [x] AC-2: A txtar e2e suite covers: populated repo (counts, per-REQ completion, blocked-with-unmet-deps, locks), empty repo, degraded repo (parse error + dangling refs), and `--json` output shape.
- [x] AC-3: Running `fledge colony` in this repository after implementation reflects its own specs accurately (human-verified against `status`/`ready`/`locks`).
- [x] AC-4: `fledge preen` reports no findings for the spec set after authoring.

## Out of Scope
- Scoping/filtering (e.g. `fledge colony PLM-001`) — repo-wide only.
- Historical trends, burndown, or anything requiring state over time.
- New validation rules — `fledge preen` remains the validator; report only surfaces what it encounters while reading.
- Markdown/HTML export formats beyond text and `--json`.

## Open Questions
None — all decisions resolved in interrogation (2026-07-06).
