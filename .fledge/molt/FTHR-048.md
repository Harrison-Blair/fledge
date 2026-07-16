# FTHR-048 — Wire fledge nest status into planning freshness gate — evidence

## AC-1
Test written first (`cmd/fledge/testdata/freshness_gate.txtar`) and observed
FAILING against the current (unedited) scaffolded content, for the expected
reason — the gate does not yet run `fledge nest status --json`.

Command:
```
go test ./cmd/fledge -run 'TestScripts/freshness_gate'
```

Captured output (verbatim, pre-implementation):
```
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/freshness_gate (0.00s)
        ...
            FAIL: testdata/freshness_gate.txtar:14: no match for `fledge nest status --json` found in .fledge/skills/fledge-orchestrate/planning.md
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.005s
FAIL
```

Passing run after implementation (planning.md §1 rewritten, implementation.md
cross-reference updated, scaffold refreshed):
```
$ go test ./cmd/fledge -run 'TestScripts/freshness_gate'
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.005s
```

## AC-2
`planning.md`'s §1 freshness gate now reads `index_commit_matches` from
`fledge nest status --json` for the equality verdict, keeps its own
`git log --oneline` staleness summary (keyed off the JSON `index_commit`
field) for the mismatch case, and `implementation.md`'s cross-reference is
updated to match.

Scaffolded `planning.md` §1 (`.fledge/skills/fledge-orchestrate/planning.md`):
```
## 1. Freshness gate

- If `.fledge/nest/index.md` does not exist → go to step 2.
- Otherwise run `fledge nest status --json` and read `index_commit_matches`:
  - `true` → context is fresh; skip to step 3.
  - `false` → summarize the staleness (`git log --oneline <index_commit>..HEAD`, where `<index_commit>` is the `index_commit` field from that same JSON: how many commits, which areas changed) and run a `confirm-gate` (decision): regenerate context, or proceed with existing context. Respect their choice.
```

Scaffolded `implementation.md` cross-reference (line 42):
```
- Context freshness: apply the freshness gate from `planning.md` step 1 (`fledge nest status --json` → `index_commit_matches`; ask before regenerating).
```

Source of truth edited: `internal/bootstrap/core/skills/fledge-orchestrate/planning.md`
and `.../implementation.md`. `index_commit_matches` / `index_commit` are the
real JSON fields emitted by `fledge nest status --json`
(`internal/nest/nest.go` `StatusResult`). Satisfies PLM-024 FC-2, AC-2.

## AC-3
`fledge init --refresh` regenerated this repo's scaffolded copies to match the
new embedded source, and `fledge preen` reports the scaffold healthy; the full
`cmd/fledge` script suite passes.
```
$ fledge init --refresh --force   # (regenerated .fledge/skills/... copies)
$ fledge preen
spec clean: 26 plumages, 56 feathers
$ go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.094s
```
