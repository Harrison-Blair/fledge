---
id: FTHR-035
title: Test status and set --json output shapes
plumage: PLM-017
status: fledged
priority: P2
depends_on: []
authored: 2026-07-15T15:24:33Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-035: Test status and set --json output shapes

## Description
Add txtar acceptance coverage for the `--json` output shapes of `fledge status` and
`fledge set`, which today are asserted only for human-readable output. Test-only; no
production change.

Satisfies the `status`/`set` portion of PLM-017 FC-2. Sibling FTHR-034 covers the
broods-family commands; the two touch disjoint fixture files and run in parallel.

## Affected Modules
- `cmd/fledge/testdata/status.txtar` and `cmd/fledge/testdata/set.txtar` — the existing
  acceptance tests for these commands, which assert only human-readable output. Extend them.
  See `.fledge/nest/modules.md → cmd`.
- Behavior under test (not modified): `internal/cli/status.go` (`--json` emits
  `{id, from, to}`, lines ~85–86) and `internal/cli/set.go` (`--json` emits
  `{id, field, value}`, lines ~119–120).

## Approach
1. In `status.txtar`, drive a status transition with `--json` (e.g. `fledge status FTHR-###
   <next> --json`) and assert the emitted object has the keys `id`, `from`, and `to` with
   the expected values for that transition.
2. In `set.txtar`, drive a `fledge set` mutation with `--json` (e.g.
   `fledge set FTHR-### priority P2 --json`) and assert the emitted object has keys `id`,
   `field`, and `value` with the expected values.
3. Use testscript `stdout` regex assertions matching the keys and the specific
   from/to/field/value for the driven mutation, not volatile fields.

Constraints: extend the existing `status.txtar`/`set.txtar` rather than adding new files;
do not touch `lock.txtar` (FTHR-034's file). Assert behavior only — change no command code.

## Tests
This feather is test authoring; the assertions are shown to bite before being relied on:
- Extend `cmd/fledge/testdata/status.txtar` and `cmd/fledge/testdata/set.txtar` with the
  `--json` assertions above.
- Demonstrate they bite (PLM-017 AC-3): perturb the source — e.g. rename `from`/`to` in
  `status.go` or `field`/`value` in `set.go` — run `go test ./cmd/fledge -run
  TestScripts/status` and `.../set`, observe the new assertions FAIL, then revert and
  confirm they pass. Record the failing output in the evidence file.

## Acceptance Criteria
- [x] AC-1: `status.txtar` asserts `status --json` emits `{id, from, to}` with the correct values for a driven transition (FC-2, status).
- [x] AC-2: `set.txtar` asserts `set --json` emits `{id, field, value}` with the correct values for a driven mutation (FC-2, set).
- [x] AC-3: A recorded perturbation (renamed JSON key in `status.go`/`set.go`) makes the new assertions fail; reverting restores green (satisfies PLM-017 AC-3 for these commands).
- [x] AC-4: `fledge preen` passes and `go test ./cmd/fledge -run TestScripts/status`, `.../set`, and `go test ./...` are green.
