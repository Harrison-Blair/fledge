# FTHR-089 evidence: Lease-only liveness with declared quiet periods

## AC-1

Tests written first (`cmd/fledge/testdata/heartbeat.txtar` extended, then
`internal/ledger/ledger_test.go` rewritten), run against the unchanged code,
observed failing for the expected reason, captured below. Passing
(post-implementation) runs follow each failing capture.

### Pre-implementation: `heartbeat.txtar`, no-PID assertion (behavioral)

```
$ go test ./cmd/fledge -run TestScripts/heartbeat -v
...
        # --json emits the written record: subject, kind, note, and no pid (0.001s)
        > exec fledge heartbeat fledge-brooder-adelie --note 'running tests' --json
        [stdout]
        {
          "subject": "fledge-brooder-adelie",
          "kind": "status",
          "timestamp": "2026-07-17T08:20:16Z",
          "payload": {
            "pid": 392495,
            "note": "running tests",
            "updated_at": "2026-07-17T08:20:16Z"
          }
        }
        > stdout '"subject": "fledge-brooder-adelie"'
        > stdout '"kind": "status"'
        > stdout '"note": "running tests"'
        > ! stdout '"pid"'
        FAIL: testdata/heartbeat.txtar:14: unexpected match for `"pid"` found in stdout: "pid"

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/heartbeat (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.005s
```

testscript aborts a script at its first failing assertion, so the later
`--expect` lines in the same file were not reached by this run; the flag's
own behavioral failure is captured directly below against the pre-change
binary (same code, same defect, no compile step involved).

### Pre-implementation: `--expect` flag does not exist (AC-2 behavioral evidence)

```
$ go build -o /tmp/fledge-pre ./cmd/fledge
$ /tmp/fledge-pre heartbeat w --expect 12m --json
flag provided but not defined: -expect
Usage of heartbeat:
  -json
    	machine-readable output
  -note string
    	short free-text status note
exit=2
```

### Pre-implementation: `TestClassifyLiveness` fresh-lease case (AC-3), old signature

A probe against the unchanged 3-arg `ClassifyLiveness` (kept compiling on
purpose, to keep this a behavioral failure rather than a build break),
reproducing PLM-035's measured defect: a dead PID (the ephemeral
`fledge`-CLI-process pattern) with a 2-second-old lease is classified stalled
today.

```go
func TestAC3Probe_FreshLeaseNotStalled(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	stalled, reason := ClassifyLiveness(deadPID, now.Add(-2*time.Second), now)
	if stalled {
		t.Errorf("ClassifyLiveness(dead pid, 2s-old lease) stalled = true (reason %q), want false: a 2-second-old lease must not classify stalled", reason)
	}
}
```

```
$ go test ./internal/ledger/... -run TestAC3Probe -v
=== RUN   TestAC3Probe_FreshLeaseNotStalled
    zz_ac3_probe_test.go:17: ClassifyLiveness(dead pid, 2s-old lease) stalled = true (reason "pid 2147483647 is not alive"), want false: a 2-second-old lease must not classify stalled
--- FAIL: TestAC3Probe_FreshLeaseNotStalled (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/ledger	0.001s
```

The probe file was scratch, removed once `TestClassifyLiveness` was rewritten
to the new 3-arg `(lastUpdated, expect, now)` signature, which subsumes it
(the "fresh lease, default expect" row below) and adds the 6-minute/30m-declared
case that is the plumage's whole point.

### Post-implementation: all pass

