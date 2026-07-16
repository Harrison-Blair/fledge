---
id: FTHR-051
title: Report worktree-exists and add --stale filter to fledge broods
plumage: PLM-025
status: fledged
priority: P2
depends_on: [FTHR-050]
authored: 2026-07-16T02:00:39Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-051: Report worktree-exists and add --stale filter to fledge broods

## Description
With `lock.Record.Worktree` now populated at claim time (FTHR-050),
`fledge broods` can report whether each lock's worktree still exists instead
of leaving that cross-reference to the orchestrator hand-correlating against
`git worktree list`. This feather adds a computed `worktree_exists` boolean
per record to `broods --json`/text output, and a `--stale` flag that filters
to records where it's `false`. Per this plumage's settled design: `broods`
stays read-only (no release side effect), and a record with an empty
`Worktree` (legacy, pre-FTHR-050) reports `worktree_exists: false` — the
simpler two-state classification the user chose over a third "unknown"
state.

## Affected Modules
- `internal/cli/brood.go` — `runLocks` (the `fledge broods` command, line
  ~155-197), specifically the `lockOut` struct and the per-record loop (see
  `.fledge/nest/modules.md` → internal/cli).

## Approach
Add `WorktreeExists bool \`json:"worktree_exists"\`` to the existing
`lockOut` struct (alongside `PIDAlive`). Compute it per record: `false` if
`rec.Worktree == ""`; otherwise `true` iff `os.Stat(rec.Worktree)` succeeds
and reports a directory (mirrors `pidAlive`'s "informational, no git-registry
cross-check" spirit — a plain existence stat is sufficient and matches this
plumage's Out of Scope, which doesn't require confirming git itself still
recognizes it as a registered worktree). Add a `--stale` bool flag to
`runLocks`'s `flag.FlagSet`; when set, filter the `out` slice (both the
`--json` and text-output list) to entries where `WorktreeExists == false`
before printing. Text output gains a `(worktree gone)` annotation analogous
to the existing `(pid not alive)` one when relevant and `--stale` isn't set
(so plain `fledge broods` still surfaces both signals at a glance).

## Tests
- A `cmd/fledge` txtar test: brood a feather with `--worktree <path>` pointed
  at a directory that exists → `fledge broods --json` reports
  `worktree_exists: true`; brood another with `--worktree <path>` pointed at
  a nonexistent path → reports `false`; a legacy-style record with no
  `--worktree` (empty field) → reports `false`.
- A `cmd/fledge` txtar test: `fledge broods --stale` (mixed set of live and
  gone-worktree records) returns only the gone-worktree ones, in both text
  and `--json` output; plain `fledge broods` (no flag) still returns the full
  set.
Written first against the unchanged `runLocks` (no `worktree_exists` field,
no `--stale` flag) and confirmed to FAIL, then implemented until they pass
(satisfies PLM-025 FC-2 and FC-3, AC-2 and AC-3).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-051.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-025 FC-2"). AC-1 is always:
- [x] AC-1: The tests listed above were observed failing before implementation
  and pass after; evidence captured verbatim.
- [x] AC-2: `fledge broods --json` reports `worktree_exists` per record,
  `true` only when the stored path exists on disk, `false` for a missing
  path or an empty (legacy) one (satisfies PLM-025 FC-2, AC-2).
- [x] AC-3: `fledge broods --stale` filters to `worktree_exists: false`
  records in both text and `--json` output, and plain `fledge broods` is
  unchanged (satisfies PLM-025 FC-3, AC-3).
- [x] AC-4: `go test ./internal/cli/... ./cmd/fledge -run TestScripts` passes.
