---
id: FTHR-084
title: preen advisory report for untidied completed work
plumage: PLM-032
status: egg
priority: P2
depends_on: [FTHR-083]
authored: 2026-07-17T03:32:42Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-084: preen advisory report for untidied completed work

## Description

Makes `fledge preen` report how much completed work is waiting to be roosted.

This exists to counter the one honest cost of the explicit-command design. Relocation was
deliberately kept out of the merge path (PLM-032 FC-2), and the price is that tidiness
drifts until someone runs the command. FTHR-085 makes phase-close run it; this feather makes
the drift *visible* in the meantime, so the distinction the directories are meant to carry
does not decay silently.

**The report is advisory and must never fail the check.** `preen` gates the build and runs
in CI; an untidied repository is not a broken one. Getting this wrong would turn a
convenience into a CI outage — that constraint is the substance of this feather, not a
footnote.

Depends on FTHR-083 for the eligibility predicate. It is deliberately *reused*, not
reimplemented: a second copy of the unit rule would drift from the first, and then `preen`
would advertise a count `roost` disagrees with.

Parallel-safe with FTHR-085 (`internal/bootstrap/core/**`) and FTHR-086 (`.fledge/pluma/**`):
this feather's change is contained to `internal/cli/preen.go`.

Satisfies PLM-032 FC-12.

## Affected Modules

See `.fledge/nest/modules.md` → `internal/cli`, `internal/check`;
`.fledge/nest/conventions.md` (exit codes; `--json` on every command);
`.fledge/nest/testing.md` (`preen.txtar`, `check_test.go`).

- `internal/cli/preen.go` — the whole change. `runCheck` (`preen.go:16`), the summary line
  (`preen.go:61-63`), `summaryLine` (`preen.go:184`).
- `internal/cli/roost.go` — **read only**; consumes FTHR-083's exported predicate.
- `internal/check/check.go` — **read only**. See Approach: the count is deliberately *not* a
  check.Finding.

## Approach

**Do not make it a `check.Finding`.** This is the central design decision. `internal/check`
produces findings with error/warning severity, and `runCheck` derives its exit code from
them (`preen.go:57-63`, `summaryLine` at `preen.go:184`). Routing the count through that
machinery risks it counting toward the error tally — and `check_test.go` covers 15 named
rules whose semantics would then need re-reasoning. Instead, compute the count in
`preen.go` from the already-loaded `*spec.Set` and print it as a separate advisory line,
outside the findings loop and outside the exit-code calculation.

**Reuse the predicate.** Call FTHR-083's exported eligibility function on the `Set` `preen`
already loads. No extra I/O, no second traversal, and — the point — no second copy of the
unit rule.

**Wording.** Report the count of eligible *units* (plumages), not files: the unit is what
`roost` acts on, so a count of files would not match what the command then reports. Say
nothing when the count is zero — a clean repo should stay quiet, and silence is what AC-4
pins.

**`--json`.** Add the count as a field alongside the existing scaffold/spec output rather
than restructuring the payload; consumers parse the existing shape.

**Constraint: exit code is untouched, always.** No input — untidied repo, tidied repo,
repo with findings — may change `preen`'s exit status relative to today. AC-2 and AC-3 pin
both directions.

## Tests

Acceptance tests in `cmd/fledge/testdata/preen_roost.txtar` (**new file**, deliberately not
an edit to the existing `preen.txtar`, so this feather stays merge-disjoint from the other
four). Per `.fledge/nest/testing.md`, `preen`'s user-visible behavior is covered by txtar,
which is the right level for an output-and-exit-code change.

Run: `go test ./cmd/fledge -run TestScripts/preen_roost`, then `go test ./...`.

- *untidied repo reports the count and still succeeds* — a repo with one fully-fledged unit
  at top level; `fledge preen` names the count of units awaiting roosting and **exits 0**.
  The exit code is the assertion that matters → AC-2, PLM-032 AC-10.
- *untidied repo with a real error still fails, and still reports the count* — an eligible
  unit plus a genuine validation error; exit is non-zero **because of the error**, and the
  advisory line is still present. Pins that the advisory neither causes nor masks a failure
  → AC-3.
- *tidied repo says nothing* — after roosting, no advisory line appears; exit 0 → AC-4,
  PLM-032 AC-10.
- *ineligible units are not counted* — a fledged plumage with one non-fledged feather is
  **not** reported as awaiting roosting, matching what `roost` would actually do → AC-5.
- *count matches what roost relocates* — `preen` reports N units; `fledge roost` then
  relocates exactly N. This is the anti-drift test: it fails if anyone reimplements the
  predicate → AC-5.
- *`--json` carries the count* — present when non-zero, and the existing payload shape is
  unchanged → AC-6.

Test-first order is fixed: write these, run them against unchanged code and observe them
FAIL for the expected reason (no advisory line emitted), then implement until they pass.

## Acceptance Criteria

- [ ] AC-1: The tests listed above were observed failing before implementation and pass
      after.
- [ ] AC-2: On a repository with completed work awaiting relocation, `fledge preen` reports
      how many units are waiting and exits 0. Satisfies PLM-032 FC-12, AC-10.
- [ ] AC-3: The advisory never affects the exit status: a repository with real findings
      fails for those findings alone, and a repository whose only "issue" is untidied work
      succeeds.
- [ ] AC-4: On a tidied repository, no advisory is reported. Satisfies PLM-032 AC-10.
- [ ] AC-5: The reported count is computed with FTHR-083's eligibility predicate and equals
      the number of units `fledge roost` would relocate — ineligible units are excluded.
- [ ] AC-6: `--json` output carries the count when non-zero, with the existing payload shape
      otherwise unchanged.
- [ ] AC-7: `go test ./...` passes with every existing fixture, including `preen.txtar`,
      unmodified.
