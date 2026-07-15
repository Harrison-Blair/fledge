# FTHR-036 Molt Evidence

## AC-1

Wrote the 4 new tests in `internal/bootstrap/worker_protocols_test.go` (package
`bootstrap`), reading the embedded `worker-protocols.md` the same way
`TestCoreNeutral`/`TestCorePrimitivesReferenced` do in `registry_test.go`.

Command:

```
go test ./internal/bootstrap -run TestSkua -v
```

Captured output against the UNMODIFIED file (pre-implementation):

```
=== RUN   TestSkuaConcessionHardened
    worker_protocols_test.go:74: ### Verdict still contains the old lenient concession sentence: "If a brooder pushes back on a finding with a fact verified to be correct, withdraw the finding"
    worker_protocols_test.go:78: ### Verdict must state the skua itself re-verifies the brooder's disproof before withdrawing a finding
    worker_protocols_test.go:81: ### Verdict must require the disproof to be independently checkable
    worker_protocols_test.go:84: ### Verdict must state a bare/unverified counter-assertion never withdraws a finding
--- FAIL: TestSkuaConcessionHardened (0.00s)
=== RUN   TestSkuaEvidenceGuiltyUntilProven
    worker_protocols_test.go:100: ### Reviewing a feather criteria-audit item must state ambiguous/incomplete/terse-log evidence "is NOT proof"
    worker_protocols_test.go:106: ### Reviewing a feather criteria-audit item must require that for any command not re-run, the recorded output itself must be sufficient to independently confirm the claim
--- FAIL: TestSkuaEvidenceGuiltyUntilProven (0.00s)
=== RUN   TestSkuaRedTeamPass
    worker_protocols_test.go:124: ### Reviewing a feather: no "Red-team pass" item found
--- FAIL: TestSkuaRedTeamPass (0.00s)
=== RUN   TestSkuaUnchangedInvariants
--- PASS: TestSkuaUnchangedInvariants (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

All four tests ran against the unmodified `worker-protocols.md`. Three failed
for the expected reason (new hardened language not yet present in the
unmodified file): `TestSkuaConcessionHardened`, `TestSkuaEvidenceGuiltyUntilProven`,
`TestSkuaRedTeamPass`. `TestSkuaUnchangedInvariants` passed already, as
expected, since it only guards pre-existing sentences that this feather must
not touch.

Post-implementation, after editing the `## Skua` section per the Approach, all
four tests pass:

```
=== RUN   TestSkuaConcessionHardened
--- PASS: TestSkuaConcessionHardened (0.00s)
=== RUN   TestSkuaEvidenceGuiltyUntilProven
--- PASS: TestSkuaEvidenceGuiltyUntilProven (0.00s)
=== RUN   TestSkuaRedTeamPass
--- PASS: TestSkuaRedTeamPass (0.00s)
=== RUN   TestSkuaUnchangedInvariants
--- PASS: TestSkuaUnchangedInvariants (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

## AC-2

`### Reviewing a feather`'s criteria-audit item (now item 6, renumbered after
inserting the red-team item) states the guilty-until-proven default:

> Evidence is guilty until proven: for each acceptance criterion, verify its
> claim against its `## AC-N` section in the evidence file, and treat an
> ambiguous, incomplete, or terse-log-only section (e.g. just an exit code or
> a one-line summary, with no visible assertions/diffs/output) as NOT proof:
> leave that box unchecked and file a finding instead. Re-run commands where
> cheap; for any command not re-run, the recorded output itself must be
> sufficient to independently confirm the claim, or it is a finding.

Command:

```
go test ./internal/bootstrap -run TestSkuaEvidenceGuiltyUntilProven -v
```

Output:

```
=== RUN   TestSkuaEvidenceGuiltyUntilProven
--- PASS: TestSkuaEvidenceGuiltyUntilProven (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

## AC-3

`### Reviewing a feather` now has a "Red-team pass" item (item 4) positioned
after "Diff vs. spec" (item 3) and before "Scope and simplicity" (item 5),
stating it runs every review cycle and that gaps are reported as findings
only — the skua "never writes or commits the missing test itself":

> **Red-team pass.** Run every review cycle, not only the first: read the
> implementation for branches, inputs, and interactions the spec's Tests
> section never names, and probe them using only throwaway, never-committed
> means — ad hoc invocations with uncovered inputs, or a scratch test file
> kept outside the tracked worktree. Any gap found is reported as a numbered
> finding; the skua never writes or commits the missing test itself.

Command:

```
go test ./internal/bootstrap -run TestSkuaRedTeamPass -v
```

Output:

```
=== RUN   TestSkuaRedTeamPass
--- PASS: TestSkuaRedTeamPass (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

## AC-4

`### Verdict`'s closing concession paragraph now requires independently
re-verified disproof before a finding withdraws:

> A finding withdraws only when the brooder supplies concrete, independently
> checkable disproof — a specific test run, line reference, or spec citation
> that directly contradicts the finding — **and** the skua itself re-verifies
> that disproof (re-runs the cited command, reads the cited line/spec text)
> before withdrawing; a bare counter-assertion, re-explanation, or unverified
> "that's intentional" never withdraws a finding — if the disproof doesn't
> meet this bar, the finding stands. A genuine judgment call unresolved after
> one round still escalates to the orchestrator rather than looping.

The old lenient sentence ("If a brooder pushes back on a finding with a fact
verified to be correct, withdraw the finding...") is gone.

Command:

```
go test ./internal/bootstrap -run TestSkuaConcessionHardened -v
```

Output:

```
=== RUN   TestSkuaConcessionHardened
--- PASS: TestSkuaConcessionHardened (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

## AC-5

The "Third rejection" sentence in `### Verdict` and the cited `## Brooder`
fix-loop sentence are unchanged verbatim — `git diff` against the pre-edit
file touches only the criteria-audit item, the new red-team item, and the
Verdict concession paragraph; nothing else in the file changed
(`git diff --stat` shows a single file, 4 insertions/3 deletions, confined to
those spots).

Command:

```
go test ./internal/bootstrap -run TestSkuaUnchangedInvariants -v
```

Output:

```
=== RUN   TestSkuaUnchangedInvariants
--- PASS: TestSkuaUnchangedInvariants (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

`TestSkuaUnchangedInvariants` also passed against the pre-edit file (see
AC-1's captured run above), confirming these sentences were never at risk.

## AC-6

Commands:

```
go vet ./...
go test ./internal/bootstrap/...
go test ./...
```

Output (`go vet ./...` produced no output — clean; exit code 0):

```
$ go vet ./...
$ echo $?
0
```

```
$ go test ./internal/bootstrap/...
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
```

```
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.091s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.010s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.129s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s
```
