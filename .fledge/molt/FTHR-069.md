# FTHR-069 evidence

## AC-1

Both tests observed failing before implementation, then passing after.

**Pre-implementation (unchanged `implementation.md`) — FAILING:**

Command (run in worktree `/home/penguin/source/fledge/.fledge/burrows/FTHR-069`):

```
go test ./internal/bootstrap -run 'TestImplementationDocDescribesDigest' -v
```

Output (verbatim):

```
=== RUN   TestImplementationDocDescribesDigestWrite
    implementation_digest_test.go:40: implementation.md step 5 missing digest-write wording ".fledge/scratch/digest-implementation.md"
    implementation_digest_test.go:40: implementation.md step 5 missing digest-write wording "overwrit"
--- FAIL: TestImplementationDocDescribesDigestWrite (0.00s)
=== RUN   TestImplementationDocDescribesDigestRead
    implementation_digest_test.go:56: implementation.md step 1 missing digest-read wording ".fledge/scratch/digest-planning.md"
    implementation_digest_test.go:56: implementation.md step 1 missing digest-read wording "best-effort"
--- FAIL: TestImplementationDocDescribesDigestRead (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

Both tests fail for the expected reason: no digest language exists yet in
`implementation.md` step 5 ("End of run") or step 1 ("Resolve scope").

**Post-implementation — PASSING:**

Command (same worktree):

```
go test ./internal/bootstrap -run 'TestImplementationDocDescribesDigest' -v
```

Output (verbatim):

```
=== RUN   TestImplementationDocDescribesDigestWrite
--- PASS: TestImplementationDocDescribesDigestWrite (0.00s)
=== RUN   TestImplementationDocDescribesDigestRead
--- PASS: TestImplementationDocDescribesDigestRead (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

## AC-2

`implementation.md` step 5 ("End of run") now documents writing
`digest-implementation.md` with the FC-3 content shape (outcome, key
decisions, spec pointers; prose not transcript) and the FC-4 overwrite
semantics.

Command:

```
sed -n '120,131p' internal/bootstrap/core/skills/fledge-orchestrate/implementation.md
```

Output (verbatim, trimmed to the section):

```
## 5. End of run

When no feathers remain in the set that are unfinished and dispatchable, verify every pair from the run has been torn down — for any straggler, request shutdown by name and then confirm it is gone, forcing termination via your adapter's mechanism if it does not exit — then reconcile the team task list (Tier C): every team task dispatched this run should be completed — complete any stragglers yourself and note discrepancies. Then report:

- feathers completed (merged, suite green) vs. blocked or escalated, with reasons
- merges performed and the final suite status on main
- any feathers newly unblocked outside the run's scope that could be implemented next

Then write `.fledge/scratch/digest-implementation.md` (`write-file`, overwriting any prior one — only the latest run's outcome matters) containing: the outcome (which feathers merged, the current suite status on main), the key decisions made during escalation triage (§4) if any — the resolved decisions and their rationale, not the full exchange — and pointers to the merged feathers' spec files. It is prose, not a transcript replay; the next phase reads it instead of assuming this conversation is still available.
```

Verified by `TestImplementationDocDescribesDigestWrite` (see AC-1 passing run).

## AC-3

`implementation.md` step 1 ("Resolve scope") now documents best-effort
reading `digest-planning.md` if present.

Command:

```
sed -n '31,33p' internal/bootstrap/core/skills/fledge-orchestrate/implementation.md
```

Output (verbatim):

```
## 1. Resolve scope

If `.fledge/scratch/digest-planning.md` exists, read it first as grounding context — it is the planning phase's close-out digest (what was decided, what was produced, where to look), written so you don't have to rely on the conversation still holding that detail. Reading it is best-effort: a missing file (e.g. no planning phase has run in this repo yet) means proceed without it.
```

Verified by `TestImplementationDocDescribesDigestRead` (see AC-1 passing run).

## AC-4

Command:

```
go test ./internal/bootstrap/...
```

Output (verbatim):

```
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
```

Additionally the full suite passes in the worktree (`go test ./...` — all
packages `ok`, including `cmd/fledge` txtar tests), and `gofmt -l .` /
`go vet ./...` are clean.
