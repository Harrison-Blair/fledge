# FTHR-070 evidence

## AC-1

Test-file placement confirmed per the spec's grep before writing the test:

```
$ grep -rl "team-loop.md\|team_loop" internal/bootstrap --include='*_test.go'
internal/bootstrap/tmux_autodefault_test.go
internal/bootstrap/registry_test.go
```

Existing team-loop doc tests live in `internal/bootstrap` (package `bootstrap`),
reading the embedded doc via `FS.ReadFile("adapters/claude/team-loop.md")` —
the new test `TestTeamLoopDocDescribesCompactAdvisory` was placed alongside
them in `internal/bootstrap/compact_advisory_test.go`.

Pre-implementation run against the unchanged `team-loop.md` (captured before
any doc change was made):

```
$ go test ./internal/bootstrap -run TestTeamLoopDocDescribesCompactAdvisory
--- FAIL: TestTeamLoopDocDescribesCompactAdvisory (0.00s)
    compact_advisory_test.go:28: team-loop.md missing compact-advisory wording "`/compact` is safe to run now"
    compact_advisory_test.go:28: team-loop.md missing compact-advisory wording "digest-planning.md"
    compact_advisory_test.go:28: team-loop.md missing compact-advisory wording "digest-foraging.md"
    compact_advisory_test.go:28: team-loop.md missing compact-advisory wording "digest-implementation.md"
    compact_advisory_test.go:28: team-loop.md missing compact-advisory wording "user-facing guidance only"
    compact_advisory_test.go:28: team-loop.md missing compact-advisory wording "no automated trigger"
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

Failure is the expected reason: the doc has no compact-advisory wording yet.

Post-implementation run (after adding the "## Digest and compaction" section):

```
$ go test ./internal/bootstrap -run TestTeamLoopDocDescribesCompactAdvisory -v
=== RUN   TestTeamLoopDocDescribesCompactAdvisory
--- PASS: TestTeamLoopDocDescribesCompactAdvisory (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

## AC-2

`internal/bootstrap/adapters/claude/team-loop.md` now has a
"## Digest and compaction" section immediately after "## Planning delegation"
(before "## The team task list"). Its full text:

```
## Digest and compaction

Once a phase's digest file is written (`digest-planning.md`, `digest-foraging.md`, or `digest-implementation.md`, per `planning.md`/`foraging.md`/`implementation.md`), your close-out reply to the user for that phase includes a one-line note that `/compact` is safe to run now if the session's context has grown large. This applies to all three phase closes. It is user-facing guidance only — Claude Code exposes no mechanism for an agent to compact its own context mid-session, so there is no automated trigger to wire up; whether to run `/compact` is the user's call.
```

This ties the advisory to digest completion, covers all three phase closes,
and explicitly frames it as user-facing guidance with no automated trigger
(satisfies PLM-029 AC-3). Only the embedded source was edited — the scaffolded
`.claude/team-loop.md` copy was not touched:

```
$ git status --short
 M internal/bootstrap/adapters/claude/team-loop.md
?? .fledge/molt/FTHR-070.md
?? internal/bootstrap/compact_advisory_test.go
```

## AC-3

```
$ go test ./internal/bootstrap/...
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.006s
```

Also ran the full suite plus lint in the worktree — all green:

```
$ gofmt -l . && go vet ./... && go test ./... 2>&1 | tail -20
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.105s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.017s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.164s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.012s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.012s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.014s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.008s
```