```
$ go test ./cmd/fledge -run TestScripts/heartbeat -v
...
        # --json emits the written record: subject, kind, note, and no pid (0.001s)
        > exec fledge heartbeat fledge-brooder-adelie --note 'running tests' --json
        [stdout]
        {
          "subject": "fledge-brooder-adelie",
          "kind": "status",
          "timestamp": "2026-07-17T08:23:50Z",
          "payload": {
            "note": "running tests",
            "expect": "5m0s",
            "updated_at": "2026-07-17T08:23:50Z"
          }
        }
        > stdout '"subject": "fledge-brooder-adelie"'
        > stdout '"kind": "status"'
        > stdout '"note": "running tests"'
        > ! stdout '"pid"'
        > stdout '"timestamp": "'
        # --expect declares a quiet period, stored as a duration (0.001s)
        > exec fledge heartbeat fledge-brooder-adelie --expect 12m --json
        [stdout]
        {
          "subject": "fledge-brooder-adelie",
          "kind": "status",
          "timestamp": "2026-07-17T08:23:50Z",
          "payload": {
            "note": "",
            "expect": "12m",
            "updated_at": "2026-07-17T08:23:50Z"
          }
        }
        > stdout '"expect": "12m"'
        # --expect omitted defaults to the 5m StaleAfter default (0.001s)
        > exec fledge heartbeat fledge-brooder-adelie --json
        [stdout]
        {
          "subject": "fledge-brooder-adelie",
          "kind": "status",
          "timestamp": "2026-07-17T08:23:50Z",
          "payload": {
            "note": "",
            "expect": "5m0s",
            "updated_at": "2026-07-17T08:23:50Z"
          }
        }
        > stdout '"expect": "5m0s"'
        # an unparseable --expect is a usage error naming the flag (0.001s)
        > ! exec fledge heartbeat fledge-brooder-adelie --expect nonsense
        [stderr]
        fledge: invalid --expect "nonsense": time: invalid duration "nonsense"
        [exit status 2]
        > stderr 'expect'
        # no cap: a declared period well beyond the default is accepted (0.001s)
        > exec fledge heartbeat fledge-brooder-adelie --expect 90m --json
        [stdout]
        {
          "subject": "fledge-brooder-adelie",
          "kind": "status",
          "timestamp": "2026-07-17T08:23:50Z",
          "payload": {
            "note": "",
            "expect": "90m",
            "updated_at": "2026-07-17T08:23:50Z"
          }
        }
        > stdout '"expect": "90m"'
        ...
        PASS
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/heartbeat (0.02s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	(cached)

$ go test ./internal/ledger -run TestClassifyLiveness -v
=== RUN   TestClassifyLiveness
=== RUN   TestClassifyLiveness/fresh_lease,_default_expect
=== RUN   TestClassifyLiveness/lease_just_inside_default_expect
=== RUN   TestClassifyLiveness/lease_past_default_expect
=== RUN   TestClassifyLiveness/lease_far_past_default_expect
=== RUN   TestClassifyLiveness/lease_past_old_ttl_but_inside_a_long_declared_period
=== RUN   TestClassifyLiveness/lease_past_a_long_declared_period
--- PASS: TestClassifyLiveness (0.00s)
    --- PASS: TestClassifyLiveness/fresh_lease,_default_expect (0.00s)
    --- PASS: TestClassifyLiveness/lease_just_inside_default_expect (0.00s)
    --- PASS: TestClassifyLiveness/lease_past_default_expect (0.00s)
    --- PASS: TestClassifyLiveness/lease_far_past_default_expect (0.00s)
    --- PASS: TestClassifyLiveness/lease_past_old_ttl_but_inside_a_long_declared_period (0.00s)
    --- PASS: TestClassifyLiveness/lease_past_a_long_declared_period (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/ledger	(cached)
```

## AC-2

The `--expect` capture above — `flag provided but not defined: -expect` —
was observed by running the **built binary**, not a `go build`/compile
failure. This satisfies the requirement (PLM-035 AC-2, this feather's AC-2)
to avoid the FTHR-088 vacuous-build-break trap the feather spec and PLM-035's
Context both call out: the classifier's signature changed twice in this
feather, and a unit-test-first approach against the package would have
produced only compile errors, proving nothing behavioral.

## AC-3

Pre-implementation probe (see AC-1 above) proves the exact misclassification
PLM-035 measures: a dead PID with a 2-second-old lease classified `stalled`
under the old code. Post-implementation, `TestClassifyLiveness/fresh_lease,_default_expect`
(`now.Add(-time.Second)`, `expect=StaleAfter`) asserts `stalled=false` and
passes — satisfies FC-1 / AC-3.

## AC-4

`StatusRecord` (`internal/ledger/ledger.go`) has no `PID` field:

```go
type StatusRecord struct {
	Note      string `json:"note"`
	Expect    string `json:"expect"`
	UpdatedAt string `json:"updated_at"`
}
```

`heartbeat.txtar`'s `! stdout '"pid"'` assertion (both the raw JSON and the
human-readable path — human output is `heartbeat <name> at <timestamp>`,
which was already PID-free and unchanged) passes post-implementation, per
the full run captured under AC-1. Satisfies FC-2 and the status-record half
of PLM-035 AC-4 — FTHR-090 closes the feather-claim half; neither feather
closes PLM-035 AC-4 alone.

