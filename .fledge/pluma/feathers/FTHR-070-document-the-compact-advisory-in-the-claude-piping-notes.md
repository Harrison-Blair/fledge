---
id: FTHR-070
title: Document the compact advisory in the Claude piping notes
plumage: PLM-029
status: fledged
priority: P2
depends_on: []
authored: 2026-07-16T16:38:10Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-070: Document the compact advisory in the Claude piping notes

## Description
Add the `/compact`-is-safe-now advisory to the Claude Code piping notes: once a phase's digest is written, the orchestrator's close-out message to the user should note that compaction is now safe if the session's context is large (satisfies PLM-029 FC-6, AC-3). Advisory prose only — no automated trigger.

## Affected Modules
- `internal/bootstrap/adapters/claude/team-loop.md` (source; scaffolded to `.claude/team-loop.md`) — the "## Planning delegation" section (lines 31-38) is the natural home, since it's where Claude-specific relay behavior around `PHASE-CLOSE` is already documented; extend it (or add a small adjacent subsection) to cover all three phase closes, not just planning's.

## Approach
Add a short paragraph (in "## Planning delegation" or a new "## Digest and compaction" section immediately after it) stating: once a phase's digest file is written (`digest-planning.md`/`digest-foraging.md`/`digest-implementation.md` per `planning.md`/`foraging.md`/`implementation.md`), the orchestrator's close-out reply to the user includes a one-line note that `/compact` is safe to run now if the session's context has grown large. Make explicit that this is user-facing guidance only — Claude Code exposes no mechanism for an agent to compact its own context mid-session, so there is no automated trigger to wire up.

## Tests
- `TestTeamLoopDocDescribesCompactAdvisory` (new, `internal/bootstrap`, alongside other adapter-doc tests — confirm exact test file placement via `grep -rl "team-loop.md\|team_loop" internal/bootstrap/**/*_test.go` first): reads the embedded `team-loop.md` and asserts it mentions `/compact` alongside "digest" and states it's user-facing/advisory (not automated).
- Implementation order: write the test against the unchanged file (fails — no compact mention yet), add the paragraph, confirm it passes.

## Acceptance Criteria
- [x] AC-1: The test listed above was observed failing before implementation and passes after.
- [x] AC-2: `team-loop.md` documents the `/compact`-is-safe-now advisory tied to digest completion, explicitly framed as user-facing guidance rather than an automated step (satisfies PLM-029 AC-3).
- [x] AC-3: `go test ./internal/bootstrap/...` passes.
