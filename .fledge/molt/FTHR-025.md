# FTHR-025 evidence

## AC-1

Tests written first (`internal/hooktest/precommit_test.go`), run against the
not-yet-existing `scripts/hooks/pre-commit`. All 5 fail for the expected
reason: the hook script does not exist yet, so `core.hooksPath` points at
nothing.

Command: `go test ./internal/hooktest/... -v`

```
=== RUN   TestPreCommitHook_BlocksUnformattedFile
    precommit_test.go:105: read source hook script: open /home/penguin/source/fledge/.fledge/burrows/FTHR-025/scripts/hooks/pre-commit: no such file or directory
--- FAIL: TestPreCommitHook_BlocksUnformattedFile (0.00s)
=== RUN   TestPreCommitHook_BlocksVetViolation
    precommit_test.go:121: read source hook script: open /home/penguin/source/fledge/.fledge/burrows/FTHR-025/scripts/hooks/pre-commit: no such file or directory
--- FAIL: TestPreCommitHook_BlocksVetViolation (0.00s)
=== RUN   TestPreCommitHook_AllowsCleanCommit
    precommit_test.go:137: read source hook script: open /home/penguin/source/fledge/.fledge/burrows/FTHR-025/scripts/hooks/pre-commit: no such file or directory
--- FAIL: TestPreCommitHook_AllowsCleanCommit (0.00s)
=== RUN   TestPreCommitHook_NoOpWithoutHooksPathConfigured
    precommit_test.go:162: read source hook script: open /home/penguin/source/fledge/.fledge/burrows/FTHR-025/scripts/hooks/pre-commit: no such file or directory
--- FAIL: TestPreCommitHook_NoOpWithoutHooksPathConfigured (0.00s)
=== RUN   TestPreCommitHook_MatchesCICommands
    precommit_test.go:181: read hook script: open /home/penguin/source/fledge/.fledge/burrows/FTHR-025/scripts/hooks/pre-commit: no such file or directory
--- FAIL: TestPreCommitHook_MatchesCICommands (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/hooktest	0.011s
FAIL
```

After authoring `scripts/hooks/pre-commit`, all 5 pass:

Command: `go test ./internal/hooktest/... -v`

```
=== RUN   TestPreCommitHook_BlocksUnformattedFile
--- PASS: TestPreCommitHook_BlocksUnformattedFile (0.03s)
=== RUN   TestPreCommitHook_BlocksVetViolation
--- PASS: TestPreCommitHook_BlocksVetViolation (0.05s)
=== RUN   TestPreCommitHook_AllowsCleanCommit
--- PASS: TestPreCommitHook_AllowsCleanCommit (0.05s)
=== RUN   TestPreCommitHook_NoOpWithoutHooksPathConfigured
--- PASS: TestPreCommitHook_NoOpWithoutHooksPathConfigured (0.01s)
=== RUN   TestPreCommitHook_MatchesCICommands
--- PASS: TestPreCommitHook_MatchesCICommands (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.137s
```

## AC-2

`TestPreCommitHook_BlocksUnformattedFile` stages `bad.go` (missing space
before `{`, no space around `:=`) with `core.hooksPath` configured, commits,
and asserts a non-zero exit and that the hook's combined output names
`bad.go`. Passes (see AC-1 post-implementation run above); the hook's
`gofmt -l .` step lists the file and the script prints:
`pre-commit: the following files are not gofmt-formatted:` followed by the
`gofmt -l` listing (`bad.go`), then exits 1.

## AC-3

`TestPreCommitHook_BlocksVetViolation` stages `vetbad.go` — gofmt-clean but
containing a `fmt.Printf("%d\n", "hello")` format/argument mismatch — with
`core.hooksPath` configured, commits, and asserts a non-zero exit and that
the hook's output names `vetbad.go` (i.e. shows `go vet`'s diagnostic).
Passes (see AC-1 post-implementation run above); the hook's `go vet ./...`
step reports `vet: ./vetbad.go:6:18: Printf format %d has arg "hello" of
wrong type string`, which the script prints under
`pre-commit: go vet ./... reported issues:`, then exits 1.

## AC-4

`TestPreCommitHook_AllowsCleanCommit` stages a fully clean `.go` file with
`core.hooksPath` configured, commits, and asserts the commit succeeds (exit
0) and that the file's bytes on disk are identical before and after the
commit. Passes (see AC-1 post-implementation run above) — the hook script
only ever reads (`gofmt -l`, `go vet`) and never writes any file.

## AC-5

`TestPreCommitHook_NoOpWithoutHooksPathConfigured` stages the same
badly-formatted file used in AC-2 but leaves `core.hooksPath` unset
(default), commits, and asserts the commit succeeds — since git never
invokes a hook it isn't configured to run. Passes (see AC-1 post-implementation
run above).

## AC-6

`TestPreCommitHook_MatchesCICommands` reads `scripts/hooks/pre-commit` and
asserts it contains the literal strings `gofmt -l .` and `go vet ./...` —
the same commands PLM-012's CI lint job runs (see FTHR-022's Approach:
"`gofmt -l .` ... `go vet ./...`", and FTHR-023's Approach:
"re-run `gofmt -l .`, `go vet ./...`, ..."). PLM-012's feathers (FTHR-022,
FTHR-023) haven't merged yet in this repo (no `.github/workflows/` present),
so per the feather's Tests note this assertion compares against the literal
command strings specified in those feathers' Approach sections rather than
reading actual workflow files. Passes (see AC-1 post-implementation run
above).

## Full suite / repo-wide checks

Commands run from the worktree root after implementation:

```
$ gofmt -l .
internal/bootstrap/drift_test.go
internal/bootstrap/primitives.go
internal/bootstrap/registry.go
internal/bootstrap/registry_test.go
internal/cli/agents.go
internal/cli/init.go
internal/cli/preen.go
internal/nest/nest.go
internal/nest/nest_test.go
```

(All pre-existing, unrelated to this feather's files — none of
`internal/hooktest/precommit_test.go`, `scripts/hooks/pre-commit`, or
`CLAUDE.md` appear in this list, i.e. this feather's new/changed files are
gofmt-clean.)

```
$ go vet ./...
$ echo $?
0
$ go build ./...
$ echo $?
0
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.069s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.012s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.129s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.002s
```
