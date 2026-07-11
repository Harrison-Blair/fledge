---
id: PLM-011
title: Relocate pluma/ spec directory into .fledge/pluma/
status: hatched
priority: P2
authored: 2026-07-11T02:14:33Z
agent: fledge-orchestrate/planning
fledge_version: 0.3.4
---

# PLM-011: Relocate pluma/ spec directory into .fledge/pluma/

## Context
fledge keeps its spec files — plumages (`PLM-###`) and feathers (`FTHR-###`) — in a top-level `pluma/` directory, separate from `.fledge/`, which holds every other piece of fledge-managed repo state (context nest, broods, scaffold stamp). This split is a historical artifact rather than a deliberate design choice: `pluma/` is exclusively fledge tooling state, never something a repo's own application code touches, so it belongs alongside the rest of fledge's footprint under `.fledge/`. Moving it there declutters the repo root and makes the convention consistent: everything fledge owns lives under one directory. This is a global convention change to `internal/repo`'s path derivation (`RequirementsDir()`/`TasksDir()`), not a per-repo opt-in — every future `fledge init` scaffolds specs under `.fledge/pluma/` from this point on, and this repo (which dogfoods fledge on itself) migrates its own 26 existing spec files to match.

## User Stories
- US-1: As a fledge maintainer, I want spec files (`pluma/plumage/`, `pluma/feathers/`) nested under `.fledge/`, so the repo root isn't cluttered with a directory that's really part of fledge's own tooling state (consistent with `.fledge/nest/`, `.fledge/broods/`, etc.).
- US-2: As a fledge user running `fledge init` on any repo (present or future), I want the scaffolded spec location to already follow the `.fledge/`-centric convention, so I don't have to reorganize later.

## Functional Criteria
1. FC-1: `internal/repo.RequirementsDir()` returns `<FledgeDir>/pluma/plumage`; `TasksDir()` returns `<FledgeDir>/pluma/feathers`.
2. FC-2: A fresh `fledge init` scaffolds `.fledge/pluma/plumage/.gitkeep` and `.fledge/pluma/feathers/.gitkeep` (not a root-level `pluma/`).
3. FC-3: This repo's own 26 existing spec files are physically relocated to `.fledge/pluma/plumage/` and `.fledge/pluma/feathers/` with zero content/frontmatter changes, and all `fledge` commands (`preen`, `vee`, `ready`, `status`, etc.) operate on them correctly at the new location.
4. FC-4: Every reference to the old `pluma/` path — Go source, txtar + Go unit tests, embedded scaffold prose (`internal/bootstrap/core/**`), and docs (README.md, CLAUDE.md, MIGRATION.md, docs/generalization-plan.md) — is updated to `.fledge/pluma/`.

## Acceptance Criteria
- [ ] AC-1: `go build ./...` and `go test ./...` pass with the new convention in place.
- [ ] AC-2: `fledge preen` reports the scaffold healthy on a freshly-refreshed repo using the new paths.
- [ ] AC-3: This repo's top-level `pluma/` directory no longer exists; `.fledge/pluma/plumage/` and `.fledge/pluma/feathers/` contain all 26 relocated specs.
- [ ] AC-4: `MIGRATION.md` documents the manual `git mv pluma .fledge/pluma` step for repos upgrading from the old convention.

## Out of Scope
- A per-repo config knob to choose old vs. new spec location — this is a global convention change, not configurable.
- Any automatic migration/compat code path in fledge itself (no dual-path lookup, no auto-detection of the old location, no deprecation warnings) — migration is manual, documented in MIGRATION.md.
- Renaming the `pluma` grouping name itself, or restructuring beyond the existing `plumage/`+`feathers/` two-subdir shape — only the parent location changes.
- Changing spec file content, frontmatter schema, or spec IDs — only physical location changes.
- Migrating any other repo's already-scaffolded `pluma/` — this plumage covers fledge's own code/docs/prose and this repo's own dogfooded specs; other repos migrate manually per MIGRATION.md at their own pace.

## Open Questions
None — all decisions resolved during interrogation.
