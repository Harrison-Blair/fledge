---
id: PLM-036
title: "Planning draft quality: Opus incubator and adversarial shrike review"
status: hatched
priority: P2
authored: 2026-07-17T18:08:39Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.10
---

# PLM-036: Planning draft quality: Opus incubator and adversarial shrike review

## Context
The planning phase produces the specs (plumages and feathers) that every
downstream implementation run is built from. Two gaps limit how good those
drafts are before they reach the user's review gate.

First, the delegated planner — the `fledge-incubator` — currently runs on the
`fable` model (see `internal/bootstrap/adapters/claude/agents/fledge-incubator.md`).
Interrogation synthesis and spec drafting are reasoning-heavy work where a
stronger model yields materially better drafts. Raising the incubator to Opus
is a direct, low-cost quality lever.

Second, a draft today is reviewed by exactly one party — the user, at the
confirm-gate. There is no independent, adversarial pass to catch the spec
defects that are cheapest to fix before the user spends attention on them:
acceptance criteria that no test could pin down, internal contradictions,
vague or unfalsifiable functional criteria, scope creep, or missing edge
cases. The implementation phase already pairs every implementer (brooder) with
an adversarial reviewer (skua); planning has no equivalent. This plumage adds
one: a **shrike** — an ephemeral, adversarial reviewer that vets each
plumage/feather draft *before* its user gate, so the user reviews a
pre-vetted draft and sees what the adversarial pass surfaced.

The two changes share one concern — the fidelity and rigor of what planning
hands to the user — and both are edits to the agent-neutral workflow prose
(`internal/bootstrap/core/skills/fledge-orchestrate/`) and the Claude adapter
(`internal/bootstrap/adapters/claude/`), the source of truth this repo
dogfoods.

## User Stories
- As a fledge user, I want the delegated planner to draft specs with a stronger
  model, so that the plumages and feathers I review are higher-fidelity and need
  fewer revision cycles at the gate.
- As a fledge user, I want each spec draft adversarially reviewed before it
  reaches my review gate, so that untestable criteria, contradictions, vagueness,
  and scope creep are caught and addressed before I spend attention on the draft.
- As a fledge user, I want the adversarial reviewer's findings and their
  resolutions surfaced at my review gate, so that I can see what was challenged
  and decide with that context — while keeping final say myself.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …

1. FC-1: The scaffolded `fledge-incubator` agent definition declares `model: opus`
   (changed from `fable`); `effort: medium` is unchanged.
2. FC-2: A new adversarial-reviewer role, **shrike**, is defined in the
   agent-neutral core skill as a role protocol (analogous to `skua.md`),
   describing an adversarial pass over a plumage or feather draft: it reads the
   on-disk draft plus the cited `.fledge/nest/` context, and returns findings.
3. FC-3: The shrike vets **both** plumage drafts and feather drafts, engaging as a
   **pre-gate** pass — after the incubator writes a draft but before that draft's
   review gate is relayed to the user.
4. FC-4: The shrike is an ephemeral spawned worker. Because the incubator cannot
   spawn workers itself, the incubator obtains the shrike via a SPAWN-REQUEST to
   the orchestrator (mirroring the forager path); the orchestrator owns the
   shrike's spawn and lifecycle.
5. FC-5: The shrike's findings are **advisory**: they return to the incubator,
   which revises the draft or defends each finding; the draft's user review gate
   then proceeds. The shrike never blocks the gate and never edits the draft
   itself.
6. FC-6: A single shrike serves the whole planning phase — reused across every
   draft it vets — and is torn down at phase close, following the same deterministic
   shutdown discipline the incubator and forager use.
7. FC-7: The draft's user review gate surfaces a concise summary of the
   adversarial pass: what the shrike flagged and how each finding was resolved
   (addressed in the draft, or defended with rationale).
8. FC-8: The Claude adapter scaffolds a `fledge-shrike` agent definition
   (`model: opus`, `effort: medium`) and registers it in the adapter manifest so
   `fledge init` emits it.
9. FC-9: The shrike role is capability-conditional in the same way planning
   delegation is: it engages only where the planning phase is delegated to an
   incubator (a harness providing `spawn-worker` + `message-peer`). Where planning
   runs inline, the workflow prose states the fallback (the inline planner performs
   the adversarial self-check, or the pass is skipped) rather than assuming a
   shrike exists.

## Acceptance Criteria
- [ ] AC-1: The scaffolded `fledge-incubator` agent definition declares `model: opus` with `effort: medium` unchanged, and the scaffold-output fixtures (`agents.txtar` / `init_agents.txtar`) assert the new value.
- [ ] AC-2: A shrike role protocol exists in the agent-neutral core skill describing the adversarial pre-gate review of plumage and feather drafts (scope, read-only inputs, findings output, advisory authority), and a bootstrap invariant test asserts its presence and key contract points.
- [ ] AC-3: The planning/incubator workflow prose specifies the pre-gate shrike pass: each plumage and feather draft is vetted by the shrike before its user review gate, findings return to the incubator (advisory), one shrike is reused across the phase and torn down at close, and the incubator obtains it via SPAWN-REQUEST.
- [ ] AC-4: The Claude adapter scaffolds a `fledge-shrike` agent definition (`model: opus`, `effort: medium`) registered in the manifest; `fledge init`/`--refresh` emits it and the scaffold-output fixtures assert it.
- [ ] AC-5: The workflow prose specifies that the user's draft review gate surfaces a summary of the adversarial pass and how each finding was resolved.
- [ ] AC-6: The workflow prose states the capability-conditional behavior for harnesses where planning is not delegated (inline fallback), consistent with how planning delegation is already gated on `spawn-worker` + `message-peer`.
- [ ] AC-7: `fledge preen` passes, and `go test ./...` (including the updated txtar and bootstrap invariant tests) is green after the changes.

## Out of Scope
- Any change to the implementation-phase skua (the feather/code reviewer) — the
  shrike is a distinct, planning-only role and does not alter skua behavior.
- Blocking authority for the shrike — findings are advisory only; the user's gate
  remains the decision point.
- Model/effort changes to any other agent (forager, scout, brooder, skua) — this
  plumage touches only the incubator (model) and the new shrike.
- Non-delegated (Tier A/B) harness *implementations* of the adversarial pass beyond
  stating the fallback in prose; wiring a full inline shrike-equivalent for solo
  planning is not required here.
- The scratchpad/interrogation-delivery mechanism (PLM-B), the orchestrator
  pure-relay and deterministic-signaling changes (PLM-C), and concurrent sessions
  (PLM-D) — reconciled where they touch shared files, but owned by their own plumages.

## Open Questions
- Whether the shrike re-reviews a draft after the incubator revises per findings, or
  reviews each draft once (baseline: one pass per draft, with the incubator free to
  request a re-review). To be settled at feather-authoring time.
