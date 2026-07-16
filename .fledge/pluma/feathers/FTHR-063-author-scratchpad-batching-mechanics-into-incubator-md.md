---
id: FTHR-063
title: Author scratchpad batching mechanics into incubator.md
plumage: PLM-028
status: egg
priority: P2
depends_on: [FTHR-057]
authored: 2026-07-16T16:25:35Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-063: Author scratchpad batching mechanics into incubator.md

## Description
Add a new subsection to `incubator.md` (created by FTHR-057) documenting the scratchpad-batching mechanism: the batchable/individual-gate rule, scratchpad file naming and lifecycle, and how the single `GATE review` envelope is reused to relay a batch (satisfies PLM-028 FC-2, FC-3, FC-4, FC-5, FC-6, FC-7, AC-2).

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/incubator.md` (created by FTHR-057; per `.fledge/nest/modules.md` → internal-bootstrap-core).

## Approach
Add a new subsection (e.g. "### Scratchpad batching") under `incubator.md`'s existing structure, near the "Relay envelope" section it extends. Content, drawn directly from PLM-028's resolved interrogation:
- **The rule:** a question is batchable when its answer doesn't change what else needs asking (independent leaves: naming, priority, in/out-of-scope calls, test-framework picks, oversight level); it stays an individually relayed `GATE`/`QUESTION` when it's load-bearing for the rest of the tree (the plumage-breakdown decision, and every spec-draft review gate).
- **Mechanics:** for a batch of resolvable questions, write them all — each with the incubator's recommended answer — to `.fledge/scratch/PLM-<slug-or-###>-questions.md` (or `FTHR-<slug-or-###>-questions.md`), overwriting any prior batch for the same tree (no archiving). Send **one** `GATE review` pointing at the file path with the instruction "answer inline, then Accept" — this reuses the existing `GATE review` envelope (material + Accept/Make changes), not a new envelope kind. On "Accept", re-read the file from disk to pick up the inline answers before proceeding; on "Make changes", wait for a re-send of the same gate. Leave the file on disk once consumed (harmless, gitignored, a paper trail).
- **Scope:** applies to both plumage interrogation (planning.md step 3) and feather interrogation (step 4) — same rule governs both.
- Structural/load-bearing decisions (plumage breakdown, spec-draft review gates) are explicitly called out as staying individually relayed, never placed in a scratchpad batch.

## Tests
- `TestIncubatorDocDescribesScratchpadBatching` (new, `internal/bootstrap`, alongside FTHR-057's doc-structure tests): reads embedded `incubator.md` and asserts it contains the batchable-rule phrase (e.g. "independent leaves"), the scratchpad path pattern (`.fledge/scratch/`), the "one `GATE review`" reuse statement, and mentions both plumage and feather interrogation.
- Implementation order: write the test against `incubator.md` as FTHR-057 leaves it (fails — none of this content exists yet), then add the subsection, confirm it passes.

## Acceptance Criteria
- [x] AC-1: The test listed above was observed failing before implementation and passes after.
- [x] AC-2: `incubator.md` documents the batchable/individual-gate rule, scratchpad naming/lifecycle, and the `GATE review` reuse mechanics (satisfies PLM-028 AC-2).
- [x] AC-3: The new subsection states the batching model applies to both plumage and feather interrogation (satisfies PLM-028 FC-9).
- [x] AC-4: `go test ./internal/bootstrap/...` passes.
