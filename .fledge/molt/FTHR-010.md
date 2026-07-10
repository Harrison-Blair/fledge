# FTHR-010 Evidence

## AC-1

### Pre-implementation: tests failing (required — captured before any code changes)

```
$ cd /home/penguin/source/fledge/.fledge/burrows/FTHR-010 && go test ./cmd/fledge -run TestScripts/stamp_warning -v
=== RUN   TestScripts
=== RUN   TestScripts/stamp_warning
=== PAUSE TestScripts/stamp_warning
=== CONT  TestScripts/stamp_warning
    testscript.go:584: WORK=$WORK
        PATH=...
        GIT_CONFIG_GLOBAL=/dev/null
        ...

        > exec git init -q .
        # preen with mismatched stamp (0.0.1 vs binary 0.2.1) → exit 0, one-line stderr warning (0.001s)
        > exec fledge preen
        [stdout]
        spec clean: 0 plumages, 0 feathers
        > stderr 'fledge: scaffold was written by fledge 0\.0\.1, binary is 0\.2\.1 — run .fledge init --refresh.'
        FAIL: testdata/stamp_warning.txtar:9: no match for `fledge: scaffold was written by fledge 0\.0\.1, binary is 0\.2\.1 — run .fledge init --refresh.` found in stderr

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/stamp_warning (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.003s
```

Test fails because the mismatch warning is never emitted — `Run()` in `internal/cli/cli.go` dispatches commands without any stamp check. This is the expected pre-implementation failure.

### Post-implementation: tests passing

```
$ go test ./cmd/fledge -run TestScripts/stamp_warning -v
=== RUN   TestScripts
=== RUN   TestScripts/stamp_warning
...
        > exec fledge preen
        [stdout]
        spec clean: 0 plumages, 0 feathers
        [stderr]
        fledge: scaffold was written by fledge 0.0.1, binary is 0.2.1 — run 'fledge init --refresh'
        > stderr 'fledge: scaffold was written by fledge 0\.0\.1, binary is 0\.2\.1 — run .fledge init --refresh.'
        > stdout 'spec clean'
        ...
        > exec fledge version
        [stdout]
        fledge 0.2.1
        > ! stderr 'scaffold was written'
        ...
        > exec fledge init
        ...
        > ! stderr 'scaffold was written'
        ...
        > cd subdir
        > exec fledge preen
        [stderr]
        fledge: scaffold was written by fledge 0.0.1, binary is 0.2.1 — run 'fledge init --refresh'
        > stderr 'scaffold was written by fledge 0\.0\.1'
        PASS

--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/stamp_warning (0.01s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.011s
```

## AC-2

`fledge preen` with stamp version 0.0.1 and binary 0.2.1 emits exactly:
```
fledge: scaffold was written by fledge 0.0.1, binary is 0.2.1 — run 'fledge init --refresh'
```
The txtar `cd subdir` step confirms the upward walk fires from a subdirectory. The `! stderr 'scaffold was written'` assertions for `version` and `init` confirm those commands are excluded.

## AC-3

The `--json` step shows stdout is `{\n  "findings": [],\n  "ok": true\n}` — byte-identical JSON regardless of the warning. The warning goes to stderr only. Exit codes are ExitOK (0) throughout the mismatch and matching/absent steps.

```
$ go test ./... -count=1
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.064s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.002s
```

## AC-4

```
$ go test ./... -count=1
(all packages pass — see AC-3 output above)

$ go vet ./...
(no output — clean)
```
