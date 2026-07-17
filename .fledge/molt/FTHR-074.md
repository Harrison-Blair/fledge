## AC-1

Tests written first (`internal/cli/verdict_test.go`, `internal/cli/escalate_test.go`,
`internal/cli/ledgerread_test.go`, `cmd/fledge/testdata/verdict.txtar`,
`cmd/fledge/testdata/escalate.txtar`, `cmd/fledge/testdata/ledger-read.txtar`),
then run against the unchanged code (no `verdict`/`escalate`/`ledger`
commands registered yet) to confirm they fail for the expected reason
(`fledge: unknown command "verdict"` / `"escalate"` / `"ledger"`, exit 2).

Command:
```
go test ./internal/cli/... -run 'TestVerdict|TestEscalate|TestLedgerRead' -v
```

Verbatim output (failing, pre-implementation):
```
=== RUN   TestEscalateWritesRecord
fledge: unknown command "escalate"

usage: fledge <command> [args]

commands:
  fledge init [--agent <name>]... [--dev=<path>] [--refresh] [--force] [--list-agents] [--json]
  fledge agents [--json]
  fledge scan [--json]
  fledge new plumage --title <t> [--priority P1] [--agent <s>] [--json]
  fledge new feather --title <t> --plumage PLM-### [--depends-on a,b] [--priority P1] [--oversight merge|during] [--force] [--json]
  fledge nest new <doc> | scaffold | scout --module <m> | stamp <file> | status [flags]
  fledge preen [--strict] [--json]
  fledge ready [--json]
  fledge vee [--format text|dot|json] [--json] [PLM-###]
  fledge colony [--json]
  fledge unfledged [--plumage] [--feathers] [--json]
  fledge status <ID> [<new-status>] [--force] [--json]
  fledge set <ID> <field> <value> [--json]  (fields: priority, oversight, depends_on, title)
  fledge criteria <ID> [--json] | fledge criteria check|uncheck <ID> <AC-N> [--json]
  fledge brood FTHR-### --owner <name> [--branch <b>] [--worktree <path>] [--json]
  fledge abandon FTHR-### [--fledged] [--force] [--json]
  fledge broods [--stale] [--json]
  fledge heartbeat <name> [--note <text>] [--json]
  fledge await <subject> --kind <kind> [--timeout <duration>] [--json]
  fledge roster [--json] | roster assign (--feather FTHR-### [--pair] | --for <purpose>) [--json] | roster release <name> [--json]
  fledge version [--json]
  fledge update [--yes] [--json]
    escalate_test.go:15: escalate exit = 2, want 0 (ExitOK)
--- FAIL: TestEscalateWritesRecord (0.00s)
=== RUN   TestLedgerReadAllKinds
=== RUN   TestLedgerReadAllKinds/status
heartbeat subj-status at 2026-07-17T03:10:24Z
fledge: unknown command "ledger"
...
    ledgerread_test.go:29: ledger read exit = 2, want 0
=== RUN   TestLedgerReadAllKinds/verdict
fledge: unknown command "verdict"
...
    ledgerread_test.go:26: [verdict subj-verdict --result pass --note ok] exit = 2, want 0
=== RUN   TestLedgerReadAllKinds/escalation
fledge: unknown command "escalate"
...
    ledgerread_test.go:26: [escalate subj-escalation --message help] exit = 2, want 0
--- FAIL: TestLedgerReadAllKinds (0.00s)
    --- FAIL: TestLedgerReadAllKinds/status (0.00s)
    --- FAIL: TestLedgerReadAllKinds/verdict (0.00s)
    --- FAIL: TestLedgerReadAllKinds/escalation (0.00s)
=== RUN   TestLedgerReadMissing
fledge: unknown command "ledger"
...
    ledgerread_test.go:40: ledger read exit = 2, want 1 (ExitFail)
--- FAIL: TestLedgerReadMissing (0.00s)
=== RUN   TestVerdictRejectsInvalidResult
fledge: unknown command "verdict"
...
--- PASS: TestVerdictRejectsInvalidResult (0.00s)
=== RUN   TestVerdictWritesRecord
fledge: unknown command "verdict"
...
    verdict_test.go:45: verdict exit = 2, want 0 (ExitOK)
--- FAIL: TestVerdictWritesRecord (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/cli	0.006s
FAIL
```

