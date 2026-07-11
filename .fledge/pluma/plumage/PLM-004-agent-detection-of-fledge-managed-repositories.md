---
id: PLM-004
title: Agent detection of fledge-managed repositories
status: fledged
priority: P1
authored: 2026-07-08T05:58:02Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# PLM-004: Agent detection of fledge-managed repositories

## Context
When a coding agent (Claude Code specifically) opens a fledge-managed repository, it
frequently fails to recognize that the repo is fledge-managed and improvises spec/feature
work by hand instead of routing through the orchestration workflow. The root cause is a
scaffolding gap: `fledge init` for the Claude harness emits `.claude/` adapter files and
skill symlinks, but no top-level instruction that the harness is guaranteed to read on
entry. Claude Code auto-loads only its canonical project-memory file (not `AGENTS.md`),
and skills surface only by description when a request already matches a trigger phrase — so
nothing reliably tells the agent, up front, that this repo drives work through fledge. The
Codex adapter already emits an equivalent top-level pointer; the Claude adapter should too.
This repository only routes correctly today because its project-memory file was authored by
hand — a freshly initialized Claude repo gets no such guidance.

## User Stories
- As a developer who runs `fledge init` in a Claude Code repository, I want the agent to
  recognize on entry that the repo is fledge-managed, so that it routes feature, spec, and
  implementation requests through the orchestration workflow instead of improvising.
- As a maintainer, I want that detection pointer scaffolded automatically and
  non-destructively, so that any existing project instructions are preserved and repeated
  initialization stays safe and idempotent.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: `fledge init` for the Claude harness ensures a top-level instruction, in the
   project-memory file Claude Code auto-loads, that marks the repository as fledge-managed
   and directs the agent to the orchestration workflow and the Claude primitive map.
2. FC-2: The pointer is injected additively — pre-existing project-memory content is
   preserved verbatim, and the operation is idempotent across repeated `init` / `--refresh`.
3. FC-3: The pointer's wording is consistent with the equivalent pointer emitted for other
   harnesses (e.g. Codex), so the detection cue is uniform across adapters.

## Acceptance Criteria
- [x] AC-1: Initializing a Claude repository that has no pre-existing project-memory file produces one containing the fledge detection pointer.
- [x] AC-2: Initializing a Claude repository that already has a project-memory file appends the pointer without altering existing content, and repeating initialization neither duplicates nor rewrites it.
- [x] AC-3: The scaffolded pointer directs the agent to the fledge orchestration skill and the Claude adapter/primitive map, matching the wording used for the Codex adapter.
- [x] AC-4: Automated acceptance tests cover AC-1..AC-3 and the full test suite passes.

## Out of Scope
- Native skill auto-discovery and the `.claude/settings.json` `skills`-pointer discrepancy
  (the adapter doc claims discovery via a `settings.json` pointer that the scaffolded file
  does not contain) — logged as a separate investigation.
- Skill-description / trigger-phrase tuning.
- A cross-agent shared `AGENTS.md` source pattern.
- Terminology comprehension (covered by a separate plumage).
- Non-Claude adapters (Codex already emits its pointer; pi is out of scope).

## Open Questions
None — all decisions resolved during the 2026-07-08 interrogation.
