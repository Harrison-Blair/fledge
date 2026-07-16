# FTHR-067 evidence

## AC-1

Both tests observed failing before implementation, passing after.

**Pre-implementation (unchanged `planning.md`):**

```
$ go test ./internal/bootstrap -run 'TestPlanningDocDescribesDigest' -v
=== RUN   TestPlanningDocDescribesDigestWrite
    planning_digest_test.go:42: step 4.7 must instruct writing .fledge/scratch/digest-planning.md
--- FAIL: TestPlanningDocDescribesDigestWrite (0.00s)
=== RUN   TestPlanningDocDescribesDigestRead
    planning_digest_test.go:66: step 1 must mention reading .fledge/scratch/digest-implementation.md
--- FAIL: TestPlanningDocDescribesDigestRead (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

**Post-implementation (planning.md step 1 + step 4.7 additions in place):**

```
$ go test ./internal/bootstrap -run 'TestPlanningDocDescribesDigest' -v
=== RUN   TestPlanningDocDescribesDigestWrite
--- PASS: TestPlanningDocDescribesDigestWrite (0.00s)
=== RUN   TestPlanningDocDescribesDigestRead
--- PASS: TestPlanningDocDescribesDigestRead (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

## AC-2

`planning.md` step 4.7 now documents writing the digest with the FC-3 content
shape (outcome, key user decisions, spec pointers), overwriting per FC-4:

```
$ grep -n "digest-planning" internal/bootstrap/core/skills/fledge-orchestrate/planning.md
58:7. After the last feather, run `fledge preen` and fix every finding before closing. Close by listing the created files, the dependency waves (`fledge vee`), the ready-to-start feathers (`fledge ready`, the dispatchable-now subset), and the full remaining slate of non-fledged plumage and feathers (`fledge unfledged`, everything not yet complete). Then write `.fledge/scratch/digest-planning.md` (`write-file`, overwriting any prior one) containing the phase's outcome (hatched plumage/feather IDs and file paths), the key user decisions made during interrogation (not the full Q&A — just the resolved decisions and their rationale), and pointers to the created spec files. When delegated, this closing report is your `PHASE-CLOSE` message to the orchestrator. Offer to start the implementation phase on the ready feathers.
```

Pinned by `TestPlanningDocDescribesDigestWrite` (passing run above), which
asserts the digest path plus "outcome", "decisions", "overwriting", and
"pointers to the created spec files" all appear in step 4.7's prose.

## AC-3

`planning.md` step 1 now documents best-effort reading of the prior
implementation digest:

```
$ grep -n "digest-implementation" internal/bootstrap/core/skills/fledge-orchestrate/planning.md
23:- If `.fledge/scratch/digest-implementation.md` exists, read it as grounding context — the previous implementation phase's close-out digest — before continuing. This is best-effort: a missing file (e.g. on a repo's first-ever planning phase) means proceed without it.
```

Pinned by `TestPlanningDocDescribesDigestRead` (passing run above), which
asserts the path, "best-effort", and "proceed without it" appear inside the
"## 1. Freshness gate" section.

## AC-4

```
$ go test ./internal/bootstrap/...
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
```

Full suite also green in the worktree (plus gofmt/vet clean):

```
$ go test ./... | tail -25
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.100s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.010s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.127s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.013s
$ gofmt -l .
$ go vet ./...
```

