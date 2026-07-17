---
id: FTHR-088
title: Existence-wait mode and mandatory timeout for fledge await
plumage: PLM-034
status: fledged
priority: P1
depends_on: []
authored: 2026-07-17T04:07:23Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-088: Existence-wait mode and mandatory timeout for fledge await

## Description

Fix both `fledge await` deadlocks identified in PLM-034 by adding an opt-in existence-wait mode (`--exists`), making `--timeout` mandatory on both wait paths, reworking `await.txtar`'s happy path to be race-free by construction, and stating the correct wait mode per record kind in the command's own usage text.

This is the whole of PLM-034 in one feather. There is no parallelism available and no split worth making: every criterion lands in `internal/cli/await.go`, and the txtar rework is structurally dependent on `--exists` existing (its reworked script writes the record *first*, which is precisely the condition that deadlocks change-wait today). Splitting would force `await.txtar` to be written twice and leave an intermediate commit with a half-changed contract.

## Affected Modules

Three files, and no others (verified by grep during planning — see Approach):

- `internal/cli/await.go` — the command and its polling loop. See `.fledge/nest/modules.md` → `internal/cli`, and `.fledge/nest/entry-points.md` for the CLI command surface.
- `internal/cli/await_test.go` — existing fake-clock unit tests (`TestAwaitReturnsOnAppearance`, `TestAwaitReturnsOnChange`, `TestAwaitTimesOutNoChange`). See `.fledge/nest/testing.md` → "Unit tests".
- `cmd/fledge/testdata/await.txtar` — the testscript acceptance script. See `.fledge/nest/testing.md` → "Acceptance tests".

**Deliberately untouched:** `internal/ledger` (record shapes, `Read`/`Write`), `internal/lock`, `internal/cli/cli.go`, and FTHR-073's spec and `.fledge/molt/FTHR-073.md` evidence (PLM-034 Out of Scope — the fledged record stays an honest account of what shipped).

## Approach

**Findings from planning that save you a wrong turn — do not re-derive these:**

- **`"await"` is already in `commandOrder` (`internal/cli/cli.go:108`).** This feather modifies an existing command; it registers nothing. **No `commandOrder` edit, no `fledge init --refresh`, no `scaffold.json` rewrite** — and therefore none of the merge contention that scaffold-touching feathers suffer.
- **`await`'s usage string is not echoed into any generated or scaffold file.** The only occurrences outside `await.go` are spec/molt documents, which are historical records and stay untouched.
- **`ExitTimeout` (4) already exists** in `cli.go` — reuse it, don't add an exit code.

**Existence-wait (`--exists`, FC-1).** Add an `exists bool` parameter to `pollAwait`. In existence-wait, the loop returns `awaitResult{record: rec}` as soon as `read` yields a record — with no baseline sampling and **without consulting `Payload` or `Timestamp` at all**. That is the property that makes it immune to both deadlocks, and it is why the `RFC3339` second-granularity problem (PLM-034 Context) does not need solving: existence-wait never compares timestamps. Keep the existing `awaitClock` seam — it is what makes these tests fast and deterministic.

**Change-wait (FC-2) is unchanged.** Without `--exists`, baseline-then-poll behavior stays exactly as FTHR-073 shipped it. Do not "improve" it.

**Mandatory `--timeout` (FC-3).** Enforce at the **CLI boundary in `runAwait`**, not in `pollAwait`: reject a missing `--timeout` with `usageErr(...)` naming the flag. Place the check **before `repo.Find()`** (as the existing `--kind` check is) so it is a pure usage error that needs no repo — this also makes it unit-testable by calling `runAwait` directly. `pollAwait` keeps its `hasTimeout` parameter so its unit tests can still exercise the unbounded loop in isolation; the guarantee that no *user* can request an unbounded wait lives at the boundary.

**Usage text (FC-7).** Extend `await`'s registered usage/help text to name the per-kind guidance: `verdict`/`escalation` → `--exists`; `status` → change-wait. This is the shipped home for the constraint because the orchestration prose does not discuss the ledger yet — adding it there is FTHR-075's job under PLM-030 and is explicitly out of scope here.

**txtar rework (FC-6).** Delete the backgrounded happy path. Replace with: write the record via `heartbeat`, *then* `fledge await ... --exists --timeout <d>` returns immediately. One process at a time, no `&`, no `wait`. Add `--timeout` to every `await` invocation in the file, and add usage-error coverage for its omission.

**Precedent to reuse for the AC-10 help-text assertion:** `internal/cli/command_parity_test.go` (guards usage/registration drift) and `internal/doctest/docs_test.go` / `claude_md_test.go` (assert prose against reality via tolerant substring extraction, not snapshots). Follow that pattern — a substring assertion, not a golden file.

## Tests

Order is fixed: **(1)** write these tests; **(2)** run them against unchanged code and capture the FAIL output **verbatim** into `.fledge/molt/FTHR-088.md` under a bare `## AC-1` heading, confirming each fails for the *expected* reason (undefined `--exists`/parameter arity, not an unrelated compile break); **(3)** implement until they pass.

**Unit — `internal/cli/await_test.go`** (fake clock; no real sleeps):

