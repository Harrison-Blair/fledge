---
id: FTHR-064
title: Document the batching exception in fledge-interrogate SKILL.md
plumage: PLM-028
status: egg
priority: P2
depends_on: [FTHR-057]
authored: 2026-07-16T16:25:35Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-064: Document the batching exception in fledge-interrogate SKILL.md

## Description
`fledge-interrogate/SKILL.md:8` currently states "Ask the questions one at a time... Asking multiple questions at once is bewildering," with no exception. Add a one-line documented exception for delegated incubators batching resolvable questions via the `incubator.md` scratchpad mechanism (satisfies PLM-028 FC-8, AC-3), without altering the skill's own question-generation logic.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-interrogate/SKILL.md` (line 8 and surrounding prose; per `.fledge/nest/modules.md` → internal-bootstrap-core).

## Approach
Immediately after the existing "Ask the questions one at a time... bewildering" sentence (line 8), add one sentence: an exception noting that a delegated incubator may batch multiple resolved, independent questions into one scratchpad file rather than relaying one at a time, per `incubator.md`'s scratchpad-batching rule (worker-protocols.md's role split) — and that this exception does not change the question-generation approach below (walk the tree, one branch resolved before the next, recommended answer first), only how resolvable answers are delivered. Do not touch any other line in the file.

## Tests
- `TestInterrogateSkillDocumentsBatchingException` (new, `internal/bootstrap`): reads embedded `fledge-interrogate/SKILL.md` and asserts it still contains the original "one at a time... bewildering" sentence (unchanged) AND now also references "incubator.md" and batching/scratchpad in the same vicinity.
- Implementation order: write the test against the unchanged file (fails — no batching exception present yet), add the sentence, confirm it passes.

## Acceptance Criteria
- [x] AC-1: The test listed above was observed failing before implementation and passes after.
- [x] AC-2: `fledge-interrogate/SKILL.md` documents the delegated-incubator batching exception referencing `incubator.md`, while its original one-question-at-a-time instruction remains intact for the general case (satisfies PLM-028 AC-3).
- [x] AC-3: `go test ./internal/bootstrap/...` passes.
