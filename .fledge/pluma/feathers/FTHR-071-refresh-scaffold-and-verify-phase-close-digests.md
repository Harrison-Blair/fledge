---
id: FTHR-071
title: Refresh scaffold and verify phase-close digests
plumage: PLM-029
status: egg
priority: P2
depends_on: [FTHR-067, FTHR-068, FTHR-069, FTHR-070]
authored: 2026-07-16T16:44:19Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-071: Refresh scaffold and verify phase-close digests

## Description
Sole integration point for PLM-029 (and the last feather of this entire planning phase): once G1-G4 land, regenerate this repo's scaffold and verify the full suite green. Doc-only plumage — no Go source changes, so no rebuild is needed (unlike FTHR-066).

## Affected Modules
- `.fledge/scaffold.json`, `.fledge/skills/fledge-orchestrate/{planning,foraging,implementation}.md`, `.claude/team-loop.md` (regenerated copies).

## Approach
1. `fledge init --refresh` — regenerate this repo's own scaffolded copies from the updated `internal/bootstrap/core`/`adapters/claude` sources.
2. `git status`/`git diff` — confirm only the expected files changed.
3. `fledge preen`, `go vet ./...`, `go test ./...`, `go test ./cmd/fledge -run TestScripts` — all green.
4. As a live demonstration of PLM-029 AC-4 (a sample digest showing the intended format), once this planning phase itself closes out (per the newly-authored `planning.md` step 4.7), it will write this very phase's `digest-planning.md` — confirm after the fact that the format matches FC-3 (outcome, key decisions, spec pointers, no full transcript) once produced.

## Tests
Black-box verification suite, no new unit test:
- `fledge preen` (clean)
- `go vet ./...` (clean)
- `go test ./...` (all packages, including G1-G4's new `internal/bootstrap` doc-assertion tests)
- `go test ./cmd/fledge -run TestScripts` (full txtar suite)
- Test-first framing: before `--refresh`, `fledge preen` is expected to show scaffold staleness; capture that as pre-state, confirm clean after.

## Acceptance Criteria
- [x] AC-1: `fledge init --refresh` completes with only the expected file set touched.
- [x] AC-2: `fledge preen` passes.
- [x] AC-3: `go vet ./...` and `go test ./...` pass (satisfies PLM-029 AC-5).
- [x] AC-4: `go test ./cmd/fledge -run TestScripts` passes in full (satisfies PLM-029 AC-5).
- [ ] AC-5: Once this planning phase's own close-out produces `digest-planning.md`, its content is confirmed to match the FC-3 format (outcome/decisions/pointers, no transcript) — satisfies PLM-029 AC-4.
