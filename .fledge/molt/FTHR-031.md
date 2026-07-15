# FTHR-031 evidence

## AC-1

Test-first: `TestConcurrentAllocationYieldsDistinctIDs` in
`internal/spec/ids_test.go` launches 20 goroutines through a start barrier,
all calling `AllocateAndCreate(dir, "FTHR", ...)` against one shared temp
dir, over 5 rounds. To capture the pre-fix failure, `AllocateAndCreate` was
temporarily reverted to the unlocked sequence (`NextID` then `O_EXCL` create,
no flock — i.e. the exact race `fledge new` had before this feather) and the
test run against it:

```
$ go test ./internal/spec -run TestConcurrentAllocationYieldsDistinctIDs -v
=== RUN   TestConcurrentAllocationYieldsDistinctIDs
    ids_test.go:98: round 0: id FTHR-011 allocated 4 times, want distinct IDs (ids: [FTHR-001 FTHR-004 FTHR-011 FTHR-007 FTHR-002 FTHR-008 FTHR-005 FTHR-004 FTHR-003 FTHR-006 FTHR-011 FTHR-011 FTHR-006 FTHR-010 FTHR-009 FTHR-010 FTHR-010 FTHR-006 FTHR-011 FTHR-012])
--- FAIL: TestConcurrentAllocationYieldsDistinctIDs (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/spec	0.002s
```

This fails for the expected reason: without serialization, multiple
goroutines scan the same `NextID` state before any of them has created its
file, so several land on the same ID (`FTHR-011` allocated 4 times here).

After restoring the flock (`lockAllocDir` wraps `NextID` + the `O_EXCL`
create in `AllocateAndCreate`), the same test passes:

```
$ go test ./internal/spec -run TestConcurrentAllocationYieldsDistinctIDs -v
=== RUN   TestConcurrentAllocationYieldsDistinctIDs
--- PASS: TestConcurrentAllocationYieldsDistinctIDs (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.004s

$ go test ./internal/spec -run TestConcurrentAllocationYieldsDistinctIDs -race -v
=== RUN   TestConcurrentAllocationYieldsDistinctIDs
--- PASS: TestConcurrentAllocationYieldsDistinctIDs (0.01s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/spec	1.019s
```

## AC-2

`internal/spec/ids.go`: `AllocateAndCreate(dir, prefix, build)` acquires an
exclusive `flock` (`lockAllocDir`, `syscall.Flock(..., LOCK_EX)`) on
`<dir>/.alloc.lock` before calling `NextID` and creating the file
(`O_CREATE|O_EXCL`), and releases it (`LOCK_UN` + close) via `defer` on every
return path, including errors. `internal/cli/new.go` now routes both the
`plumage` (`r.RequirementsDir()`) and `feather` (`r.TasksDir()`) creation
paths through `AllocateAndCreate`, each with its own directory and therefore
its own `.alloc.lock` — a plumage allocation and a feather allocation lock
independently and never block each other (verified: `.alloc.lock` files are
created separately under `.fledge/pluma/plumage/` and
`.fledge/pluma/feathers/` in the scratch-repo run below, and the concurrency
test only exercises one dir's lock at a time — the two locks are simply
distinct `flock`s on distinct files, so they cannot contend by construction).

The concurrency test above (20 goroutines x 5 rounds, plus `-race`) confirms
no two allocations in the same dir ever share an ID once the flock is in
place.

## AC-3

`new.go:123`'s old comment (`// O_EXCL: a concurrent allocation of the same
ID fails loudly.`) described a guarantee that wasn't true (two different
titles for the same ID never collided on the O_EXCL path). It has been
removed from `new.go` — `O_EXCL` create is now just the inner step of
`spec.AllocateAndCreate`, which is documented at its declaration in
`internal/spec/ids.go`:

```go
// AllocateAndCreate serializes NextID and the O_EXCL file create it guards
// behind an exclusive flock on <dir>/.alloc.lock, so two processes racing to
// allocate an ID in the same dir never both win: the loser blocks on the
// flock until the winner has created its file and released the lock, then
// sees the winner's file in its own NextID scan. build receives the
// allocated id and returns the file path and content to create. Separate
// dirs (e.g. plumages vs. feathers) use separate lock files and never block
// each other.
```

`fledge preen` with `.alloc.lock` dotfiles present, run against a scratch
repo created via `fledge new plumage` / `fledge new feather` (which leaves
`.alloc.lock` in both `.fledge/pluma/plumage/` and
`.fledge/pluma/feathers/`):

```
$ fledge new plumage --title "Scratch plumage" --json
{
  "id": "PLM-001",
  "path": ".fledge/pluma/plumage/PLM-001-scratch-plumage.md"
}
$ fledge status PLM-001 hatched
$ fledge new feather --title "Scratch feather" --plumage PLM-001 --json
{
  "id": "FTHR-001",
  "path": ".fledge/pluma/feathers/FTHR-001-scratch-feather.md"
}
$ ls .fledge/pluma/feathers .fledge/pluma/plumage
.fledge/pluma/feathers:
.alloc.lock  FTHR-001-scratch-feather.md  .gitkeep

.fledge/pluma/plumage:
.alloc.lock  .gitkeep  PLM-001-scratch-plumage.md

$ fledge preen
spec clean: 1 plumages, 1 feathers
$ echo "preen exit: $?"
preen exit: 0
```

`.alloc.lock` is not matched by `NextID`'s `^PREFIX-(\d+)[-.]` regexp and has
no `.md` suffix, so `preen`, spec loading, and `NextID` all ignore it —
`preen` reports "spec clean" with it present.

## AC-4

`fledge preen` in this worktree (pre-existing warnings are about brood
locks / scaffold drift unrelated to this feather; exit code 0, no errors):

```
$ fledge preen
WARN  .fledge/pluma/feathers/FTHR-028-...md: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-031-...md: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-032-...md: status hatching but no brood is held
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
5 warning(s)
exit=0
```

`go test ./...`:

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap
ok  	github.com/Harrison-Blair/fledge/internal/check
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig
ok  	github.com/Harrison-Blair/fledge/internal/cli
ok  	github.com/Harrison-Blair/fledge/internal/graph
ok  	github.com/Harrison-Blair/fledge/internal/hooktest
ok  	github.com/Harrison-Blair/fledge/internal/lock
ok  	github.com/Harrison-Blair/fledge/internal/nest
ok  	github.com/Harrison-Blair/fledge/internal/repo
ok  	github.com/Harrison-Blair/fledge/internal/scan
ok  	github.com/Harrison-Blair/fledge/internal/spec
```

`go vet ./...`: clean (no output).
