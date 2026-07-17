---
id: FTHR-086
title: "Roost this repository's completed specs"
plumage: PLM-032
status: egg
priority: P2
depends_on: [FTHR-083]
oversight: merge
authored: 2026-07-17T03:32:42Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-086: Roost this repository's completed specs

## Description

Runs `fledge roost` once against this repository and commits the result. This is the
feather where the feature stops being a capability and becomes the thing the user asked
for.

Scale, measured at authoring time: **25 fledged plumages and ~70 feathers** relocate as
units. The active directories go from 31 plumages / 81 feathers to roughly **6 plumages and
8 feathers** — only drafting and ready-to-implement work, which was the entire request.

Three fledged feathers stay at top level on purpose: FTHR-072 (under PLM-030, still
hatched) and FTHR-077/080 (under PLM-031, still hatched). Their units are not eligible
because their plumages are open. **This is correct under the per-plumage rule the user
chose, not a defect** — a reviewer seeing fledged feathers left behind should read this
paragraph, not file a bug.

`oversight: merge` — this is one commit containing ~95 renames, and the user signs off on
the diff before it merges.

Depends on FTHR-083 for the command. Parallel-safe with FTHR-084 (`internal/cli/preen.go`)
and FTHR-085 (`internal/bootstrap/core/**`): this feather touches only `.fledge/pluma/**`
and no Go code at all.

Satisfies PLM-032 FC-14, AC-12.

## Affected Modules

See `.fledge/nest/data-model.md` (spec directory layout); `.fledge/nest/testing.md` (suite
entry points).

- `.fledge/pluma/plumage/**`, `.fledge/pluma/feathers/**` — ~95 files relocated. **No Go
  code changes in this feather.**

## Approach

**This is a data migration, not a code change.** Run the command, verify nothing moved
semantically, commit.

**Sequence.**
1. Confirm a clean tree first. `fledge broods` must show no live claim, and no feather may
   be mid-flight — a concurrent brooder holding a spec file open while ~95 files move is
   exactly the collision this design avoids elsewhere. If anything is in flight, wait.
2. Capture a **pre-migration baseline**: `fledge vee --format json`, `fledge colony --json`,
   `fledge unfledged --json`, `fledge ready --json` to a scratch file.
3. Run `fledge roost`.
4. Capture the same four outputs and compare against the baseline. **They must be identical
   except for reported file paths.** This is the whole verification: it proves ~95 renames
   changed nothing about what the tooling reports, which is PLM-032 AC-2's claim tested on
   real data rather than a fixture.
5. Run `go test ./...` and `fledge preen`. Both green.
6. Commit as a single commit. Git detects the renames from content identity — `os.Rename`
   preserves bytes, so this shows as renames, not delete+add.

**A risk already retired.** `preen`'s validation rules were checked during planning and are
already location-independent: `checkIDFilename` (`check.go:242`) derives the filename from
`filepath.Base(path)`, and `criteria-evidence` (`check.go:279`) builds evidence paths from
`evidenceDir` + spec ID, never from the spec's own location. So the relocation should not
trip any rule. Step 5 is what confirms that on real data rather than by inspection.

**Do not hand-move anything.** If a unit does not relocate, that is a defect in FTHR-083 to
fix there — not something to paper over with a manual `git mv` here. This feather's value is
precisely that it exercises the real command against real data.

**Do not commit the scratch baseline.** `.fledge/scratch/` is gitignored; the baseline is
working evidence, and belongs in `.fledge/molt/FTHR-086.md` as captured output, not in the
tree.

## Tests

**An honest note on the test-first cycle.** This feather builds no new behavior — it applies
FTHR-083's command to real data. There is therefore no test that fails against unchanged
code and passes after, because the code does not change. The test-first requirement is
satisfied **upstream**: FTHR-082 and FTHR-083 carry the tests that prove discovery,
allocation, eligibility, and relocation, and each observed a real FAIL→PASS. Manufacturing
a synthetic failing test here would be theatre, and a test that has only ever been seen
passing proves nothing.

What this feather has instead is **differential verification on real data**, which is
stronger than a fixture test for this particular risk:

Run: `fledge roost`, then `go test ./...` and `fledge preen`.

- *pre/post equivalence* — `vee`, `colony`, `unfledged`, and `ready` JSON captured before
  and after; identical modulo path fields. Fails loudly if any of ~95 renames changed what
  the tooling reports. → AC-2
- *active directories hold only live work* — after roosting, the plumage directory contains
  only the 6 non-fledged plumages, and the feather directory only the 8 non-fledged feathers
  plus the 3 deliberately-ineligible fledged stragglers. → AC-3
- *stragglers stayed for the right reason* — FTHR-072, FTHR-077, FTHR-080 are present at top
  level, and `fledge roost` reported each one's unit as skipped, naming the open plumage.
  Distinguishes "correctly ineligible" from "silently missed". → AC-4
- *suite and health check green* — `go test ./...` and `fledge preen` both pass against the
  migrated tree. → AC-5
- *renames, not rewrites* — `git show --stat` reports the commit as renames; no spec file's
  content is modified. → AC-6
- *idempotent on real data* — a second `fledge roost` relocates nothing and exits 0. → AC-7

Evidence for each goes in `.fledge/molt/FTHR-086.md`, including the captured before/after
output.

## Acceptance Criteria

- [ ] AC-1: The test-first cycle is satisfied upstream by FTHR-082 and FTHR-083, whose tests
      prove the mechanism this feather applies; this feather adds no new behavior and no new
      tests. The differential verification below was performed instead, with output captured
      as evidence.
- [ ] AC-2: `vee`, `colony`, `unfledged`, and `ready` produce output identical to their
      pre-migration baseline except for reported file paths. Satisfies PLM-032 AC-2.
- [ ] AC-3: The active spec directories contain only non-fledged work and the deliberately
      ineligible stragglers; every eligible unit's plumage and feathers are under `fledged/`.
      Satisfies PLM-032 FC-14, AC-12.
- [ ] AC-4: FTHR-072, FTHR-077, and FTHR-080 remain at top level, and the command reported
      their units as skipped, naming the open plumage in each case.
- [ ] AC-5: `go test ./...` and `fledge preen` both pass against the migrated tree.
      Satisfies PLM-032 AC-12.
- [ ] AC-6: The migration is a single commit that git reports as renames; no spec file's
      content changed.
- [ ] AC-7: A second `fledge roost` relocates nothing and exits 0.
- [ ] AC-8: The diff was reviewed and signed off by the user before merge (`oversight: merge`).
