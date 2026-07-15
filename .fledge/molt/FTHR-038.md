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
