---
id: FTHR-037
title: Rebuild and resync scaffold for hardened skua prose
plumage: PLM-018
status: fledged
priority: P2
depends_on: [FTHR-036]
authored: 2026-07-15T18:48:30Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-037: Rebuild and resync scaffold for hardened skua prose

## Description
Resync this repo's own dogfooded scaffold with FTHR-036's rewritten `worker-protocols.md` source: rebuild the `fledge` binary, run `fledge init --refresh` to regenerate the on-disk `.fledge/skills/fledge-orchestrate/worker-protocols.md` copy and `.fledge/scaffold.json`, and confirm the full test suite (including `cmd/fledge/testdata/*.txtar`) stays green — updating any fixture that trips on the new content. This is the mandatory "rebuild + `fledge init --refresh`" dogfooding step CLAUDE.md requires after any `internal/bootstrap/core/` change; it has no independent behavior of its own beyond making FTHR-036's already-merged source change visible in this repo's committed copies. Satisfies PLM-018 AC-5, AC-6.

## Affected Modules
- Repo root: rebuild via `go install ./cmd/fledge` (installs to `$(go env GOPATH)/bin`, must be on `PATH`; `hash -r` after if the shell caches the old binary).
- `.fledge/skills/fledge-orchestrate/worker-protocols.md` — regenerated copy, written by `fledge init --refresh` (default write policy: copy, skip-if-exists normally, but `--refresh` force-resyncs it since it's fledge-owned and the embedded content changed — see `.fledge/nest/architecture.md` → "Scaffold write policies and drift").
- `.fledge/scaffold.json` — the stamp `fledge init --refresh` rewrites with the new content hash for `worker-protocols.md`.
- `cmd/fledge/testdata/*.txtar` — verify, don't assume: no fixture is known to assert on `worker-protocols.md`'s prose content (confirmed via grep during planning — only `exists .claude/team-loop.md`-style existence checks were found for the adjacent tmux file, and no fixture referenced skua review wording at all), but this feather must re-verify against the actual FTHR-036 diff, not this planning-time grep, and update any fixture that does trip.
- See `.fledge/nest/testing.md` → "Acceptance test coverage" and CLAUDE.md's "Rebuild, reinstall, and verify the installed binary" section for the exact rebuild/verify sequence.

## Approach
1. Confirm FTHR-036 is merged to the branch this feather works from (its `depends_on` gate already enforces this at dispatch).
2. `go install ./cmd/fledge && hash -r && command -v fledge && fledge version` — verify the installed binary matches `VERSION`.
3. **Before refreshing**, run `fledge preen -strict` and capture its output: `worker-protocols.md` should now be flagged as drifted/stale (the installed binary's embedded content moved on from the still-old on-disk copy) — this is the test-first "failing" capture (see Tests).
4. `fledge init --refresh` (non-interactive is fine here since this is fledge's own repo and the change is expected/intended — no unexpected user-edit conflict). Review `git status`/`git diff` to confirm only `.fledge/skills/fledge-orchestrate/worker-protocols.md` and `.fledge/scaffold.json` changed, and that the regenerated copy is byte-identical to `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md`.
5. Run `fledge preen -strict` again — capture it passing clean. This is the test-first "passing" capture.
6. `go test ./...` and `go vet ./...` — must be green. If any `cmd/fledge/testdata/*.txtar` fixture fails because it asserted on old prose, update that fixture's expected output to match (per CLAUDE.md: any change to `internal/bootstrap/core/` content must update `init.txtar`/`init_agents.txtar`/`agents.txtar` if they assert on it).
7. Commit the regenerated `.fledge/skills/...` copy and `.fledge/scaffold.json` (and any updated txtar fixture) together.

No new Go production code; this feather only runs existing CLI commands and commits their output.

## Tests
This feather's "tests" are the `fledge preen -strict` drift check itself, used as the test-first mechanism (no new Go test file):

- **Failing capture:** immediately after `go install` (step 2) and before `fledge init --refresh` (step 4), run `fledge preen -strict` and record its verbatim output in the evidence file under `## AC-1` — it must show `worker-protocols.md` (or the relevant scaffold file) flagged as stale/drifted, for the expected reason (embedded content changed, on-disk copy didn't).
- **Passing capture:** after `fledge init --refresh` (step 4), run `fledge preen -strict` again and record it passing clean (exit 0, no drift findings) in the same `## AC-1` section.
- **Regression guard:** `go test ./...` full-suite output, captured under `## AC-2`, showing all packages pass including `cmd/fledge` (`TestScripts`).

## Acceptance Criteria
- [x] AC-1: `fledge preen -strict` was observed flagging `worker-protocols.md` as stale immediately after rebuild (before refresh), and passing clean after `fledge init --refresh`.
- [x] AC-2: `go test ./...` and `go vet ./...` pass, including all `cmd/fledge/testdata/*.txtar` scripts; any fixture that needed updating for the new prose was updated (satisfies PLM-018 AC-6).
- [x] AC-3: `.fledge/skills/fledge-orchestrate/worker-protocols.md` is byte-identical to `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md`, and `.fledge/scaffold.json` reflects the new content hash (satisfies PLM-018 AC-5).
- [x] AC-4: `git status` shows only the expected files changed (the regenerated copy, the scaffold stamp, and any updated txtar fixture) — no unrelated drift.