(Note: `TestVerdictRejectsInvalidResult` passes trivially pre-implementation
because an unregistered command also exits `ExitUsage`(2) — the same code the
real validation must produce. Confirmed separately post-implementation that
it still passes against the real `--result` validation path, and that a
regression there — e.g. accepting `maybe` — is caught, see AC-2 below.)

Command:
```
go test ./cmd/fledge/... -run 'TestScripts/verdict|TestScripts/escalate|TestScripts/ledger-read' -v
```

Verbatim output (failing, pre-implementation):
```
=== RUN   TestScripts
=== RUN   TestScripts/escalate
=== PAUSE TestScripts/escalate
=== RUN   TestScripts/ledger-read
=== PAUSE TestScripts/ledger-read
=== RUN   TestScripts/verdict
=== PAUSE TestScripts/verdict
=== CONT  TestScripts/escalate
=== CONT  TestScripts/verdict
=== CONT  TestScripts/ledger-read
=== NAME  TestScripts/escalate
    ...
        # fledge escalate: write an escalation record to the ledger (0.001s)
        > exec git init -q .
        # happy path: writes one escalation record for the named subject (0.001s)
        > exec fledge escalate some-worker --message 'blocked on ambiguous spec'
        [stderr]
        fledge: unknown command "escalate"
        ...
        [exit status 2]
        FAIL: testdata/escalate.txtar:5: unexpected command failure

=== NAME  TestScripts/verdict
        ...
        # fledge verdict: write a pass/fail verdict record to the ledger (0.001s)
        > exec git init -q .
        # happy path: writes one verdict record for the named subject (0.001s)
        > exec fledge verdict some-review --result pass --note 'looks good'
        [stderr]
        fledge: unknown command "verdict"
        ...
        [exit status 2]
        FAIL: testdata/verdict.txtar:5: unexpected command failure

=== NAME  TestScripts/ledger-read
        ...
        # fledge ledger read: generic reader across all three ledger record kinds (0.001s)
        > exec git init -q .
        # round-trip: write via heartbeat/verdict/escalate, read back via `ledger read` (0.002s)
        > exec fledge heartbeat watcher --note 'running' --json
        [stdout]
        {
          "subject": "watcher",
          "kind": "status",
          "timestamp": "2026-07-17T03:10:24Z",
          "payload": {
            "pid": 1097835,
            "note": "running",
            "updated_at": "2026-07-17T03:10:24Z"
          }
        }
        > exec fledge ledger read watcher --kind status
        [stderr]
        fledge: unknown command "ledger"
        ...
        [exit status 2]
        FAIL: testdata/ledger-read.txtar:6: unexpected command failure

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/escalate (0.00s)
    --- FAIL: TestScripts/verdict (0.00s)
    --- FAIL: TestScripts/ledger-read (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.005s
FAIL
```

All failures are for the expected reason: the `verdict`, `escalate`, and
`ledger` commands do not exist yet on unchanged code.

## AC-2

`fledge verdict <subject> --result pass|fail [--note "<text>"]` (`internal/cli/verdict.go`)
validates `--result` is exactly `pass` or `fail`, rejects anything else with
`ExitUsage` and writes nothing, and writes a `ledger.VerdictRecord{Result, Note}`
for `kind=verdict` on the happy path.

Command:
```
go test ./internal/cli/... -run 'TestVerdict' -v
```
Verbatim output (passing):
```
=== RUN   TestVerdictRejectsInvalidResult
fledge: usage: fledge verdict <subject> --result pass|fail: got --result "maybe"
--- PASS: TestVerdictRejectsInvalidResult (0.00s)
=== RUN   TestVerdictWritesRecord
verdict some-subject: pass at 2026-07-17T03:14:24Z
--- PASS: TestVerdictWritesRecord (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.004s
```

