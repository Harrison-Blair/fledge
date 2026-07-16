---
id: FTHR-047
title: Wire fledge colony into plumage closeout
plumage: PLM-024
status: fledged
priority: P2
depends_on: []
authored: 2026-07-16T01:57:23Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-047: Wire fledge colony into plumage closeout

## Description
`implementation.md`'s closeout step (solo §2 step 11, team §3 step 5)
currently gates on the orchestrator mentally tracking "if that was the last
unfinished feather of its plumage." `fledge colony --json` already reports
per-requirement (plumage) `done`/`total` feather counts
(`internal/cli/colony.go` `reqCompletion`), so this feather rewrites the
closeout step to query it instead of relying on in-context tracking.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` —
  solo closeout (~line 67) and team closeout (~line 109) (see
  `.fledge/nest/modules.md` → bootstrap/core skills).
- `internal/cli/colony.go` — `reqCompletion{ID, Title, Status, Done, Total}`,
  already emitted by `fledge colony --json`; no Go change needed here, only
  its consumption in prose.
- `cmd/fledge/testdata/` — whichever txtar fixture(s) assert on the scaffolded
  `implementation.md` content, to be updated alongside.

## Approach
Rewrite both closeout steps' opening clause from "if that was the last
unfinished feather of its plumage" to an explicit instruction: run
`fledge colony --json`, find the current plumage's entry, and treat it as
ready to close only when `done == total` for that entry — replacing the
mental-tracking language entirely. Keep the rest of each step (AC-by-AC
accounting, `confirm-gate`, `fledge criteria check`, `fledge status ...
fledged`) unchanged; this feather only changes how "is this the last
feather" is decided.

## Tests
- A `cmd/fledge` txtar test asserting the scaffolded `implementation.md`
  (post `fledge init`) instructs querying `fledge colony --json` for the
  plumage's `done`/`total` counts in both the solo and team closeout
  sections, and no longer contains the old "if that was the last unfinished
  feather" mental-tracking phrasing. Written first against the *current*
  scaffolded content and confirmed to FAIL (old phrasing still present, new
  instruction absent), then the prose is rewritten and `fledge init
  --refresh` regenerates the scaffold until the assertion passes (satisfies
  PLM-024 FC-1, AC-1).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-047.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-024 FC-2"). AC-1 is always:
- [x] AC-1: The test listed above was observed failing before implementation
  and passes after; evidence captured verbatim.
- [x] AC-2: `implementation.md`'s solo and team closeout steps both instruct
  querying `fledge colony --json` for the last-feather decision (satisfies
  PLM-024 FC-1, AC-1).
- [x] AC-3: `fledge init --refresh` regenerates this repo's scaffolded copy to
  match, and `go test ./cmd/fledge -run TestScripts` passes.
