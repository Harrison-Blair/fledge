---
id: PLM-028
title: "Batch interrogation Q&A via a scratchpad document"
status: hatched
priority: P2
authored: 2026-07-16T15:55:04Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# PLM-028: Batch interrogation Q&A via a scratchpad document

## Context
The delegated incubator relays every interrogation question through the orchestrator to the user one at a time (`worker-protocols.md` §Incubator relay envelope, soon `incubator.md` per PLM-027). Each question is a full roundtrip: incubator → orchestrator → user → orchestrator → incubator. For a plumage or feather tree with a dozen resolvable leaves (naming, priority, out-of-scope calls, test-framework picks, oversight level), that's a dozen roundtrips, and the orchestrator — a pure relay holding no planning state — has been observed ballooning to ~300k tokens of context after a single planning phase purely from relaying that volume verbatim.

This plumage introduces a scratchpad document: for a batch of independent, resolvable questions, the incubator writes them all to one file (with its recommended answer alongside each), sends a single `GATE review` pointing at the file path ("answer inline, then Accept"), and re-reads the file once the gate returns. Structural, tree-shaping decisions (the plumage breakdown, final spec-draft review gates) are not affected — they remain individually relayed `GATE`/`QUESTION` messages, since batching only helps when a question's answer doesn't change what else needs asking.

## User Stories
- As the user being interrogated during planning, I want to answer a batch of independent questions in one sitting inside a file, so that I'm not round-tripping through the orchestrator relay once per question.
- As the orchestrator relaying planning-phase gates, I want fewer, smaller relay messages, so that my own context doesn't balloon from carrying verbatim Q&A volume that isn't mine to reason about.
- As the incubator conducting interrogation, I want to batch resolvable questions into a scratchpad file rather than holding each one open in conversation memory, so that my own context stays focused on the current decision tree instead of accumulating already-answered questions.

## Functional Criteria
1. FC-1: A new gitignored directory `.fledge/scratch/` holds scratchpad files; `internal/cli/init.go`'s `gitignoreLines` gains one entry for it (same pattern as the existing `.fledge/roster/` line).
2. FC-2: The incubator may batch a set of **independent, resolvable** interrogation questions (per the rule: batchable when answering one doesn't change what else needs asking — naming, priority, in/out-of-scope calls, test-framework picks, oversight level) into a single scratchpad file instead of one relayed `QUESTION` per item.
3. FC-3: **Structural/load-bearing decisions stay individually relayed** and are never placed in the scratchpad batch: the plumage-breakdown decision (planning.md step 3.2), and every spec-draft review gate (3.4, 4.5/4.6, the create-then-gate reviews).
4. FC-4: The scratchpad file is named `PLM-<slug>-questions.md` (or `FTHR-<slug>-questions.md`) before a real ID exists, or `PLM-###-questions.md`/`FTHR-###-questions.md` once `fledge new` has allocated the ID; it lists each open question with the incubator's recommended answer and a blank/answer line for the user to fill in.
5. FC-5: The relay for a scratchpad batch is a single `GATE review` message pointing at the file path (not pasting question text into the relay) with the instruction "answer inline, then Accept"; this reuses the existing `GATE review` envelope shape (material + Accept/Make changes) rather than introducing a new envelope kind.
6. FC-6: On "Accept", the incubator re-reads the scratchpad file from disk to pick up the user's inline answers before proceeding; on "Make changes" (the user isn't done yet), the incubator waits for a re-send of the same gate.
7. FC-7: The scratchpad file is overwritten (not appended) each time the incubator has a fresh batch for the same tree; consumed/answered batches are not archived — the file reflects only the current open batch. Left on disk (not deleted) once consumed.
8. FC-8: `fledge-interrogate/SKILL.md` gains a documented exception: a delegated incubator may batch multiple resolved questions into one scratchpad file rather than relaying one at a time, per `incubator.md`; the skill's own question-generation logic (walk the tree, one branch resolved before the next, recommended answer first) is otherwise unchanged.
9. FC-9: The batching model applies to both plumage interrogation (planning.md step 3) and feather interrogation (step 4) — the same batchable/individual-gate rule governs both.

## Acceptance Criteria
- [ ] AC-1: `.fledge/scratch/` is gitignored (verify via `git check-ignore .fledge/scratch/test.md` after `fledge init`/`--refresh`), and `internal/cli/init_test.go` or equivalent asserts the new gitignore line.
- [ ] AC-2: `incubator.md` (from PLM-027) documents the batchable/individual-gate rule and the scratchpad file mechanics (naming, single `GATE review` envelope, re-read-on-accept, overwrite-per-batch).
- [ ] AC-3: `fledge-interrogate/SKILL.md` contains the one-line delegated-incubator batching exception.
- [ ] AC-4: `planning.md` steps 3 and 4 reference the scratchpad batching option where they describe interrogation.
- [ ] AC-5: `fledge preen` passes after the change, and `fledge init --refresh` on this repo cleanly picks up the new gitignore line with no unexpected drift.

## Out of Scope
- Any new `fledge` CLI command or subcommand for scratchpad management (creation/reading is plain file I/O via existing `write-file`/`read-only-shell` primitives — no new primitive, no new CLI verb).
- A notification mechanism for "the user edited the file" — per the ground rules, no primitive models this; the existing gate-then-re-read pattern (mirroring the spec-draft path+diff convention) is the whole mechanism, not a polling or push scheme.
- Changing the plumage-breakdown gate, or any final spec-draft review gate, to be batchable — those remain individually relayed per FC-3.
- Retroactively archiving or versioning old scratchpad batches — FC-7 explicitly leaves overwrite semantics, no history.

## Open Questions
None — the interrogation resolved scope (both plumage and feather interrogation), the `fledge-interrogate/SKILL.md` treatment (documented exception), the batchable/individual-gate rule, scratchpad naming/lifecycle, and priority with the user directly.
