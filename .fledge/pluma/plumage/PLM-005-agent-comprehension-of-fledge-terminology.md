---
id: PLM-005
title: Agent comprehension of fledge terminology
status: hatched
priority: P2
authored: 2026-07-08T06:05:48Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# PLM-005: Agent comprehension of fledge terminology

## Context
fledge's vocabulary is bird-themed and load-bearing — command names, package names, agent
roles, lifecycle states, and spec artifacts all use it (plumage, feather, nest, brood, molt,
preen, vee, colony, forager, skua, tier, oversight, and more). Coding agents frequently do
not innately understand these terms, and today the definitions are scattered and partial:
`README.md` decodes only four of them (and is not auto-loaded), `SKILL.md` inlines three
(and only when routing), and the nest `domain.md` glossary is planning-context that is
regenerated per-repo and not authoritative. Nothing that an agent reliably reads defines a
term at the moment it encounters one — so agents guess at meaning while reading specs, CLI
output, or commit messages. fledge's own philosophy is that authoritative facts live in the
binary and are surfaced through the CLI; terminology should follow the same pattern.

## User Stories
- As a coding agent working in a fledge repo, I want an authoritative, on-demand definition
  of any fledge term, so that I can act on specs and CLI output without guessing at the
  vocabulary.
- As a developer, I want a single command that decodes fledge's terminology, so that the
  glossary can't drift out of sync with the tool the way copies in prose docs do.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: A CLI command emits an authoritative, categorized glossary of every load-bearing
   fledge term, and can resolve a single term on request.
2. FC-2: The glossary is the single source of truth, maintained in the binary, and cannot
   silently fall behind the CLI's own command set or lifecycle states.
3. FC-3: Each harness whose adapter has a guaranteed-read entry file carries an always-loaded
   cue directing the agent to that command when it meets unfamiliar vocabulary.

## Acceptance Criteria
- [ ] AC-1: `fledge glossary` prints the full categorized lexicon; `fledge glossary <term>` prints a single term's definition; an unknown term exits non-zero with a suggestion; `--json` yields structured output.
- [ ] AC-2: The lexicon covers every load-bearing term across the five categories (artifacts, lifecycle, operations, agent roles, structural concepts).
- [ ] AC-3: An automated drift-guard asserts the glossary includes every registered command name and every lifecycle status.
- [ ] AC-4: `fledge init` scaffolds an always-loaded glossary cue into each adapter's guaranteed-read entry file (Claude, Codex) directing the agent to `fledge glossary` — additive, idempotent, and non-clobbering.
- [ ] AC-5: Automated tests (command behavior, init scaffolding, and the drift-guard) cover AC-1..AC-4 and the full suite passes.

## Out of Scope
- Refactoring `README.md`, `SKILL.md`, or the nest `domain.md` to derive from the glossary —
  they remain independently maintained.
- The pi adapter (no guaranteed-read root entry file today); AC-4 covers Claude and Codex.
- Per-term examples, etymology, or extended prose beyond concise definitions.
- Any change to the PLM-004 detection pointer line (the glossary cue is a distinct line).
- Detection itself (PLM-004).

## Open Questions
None — all decisions resolved during the 2026-07-08 interrogation. Depends on PLM-004: the
glossary cue's always-loaded home is the entry file PLM-004 establishes.