## AC-5

`ClassifyLiveness`'s signature (`internal/ledger/ledger.go`) takes no PID
parameter:

```go
func ClassifyLiveness(lastUpdated time.Time, expect time.Duration, now time.Time) (stalled bool, reason string)
```

`pidAlive` and its `syscall` import are deleted from `internal/ledger`
entirely (confirmed: `grep -rn "PID\|pidAlive\|syscall" internal/ledger/
internal/cli/heartbeat.go` returns only doc-comment prose describing the
removal, no code). Satisfies FC-1.

## AC-6

`TestClassifyLiveness/lease_past_old_ttl_but_inside_a_long_declared_period`
(`lastUpdated = now.Add(-6*time.Minute)`, `expect = 30*time.Minute`) asserts
`stalled = false` — past the old fixed 5-minute threshold, not stalled
because it declared 30m. This is the case PLM-035's Context and this
feather's spec both call "impossible today... not on a technicality but
because the design cannot express the concept."
`TestClassifyLiveness/lease_past_a_long_declared_period`
(`lastUpdated = now.Add(-31*time.Minute)`, `expect = 30*time.Minute`) asserts
`stalled = true` once that same declared period elapses. Both directions
pass (see the AC-1 post-implementation run). Satisfies FC-3, FC-5, AC-5.

## AC-7

`TestClassifyLiveness/lease_past_default_expect`
(`lastUpdated = now.Add(-6*time.Minute)`, `expect = StaleAfter`) asserts
`stalled = true` for a lease that declares nothing (the default), exactly
matching pre-PLM-035 behavior for every existing call site. Passes (AC-1
post-implementation run). Satisfies FC-3, AC-6.

## AC-8

`StatusRecord.Expect` (a duration string, e.g. `"12m"` or the default
`"5m0s"`) sits alongside `StatusRecord.UpdatedAt` — both are plain JSON
fields on the same record, no absolute-deadline field introduced. Confirmed
readable end-to-end by the `heartbeat.txtar` `--json` captures under AC-1
(`"expect": "12m"`, `"updated_at": "2026-07-17T08:23:50Z"` both present in
the same payload). Satisfies FC-4, AC-7.

## AC-9

`heartbeat.txtar`'s no-cap case: `fledge heartbeat fledge-brooder-adelie
--expect 90m --json` succeeds and stores `"expect": "90m"` — well beyond the
5-minute default, accepted without clamping or rejection (captured under
AC-1). Satisfies FC-6.

## AC-10

`heartbeat.txtar`'s usage-error case: `fledge heartbeat fledge-brooder-adelie
--expect nonsense` exits with `[exit status 2]` and stderr `invalid --expect
"nonsense": time: invalid duration "nonsense"` — names the flag, exits
`ExitUsage` (2) (captured under AC-1).

## AC-11

```
$ go build ./...      # clean
$ go vet ./...         # clean
$ gofmt -l .            # no output — clean
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.429s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.021s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.127s
ok  	github.com/Harrison-Blair/fledge/internal/ledger	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.012s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.014s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s

$ go build -o /tmp/fledge-post ./cmd/fledge && /tmp/fledge-post preen
WARN  .fledge/pluma/feathers/FTHR-061-...: checked criteria missing evidence sections ...
1 warning(s)
```

The single `preen` warning is pre-existing (FTHR-061's evidence file, an
unrelated feather never touched by this branch) and reproduces identically
on `main` — not introduced by this feather. `go test ./...` is green, `go
vet ./...` and `gofmt -l .` are clean. Satisfies AC-11 / PLM-035 AC-13.
