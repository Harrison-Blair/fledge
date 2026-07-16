# FTHR-057 evidence

## AC-1

Tests written first (`incubator_test.go`, `brooder_test.go`, `skua_test.go`,
`worker_protocols_stub_test.go`) and run against the unchanged docs.

Note: three test names (`TestSkuaConcessionHardened`, `TestSkuaEvidenceGuiltyUntilProven`,
`TestSkuaRedTeamPass`) are deliberately preserved from the old `worker_protocols_test.go`,
so the package did not compile until the old file (whose deletion is itself mandated by
AC-4) was removed. First run, with both files present:

```
$ go test ./internal/bootstrap -run 'TestIncubatorDocSections|TestBrooderDocSections|TestBrooderFixLoopInvariant|TestSkuaDocSections|TestWorkerProtocolsStub' -v
# github.com/Harrison-Blair/fledge/internal/bootstrap [github.com/Harrison-Blair/fledge/internal/bootstrap.test]
internal/bootstrap/worker_protocols_test.go:67:6: TestSkuaConcessionHardened redeclared in this block
	internal/bootstrap/skua_test.go:82:6: other declaration of TestSkuaConcessionHardened
internal/bootstrap/worker_protocols_test.go:90:6: TestSkuaEvidenceGuiltyUntilProven redeclared in this block
	internal/bootstrap/skua_test.go:103:6: other declaration of TestSkuaEvidenceGuiltyUntilProven
internal/bootstrap/worker_protocols_test.go:113:6: TestSkuaRedTeamPass redeclared in this block
	internal/bootstrap/skua_test.go:124:6: other declaration of TestSkuaRedTeamPass
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap [build failed]
FAIL
```

After `git rm internal/bootstrap/worker_protocols_test.go` (docs still unchanged), the
new tests fail for the expected reasons — the per-role files do not exist and
`worker-protocols.md` still has the old combined structure:

