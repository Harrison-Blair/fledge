---
id: PLM-038
title: Deterministic ledger-driven coordination and pure-relay orchestrator
status: hatched
priority: P1
authored: 2026-07-17T21:07:03Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.10
---

# PLM-038: Deterministic ledger-driven coordination and pure-relay orchestrator

> **SUPERSEDED (2026-07-17) — parked, not to be implemented as written.** The
> multi-harness migration program adopts a subprocess / provider-adapter
> execution model in which fledge owns each agent process's lifecycle. Owning the
> subprocess dissolves the "who wakes the receiver" problem this plumage was
> engineering around: state transitions become harness-controlled facts of the
> **Run State Machine**, not ledger records paired with nudges over an interactive
> message channel. This plumage's durable ideas are absorbed by the program:
> harness-controlled transitions and "no model advances its own state" → the
> **Run State Machine** plumage (P3); deterministic, non-cognitive orchestration
> and forager/worker lifecycle ownership → the **Process Runner** (P1) and
> **Plan→Implement workflow** (P4). Left hatched-and-parked for provenance; the
> body below reflects the pre-reframe design.

## Context
Fledge's multi-agent workflow already prefers ledger records over message state
for the load-bearing handoffs (verdict, escalation, liveness). But three gaps
remain, and the user wants them closed decisively.

1. **State signaling is still mixed.** The worker protocols pair every ledger
   record with a "stateless nudge" over the message channel ("I wrote a record,
   go check"), and lifecycle/idle notifications still tempt agents to infer
   progress from them. The user wants completion and task status to be a
   **deterministic CLI/ledger fact, full stop**: a party learns another party's
   state only by `fledge await` on a ledger record; the message channel is for
   content only; idle notifications are ignored outright.

2. **The orchestrator babysits foraging.** During planning the incubator acts as
   the forager's commissioner — running the await/pulse wait, verifying the nest,
   tearing it down. The user wants that monitoring off the incubator and onto the
   orchestrator (req 6), leaving the incubator only the planning decisions.

3. **The orchestrator does more than relay.** The user wants the orchestrator
   (the main session) to be a **pure relay**: it should do no planning or
   implementation *thinking* itself — only relay user feedback and run
   deterministic, non-cognitive coordination (req 3).

This plumage makes the coordination substrate deterministic and reduces the
orchestrator to a pure relay plus deterministic mechanic. It is the **discipline**
layer; it is deliberately **transport-agnostic about the content channel** —
today that channel is the harness's native messaging (SendMessage on Claude
Code), and PLM-E later migrates it to a fledge-CLI-backed, provider-agnostic
channel without reworking this plumage. The changes are confined to the
agent-neutral workflow prose
(`internal/bootstrap/core/skills/fledge-orchestrate/`), the Claude adapter piping
(`internal/bootstrap/adapters/claude/team-loop.md`), and a minimal new awaitable
ledger kind in the CLI (`internal/ledger`, `internal/cli`).

## User Stories
- As a fledge user, I want every "is it done / ready / resolved" question answered
  by a deterministic ledger fact rather than an LLM-authored message or an idle
  notification, so that coordination is reliable and never guesses.
- As a fledge user, I want the orchestrator to stop babysitting context
  regeneration, so that the incubator focuses on planning and the orchestrator owns
  the forager's mechanical lifecycle.
- As a fledge user, I want the main session to be a pure relay — surfacing every
  decision to me and never doing the planning/implementation thinking itself — so
  that the substantive work lives in the specialized agents and I stay in control of
  every real decision.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …

1. FC-1: Every inter-agent **state** — done, ready, resolved, verdict, escalation,
   blocked, liveness — is a fledge ledger record learned **exclusively** via
   `fledge await`. No agent infers another agent's state from message arrival or
   from a lifecycle/idle notification.
