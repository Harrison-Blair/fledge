---
id: TASK-002
title: "report widening: blocked detail, active locks, degraded-data issues"
requirement: REQ-001
status: blocked
priority: P2
depends_on: [TASK-001]
authored: 2026-07-06T23:45:00Z
agent: fledge-orchestrate/planning
fledge_version: 0.1.0
---

# TASK-002: report widening: blocked detail, active locks, degraded-data issues

## Description
Widens the TASK-001 tracer to complete REQ-001: the report additionally lists each blocked task with its specific not-yet-done `depends_on` IDs (FC-4), active locks with owners (FC-5), and a degraded-data issues section covering unparseable spec files and dangling references, still exiting 0 (FC-7). Both new sections appear in text and `--json` from the same computed report value (FC-6 continues to hold).

## Affected Modules
- `internal/cli` — extend `internal/cli/report.go` (the struct/render seam TASK-001 left open); reuse lock listing from `internal/lock` as `internal/cli/lock.go` does (see `.fledge/context/modules.md` → internal/cli, internal/lock).
- `cmd/fledge` — extend `cmd/fledge/testdata/report.txtar` (see `.fledge/context/testing.md`).

## Approach
- Add `Blocked []blockedTask`, `Locks []lockEntry`, `Issues issues` fields to the report struct from TASK-001; render as additional text sections and JSON keys without reshaping TASK-001's output (its txtar assertions must keep passing unmodified).
- Blocked detail: for each task not `done` with at least one dependency not `done`, list those unmet dependency IDs; dangling `depends_on` IDs count as never-done and are included (and also surface under issues as dangling refs). Reuse `graph`/`spec.Set` lookups — no new graph algorithms.
- Locks: read `.fledge/locks/` via the existing `internal/lock` listing used by `fledge locks`; report task ID + `Record.Owner` only (no PID liveness judgment — informational, per domain.md).
- Issues: `set.Errors` (per-file parse failures) plus dangling `requirement`/`depends_on` refs observed while computing. Report never exits 1 on these (FC-7/FC-9); it observes, `check` validates.

## Tests
Extend `cmd/fledge/testdata/report.txtar`:
- blocked repo: a dep chain with one done, one not, one dangling dep → asserts each blocked task line names exactly its unmet dep IDs (pins FC-4).
- locked repo: `fledge lock TASK-###` then `report` → asserts the lock section shows the task and owner (pins FC-5).
- degraded repo: one file with broken frontmatter + a task with a dangling requirement ref → report succeeds (exit 0), issues section names the parse failure and dangling refs, parsed specs still summarized (pins FC-7).
- `--json`: asserts `blocked`, `locks`, `issues` keys in the same document (pins FC-6).

Implementation order is fixed: (1) write the new txtar sections; (2) run `go test ./cmd/fledge -run TestScript/report` against the merged TASK-001 code and confirm the new assertions FAIL (sections absent from output); (3) implement until green.

## Acceptance Criteria
1. AC-1: The tests listed above were observed failing before implementation (missing-section failures captured) and pass after.
2. AC-2: Report output satisfies REQ-001 FC-4, FC-5, FC-7 as pinned by the txtar assertions; TASK-001's existing assertions pass unmodified.
3. AC-3: `go test ./...` green; `go vet ./...` clean.
4. AC-4: `fledge report` run in this repository reflects its own specs accurately (REQ-001 AC-3 self-hosting check).
