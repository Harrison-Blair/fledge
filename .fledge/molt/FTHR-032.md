# FTHR-032 evidence — Atomic brood-file writes and corrupt-file-resilient broods listing

## AC-1: Tests observed failing before implementation, passing after

The spec's two named tests (`TestListSkipsCorruptBroodFile`, `TestAcquireWritesAtomically`)
require `lock.List` to report which `.brood` files it skipped, which the pre-change
signature (`List(dir string) ([]Record, error)`) has no way to express. So the
test-first capture is a **compile failure**: the tests as written (calling the new
3-return `List`) don't build against the unchanged 2-return `List`, which is the
expected reason — the capability literally doesn't exist yet in the API.

Command (run against the code with the tests added but `lock.go`/`brood.go`/`colony.go`
still unchanged):

```
$ go test ./internal/lock/... -run 'TestListSkipsCorruptBroodFile|TestAcquireWritesAtomically' -race -v
# github.com/Harrison-Blair/fledge/internal/lock [github.com/Harrison-Blair/fledge/internal/lock.test]
internal/lock/lock_test.go:83:26: assignment mismatch: 3 variables but List returns 2 values
internal/lock/lock_test.go:88:23: assignment mismatch: 3 variables but List returns 2 values
internal/lock/lock_test.go:117:23: assignment mismatch: 3 variables but List returns 2 values
FAIL	github.com/Harrison-Blair/fledge/internal/lock [build failed]
FAIL
```

Supplementary evidence for the runtime symptom the spec describes ("List returns an
error, zero records"): reading the unchanged `List` (git history / `HEAD~` at
`internal/lock/lock.go`, pre-this-branch) shows the loop does `rec, err := Get(...); if
err != nil { return nil, err }` — the very first `Get` failure (line ~93 in the old
code) returns `nil, err` immediately, discarding any healthy records already
collected and any not yet visited. `Get`'s error message is `"corrupt brood file for
%s: %w"`, confirming a single non-JSON `.brood` file aborts the whole listing.

After implementation (temp-file + `os.Link` `Acquire`, skip-and-continue `List`
returning `(records, skipped, error)`):

```
$ go test ./internal/lock/... -race -v -count=3
=== RUN   TestAcquireReleaseGet
--- PASS: TestAcquireReleaseGet (0.00s)
=== RUN   TestAcquireHeld
--- PASS: TestAcquireHeld (0.00s)
=== RUN   TestAcquireContention
--- PASS: TestAcquireContention (0.00s)
=== RUN   TestList
--- PASS: TestList (0.00s)
=== RUN   TestListSkipsCorruptBroodFile
--- PASS: TestListSkipsCorruptBroodFile (0.00s)
=== RUN   TestAcquireWritesAtomically
--- PASS: TestAcquireWritesAtomically (0.01s)
... (repeated x3 for -count=3, all PASS)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/lock	1.035s
```

## AC-2: List surfaces (does not swallow) corrupt/partial `.brood` files; `broods` warns

`internal/lock/lock.go` `List` now skips a `.brood` file that fails `Get` (bad JSON or
zero-length) and appends its filename to a `skipped []string` return instead of
aborting; it still returns the healthy records sorted by task ID. `TestListSkipsCorruptBroodFile`
(`internal/lock/lock_test.go`) writes two valid claims plus a non-JSON garbage file and
a zero-length file, and asserts both healthy records come back and both bad filenames
are reported as skipped — see the passing run under AC-1.

`internal/cli/brood.go` `runLocks` now prints a stderr warning per skipped file. Manual
end-to-end check:

```
$ cd /tmp/broodtest && git init -q && fledge init >/dev/null 2>&1
$ mkdir -p .fledge/broods
$ echo '{"feather":"FTHR-100","owner":"a","pid":1,"created":"x","branch":"main"}' > .fledge/broods/FTHR-100.brood
$ echo 'garbage-not-json' > .fledge/broods/FTHR-999.brood
$ fledge broods
warning: skipping corrupt brood file FTHR-999.brood
FTHR-100  a  since x  branch main
$ echo "exit=$?"
exit=0
```

The healthy claim (`FTHR-100`) is listed and the corrupt one (`FTHR-999`) is named in a
warning rather than aborting the command (old behavior: `fledge broods` would have
failed with `corrupt brood file for FTHR-999: ...` and shown nothing).

## AC-3: Acquire writes atomically; still returns `*HeldError`; no partial file observable

`internal/lock/lock.go` `Acquire` now marshals the record, writes it to a temp file
created via `os.CreateTemp(dir, ".fledge-tmp-*")`, closes it, then places it at the
final `<task>.brood` path with `os.Link(tmpName, finalPath)` — `os.Link` fails
`EEXIST` when the claim is already held (mapped to the existing `*HeldError`, reading
the current holder via `Get`, exactly as before), and on success the fully-written
file appears in one atomic step. The temp file is removed via `cleanup()` on every
path (success, held, or write error).

`TestAcquireWritesAtomically` (`internal/lock/lock_test.go`) runs a concurrent reader
goroutine that tightly polls the target `.brood` path while 500 Acquire/Release cycles
run, and fails the test if it ever observes a zero-length or non-JSON-parseable file at
that path; it then does one more `Acquire`, asserts the broods dir contains exactly one
entry (`FTHR-777.brood`, i.e. no leftover `.fledge-tmp-*` file), and asserts a second
`Acquire` for the same task returns `*HeldError` with the correct holder. Passing run
is under AC-1 (`-race -count=3`, all iterations clean).

`TestAcquireHeld` and `TestAcquireContention` (pre-existing tests, unmodified) continue
to pass, confirming the `*HeldError` contract and exactly-one-winner-under-contention
semantics survived the rewrite — see the AC-1 run above.

## AC-4: `fledge preen` passes and `go test ./...` is green

```
$ go vet ./...
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.080s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.122s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.002s
```

```
$ go build -o /tmp/fledge-fthr032 ./cmd/fledge && /tmp/fledge-fthr032 preen; echo exit=$?
WARN  .fledge/pluma/feathers/FTHR-028-...md: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-031-...md: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-032-...md: status hatching but no brood is held
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
5 warning(s)
exit=0
```

`preen` exits 0 (warnings only). The 5 warnings are pre-existing/environmental to this
worktree (no brood literally held in this checkout for the three in-flight sibling
feathers, and two scaffold files this worktree doesn't carry) — none reference
`internal/lock`, `internal/cli/brood.go`, or `internal/cli/colony.go`, and `preen`
itself exercises `lock.List` (via `runPreen` → the same code path `colony.go:154` now
calls with the updated 3-return signature), which ran cleanly here.

## Notes on scope

`internal/cli/colony.go:154` (`fledge preen`'s report builder) also calls `lock.List`
and was updated for the new signature (`if recs, _, err := lock.List(...)`); it
discards `skipped` because `preen` has its own separate corrupt-file reporting via
`set.Errors`/`ParseErrors` for spec files and this feather's scope is `lock.List` +
`runLocks`'s warning per the spec's Affected Modules — `colony.go`'s change is the
minimal one-line signature fix needed to keep it compiling, not new behavior.
