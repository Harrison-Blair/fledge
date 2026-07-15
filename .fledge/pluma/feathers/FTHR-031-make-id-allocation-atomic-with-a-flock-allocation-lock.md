---
id: FTHR-031
title: Make ID allocation atomic with a flock allocation lock
plumage: PLM-016
status: pipping
priority: P1
depends_on: []
authored: 2026-07-15T15:13:39Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-031: Make ID allocation atomic with a flock allocation lock

## Description
Close the ID-allocation race: today `fledge new` computes the next ID by scanning
filenames (`spec.NextID`) and then creates the file with `O_EXCL` on the *full*
`<ID>-<title>.md` path, so the exclusivity guard only fires when the ID *and* the title
collide. Two concurrent creations with different titles allocate the same ID and both
succeed. Serialize the allocate-then-create critical section with an exclusive `flock` so
the scan and the create are atomic across fledge processes.

Satisfies PLM-016 FC-1.

## Affected Modules
- `internal/cli/new.go` — the allocation + `O_EXCL` create block (lines ~105–134 for
  feathers, ~60–80 for plumages) and the now-inaccurate comment at `new.go:123`
  ("a concurrent allocation of the same ID fails loudly"). See
  `.fledge/nest/modules.md → internal/cli`.
- `internal/spec/ids.go` — `NextID` (the scan). The critical section wraps `NextID` +
  create; `NextID` itself is unchanged.
- Possibly a small helper (in `internal/spec` or `internal/cli`) that acquires/releases the
  allocation lock, so both the plumage and feather creation paths share it.

## Approach
1. Introduce an exclusive advisory file lock (`syscall.Flock` with `LOCK_EX`, released via
   `LOCK_UN`/close) held around the whole `NextID` → `O_EXCL` create sequence. fledge is
   Unix-only (it already uses `syscall.Kill` for brood pid-liveness), so `flock` is
   available and consistent with the platform stance.
2. Use a dedicated hidden lock file per allocation directory (e.g. `<dir>/.alloc.lock`,
   created if absent) — one for `TasksDir()` and one for `RequirementsDir()` so plumage and
   feather creation don't serialize against each other. The lock file is a dotfile: it does
   not match the `PLM/FTHR-###` ID regex in `NextID` and is not a `*.md` spec, so scanners,
   loaders, and `preen` ignore it. Confirm `preen` stays clean with it present.
3. Acquire the lock, `NextID`, build the spec, `O_EXCL`-create and write the file, then
   release the lock. On any error the lock is released (defer). Keep the `O_EXCL` create as
   a secondary guard.
4. Fix the misleading comment at `new.go:123` to describe the real guarantee (the flock),
   not the filename `O_EXCL`.

Constraints: serialize only the allocate+create section, not the whole command; do not
change the ID format or `NextID`'s scanning logic. No new third-party dependency —
`golang.org/x/sys` is unnecessary, stdlib `syscall.Flock` suffices.

## Tests
Written test-first, failing before the fix, passing after:
- `TestConcurrentAllocationYieldsDistinctIDs` (in `internal/cli` or `internal/spec`, at the
  allocate+create function level) — launches N goroutines that all allocate+create against
  one temp dir, released simultaneously via a start barrier (`sync.WaitGroup`) to force
  contention, then asserts every created file has a distinct ID and N files exist. Against
  the pre-fix code this reliably produces a duplicate ID (two files sharing `<ID>`); with
  the flock it always yields N distinct IDs. If reproducing pre-fix needs iteration, loop
  the barrier a handful of times so the failure is deterministic.

## Acceptance Criteria
- [ ] AC-1: The test above was observed failing before implementation (duplicate ID under concurrency) and passes after.
- [ ] AC-2: Concurrent allocate-then-create is serialized by an exclusive flock so no two specs share an ID (FC-1); plumage and feather allocation use separate locks and do not block each other.
- [ ] AC-3: The `new.go:123` comment accurately describes the flock guarantee; `fledge preen` passes with the `.alloc.lock` dotfile present.
- [ ] AC-4: `fledge preen` passes and `go test ./...` is green.
