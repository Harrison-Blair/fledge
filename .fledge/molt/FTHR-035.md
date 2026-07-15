# FTHR-035 evidence: Test status and set --json output shapes

## AC-1: `status.txtar` asserts `status --json` emits `{id, from, to}` with correct values

Added to `cmd/fledge/testdata/status.txtar` (appended at end, driving
`FTHR-002` back to `pipping` via `--force` then forward to `hatching` with
`--json`, asserting the emitted keys/values):

```
# --json emits {id, from, to} for a driven transition
exec fledge status FTHR-002 pipping --force
exec fledge status FTHR-002 hatching --json
stdout '"id": "FTHR-002"'
stdout '"from": "pipping"'
stdout '"to": "hatching"'
```

Pre-perturbation, passing run:

```
$ go test ./cmd/fledge -run 'TestScripts/status'
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.027s
```

Bite proof — perturbed `internal/cli/status.go` (`emitJSON` call in
`runStatus`) to rename keys `from`/`to` -> `fromX`/`toX`:

```go
// before
return emitJSON(map[string]string{"id": id, "from": from, "to": next})
// after (perturbation)
return emitJSON(map[string]string{"id": id, "fromX": from, "toX": next})
```

Ran with the perturbation in place:

```
$ go test ./cmd/fledge -run 'TestScripts/status' -v
...
        # --json emits {id, from, to} for a driven transition (0.003s)
        > exec fledge status FTHR-002 pipping --force
        [stdout]
        FTHR-002: fledged -> pipping
        > exec fledge status FTHR-002 hatching --json
        [stdout]
        {
          "fromX": "pipping",
          "id": "FTHR-002",
          "toX": "hatching"
        }
        > stdout '"id": "FTHR-002"'
        > stdout '"from": "pipping"'
        FAIL: testdata/status.txtar:49: no match for `"from": "pipping"` found in stdout

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/status (0.02s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.023s
FAIL
```

New assertion fails for the expected reason (renamed key no longer matches).
Reverted the perturbation and confirmed green:

```
$ go test ./cmd/fledge -run 'TestScripts/status'
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.027s
```

`git diff --stat` after revert showed only the two `testdata/*.txtar` files
changed — no production code left modified.

## AC-2: `set.txtar` asserts `set --json` emits `{id, field, value}` with correct values

Added to `cmd/fledge/testdata/set.txtar` (appended after the existing
`priority` checks, driving a fresh mutation on `FTHR-001` with `--json`):

```
# --json emits {id, field, value} for a driven mutation
exec fledge set FTHR-001 priority P2 --json
stdout '"id": "FTHR-001"'
stdout '"field": "priority"'
stdout '"value": "P2"'
grep 'priority: P2' .fledge/pluma/feathers/FTHR-001-first.md
```

Passing run:

```
$ go test ./cmd/fledge -run 'TestScripts/set'
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.020s
```

## AC-3: recorded perturbation makes the new assertions fail; revert restores green

Same procedure applied to `set.go`'s `emitJSON` call in `runSet`
(`field`/`value` -> `fieldX`/`valueX`):

```go
// before
return emitJSON(map[string]string{"id": id, "field": field, "value": value})
// after (perturbation)
return emitJSON(map[string]string{"id": id, "fieldX": field, "valueX": value})
```

Ran with the perturbation in place:

```
$ go test ./cmd/fledge -run 'TestScripts/set' -v
...
        # --json emits {id, field, value} for a driven mutation (0.001s)
        > exec fledge set FTHR-001 priority P2 --json
        [stdout]
        {
          "fieldX": "priority",
          "id": "FTHR-001",
          "valueX": "P2"
        }
        > stdout '"id": "FTHR-001"'
        > stdout '"field": "priority"'
        FAIL: testdata/set.txtar:12: no match for `"field": "priority"` found in stdout

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/set (0.01s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.007s
FAIL
```

New assertion fails for the expected reason. Reverted the perturbation and
confirmed green:

```
$ go test ./cmd/fledge -run 'TestScripts/set'
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.020s
```

Both perturbations (status.go and set.go) were applied and reverted one at a
time, never left in place together; `git diff` after each revert showed only
the txtar files changed.

## AC-4: `fledge preen` passes; targeted and full test suites are green

```
$ go build -o /tmp/fledge_preen ./cmd/fledge && /tmp/fledge_preen preen; echo "exit=$?"
WARN  .fledge/pluma/feathers/FTHR-029-...md: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-032-...md: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-033-...md: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-034-...md: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-035-...md: status hatching but no brood is held
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
7 warning(s)
exit=0
```

Exit 0 — preen passes (warnings only; all pre-existing and unrelated to this
feather's brood/lock and scaffold state, not caused by this change).

```
$ go test ./cmd/fledge -run 'TestScripts/status'
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.027s

$ go test ./cmd/fledge -run 'TestScripts/set'
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.020s

$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.085s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.120s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.007s
```

## Scope

Only `cmd/fledge/testdata/status.txtar` and `cmd/fledge/testdata/set.txtar`
were changed (plus this evidence file). No production code
(`internal/cli/status.go`, `internal/cli/set.go`) was left modified —
perturbations were transient, applied and reverted one at a time to capture
the bite proof. `lock.txtar` was not touched.
