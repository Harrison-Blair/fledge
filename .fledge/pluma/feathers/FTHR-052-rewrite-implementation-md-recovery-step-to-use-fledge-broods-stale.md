---
id: FTHR-052
title: Rewrite implementation.md recovery step to use fledge broods --stale
plumage: PLM-025
status: fledged
priority: P2
depends_on: [FTHR-051]
authored: 2026-07-16T02:00:41Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-052: Rewrite implementation.md recovery step to use fledge broods --stale

## Description
`implementation.md` §6's recovery step currently reads: "Inventory reality:
`git worktree list`, feather branches, `fledge broods`... Locks whose feather
has no surviving worktree are stale — release them with `fledge abandon
FTHR-### --force`..." — a hand-correlation between two command outputs. With
`fledge broods --stale` now available (FTHR-051), this feather rewrites §6 to
use it directly for classification, and adds the legacy-record caveat the
user required during interrogation: a lock reported stale for lacking a
stored worktree path (as opposed to one whose path was checked and found
gone) should be re-verified against `git worktree list` before force-release,
since an empty path is classified stale by convention (FTHR-051), not
because its worktree was actually confirmed gone.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` — §6
  recovery, step 2 ("Inventory reality...") (see `.fledge/nest/modules.md` →
  bootstrap/core skills).
- `cmd/fledge/testdata/` — whichever txtar fixture(s) assert on the
  scaffolded `implementation.md` content.

## Approach
Rewrite §6 step 2 to: run `fledge broods --stale` to get the classification
directly; feathers with a held lock and `worktree_exists: true` are the
resume set; locks reported by `--stale` are release candidates — release with
`fledge abandon FTHR-### --force`, then set status explicitly (`fledge status
FTHR-### pipping --force`) as today. Add the caveat sentence: if a `--stale`
entry has an empty `worktree` field (a lock brooded before this repo adopted
`--worktree`, i.e. predates FTHR-050), re-check it against `git worktree
list` before force-releasing, since it was classified stale by convention
rather than by an actual path check. Keep `git worktree list` in the step as
the tool for that manual re-check case, even though it's no longer the
primary classification mechanism.

## Tests
- A `cmd/fledge` txtar test asserting the scaffolded `implementation.md` §6
  recovery step instructs running `fledge broods --stale` for classification
  and states the legacy-empty-worktree-path re-check caveat, and no longer
  presents `git worktree list` + `fledge broods` cross-referencing as the
  primary classification mechanism. Written first against the *current*
  scaffolded content (old hand-correlation language present, new instruction
  absent) and confirmed to FAIL, then the prose is rewritten and `fledge init
  --refresh` regenerates the scaffold until the assertion passes (satisfies
  PLM-025 FC-4, AC-4).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-052.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-025 FC-2"). AC-1 is always:
- [x] AC-1: The test listed above was observed failing before implementation
  and passes after; evidence captured verbatim.
- [x] AC-2: `implementation.md`'s §6 recovery step instructs `fledge broods
  --stale` for classification and states the legacy-empty-path re-check
  caveat (satisfies PLM-025 FC-4, AC-4).
- [x] AC-3: `fledge init --refresh` regenerates this repo's scaffolded copy to
  match, and `go test ./cmd/fledge -run TestScripts` passes.
