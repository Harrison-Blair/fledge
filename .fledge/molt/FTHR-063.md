# FTHR-063 evidence — Author scratchpad batching mechanics into incubator.md

## AC-1

New test `TestIncubatorDocDescribesScratchpadBatching` (`internal/bootstrap/incubator_test.go`) run against the unchanged `incubator.md` (pre-implementation), captured verbatim:

```
$ go test ./internal/bootstrap -run TestIncubatorDocDescribesScratchpadBatching
--- FAIL: TestIncubatorDocDescribesScratchpadBatching (0.00s)
    incubator_test.go:47: incubator.md must contain the "### Scratchpad batching" subsection heading
    incubator_test.go:58: incubator.md must contain "independent leaves"
    incubator_test.go:58: incubator.md must contain ".fledge/scratch/"
    incubator_test.go:58: incubator.md must contain "one `GATE review`"
    incubator_test.go:58: incubator.md must contain "plumage interrogation"
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

Failing for the expected reason: none of the scratchpad-batching content exists in `incubator.md` yet. (The `"feather interrogation"` assertion alone does not appear in the failure list because the pre-existing intro line "plumage and feather interrogation" matches it as a substring; the test still fails on the other four assertions plus the missing heading.)

Post-implementation passing run, after adding the `### Scratchpad batching` subsection to `incubator.md`, captured verbatim:

```
$ go test ./internal/bootstrap -run TestIncubatorDocDescribesScratchpadBatching -v
=== RUN   TestIncubatorDocDescribesScratchpadBatching
--- PASS: TestIncubatorDocDescribesScratchpadBatching (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

(Iteration note, visible in the branch history: the first draft of the subsection wrote "Relay **one** `GATE review`", which broke the literal ``one `GATE review` `` assertion; the bold was removed — "Relay exactly one `GATE review`" — and the test passed. No assertion was weakened.)

## AC-2

`incubator.md` (`internal/bootstrap/core/skills/fledge-orchestrate/incubator.md`) now contains a `### Scratchpad batching` subsection, placed directly after `### Relay envelope`, documenting all three required elements:

1. **Batchable/individual-gate rule** — "a question is batchable when its answer doesn't change what else needs asking" (independent leaves: naming, priority, in/out-of-scope calls, test-framework picks, oversight level) vs. load-bearing questions that stay individually relayed (`GATE`/`QUESTION`), with the plumage-breakdown decision and every spec-draft review gate explicitly called out as always individual, never batched.
2. **Scratchpad naming/lifecycle** — `.fledge/scratch/PLM-<slug-or-###>-questions.md` / `FTHR-<slug-or-###>-questions.md`, recommended answer included per question, overwrite any prior batch for the same tree (no archiving), file left on disk once consumed (harmless, gitignored, a paper trail).
3. **`GATE review` reuse mechanics** — exactly one `GATE review` pointing at the file path with the instruction "answer inline, then Accept"; explicitly stated to reuse the existing `GATE review` envelope (material + Accept / Make changes), not a new envelope kind; on "Accept" the file is re-read from disk to pick up inline answers; on "Make changes" the incubator waits for a re-send of the same gate.

Verified by the phrase assertions in `TestIncubatorDocDescribesScratchpadBatching` (heading, "independent leaves", ".fledge/scratch/", "one `GATE review`") — passing run captured under AC-1.

## AC-3

The subsection's closing line states scope over both interrogations, verbatim from the doc:

```
The same rule governs both plumage interrogation (`planning.md` step 3) and feather interrogation (step 4).
```

Pinned by the test's "plumage interrogation" and "feather interrogation" assertions (passing run under AC-1).

## AC-4

```
$ go test ./internal/bootstrap/...
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
```

Also ran `go vet ./...` (clean) and the full `go test ./...` suite in the worktree — all packages ok, including `cmd/fledge` (txtar scaffold fixtures):

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.099s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.011s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.129s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s
```

