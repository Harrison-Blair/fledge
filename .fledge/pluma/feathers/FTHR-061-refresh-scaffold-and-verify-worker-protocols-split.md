---
id: FTHR-061
title: Refresh scaffold and verify worker-protocols split
plumage: PLM-027
status: egg
priority: P1
depends_on: [FTHR-058, FTHR-059, FTHR-060]
authored: 2026-07-16T16:21:11Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-061: Refresh scaffold and verify worker-protocols split

## Description
Sole integration point for PLM-027: once the content split (FTHR-057) and all three reference-repointing feathers (FTHR-058/059/060) have landed, regenerate this repo's own scaffolded copies (`fledge init --refresh`) so `.fledge/skills/fledge-orchestrate/` and `.claude/` reflect the new `internal/bootstrap/core` source, then verify the whole suite is green end to end (satisfies PLM-027 FC-7, AC-6). This feather is deliberately the only one in the plumage that runs `fledge init --refresh` — per this repo's own dogfooding note, concurrent scaffold-refresh feathers collide on the shared `.fledge/scaffold.json` stamp, so it must run after, not in parallel with, B/C/D.

## Affected Modules
Per `.fledge/nest/architecture.md`'s "Scaffolding mechanics" section:
- `.fledge/scaffold.json` (the stamp — rewritten by `--refresh`)
- `.fledge/skills/fledge-orchestrate/{worker-protocols,incubator,brooder,skua}.md` (regenerated copies of the `internal/bootstrap/core` source)
- No other files should change — `.claude/agents/*.md` are symlinks (per architecture.md) so they pick up FTHR-059's edits automatically without a refresh step; `.claude/skills/...` symlinks the whole skill directory per the Claude manifest, so it needs no separate action either.

## Approach
1. Run `fledge init --refresh` (non-interactive is fine here since this is a scripted verification step, not an ad hoc terminal run — but if it would prompt, run it non-interactively with the documented force flag rather than blocking).
2. `git status` / `git diff` to confirm the change set matches expectations: `.fledge/scaffold.json` stamp update, `.fledge/skills/fledge-orchestrate/{worker-protocols,incubator,brooder,skua}.md` written, and nothing else (no unrelated drift).
3. Run `fledge preen` — must pass.
4. Run `go vet ./...`, `go test ./...` (unit tests), and `go test ./cmd/fledge -run TestScripts` (full txtar suite) — all must pass.

## Tests
This feather's "tests" are the full verification suite itself, run as a black-box check of the whole plumage's integration — there's no new unit test to author:
- `fledge preen` (exit 0, no findings)
- `go vet ./...` (no findings)
- `go test ./...` (all packages pass, including the new `incubator_test.go`/`brooder_test.go`/`skua_test.go` from FTHR-057 and the doc-reference tests from FTHR-058/059)
- `go test ./cmd/fledge -run TestScripts` (all txtar fixtures pass, including the updated `forager_contract.txtar`/`init_agents.txtar` from FTHR-060)
- Test-first framing: before this feather's `--refresh` step runs, `fledge preen` is expected to report the scaffold as stale/drifted against the new `internal/bootstrap/core` content (since B/C/D changed the source but this repo's own `.fledge/skills/` copies haven't been resynced yet) — capture that as the "failing for the expected reason" state, then run `--refresh` and confirm `preen` turns clean.

## Acceptance Criteria
- [ ] AC-1: `fledge preen` was observed reporting scaffold drift before `--refresh` (or, if FTHR-058/059/060 already ran `--refresh` incidentally — unlikely per Approach step-ordering but note it if so) and reports clean after.
- [ ] AC-2: `fledge init --refresh` completes with only the expected file set touched (satisfies PLM-027 FC-7).
- [ ] AC-3: `fledge preen` passes (satisfies PLM-027 AC-6).
- [ ] AC-4: `go vet ./...` and `go test ./...` pass (satisfies PLM-027 AC-5).
- [ ] AC-5: `go test ./cmd/fledge -run TestScripts` passes in full (satisfies PLM-027 AC-5).
