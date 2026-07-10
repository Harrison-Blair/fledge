---
id: FTHR-013
title: "Version bump 0.3.0, docs, and dogfood refresh"
plumage: PLM-009
status: hatching
priority: P1
depends_on: [FTHR-010, FTHR-011, FTHR-012]
oversight: merge
authored: 2026-07-10T15:02:44Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# FTHR-013: Version bump 0.3.0, docs, and dogfood refresh

## Description
The release closeout: bump `VERSION` and `binaryVersion` to 0.3.0 (scaffold
output changed), update the documentation that promises the old refresh
semantics, add a short migration note for pre-stamp repos, and dogfood —
reinstall the binary and run `fledge init --refresh` in this repository so it
carries its own clean stamp. `oversight: merge` — the user reviews the final
diff (including the repo's own regenerated scaffold and stamp) before merge.

## Affected Modules
- **Root** — `VERSION` → `0.3.0`; migration note (README section or
  docs/, wherever release notes live — match existing docs conventions).
- **`internal/cli/version.go`** — `binaryVersion` default → `0.3.0`
  (version_test.go pins it to VERSION; both must move together).
- **`CLAUDE.md`** — the `init --refresh` description (re-sync semantics now
  preserve/prune) and the rebuild/reinstall ritual, plus a line noting
  `.fledge/scaffold.json`.
- **This repo's scaffold** — `.fledge/scaffold.json` created by the dogfood
  refresh; regenerated `.fledge/skills/` / `.claude/` output if the embed
  changed during the plumage.

## Approach
- Order: docs + version bump → `go install ./cmd/fledge && hash -r` →
  `fledge version` == 0.3.0 → `fledge init --refresh` at repo root →
  `git status` review.
- Expected dogfood outcome: stamp created; zero prunes; zero
  `kept (user-edited)` lines (this repo's scaffold is unedited generated
  output); `fledge preen` shows scaffold healthy afterwards.
- Mention `.fledge/scaffold.json` merge-conflict behavior (deterministic
  sorted output) in the migration note.

## Tests
Written test-first where a test applies; this feather is mostly release
mechanics verified by existing suites.
- `internal/cli/version_test.go` — already pins `binaryVersion` == VERSION;
  observed failing when only one side is bumped, passing when both are.
- Full `go test ./...` + `go vet ./...` green at 0.3.0.
- Manual verification recorded in the evidence file: `fledge version` output,
  the refresh report from this repo, and `fledge preen` scaffold-healthy
  output after the dogfood refresh.

## Acceptance Criteria
- [x] AC-1: `VERSION` and `binaryVersion` both read 0.3.0 and version_test passes (satisfies PLM-009 FC-6).
- [x] AC-2: CLAUDE.md and the migration note accurately describe the stamp file and the new preserve/prune refresh semantics.
- [x] AC-3: `fledge init --refresh` in this repository produces `.fledge/scaffold.json` with no prunes and no kept-as-edited reports, and `fledge preen` reports the scaffold healthy (satisfies PLM-009 AC-6).
- [x] AC-4: Full `go test ./...` and `go vet ./...` pass on the final tree.