Command:
```
go test ./cmd/fledge/... -run 'TestScripts/verdict' -v
```
Verbatim output (passing):
```
# fledge verdict: write a pass/fail verdict record to the ledger (0.001s)
> exec git init -q .
# happy path: writes one verdict record for the named subject (0.002s)
> exec fledge verdict some-review --result pass --note 'looks good'
[stdout]
verdict some-review: pass at 2026-07-17T03:14:24Z
> exists .fledge/ledger/some-review.verdict.json
> stdout 'some-review'
# --json emits the written record: subject, kind, result, note (0.001s)
> exec fledge verdict some-review --result fail --note 'needs work' --json
[stdout]
{
  "subject": "some-review",
  "kind": "verdict",
  "timestamp": "2026-07-17T03:14:24Z",
  "payload": {
    "result": "fail",
    "note": "needs work"
  }
}
> stdout '"subject": "some-review"'
> stdout '"kind": "verdict"'
> stdout '"result": "fail"'
> stdout '"note": "needs work"'
> stdout '"timestamp": "'
# --note is optional (0.001s)
> exec fledge verdict some-review --result pass --json
[stdout]
{
  "subject": "some-review",
  "kind": "verdict",
  "timestamp": "2026-07-17T03:14:24Z",
  "payload": {
    "result": "pass",
    "note": ""
  }
}
> stdout '"note": ""'
# malformed input: an invalid --result value is a usage error (exit 2) (0.001s)
> ! exec fledge verdict some-review --result maybe
[stderr]
fledge: usage: fledge verdict <subject> --result pass|fail: got --result "maybe"
[exit status 2]
> stderr 'usage'
# malformed input: missing <subject> is a usage error (exit 2) (0.001s)
> ! exec fledge verdict --result pass
[stderr]
fledge: usage: fledge verdict <subject> --result pass|fail [--note <text>]
[exit status 2]
> stderr 'usage'
PASS

--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/verdict (0.01s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.009s
```

## AC-3

`fledge escalate <subject> --message "<text>"` (`internal/cli/escalate.go`)
writes a `ledger.EscalationRecord{Message}` for `kind=escalation`.

Command:
```
go test ./internal/cli/... -run 'TestEscalate' -v
```
Verbatim output (passing):
```
=== RUN   TestEscalateWritesRecord
escalate some-subject at 2026-07-17T03:14:25Z
--- PASS: TestEscalateWritesRecord (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.003s
```

Command:
```
go test ./cmd/fledge/... -run 'TestScripts/escalate' -v
```
Verbatim output (passing):
```
# fledge escalate: write an escalation record to the ledger (0.001s)
> exec git init -q .
# happy path: writes one escalation record for the named subject (0.002s)
> exec fledge escalate some-worker --message 'blocked on ambiguous spec'
[stdout]
escalate some-worker at 2026-07-17T03:14:25Z
> exists .fledge/ledger/some-worker.escalation.json
> stdout 'some-worker'
# --json emits the written record: subject, kind, message (0.001s)
> exec fledge escalate some-worker --message 'blocked on ambiguous spec' --json
[stdout]
{
  "subject": "some-worker",
  "kind": "escalation",
  "timestamp": "2026-07-17T03:14:25Z",
  "payload": {
    "message": "blocked on ambiguous spec"
  }
}
> stdout '"subject": "some-worker"'
> stdout '"kind": "escalation"'
> stdout '"message": "blocked on ambiguous spec"'
> stdout '"timestamp": "'
# malformed input: missing --message is a usage error (exit 2) (0.001s)
> ! exec fledge escalate some-worker
[stderr]
fledge: usage: fledge escalate <subject> --message <text>
[exit status 2]
> stderr 'usage'
# malformed input: missing <subject> is a usage error (exit 2) (0.001s)
> ! exec fledge escalate --message 'x'
[stderr]
fledge: usage: fledge escalate <subject> --message <text>
[exit status 2]
> stderr 'usage'
PASS

--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/escalate (0.01s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.007s
```

## AC-4

