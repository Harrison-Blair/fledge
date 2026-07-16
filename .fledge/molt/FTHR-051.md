# FTHR-051 evidence — Report worktree-exists and add --stale filter to fledge broods

Feather: `.fledge/pluma/feathers/FTHR-051-report-worktree-exists-and-add-stale-filter-to-fledge-broods.md`
Branch: `feather/FTHR-051-broods-stale-worktree-exists`
Test fixture: `cmd/fledge/testdata/broods_stale.txtar`

## AC-1
Tests observed FAILING before implementation, PASSING after; captured verbatim.

### Pre-implementation (FAIL) — `go test ./cmd/fledge -run TestScripts/broods_stale`

The txtar fixture was written first against the unchanged `runLocks` (no
`worktree_exists` field, no `--stale` flag). The `broods --json` output shows
records with only `pid_alive` and no `worktree_exists` key, so the first
`worktree_exists` assertion fails:

```
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/broods_stale (0.01s)
            FAIL: testdata/broods_stale.txtar:14: no match for `(?s)"feather": "FTHR-001".*?"worktree_exists": true` found in stdout
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.008s
FAIL
```

Relevant captured `broods --json` output at failure time (note the absence of
any `worktree_exists` field — only `pid_alive` is present):

```
[
  {
    "feather": "FTHR-001",
    "owner": "adelie",
    "pid": 1528806,
    "created": "2026-07-16T03:10:21Z",
    "branch": "",
    "worktree": "$WORK/livewt",
    "pid_alive": false
  },
  ...
  {
    "feather": "FTHR-005",
    "owner": "weddell",
    "pid": 1,
    "created": "2026-07-01T00:00:00Z",
    "branch": "main",
    "worktree": "",
    "pid_alive": true
  }
]
```

### Post-implementation (PASS) — `go test ./cmd/fledge -run TestScripts/broods_stale`

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.051s
```

Same fixture, now passing after adding `WorktreeExists` + `--stale` to
`runLocks`. The `broods --json` output now carries `worktree_exists` per
record (`true` for the existing dir `$WORK/livewt`, `false` for the missing
`$WORK/gonewt`, and `false` for the empty-field legacy FTHR-005):

```
[
  { "feather": "FTHR-001", ... "worktree": "$WORK/livewt", "pid_alive": false, "worktree_exists": true },
  { "feather": "FTHR-002", ... "worktree": "$WORK/gonewt", "pid_alive": false, "worktree_exists": false },
  { "feather": "FTHR-005", ... "worktree": "",             "pid_alive": true,  "worktree_exists": false }
]
```

## AC-2
`fledge broods --json` reports `worktree_exists` per record — `true` only when
the stored path exists on disk, `false` for a missing path or an empty (legacy)
one (PLM-025 FC-2).

Covered by these fixture assertions (`broods_stale.txtar`):
```
exec fledge broods --json
stdout '(?s)"feather": "FTHR-001".*?"worktree_exists": true'   # existing dir
stdout '(?s)"feather": "FTHR-002".*?"worktree_exists": false'  # nonexistent path
stdout '(?s)"feather": "FTHR-005".*?"worktree_exists": false'  # empty/legacy field
```
All three assertions pass (see the AC-1 post-implementation `--json` capture
above). Text output also gains a `(worktree gone)` annotation for the
missing/legacy records under a plain `fledge broods`:
```
FTHR-001  adelie  since ...  branch   (pid not alive)
FTHR-002  gentoo  since ...  branch   (pid not alive)  (worktree gone)
FTHR-005  weddell since ...  branch main  (worktree gone)
```
Implementation: `worktreeExists(path)` returns `false` for `path == ""`, else
`os.Stat(path)` succeeds AND `info.IsDir()` (`internal/cli/brood.go`).

## AC-3
`fledge broods --stale` filters to `worktree_exists: false` records in both text
and `--json` output; plain `fledge broods` is unchanged (PLM-025 FC-3).

Covered by these fixture assertions (`broods_stale.txtar`):
```
exec fledge broods --stale --json
! stdout '"feather": "FTHR-001"'   # live worktree excluded
stdout '"feather": "FTHR-002"'
stdout '"feather": "FTHR-005"'
exec fledge broods --stale
! stdout 'FTHR-001'
stdout 'FTHR-002'
stdout 'FTHR-005'
exec fledge broods          # plain: full set unchanged
stdout 'FTHR-001'
stdout 'FTHR-002'
stdout 'FTHR-005'
```
All pass. `--stale --json` returns only FTHR-002 and FTHR-005 (both
`worktree_exists: false`); FTHR-001 (live worktree) is excluded. `--stale`
text output likewise lists only FTHR-002 and FTHR-005; plain `fledge broods`
still lists all three. Per spec, the `(worktree gone)` annotation is suppressed
under `--stale` (redundant there) — captured `--stale` text output:
```
FTHR-002  gentoo  since ...  branch   (pid not alive)
FTHR-005  weddell since ...  branch main
```
Filter implemented in `runLocks`: `if *staleOnly && worktreeExists(rec.Worktree) { continue }`.

## AC-4
`go test ./internal/cli/... ./cmd/fledge -run TestScripts` passes.

```
$ go test ./internal/cli/... ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.001s [no tests to run]
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.107s
```

Full `go test ./...` also green (including the updated `lock.txtar`, whose two
legacy no-worktree seeded records now correctly show `(worktree gone)`), and
`gofmt -l internal/cli/brood.go` / `go vet ./...` clean.
```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.123s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.013s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.015s
...  (all packages ok)
```
