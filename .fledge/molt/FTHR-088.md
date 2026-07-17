# FTHR-088 evidence

## AC-1

Test-first order: (1) wrote the unit tests in `internal/cli/await_test.go`
(new `exists` parameter to `pollAwait`, new tests for existence-wait,
`TestAwaitRequiresTimeoutBothModes`, `TestAwaitUsageTextNamesPerKindModes`)
and reworked `cmd/fledge/testdata/await.txtar` to the race-free shape, all
against the **unchanged** `internal/cli/await.go`. (2) Ran them and captured
the FAIL output verbatim below. (3) Only then implemented `await.go`.

### Unit tests, pre-implementation (FAIL for the expected reason: `pollAwait`'s call sites pass the new `exists bool` argument that the unchanged 7-argument signature doesn't accept — a parameter-arity compile error, not an unrelated break)

```
$ go test ./internal/cli -run 'TestAwait' -v
# github.com/Harrison-Blair/fledge/internal/cli [github.com/Harrison-Blair/fledge/internal/cli.test]
internal/cli/await_test.go:36:76: too many arguments in call to pollAwait
	have (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, bool, number, bool)
	want (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, "time".Duration, bool)
internal/cli/await_test.go:65:76: too many arguments in call to pollAwait
	have (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, bool, number, bool)
	want (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, "time".Duration, bool)
internal/cli/await_test.go:88:88: too many arguments in call to pollAwait
	have (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, bool, "time".Duration, bool)
	want (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, "time".Duration, bool)
internal/cli/await_test.go:114:76: too many arguments in call to pollAwait
	have (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, bool, number, bool)
	want (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, "time".Duration, bool)
internal/cli/await_test.go:142:79: too many arguments in call to pollAwait
	have (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, bool, number, bool)
	want (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, "time".Duration, bool)
internal/cli/await_test.go:171:88: too many arguments in call to pollAwait
	have (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, bool, "time".Duration, bool)
	want (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, "time".Duration, bool)
internal/cli/await_test.go:196:88: too many arguments in call to pollAwait
	have (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, bool, "time".Duration, bool)
	want (func(dir string, subject string, kind string) (*ledger.Record, error), awaitClock, string, string, string, "time".Duration, bool)
FAIL	github.com/Harrison-Blair/fledge/internal/cli [build failed]
FAIL
```

### Acceptance test, pre-implementation (FAIL for the expected reason: unchanged `await.go` doesn't define `--exists`)

```
$ go test ./cmd/fledge -run 'TestScripts/await' -v
=== RUN   TestScripts
=== RUN   TestScripts/await
...
        > exec fledge heartbeat watcher-subject --note 'now running'
        [stdout]
        heartbeat watcher-subject at 2026-07-17T06:37:22Z
        > exec fledge await watcher-subject --kind status --exists --timeout 5s --json
        [stderr]
        flag provided but not defined: -exists
        Usage of await:
          -json
            	machine-readable output
          -kind string
            	ledger record kind (required)
          -timeout string
            	maximum time to block, e.g. 200ms, 5s (default: block indefinitely)
        [exit status 2]
        FAIL: testdata/await.txtar:9: unexpected command failure

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/await (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.005s
FAIL
```

### Post-implementation (PASS)

```
$ go test ./internal/cli -run 'TestAwait' -v
=== RUN   TestAwaitReturnsOnAppearance
--- PASS: TestAwaitReturnsOnAppearance (0.00s)
=== RUN   TestAwaitChangeWaitStillDetectsPayloadChange
--- PASS: TestAwaitChangeWaitStillDetectsPayloadChange (0.00s)
=== RUN   TestAwaitTimesOutNoChange
--- PASS: TestAwaitTimesOutNoChange (0.00s)
=== RUN   TestAwaitExistsReturnsImmediatelyWhenPresent
--- PASS: TestAwaitExistsReturnsImmediatelyWhenPresent (0.00s)
=== RUN   TestAwaitExistsReturnsOnAppearance
--- PASS: TestAwaitExistsReturnsOnAppearance (0.00s)
=== RUN   TestAwaitExistsIgnoresIdenticalPayloadRewrite
--- PASS: TestAwaitExistsIgnoresIdenticalPayloadRewrite (0.00s)
=== RUN   TestAwaitExistsTimesOut
--- PASS: TestAwaitExistsTimesOut (0.00s)
=== RUN   TestAwaitRequiresTimeoutBothModes
=== RUN   TestAwaitRequiresTimeoutBothModes/change-wait
fledge: --timeout is required: usage: fledge await <subject> --kind <kind> --timeout <duration> [--exists] [--json]
=== RUN   TestAwaitRequiresTimeoutBothModes/exists
fledge: --timeout is required: usage: fledge await <subject> --kind <kind> --timeout <duration> [--exists] [--json]
--- PASS: TestAwaitRequiresTimeoutBothModes (0.00s)
    --- PASS: TestAwaitRequiresTimeoutBothModes/change-wait (0.00s)
    --- PASS: TestAwaitRequiresTimeoutBothModes/exists (0.00s)
=== RUN   TestAwaitUsageTextNamesPerKindModes
--- PASS: TestAwaitUsageTextNamesPerKindModes (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.002s

$ go test ./cmd/fledge -run 'TestScripts/await' -v
=== RUN   TestScripts
=== RUN   TestScripts/await
...
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.41s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.415s
```

