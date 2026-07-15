---
id: PLM-019
title: Tmux auto-default (no prompt)
status: hatched
priority: P2
authored: 2026-07-15T18:58:15Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# PLM-019: Tmux auto-default (no prompt)

## Context
Fledge's Tier C team loop (Claude Code) runs teammates in tmux panes so the orchestrator can watch them work. Today, per `internal/bootstrap/adapters/claude/team-loop.md` § Teammate display (tmux), the precondition `test -n "$TMUX"` is surfaced via a `confirm-gate` (`implementation.md` §1) when unmet: the user must choose to stop and restart inside tmux, or proceed degraded with in-process teammates. This blocks every Tier C run that doesn't happen to start inside a tmux session, for a decision that has an obvious, low-stakes default either way (tmux present → use it; absent → degrade). This plumage removes the gate specifically for this precondition: the orchestrator auto-resolves it — degraded when tmux is unavailable, with panes when it is — and reports which path it took as plain run narration, never as a blocking prompt. The separate permission-mode precondition (also gated in `team-loop.md`, also referenced by the same `implementation.md` §1 sentence today) is explicitly untouched — this plumage narrows the neutral doc's "never silently proceed" instruction so it no longer implies tmux must gate, while leaving it intact for permission-mode.

## User Stories
- As a Claude Code user running fledge's implementation phase, I want tmux availability to auto-resolve without a prompt, so that starting a Tier C run outside tmux doesn't force me to stop and restart just to get past a low-stakes default.
- As a Claude Code user, I want to know (without being blocked) whether my run is using split-pane display or running degraded, so I'm not left guessing why I don't see teammate panes.

## Functional Criteria
1. FC-1: When resolving Tier C scope (`implementation.md` §1), if the tmux precondition (`test -n "$TMUX"`) is unmet, the orchestrator proceeds automatically with in-process teammates (no panes) — no `confirm-gate`, no wait for user input.
2. FC-2: If the tmux precondition is met, the orchestrator proceeds automatically with teammates in tmux panes — likewise no `confirm-gate`.
3. FC-3: In both FC-1 and FC-2, the orchestrator reports which path it took as one line of non-blocking run narration (not a gate, no wait-for-response) — e.g. "tmux not detected — proceeding degraded, in-process teammates" or "tmux detected — spawning teammates in panes."
4. FC-4: `implementation.md` §1's neutral "Tier C only — harness piping preconditions" language is reworded so its "never silently proceed past a precondition your piping file says to surface" instruction is scoped to preconditions the piping file actually says to gate — it must no longer imply the tmux precondition gates, since per FC-1/FC-2 it now doesn't.
5. FC-5: The permission-mode precondition (`team-loop.md`'s separate confirm-gate: switch to a non-prompting mode or stop) is unchanged — still gated, still covered by "never silently proceed" — this plumage does not touch its behavior or wording beyond what FC-4's rewording incidentally requires for the sentence to stay accurate.
6. FC-6: No other `team-loop.md` or `implementation.md` behavior changes — communication topology, spawning, shutdown, planning delegation, team task list, skill loading, and recovery sections are untouched.

## Acceptance Criteria
- [ ] AC-1: `team-loop.md` § Teammate display (tmux) states the auto-resolve behavior (FC-1, FC-2) with no confirm-gate language remaining for this precondition.
- [ ] AC-2: `team-loop.md` § Teammate display (tmux) (or `implementation.md` §1, wherever it's most natural) documents the non-blocking notice (FC-3).
- [ ] AC-3: `implementation.md` §1's "never silently proceed" sentence is scoped so it no longer applies to the tmux precondition (FC-4), verified by re-reading the full §1 precondition paragraph in context.
- [ ] AC-4: `team-loop.md`'s permission-mode subsection and confirm-gate wording are verified byte-unchanged (FC-5).
- [ ] AC-5: `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` and `internal/bootstrap/adapters/claude/team-loop.md` (source of truth) and their regenerated/resynced copies (`.fledge/skills/fledge-orchestrate/implementation.md` and `.claude/team-loop.md`) are updated and consistent, via rebuild (`go install ./cmd/fledge`) + `fledge init --refresh`, with `.fledge/scaffold.json` reflecting the new content hashes.
- [ ] AC-6: `cmd/fledge/testdata/*.txtar` fixtures and any other assertions on the changed prose are confirmed passing (updated if the new wording trips an existing assertion) — `go test ./...` is green.

## Out of Scope
- Any change to the permission-mode precondition, its confirm-gate, or its fallback wording (FC-5).
- Any change to Tier A/B (Codex, Pi) behavior — they have no `spawn-worker`/tmux concept; this plumage is Claude-Tier-C-only.
- Any change to how teammates are spawned, addressed, or shut down, the team task list, planning delegation, or recovery-after-resume sections of `team-loop.md` (FC-6).
- Adversarial skua review — that is PLM-018, a separate plumage.

## Open Questions
None — all interrogation branches resolved with the user.
