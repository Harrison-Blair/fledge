---
id: FTHR-039
title: Rebuild and resync scaffold for tmux auto-default
plumage: PLM-019
status: egg
priority: P2
depends_on: [FTHR-038]
authored: 2026-07-15T19:02:04Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-039: Rebuild and resync scaffold for tmux auto-default

## Description
Resync this repo's own dogfooded scaffold with FTHR-038's rewritten `team-loop.md` and `implementation.md` source: rebuild the `fledge` binary, run `fledge init --refresh` to regenerate the on-disk `.claude/team-loop.md` copy, the `.fledge/skills/fledge-orchestrate/implementation.md` copy, and `.fledge/scaffold.json`, and confirm the full test suite (including `cmd/fledge/testdata/*.txtar`) stays green — updating any fixture that trips on the new content. Same dogfooding step as FTHR-037, applied to this plumage's two changed files (one of which, `team-loop.md`, is `overwrite`-policy, not symlinked, so it needs the same explicit resync as `implementation.md`'s copy). Satisfies PLM-019 AC-5, AC-6.

## Affected Modules
- Repo root: rebuild via `go install ./cmd/fledge` (installs to `$(go env GOPATH)/bin`, must be on `PATH`; `hash -r` after if the shell caches the old binary).
- `.claude/team-loop.md` — regenerated copy of `internal/bootstrap/adapters/claude/team-loop.md`, write policy `overwrite` (always repaired on refresh) per `.fledge/nest/architecture.md` → "Scaffold write policies and drift".
- `.fledge/skills/fledge-orchestrate/implementation.md` — regenerated copy of the core source, written by `fledge init --refresh`.
- `.fledge/scaffold.json` — the stamp `fledge init --refresh` rewrites with the new content hashes for both files.
- `cmd/fledge/testdata/*.txtar` — verify, don't assume: confirmed via grep during planning that no fixture asserts on tmux-precondition prose content (only `exists .claude/team-loop.md`-style existence checks were found), but this feather must re-verify against the actual FTHR-038 diff and update any fixture that trips.
- See `.fledge/nest/testing.md` → "Acceptance test coverage" and CLAUDE.md's "Rebuild, reinstall, and verify the installed binary" section for the exact rebuild/verify sequence.

## Approach
1. Confirm FTHR-038 is merged to the branch this feather works from (its `depends_on` gate already enforces this at dispatch).
2. `go install ./cmd/fledge && hash -r && command -v fledge && fledge version` — verify the installed binary matches `VERSION`.
3. **Before refreshing**, run `fledge preen -strict` and capture its output: both `.claude/team-loop.md` and `.fledge/skills/fledge-orchestrate/implementation.md` should now be flagged as drifted/stale (the installed binary's embedded content moved on from the still-old on-disk copies) — this is the test-first "failing" capture (see Tests).
4. `fledge init --refresh`. Review `git status`/`git diff` to confirm only `.claude/team-loop.md`, `.fledge/skills/fledge-orchestrate/implementation.md`, and `.fledge/scaffold.json` changed, and that both regenerated copies are byte-identical to their `internal/bootstrap/...` sources.
5. Run `fledge preen -strict` again — capture it passing clean. This is the test-first "passing" capture.
6. `go test ./...` and `go vet ./...` — must be green. If any `cmd/fledge/testdata/*.txtar` fixture fails because it asserted on old prose, update that fixture's expected output to match.
7. Commit the regenerated `.claude/team-loop.md`, `.fledge/skills/.../implementation.md`, `.fledge/scaffold.json` (and any updated txtar fixture) together.

No new Go production code; this feather only runs existing CLI commands and commits their output.

## Tests
This feather's "tests" are the `fledge preen -strict` drift check itself, used as the test-first mechanism (no new Go test file) — same shape as FTHR-037:

- **Failing capture:** immediately after `go install` (step 2) and before `fledge init --refresh` (step 4), run `fledge preen -strict` and record its verbatim output in the evidence file under `## AC-1` — it must show both `.claude/team-loop.md` and `.fledge/skills/fledge-orchestrate/implementation.md` (or the relevant scaffold entries) flagged as stale/drifted, for the expected reason (embedded content changed, on-disk copies didn't).
- **Passing capture:** after `fledge init --refresh` (step 4), run `fledge preen -strict` again and record it passing clean (exit 0, no drift findings) in the same `## AC-1` section.
- **Regression guard:** `go test ./...` full-suite output, captured under `## AC-2`, showing all packages pass including `cmd/fledge` (`TestScripts`).

## Acceptance Criteria
- [x] AC-1: `fledge preen -strict` was observed flagging both `.claude/team-loop.md` and `.fledge/skills/fledge-orchestrate/implementation.md` as stale immediately after rebuild (before refresh), and passing clean after `fledge init --refresh`.
- [x] AC-2: `go test ./...` and `go vet ./...` pass, including all `cmd/fledge/testdata/*.txtar` scripts; any fixture that needed updating for the new prose was updated (satisfies PLM-019 AC-6).
- [x] AC-3: `.claude/team-loop.md` is byte-identical to `internal/bootstrap/adapters/claude/team-loop.md`, `.fledge/skills/fledge-orchestrate/implementation.md` is byte-identical to `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md`, and `.fledge/scaffold.json` reflects both new content hashes (satisfies PLM-019 AC-5).
- [x] AC-4: `git status` shows only the expected files changed (the two regenerated copies, the scaffold stamp, and any updated txtar fixture) — no unrelated drift.
