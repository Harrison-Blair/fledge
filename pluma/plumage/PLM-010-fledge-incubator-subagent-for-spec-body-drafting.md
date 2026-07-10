---
id: PLM-010
title: fledge-incubator subagent for spec-body drafting
status: hatched
priority: P2
authored: 2026-07-10T21:12:21Z
agent: fledge-orchestrate/planning
fledge_version: 0.3.0
---

# PLM-010: fledge-incubator subagent for spec-body drafting

## Context
The planning phase is interactive by design: the orchestrator interrogates the user one
question at a time and runs confirmation gates, all in the main session. Two of its steps —
authoring a plumage body (3.4) and a feather body (4.6) — are not interactive: once the
interrogation has resolved every decision, drafting the file body is a bounded writing task
that consumes context (loading the relevant `.fledge/nest/` concern docs and the spec
template, then composing prose). Doing that inline bloats the orchestrator's context with
material it does not need once the draft exists, working against the phase's goal of leaving
the orchestrator lean enough to then drive implementation.

fledge already delegates the other context-heavy, non-interactive slice of planning —
repository foraging — to a worker (the forager), keeping that work out of the orchestrator's
context. The drafting slice should follow the same shape: a `fledge-incubator` worker that
the orchestrator hands the resolved decisions, which loads the template and cited concern
docs itself and returns a finished draft. The orchestrator keeps everything user-facing (the
freshness gate, the full interrogation, and every confirm-gate) and remains the only agent
that mutates specs — it commits via `fledge new` only after the gate passes, so no un-gated
file ever lands on disk.

## User Stories
- As an orchestrator authoring a plumage or feather, I want to hand the resolved decisions to
  an incubator and get back a finished draft, so that loading templates and concern docs to
  compose the body does not accumulate in my context.
- As a fledge user, I want the confirm-gate and spec commit to stay with the orchestrator I am
  already talking to, so that delegation adds no round-trips to my interrogation and no
  un-reviewed file is ever written.
- As an agent running the planning phase in any harness, I want the empty nest produced right
  after `fledge nest scaffold` to be documented as the normal intermediate state, so that I
  don't misread it as a forager failure.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: A `fledge-incubator` subagent drafts a plumage or feather body from resolved
   decisions and returns it as a single final message; it is non-interactive, stateless
   (one draft per spawn), and mutates no spec.
2. FC-2: The incubator reads the spec template and the cited concern docs itself, given only
   the resolved decisions and pointers (prospective ID, template, concern docs to cite, and
   for feathers the plumage link / depends_on / oversight) — the orchestrator need not
   pre-load them.
3. FC-3: Drafting delegation is capability-conditional in the agent-neutral planning phase:
   where `spawn-worker` is provided the orchestrator delegates the draft to an incubator;
   otherwise it drafts inline. The confirm-gate and the `fledge new` commit remain with the
   orchestrator either way.
4. FC-4: The empty nest state immediately after `fledge nest scaffold` (placeholder concern
   docs, stub raw reports, index stamped to HEAD) is documented in core guidance as the
   expected intermediate state, not a failure.

## Acceptance Criteria
- [ ] AC-1: A `fledge-incubator` agent spec is scaffolded by the Claude adapter (listed in its
      manifest, written to `.claude/agents/fledge-incubator.md` by `fledge init`); it runs on
      claude-sonnet-5 and instructs a non-interactive, one-shot draft-and-return of a plumage
      or feather body with no spec mutation.
- [ ] AC-2: The incubator's instructions define its input/output contract: it is given the
      resolved decisions + pointers, reads the template and cited concern docs itself, and
      returns the full drafted body (frontmatter fields + all sections) as its final message.
- [ ] AC-3: `planning.md` (core) delegates plumage-body (3.4) and feather-body (4.6) drafting
      capability-conditionally on `spawn-worker`, with the confirm-gate and `fledge new`
      commit explicitly retained by the orchestrator and the incubator explicitly barred from
      committing.
- [ ] AC-4: Core guidance (foraging.md and/or planning.md) documents the empty-post-scaffold
      nest as the expected intermediate state so agents do not flag it as a failure.
- [ ] AC-5: The scaffold is regenerated (`fledge init --refresh`) and the affected acceptance
      fixtures (`init_agents.txtar`, `agents.txtar`, and `init.txtar` if its file list
      changes) are updated to include the new agent; `go test ./...` passes.
- [ ] AC-6: Automated tests assert the Claude adapter includes `fledge-incubator` and
      scaffolds it, and that the adapter's derived tier and primitive coverage are unchanged
      (the incubator introduces no new primitive).

## Out of Scope
- Delegating the interrogation itself or any confirm-gate to the incubator (planning stays
  interactive in the orchestrator; the incubator is drafting-only).
- Any change to the freshness gate or foraging ownership (the orchestrator keeps both).
- The incubator running `fledge new`/`status`/`set` or otherwise mutating specs.
- An incubator for the Tier-A harnesses (codex, pi); lacking `spawn-worker` they keep drafting
  inline. Only the capability-conditional hook is added to core.
- Any change to the implementation phase.

## Open Questions
None — all decisions resolved during the 2026-07-10 interrogation.
