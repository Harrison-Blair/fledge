---
id: FTHR-001
title: "report command tracer: status counts and per-REQ completion"
plumage: PLM-001
status: fledged
priority: P2
depends_on: []
oversight: merge
authored: 2026-07-06T23:43:27Z
agent: fledge-orchestrate/planning
fledge_version: 0.1.0
---

# FTHR-001: report command tracer: status counts and per-REQ completion

## Description
The tracer-bullet slice of PLM-001: a working `fledge colony` command, end to end. It computes and prints task counts by status (FC-1), per-requirement completion with orphan tasks surfaced separately (FC-2, FC-3), in both human text and `--json` from one computed report value (FC-6). Empty repos produce a valid zero report (FC-8); exit codes follow the 0/2/3 taxonomy with no exit-1 paths (FC-9). Blocked-task detail, locks, and the degraded-data issues section are deliberately deferred to FTHR-002 — but the report struct this task defines is the seam FTHR-002 extends.

## Affected Modules
- `internal/cli` — new file `internal/cli/report.go`, plus registering `report` in `commandOrder` in `internal/cli/cli.go` (see `.fledge/nest/modules.md` → internal/cli; `.fledge/nest/conventions.md` → one-command-per-file, dual output).
- `cmd/fledge` — new e2e suite `cmd/fledge/testdata/report.txtar` (see `.fledge/nest/testing.md`).

## Approach
Follow the existing command pattern exactly (model: `internal/cli/ready.go`):
- `init() { register("report", runReport, "fledge colony [--json]") }`; add `"report"` to `commandOrder` in `cli.go`.
- Reuse `loadSet()` (`internal/cli/specload.go`) for repo detection + spec loading; its env/exit handling gives FC-9 for free. Unlike `ready`, do NOT refuse on check errors — report is an observer (PLM-001 FC-7); this task simply reports over whatever parsed (`set.Errors` handling proper is FTHR-002).
- Define an exported-shape report struct with JSON tags, e.g. `report{Counts counts; Requirements []reqCompletion; Orphans []orphanTask}` where counts has `blocked/ready/in_progress/done/total` and reqCompletion has `id/title/status/done/total`. Compute once; render text or `emitJSON(report)` (`internal/cli/cli.go:64`) from the same value. Requirements sorted by ID; orphan tasks (dangling `requirement` ref) listed by ID under their own heading, never counted into any REQ.
- Keep the struct open for FTHR-002 to add `Blocked`, `Locks`, `Issues` fields without reshaping existing output.

## Tests
`cmd/fledge/testdata/report.txtar`, testscript-driven like the existing suites (`ready.txtar`, `status.txtar`):
- populated repo: two PLMs with feathers in mixed statuses → asserts status counts, per-PLM fledged/total lines, and an orphan feather (plumage: PLM-999) surfaced separately (pins FC-1, FC-2, FC-3).
- `--json`: same repo → asserts JSON fields for counts, requirements, orphans (pins FC-6).
- empty repo (`.fledge/` present, no specs) → valid zero report, exit 0 (pins FC-8).
- usage error (`fledge colony --bogus`) → exit 2; outside a fledge repo → exit 3 (pins FC-9).

Implementation order is fixed: (1) write the txtar; (2) run `go test ./cmd/fledge -run TestScript/report` against unchanged code and confirm it FAILS with `unknown command "report"`; (3) implement until green.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation (unknown-command failure captured) and pass after.
- [x] AC-2: `fledge colony` and `fledge colony --json` satisfy PLM-001 FC-1, FC-2, FC-3, FC-6, FC-8, FC-9 as pinned by the txtar assertions.
- [x] AC-3: `go test ./...` green; `go vet ./...` clean; full suite unaffected.
- [x] AC-4: `fledge colony` appears in the usage listing (`commandOrder`).
