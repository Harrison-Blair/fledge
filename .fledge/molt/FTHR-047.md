# FTHR-047 — Wire fledge colony into plumage closeout — molt evidence

## AC-1

Test-first: added assertions to `cmd/fledge/testdata/init.txtar` requiring the
scaffolded `implementation.md` to instruct `fledge colony --json` in both the
solo and team closeout steps (exactly 2 occurrences) and to no longer contain
the old "if that was the last unfinished feather" mental-tracking phrasing.

New assertions added to `init.txtar`:

```
grep -count=2 'fledge colony --json' .fledge/skills/fledge-orchestrate/implementation.md
! grep 'last unfinished feather' .fledge/skills/fledge-orchestrate/implementation.md
```

### Failing run BEFORE implementation (against current/old scaffolded content)

```
$ go test ./cmd/fledge -run TestScripts/init
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/init (0.01s)
            > grep -count=2 'fledge colony --json' .fledge/skills/fledge-orchestrate/implementation.md
            FAIL: testdata/init.txtar:85: no match for `fledge colony --json` found in .fledge/skills/fledge-orchestrate/implementation.md
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.051s
FAIL
```

The current scaffold has 0 occurrences of `fledge colony --json` (mental-tracking
phrasing "last unfinished feather" appears twice), so the new `grep -count=2`
assertion fails, confirming the test pins the behavior change.

### Passing run AFTER implementation

After rewriting the closeout steps in the embedded source
(`internal/bootstrap/core/skills/fledge-orchestrate/implementation.md`),
`go install ./cmd/fledge`, and `fledge init --refresh` to regenerate the
scaffolded copy:

```
$ go test ./cmd/fledge -run TestScripts/init
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.049s
```

## AC-2

Both closeout steps in the embedded source now instruct running
`fledge colony --json` and closing the plumage only when the entry's `fledged`
count equals its `total`, replacing the old mental-tracking phrasing. The
scaffolded copy matches after refresh:

```
$ grep -n 'colony --json\|last unfinished' .fledge/skills/fledge-orchestrate/implementation.md
67:11. **Plumage closeout:** run `fledge colony --json` and find the just-completed feather's plumage in its `plumages` array; treat the plumage as ready to close only when that entry's `fledged` count equals its `total` (every feather fledged — `done == total`). When it is, verify each plumage acceptance criterion — ...
109:5. **Plumage closeout:** run `fledge colony --json` and find this feather's plumage in its `plumages` array; treat the plumage as ready to close only when that entry's `fledged` count equals its `total` (every feather fledged — `done == total`). When it is, first confirm every pair for this plumage's feathers has already been torn down at its green teardown — ...
```

Line 67 is the solo closeout (§2 step 11); line 109 is the team closeout
(§3 step 5). No occurrences of "last unfinished feather" remain. The
`fledged`/`total` fields are the real JSON keys emitted per plumage by
`fledge colony --json` (`internal/cli/colony.go` `reqCompletion`: `Done`→`fledged`,
`Total`→`total`).

## AC-3

`fledge init --refresh` regenerated this repo's scaffolded copy, `fledge preen`
reports the scaffold healthy, and the full `TestScripts` suite passes:

```
$ fledge init --refresh --force
updated .fledge/skills/fledge-orchestrate/implementation.md
updated .fledge/scaffold.json

$ fledge preen
spec clean: 26 plumages, 56 feathers

$ go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.082s
```

Full `go test ./...` also passes across all packages.
