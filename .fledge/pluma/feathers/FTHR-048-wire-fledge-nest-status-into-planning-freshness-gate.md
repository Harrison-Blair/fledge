---
id: FTHR-048
title: Wire fledge nest status into planning freshness gate
plumage: PLM-024
status: pipping
priority: P2
depends_on: []
authored: 2026-07-16T01:57:23Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-048: Wire fledge nest status into planning freshness gate

## Description
`planning.md`'s freshness gate (§1) currently has the LLM hand-parse
`index.md` frontmatter `commit` and compare it to `git rev-parse HEAD`.
`fledge nest status --json` already computes this exact equality
(`internal/nest/nest.go`'s `IndexCommitMatches`, JSON field
`index_commit_matches`), so this feather rewires the gate to read that field
for the equality verdict, while keeping the gate's own `git log --oneline
<commit>..HEAD` staleness summary for the mismatch case (since `nest status`
doesn't produce that). `implementation.md`'s ~line 42 cross-reference to this
gate is updated to match.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/planning.md` — §1
  Freshness gate (see `.fledge/nest/modules.md` → bootstrap/core skills).
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` —
  ~line 42, the freshness-gate cross-reference.
- `internal/cli/nest.go` / `internal/nest/nest.go` — `IndexCommitMatches` /
  `index_commit_matches`, already emitted by `fledge nest status --json`; no
  Go change needed, only its consumption in prose.
- `cmd/fledge/testdata/` — whichever txtar fixture(s) assert on the
  scaffolded `planning.md`/`implementation.md` content.

## Approach
Rewrite planning.md §1 to: if `.fledge/nest/index.md` doesn't exist, go to
step 2 (unchanged) — running `fledge nest status` against a nonexistent nest
already reports missing docs, but the existence check stays a cheap
first-pass short-circuit. Otherwise, run `fledge nest status --json` and read
`index_commit_matches`: `true` → context is fresh, skip to step 3 (unchanged
behavior, new mechanism); `false` → still run `git log --oneline
<index_commit>..HEAD` to summarize the staleness for the `confirm-gate`
(this part is unchanged — `nest status` doesn't produce it), and gate on
regenerate-vs-proceed as today. Update `implementation.md`'s ~line 42
reference to describe the same `nest status --json` mechanism instead of
"compare `.fledge/nest/index.md` commit to HEAD."

## Tests
- A `cmd/fledge` txtar test asserting the scaffolded `planning.md` instructs
  running `fledge nest status --json` and branching on
  `index_commit_matches`, retains the `git log --oneline` staleness-summary
  step for the mismatch case, and that `implementation.md`'s freshness-gate
  cross-reference matches. Written first against the *current* scaffolded
  content (hand-parse language present, new instruction absent) and confirmed
  to FAIL, then the prose is rewritten and `fledge init --refresh`
  regenerates the scaffold until the assertion passes (satisfies PLM-024
  FC-2, AC-2).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-048.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-024 FC-2"). AC-1 is always:
- [x] AC-1: The test listed above was observed failing before implementation
  and passes after; evidence captured verbatim.
- [x] AC-2: `planning.md`'s freshness gate reads `index_commit_matches` from
  `fledge nest status --json` for the equality verdict, retains its own
  `git log --oneline` staleness summary for the mismatch case, and
  `implementation.md`'s cross-reference is updated to match (satisfies
  PLM-024 FC-2, AC-2).
- [x] AC-3: `fledge init --refresh` regenerates this repo's scaffolded copies
  to match, and `go test ./cmd/fledge -run TestScripts` passes.
