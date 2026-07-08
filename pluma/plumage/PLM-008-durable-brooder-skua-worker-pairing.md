---
id: PLM-008
title: Durable brooder-skua worker pairing
status: hatched
priority: P2
authored: 2026-07-08T06:26:19Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# PLM-008: Durable brooder-skua worker pairing

## Context
In the implementation loop, each dispatched feather gets a brooder (implementor) and a skua
(reviewer) that must share one penguin species so they are unambiguously paired
(`fledge-brooder-adelie` ↔ `fledge-skua-adelie`). Today that pairing lives only in the
orchestrator's in-context bookkeeping and the ephemeral spawn prompts: the orchestrator
picks "the first unused species," injects both names by hand across two separate spawns,
and tracks reuse until both workers shut down. Nothing durable pins it, so a miscount or a
loss of orchestrator context (e.g. on `/resume`) can drift the pairing — the brood lock
records only a single owner name, no species and no reviewer. fledge's discipline is that
deterministic facts belong to the CLI, not to agent context (IDs and locks already work
this way); the species pairing should too. When the CLI owns species allocation and records
it in the brood, the orchestrator stops inventing names and simply reads the pair back,
guaranteeing linkage by name and by feather across context loss.

## User Stories
- As an orchestrator dispatching a feather, I want the CLI to hand me the paired brooder and
  skua names, so that I never miscompute a species or mismatch the pair across two spawns.
- As an orchestrator resuming after context loss, I want the brood lock to reproduce the
  exact pairing, so that I can re-address the same workers without guessing.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: Claiming a feather allocates a species for its worker pair and records it durably,
   returning both canonical worker names.
2. FC-2: Species availability is derived from active claims — a species in use cannot be
   allocated again, and releasing a claim frees its species automatically.
3. FC-3: The recorded pairing is recoverable from the claim alone, independent of any
   orchestrator context.
4. FC-4: The implementation workflow allocates via the CLI, reads the returned names, and
   spawns the pair as one linked unit — carrying no species bookkeeping of its own.

## Acceptance Criteria
- [ ] AC-1: `fledge brood FTHR-### [--branch] [--species <sp>] [--json]` allocates the first free penguin species (numeric-suffixing past the base pool), creates the lock, sets status hatching, and returns feather, species, brooder name, skua name, and branch; `--species` pins a specific free species and errors if it is already held.
- [ ] AC-2: Species availability is derived from active broods — a species held by any active brood is unavailable, and `fledge abandon` frees it automatically with no separate release step.
- [ ] AC-3: The brood record persists the allocated species; `fledge broods` (text and `--json`) shows each brood's species and both derived worker names.
- [ ] AC-4: The pairing survives loss of orchestrator context — re-reading the brood reproduces the exact species and both names.
- [ ] AC-5: `--owner` is removed from `brood`; existing consumers (`colony`, `broods`, PID liveness) operate on the CLI-derived brooder name.
- [ ] AC-6: `implementation.md` (core) is rewired to allocate via `brood`, read the returned names, and spawn brooder and skua in a single message, with in-context species bookkeeping removed; the scaffold is regenerated and the core/`init`/`lock` txtar fixtures updated.
- [ ] AC-7: Automated tests (allocation, pool exhaustion/suffix, `--species` pin and collision error, abandon-frees-species, `broods` output shape, record persistence) cover AC-1..AC-5 and the full suite passes.

## Out of Scope
- CLI species allocation for the forager or any solo spawn (logged as a follow-up).
- Tracking the skua's PID/liveness (only the brooder/holder PID is tracked, as today).
- Changing the 1:1 brooder↔skua topology.
- Reintroducing a spawn-pool / spawn-pair primitive (the guarantee comes from the CLI-owned
  record, not a new primitive).
- `planning.md` forager-spawn prose (rides with the forager follow-up).

## Open Questions
None — all decisions resolved during the 2026-07-08 interrogation.
