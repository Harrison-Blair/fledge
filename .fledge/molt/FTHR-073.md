# FTHR-073 evidence: fledge await command

## AC-1

Tests written first (`internal/cli/await_test.go`, `cmd/fledge/testdata/await.txtar`),
run against the unmodified codebase (no `await.go`, no `ExitTimeout`), captured
failing for the expected reason (undefined symbols / unknown command).

### Unit tests — failing, pre-implementation

Command: `go test ./internal/cli/... -run 'TestAwait' -v`

```
# github.com/Harrison-Blair/fledge/internal/cli [github.com/Harrison-Blair/fledge/internal/cli.test]
internal/cli/await_test.go:13:20: undefined: awaitClock
internal/cli/await_test.go:15:12: undefined: awaitClock
internal/cli/await_test.go:35:17: undefined: pollAwait
internal/cli/await_test.go:63:17: undefined: pollAwait
internal/cli/await_test.go:86:17: undefined: pollAwait
FAIL	github.com/Harrison-Blair/fledge/internal/cli [build failed]
FAIL
```

### txtar test — failing, pre-implementation

Command: `go test ./cmd/fledge -run 'TestScripts/await' -v`

```
=== RUN   TestScripts
=== RUN   TestScripts/await
=== PAUSE TestScripts/await
=== CONT  TestScripts/await
    testscript.go:584: WORK=$WORK
        ...
        # fledge await: block until a ledger record appears/changes, or --timeout elapses (0.002s)
        > exec git init -q .
        # happy path: await is started against a subject with no record yet, then a
        # heartbeat writes one; await observes the appearance and returns it (0.003s)
        > exec fledge await watcher-subject --kind status --json &await
        [stderr]
        fledge: unknown command "await"

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
          fledge roster [--json] | roster assign (--feather FTHR-### [--pair] | --for <purpose>) [--json] | roster release <name> [--json]
          fledge version [--json]
          fledge update [--yes] [--json]
        [exit status 2]
        FAIL: testdata/await.txtar:6: unexpected command failure

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/await (0.01s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.008s
FAIL
```

Both fail for the expected reason: `await` does not exist yet. Passing runs
recorded below once implemented.

### Passing, post-implementation

Command: `go test ./internal/cli/... -run 'TestAwait' -v`

```
=== RUN   TestAwaitReturnsOnAppearance
--- PASS: TestAwaitReturnsOnAppearance (0.00s)
=== RUN   TestAwaitReturnsOnChange
--- PASS: TestAwaitReturnsOnChange (0.00s)
=== RUN   TestAwaitTimesOutNoChange
--- PASS: TestAwaitTimesOutNoChange (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.001s
```

Command: `go test ./cmd/fledge -run 'TestScripts/await' -v`

```
=== RUN   TestScripts
=== RUN   TestScripts/await
=== PAUSE TestScripts/await
=== CONT  TestScripts/await
        ...
        # fledge await: block until a ledger record appears/changes, or --timeout elapses (0.001s)
        > exec git init -q .
        # happy path: await is started against a subject with no record yet, then a
        # heartbeat writes one; await observes the appearance and returns it (1.002s)
        > exec fledge await watcher-subject --kind status --json &
        > exec fledge heartbeat watcher-subject --note 'now running'
        [stdout]
        heartbeat watcher-subject at 2026-07-17T03:04:45Z
        > wait
        [background] fledge await watcher-subject --kind status --json: exit status 0
        [stdout]
        {
          "record": {
            "subject": "watcher-subject",
            "kind": "status",
            "timestamp": "2026-07-17T03:04:45Z",
            "payload": {
              "pid": 1028966,
              "note": "now running",
              "updated_at": "2026-07-17T03:04:45Z"
            }
          }
        }
        > stdout '"subject": "watcher-subject"'
        > stdout '"note": "now running"'
        > ! stdout '"timed_out"'
        # real-elapsed-time timeout: no record is ever written for this subject, so
        # await must block until --timeout elapses and exit the new ExitTimeout code
        # (4, distinct from ExitFail/1) — asserted precisely via a shell exit check
        # since testscript's `!` prefix only distinguishes zero from non-zero. (0.203s)
        > exec sh -c 'fledge await never-appears --kind status --timeout 200ms --json; test $? -eq 4'
        [stdout]
        {
          "record": null,
          "timed_out": true
        }
        > stdout '"timed_out": true'
        > stdout '"record": null'
        # malformed input: missing --kind is a usage error (exit 2) (0.001s)
        > ! exec fledge await some-subject
        [stderr]
        fledge: usage: fledge await <subject> --kind <kind> [--timeout <duration>]
        [exit status 2]
        > stderr 'usage'
        PASS

--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/await (1.21s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	1.209s
```

