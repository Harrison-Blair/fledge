# FTHR-050 evidence — Store worktree path in lock Record at brood time

## AC-1
The tests listed in the feather's Tests section were written first and observed
FAILING against the unchanged `Record`/`runLock` (no `Worktree` field, no
`--worktree` flag), then pass after implementation.

### Pre-implementation FAILING run (verbatim)

Command: `go test ./internal/lock/...`

```
# github.com/Harrison-Blair/fledge/internal/lock [github.com/Harrison-Blair/fledge/internal/lock.test]
internal/lock/lock_test.go:45:5: in.Worktree undefined (type Record has no field or method Worktree)
internal/lock/lock_test.go:53:9: got.Worktree undefined (type *Record has no field or method Worktree)
internal/lock/lock_test.go:54:60: got.Worktree undefined (type *Record has no field or method Worktree)
internal/lock/lock_test.go:60:31: recs[0].Worktree undefined (type Record has no field or method Worktree)
internal/lock/lock_test.go:78:9: got.Worktree undefined (type *Record has no field or method Worktree)
internal/lock/lock_test.go:79:59: got.Worktree undefined (type *Record has no field or method Worktree)
FAIL	github.com/Harrison-Blair/fledge/internal/lock [build failed]
FAIL
```

Command: `go test ./cmd/fledge -run 'TestScripts/lock'`

```
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/lock (0.06s)
        ...
            # brood --json emits the claim record shape (0.004s)
            # --worktree omitted: worktree is empty (0.000s)
            > stdout '"worktree": ""'
            FAIL: testdata/lock.txtar:82: no match for `"worktree": ""` found in stdout

FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.061s
FAIL
```

Both failures are for the expected reason: the `Worktree` field and
`--worktree` flag do not yet exist.

### Post-implementation PASSING run (verbatim)

Command: `go test ./internal/lock/... -run 'Worktree' -count=1 -v`

```
=== RUN   TestWorktreeRoundTrip
--- PASS: TestWorktreeRoundTrip (0.00s)
=== RUN   TestWorktreeBackwardCompat
--- PASS: TestWorktreeBackwardCompat (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.001s
```

Command: `go test ./cmd/fledge -run 'TestScripts/lock' -count=1`

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.049s
```

The lock.txtar additions assert `"worktree": ""` when `--worktree` is omitted
and `"worktree": "/some/path"` when passed.

## AC-2
`lock.Record` has a `Worktree` field, populated by `fledge brood --worktree
<path>`, defaulting to empty when omitted (PLM-025 FC-1, AC-1).

- `internal/lock/lock.go`: `Record` now has `Worktree string \`json:"worktree"\``.
- `internal/cli/brood.go`: `runLock` adds a `--worktree` string flag
  (default `""`) and sets `rec.Worktree = *worktree` when building the record;
  usage string updated to `[--worktree <path>]`.
- Round-trip proven by `TestWorktreeRoundTrip` (non-empty value survives
  Acquire/Get/List) and by lock.txtar (`--worktree /some/path` →
  `"worktree": "/some/path"`; omitted → `"worktree": ""`).
- Empty-default / backward-compat proven by `TestWorktreeBackwardCompat` (a
  legacy `.brood` JSON with no `worktree` key unmarshals with `Worktree == ""`).

## AC-3
`implementation.md` §3.1 step 5 claim instruction passes `--worktree <path>`;
this repo's scaffolded copy refreshed to match.

- Embedded source
  `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` §3.1
  step 5 now reads: `fledge brood FTHR-### --owner <worker-name> --branch
  feather/FTHR-###-<kebab> --worktree <path>` (the same worktree path created
  in step 2).
- `fledge init --refresh` regenerated the scaffolded copy
  `.fledge/skills/fledge-orchestrate/implementation.md` (line 87 shows the
  `--worktree <path>` change) and updated `.fledge/scaffold.json`.
- `fledge preen` after refresh:

```
spec clean: 26 plumages, 56 feathers
```

## AC-4
`go test ./internal/lock/... ./internal/cli/...` and `go test ./cmd/fledge -run
TestScripts` pass; full `go test ./...` green; `gofmt -l .` and `go vet ./...`
clean.

```
ok  	github.com/Harrison-Blair/fledge/internal/lock	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/cli	(cached)
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.111s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.010s
... (all packages ok) ...
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s
```

`gofmt -l .` → empty (clean); `go vet ./...` → no output (clean).
