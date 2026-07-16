---
id: PLM-024
title: Wire deterministic CLI queries into workflow closeout and freshness gates
status: hatched
priority: P2
authored: 2026-07-16T01:33:22Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# PLM-024: Wire deterministic CLI queries into workflow closeout and freshness gates

## Context
A `/code-review med` pass over recent commits found three places where the
fledge-orchestrate workflow prose has the LLM hand-derive a fact that an
existing `fledge` command already computes deterministically, or where a
trivial gap makes machine-local state look like repo noise:

- **Plumage closeout** (`implementation.md`, solo and team paths) gates on
  the orchestrator mentally tracking "was that the last unfinished feather of
  its plumage?" `fledge colony --json` already reports per-plumage
  `fledged`/`total` counts (`internal/cli/colony.go:160-165`) but is invoked
  nowhere in any workflow prose — a miscount can close a plumage early or
  leave a done one stuck `hatching`.
- **Planning freshness gate** (`planning.md` §1) has the LLM hand-parse
  `index.md` frontmatter `commit` and compare it to `git rev-parse HEAD` to
  decide whether nest context is current. `fledge nest status --json`
  already computes this exact equality (`IndexCommitMatches`,
  `internal/cli/nest.go`) but is a superset check — it also requires all docs
  synthesized/no stubs and fails on incomplete docs — and it does not produce
  the `git log --oneline <commit>..HEAD` staleness summary the gate shows the
  user. The fix reads the equality verdict from `nest status --json`, not a
  blind swap of the whole gate.
- **`.alloc.lock`** files, created by the ID allocator on every spec
  allocation and never removed by design (they're flock targets;
  `internal/spec/ids.go:96`), are not gitignored. Two sit untracked in this
  repo right now. A routine `git add -A` would commit zero-byte machine-local
  files, and they reappear as untracked noise on every fledge-managed clone.

All three fixes wire an existing deterministic mechanism into a place that
currently relies on LLM judgment or hand-tracking, per this review's stated
interest in cutting token cost, context buildup, and drift. None require new
`fledge` CLI functionality — `colony` and `nest status` already emit what's
needed; the gitignore fix is a one-line addition to the shipped scaffold.

## User Stories
- As an orchestrator running the implementation-phase closeout step, I want
  to query `fledge colony --json` for a plumage's fledged/total counts
  instead of mentally tracking whether a feather was "the last one," so a
  plumage can't be closed early or left open by miscount.
- As an orchestrator running the planning-phase freshness gate, I want to
  read the commit-freshness verdict from `fledge nest status --json` instead
  of hand-parsing `index.md` frontmatter, so the gate can't disagree with the
  CLI's own notion of "is the nest current."
- As anyone working in a fledge-managed repo, I want `.alloc.lock` files
  ignored by git like other machine-local intermediates, so they don't show
  up as untracked noise or get committed by a broad `git add`.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: `implementation.md`'s closeout step (solo path and team path) queries
   `fledge colony --json` for the plumage's `fledged`/`total` feather counts
   and uses that result — not mental tracking — to decide whether the
   plumage is fully fledged and ready to close out.
2. FC-2: `planning.md`'s freshness gate (§1) reads the commit-equality verdict
   from `fledge nest status --json`'s `index_commit_matches` field instead of
   hand-parsing `index.md` frontmatter and running `git rev-parse HEAD`
   itself; when the verdict is false, the gate still runs its own
   `git log --oneline <index_commit>..HEAD` to produce the staleness summary
   shown to the user (since `nest status` doesn't emit that). The
   `implementation.md` reference (~line 42) that defers to this gate is
   updated to match.
3. FC-3: `.alloc.lock` is added to the `gitignoreLines` slice in
   `internal/cli/init.go`, so both this repo (after a refresh) and every
   future `fledge init`/`fledge init --refresh` target ignore it.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [ ] AC-1: `implementation.md`'s closeout step (both solo and team paths, core
  source in `internal/bootstrap/core/skills/fledge-orchestrate/`) instructs
  querying `fledge colony --json` and using its `fledged`/`total` counts to
  gate plumage closeout, replacing the prior "was this the last feather"
  mental-tracking language; this repo's scaffolded copy is refreshed to
  match (FC-1).
- [ ] AC-2: `planning.md`'s freshness gate (core source) instructs running
  `fledge nest status --json` and branching on `index_commit_matches`, with
  the existing `git log --oneline` staleness-summary step retained for the
  mismatch case; the `implementation.md` cross-reference is updated; this
  repo's scaffolded copies are refreshed to match (FC-2).
- [ ] AC-3: `internal/cli/init.go`'s `gitignoreLines` includes `.alloc.lock`;
  a `cmd/fledge` txtar test (e.g. `init.txtar` or `init_agents.txtar`) asserts
  the generated `.gitignore` contains it; this repo's own `.gitignore` is
  refreshed and `git check-ignore` confirms both existing `.alloc.lock` files
  are now ignored (FC-3).
- [ ] AC-4: `go test ./...` passes and `fledge preen` reports the scaffold
  healthy after `fledge init --refresh`.

## Out of Scope
- Stale-lock worktree classification (F5) and the worker roster/species
  allocator (F6) — separate plumages.
- PLM-021's FC-2 wording tightening (F9) — handled as a direct edit to that
  existing plumage, not part of this one.
- Replacing the `depends_on` transitive closure check in the feather-readiness
  gate with `fledge colony` — refuted during review: `colony`'s `unmet` list
  is direct-only and set-unaware, so it cannot replace that closure walk.
- Adding a narrower pure-commit-freshness field/flag to `nest status` beyond
  what `index_commit_matches` already provides — FC-2 uses the existing field
  as-is.

## Open Questions
None — priority (P2) and grounding for all three findings were confirmed
during interrogation.
