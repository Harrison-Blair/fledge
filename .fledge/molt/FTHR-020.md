# FTHR-020 evidence

## AC-1

`go test ./cmd/fledge -run TestScripts` was run BEFORE any fixture edits, against the worktree base that already includes FTHR-017's code change (repo.go/init.go resolve specs under `.fledge/pluma/`). It failed as expected — every fixture still asserting the old `pluma/plumage`/`pluma/feathers` paths mismatched actual (now `.fledge/pluma/...`) output:

```
$ go test ./cmd/fledge -run TestScripts
--- FAIL: TestScripts/new (...)
    ...
    FAIL: testdata/new.txtar:...: no match for `.fledge/pluma/plumage/...` (etc.)
--- FAIL: TestScripts/check (...)
--- FAIL: TestScripts/report (...)
    FAIL: testdata/report.txtar:40: no match for `Feathers: 4 total \(egg: 1, pipping: 2, hatching: 0, fledged: 1\)` found in stdout
--- FAIL: TestScripts/lock (...)
--- FAIL: TestScripts/set (...)
--- FAIL: TestScripts/status (...)
    testscript.go:584: # fledge status: reads and legal transitions (0.004s)
        > exec git init -q .
        > exec fledge status FTHR-001
        [stderr]
        fledge: FTHR-001 not found
        [exit status 1]
        FAIL: testdata/status.txtar:4: unexpected command failure
--- FAIL: TestScripts/unfledged (...)
    FAIL: testdata/unfledged.txtar:43: no match for `PLM-002  hatched  P2  Open plumage` found in stdout
--- FAIL: TestScripts/criteria (...)
--- FAIL: TestScripts/graph (...)
    FAIL: testdata/graph.txtar:5: no match for `wave 1: FTHR-001 \[fledged\]` found in stdout
--- FAIL: TestScripts/ready (...)
    FAIL: testdata/ready.txtar:6: no match for `FTHR-002  P1  Second  \(plumage PLM-001\)` found in stdout
--- FAIL: TestScripts/init (...)
    FAIL: testdata/init.txtar:8: no match for `created pluma/plumage/.gitkeep` found in stdout
--- FAIL: TestScripts/stamp_warning (...)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.062s
```

(Full verbatim capture at time of run saved to `/tmp/fthr020_prefail.txt` in the worktree session; the 12 subtests above — new, check, report, lock, set, status, unfledged, criteria, graph, ready, init, stamp_warning — are exactly the 12 fixture files listed in the feather's Affected Modules.)

After updating all 12 fixture files' `pluma/plumage`/`pluma/feathers` path references to `.fledge/pluma/plumage`/`.fledge/pluma/feathers` (path strings only — no assertion-logic or ordering changes), the full suite was re-run and passes:

```
$ go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.082s

$ go test ./cmd/fledge -run TestScripts -v
=== RUN   TestScripts
=== RUN   TestScripts/agents
=== RUN   TestScripts/check
=== RUN   TestScripts/criteria
=== RUN   TestScripts/e2e
=== RUN   TestScripts/graph
=== RUN   TestScripts/init
=== RUN   TestScripts/init_agents
=== RUN   TestScripts/lock
=== RUN   TestScripts/nest
=== RUN   TestScripts/new
=== RUN   TestScripts/plan_delegation
=== RUN   TestScripts/preen_scaffold
=== RUN   TestScripts/ready
=== RUN   TestScripts/refresh_scaffold
=== RUN   TestScripts/report
=== RUN   TestScripts/scan
=== RUN   TestScripts/set
=== RUN   TestScripts/stamp_warning
=== RUN   TestScripts/status
=== RUN   TestScripts/unfledged
--- PASS: TestScripts (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.081s
```

Same tests (FR-4-pinning acceptance suite), unchanged commands: FAILING pre-fix (post-FTHR-017), PASSING post-fix. AC-1 satisfied.

## AC-2

Grepped each of the 12 listed files for any residual `pluma/plumage` or `pluma/feathers` reference NOT already prefixed with `.fledge/` (negative lookbehind for `.fledge/`):

```
$ for f in new check report lock set status unfledged criteria graph ready init stamp_warning; do
    c=$(perl -ne 'print "$_" if /(?<!\.fledge\/)pluma\/(plumage|feathers)/' "cmd/fledge/testdata/$f.txtar" | wc -l)
    echo "$f: $c"
  done
new: 0
check: 0
report: 0
lock: 0
set: 0
status: 0
unfledged: 0
criteria: 0
graph: 0
ready: 0
init: 0
stamp_warning: 0
```

All 12 files now read `.fledge/pluma/...` exclusively — zero residual `pluma/plumage`/`pluma/feathers` refs. `git diff --stat` confirms only these 12 files changed, 104 insertions / 104 deletions (matching the spec's ~104 ref count), and `e2e.txtar`/`preen_scaffold.txtar` are untouched (`git status --short` shows only the 12 listed files as modified). AC-2 satisfied.

## AC-3

The full 21-file `cmd/fledge` acceptance suite passes (see AC-1 post-fix run above — `ok  github.com/Harrison-Blair/fledge/cmd/fledge` with all 21 `=== RUN TestScripts/*` subtests, including previously-unaffected ones like `agents`, `e2e`, `init_agents`, `nest`, `plan_delegation`, `preen_scaffold`, `refresh_scaffold`, `scan`). AC-3 satisfied.
