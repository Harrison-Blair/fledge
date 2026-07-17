# FTHR-092 evidence

## AC-1

Tests were written before implementation and observed failing, then made to
pass. See AC-2 for the behavioral pre-implementation failure. Post-
implementation, the full command:

```
$ go test ./cmd/fledge -run TestScripts/pulse -v
$ go test ./internal/cli -run TestPulse -v
$ go test ./...
```

all pass — captured below under AC-2/AC-3/etc. and in the final `go test
./...` run under AC-11.

## AC-2

`cmd/fledge/testdata/pulse.txtar` was written first (before `internal/cli/pulse.go`
existed) and run against the pre-implementation binary. `pulse` was not yet a
registered command, so the failure is **behavioral** — an unknown-command
error surfaced through the CLI dispatch, not a compile error:

```
$ go build -o fledge ./cmd/fledge
$ go test ./cmd/fledge -run TestScripts/pulse -v
...
        > exec fledge pulse watcher-subject
        [stderr]
        fledge: unknown command "pulse"

        usage: fledge <command> [args]
        ...
        [exit status 2]
        FAIL: testdata/pulse.txtar:6: unexpected command failure

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/pulse (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.005s
```

This is the verbatim captured output at the time `pulse.txtar` was first run,
before any of `internal/cli/pulse.go`, `pulse_test.go`, or the `commandOrder`
registration existed.

Post-implementation, the same script passes:

```
$ go test ./cmd/fledge -run TestScripts/pulse -v
--- PASS: TestScripts (...)
    --- PASS: TestScripts/pulse (...)
PASS
```

(Full output captured under AC-3 below.)

## AC-3

`fledge pulse <name>` reports `stalled` and `reason` mirroring
`ledger.ClassifyLiveness`'s return, in both human-readable and `--json`
output. From `pulse.txtar`:

```
exec fledge pulse watcher-subject
stdout 'stalled=false'

exec fledge pulse watcher-subject --json
stdout '"name": "watcher-subject"'
stdout '"stalled": false'
stdout '"reason": "'
```

`internal/cli/pulse_test.go`'s `TestClassifyPulse` asserts `reason` is
exactly `ledger.ClassifyLiveness`'s returned string (not reworded), for both
the stalled and not-stalled cases.

## AC-4

The output includes the declared quiet period (`expect`) and the elapsed
time (`elapsed`) against it, both present in `--json` output:

```
exec fledge pulse watcher-subject --json
stdout '"expect": "5m0s"'
stdout '"elapsed": "'
```

`TestClassifyPulse` in `pulse_test.go` asserts both `report.Expect` and
`report.Elapsed` are non-empty and reflect the input record.

## AC-5

A worker with no status record reports a distinct state: `stalled: false`
with a reason naming the absence, and exit 0:

```
exec fledge pulse nobody-yet --json
stdout '"stalled": false'
stdout '"reason": "no status record'
```

`TestClassifyPulseNoRecord` in `pulse_test.go` pins this directly against
`classifyPulse(name, nil, now)` — the no-record path has no
`ClassifyLiveness` coverage by construction, so it's tested at the
`internal/cli` glue layer.

## AC-6

`fledge pulse` on a stalled worker exits `ExitOK` (0):

```
cp aged-stalled.json .fledge/ledger/gone-quiet.status.json
exec fledge pulse gone-quiet --json
stdout '"stalled": true'
exec sh -c 'fledge pulse gone-quiet --json; test $? -eq 0'
```

The lease is aged via a back-dated `updated_at` (`2024-01-01T00:00:00Z`)
written directly into the ledger file — no sleeping.

## AC-7

A worker whose lease declared a period (`438000h`, 50 years) far longer than
the default (5m) reports **not stalled** even though its lease is years old
— well past the old 5-minute threshold, impossible before PLM-035:

```
cp aged-not-stalled.json .fledge/ledger/long-runner.status.json
exec fledge pulse long-runner --json
stdout '"stalled": false'
stdout '"expect": "438000h"'
```

The CLI acceptance test uses a fixed far-past timestamp with a deliberately
huge declared period so the assertion holds regardless of when the suite
runs (no wall-clock dependency, no sleeping). The precise bounded case (a
30m-declared lease aged 10m, real-world scale) is covered by
`TestClassifyPulseNotStalledPastOldThreshold` in `pulse_test.go`, which
passes an explicit `now` to `classifyPulse` rather than touching the wall
clock, per the `awaitClock` injectable-time convention:

```
$ go test ./internal/cli -run TestClassifyPulseNotStalledPastOldThreshold -v
--- PASS: TestClassifyPulseNotStalledPastOldThreshold (0.00s)
PASS
```

## AC-8

`pulse` contains no liveness logic of its own. `internal/cli/pulse.go`'s
`classifyPulse` calls `ledger.ClassifyLiveness` for the actual stalled/reason
decision; it does no staleness arithmetic itself beyond decoding the record
and computing `elapsed` for display (`now.Sub(updatedAt)`, a straight
subtraction for reporting, not a threshold comparison). `internal/ledger` is
unmodified by this feather (`git diff main -- internal/ledger` is empty).

## AC-9

`--json` is supported:

```
exec fledge pulse watcher-subject --json
```

throughout `pulse.txtar`.

## AC-10

A missing name is `ExitUsage` (2), and an escaping subject is rejected:

```
! exec fledge pulse
stderr 'usage'

! exec fledge pulse ../escape
stderr 'invalid'
! exec fledge pulse a/b
stderr 'invalid'
```

## AC-11

```
$ go build -o fledge ./cmd/fledge && go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.430s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.013s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.041s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.178s
ok  	github.com/Harrison-Blair/fledge/internal/ledger	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.012s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.019s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.010s

$ go vet ./...
(no output, exit 0)

$ gofmt -l .
(no output, exit 0)

$ ./fledge preen
WARN  .fledge/pluma/feathers/FTHR-061-...md: checked criteria missing evidence
sections in .../.fledge/molt/FTHR-061.md: AC-1..AC-5 (heading must be the bare
form "## AC-N")
1 warning(s)
(exit 0 — this warning is pre-existing, unrelated to FTHR-092: FTHR-061 is a
different, already-fledged feather whose evidence file predates this
worktree; `pulse` introduces no new preen errors or warnings)
```
