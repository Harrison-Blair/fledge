---
id: FTHR-073
title: fledge await command
plumage: PLM-030
status: fledged
priority: P1
depends_on: [FTHR-072]
authored: 2026-07-16T22:21:37Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-073: fledge await command

## Description
Delivers `fledge await <subject> --kind <kind> [--timeout <duration>]`, the blocking wake-up primitive from PLM-030: it polls the named ledger record until it first appears or its content changes from what it was at call time, then returns. Replaces ad hoc "wait and hope the next message means something" with a deterministic, timeout-bounded command any worker or the orchestrator can call directly. Widens the tracer bullet FTHR-072 established (ledger package + one CLI command) to the second CLI command, built purely on `internal/ledger.Read` — no changes to FTHR-072's package internals expected, only additive use of its public API.

## Affected Modules
- New CLI command file `internal/cli/await.go`, registered via the same `init()`/`register()` pattern as every other command (`.fledge/nest/conventions.md` → Go code conventions; see `internal/cli/cli.go`).
- `internal/cli/cli.go` — add the new `ExitTimeout` constant alongside `ExitOK/Fail/Usage/Env`, and register `await` in `commandOrder`.
- Reads (never writes) via `internal/ledger.Read` from FTHR-072 — no new package code expected here.

## Approach
- `fledge await <subject> --kind <kind> [--timeout <duration>]`: at call time, attempt `ledger.Read(dir, subject, kind)`. Record the outcome as the baseline (either the current content hash, or "absent" if not-found).
- Poll on a short fixed interval (recommend 1s, matching the "1-2s" range agreed during plumage interrogation) via `ledger.Read` again each tick:
  - If previously absent and now present → return the new record, exit `ExitOK`.
  - If previously present and the content differs (compare marshaled JSON or a hash of it) → return the new record, exit `ExitOK`.
  - If `--timeout` elapses with neither of the above → print the last-known record (or `null` if still absent) and exit `ExitTimeout`.
  - No `--timeout` given → block indefinitely (still polling, no busy-loop — same interval).
- `--json` output always includes the record (or `null`) and, specifically on the timeout path, an explicit `"timed_out": true` field (present and `false` on the success path, or simply omitted on success — pick whichever is more consistent with this codebase's `omitempty` convention per `.fledge/nest/conventions.md`).
- Uses `context`/timer-based cancellation rather than a manual sleep loop with elapsed-time bookkeeping, for a clean, testably-mockable time source — inject a `now func() time.Time` and a `sleep func(time.Duration)` (or an equivalent small interface) so the timeout test in Tests below can run fast without a real multi-second sleep... **except** PLM-030 AC-3 explicitly requires the timeout path to be proven with a *real* elapsed-time test, not mocked away — so provide the injectable clock for unit-level coverage of the polling loop's logic, AND include one CLI-level (txtar) test using a short real `--timeout` (e.g. `100ms`–`500ms`) against a record that genuinely never appears, asserting on wall-clock-observed exit code and elapsed time bounds. Both together satisfy AC-3 without making the full suite slow (small unit-level fake-clock tests do the bulk of coverage; only one test pays real wall-clock cost).

## Tests
- `internal/cli/await_test.go` (or wherever the codebase's convention places CLI-logic-level tests — check `internal/cli/*_test.go` for the existing pattern before placing this):
  - `TestAwaitReturnsOnAppearance` — record absent at start, appears mid-poll (fake clock/injected write), await returns it, no error.
  - `TestAwaitReturnsOnChange` — record present at start, changes mid-poll, await returns the new value.
  - `TestAwaitTimesOutNoChange` — fake clock advances past `--timeout` with no change; returns `ExitTimeout`-equivalent outcome and the last-known record.
- `cmd/fledge/testdata/await.txtar`:
  - Happy path: a record is written (via `fledge heartbeat` from FTHR-072) after `fledge await` is invoked in the background/sequenced so the change is observed; `--json` shows the record, exit `0`.
  - Real-elapsed-time timeout: `fledge await some-subject --kind status --timeout 200ms` against a subject with no record ever written; asserts exit code equals the new `ExitTimeout` value and `--json` shows `"timed_out": true`.
  - Malformed input: missing `--kind` exits `ExitUsage`.
- Implementation order fixed: write all tests first, confirm they fail (command doesn't exist / `ExitTimeout` undefined), then implement.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `fledge await` blocks until the target record appears or changes, or a `--timeout` elapses, satisfying PLM-030 FC-5.
- [x] AC-3: The timeout path exits the new dedicated `ExitTimeout` code (distinct from `ExitFail`) and is proven by a real-elapsed-time txtar test, satisfying PLM-030 AC-3.
- [x] AC-4: `fledge await --json` output includes the record (or `null`) and an explicit `timed_out` field on the timeout path, satisfying PLM-030 FC-6.
- [x] AC-5: `go test ./internal/cli/... ./cmd/fledge/...` passes.
