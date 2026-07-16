---
id: FTHR-060
title: Repoint txtar fixture assertions to per-role files
plumage: PLM-027
status: egg
priority: P1
depends_on: [FTHR-057]
authored: 2026-07-16T16:17:46Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-060: Repoint txtar fixture assertions to per-role files

## Description
Two txtar acceptance fixtures assert directly on `.fledge/skills/fledge-orchestrate/worker-protocols.md` content that FTHR-057 moves into `incubator.md`. Update both fixtures' assertions to target the new per-role file(s) that now hold that content (satisfies PLM-027 FC-4/FC-6, the txtar-fixture half).

## Affected Modules
Per `.fledge/nest/modules.md` (cmd) and `.fledge/nest/testing.md`:
- `cmd/fledge/testdata/forager_contract.txtar` — lines 16-20 and 26-27 assert forager wait-contract phrasing against `worker-protocols.md`.
- `cmd/fledge/testdata/init_agents.txtar` — line 160 asserts a 3x-repeated lifecycle force-terminate phrase against `worker-protocols.md`.

## Approach
- `forager_contract.txtar` lines 16-20 (the `! grep '...' .fledge/skills/fledge-orchestrate/worker-protocols.md` block forbidding pipeline-stage/failure-mode leakage) and lines 26-27 (the `grep '...' .fledge/skills/fledge-orchestrate/worker-protocols.md` block requiring the hardened two-input framing) both check content that lives in the Incubator section (the commissioner/forager-wait prose) — repoint both blocks' target file from `worker-protocols.md` to `incubator.md`. Leave the parallel `planning.md` assertions (lines 10-14, 23-24) and the `foraging.md` assertions (lines 34-45) untouched — they're unaffected by this split.
- `init_agents.txtar:160` — currently `grep -count=3 '<phrase>' .fledge/skills/fledge-orchestrate/worker-protocols.md`, asserting the same lifecycle force-terminate sentence appears 3× in the one combined file (once per role's Lifecycle subsection). Post-split, each occurrence lives in a different file — replace with three separate `grep -count=1 '<phrase>' .fledge/skills/fledge-orchestrate/<role>.md` lines (one each for `incubator.md`, `brooder.md`, `skua.md`), preserving the same total-count intent (verify all three lifecycle sections still carry the sentence) without relying on a single file holding all three.
- Update the comment above `init_agents.txtar:151-154` to describe checking three files instead of one combined file's three subsections.

## Tests
The tests ARE the txtar scripts themselves (testscript acceptance fixtures, run via `go test ./cmd/fledge -run TestScripts`) — there's no separate unit test to write. Test-first cycle:
- Run `go test ./cmd/fledge -run TestScripts/forager_contract` and `go test ./cmd/fledge -run TestScripts/init_agents` against the unchanged repo state at this feather's start (i.e., after FTHR-057 has landed but before this feather's edits) and confirm they FAIL — the `grep`/`! grep` lines against `worker-protocols.md` will no longer find the moved content, or will find it now-absent, for the expected reason (content relocated to `incubator.md`).
- Edit the two txtar files per Approach, then confirm both scripts PASS.

## Acceptance Criteria
- [ ] AC-1: `go test ./cmd/fledge -run TestScripts/forager_contract` and `TestScripts/init_agents` were observed failing (against post-FTHR-057, pre-this-feather state) and pass after this feather's edits.
- [ ] AC-2: `forager_contract.txtar`'s forbidden/required-phrase blocks (formerly lines 16-20, 26-27) target `incubator.md` instead of `worker-protocols.md`; the `planning.md`/`foraging.md` blocks are unchanged (satisfies PLM-027 FC-6).
- [ ] AC-3: `init_agents.txtar`'s force-terminate lifecycle assertion checks all three of `incubator.md`, `brooder.md`, `skua.md` (one occurrence each) instead of a 3x-count on `worker-protocols.md` (satisfies PLM-027 FC-6).
- [ ] AC-4: `go test ./cmd/fledge -run TestScripts` passes in full (no other fixture regresses).
