---
id: FTHR-049
title: Gitignore .alloc.lock in scaffolded .gitignore
plumage: PLM-024
status: fledged
priority: P2
depends_on: []
authored: 2026-07-16T01:57:23Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-049: Gitignore .alloc.lock in scaffolded .gitignore

## Description
`.alloc.lock` (created by the ID allocator on every `fledge new` allocation,
never removed by design — it's a flock target, `internal/spec/ids.go:96`) is
not gitignored. `internal/cli/init.go`'s `gitignoreLines` slice already lists
`.fledge/nest/raw/` and `.fledge/broods/` as regenerable/machine-local
intermediates; this feather adds `.alloc.lock` to the same list, so both this
repo (after a refresh) and every future `fledge init`/`fledge init --refresh`
target ignore it.

## Affected Modules
- `internal/cli/init.go` — `gitignoreLines` (line 30) and its two consumers
  (line 398 `Lines: gitignoreLines`, line 471 the write/verify loop) (see
  `.fledge/nest/modules.md` → internal/cli).
- Repo `.gitignore` — regenerated via `fledge init --refresh`.

## Approach
Add `".alloc.lock"` to the `gitignoreLines` slice in `internal/cli/init.go`.
Since `NextID`'s allocation directories can be nested (e.g. `.fledge/pluma/plumage/.alloc.lock`,
`.fledge/pluma/feathers/.alloc.lock`), confirm the existing gitignore-writing
mechanism (line ~398/471) treats each line as a pattern matched at any depth
(as `.fledge/broods/` presumably already is, being under `.fledge/`) — if the
current lines are written as repo-root-relative exact paths rather than
patterns, use `**/.alloc.lock` instead of a bare `.alloc.lock` so it matches
under both `.fledge/pluma/plumage/` and `.fledge/pluma/feathers/`. Verify by
running `git check-ignore` against both existing untracked `.alloc.lock`
files in this repo after regeneration.

## Tests
- A `cmd/fledge` txtar test (extending an existing init/gitignore fixture)
  asserts the generated `.gitignore` contains an entry matching
  `.alloc.lock` after `fledge init`. Written first against the current
  behavior and confirmed to FAIL (entry absent), then the line is added and
  the test passes (satisfies PLM-024 FC-3, AC-3).
- A unit or txtar check that `.alloc.lock` files at both
  `.fledge/pluma/plumage/.alloc.lock` and `.fledge/pluma/feathers/.alloc.lock`
  depths are ignored (not just a root-level one), confirming the pattern
  choice from Approach is correct.

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-049.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-024 FC-2"). AC-1 is always:
- [x] AC-1: The tests listed above were observed failing before implementation
  and pass after; evidence captured verbatim.
- [x] AC-2: `internal/cli/init.go`'s `gitignoreLines` includes an entry that
  matches `.alloc.lock` at any allocation-directory depth (satisfies PLM-024
  FC-3, AC-3).
- [x] AC-3: This repo's own `.gitignore` is refreshed via `fledge init
  --refresh`, and `git check-ignore .fledge/pluma/plumage/.alloc.lock
  .fledge/pluma/feathers/.alloc.lock` confirms both are ignored (satisfies
  PLM-024 AC-3).
- [x] AC-4: `go test ./cmd/fledge -run TestScripts` passes.
