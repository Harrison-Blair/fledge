---
id: FTHR-034
title: Test broods stale-PID detection and brood abandon broods --json shapes
plumage: PLM-017
status: fledged
priority: P2
depends_on: []
authored: 2026-07-15T15:24:33Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-034: Test broods stale-PID detection and brood abandon broods --json shapes

## Description
Add txtar acceptance coverage for the broods-family stateful commands: the stale-claim
(`pid_alive`) detection that no test exercises today, and the `--json` output shapes of
`brood`, `broods`, and `abandon` — including `abandon --json`'s null-vs-string `status`
branch. Test-only; no production change.

Satisfies PLM-017 FC-1 (stale detection) and the broods-family portion of FC-2 (`--json`
shapes). Sibling FTHR-035 covers `status`/`set`; the two touch disjoint fixture files and
run in parallel.

## Affected Modules
- `cmd/fledge/testdata/lock.txtar` — the existing broods-family acceptance test (claim,
  conflict, `broods`, abandon flows). Extend it here. See `.fledge/nest/modules.md → cmd`.
- Behavior under test (not modified): `internal/cli/brood.go` — `pidAlive` (line ~198),
  the `(pid not alive)` text (lines ~188–191), the `pid_alive` JSON field (lines ~174–178),
  and the `abandon --json` `status` null-vs-string branch (lines ~142–149).

## Approach
1. **Stale-PID detection.** The `brood` command always stamps the claim with the live
   process's own PID, so seed a not-alive holder directly: add a `.fledge/broods/FTHR-###.brood`
   file block to the txtar archive with a PID that is not alive (a very large value such as
   `2147483647`, which `syscall.Kill(pid, 0)` reports as `ESRCH`). Run `fledge broods` and
   assert the `(pid not alive)` marker appears for it; run `fledge broods --json` and assert
   `"pid_alive": false` for that record. Keep an existing live-PID claim (via `fledge brood`)
   in the same run and assert it renders as alive, so both branches are covered.
2. **`--json` shapes.** Assert the machine output of the broods-family commands:
   - `fledge brood FTHR-### --owner X --json` → emits the claim record with the expected
     keys (`feather`, `owner`, `pid`, `created`, `branch`).
   - `fledge broods --json` → array of records each including `pid_alive`.
   - `fledge abandon FTHR-### --json` (no `--fledged`) → `status` is JSON `null`; and
     `fledge abandon FTHR-### --fledged --json` → `status` is the terminal string
     (`"fledged"`). This null-vs-string branch is the key distinct path.
   Use testscript `stdout` regex assertions on the emitted JSON (the harness asserts on
   stdout text); match on the key/branch, not on volatile values like timestamps.

Constraints: extend the existing `lock.txtar` rather than duplicating its setup; do not
touch `status.txtar`/`set.txtar` (FTHR-035's files). Assert behavior only — change no
command code.

## Tests
This feather *is* test authoring; "written test-first" here means the assertions are shown
to bite before they are relied on:
- Extend `cmd/fledge/testdata/lock.txtar` with the stale-PID and `--json` assertions above.
- Demonstrate they bite (PLM-017 AC-3): before finalizing, perturb the source to confirm
  failure — e.g. invert `pidAlive`'s sense or rename the `pid_alive`/`abandon.status` key,
  run `go test ./cmd/fledge -run TestScripts/lock`, observe the new assertions FAIL, then
  revert and confirm they pass. Record the failing output in the evidence file.

## Acceptance Criteria
- [x] AC-1: `lock.txtar` seeds a not-alive-PID claim and asserts both the `(pid not alive)` text and `"pid_alive": false` (`broods --json`), plus a live-PID claim asserted alive (FC-1).
- [x] AC-2: `lock.txtar` asserts the `--json` shapes of `brood`, `broods`, and `abandon`, including `abandon`'s `status` null (no `--fledged`) vs `"fledged"` (with `--fledged`) branch (FC-2, broods family).
- [x] AC-3: A recorded perturbation (inverted liveness check or renamed key) makes the new assertions fail; reverting restores green (satisfies PLM-017 AC-3 for these commands).
- [x] AC-4: `fledge preen` passes and `go test ./cmd/fledge -run TestScripts/lock` and `go test ./...` are green.
