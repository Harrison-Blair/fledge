---
id: FTHR-020
title: Update CLI acceptance test fixtures (txtar) to .fledge/pluma/
plumage: PLM-011
status: fledged
priority: P2
depends_on: [FTHR-017]
authored: 2026-07-11T02:32:39Z
agent: fledge-orchestrate/planning
fledge_version: 0.3.4
---

# FTHR-020: Update CLI acceptance test fixtures (txtar) to .fledge/pluma/

## Description
Update the ~104 `pluma/` path references across the CLI acceptance-test fixture files in `cmd/fledge/testdata/*.txtar` so every scaffolded/asserted path matches the new `.fledge/pluma/plumage`/`.fledge/pluma/feathers` convention introduced by FTHR-017. These fixtures are hermetic testscript files (script + inline file content) that scaffold a temp repo and assert exact stdout/paths, so once `internal/repo`/`internal/cli/init.go` change, every fixture referencing the old path breaks until updated here.

## Affected Modules
Per `.fledge/nest/testing.md` and the reference count mapped during planning — 12 files under `cmd/fledge/testdata/`:
`new.txtar` (15 refs), `check.txtar` (13), `report.txtar` (13), `lock.txtar` (11), `set.txtar` (10), `status.txtar` (10), `unfledged.txtar` (10), `criteria.txtar` (8), `graph.txtar` (6), `ready.txtar` (4), `init.txtar` (2), `stamp_warning.txtar` (2). (`e2e.txtar` and `preen_scaffold.txtar` have zero path refs — only the words "plumages"/"feathers" — and need no changes.)

## Approach
1. With FTHR-017 already merged (repo.go/init.go changed), run `go test ./cmd/fledge -run TestScripts` and capture the failures — every fixture still targeting the old `pluma/` path will fail (wrong path in assertions or in the scaffolded inline fixture content). This is the pre-fix FAILING evidence.
2. Go file by file through the 12 listed `.txtar` files, replacing every `pluma/plumage`/`pluma/feathers` path reference (in script assertions, `exec` args, and inline fixture file paths/headers) with `.fledge/pluma/plumage`/`.fledge/pluma/feathers`. Change only path strings — no assertion logic, ordering, or unrelated content.
3. Re-run `go test ./cmd/fledge -run TestScripts` after each file (or in batches) until the full suite is green.
4. Run the full suite once more (`go test ./cmd/fledge -run TestScripts -v`) for final passing evidence.

## Tests
- `go test ./cmd/fledge -run TestScripts` (all 21 acceptance tests, testscript-driven per `cmd/fledge/main_test.go:TestScripts`) — pins FC-4 for this surface. Captured FAILING against the 12 unfixed fixtures post-FTHR-017, then passing once all are updated.

## Acceptance Criteria
- [x] AC-1: `go test ./cmd/fledge -run TestScripts` was observed failing (against unfixed fixtures, post-FTHR-017 code change) and passing after every fixture is updated.
- [x] AC-2: None of the 12 listed `.txtar` files contain a `pluma/plumage` or `pluma/feathers` path reference (all read `.fledge/pluma/...`) — satisfies PLM-011 FC-4 for this surface.
- [x] AC-3: The full `cmd/fledge` acceptance suite (all 21 `.txtar` files) passes.
