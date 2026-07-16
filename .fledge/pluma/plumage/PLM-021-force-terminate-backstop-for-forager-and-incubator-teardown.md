---
id: PLM-021
title: Force-terminate backstop for forager and incubator teardown
status: hatched
priority: P2
authored: 2026-07-16T00:09:28Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# PLM-021: Force-terminate backstop for forager and incubator teardown

## Context
Fledge's Claude Code adapter runs named, addressable teammates for four roles: `fledge-brooder`, `fledge-skua`, `fledge-forager`, and `fledge-incubator`. Commit 96a3ac3 ("Fix teammate shutdown: make TaskStop the real teardown mechanism") established that a `SendMessage` shutdown request alone never terminates a named teammate — it can only prompt an acknowledgement and leave the teammate idle, because a named teammate cannot end its own session. That commit added a force-terminate backstop (`TaskStop`, confirmed by the teammate's observed absence from the roster, not its acknowledgement) for the brooder/skua pair only: `worker-protocols.md`'s Brooder and Skua Lifecycle sections, and `team-loop.md`'s "Shutting down teammates" section (explicitly scoped to "the brooder *and* its paired skua").

The forager and incubator are also named, addressable Claude Code teammates spawned during planning (`planning.md` §0/§2, `worker-protocols.md` §Incubator, `foraging.md` §Forager), and every one of their teardown paths — `foraging.md`'s Commissioner "verify and release" step and its Forager Lifecycle section, `planning.md`'s incubator-release step (§0) and forager-release step (§2), and `worker-protocols.md`'s Incubator Lifecycle section — still end at a bare "request shutdown by name," with no force-terminate backstop and no "confirmed = observably gone" definition. This is a live gap, not theoretical: during this very planning run, the commissioned forager (`fledge-forager-king`) acknowledged a graceful shutdown request and then had to be force-terminated manually via `TaskStop` by the orchestrator, because the protocol it was following names no backstop step. Left unfixed, every planning phase and every standalone context regeneration leaves a lingering idle teammate on the roster whenever the cooperative shutdown request alone doesn't stick.

Separately, `team-loop.md`'s shared "confirmed shutdown" definition ("no longer appears in the team roster and its tmux pane has closed") assumes a tmux pane exists; in a degraded/no-tmux session, no teammate ever had a pane, so that clause is inapplicable rather than satisfied, and the prose doesn't say so. Fixing this definition for the forager/incubator paths this plumage adds means also correcting the wording for the existing brooder/skua path that shares the same sentence (no functional change to brooder/skua teardown, just a wording fix so the definition is well-formed with or without tmux).

Finally, a related regression: commit 49e32cb ("re-init") dropped a bullet from `internal/bootstrap/adapters/claude/agents/fledge-forager.md` — "gate your final message on `fledge nest status`" — that told the forager to self-verify completeness before reporting done. The 0.5.5 scaffold refresh (f8bd7f9) resynced the repo's own `.fledge/` copies but never restored this adapter *source* file, so the bullet is still missing (the file jumps from the `nest stamp` bullet straight to the teammate-exit bullet). This plumage restores it alongside the backstop work since it lives in the same file family and is a small, related correctness fix, not a new plumage's worth of scope.

## User Stories
- As the orchestrator (team-lead), I want a reliable way to force-terminate lingering forager/incubator teammates, so that planning-phase workers don't linger idle after their work is done, consuming roster/session resources indefinitely.
- As a developer maintaining fledge's orchestration prose, I want the forager's self-verification bullet restored, so that foragers correctly gate their final message on `fledge nest status` as originally specified (fixing the dropped-in-re-init regression).

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: `worker-protocols.md`'s Incubator Lifecycle section states that the orchestrator will force-terminate the incubator if it does not exit promptly after a shutdown request, mirroring the existing Brooder/Skua Lifecycle language.
2. FC-2: `foraging.md`'s Forager Lifecycle section and Commissioner "verify and release" step state that the commissioner (incubator or orchestrator, whichever commissioned it) will force-terminate the forager if it does not exit promptly after a shutdown request.
3. FC-3: `planning.md`'s forager-release step (§2) and incubator-release step (§0) reference the force-terminate backstop, not just "request shutdown by name."
4. FC-4: `team-loop.md`'s "Shutting down teammates" section explicitly covers all four teammate roles — brooder, skua, forager, incubator — not only "the brooder and its paired skua."
5. FC-5: `team-loop.md`'s "confirmed shutdown" definition explicitly handles the no-tmux/degraded case: roster-absence alone is sufficient confirmation when no pane was ever created; the pane-closed clause applies only when a pane exists. This corrected definition applies uniformly to all four teammate roles' teardown wording.
6. FC-6: `internal/bootstrap/adapters/claude/agents/fledge-forager.md` restores the dropped self-verification bullet — gate the final message on `fledge nest status` — between the `nest stamp` bullet and the teammate-exit bullet.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [ ] AC-1: `worker-protocols.md`'s Incubator Lifecycle section documents the force-terminate backstop (satisfies FC-1).
- [ ] AC-2: `foraging.md`'s Forager Lifecycle/Commissioner sections document the force-terminate backstop (satisfies FC-2).
- [ ] AC-3: `planning.md`'s forager-release (§2) and incubator-release (§0) steps reference the backstop (satisfies FC-3).
- [ ] AC-4: `team-loop.md`'s "Shutting down teammates" section explicitly covers all four teammate roles — brooder, skua, forager, incubator (satisfies FC-4).
- [ ] AC-5: `team-loop.md`'s confirmed-shutdown definition explicitly handles the no-tmux/degraded case, applied uniformly across all four roles' teardown wording (satisfies FC-5).
- [ ] AC-6: `fledge-forager.md`'s dropped self-verification bullet is restored (satisfies FC-6).
- [ ] AC-7: the `cmd/fledge` txtar fixtures asserting on the affected scaffolded files (at least `init.txtar`, `init_agents.txtar`, `agents.txtar`) are updated to match, and `go test ./cmd/fledge -run TestScripts` passes.
- [ ] AC-8: `fledge init --refresh` regenerates this repo's own scaffold from the new prose and `fledge preen` passes clean afterward.

## Out of Scope
- Any Go behavior change — this plumage is prose-only (core skill docs + Claude adapter docs + one agent-definition file).
- Codex and Pi adapters — they provide no `spawn-worker`/`message-peer` team loop (Tier A/B, not Tier C) and have no named teammates to force-terminate.
- Scout lifecycle — scouts are already unnamed and self-terminate on their one-line final message; unaffected by this plumage.
- The tmux-precondition auto-resolve logic itself (`team-loop.md`'s "Teammate display (tmux)" section) — only the shutdown-confirmation definition changes, not how tmux presence is detected or how panes are used when present.
- Naming/prefix-stripping defect (bare species instead of `fledge-<role>-<species>`) — tracked as a separate plumage.

## Open Questions
None — every branch of this plumage's interrogation was resolved with the user before drafting.
