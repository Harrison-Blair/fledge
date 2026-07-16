---
id: FTHR-065
title: Reference scratchpad batching in planning.md steps 3 and 4
plumage: PLM-028
status: egg
priority: P2
depends_on: [FTHR-058]
authored: 2026-07-16T16:25:35Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-065: Reference scratchpad batching in planning.md steps 3 and 4

## Description
`planning.md` steps 3 (plumage interrogation) and 4 (feather interrogation) currently describe interrogation as strictly one-relayed-question-at-a-time (via `worker-protocols.md`/`incubator.md`'s relay envelope). Add a reference in each step to the scratchpad-batching option FTHR-063 documents in `incubator.md`, so a delegated incubator following `planning.md` knows the option exists at the point it's making interrogation decisions (satisfies PLM-028 FC-9, AC-4). Depends on FTHR-058 (also edits `planning.md`) to avoid a same-file merge collision — this feather's edits are additive to whatever FTHR-058 leaves behind (the `worker-protocols.md`→`incubator.md` reference repointing).

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/planning.md`, step 3 (plumage interrogation, the "run the interrogate protocol... one question at a time" sentence) and step 4.1 (feather interrogation, "still one question at a time"). Per `.fledge/nest/modules.md` → internal-bootstrap-core.

## Approach
- In step 3 (currently: "run the interrogate protocol from the `fledge-interrogate` skill... one question at a time, recommended answer first..."), add a clause noting that resolvable, independent questions may instead be batched into a scratchpad file per `incubator.md`'s scratchpad-batching rule, when the phase is delegated.
- In step 4.1 (currently: "continue interrogating — still one question at a time — over the decomposition..."), add the same cross-reference for feather-level interrogation.
- Keep both additions short (one clause/sentence each) and point at `incubator.md` for the actual mechanics rather than duplicating them — `planning.md` should say *that* batching is available and *where* the rule lives, not restate the rule.

## Tests
- `TestPlanningDocReferencesScratchpadBatching` (new, `internal/bootstrap`): reads embedded `planning.md` and asserts both step 3 and step 4's interrogation prose mention scratchpad batching (or reference `incubator.md`'s batching rule) alongside the original one-question-at-a-time framing.
- Implementation order: write the test against `planning.md` as FTHR-058 leaves it (fails — no batching reference in either step), add the two clauses, confirm it passes.

## Acceptance Criteria
- [ ] AC-1: The test listed above was observed failing before implementation and passes after.
- [ ] AC-2: `planning.md` step 3 references the scratchpad-batching option for plumage interrogation.
- [ ] AC-3: `planning.md` step 4.1 references the scratchpad-batching option for feather interrogation (satisfies PLM-028 FC-9, AC-4).
- [ ] AC-4: `go test ./internal/bootstrap/...` passes.
