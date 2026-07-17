# FTHR-072 evidence

## AC-1

Tests written first (`internal/ledger/ledger_test.go`, `cmd/fledge/testdata/heartbeat.txtar`), run against unchanged code.

### Pre-implementation run (FAILING)

```
$ go test ./internal/ledger/... ./cmd/fledge/...
# github.com/Harrison-Blair/fledge/internal/ledger [github.com/Harrison-Blair/fledge/internal/ledger.test]
internal/ledger/ledger_test.go:20:8: undefined: StatusRecord
internal/ledger/ledger_test.go:21:15: undefined: Write
internal/ledger/ledger_test.go:21:51: undefined: KindStatus
internal/ledger/ledger_test.go:24:14: undefined: Read
internal/ledger/ledger_test.go:24:49: undefined: KindStatus
internal/ledger/ledger_test.go:28:59: undefined: KindStatus
internal/ledger/ledger_test.go:29:79: undefined: KindStatus
internal/ledger/ledger_test.go:34:10: undefined: StatusRecord
internal/ledger/ledger_test.go:47:15: undefined: Write
internal/ledger/ledger_test.go:47:36: undefined: KindStatus
internal/ledger/ledger_test.go:47:36: too many errors
FAIL	github.com/Harrison-Blair/fledge/internal/ledger [build failed]
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/heartbeat (0.00s)
        testscript.go:584: # fledge heartbeat: write/refresh a worker's status record in the ledger (0.001s)
            # happy path: writes one status record for the named subject (0.002s)
            > exec fledge heartbeat fledge-brooder-adelie --note 'running tests'
            [stderr]
            fledge: unknown command "heartbeat"
            
            usage: fledge <command> [args]
            
            commands:
              fledge init [--agent <name>]... [--refresh] [--force] [--list-agents] [--json]
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
              fledge roster [--json] | roster assign (--feather FTHR-### [--pair] | --for <purpose>) [--json] | roster release <name> [--json]
              fledge version [--json]
              fledge update [--yes] [--json]
            [exit status 2]
            FAIL: testdata/heartbeat.txtar:5: unexpected command failure
            
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.091s
FAIL
```

### Post-implementation run (PASSING)

```
$ go test ./internal/ledger/... ./cmd/fledge/...
ok  	github.com/Harrison-Blair/fledge/internal/ledger	0.002s
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.102s
```

Verification that the tests bite (mutation checks; each reverted afterwards):

| Mutation | Result |
| --- | --- |
| `StaleAfter` 5m -> 1h | `TestClassifyLiveness/live_pid,_lease_past_ttl` + `TestStaleAfterIsFiveMinutes` FAIL |
| dead-PID branch disabled | `TestClassifyLiveness/dead_pid,_fresh_lease` FAIL |
| `os.Rename` -> `os.Link` in `Write` | `TestWriteOverwritesPriorRecord`, `TestConcurrentWrites` FAIL |

## AC-2

`internal/ledger` provides atomic `Write`/`Read` for all three kinds (`KindStatus`, `KindVerdict`, `KindEscalation`) at `.fledge/ledger/<subject>.<kind>.json`, latest-value-only (FC-1, FC-2).

```
$ go test -count=1 -race ./internal/ledger/ -run "TestWriteReadRoundtrip|TestWriteOverwritesPriorRecord|TestReadMissingRecord|TestReadCorruptRecord|TestConcurrentWrites|TestWriteAllKinds" -v
=== RUN   TestWriteReadRoundtrip
--- PASS: TestWriteReadRoundtrip (0.00s)
=== RUN   TestWriteOverwritesPriorRecord
--- PASS: TestWriteOverwritesPriorRecord (0.00s)
=== RUN   TestReadMissingRecord
--- PASS: TestReadMissingRecord (0.00s)
=== RUN   TestReadCorruptRecord
=== RUN   TestReadCorruptRecord/not_json
=== RUN   TestReadCorruptRecord/empty
=== RUN   TestReadCorruptRecord/truncated
--- PASS: TestReadCorruptRecord (0.00s)
    --- PASS: TestReadCorruptRecord/not_json (0.00s)
    --- PASS: TestReadCorruptRecord/empty (0.00s)
    --- PASS: TestReadCorruptRecord/truncated (0.00s)
=== RUN   TestConcurrentWrites
--- PASS: TestConcurrentWrites (0.00s)
=== RUN   TestWriteAllKinds
--- PASS: TestWriteAllKinds (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/ledger	1.008s
```

