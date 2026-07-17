---
id: FTHR-076
title: Refresh scaffold and verify suite green
plumage: PLM-030
status: fledged
priority: P1
depends_on: [FTHR-075]
authored: 2026-07-16T22:26:24Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-076: Refresh scaffold and verify suite green

## Description
Sole integration point for PLM-030: once FTHR-072/073/074/075 have all landed (new `internal/ledger` package, `fledge heartbeat`/`await`/`verdict`/`escalate`/`ledger read` commands, and the seven rewritten core prose files), rebuild and reinstall the `fledge` binary (Go source changed, unlike a doc-only plumage), regenerate this repo's own scaffold from the updated `internal/bootstrap/core` sources, and verify the full suite green. Sole owner of the rebuild+refresh mutation, run after (not parallel with) FTHR-072..075 — matches this repo's established closing pattern (e.g. FTHR-066, FTHR-071).

## Affected Modules
- The `fledge` binary itself (rebuilt from `internal/cli`/`internal/ledger` per FTHR-072/073/074's changes).
- `.fledge/scaffold.json`, `.fledge/skills/fledge-orchestrate/{worker-protocols,incubator,brooder,skua,foraging,implementation,planning}.md` (regenerated copies of FTHR-075's rewrite).
- No new source changes of its own — this feather only builds, refreshes, and verifies what FTHR-072..075 already wrote.

## Approach
1. `go install ./cmd/fledge && hash -r && fledge version` — confirm the installed binary matches `VERSION` (per this repo's CLAUDE.md rebuild/reinstall convention).
2. `fledge init --refresh` — regenerate this repo's own scaffolded copies from the updated `internal/bootstrap/core` sources.
3. `git status`/`git diff` — confirm only the expected files changed (scaffold stamp, the seven touched skill docs).
4. `fledge preen`, `go vet ./...`, `go test ./...` (including `-race` for `internal/ledger` per FTHR-072 AC-5), `go test ./cmd/fledge -run TestScripts` — all green.
5. As a live check of PLM-030's own dogfooding potential (not a hard requirement — the ledger doesn't need to be in active use by this planning phase to close, since this phase itself predates the feature): confirm `fledge heartbeat --help`, `fledge await --help`, `fledge verdict --help`, `fledge escalate --help`, `fledge ledger read --help` all resolve on the freshly installed binary, so the very next implementation phase that dispatches this plumage's own feathers can actually use the commands it's about to build.

## Tests
Black-box verification suite, no new unit test:
- `fledge preen` (clean)
- `go vet ./...` (clean)
- `go test ./...` (all packages, including FTHR-072's `internal/ledger` race-enabled tests and FTHR-075's new prose guard tests)
- `go test ./cmd/fledge -run TestScripts` (full txtar suite, including FTHR-072/073/074's new `heartbeat.txtar`/`await.txtar`/`verdict.txtar`/`escalate.txtar`/`ledger-read.txtar` fixtures)
- Test-first framing: before rebuild+refresh, `fledge preen`/`fledge version` are expected to show the old binary and a stale scaffold; capture that as the pre-state, then confirm clean after.

## Acceptance Criteria
- [x] AC-1: `fledge version` matches `VERSION` after reinstall.
- [x] AC-2: `fledge init --refresh` completes with only the expected file set touched.
- [x] AC-3: `fledge preen` passes.
- [x] AC-4: `go vet ./...` and `go test ./... -race` pass, satisfying PLM-030 AC-6.
- [x] AC-5: `go test ./cmd/fledge -run TestScripts` passes in full, including every new ledger-related txtar fixture, satisfying PLM-030 AC-2/AC-3/AC-5/AC-6.
- [x] AC-6: All five new commands (`heartbeat`, `await`, `verdict`, `escalate`, `ledger read`) resolve `--help` on the freshly reinstalled binary.