2. FC-2: Lifecycle/idle notifications are **ignored entirely** across the workflow:
   never forwarded between agents, never treated as a done/stall/ready signal, never
   awaited upon. (This makes team-loop.md's current lean absolute.)
3. FC-3: The "record a ledger fact, then send a stateless nudge" pattern is
   **removed everywhere** it appears (verdict, escalation, forager-done, and any
   other handoff). `fledge await` alone is the mechanism; the content channel never
   carries a contentless state wake-up.
4. FC-4: A minimal awaitable **`signal`** ledger kind is introduced — a named state
   record any agent writes and any agent detects with `fledge await` — covering the
   request/response handoffs the existing `status`/`verdict`/`escalation` kinds do
   not: the PLM-B question round-trip and gate pending/resolved. Existing kinds are
   unchanged. (Exact CLI verb/name is settled at feather-authoring time.)
5. FC-5: For user-facing gates, the human still **sees and answers every gate
   in-terminal** (that interaction is content and is unchanged). The inter-agent
   gate **state** (pending → resolved) is a `signal` ledger fact the waiting party
   `fledge await`s; the decision content is persisted to the shared artifact and
   read from disk, never inferred from a message arriving.
6. FC-6: The **orchestrator owns the forager's entire lifecycle** — it spawns the
   forager, runs the `fledge await`/`fledge pulse` commissioner wait, verifies
   `fledge nest status`, and tears it down. The **incubator** runs only the
   freshness gate and, if regeneration is chosen, requests the orchestrator to
   commission a forager, then `fledge await`s a "nest ready" `signal` and proceeds;
   the incubator never runs the commissioner wait.
7. FC-7: The orchestrator performs **no planning/implementation cognition** — it
   never drafts specs, writes or reviews code, judges spec content, or decides
   anything substantive. It relays every decision and substantive question to the
   user (or the owning worker). It retains only **deterministic, non-cognitive
   mechanics**: spawning workers, claiming feathers, merging a branch once a
   deterministic skua `verdict pass` is on the ledger, running `fledge await`, and
   routing escalations. It may resolve only a **pure, unambiguous factual repo
   lookup** itself (retrieval, not cognition); every genuine ambiguity or decision
   is relayed to the user.
8. FC-8: The discipline prose refers to **"the content channel"** abstractly (today
   the harness-native message primitive) rather than hard-coding SendMessage, so
   PLM-E can swap the transport to a fledge-CLI-backed channel without rewriting
   this plumage.

## Acceptance Criteria
- [ ] AC-1: The worker protocols and adapter piping contain no instruction to send a stateless nudge / prose wake-up for state; every state handoff specifies `fledge await` on a ledger record. A bootstrap invariant test asserts the nudge pattern is absent and the await pattern present.
- [ ] AC-2: The prose states that lifecycle/idle notifications are ignored entirely (never forwarded, never a signal, never awaited); a bootstrap invariant test asserts it.
- [ ] AC-3: A `signal` ledger kind exists in the CLI — writable and detectable via `fledge await --kind signal` — with unit tests and a txtar acceptance test; the CLI verb is documented and `--json` supported.
- [ ] AC-4: The planning/foraging prose places the forager's full lifecycle on the orchestrator, and specifies the incubator requests regeneration and `fledge await`s a "nest ready" `signal` rather than running the commissioner wait; an invariant test asserts the incubator no longer commissions/pulses the forager.
- [ ] AC-5: The implementation/team-loop prose encodes the pure-relay cognition boundary: no drafting/coding/reviewing/judging by the orchestrator; merge gated on a deterministic ledger verdict; all decisions relayed; only pure-fact lookups resolved locally. An invariant test asserts the boundary language is present and that cognition verbs are not assigned to the orchestrator.
- [ ] AC-6: The gate-handling prose states the human answers gates in-terminal (content) while inter-agent gate pending/resolved is a `signal` ledger fact and the decision content is read from the shared artifact.
- [ ] AC-7: The discipline prose refers to "the content channel" transport-agnostically — it does not hard-code SendMessage for the coordination discipline — so PLM-E can migrate the transport. An invariant test asserts no SendMessage-specific dependency remains in the discipline sections.
- [ ] AC-8: `fledge preen` passes, and `go test ./...` (new ledger/CLI tests plus updated bootstrap invariant and txtar fixtures) is green after the changes.

## Out of Scope
- The CLI-backed, provider-agnostic content-messaging transport and the
  primitive/adapter/tier re-architecture — owned by **PLM-E**; this plumage stays
  transport-agnostic and does not touch `message-peer`'s realization.
- The editor-free planning interrogation UX — owned by **PLM-B**, which consumes the
  `signal` kind this plumage introduces.
- Concurrent planning/implementation sessions and multiple teams — owned by **PLM-D**,
  which relies on this deterministic coordination.
- The adversarial shrike reviewer — **PLM-A**.
- Any redesign of the existing `status`/`verdict`/`escalation` kinds beyond removing
  their paired nudges; they keep their current semantics.

## Open Questions
- Exact `signal` CLI verb/name, and whether it later unifies with PLM-E's message
  channel into one fledge comms subsystem — settled during PLM-E interrogation.
- Precise fact-vs-decision routing wording for implementation.md §4 (escalation
  triage) under the pure-relay boundary — refined at feather-authoring time.