The real-elapsed-time timeout step (line with `--timeout 200ms`) took **0.203s**
wall-clock — a genuine, unmocked sleep bounded tightly to the requested
timeout, not a fallback to the 1s default poll interval.

## AC-2

`fledge await <subject> --kind <kind> [--timeout <duration>]` blocks until the
target record appears or changes, or `--timeout` elapses, satisfying PLM-030
FC-5. Implemented in `internal/cli/await.go`:
- `pollAwait` (the polling loop, unit-tested — see AC-1 passing run) reads a
  baseline via `ledger.Read` at call time (absent, or a payload hash), then
  polls on a fixed 1s interval (`awaitPollInterval`, matching the plumage's
  1-2s guidance), returning as soon as the record newly appears or its
  payload differs from the baseline.
- No `--timeout` given → `hasTimeout=false`, the loop never checks a
  deadline and polls indefinitely (see the happy-path txtar step above: the
  background `await` call with no `--timeout` blocked until the `heartbeat`
  call wrote the record, then returned exit 0).
- `--timeout` given → the loop checks the deadline each tick and returns
  `timedOut: true` once elapsed, sleeping only the remaining time on the
  final tick (not the full 1s) so the wall-clock timeout test above
  completes in ~0.2s rather than ~1s.

Evidence: the AC-1 passing unit-test run above (`TestAwaitReturnsOnAppearance`,
`TestAwaitReturnsOnChange`) and the txtar happy-path step (background await +
heartbeat + wait, exit 0, record observed).

## AC-3

The timeout path exits the new dedicated `ExitTimeout` code (4, distinct from
`ExitFail`=1), added in `internal/cli/cli.go`:

```go
const (
	ExitOK      = 0 // success
	ExitFail    = 1 // domain failure: check findings, lock held, illegal transition, cycle
	ExitUsage   = 2 // usage error
	ExitEnv     = 3 // environment error: not a git repo, no .fledge/ where required
	ExitTimeout = 4 // fledge await: --timeout elapsed with no appearance/change
)
```

Proven two ways, per the spec's two-layer requirement:
- **Fake-clock unit test** (fast, logic-level): `TestAwaitTimesOutNoChange` in
  `internal/cli/await_test.go` — injects an `awaitClock` whose `sleep`
  advances a fake `now` with no real delay, asserting `pollAwait` returns
  `timedOut: true` once the fake clock crosses the deadline. See AC-1 passing
  run above.
- **Real-elapsed-time txtar test**: `cmd/fledge/testdata/await.txtar`'s
  second step invokes `fledge await never-appears --kind status --timeout
  200ms --json` against a subject with no record ever written, and asserts
  the process's real exit code equals 4 via `sh -c '...; test $? -eq 4'`
  (testscript's `!`/no-`!` prefix only distinguishes zero/non-zero, not a
  specific code, hence the shell wrapper). Measured wall-clock for that step
  was **0.203s** — see the AC-1 passing txtar run above — a genuine sleep,
  not a mocked one.

## AC-4

`fledge await --json` output always includes the record (or `null`), and an
explicit `"timed_out": true` field on the timeout path; `timed_out` is
omitted (via `json:"timed_out,omitempty"`) on the success path, per this
codebase's `omitempty` convention (matches `heartbeat`'s use of the same
tag style). Implemented as `awaitEnvelope` in `internal/cli/await.go`:

```go
type awaitEnvelope struct {
	Record   *ledger.Record `json:"record"`
	TimedOut bool           `json:"timed_out,omitempty"`
}
```

Evidence: the AC-1 passing txtar run above —
- happy path: `stdout '"subject": "watcher-subject"'`, `stdout '"note": "now
  running"'`, and `! stdout '"timed_out"'` (field omitted on success) all
  pass.
- timeout path: `stdout '"timed_out": true'` and `stdout '"record": null'`
  both pass.

## AC-5

`go test ./internal/cli/... ./cmd/fledge/...` passes:

```
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.014s
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	1.236s
```

Also verified `go build ./...`, `go vet ./...`, and the full `go test ./...`
suite (all packages) pass with these changes — see commit history for the
run.
