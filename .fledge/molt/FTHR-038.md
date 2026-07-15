# FTHR-038 molt evidence

## AC-1

New file `internal/bootstrap/tmux_autodefault_test.go` written first, then run
against the unmodified `team-loop.md` / `implementation.md`. Command:

```
go test ./internal/bootstrap -run "TestTmux|TestImplementationPrecondition|TestPermissionModeUnchanged" -v
```

Pre-implementation output (captured verbatim):

```
=== RUN   TestTmuxPreconditionAutoResolves
    tmux_autodefault_test.go:24: team-loop.md still contains old gating language "surfaces this via a `confirm-gate`"
    tmux_autodefault_test.go:24: team-loop.md still contains old gating language "stop and restart inside tmux (recommended)"
    tmux_autodefault_test.go:34: team-loop.md missing expected auto-resolve wording "no `confirm-gate`"
    tmux_autodefault_test.go:34: team-loop.md missing expected auto-resolve wording "tmux detected — spawning teammates in panes"
    tmux_autodefault_test.go:34: team-loop.md missing expected auto-resolve wording "tmux not detected — proceeding degraded, in-process teammates"
--- FAIL: TestTmuxPreconditionAutoResolves (0.00s)
=== RUN   TestImplementationPreconditionCarveOut
    tmux_autodefault_test.go:52: implementation.md still contains the old unqualified sentence
    tmux_autodefault_test.go:56: implementation.md missing wording that some preconditions auto-resolve without a gate
    tmux_autodefault_test.go:59: implementation.md missing scoped 'never silently proceed' instruction for gated preconditions
--- FAIL: TestImplementationPreconditionCarveOut (0.00s)
=== RUN   TestPermissionModeUnchanged
--- PASS: TestPermissionModeUnchanged (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

`TestTmuxPreconditionAutoResolves` and `TestImplementationPreconditionCarveOut`
fail for the expected reason: the "absent" (old-language) assertions pass,
but the "present" (new-wording) assertions fail because the replacement
wording doesn't exist yet. `TestPermissionModeUnchanged` already passes
against the unmodified file, as expected (it guards content this feather
must NOT change).

## AC-2

`team-loop.md` § Teammate display (tmux) precondition paragraph rewritten to
state the auto-resolve behavior (tmux present → panes; tmux absent →
degraded in-process teammates) with no `confirm-gate` language for this
precondition. `TestTmuxPreconditionAutoResolves` passes post-implementation
(see command/output under AC-6 below, same run).

## AC-3

Same paragraph documents the non-blocking notice for both paths as example
lines: `"tmux detected — spawning teammates in panes"` and `"tmux not
detected — proceeding degraded, in-process teammates"`. Covered by
`TestTmuxPreconditionAutoResolves` (asserts both example strings present).

## AC-4

`implementation.md` §1's "Tier C only — harness piping preconditions" bullet
rewritten: states some preconditions auto-resolve without a gate (piping
file says which), and scopes "never silently proceed" to preconditions the
piping file says to gate — no longer a blanket statement covering tmux.
`TestImplementationPreconditionCarveOut` passes post-implementation.

## AC-5 / AC-6

`team-loop.md`'s permission-mode paragraph (in "## Spawning and addressing
teammates") and every other section of both files are unchanged — verified
by `TestPermissionModeUnchanged` (passes, asserting the permission-mode
sentence and its confirm-gate reference are present verbatim) and by `git
diff` scoped to exactly the two edited paragraphs:

```
$ git diff --stat internal/bootstrap/adapters/claude/team-loop.md internal/bootstrap/core/skills/fledge-orchestrate/implementation.md
 internal/bootstrap/adapters/claude/team-loop.md                     | 2 +-
 internal/bootstrap/core/skills/fledge-orchestrate/implementation.md | 2 +-
 2 files changed, 2 insertions(+), 2 deletions(-)
```

Each file shows exactly one changed line (the targeted paragraph), confirming
no other section changed (PLM-019 FC-6).

Full post-implementation test run:

```
$ go test ./internal/bootstrap -run "TestTmux|TestImplementationPrecondition|TestPermissionModeUnchanged" -v
=== RUN   TestTmuxPreconditionAutoResolves
--- PASS: TestTmuxPreconditionAutoResolves (0.00s)
=== RUN   TestImplementationPreconditionCarveOut
--- PASS: TestImplementationPreconditionCarveOut (0.00s)
=== RUN   TestPermissionModeUnchanged
--- PASS: TestPermissionModeUnchanged (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

`go vet ./...` and `go test ./internal/bootstrap/...`:

```
$ go vet ./...
$ go test ./internal/bootstrap/...
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
```

Both green — satisfies AC-6.
