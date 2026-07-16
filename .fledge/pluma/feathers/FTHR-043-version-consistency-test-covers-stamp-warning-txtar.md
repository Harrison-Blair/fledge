---
id: FTHR-043
title: Version-consistency test covers stamp_warning.txtar
plumage: PLM-023
status: pipping
priority: P1
depends_on: []
authored: 2026-07-16T01:52:56Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-043: Version-consistency test covers stamp_warning.txtar

## Description
`internal/cli/version_test.go` already pins `binaryVersion` to the repo-root
`VERSION` file (`TestBinaryVersionMatchesVersionFile`), but nothing pins the
third must-move-together file: `cmd/fledge/testdata/stamp_warning.txtar`,
which hardcodes the current binary version twice in its script (the
"binary is 0.5.5" warning text and the "0.0.1" mismatched-stamp fixture
version it's compared against). A release bump that updates `VERSION` and
`version.go` but misses this fixture passes the existing test, then fails CI
on the stale txtar assertion and burns the version number. This feather
extends the version-consistency coverage to include the txtar fixture, so
one test catches a bump that misses any of the three files.

## Affected Modules
- `internal/cli/version_test.go` — existing version-consistency test, to be
  extended (see `.fledge/nest/testing.md` → CLI unit tests, `.fledge/nest/conventions.md` → Versioning & release).
- `cmd/fledge/testdata/stamp_warning.txtar` — the fixture whose pinned
  "binary is 0.5.5" string must agree with `VERSION`/`binaryVersion`.

## Approach
Add a new test function (or extend `TestBinaryVersionMatchesVersionFile`) in
`internal/cli/version_test.go` that reads `../../cmd/fledge/testdata/stamp_warning.txtar`,
extracts the version string from its `stderr 'fledge: scaffold was written by
fledge 0\.0\.1, binary is 0\.5\.5 —...'` line via a regexp capturing the
"binary is X.Y.Z" segment, and asserts it equals `binaryVersion` (and, by
transitivity via the existing test, the `VERSION` file). Fail with a message
naming the txtar file if it diverges, mirroring the existing test's error
style ("bump cmd/fledge/testdata/stamp_warning.txtar").

## Tests
- `TestStampWarningTxtarVersionMatchesBinary` (new): reads the txtar fixture,
  extracts the pinned "binary is X.Y.Z" version, asserts equality with
  `binaryVersion`. Written first; run against the current (already-correct)
  fixture to confirm it passes, then temporarily edit the fixture's pinned
  version to a divergent value and confirm the test FAILS for the expected
  reason (wrong-version message), then revert the fixture edit — this
  captures the required "observed failing" evidence without needing the bug
  to already exist in committed state (satisfies PLM-023 FC-1).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-043.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-023 FC-2"). AC-1 is always:
- [ ] AC-1: The tests listed above were observed failing before implementation (via the deliberate-divergence step) and pass after; evidence captured verbatim.
- [ ] AC-2: `go test ./internal/cli/...` passes, including the new test, at rest (fixture reverted to its correct pinned version) (satisfies PLM-023 FC-1, AC-1).
