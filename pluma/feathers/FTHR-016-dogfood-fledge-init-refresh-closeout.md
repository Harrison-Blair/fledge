---
id: FTHR-016
title: Dogfood fledge init --refresh closeout
plumage: PLM-010
status: hatching
priority: P2
depends_on: [FTHR-014, FTHR-015]
authored: 2026-07-10T21:22:50Z
agent: fledge-orchestrate/planning
fledge_version: 0.3.0
---

# FTHR-016: Dogfood fledge init --refresh closeout

## Description
Integration closeout for PLM-010: now that the incubator is wired into the Claude adapter
(FTHR-014) and the core planning/foraging prose is updated (FTHR-015), regenerate *this* repo's
own scaffolded output so it dogfoods the new binary — the incubator agent file appears under
`.claude/`, the scaffolded core skills carry the new prose, and `.fledge/scaffold.json` records
the new content hashes. No source changes; this feather rebuilds/reinstalls the binary and runs
`fledge init --refresh`, then verifies the whole repo is green and drift-free.

## Affected Modules
- **Generated scaffold (not source)** — `.claude/agents/fledge-incubator.md` (newly written by
  refresh), `.fledge/skills/fledge-orchestrate/{planning.md,foraging.md}` (updated), and
  `.fledge/scaffold.json` (updated hashes + agent list). See `.fledge/nest/architecture.md` →
  scaffold-stamp/refresh mechanics; CLAUDE.md → "Rebuild, reinstall, and verify".
- No `internal/` or `cmd/` source is edited here — those land in FTHR-014/015.

## Approach
- Follow CLAUDE.md's reinstall discipline first (the embedded content changed in 014/015):
  `go install ./cmd/fledge`, `hash -r`, confirm `fledge version` matches the `VERSION` file.
- Run `fledge init --refresh`; it rewrites unedited scaffolded files to the new embedded
  versions, writes the new incubator agent, preserves any user-edited files (reporting them),
  and updates `.fledge/scaffold.json`. Review `git status` to confirm only expected files
  changed (the incubator agent, the two core skill files, the stamp, and the resolution of the
  pre-existing `.fledge/nest/raw/.gitkeep` scaffold warning).
- Commit the regenerated scaffold as the closeout of the plumage.

## Tests
This is a regeneration/verification feather; its check is an observable drift transition plus a
green suite, not a new unit test:
- **Before**: `fledge preen` reports scaffold drift — `.claude/agents/fledge-incubator.md`
  missing and the two core skill files stale relative to the rebuilt binary's stamp.
- **After** `fledge init --refresh`: `fledge preen` is clean (no drift, no warnings), and
  `exists .claude/agents/fledge-incubator.md`.
- `go test ./...` and `go vet ./...` green after refresh.

## Acceptance Criteria
- [x] AC-1: The drift transition was observed — `fledge preen` showed the incubator missing /
      core stale before `fledge init --refresh`, and reports clean after.
- [x] AC-2: `fledge init --refresh` regenerates this repo's scaffold so
      `.claude/agents/fledge-incubator.md` exists and `.fledge/skills/fledge-orchestrate/`
      carries the FTHR-015 prose, with `.fledge/scaffold.json` updated (satisfies PLM-010 AC-5).
- [x] AC-3: `go test ./...` and `go vet ./...` pass, and `fledge preen` is clean for the whole
      repo.
