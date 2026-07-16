---
id: FTHR-050
title: Store worktree path in lock Record at brood time
plumage: PLM-025
status: fledged
priority: P2
depends_on: []
authored: 2026-07-16T02:00:37Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-050: Store worktree path in lock Record at brood time

## Description
`lock.Record` (`internal/lock/lock.go`) stores `{Task, Owner, PID, Created,
Branch}` — no worktree path — so nothing downstream can answer "does this
claim's worktree still exist." `fledge brood` is run on main, after the
worktree already exists (implementation.md step 2 creates it, step 5 claims
the brood), so the path can't be auto-derived the way `--branch` derives from
`git rev-parse --abbrev-ref HEAD` — it must be passed explicitly. This
feather adds a `Worktree` field to `Record` and a `--worktree <path>` flag to
`fledge brood` that populates it; omitting the flag leaves it empty (the
convention this plumage's PLM-025 settled for legacy/pre-change records).

## Affected Modules
- `internal/lock/lock.go` — `Record` struct (line ~14-20) (see
  `.fledge/nest/modules.md` → internal/lock).
- `internal/cli/brood.go` — `runLock` (the `fledge brood` command), which
  builds the `lock.Record` (line ~74-77) (see `.fledge/nest/modules.md` →
  internal/cli).
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` —
  §3.1 step 5, the `fledge brood FTHR-### --owner <worker-name> --branch
  feather/FTHR-###-<kebab>` claim instruction, so the workflow actually
  passes `--worktree <path>` at claim time (otherwise the flag exists but is
  never invoked, and the field never populates in practice).

## Approach
Add `Worktree string \`json:"worktree"\`` to `lock.Record`. Add a
`--worktree` string flag to `runLock` in `brood.go` (parallel to the existing
`--branch` flag), defaulting to `""` when omitted, and set `rec.Worktree =
*worktree` when building the record. No validation that the path exists at
claim time (a legitimate worktree can be relative to repo root or absolute;
existence is FTHR-051's concern, not this feather's) — just plumbing the
value through. Also update `implementation.md`'s §3.1 step 5 claim
instruction to pass `--worktree <path>` (the same path created in step 2),
so the flag is actually exercised by the workflow going forward.

## Tests
- A unit test in `internal/lock` round-trips a `Record` with a non-empty
  `Worktree` through `Acquire`/`Get`/`List`, confirming the field survives
  JSON marshal/unmarshal.
- A unit test confirms a `Record` written before this change (JSON lacking a
  `worktree` key) still unmarshals cleanly with `Worktree == ""` (backward
  compatibility with existing on-disk `.brood` files).
- A `cmd/fledge` txtar test exercises `fledge brood FTHR-### --owner X
  --worktree /some/path --json` and asserts the JSON output includes
  `"worktree":"/some/path"`; a call omitting `--worktree` asserts
  `"worktree":""`.
Written first against the unchanged `Record`/`runLock` (no `Worktree` field,
no `--worktree` flag) and confirmed to FAIL (compile error / unknown flag),
then implemented until they pass (satisfies PLM-025 FC-1, AC-1).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-050.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-025 FC-2"). AC-1 is always:
- [x] AC-1: The tests listed above were observed failing before implementation
  and pass after; evidence captured verbatim.
- [x] AC-2: `lock.Record` has a `Worktree` field, populated by `fledge brood
  --worktree <path>` at claim time, defaulting to empty when omitted
  (satisfies PLM-025 FC-1, AC-1).
- [x] AC-3: `implementation.md`'s §3.1 step 5 claim instruction passes
  `--worktree <path>`; this repo's scaffolded copy is refreshed to match.
- [x] AC-4: `go test ./internal/lock/... ./internal/cli/...` passes and
  `go test ./cmd/fledge -run TestScripts` passes.
