---
id: FTHR-046
title: "Regenerate this repo's nest with correct counts and release facts"
plumage: PLM-023
status: fledged
priority: P1
depends_on: [FTHR-044]
oversight: during
authored: 2026-07-16T01:55:18Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-046: Regenerate this repo's nest with correct counts and release facts

## Description
Once FTHR-044's exact-computation-for-counts rule is in the core forager/scout
prose, this feather runs a fresh forager against this repo to regenerate
`.fledge/nest/`, and verifies the regenerated docs state the correct ground
truth: 19 commands (the pre-regeneration nest states 18) in
`entry-points.md`/`modules.md`/`index.md`, 25 txtar fixtures (the
pre-regeneration nest states 23) in `testing.md`/`modules.md`/`dependencies.md`, and
`conventions.md`'s "Versioning & release" section naming all three
must-move-together files (`VERSION`, `internal/cli/version.go`
`binaryVersion`, `cmd/fledge/testdata/stamp_warning.txtar`) rather than
dropping the third. This satisfies the parent plumage's FC-3 (count
correction) and AC-5 (release-file-list correction) together, since both are
outcomes of the same regeneration pass rather than separate mechanisms — the
release-file omission wasn't a "count," so it's fixed here by the scout
correctly reading source rather than by a second generic rule.

`oversight: during` — this is a real forager run against a moving target
(this repo's own state), not a mechanical code diff; the user reviews the
regenerated content (or spot-checks it) before it's accepted, relayed via the
orchestrator as decision checkpoints during the build rather than only at
merge.

## Affected Modules
- `.fledge/nest/entry-points.md`, `modules.md`, `index.md`, `testing.md`,
  `dependencies.md`, `conventions.md` — regenerated forager output (see
  `.fledge/nest/modules.md` → bootstrap; these are the concern docs the
  finding's ground truth was checked against).
- Ground truth sources the regeneration must agree with: `internal/cli/cli.go`
  `commandOrder` (19 commands), `cmd/fledge/testdata/*.txtar` (25 files),
  `VERSION` / `internal/cli/version.go` / `cmd/fledge/testdata/stamp_warning.txtar`
  (the three release files, per FTHR-043).

## Approach
Obtain a fresh forager per `foraging.md`'s Commissioner protocol (a new
species, one-shot) to regenerate `.fledge/nest/`. Because FTHR-044 has
already landed, the scout/forager prose now requires counts to be derived
from an exact computation rather than eyeballed, so the regenerated
`entry-points.md`/`testing.md`/etc. should state 19 and 25 correctly; verify
this explicitly rather than assuming. Also verify `conventions.md` lists all
three release files. If the regeneration still gets a fact wrong despite
FTHR-044 landing, treat that as a defect to fix in this feather's scope
(before closing it out) rather than deferring — the whole point of this
plumage is that these facts are correct once regenerated.

## Tests
Structural (grep/count) assertions against the regenerated `.fledge/nest/`
docs, run after regeneration and recorded as this feather's verification
evidence:
- `grep -c` / exact-match assertions that `entry-points.md`, `modules.md`,
  and `index.md` state "19" (commands) and not "18".
- Assertions that `testing.md`, `modules.md`, and `dependencies.md` state
  "25" (txtar fixtures) and not "23".
- An assertion that `conventions.md`'s "Versioning & release" section
  mentions `stamp_warning.txtar` alongside `VERSION` and `version.go`.
- `fledge nest status` reports complete, with `index.md`'s `commit` at HEAD.
Since this feather's product is regenerated prose rather than new code, these
are structural checks run and recorded (not a `go test` suite), per the
plumage's original verification-first framing for this feather. Run them
first against the *current* (wrong) nest content to confirm they FAIL for the
expected reason (states 18/23/two-files), then regenerate and re-run until
they pass (satisfies PLM-023 FC-3, AC-1).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-046.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-023 FC-2"). AC-1 is always:
- [x] AC-1: The structural checks listed above were observed failing against
  the pre-regeneration nest content, and pass against the regenerated
  content; evidence captured verbatim.
- [x] AC-2: `entry-points.md`, `modules.md`, and `index.md` state 19 commands
  (satisfies PLM-023 FC-3, AC-3).
- [x] AC-3: `testing.md`, `modules.md`, and `dependencies.md` state 25 txtar
  fixtures (satisfies PLM-023 FC-3, AC-3).
- [x] AC-4: `conventions.md`'s "Versioning & release" section names all three
  must-move-together files (satisfies PLM-023 AC-5).
- [x] AC-5: `fledge nest status` reports complete and stamped to HEAD after
  regeneration.
