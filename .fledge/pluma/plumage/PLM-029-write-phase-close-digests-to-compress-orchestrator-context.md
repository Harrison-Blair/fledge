---
id: PLM-029
title: Write phase-close digests to compress orchestrator context
status: fledged
priority: P2
authored: 2026-07-16T16:07:27Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# PLM-029: Write phase-close digests to compress orchestrator context

## Context
The main orchestrator is a pure relay across every phase of the fledge workflow (planning, foraging, implementation) — it holds no domain state of its own, but today it accumulates the *entire* conversation history of every phase it drives or relays: every relayed question, every gate, every worker message. The user observed the orchestrator's context ballooning to ~300k tokens after a single planning phase, purely from carrying that verbatim relay volume forward into the next phase. Nothing about that history is needed once a phase closes — only its outcome is.

This plumage introduces a phase-close digest: whoever ran a phase (the incubator for planning, the forager for foraging, the orchestrator itself for implementation) writes a short digest file summarizing that phase's outcome — what was decided, what was produced, where to look — before the phase is considered closed. The next phase's prose is written to read the digest instead of relying on the conversation to still contain the prior phase's detail. On Claude Code specifically, once the digest is written the orchestrator's user-facing close-out message notes that `/compact` is now safe to run if the session's context has grown large — compaction itself stays a user decision, not an automated step, since no fledge primitive models an agent compacting its own context.

## User Stories
- As the user running a multi-phase fledge workflow (plan → forage → implement) in one long session, I want each phase to leave behind a short written summary of its outcome, so that I can safely `/compact` between phases without losing anything the next phase needs.
- As the orchestrator starting implementation after planning closed out, I want to read a short planning digest instead of re-deriving "what got decided" from scrollback, so that my context stays proportional to the current phase's work rather than accumulating every prior phase's detail.
- As a maintainer debugging a workflow run, I want the digest files on disk as a lightweight paper trail of what each phase concluded, so that I can reconstruct what happened without replaying the full conversation.

## Functional Criteria
1. FC-1: Each of the three phases writes a digest to its own file when it closes: planning → `.fledge/scratch/digest-planning.md`, foraging → `.fledge/scratch/digest-foraging.md`, implementation → `.fledge/scratch/digest-implementation.md` (same gitignored `.fledge/scratch/` directory introduced by PLM-028). Written via plain file I/O (`write-file`) — no new `fledge` CLI command.
2. FC-2: A digest is written by whoever ran that phase — the incubator for planning (as part of its `PHASE-CLOSE`), the forager for foraging (as part of its coverage-summary final message), the orchestrator itself for implementation (once feathers are merged and verified) — never by a different party after the fact.
3. FC-3: A digest's content is: the phase's outcome (what was produced — e.g. hatched plumage/feather IDs and file paths, or merged feather IDs), the key user decisions made during the phase (not the full Q&A transcript — just the resolved decisions and their rationale), and pointers to the specs/files involved. It is prose, not a transcript replay.
4. FC-4: Each digest file is overwritten on that phase's next close (not appended/versioned) — only the latest close's outcome is relevant to what comes next.
5. FC-5: `planning.md`, `foraging.md`, and `implementation.md` are each updated: (a) their phase-close step now includes writing the digest per FC-1–FC-4, and (b) their opening steps note that a prior-phase digest, if present at its known path, should be read as grounding context instead of assuming the conversation still holds it — reading it is best-effort (missing file = proceed without it, e.g. on a phase's first-ever run).
6. FC-6: The Claude Code piping notes (`.claude/team-loop.md` or the relevant adapter piping doc) document that once a phase's digest is written, the orchestrator's close-out message to the user includes a one-line note that `/compact` is now safe to run if the session's context is large. This is advisory prose only — no automated compaction trigger, since Claude Code exposes no mechanism for an agent to compact its own context mid-session.
7. FC-7: Codex/pi adapters (Tier A, solo in-session) are unaffected — they have no orchestrator/worker relay to compress in the first place, so no piping-note change is needed there.

## Acceptance Criteria
- [x] AC-1: `planning.md`'s phase-close step (3.4/4.7 closing report) includes writing `digest-planning.md`; `foraging.md`'s Commissioner section's forager-final-message step includes writing `digest-foraging.md`; `implementation.md`'s closing step includes writing `digest-implementation.md`.
- [x] AC-2: Each of the three phase files' opening step references reading its predecessor's digest (if present) as grounding context.
- [x] AC-3: `.claude/team-loop.md` (or the correct piping doc) documents the `/compact`-is-safe-now advisory note tied to digest completion.
- [x] AC-4: A sample digest file for at least one phase (e.g. this very planning phase's `digest-planning.md`, written when this plumage's own planning phase closes) demonstrates the format: outcome, key decisions, spec pointers — no full Q&A transcript.
- [x] AC-5: `fledge preen` passes after the change; `fledge init --refresh` on this repo shows no unexpected drift beyond the intended prose changes.

## Out of Scope
- Any new `fledge` CLI command for digest read/write (plain file I/O, per FC-1 — consistent with PLM-028's scratchpad mechanism).
- Automated `/compact` invocation, or any equivalent for other harnesses — FC-6 is advisory prose to the user only.
- Digest history/versioning — FC-4 explicitly overwrites; no archive of past phase-close digests.
- Codex/pi adapter changes — FC-7 explicitly excludes them.
- Changing what a `PHASE-CLOSE`/coverage-summary message itself contains when relayed to the user — the digest is a separate on-disk artifact for the *next phase's agent*, not a replacement for the existing user-facing close-out report.

## Open Questions
None — the interrogation resolved phase scope (all three), file location/naming (gitignored scratch, per-phase-kind, overwritten), mechanism (plain file I/O), the Claude compaction note's meaning (user-facing advisory, not automated), and priority with the user directly.
