---
id: PLM-037
title: Editor-free planning interrogation
status: hatched
priority: P1
authored: 2026-07-17T20:54:26Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.10
---

# PLM-037: Editor-free planning interrogation

> **RESHAPED (2026-07-17) — parked, folded into the migration program.** Under the
> adopted subprocess / provider-adapter model, planning runs as a non-interactive
> provider task (`codex exec`) that emits a typed `plan.json` artifact, not a
> turn-by-turn interactive interrogation over a scratchpad file. The durable
> requirement — the human answers/approves in-terminal on every surface (web,
> mobile, plain terminal) with no file editing — is absorbed by the **Human
> Approval Gates** portion of the **Permissions + Approval + Policy** plumage (P6).
> Left hatched-and-parked for provenance; the body below reflects the pre-reframe
> interactive-teammate design.

## Context
When the planning phase is delegated, the incubator interrogates the user
through the orchestrator. Independent, low-stakes questions are batched today
via a scratchpad file (`incubator.md` "Scratchpad batching"): the incubator
writes the batch to `.fledge/scratch/PLM-…-questions.md`, and the user is asked
to **open that file and type answers inline** before accepting the gate.

That assumption breaks on the Claude Code surfaces where there is no file
editor — web and mobile — and is friction even in a plain terminal. The user
should never have to open or edit a file to answer planning questions.

This plumage removes that assumption: the orchestrator presents the batched
questions **in-terminal** (via the confirm-gate / AskUserQuestion mechanism),
collects the answers, and writes them into the scratchpad **on the user's
behalf**. The scratchpad file survives as an internal incubator↔orchestrator
artifact and paper trail — the user simply never touches it.

Two constraints shape the mechanism:

- **The user's interaction is entirely in-terminal, on every surface.** No step
  may require opening or editing a file. This is the core requirement.
- **State signaling is CLI-driven, not message-driven.** The two inter-agent
  handoffs of the round-trip — incubator → orchestrator ("a question batch is
  ready") and orchestrator → incubator ("answers are written, re-read them") —
  are deterministic fledge CLI/ledger transitions the other party `fledge
  await`s, **not** SendMessage nudges. SendMessage carries only content; in the
  batch flow the content lives in the scratchpad file, so the round-trip uses no
  SendMessage at all. The concrete CLI state-signaling primitive is defined by
  PLM-C (deterministic ledger-driven coordination); **this plumage depends on it
  and is its first consumer**, and must not invent a competing primitive.

The change is confined to the agent-neutral workflow prose
(`internal/bootstrap/core/skills/fledge-orchestrate/incubator.md`) and the Claude
adapter's planning-delegation piping
(`internal/bootstrap/adapters/claude/team-loop.md`).

## User Stories
- As a fledge user on any Claude Code surface — including web and mobile where no
  file editor exists — I want to answer batched planning questions in-terminal, so
  that I never have to open or edit a file to make progress.
- As a fledge user, I want the orchestrator to record my answers into the planning
  scratchpad for me, so that the paper trail is preserved without any manual file
  editing on my part.
- As a fledge user, I want the planning agents to coordinate the question round-trip
  through deterministic CLI state, so that progress never depends on an LLM-prose
  nudge being sent, received, or interpreted.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …

1. FC-1: No step of the planning interrogation requires the user to open or edit a
   file. All question delivery and answer collection happens through in-terminal
   confirm-gate / AskUserQuestion prompts, functioning on every Claude Code surface
   (web, mobile, plain terminal).
2. FC-2: The incubator writes its question batch to the internal scratchpad file
   (the questions section). The orchestrator reads it, presents the questions
   in-terminal — chunking a batch larger than the confirm-gate tool's per-prompt
   question limit into multiple prompts — collects the answers, and writes them
   into the **same file** (a disjoint answers section) on the user's behalf.
3. FC-3: The scratchpad file is retained as an internal incubator↔orchestrator
   artifact and paper trail. It is one file with disjoint sections: the incubator
   owns the questions section, the orchestrator owns the answers section. The user
   never opens it.
4. FC-4: The two round-trip handoffs — "question batch ready" (incubator →
   orchestrator) and "answers written, re-read" (orchestrator → incubator) — are
   signaled as deterministic fledge CLI/ledger state transitions that the waiting
   party detects with `fledge await`, never via a SendMessage nudge. The incubator
   re-reads the scratchpad from disk to pick up answers; it never treats message
   arrival as the signal.
5. FC-5: SendMessage between the incubator and orchestrator is used exclusively to
   carry content (question or answer substance, gate material). Because the batch
   round-trip's content lives in the scratchpad file and its signaling is CLI-driven,
   the batch flow uses no SendMessage for state.
6. FC-6: The workflow prose is rewritten to describe this editor-free, CLI-signaled
   flow: `incubator.md`'s "Scratchpad batching" section removes the "user opens the
   file and answers inline, then Accept" instruction, and the Claude adapter's
   planning-delegation section documents in-terminal presentation and
   orchestrator-written answers.
7. FC-7: The prose states that the whole planning interrogation (both individual
   load-bearing questions and batched questions) is editor-free; individual
   questions are already delivered in-terminal and remain so.

## Acceptance Criteria
- [ ] AC-1: `incubator.md`'s scratchpad-batching section no longer instructs the user to open or edit the scratchpad file; it describes in-terminal question delivery and orchestrator-written answers. A bootstrap invariant test asserts the old file-opening instruction ("answer inline, then Accept") is gone and the new flow is present.
- [ ] AC-2: The prose specifies the orchestrator presents batched questions in-terminal, chunking a batch beyond the confirm-gate tool's per-prompt question limit into multiple prompts, and writes the collected answers into the scratchpad's answers section on the user's behalf.
- [ ] AC-3: The prose specifies the two round-trip handoffs are deterministic fledge CLI/ledger transitions awaited with `fledge await`, with no SendMessage nudge for state, and explicitly notes the dependency on PLM-C's CLI-driven state-signaling primitive (no competing primitive introduced here).
- [ ] AC-4: The Claude adapter's `team-loop.md` planning-delegation section reflects the editor-free, in-terminal flow (present the questions via AskUserQuestion; write answers into the scratchpad for the user) and drops any assumption the user opens a file.
- [ ] AC-5: The scratchpad layout is documented as one file with disjoint questions/answers sections and a stated ownership boundary (incubator writes questions, orchestrator writes answers as a relay/transcription action consistent with its pure-relay role).
- [ ] AC-6: `fledge preen` passes, and `go test ./...` (including the updated bootstrap invariant tests) is green after the changes.

## Out of Scope
- The concrete CLI/ledger state-signaling primitive itself — defined by PLM-C;
  this plumage consumes it and depends on it.
- The broader orchestrator pure-relay role and the general elimination of state
  nudges across every worker handoff — owned by PLM-C; PLM-B only applies the
  resulting mechanism to the planning question round-trip.
- Any change to non-delegated (inline, Tier A/B) planning: with no incubator↔
  orchestrator split there is no scratchpad round-trip; the prose notes this rather
  than building an equivalent.
- The adversarial shrike reviewer (PLM-A) and concurrent sessions (PLM-D).

## Open Questions
- Exact ledger kind / CLI verb for the "batch ready" and "answers written"
  transitions is settled in PLM-C; PLM-B adopts whatever PLM-C defines. If PLM-C's
  mechanism is not yet available when PLM-B is implemented, PLM-B's feathers depend
  on the PLM-C feather that introduces it.
