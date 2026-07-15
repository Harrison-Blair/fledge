---
id: PLM-020
title: Harden forager wait-contract as request-response state machine
status: fledged
priority: P1
authored: 2026-07-15T21:58:11Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# PLM-020: Harden forager wait-contract as request-response state machine

## Context
In a real planning run, a forager was actively synthesizing the eight `.fledge/nest/` concern docs one at a time (architecture → modules → conventions, landing progressively). The orchestrator, mid-synthesis, read the still-empty stub docs on disk, decided the forager had "stalled at the step-4→step-5 synthesis boundary — the failure mode the protocol warns about," sent it a stand-down message, and began reading and writing the nest files itself — nearly clobbering a working agent. This burned tokens and confused the agent team.

Two things enable this failure, both in the orchestrator/incubator-facing prose (`internal/bootstrap/core/skills/fledge-orchestrate/planning.md` §0–2 and `worker-protocols.md` §Incubator — the single source of truth; `.fledge/skills/...` and `.claude/...` in this repo are scaffolded copies regenerated from it):

1. The wait rule is framed as a defensive prohibition ("do not eagerly react on file changes… never nudge, terminate, or take over on file state alone") sitting next to descriptions of forager disk state, rather than as a hard request-response state machine with exactly two inputs.
2. The commissioner (orchestrator or incubator) is exposed to the forager's *internal* pipeline stages and named failure mode (the "step-4→step-5 synthesis boundary" stall, the "half-filled nest" intermediate, a direct cross-reference into `foraging.md` — a document that describes the forager's own internals). Once the commissioner has a mental model of the forager's pipeline stages, it is tempted to police that pipeline by polling disk — exactly what happened. The forager already owns its own self-resume across that boundary (`foraging.md` § Forager, step 4's "on every wake, re-anchor... proceed straight into synthesis").

This plumage hardens the contract so the commissioner has no pipeline-stage model to police: it is a pure message-driven state machine, and forager internals stay inside forager-facing documents only.

## User Stories
- As the orchestrator (or a delegated incubator) waiting on a live forager, I want the wait contract to be a strict two-input state machine, so that I never treat in-progress disk writes as evidence of a stall and never take over or terminate a working forager on file state alone.
- As a maintainer of the fledge orchestration prose, I want the commissioner-facing files to describe *only* the message contract with the forager (not the forager's internal pipeline stages or failure modes), so that the commissioner has no internal model to police and future edits can't reintroduce disk-polling temptation.

## Functional Criteria
1. FC-1: `worker-protocols.md` §Incubator and `planning.md` §2's "If a forager can be obtained" paragraph state, as the sole determinants of any commissioner decision about a live forager, exactly two inputs: (a) the forager's explicit by-name final message = done, and (b) prolonged silence with no final message = *suspected* stall, handled only by the existing by-name query → escalate-to-user procedure (≤3 queries ~2 min apart, then a `confirm-gate` decision to the user).
2. FC-2: Both files explicitly state that the on-disk state of `.fledge/nest/` (including the empty-stub intermediate right after `fledge nest scaffold`, and partially-filled concern docs mid-synthesis) is never an input to any commissioner decision about the forager.
3. FC-3: Neither file names or describes the forager's internal pipeline stages (scan, scout fan-out, synthesis, index-write) or its internal stall failure mode (the step-4→step-5 synthesis boundary, "half-filled nest," or any cross-reference into `foraging.md` for that purpose). Forager-internal detail stays exclusively in `foraging.md` § Forager and the forager's own Claude agent definition, which are unaffected by this plumage.
4. FC-4: The suspected-stall escalation procedure itself (≤3 by-name queries ~2 minutes apart, then `confirm-gate` decision to the user) is unchanged in substance from the current prose — this plumage reframes and relocates the surrounding language, it does not alter the escalation mechanics.
5. FC-5: A committed automated test (`cmd/fledge/testdata/forager_contract.txtar`) asserts, against the generated scaffold output, that the pipeline-stage/stall-failure-mode leakage strings are absent and the two-input/never-an-input framing is present in both files.

## Acceptance Criteria
- [x] AC-1: `worker-protocols.md` §Incubator and `planning.md` §2 are rewritten in place (no new heading) to state the two-input contract (FC-1) and the "on-disk state is never an input" statement (FC-2), verified by reading the source files.
- [x] AC-2: The rewritten files contain no forager-internal pipeline-stage or failure-mode language (FC-3), verified by `forager_contract.txtar`'s forbidden-string assertions passing.
- [x] AC-3: `forager_contract.txtar` exists, was confirmed to fail against the pre-edit prose for the expected reason, and passes against the post-edit prose (FC-5).
- [x] AC-4: This repo's own scaffold is regenerated (`fledge init --refresh`) and any other txtar fixtures pinning the changed paragraphs are updated so `go test ./cmd/fledge -run TestScripts` and `go vet ./...` pass.

## Out of Scope
- The forager's own pipeline description in `foraging.md` § Forager and its Claude agent definition (`internal/bootstrap/adapters/claude/agents/fledge-forager.md`) — these are forager-facing, correctly retain internal detail, and are not edited.
- `team-loop.md` and `implementation.md` — these only describe spawn mechanics, never expose forager internals, and are not edited.
- The suspected-stall escalation procedure's mechanics (query count, interval, escalation gate) — unchanged, per FC-4.

## Open Questions
None — all decisions resolved during interrogation.
