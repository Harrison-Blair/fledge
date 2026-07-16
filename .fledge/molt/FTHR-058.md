# FTHR-058 evidence

## AC-1

Both new tests were written first and observed failing against the unchanged code.

Note on the stale-pattern check: the spec's Tests section names the literal
substring `worker-protocols.md §`, but the docs backtick the filename
(`` `worker-protocols.md` §Incubator ``), so that literal never matches and
could never fail. The test instead asserts zero occurrences of the bare
substring `worker-protocols.md` in the three docs — strictly stronger, and it
does fail against the stale docs (all five Approach sites repoint to
`incubator.md`, so no reference should remain).

Pre-implementation run (`go test ./internal/bootstrap -run TestCoreDocsRepointToRoleFiles -v; go test ./internal/doctest -run TestClaudeMdReferencesRoleFiles -v`):

```
=== RUN   TestCoreDocsRepointToRoleFiles
    core_docs_repoint_test.go:23: planning.md still references "worker-protocols.md"
    core_docs_repoint_test.go:26: planning.md must reference "incubator.md"
    core_docs_repoint_test.go:23: implementation.md still references "worker-protocols.md"
    core_docs_repoint_test.go:26: implementation.md must reference "incubator.md"
    core_docs_repoint_test.go:23: foraging.md still references "worker-protocols.md"
    core_docs_repoint_test.go:26: foraging.md must reference "incubator.md"
--- FAIL: TestCoreDocsRepointToRoleFiles (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
===
=== RUN   TestClaudeMdReferencesRoleFiles
    claude_md_test.go:16: CLAUDE.md must reference "incubator.md"
    claude_md_test.go:16: CLAUDE.md must reference "brooder.md"
    claude_md_test.go:16: CLAUDE.md must reference "skua.md"
--- FAIL: TestClaudeMdReferencesRoleFiles (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
FAIL
```

Post-implementation run (same commands):

```
=== RUN   TestCoreDocsRepointToRoleFiles
--- PASS: TestCoreDocsRepointToRoleFiles (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
===
=== RUN   TestClaudeMdReferencesRoleFiles
--- PASS: TestClaudeMdReferencesRoleFiles (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
```

## AC-2

All five Approach sites in the embedded core docs repointed to `incubator.md`;
zero remaining references to `worker-protocols.md` in the three docs
(the § check is subsumed — no filename occurrence at all remains).

`grep -rn 'worker-protocols.md' internal/bootstrap/core/skills/fledge-orchestrate/{planning,implementation,foraging}.md` → no output (exit 1).

`grep -n 'incubator.md' internal/bootstrap/core/skills/fledge-orchestrate/{planning,implementation,foraging}.md`:

```
planning.md:9:... the instruction to run steps 1–4 of this file per the Incubator protocol in `incubator.md`. ...
planning.md:12:  - When the incubator sends a `GATE` or `QUESTION` message (envelope in `incubator.md`), ...
planning.md:19:... or as a relayed `GATE`/`QUESTION` message per `incubator.md` when delegated.
implementation.md:28:... — see `planning.md` §0 and `incubator.md`.)
foraging.md:9:... — `planning.md` §2 and `incubator.md` point here. ...
```

Enforced by `TestCoreDocsRepointToRoleFiles` (passing, see AC-1).

## AC-3

`CLAUDE.md` line 122–123 now reads:

```
  `WriteCore`. This is where the actual workflow prose (planning.md,
  implementation.md, worker-protocols.md, incubator.md, brooder.md, skua.md, templates/) lives.
```

`worker-protocols.md` (the stub) stays in the list; the three per-role files
are added. Enforced by `TestClaudeMdReferencesRoleFiles` (passing, see AC-1).

## AC-4

`go test ./internal/bootstrap/... ./internal/doctest/...`:

```
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
```

Additionally `gofmt -l .` (clean), `go vet ./...` (clean), and the full
`go test ./...` suite in the worktree — all packages ok.
