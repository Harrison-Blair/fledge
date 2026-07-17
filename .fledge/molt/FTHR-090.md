# FTHR-090 evidence

## AC-1

Test-first order: (1) extended `cmd/fledge/testdata/broods_stale.txtar` with
two behavioral assertions (`! stdout 'pid_alive'` on `broods --json`, `!
stdout 'pid not alive'` on `broods`) and added
`internal/bootstrap/pid_liveness_test.go` (`TestCoreSkillsNoPIDLiveness`,
walking `core/skills` for the substring `pid-alive`) — both against the
**unchanged** `internal/cli/brood.go`,
`internal/bootstrap/core/skills/fledge-orchestrate/implementation.md`, and
`internal/lock/lock.go`. (2) Ran them and captured the FAIL output verbatim
below. (3) Only then implemented the deletions.

### Guard test, pre-implementation (FAIL for the expected reason: `implementation.md` still contains the pid-alive clause)

```
$ go test ./internal/bootstrap -run TestCoreSkillsNoPIDLiveness -v
=== RUN   TestCoreSkillsNoPIDLiveness
    pid_liveness_test.go:23: core/skills/fledge-orchestrate/implementation.md: references pid-alive liveness prose, which FTHR-090 removed
--- FAIL: TestCoreSkillsNoPIDLiveness (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

### txtar, pre-implementation (FAIL for the expected reason — behavioral, not a build break: `broods --json` still emits `"pid_alive": false` on every record, and `broods` still prints `(pid not alive)` for a claim seconds old)

```
$ go test ./cmd/fledge -run TestScripts/broods_stale -v
    ...
    # FTHR-090 AC-4: no PID reporting anywhere in broods output (0.001s)
    > exec fledge broods --json
    [stdout]
    [
      {
        "feather": "FTHR-001",
        "owner": "adelie",
        "pid": 394988,
        "created": "2026-07-17T08:20:46Z",
        "branch": "",
        "worktree": "$WORK/livewt",
        "pid_alive": false,
        "worktree_exists": true
      },
      {
        "feather": "FTHR-002",
        "owner": "gentoo",
        "pid": 394997,
        "created": "2026-07-17T08:20:46Z",
        "branch": "",
        "worktree": "$WORK/gonewt",
        "pid_alive": false,
        "worktree_exists": false
      },
      {
        "feather": "FTHR-005",
        "owner": "weddell",
        "pid": 1,
        "created": "2026-07-01T00:00:00Z",
        "branch": "main",
        "worktree": "",
        "pid_alive": true,
        "worktree_exists": false
      }
    ]
    > ! stdout 'pid_alive'
    FAIL: testdata/broods_stale.txtar:44: unexpected match for `pid_alive` found in stdout: pid_alive

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/broods_stale (0.01s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.014s
FAIL
```

Note the absurdity PLM-035's Context calls out directly in the captured
output: `FTHR-001` and `FTHR-002` are claims **seconds old** (created in the
same test run) and both are annotated `pid_alive: false` — the CLI's own PID
from the `fledge brood` invocation is already dead by the time `fledge
broods` reads it back, on the very first `--json` read.

## AC-2

Both failures above are behavioral, not compile-arity: the guard test is a
runtime substring-match failure against embedded doc content, and the txtar
failure is `unexpected match for 'pid_alive' found in stdout` — an assertion
against the built binary's actual output, not a build break. This satisfies
PLM-035 AC-2.

### Post-implementation: both tests pass

```
$ go test ./internal/bootstrap -run TestCoreSkillsNoPIDLiveness -v
=== RUN   TestCoreSkillsNoPIDLiveness
--- PASS: TestCoreSkillsNoPIDLiveness (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s

$ go test ./cmd/fledge -run 'TestScripts/(broods_stale|lock)' -v
...
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/broods_stale (0.01s)
    --- PASS: TestScripts/lock (0.05s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.049s
```

## AC-3

`internal/lock/lock.go`: `Record.PID` deleted from the struct (no other
consumer in the package — confirmed by `grep`). `internal/cli/brood.go`'s
`runLock` no longer populates `PID: os.Getpid()` in the constructed `lock.Record`.
`internal/lock/lock_test.go`'s `rec()` helper updated to drop `PID: 1`
(the field no longer exists; `go build`/`go test ./internal/lock` compiles
and passes, see above).

## AC-4

`internal/cli/brood.go`'s `lockOut` struct no longer embeds `PIDAlive bool
\`json:"pid_alive"\`` — only `WorktreeExists` remains alongside the embedded
`lock.Record`. The per-record loop no longer calls `pidAlive(rec.PID)`, and
the text-output loop no longer appends the `(pid not alive)` annotation.
Proven by the txtar assertions added in AC-1/AC-2
(`cmd/fledge/testdata/broods_stale.txtar` lines 43-46,
`cmd/fledge/testdata/lock.txtar`'s `! stdout 'pid_alive'` / `! stdout 'pid not
alive'` assertions), all passing post-implementation (see above). This closes
the feather-claim half of PLM-035 AC-4; FTHR-089 closes the status-record
half.

## AC-5

`cmd/fledge/testdata/lock.txtar` and `broods_stale.txtar`'s existing
`worktree_exists`/`--stale` assertions were **not modified** — only the
PID-related lines in `lock.txtar` were changed (stale-PID-detection comment
and assertions replaced with PID-absence assertions; the `(worktree gone)`
assertions on FTHR-005/FTHR-006 are untouched in substance, still expecting
the same annotation). `broods_stale.txtar`'s AC-2/AC-3 `worktree_exists` and
`--stale` blocks (lines 1-40) are byte-for-byte unmodified from the
pre-implementation version. Both pass (see above), and `worktreeExists` /
the `--worktree` field in `internal/cli/brood.go` were not touched by this
change — confirmed by `git diff` showing no edits to that function.

## AC-6

`internal/bootstrap/pid_liveness_test.go`'s `TestCoreSkillsNoPIDLiveness`
walks every file under `core/skills` for the substring `pid-alive` and
fails if found. It failed pre-implementation (AC-1) and passes now that
`implementation.md`'s clause is removed (see above).

## AC-7

`git diff internal/bootstrap/core/skills/fledge-orchestrate/implementation.md`
shows exactly one changed line (135): `pid-alive per held lock` removed from
the parenthetical, `owner, branch, pid-alive per held lock` →
`owner, branch per held lock`. No other line in the file touched.

## AC-8

`grep -rn "PIDAlive\|pidAlive" internal/cli/` returns no matches — the
helper and its call sites are gone. The `syscall` import (used only by
`pidAlive`) was removed from `internal/cli/brood.go`; `errors` is retained
because `errors.As(err, &held)` in `runLock` still uses it. `go vet ./...`
and `gofmt -l .` are both clean (see below), which would catch an unused
import.

## AC-9

```
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap
ok  	github.com/Harrison-Blair/fledge/internal/check
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig
ok  	github.com/Harrison-Blair/fledge/internal/cli
ok  	github.com/Harrison-Blair/fledge/internal/doctest
ok  	github.com/Harrison-Blair/fledge/internal/graph
ok  	github.com/Harrison-Blair/fledge/internal/hooktest
ok  	github.com/Harrison-Blair/fledge/internal/ledger
ok  	github.com/Harrison-Blair/fledge/internal/lock
ok  	github.com/Harrison-Blair/fledge/internal/nest
ok  	github.com/Harrison-Blair/fledge/internal/repo
ok  	github.com/Harrison-Blair/fledge/internal/roster
ok  	github.com/Harrison-Blair/fledge/internal/scan
ok  	github.com/Harrison-Blair/fledge/internal/spec

$ go vet ./...
(clean, no output)

$ gofmt -l .
(clean, no output)

$ go build -o /tmp/fledge-090 ./cmd/fledge && /tmp/fledge-090 preen
WARN  .fledge/pluma/feathers/FTHR-061-....md: checked criteria missing evidence
sections in .../molt/FTHR-061.md: AC-1..AC-5 (heading must be bare "## AC-N")
1 warning(s)
```

The one `preen` warning is pre-existing (FTHR-061's evidence file, unrelated
to this feather — not touched by this change) and is a WARN, not an error;
`preen` reports no errors, satisfying PLM-035 AC-13.
