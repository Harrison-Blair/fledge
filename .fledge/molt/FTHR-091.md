# FTHR-091 Evidence

## AC-1

Test-first: extended `cmd/fledge/testdata/init.txtar` with two new assertions
before touching `internal/cli/init.go`:

- `grep '.fledge/ledger/' .gitignore` (after the existing per-entry greps)
- `grep -count=1 '.fledge/ledger/' .gitignore` (alongside the existing
  `grep -count=1 '.fledge/broods/' .gitignore` idempotency assertion)

Ran against the unchanged code and observed it FAIL for the expected reason —
the generated `.gitignore` genuinely lacks the line:

```
$ go test ./cmd/fledge -run TestScripts/init -v
...
        > grep '.fledge/nest/raw/' .gitignore
        > grep '.fledge/broods/' .gitignore
        > grep '.fledge/roster/' .gitignore
        > grep '.fledge/scratch/' .gitignore
        > grep '.alloc.lock' .gitignore
        > grep '.fledge/ledger/' .gitignore
        [.gitignore]
        # fledge — per-run intermediates, regenerable
        .fledge/nest/raw/
        .fledge/broods/
        .fledge/roster/
        .fledge/scratch/
        .alloc.lock

        FAIL: testdata/init.txtar:31: no match for `.fledge/ledger/` found in .gitignore

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/init (0.00s)
    --- PASS: TestScripts/init_agents (0.05s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.049s
```

Then added `.fledge/ledger/` to `gitignoreLines` in `internal/cli/init.go`
(one entry, matching the trailing-slash directory form of the existing
entries) and re-ran:

```
$ go test ./cmd/fledge -run TestScripts/init -v
...
--- PASS: TestScripts (0.16s)
    --- PASS: TestScripts/init (0.06s)
    --- PASS: TestScripts/init_agents (0.05s)
PASS
ok      github.com/Harrison-Blair/fledge/cmd/fledge    0.161s
```

Satisfies AC-1: failing observation captured before implementation, passing
observation captured after.

## AC-2

The failing observation above is behavioral, not a compilation/arity error:
it is a `testscript`/`txtar` acceptance test that runs the built `fledge`
binary against a freshly `git init`-ed scratch repo and greps the generated
`.gitignore` file for a literal string. The failure is `FAIL:
testdata/init.txtar:31: no match for '.fledge/ledger/' found in .gitignore`
— an assertion about the binary's observable output. Satisfies PLM-035 AC-2's
requirement of at least one behavioral failing-test observation.

## AC-3

`.fledge/ledger/` is written into a freshly initialized repository's
`.gitignore`, proven by the new `grep '.fledge/ledger/' .gitignore` assertion
in `init.txtar` (line ~31), which failed against the current code (see AC-1's
captured failure) and passes after `.fledge/ledger/` was added to
`gitignoreLines` in `internal/cli/init.go`.

```
$ go test ./cmd/fledge -run TestScripts/init -v
--- PASS: TestScripts/init (0.06s)
```

## AC-4

Re-running `fledge init` does not duplicate the ledger entry, proven by the
new `grep -count=1 '.fledge/ledger/' .gitignore` assertion added immediately
after the existing `grep -count=1 '.fledge/broods/' .gitignore` idempotency
assertion (init.txtar, second-run section). Both assertions pass:

```
$ go test ./cmd/fledge -run TestScripts/init -v
...
        > exec fledge init
        [stdout]
        exists .fledgeignore
        ...
        > grep -count=1 '.fledge/broods/' .gitignore
        > grep -count=1 '.fledge/ledger/' .gitignore
--- PASS: TestScripts/init (0.06s)
```

## AC-5

Every gitignore entry written before this feather is still written: the
pre-existing assertions in `init.txtar` (`.fledge/nest/raw/`,
`.fledge/broods/`, `.fledge/roster/`, `.fledge/scratch/`, `.alloc.lock`) were
left unmodified and pass alongside the new ledger assertion:

```
$ go test ./cmd/fledge -run TestScripts/init -v
...
        > grep '.fledge/nest/raw/' .gitignore
        > grep '.fledge/broods/' .gitignore
        > grep '.fledge/roster/' .gitignore
        > grep '.fledge/scratch/' .gitignore
        > grep '.alloc.lock' .gitignore
        > grep '.fledge/ledger/' .gitignore
--- PASS: TestScripts/init (0.06s)
```

## AC-6

```
$ go test ./...
ok      github.com/Harrison-Blair/fledge/cmd/fledge    0.488s
ok      github.com/Harrison-Blair/fledge/internal/bootstrap    0.010s
ok      github.com/Harrison-Blair/fledge/internal/check        0.001s
ok      github.com/Harrison-Blair/fledge/internal/ciconfig     0.003s
ok      github.com/Harrison-Blair/fledge/internal/cli  4.019s
ok      github.com/Harrison-Blair/fledge/internal/doctest      0.008s
ok      github.com/Harrison-Blair/fledge/internal/graph        0.008s
ok      github.com/Harrison-Blair/fledge/internal/hooktest     0.131s
ok      github.com/Harrison-Blair/fledge/internal/ledger       0.008s
ok      github.com/Harrison-Blair/fledge/internal/lock 0.013s
ok      github.com/Harrison-Blair/fledge/internal/nest 0.009s
ok      github.com/Harrison-Blair/fledge/internal/repo 0.008s
ok      github.com/Harrison-Blair/fledge/internal/roster       0.015s
ok      github.com/Harrison-Blair/fledge/internal/scan 0.017s
ok      github.com/Harrison-Blair/fledge/internal/spec 0.014s

$ go vet ./...
(no output — clean, exit 0)

$ gofmt -l .
(no output — clean, exit 0)

$ go build -o /tmp/fledge_ftr091 ./cmd/fledge && /tmp/fledge_ftr091 preen
WARN  .fledge/pluma/feathers/FTHR-061-refresh-scaffold-and-verify-worker-protocols-split.md: checked criteria missing evidence sections in .fledge/molt/FTHR-061.md: AC-1, AC-2, AC-3, AC-4, AC-5 (heading must be the bare form "## AC-N", not "## AC-N: <label>")
1 warning(s)
(exit 0 — reports no errors; the pre-existing FTHR-061 warning is unrelated
to this feather's changes, present on the worktree's base and untouched by
this diff)
```