`fledge ledger read <subject> --kind status|verdict|escalation` (`internal/cli/ledger.go`,
housing the `ledger read` subcommand per `roster.go`'s dispatch precedent)
reads and prints the record for any of the three kinds, and reports a clean
`ExitFail` ("no status record for ...", `internal/ledger`'s `*NotFoundError`)
for an absent (subject, kind) — no panic.

Command:
```
go test ./internal/cli/... -run 'TestLedgerRead' -v
```
Verbatim output (passing):
```
=== RUN   TestLedgerReadAllKinds
=== RUN   TestLedgerReadAllKinds/status
heartbeat subj-status at 2026-07-17T03:14:25Z
subj-status status at 2026-07-17T03:14:25Z: {"pid":1131700,"note":"hi","updated_at":"2026-07-17T03:14:25Z"}
=== RUN   TestLedgerReadAllKinds/verdict
verdict subj-verdict: pass at 2026-07-17T03:14:25Z
subj-verdict verdict at 2026-07-17T03:14:25Z: {"result":"pass","note":"ok"}
=== RUN   TestLedgerReadAllKinds/escalation
escalate subj-escalation at 2026-07-17T03:14:25Z
subj-escalation escalation at 2026-07-17T03:14:25Z: {"message":"help"}
--- PASS: TestLedgerReadAllKinds (0.00s)
    --- PASS: TestLedgerReadAllKinds/status (0.00s)
    --- PASS: TestLedgerReadAllKinds/verdict (0.00s)
    --- PASS: TestLedgerReadAllKinds/escalation (0.00s)
=== RUN   TestLedgerReadMissing
fledge: no status record for never-written
--- PASS: TestLedgerReadMissing (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.006s
```

Command:
```
go test ./cmd/fledge/... -run 'TestScripts/ledger-read' -v
```
Verbatim output (passing):
```
# fledge ledger read: generic reader across all three ledger record kinds (0.001s)
> exec git init -q .
# round-trip: write via heartbeat/verdict/escalate, read back via `ledger read` (0.008s)
> exec fledge heartbeat watcher --note 'running' --json
[stdout]
{
  "subject": "watcher",
  "kind": "status",
  "timestamp": "2026-07-17T03:14:25Z",
  "payload": {
    "pid": 1131796,
    "note": "running",
    "updated_at": "2026-07-17T03:14:25Z"
  }
}
> exec fledge ledger read watcher --kind status
[stdout]
watcher status at 2026-07-17T03:14:25Z: {"pid":1131796,"note":"running","updated_at":"2026-07-17T03:14:25Z"}
> stdout 'watcher status'
> stdout 'running'
> exec fledge verdict watcher --result pass --note 'ok' --json
[stdout]
{
  "subject": "watcher",
  "kind": "verdict",
  "timestamp": "2026-07-17T03:14:25Z",
  "payload": {
    "result": "pass",
    "note": "ok"
  }
}
> exec fledge ledger read watcher --kind verdict
[stdout]
watcher verdict at 2026-07-17T03:14:25Z: {"result":"pass","note":"ok"}
> stdout 'watcher verdict'
> stdout '"result":"pass"'
> exec fledge escalate watcher --message 'help' --json
[stdout]
{
  "subject": "watcher",
  "kind": "escalation",
  "timestamp": "2026-07-17T03:14:25Z",
  "payload": {
    "message": "help"
  }
}
> exec fledge ledger read watcher --kind escalation
[stdout]
watcher escalation at 2026-07-17T03:14:25Z: {"message":"help"}
> stdout 'watcher escalation'
> stdout 'help'
# --json shape (0.001s)
> exec fledge ledger read watcher --kind status --json
[stdout]
{
  "subject": "watcher",
  "kind": "status",
  "timestamp": "2026-07-17T03:14:25Z",
  "payload": {
    "pid": 1131796,
    "note": "running",
    "updated_at": "2026-07-17T03:14:25Z"
  }
}
> stdout '"subject": "watcher"'
> stdout '"kind": "status"'
# not-found: reading a subject/kind never written reports a clean failure,
# not a panic (0.001s)
> ! exec fledge ledger read never-written --kind status
[stderr]
fledge: no status record for never-written
[exit status 1]
> stderr 'not found|no status record'
# malformed input: missing --kind is a usage error (exit 2) (0.001s)
> ! exec fledge ledger read watcher
[stderr]
fledge: usage: fledge ledger read <subject> --kind status|verdict|escalation
[exit status 2]
> stderr 'usage'
PASS

--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/ledger-read (0.01s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.014s
```

## AC-5

All three commands support `--json`, evidenced directly in the AC-2/AC-3/AC-4
txtar transcripts above: `fledge verdict ... --json`, `fledge escalate ...
--json`, and `fledge ledger read ... --kind status --json` each emit the full
record (`subject`, `kind`, `timestamp`, `payload`) as indented JSON via the
same `emitJSON` helper used by `heartbeat`/`await`.

## AC-6

Command:
```
go vet ./...
go test ./internal/cli/... ./cmd/fledge/...
```
Verbatim output (passing):
```
ok  	github.com/Harrison-Blair/fledge/internal/cli	(cached)
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	1.221s
```

Also ran the repo's full test suite (`go test ./...`) to confirm no
regressions beyond the two packages named in AC-6:
```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.004s
ok  	github.com/Harrison-Blair/fledge/internal/cli	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.135s
ok  	github.com/Harrison-Blair/fledge/internal/ledger	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.008s
```
