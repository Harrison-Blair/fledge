---
id: FTHR-017
title: Change spec-directory convention to .fledge/pluma/ and update init scaffold literals
plumage: PLM-011
status: pipping
priority: P2
depends_on: []
authored: 2026-07-11T02:24:20Z
agent: fledge-orchestrate/planning
fledge_version: 0.3.4
---

# FTHR-017: Change spec-directory convention to .fledge/pluma/ and update init scaffold literals

## Description
Change `internal/repo.Repo`'s `RequirementsDir()`/`TasksDir()` so plumage/feather specs resolve under `.fledge/pluma/` instead of a root-level `pluma/`, and update the two scaffold literal lists in `internal/cli/init.go` that currently write `pluma/{plumage,feathers}/.gitkeep` at repo root. This is the tracer-bullet feather: it proves the new convention end-to-end (a fresh `fledge init` scaffolds specs at the new location, and every command that reads `RequirementsDir()`/`TasksDir()` — which is all spec-mutating commands, per `internal/spec.Load`) works correctly, before any test fixtures, docs, or this repo's own specs are touched.

## Affected Modules
Per `.fledge/nest/architecture.md` and `.fledge/nest/modules.md`:
- `internal/repo/repo.go` — `RequirementsDir()` (line 33) and `TasksDir()` (line 34): change `filepath.Join(r.Root, "pluma", "plumage")` / `filepath.Join(r.Root, "pluma", "feathers")` to join onto `r.FledgeDir()` instead of `r.Root`, matching the existing convention used by `LocksDir()`/`ContextDir()`/`EvidenceDir()` (lines 29-32).
- `internal/cli/init.go` — two hand-synced literal lists: the execution path that writes `pluma/{plumage,feathers}/.gitkeep` (~line 143-144), and `baseScaffoldEntries()`'s stamp path list (~line 387-388). Both become `.fledge/pluma/...`.
- `internal/spec/load_test.go`, `internal/spec/frontmatter_test.go` — path-derived test fixtures/assertions.
- `internal/check/check_test.go` — path-derived test fixtures/assertions.

## Approach
1. In `internal/spec/load_test.go`, `internal/spec/frontmatter_test.go`, and `internal/check/check_test.go`, update any hardcoded `pluma/plumage`/`pluma/feathers` path construction to `.fledge/pluma/plumage`/`.fledge/pluma/feathers` (or the equivalent helper call), matching what `RequirementsDir()`/`TasksDir()` will return after this change.
2. Run `go test ./internal/spec ./internal/check` and confirm the updated assertions FAIL against the unchanged `repo.go` (still returning the old root-level path) — capture the failure output.
3. Change `internal/repo/repo.go`: `RequirementsDir()` → `filepath.Join(r.FledgeDir(), "pluma", "plumage")`; `TasksDir()` → `filepath.Join(r.FledgeDir(), "pluma", "feathers")`.
4. Change `internal/cli/init.go`'s two literal lists (execution-time `.gitkeep` writes and `baseScaffoldEntries()`) from `pluma/plumage`/`pluma/feathers` to `.fledge/pluma/plumage`/`.fledge/pluma/feathers`.
5. Re-run the tests from step 2 and confirm they now pass. Run `go build ./...` and `go test ./internal/repo ./internal/spec ./internal/check ./internal/cli` to confirm no other breakage in these packages (txtar acceptance tests and this repo's own live specs are explicitly out of scope for this feather — they break here and are fixed by dependent feathers already authored/planned).

## Tests
- `internal/spec` and `internal/check` existing unit tests, updated to assert the new `.fledge/pluma/...` paths (per Approach step 1) — pins FC-1.
- A fresh-repo scenario (can reuse or extend an existing `internal/cli` or `internal/repo` test, or a small new table case) asserting that `Repo.RequirementsDir()`/`TasksDir()` return paths under `.fledge/pluma/` — pins FC-1.
- Manual/scripted check (documented in the evidence file, not necessarily a new automated test): build the binary, run `fledge init` in a scratch temp dir, confirm `.fledge/pluma/plumage/.gitkeep` and `.fledge/pluma/feathers/.gitkeep` exist and no root-level `pluma/` is created — pins FC-2.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation (against unchanged `repo.go`/`init.go`) and pass after.
- [ ] AC-2: `internal/repo.Repo.RequirementsDir()` returns `<FledgeDir>/pluma/plumage` and `TasksDir()` returns `<FledgeDir>/pluma/feathers` (satisfies PLM-011 FC-1).
- [ ] AC-3: A fresh `fledge init` in a scratch temp repo scaffolds `.fledge/pluma/plumage/.gitkeep` and `.fledge/pluma/feathers/.gitkeep`, with no root-level `pluma/` created (satisfies PLM-011 FC-2).
- [ ] AC-4: `go build ./...` succeeds and `go test ./internal/repo ./internal/spec ./internal/check ./internal/cli` pass.