Atomicity: `Write` uses temp-file + `os.Rename` (not `lock.Acquire`'s `os.Link`, which fails EEXIST and cannot overwrite — pinned by mutation M3 above). `TestConcurrentWrites` runs 16 concurrent writers with a reader spinning on the file: no torn/partial read observed, final state is exactly one written value, no leftover temp files.

## AC-3

`fledge heartbeat <name> [--note]` writes/refreshes a `status` record and supports `--json` (FC-3, FC-6).

```
$ go test -count=1 ./cmd/fledge -run TestScripts/heartbeat -v
=== RUN   TestScripts
=== RUN   TestScripts/heartbeat
=== PAUSE TestScripts/heartbeat
=== CONT  TestScripts/heartbeat
    testscript.go:584: WORK=$WORK
        PATH=/tmp/testscript-main2950148487/bin:/usr/lib/go/bin:/home/penguin/.opencode/bin:/home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin:/home/penguin/.nvm/versions/node/v25.9.0/bin:/home/penguin/go/bin:/home/penguin/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/bin:/usr/bin/site_perl:/usr/bin/vendor_perl:/usr/bin/core_perl:/opt/rocm/bin:/usr/lib/rustup/bin
        GOTRACEBACK=system
        HOME=/no-home
        TMPDIR=$WORK/.tmp
        devnull=/dev/null
        /=/
        :=:
        $=$
        exe=
        GIT_CONFIG_GLOBAL=/dev/null
        GIT_CONFIG_SYSTEM=/dev/null
        GIT_AUTHOR_NAME=test
        GIT_AUTHOR_EMAIL=test@example.invalid
        GIT_COMMITTER_NAME=test
        GIT_COMMITTER_EMAIL=test@example.invalid
        
        # fledge heartbeat: write/refresh a worker's status record in the ledger (0.001s)
        > exec git init -q .
        # happy path: writes one status record for the named subject (0.001s)
        > exec fledge heartbeat fledge-brooder-adelie --note 'running tests'
        [stdout]
        heartbeat fledge-brooder-adelie at 2026-07-16T23:51:00Z
        > exists .fledge/ledger/fledge-brooder-adelie.status.json
        > stdout 'fledge-brooder-adelie'
        # --json emits the written record: subject, kind, note and a pid (0.001s)
        > exec fledge heartbeat fledge-brooder-adelie --note 'running tests' --json
        [stdout]
        {
          "subject": "fledge-brooder-adelie",
          "kind": "status",
          "timestamp": "2026-07-16T23:51:00Z",
          "payload": {
            "pid": 585745,
            "note": "running tests",
            "updated_at": "2026-07-16T23:51:00Z"
          }
        }
        > stdout '"subject": "fledge-brooder-adelie"'
        > stdout '"kind": "status"'
        > stdout '"note": "running tests"'
        > stdout '"pid": [0-9]+'
        > stdout '"timestamp": "'
        # --note is optional (0.001s)
        > exec fledge heartbeat fledge-brooder-adelie --json
        [stdout]
        {
          "subject": "fledge-brooder-adelie",
          "kind": "status",
          "timestamp": "2026-07-16T23:51:00Z",
          "payload": {
            "pid": 585753,
            "note": "",
            "updated_at": "2026-07-16T23:51:00Z"
          }
        }
        > stdout '"note": ""'
        # repeated heartbeat refreshes the same record file: no duplicates, and the
        # latest note wins (0.002s)
        > exec fledge heartbeat fledge-brooder-adelie --note 'still going' --json
        [stdout]
        {
          "subject": "fledge-brooder-adelie",
          "kind": "status",
          "timestamp": "2026-07-16T23:51:00Z",
          "payload": {
            "pid": 585761,
            "note": "still going",
            "updated_at": "2026-07-16T23:51:00Z"
          }
        }
        > stdout '"note": "still going"'
        > exec ls .fledge/ledger
        [stdout]
        fledge-brooder-adelie.status.json
        > stdout '^fledge-brooder-adelie.status.json$'
        > ! stdout 'fledge-brooder-adelie.status.json.*fledge-brooder-adelie.status.json'
        # a second subject gets its own record (0.001s)
        > exec fledge heartbeat fledge-skua-adelie --json
        [stdout]
        {
          "subject": "fledge-skua-adelie",
          "kind": "status",
          "timestamp": "2026-07-16T23:51:00Z",
          "payload": {
            "pid": 585770,
            "note": "",
            "updated_at": "2026-07-16T23:51:00Z"
          }
        }
        > stdout '"subject": "fledge-skua-adelie"'
        > exists .fledge/ledger/fledge-skua-adelie.status.json
        > exists .fledge/ledger/fledge-brooder-adelie.status.json
        # malformed input: missing <name> is a usage error (exit 2) (0.001s)
        > ! exec fledge heartbeat
        [stderr]
        fledge: usage: fledge heartbeat <name> [--note <text>]
        [exit status 2]
        > stderr 'usage'
        # too many positionals is a usage error too (0.001s)
        > ! exec fledge heartbeat one two
        [stderr]
        fledge: usage: fledge heartbeat <name> [--note <text>]
        [exit status 2]
        > stderr 'usage'
        PASS
        
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/heartbeat (0.01s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.011s
```

## AC-4

`ClassifyLiveness` catches both failure directions against the fixed 5-minute TTL (FC-4, PLM-030 AC-4). Pure function: no ledger I/O, so both directions unit-test directly.

```
$ go test -count=1 ./internal/ledger/ -run "TestClassifyLiveness|TestStaleAfterIsFiveMinutes" -v
=== RUN   TestClassifyLiveness
=== RUN   TestClassifyLiveness/dead_pid,_fresh_lease
=== RUN   TestClassifyLiveness/dead_pid,_stale_lease
=== RUN   TestClassifyLiveness/live_pid,_fresh_lease
=== RUN   TestClassifyLiveness/live_pid,_lease_just_inside_ttl
=== RUN   TestClassifyLiveness/live_pid,_lease_past_ttl
=== RUN   TestClassifyLiveness/live_pid,_lease_far_past_ttl
--- PASS: TestClassifyLiveness (0.00s)
    --- PASS: TestClassifyLiveness/dead_pid,_fresh_lease (0.00s)
    --- PASS: TestClassifyLiveness/dead_pid,_stale_lease (0.00s)
    --- PASS: TestClassifyLiveness/live_pid,_fresh_lease (0.00s)
    --- PASS: TestClassifyLiveness/live_pid,_lease_just_inside_ttl (0.00s)
    --- PASS: TestClassifyLiveness/live_pid,_lease_past_ttl (0.00s)
    --- PASS: TestClassifyLiveness/live_pid,_lease_far_past_ttl (0.00s)
=== RUN   TestStaleAfterIsFiveMinutes
--- PASS: TestStaleAfterIsFiveMinutes (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/ledger	0.001s
```

## AC-5

```
$ go test -race -count=1 ./internal/ledger/... ./cmd/fledge/...
ok  	github.com/Harrison-Blair/fledge/internal/ledger	1.007s
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	37.317s
$ go vet ./...
$ gofmt -l .
(no output from vet/gofmt = clean)
```

Full repo suite also green:

```
$ go test ./... 2>&1 | grep -v "no test files"
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/check	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/cli	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/doctest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/graph	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/ledger	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/lock	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/nest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/repo	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/roster	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/scan	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/spec	(cached)
```

---

# Review cycle 1 — Finding 1: subject validation

Skua finding: `recordPath` never validated `subject`, so `Write` could resolve outside the ledger dir (`fledge heartbeat ../../escaped-by-cli` exited 0 and wrote outside `.fledge/ledger/`). Substantiates **AC-2** (FC-1 address space: a record always lives at `.fledge/ledger/<subject>.<kind>.json`) and **AC-3** (invalid input is `ExitUsage`).

Fix: `validSubject` rejects empty subjects, any subject containing `/` or `\`, and the `.`/`..` path elements, with a typed `*InvalidSubjectError`. Applied in **both** `Write` and `Read` so the guarantee holds for every future caller (FTHR-073 `await` reads through the same address space), not just `heartbeat`. Rejects, never sanitizes — a repaired name would silently address a different record than the caller asked for. `heartbeat` maps the typed error to `ExitUsage`.

### Tests added (written first, run against the unfixed code)

`TestWriteRejectsInvalidSubject` (9 subjects incl. `..`, `../escaped`, `a/b`, `/abs`, `a\b`, `""`; asserts no file is created *anywhere* under the temp root, not merely in the ledger dir), `TestReadRejectsInvalidSubject`, `TestValidSubjectsAccepted` (guards the rejection from over-reach: real worker names like `fledge-brooder-adelie`, `a.b` still work), plus `heartbeat.txtar` cases asserting exit 2 and that nothing is written.

Pre-fix run — the txtar reproduces the finding behaviorally (not just a build error):

```
$ go test ./cmd/fledge -run TestScripts/heartbeat   # against the unfixed code
    # a subject that would escape the ledger directory is rejected, not sanitized,
    # and writes nothing anywhere (0.001s)
    > ! exec fledge heartbeat ../escape
    [stdout]
    heartbeat ../escape at 2026-07-16T23:56:50Z
    FAIL: testdata/heartbeat.txtar:41: unexpected command success

$ go test ./internal/ledger/ -run Subject           # against the unfixed code
internal/ledger/ledger_test.go:94:13: undefined: InvalidSubjectError
FAIL	github.com/Harrison-Blair/fledge/internal/ledger [build failed]
```

### The skua's own CLI repro, re-run against the fix

```
$ fledge heartbeat '../../escaped-by-cli' --note pwn
fledge: invalid ledger subject "../../escaped-by-cli": must not contain a path separator ("/" or "\")
exit=2
$ fledge heartbeat '' --json
fledge: invalid ledger subject "": must not be empty
exit=2
$ fledge heartbeat 'a/b'
fledge: invalid ledger subject "a/b": must not contain a path separator ("/" or "\")
exit=2
$ find /tmp/hbrepo -name '*.status.json'
NONE (correct)
$ fledge heartbeat fledge-brooder-emperor --note 'still works'
heartbeat fledge-brooder-emperor at 2026-07-16T23:58:17Z
exit=0
$ find /tmp/hbrepo -name '*.status.json'
/tmp/hbrepo/.fledge/ledger/fledge-brooder-emperor.status.json
```

### Mutation checks on the new guard (each reverted)

| Mutation | Result |
| --- | --- |
| `validSubject` call removed from `Write` | `TestWriteRejectsInvalidSubject` FAILs (`want *InvalidSubjectError, got <nil>`) |
| separator check neutralized | `TestWriteRejectsInvalidSubject/{deep_traversal,leading_slash,backslash}` FAIL |

Note on process: the separator mutation first reported `ok`, which was a **false negative** — the `perl` substitution had silently failed to match, so the mutation was never applied. Re-run with the edit confirmed applied (`grep` on the mutated line), it fails as expected. An unverified mutation check proves nothing; the table above reflects only mutations verified present in the source.

### Full verification after the fix

```
$ go test -race -count=1 ./internal/ledger/... ./cmd/fledge/...
ok  	github.com/Harrison-Blair/fledge/internal/ledger	1.009s
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	37.330s
$ go test ./... 2>&1 | grep -v "no test files"
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/check	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/cli	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/doctest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/graph	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/ledger	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/lock	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/nest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/repo	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/roster	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/scan	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/spec	(cached)
$ go vet ./... && gofmt -l .
(clean)
```