```
$ go test ./internal/bootstrap -run 'TestIncubatorDocSections|TestBrooderDocSections|TestBrooderFixLoopInvariant|TestSkuaDocSections|TestSkuaConcessionHardened|TestSkuaEvidenceGuiltyUntilProven|TestSkuaRedTeamPass|TestWorkerProtocolsStub' -v
=== RUN   TestBrooderDocSections
    brooder_test.go:21: open core/skills/fledge-orchestrate/brooder.md: file does not exist
--- FAIL: TestBrooderDocSections (0.00s)
=== RUN   TestBrooderFixLoopInvariant
    brooder_test.go:42: open core/skills/fledge-orchestrate/brooder.md: file does not exist
--- FAIL: TestBrooderFixLoopInvariant (0.00s)
=== RUN   TestIncubatorDocSections
    incubator_test.go:21: open core/skills/fledge-orchestrate/incubator.md: file does not exist
--- FAIL: TestIncubatorDocSections (0.00s)
=== RUN   TestSkuaDocSections
    skua_test.go:51: open core/skills/fledge-orchestrate/skua.md: file does not exist
--- FAIL: TestSkuaDocSections (0.00s)
=== RUN   TestSkuaConcessionHardened
    skua_test.go:83: open core/skills/fledge-orchestrate/skua.md: file does not exist
--- FAIL: TestSkuaConcessionHardened (0.00s)
=== RUN   TestSkuaEvidenceGuiltyUntilProven
    skua_test.go:104: open core/skills/fledge-orchestrate/skua.md: file does not exist
--- FAIL: TestSkuaEvidenceGuiltyUntilProven (0.00s)
=== RUN   TestSkuaRedTeamPass
    skua_test.go:125: open core/skills/fledge-orchestrate/skua.md: file does not exist
--- FAIL: TestSkuaRedTeamPass (0.00s)
=== RUN   TestWorkerProtocolsStub
    worker_protocols_stub_test.go:19: worker-protocols.md must no longer contain the "## Incubator" section heading
    worker_protocols_stub_test.go:19: worker-protocols.md must no longer contain the "## Brooder" section heading
    worker_protocols_stub_test.go:19: worker-protocols.md must no longer contain the "## Skua" section heading
    worker_protocols_stub_test.go:25: worker-protocols.md stub must link to "incubator.md"
    worker_protocols_stub_test.go:25: worker-protocols.md stub must link to "brooder.md"
    worker_protocols_stub_test.go:25: worker-protocols.md stub must link to "skua.md"
--- FAIL: TestWorkerProtocolsStub (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

Post-implementation, the same tests pass (fresh run, `-count=1`):

```
$ go test ./internal/bootstrap/... -count=1 -v -run 'TestIncubatorDocSections|TestBrooderDocSections|TestBrooderFixLoopInvariant|TestSkuaDocSections|TestSkuaConcessionHardened|TestSkuaEvidenceGuiltyUntilProven|TestSkuaRedTeamPass|TestWorkerProtocolsStub'
=== RUN   TestBrooderDocSections
--- PASS: TestBrooderDocSections (0.00s)
=== RUN   TestBrooderFixLoopInvariant
--- PASS: TestBrooderFixLoopInvariant (0.00s)
=== RUN   TestIncubatorDocSections
--- PASS: TestIncubatorDocSections (0.00s)
=== RUN   TestSkuaDocSections
--- PASS: TestSkuaDocSections (0.00s)
=== RUN   TestSkuaConcessionHardened
--- PASS: TestSkuaConcessionHardened (0.00s)
=== RUN   TestSkuaEvidenceGuiltyUntilProven
--- PASS: TestSkuaEvidenceGuiltyUntilProven (0.00s)
=== RUN   TestSkuaRedTeamPass
--- PASS: TestSkuaRedTeamPass (0.00s)
=== RUN   TestWorkerProtocolsStub
--- PASS: TestWorkerProtocolsStub (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.002s
```

## AC-2

The three new files exist, and a diff of each against the corresponding section
of the pre-split `worker-protocols.md` (a pristine copy saved to `/tmp/wp-original.md`
before editing; identical to `git show HEAD` of the file) shows exactly the one-line
heading demotion and nothing else. Section line ranges are those named in the spec's
Approach: incubator 7–40, brooder 42–72, skua 74–110.

```
$ cd internal/bootstrap/core/skills/fledge-orchestrate
$ diff <(sed -n '7,40p' /tmp/wp-original.md) incubator.md
1c1
< ## Incubator
---
> # Incubator
$ diff <(sed -n '42,72p' /tmp/wp-original.md) brooder.md
1c1
< ## Brooder
---
> # Brooder
$ diff <(sed -n '74,110p' /tmp/wp-original.md) skua.md
1c1
< ## Skua
---
> # Skua
```

The files were produced mechanically (`sed -n '<range>p' | sed '1s/^## /# /'`), so the
content moves are verbatim by construction; the diffs above confirm it independently.

## AC-3

`worker-protocols.md` now contains only the intro (first paragraph verbatim; second
paragraph's sole adjustment is "which protocol below to follow" → "which protocol file
to follow") plus a three-line links list. No role section headings remain:

```
$ grep -c "## Incubator\|## Brooder\|## Skua" internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md
0
$ cat internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md
# Worker protocols

The delegated worker roles, agent-neutral: the planning incubator, and the team-loop (Tier C) brooder and skua. These are spawned workers: a spawn prompt is a worker's entire context (it inherits no conversation history) and must be fully self-contained. A `spawn-worker` is fresh, named, addressable, killable, may idle, and returns one final message.

A worker's spawn prompt tells it which protocol file to follow (incubator, brooder, or skua), its name, the orchestrator's name (the harness-assigned name the orchestrator supplies — address the orchestrator by exactly that name; e.g. on Claude Code it is `team-lead`), and its role-specific fields — for brooders and skuas: feather ID, worktree/branch, evidence-file path, and the paired counterpart's name (same species); for the incubator: the user's feature request verbatim.

Each protocol lives in its own file:

- `incubator.md` — the delegated planner: owns the planning phase end to end; relay envelope, communication rules, drafting, lifecycle.
- `brooder.md` — the feather implementer: test-first protocol, scope discipline, evidence, handoff and fix loop, lifecycle.
- `skua.md` — the paired reviewer: review checks, criteria audit, verdict rules, lifecycle.
```

## AC-4

```
$ ls internal/bootstrap/worker_protocols_test.go
ls: cannot access 'internal/bootstrap/worker_protocols_test.go': No such file or directory
$ ls internal/bootstrap/{incubator,brooder,skua}_test.go internal/bootstrap/worker_protocols_stub_test.go
internal/bootstrap/brooder_test.go
internal/bootstrap/incubator_test.go
internal/bootstrap/skua_test.go
internal/bootstrap/worker_protocols_stub_test.go
```

The new tests pass — see the verbose AC-1 post-implementation run above (all eight
tests in the three role test files plus the stub test file, all PASS).

## AC-5

```
$ go test ./internal/bootstrap/... -count=1
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.006s
```

Note on the wider suite: `go test ./...` shows two failing txtar scripts
(`TestScripts/forager_contract`, `TestScripts/init_agents` in `cmd/fledge`) — those
fixtures grep this repo's *scaffolded* `.fledge/skills/.../worker-protocols.md`, which
this feather deliberately does not touch. FTHR-060 owns repointing those two fixtures
and its Tests section explicitly plans to observe them failing "after FTHR-057 has
landed but before this feather's edits"; FTHR-061 owns the scaffold refresh. This
inter-feather red is the planned sequencing, not a defect of this feather.

