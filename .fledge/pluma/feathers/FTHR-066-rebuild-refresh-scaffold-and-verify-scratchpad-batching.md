---
id: FTHR-066
title: "Rebuild, refresh scaffold, and verify scratchpad batching"
plumage: PLM-028
status: fledged
priority: P2
depends_on: [FTHR-062, FTHR-063, FTHR-064, FTHR-065]
authored: 2026-07-16T16:28:28Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-066: Rebuild, refresh scaffold, and verify scratchpad batching

## Description
Sole integration point for PLM-028: once F1-F4 (gitignore entry, `incubator.md` content, `SKILL.md` exception, `planning.md` references) have landed, rebuild and reinstall the `fledge` binary (since F1 changed Go source, unlike PLM-027 which was doc-only), regenerate this repo's own scaffold, and verify the full suite green (satisfies PLM-028 FC-1/AC-1 end-to-end, AC-5). Sole owner of the rebuild+refresh mutation, run after (not parallel with) F1-F4.

## Affected Modules
- The `fledge` binary itself (rebuilt from `internal/cli` per FTHR-062's change).
- `.fledge/scaffold.json`, `.fledge/skills/fledge-orchestrate/{incubator,planning}.md`, `.fledge/skills/fledge-interrogate/SKILL.md` (regenerated copies), `.gitignore` (gains `.fledge/scratch/` if not already present from PLM-027's refresh cycle).

## Approach
1. `go install ./cmd/fledge && hash -r && fledge version` — confirm the installed binary matches `VERSION` (per this repo's CLAUDE.md rebuild/reinstall convention).
2. `fledge init --refresh` — regenerate this repo's own scaffolded copies.
3. `git status`/`git diff` — confirm only the expected files changed (scaffold stamp, the three touched skill docs, `.gitignore`).
4. `fledge preen`, `go vet ./...`, `go test ./...`, `go test ./cmd/fledge -run TestScripts` — all green.

## Tests
Black-box verification suite, no new unit test:
- `fledge preen` (clean)
- `go vet ./...` (clean)
- `go test ./...` (all packages, including FTHR-062's extended `init.txtar` check and FTHR-063/064/065's new `internal/bootstrap` doc-assertion tests)
- `go test ./cmd/fledge -run TestScripts` (full txtar suite)
- Test-first framing: before rebuild+refresh, `fledge preen`/`fledge version` are expected to show the old binary and a stale scaffold; capture that as the pre-state, then confirm clean after.

## Acceptance Criteria
- [x] AC-1: `fledge version` matches `VERSION` after reinstall.
- [x] AC-2: `fledge init --refresh` completes with only the expected file set touched.
- [x] AC-3: `fledge preen` passes.
- [x] AC-4: `go vet ./...` and `go test ./...` pass (satisfies PLM-028 AC-5).
- [x] AC-5: `go test ./cmd/fledge -run TestScripts` passes in full, including the new `.fledge/scratch/` gitignore assertion (satisfies PLM-028 AC-1, AC-5).
