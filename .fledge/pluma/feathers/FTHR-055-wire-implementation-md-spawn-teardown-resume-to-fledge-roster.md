---
id: FTHR-055
title: Wire implementation.md spawn/teardown/resume to fledge roster
plumage: PLM-026
status: egg
priority: P3
depends_on: [FTHR-054]
authored: 2026-07-16T02:06:38Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-055: Wire implementation.md spawn/teardown/resume to fledge roster

## Description
With `fledge roster` fully implemented (FTHR-053/054), this feather rewrites
the three places `implementation.md` currently handles species
allocation/release/reconstruction as orchestrator-context bookkeeping, so
each becomes a CLI call instead:
- §3.1 (pair spawn / naming scheme): replace the prose allocation rule
  ("assign the first unused species... append a numeric suffix...") and the
  unenumerated "18 extant penguin species" assertion with an instruction to
  run `fledge roster assign --feather FTHR-### --pair` (or without `--pair`
  for a solo spawn like forager/incubator), referencing `internal/roster`'s
  species list by name instead of asserting a bare count. Drop "keep the
  full name→feather mapping internally" — the roster file is now that
  record.
- §3.5 (teardown): replace "the pair's species frees only once both members
  are confirmed shut down" (an in-context tracking duty) with an instruction
  to run `fledge roster release <name>` for each confirmed-shutdown member.
- §6 (crash/resume): replace "treat all remembered workers as gone; clear
  the roster... reconstruct from worktrees/branches/broods" with an
  instruction to run `fledge roster` to read the current name→feather
  assignments directly.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` —
  §3.1 (~line 92-96), §3.5 (~line 106), §6 (~line 132) (see
  `.fledge/nest/modules.md` → bootstrap/core skills).
- `cmd/fledge/testdata/` — whichever txtar fixture(s) assert on the
  scaffolded `implementation.md` content.

## Approach
Rewrite each of the three sections per the Description above, preserving
every other instruction in them unchanged (e.g. §3.1's "one species per
worker pair," §3.5's "force-terminate a straggler that doesn't exit
promptly," §6's "respawn a fresh pair into the existing worktree"). The only
change in each is replacing in-context bookkeeping with the corresponding
`fledge roster` subcommand call.

## Tests
- A `cmd/fledge` txtar test asserting the scaffolded `implementation.md`:
  §3.1 instructs `fledge roster assign` (and references the species list by
  package name, not an unenumerated "18 species" assertion), §3.5 instructs
  `fledge roster release <name>`, and §6 instructs `fledge roster` for
  resume reconstruction instead of "clear the roster." Written first against
  the *current* scaffolded content (old bookkeeping language present, new
  instructions absent) and confirmed to FAIL, then the prose is rewritten
  and `fledge init --refresh` regenerates the scaffold until the assertion
  passes (satisfies PLM-026 FC-5, AC-5).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-055.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-026 FC-2"). AC-1 is always:
- [ ] AC-1: The test listed above was observed failing before implementation
  and passes after; evidence captured verbatim.
- [ ] AC-2: `implementation.md`'s §3.1, §3.5, and §6 are rewritten per FC-5
  (satisfies PLM-026 FC-5, AC-5).
- [ ] AC-3: `fledge init --refresh` regenerates this repo's scaffolded copy to
  match, and `go test ./cmd/fledge -run TestScripts` passes.
- [ ] AC-4: `go test ./...` passes and `fledge preen` reports the scaffold
  healthy after the refresh (satisfies PLM-026 AC-6).