## AC-2

Covered by `TestAwaitExistsReturnsImmediatelyWhenPresent` (unit, above): a
fake `read` that already returns a record on the very first call, in
`--exists` mode, returns it with exactly 1 read and no sleep — this is
precisely the "record already existed at call time" condition that
`pollAwait`'s unmodified change-wait logic would poll forever on (no baseline
mismatch, `hasTimeout=false`). This test failed to compile against unchanged
code (see AC-1) because `exists` didn't exist as a parameter; it passes
post-implementation.

Also covered end-to-end by `await.txtar`'s happy path (above): `heartbeat`
writes the `status` record for `watcher-subject` first, then
`fledge await watcher-subject --kind status --exists --timeout 5s --json`
returns it immediately (0.41s total script time, no polling wait involved).

## AC-3

Covered by `TestAwaitExistsReturnsOnAppearance` (unit, above): `read` returns
`NotFoundError` for the first 2 calls, then a record on the 3rd; `--exists`
mode polls and returns it, `calls >= 3` asserted. See PASS output above.

## AC-4

Covered by `TestAwaitExistsIgnoresIdenticalPayloadRewrite` (unit, above): a
fake `read` returns a record with the identical payload on every call (the
condition that hides a rewrite from change-wait's payload comparison).
`--exists` mode returns on the very first call (`calls == 1`), proving it
never waits for a payload change. `hasTimeout=true` with a 5s fake-clock
budget bounds the test in case of a future regression back to payload
comparison — it would then time out quickly (in fake-clock steps) rather than
loop unboundedly. See PASS output above.

Structurally: in `pollAwait`, the existence-wait branch (`if exists { if rec
!= nil { return ... } }`) is textually disjoint from the payload/baseline
comparison (`else if rec != nil && (!present || string(rec.Payload) !=
baselinePayload)`) and from the baseline-sampling block above the loop, which
is itself skipped entirely (`if !exists { ...baseline read... }`) when
`exists` is true. `Payload` is referenced nowhere on the `exists` code path.

## AC-5

Covered by `TestAwaitChangeWaitStillDetectsPayloadChange` (unit, above,
renamed/extended from the pre-existing `TestAwaitReturnsOnChange` — compiled
against the unchanged code before the `exists` parameter existed, so its
failure in the AC-1 capture is the same arity break, not a semantic one):
`read` returns payload `{"note":"first"}` for the first 2 calls then
`{"note":"second"}`; `exists=false` (change-wait) returns on the payload
change, matching FTHR-073's shipped semantics unaltered. `pollAwait`'s
change-wait branch (the `else if` above) is untouched line-for-line from
FTHR-073's version other than being reached via `else if` instead of a bare
`if`.

Also `TestAwaitReturnsOnAppearance` (unit, above, kept) continues to cover
change-wait's appearance-when-absent-at-baseline case unmodified.

`await.txtar`'s change-wait timeout scenario (`never-appears-2 --kind
status --timeout 200ms`, no `--exists`) exercises the change-wait path
end-to-end; see PASS output above.

## AC-6

Covered by `TestAwaitRequiresTimeoutBothModes` (unit, above): calls
`runAwait` directly (no repo fixture needed, since the check runs before
`repo.Find()`) with `{"some-subject", "--kind", "status"}` (change-wait) and
`{"some-subject", "--kind", "verdict", "--exists"}` (existence-wait), each
omitting `--timeout`; both assert `got == ExitUsage` (2). See PASS output
above — both subtests pass, and the printed message
(`--timeout is required: usage: fledge await ...`) names the flag.

Also `await.txtar`'s two "missing --timeout" lines (above) hit this at the
process level for both modes: `fledge await some-subject --kind status`
(exit 2, `stderr 'usage'`) and `fledge await some-subject --kind verdict
--exists` (exit 2, `stderr 'usage'`).

## AC-7

Covered by `TestAwaitExistsTimesOut` (unit, above; existence-wait side) and
the pre-existing `TestAwaitTimesOutNoChange` (unit, above; change-wait side,
kept and updated only for the new `exists` parameter, otherwise unchanged) —
both use the fake clock to advance past a timeout with the record never
appearing, and assert `timedOut == true`, `record == nil`.

`await.txtar`'s two real-elapsed-time timeout lines exercise both modes with
an actual wall-clock 200ms `--timeout` (not mocked), asserting exit code 4
via the `sh -c '...; test $? -eq 4'` idiom and the `--json` envelope's
`"timed_out": true` / `"record": null`:

```
> exec sh -c 'fledge await never-appears --kind verdict --exists --timeout 200ms --json; test $? -eq 4'
[stdout]
{
  "record": null,
  "timed_out": true
}
> stdout '"timed_out": true'
> stdout '"record": null'

> exec sh -c 'fledge await never-appears-2 --kind status --timeout 200ms --json; test $? -eq 4'
[stdout]
{
  "record": null,
  "timed_out": true
}
> stdout '"timed_out": true'
> stdout '"record": null'
```

(from the post-implementation `await.txtar` run captured under AC-1 above)

## AC-8

```
$ grep -nE '(^|[[:space:]])&[[:space:]]*$|^wait$' cmd/fledge/testdata/await.txtar
$ echo "exit: $?"
exit: 1
```

Grep found no match (exit 1 = no lines matched), confirming no `&`
backgrounding and no `wait` line anywhere in the reworked script. Visually:
the file's happy path is now two sequential `exec` lines (`heartbeat` then
`await --exists`), no `&`, no `wait` — see the file itself,
`cmd/fledge/testdata/await.txtar`.

## AC-9

**Structural argument (why the race is eliminated by construction, not merely made unlikely):**

The flaky happy path FTHR-073 shipped ran `fledge await ... --json &` as a
backgrounded process and then `fledge heartbeat ...` in the foreground,
followed by `wait`. That is two independent OS processes racing to reach
`pollAwait`'s baseline read before the write lands: if `await`'s baseline
sample ran *after* `heartbeat`'s write, the record was already present at
baseline, `present=true` with a matching payload, and change-wait's condition
`rec != nil && (!present || payload != baselinePayload)` was never true —
`await` polled forever with no timeout in that script, hanging until the
outer test timeout. The flake rate is a scheduling coin flip between two
processes, not a corner case in the code — hence the measured ~1-in-3 hang
rate.

The reworked script removes the second process and the race's precondition
entirely: `heartbeat` now runs to completion first (`exec fledge heartbeat
...`, a synchronous testscript command — testscript's `exec` does not return
until the child process exits and it is not followed by `&`), so the record
is on disk with a `Rename`-committed atomic write (`ledger.Write`, unchanged,
uses `os.Rename` for atomicity) *before* the `await` process is even
started. There is no longer a "started too early" outcome to race into:
`await --exists` is now started only after the write it depends on has
already returned. This is not "less likely to race" — there is only one
process alive at any point in the scenario, so there is nothing left to
interleave. Independently, `--exists` mode's own logic (AC-4) does not care
*when* the record arrived relative to the read that observes it — the first
`read` call in the loop, whenever it lands, either sees the record (already
there, or freshly written) and returns immediately, or doesn't and polls.
Both the process-ordering fix (no `&`/`wait`) and the mode fix (`--exists`
ignores the baseline distinction that the race depended on) independently
close the same hole; either alone would suffice, and both are present.

**20 consecutive runs, all green (backstop, not the proof):**

```
$ go test ./cmd/fledge -run 'TestScripts/await' -count=20 -timeout 120s
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	8.274s

$ go test ./cmd/fledge -run 'TestScripts/await' -count=20 -timeout 120s -v 2>&1 | grep -E '^(--- PASS| *--- PASS): TestScripts(/await)?'
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.41s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.41s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.41s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.41s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.41s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.41s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.41s)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (0.42s)
```

20/20 `TestScripts/await` subtests passed, 0 FAIL lines across two
independent 20-run captures (this run and an earlier one, both fresh —
per-subtest timings vary run to run, confirming these are live executions,
not a cached/repeated single result).

## AC-10

Covered by `TestAwaitUsageTextNamesPerKindModes` (unit, above): asserts
`commands["await"].usage` (the string registered via `register("await",
runAwait, ...)` in `await.go`'s `init()`) contains `--exists`, `verdict`,
`escalation`, and `status`, per the `command_parity_test.go` precedent of
asserting directly against package-internal state rather than a golden file
or process output. The registered usage string is:

```
fledge await <subject> --kind <kind> --timeout <duration> [--exists] [--json]
  wait mode: verdict/escalation kinds use --exists (has it landed?); status kind uses change-wait (has it changed?)
```

## AC-11

```
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.438s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.024s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.191s
ok  	github.com/Harrison-Blair/fledge/internal/ledger	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s

$ go vet ./...
(no output, exit 0)

$ gofmt -l .
(no output — nothing needs formatting)

$ go run ./cmd/fledge preen
WARN  .fledge/pluma/feathers/FTHR-061-refresh-scaffold-and-verify-worker-protocols-split.md: checked criteria missing evidence sections in /home/penguin/source/fledge/.fledge/burrows/FTHR-088/.fledge/molt/FTHR-061.md: AC-1, AC-2, AC-3, AC-4, AC-5 (heading must be the bare form "## AC-N", not "## AC-N: <label>")
1 warning(s)
exit: 0
```

`preen` exits 0 (warnings only). The one warning is pre-existing and
unrelated to this feather: it flags `FTHR-061`'s molt evidence heading
format, a file this feather does not touch. `go install ./cmd/fledge` was not
run/reinstalled globally because this feather makes no `commandOrder`,
`cli.go`, or scaffold change (per the spec's Approach section) — `preen` here
was run via `go run ./cmd/fledge preen` against this worktree's own build, so
no stale-binary risk applies.
