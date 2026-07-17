---
id: FTHR-074
title: Verdict and escalation commands with generic ledger read
plumage: PLM-030
status: fledged
priority: P1
depends_on: [FTHR-072]
authored: 2026-07-16T22:22:50Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-074: Verdict and escalation commands with generic ledger read

## Description
Delivers the remaining two dedicated write commands from PLM-030's record-kind set — `fledge verdict <subject> --result pass|fail [--note "<text>"]` and `fledge escalate <subject> --message "<text>"` — plus the generic `fledge ledger read <subject> --kind <kind>` reader that works across all three kinds (status, verdict, escalation). Sibling to FTHR-073: both feathers depend only on FTHR-072's `internal/ledger` package and touch disjoint CLI files, so they're implementable in parallel.

## Affected Modules
- New CLI command files `internal/cli/verdict.go`, `internal/cli/escalate.go`, `internal/cli/ledgerread.go` (or `internal/cli/ledger.go` housing the `ledger read` subcommand if this codebase's registry pattern supports subcommand grouping — check `internal/cli/roster.go` for precedent, since `fledge roster assign/release` is this repo's existing example of a command with sub-verbs, per `.fledge/nest/entry-points.md`).
- `internal/cli/cli.go` — register `verdict`, `escalate`, and `ledger` (or `ledger read`) in `commandOrder`.
- Reads/writes via `internal/ledger.Write`/`Read` from FTHR-072 (`VerdictRecord`, `EscalationRecord` types already defined there) — no new package code expected.

## Approach
- `fledge verdict <subject> --result pass|fail [--note "..."]`: validates `--result` is exactly `pass` or `fail` (usage error otherwise), writes a `VerdictRecord{Result, Note}` for kind=`verdict`. `--json` emits the written record.
- `fledge escalate <subject> --message "<text>"`: writes an `EscalationRecord{Message}` for kind=`escalation`. `--json` emits the written record.
- `fledge ledger read <subject> --kind status|verdict|escalation`: reads and prints the record for that (subject, kind); a distinct not-found exit path (reuse `ExitFail` with a clear stderr message, consistent with how other read commands in this codebase report "not found" per `.fledge/nest/conventions.md`'s exit-code conventions) versus a real error.
- Follow the same registration and `--json`/exit-code conventions established in FTHR-072/FTHR-073 — no new patterns introduced here, this feather is a straightforward widening of the same shape to two more kinds plus a reader.

## Tests
- `internal/cli/verdict_test.go` / `escalate_test.go` / `ledgerread_test.go` (or co-located, matching whatever the existing `internal/cli/*_test.go` layout convention is):
  - `TestVerdictRejectsInvalidResult` — `--result maybe` exits `ExitUsage`, no record written.
  - `TestVerdictWritesRecord` / `TestEscalateWritesRecord` — happy-path write, confirmed via `internal/ledger.Read`.
  - `TestLedgerReadAllKinds` — table-driven over status/verdict/escalation, each written then read back matching.
  - `TestLedgerReadMissing` — reading a never-written (subject, kind) reports not-found, doesn't panic.
- `cmd/fledge/testdata/verdict.txtar`, `escalate.txtar`, `ledger-read.txtar`:
  - Happy path for each write command, `--json` output shape.
  - `verdict.txtar` malformed input: invalid `--result` value exits `ExitUsage`.
  - `ledger-read.txtar`: read after a `fledge heartbeat`/`fledge verdict`/`fledge escalate` write round-trips correctly; read on a subject/kind never written reports not-found cleanly.
- Implementation order fixed: write all tests first, confirm failing, then implement.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `fledge verdict` writes a valid `pass`/`fail` verdict record and rejects invalid `--result` values, satisfying part of PLM-030 FC-1.
- [x] AC-3: `fledge escalate` writes an escalation record, satisfying part of PLM-030 FC-1.
- [x] AC-4: `fledge ledger read` reads any of the three record kinds and reports a clean not-found outcome when absent.
- [x] AC-5: All three commands support `--json`, satisfying PLM-030 FC-6.
- [x] AC-6: `go test ./internal/cli/... ./cmd/fledge/...` passes.
