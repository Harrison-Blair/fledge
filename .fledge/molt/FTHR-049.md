# FTHR-049 evidence — Gitignore .alloc.lock in scaffolded .gitignore

## AC-1
Tests observed failing before implementation, passing after; captured verbatim.

Test extended: `cmd/fledge/testdata/init.txtar` — asserts the scaffolded
`.gitignore` contains a `.alloc.lock` entry and that a bare `.alloc.lock`
pattern is honored by `git check-ignore` at both allocation-directory depths.

### Before implementation (FAILING)
Command: `go test ./cmd/fledge -run 'TestScripts/init$' -v`

```
        > grep '.fledge/nest/raw/' .gitignore
        > grep '.fledge/broods/' .gitignore
        > grep '.alloc.lock' .gitignore
        [.gitignore]
        # fledge — per-run intermediates, regenerable
        .fledge/nest/raw/
        .fledge/broods/

        FAIL: testdata/init.txtar:28: no match for `.alloc.lock` found in .gitignore

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/init (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.005s
FAIL
```

### After implementation (PASSING)
Command: `go test ./cmd/fledge -run 'TestScripts/init$' -v`

```
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/init (0.01s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.013s
```

The passing run exercises both new assertions: `grep '.alloc.lock' .gitignore`
(entry present) and `exec git check-ignore .fledge/pluma/plumage/.alloc.lock
.fledge/pluma/feathers/.alloc.lock` (bare pattern honored at both depths).

## AC-2
`internal/cli/init.go` `gitignoreLines` includes `.alloc.lock` — a bare
(slash-free) pattern, which git matches by basename at any directory depth,
so it covers every `NextID` allocation directory (satisfies PLM-024 FC-3, AC-3).

```
$ sed -n '30p' internal/cli/init.go
var gitignoreLines = []string{".fledge/nest/raw/", ".fledge/broods/", ".alloc.lock"}
```

Both consumers pick this up unchanged: `baseScaffoldEntries` (stamp `Lines`,
line ~398) and `ensureGitignore`'s append loop (line ~471).

## AC-3
This repo's `.gitignore` refreshed via `fledge init --refresh` (binary rebuilt
and installed from this worktree first); both existing `.alloc.lock` files are
ignored (satisfies PLM-024 AC-3).

```
$ fledge init --refresh
note: refreshed 1 file(s) to the shipped versions ...
created .gitignore
updated .fledge/scaffold.json
...

$ tail -2 .gitignore
# fledge — per-run intermediates, regenerable
.alloc.lock

$ fledge preen
spec clean: 26 plumages, 56 feathers        # exit 0 — scaffold healthy

$ git check-ignore .fledge/pluma/plumage/.alloc.lock .fledge/pluma/feathers/.alloc.lock
.fledge/pluma/plumage/.alloc.lock
.fledge/pluma/feathers/.alloc.lock          # exit 0 — both ignored
```

## AC-4
`go test ./cmd/fledge -run TestScripts` passes (full script suite).

```
$ go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.091s
```

Full `go test ./...`, `go build ./...`, `go vet ./...`, and
`gofmt -l internal/cli/init.go` (no output) all clean.
