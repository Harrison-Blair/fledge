---
id: FTHR-040
title: Harden forager wait-contract prose and regenerate scaffold
plumage: PLM-020
status: fledged
priority: P1
depends_on: []
authored: 2026-07-15T22:00:13Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-040: Harden forager wait-contract prose and regenerate scaffold

## Description
Rewrite the two orchestrator/incubator-facing source-of-truth files — `internal/bootstrap/core/skills/fledge-orchestrate/planning.md` §0–2 and `worker-protocols.md` §Incubator — so the forager wait-contract is a strict two-input state machine (final message = done; prolonged silence = suspected stall → existing escalation) with an explicit statement that on-disk `.fledge/nest/` state is never an input, and so that neither file names or describes the forager's internal pipeline stages or its stall failure mode. Add a committed txtar test pinning this. Regenerate this repo's own scaffold from the edited source and keep the full txtar suite green.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/planning.md` — see .fledge/nest/architecture.md "the agent-neutral orchestration workflow (planning/implementation phases...)"; the specific paragraphs are §0 ("If you provide both spawn-worker and message-peer") and §2 ("If a forager can be obtained" + the suspected-stall paragraph + the step-38 note after the bullet list).
- `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` § Incubator — the "Foraging:" paragraph (~line 32) that mirrors the same wait-contract.
- `cmd/fledge/testdata/forager_contract.txtar` (new file) — see .fledge/nest/testing.md for the testscript/txtar framework and how `init.txtar` structures `exec fledge init` + `exists`/`grep` assertions against generated scaffold output; this feather follows that same pattern.
- Possibly `cmd/fledge/testdata/init.txtar`, `init_agents.txtar`, `agents.txtar` if any existing `grep` assertion in them happens to pin substrings this feather removes or changes (check with `grep -rn` for the forbidden/changed strings across `cmd/fledge/testdata/*.txtar` before editing).
- This repo's own scaffolded copies (`.fledge/skills/fledge-orchestrate/planning.md`, `.fledge/skills/fledge-orchestrate/worker-protocols.md`, and `.fledge/scaffold.json`) — regenerated, not hand-edited, via `fledge init --refresh` after reinstalling.
- Do NOT touch `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md`, `internal/bootstrap/adapters/claude/agents/fledge-forager.md`, `team-loop.md`, or `implementation.md` — out of scope per PLM-020.

## Approach
1. In `worker-protocols.md` § Incubator, rewrite the "Foraging:" paragraph in place (no new heading) to state only: the forager's by-name final message is the sole "done" signal; a bare idle notification is not completion; on-disk `.fledge/nest/` state (including a half-populated nest mid-synthesis) is never evidence of anything and is never inspected as part of this decision; on idle-with-no-final-message, apply the unchanged suspected-stall procedure (≤3 by-name queries ~2 min apart, then escalate to the user via `confirm-gate` through the orchestrator). Drop the current parenthetical explanation ("the half-filled nest during synthesis is not a stall") entirely rather than just softening it — replace it with the flat "never an input" statement so no pipeline-stage vocabulary remains.
2. In `planning.md` §2, rewrite the "If a forager can be obtained" paragraph and the suspected-stall paragraph immediately after it, in place, to the same two-input framing. Remove: "the failure mode the protocol warns about" phrasing, "a forager fans out scouts as its own background workers, so such a notification is structurally expected *mid-pipeline* (the moment its scouts finish but before it has synthesized) and inspecting the half-filled nest then will mislead you into declaring a stall", and the closing note "Note: immediately after `fledge nest scaffold`, `.fledge/nest/` holds only empty template stubs — see `foraging.md` for what that expected intermediate state looks like and why it is not a failure." Replace with the flat two-input statement and the explicit "never an input" line; it is fine (per PLM-020 FC-3's caution) to still name `fledge-forager`/`fledge-context-scout` worker types where §0/§2 already do so for spawning purposes — only the *pipeline-stage and failure-mode* vocabulary is forbidden, not the worker-type nouns themselves.
3. Do not alter the suspected-stall escalation mechanics (query count, ~2 min interval, 3-query cap, `confirm-gate` decision) — reframe the surrounding prose only, per PLM-020 FC-4.
4. After the source edit, reinstall (`go install ./cmd/fledge && hash -r && fledge version` — confirm it matches `VERSION`) and run `fledge init --refresh` in this repo to regenerate `.fledge/skills/...`. Review `git status`/`git diff` on the regenerated files to confirm they reflect the intended prose only.
5. Run `go test ./cmd/fledge -run TestScripts` and `go vet ./...`; fix any other txtar fixture whose `grep` assertions now fail because they pinned removed language.

## Tests
- `cmd/fledge/testdata/forager_contract.txtar` (new): `exec git init -q .` → `exec fledge init` → then, against the generated `.fledge/skills/fledge-orchestrate/planning.md` and `.fledge/skills/fledge-orchestrate/worker-protocols.md`:
  - Forbidden (must NOT match — pipeline-stage/failure-mode leakage): `step-4→step-5`, `synthesis boundary`, `half-filled nest`, `mid-pipeline`, `see \`foraging.md\` for what that expected intermediate state looks like`.
  - Required (must match — hardened two-input framing): a line asserting the "only" / sole determinant framing of the final message as the done signal (e.g. grep for `only** signal that it is done is its explicit final message` or the post-edit equivalent phrase actually written), and a line containing the literal phrase `never an input` (from the "on-disk state ... is never an input" statement) in both files.
  - Use testscript's negative-match syntax (`! grep '<pattern>' <file>`) for the forbidden set and plain `grep '<pattern>' <file>` for the required set, following `init.txtar`'s existing `grep`-against-generated-file pattern.
- Write this test first against the current (unedited) source, run `go test ./cmd/fledge -run TestScripts/forager_contract`, and confirm it FAILS for the expected reason (forbidden strings present, required strings absent) — capture that output. Then make the source edit and confirm the same test passes.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `worker-protocols.md` § Incubator and `planning.md` §2 state the two-input contract (final message = done; prolonged silence = suspected stall → existing escalation) and explicitly state on-disk `.fledge/nest/` state is never an input (satisfies PLM-020 FC-1, FC-2).
- [x] AC-3: Neither file contains forager-internal pipeline-stage or stall-failure-mode language (satisfies PLM-020 FC-3), per `forager_contract.txtar`'s forbidden-string assertions.
- [x] AC-4: The suspected-stall escalation mechanics (≤3 queries, ~2 min apart, `confirm-gate` escalation) are unchanged in substance, verified by reading the rewritten paragraphs against the original (satisfies PLM-020 FC-4).
- [x] AC-5: `fledge init --refresh` was run in this repo after reinstalling, the regenerated scaffold reflects the new prose, and `go test ./cmd/fledge -run TestScripts` plus `go vet ./...` pass in full, including any other txtar fixture updated for the removed language (satisfies PLM-020 AC-4).