- `TestAwaitExistsReturnsImmediatelyWhenPresent` → **AC-2**. Record present at call time; `--exists` returns it on the first read with no sleep. *This is the exact condition that deadlocks today* — it must hang or fail against unchanged code.
- `TestAwaitExistsReturnsOnAppearance` → **AC-3**. Absent at call time, appears mid-poll, returns it.
- `TestAwaitExistsIgnoresIdenticalPayloadRewrite` → **AC-4**. A `read` returning a byte-identical payload still satisfies existence-wait immediately; asserts the payload is never consulted.
- `TestAwaitChangeWaitStillDetectsPayloadChange` → **AC-5**. Guards FTHR-073's shipped change-wait against regression (extend/keep existing `TestAwaitReturnsOnChange`).
- `TestAwaitExistsTimesOut` → **AC-7**. Fake clock past deadline in `--exists` mode → `timedOut: true`, `record: nil`.
- `TestAwaitTimesOutNoChange` (existing) → **AC-7**, change-wait side. Keep.
- `TestAwaitRequiresTimeoutBothModes` → **AC-6**. Calls `runAwait` directly with and without `--exists`, each omitting `--timeout`; asserts `ExitUsage` (2) both times. Needs no repo because the check precedes `repo.Find()`.
- `TestAwaitUsageTextNamesPerKindModes` → **AC-10**. Asserts `await`'s usage/help text mentions the per-kind guidance (substring assertion, per `command_parity_test.go` precedent).

**Acceptance — `cmd/fledge/testdata/await.txtar`** (reworked, race-free):

- Happy path → **AC-2, AC-8**: `heartbeat` writes the record, then `await --exists --timeout 5s` returns it immediately. Single process; no `&`, no `wait`.
- Timeout path, both modes → **AC-7**: `--exists --timeout 200ms` and change-wait `--timeout 200ms` against a never-written subject; assert exit 4 via the existing `sh -c '...; test $? -eq 4'` idiom (testscript's `!` only distinguishes zero from non-zero).
- Missing `--timeout`, both modes → **AC-6**: usage error, exit 2.
- Missing `--kind` (existing) → keep.

**Flake-gone demonstration → AC-9.** Concretely runnable, and the output goes into the evidence file verbatim:

```sh
go test ./cmd/fledge -run 'TestScripts/await' -count=20 -timeout 120s
```

`-count=20` defeats Go's test cache and runs the script 20 consecutive times. Pair it with the structural argument required by AC-9.

## Acceptance Criteria

- [x] AC-1: The tests listed above were observed failing before implementation and pass after, with the pre-implementation FAIL output captured verbatim in `.fledge/molt/FTHR-088.md`.
- [x] AC-2: `fledge await <subject> --kind <kind> --exists --timeout <d>` returns successfully and immediately when the record already exists at call time — the exact condition that deadlocks today — proven by a test that fails against the current code (satisfies PLM-034 FC-1, AC-2).
- [x] AC-3: `--exists` returns successfully when the record is absent at call time and appears while blocking (PLM-034 FC-1, AC-3).
- [x] AC-4: `--exists` is immune to the identical-payload rewrite defect and never consults the payload (PLM-034 FC-1, AC-4).
- [x] AC-5: Change-wait remains the default and its behavior is unchanged from FTHR-073 for the `status` kind, including detection of a payload change from the call-time baseline (PLM-034 FC-2, AC-5).
- [x] AC-6: Omitting `--timeout` is a usage error exiting `ExitUsage` (2) with a message naming the flag, proven separately on both the existence-wait and change-wait paths (PLM-034 FC-3, AC-6).
- [x] AC-7: The timeout path exits `ExitTimeout` (4) with `timed_out: true` and the last-known record (or `null`) under `--json`, proven for both modes with a real elapsed-time test, not mocked away (PLM-034 FC-4, AC-7).
- [x] AC-8: `cmd/fledge/testdata/await.txtar` contains no `&` backgrounding and no `wait`, restoring the property that no file in the txtar suite races two processes. Cheaply checkable: `grep -nE '(^|[[:space:]])&[[:space:]]*$|^wait$' cmd/fledge/testdata/await.txtar` returns nothing (PLM-034 FC-6, AC-8).
- [x] AC-9: The flake is demonstrated gone by 20 consecutive green runs of the reworked script (`go test ./cmd/fledge -run 'TestScripts/await' -count=20 -timeout 120s`), output captured verbatim in the evidence file, **accompanied by a structural argument** naming why the race is eliminated by construction rather than merely unlikely. The argument carries the claim; the runs are the backstop. (A single green run proves nothing: at the measured ~1-in-3 hang rate it would pass ~67% of the time with the bug fully intact.) (PLM-034 AC-9)
- [x] AC-10: `fledge await`'s usage/help text states the correct wait mode per record kind, asserted by a test so the guidance cannot silently drift from the behavior it describes (PLM-034 FC-7, AC-10).
- [x] AC-11: `go test ./...` is green, `go vet ./...` and `gofmt -l .` are clean, and `fledge preen` passes (PLM-034 AC-11).
