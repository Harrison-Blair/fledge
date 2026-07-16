---
id: PLM-022
title: Bind spawn name to full role-species prefix
status: fledged
priority: P2
authored: 2026-07-16T00:14:42Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# PLM-022: Bind spawn name to full role-species prefix

## Context
Fledge's worker-naming scheme (`implementation.md` §3.1, agent-neutral) is unambiguous on its own terms: "a worker's name is its role name plus a unique identifier drawn from the 18 extant penguin species — `<role>-<species>`, e.g. `fledge-brooder-adelie`." The scheme's species vocabulary — the list an orchestrator actually consults to pick the next unused identifier — is a bare-word table (`emperor`, `king`, `adelie`, `chinstrap`, …), because the *species* is what's drawn from a pool; the *role* prefix is fixed by which kind of worker is being spawned, not chosen from that pool.

The Claude Code piping file (`team-loop.md`) is where this scheme actually gets bound to a spawn tool call. Its "Spawning and addressing teammates" section says: "Spawn a teammate of a given agent type (e.g. `fledge-brooder`) named per the penguin-species scheme in `implementation.md` §3.1" — but never restates the full pattern or explicitly says the spawn tool's `name` argument must receive the complete `fledge-<role>-<species>` string. An orchestrator that reads "named per the...scheme" and then looks at §3.1's species table (which is bare words) can plausibly bind just the bare species token to that one opaque `name` parameter — species present, role prefix silently dropped. This exactly matches the symptom the user confirmed in live use: spawned teammates showing up as `emperor`, `adelie`, etc., instead of `fledge-forager-emperor`, `fledge-brooder-adelie`. There is no Go code involved in choosing a spawn name anywhere in the codebase — naming is entirely an LLM-orchestrator-follows-prose behavior, so this is a prose-precision fix, not a code fix, and it is scoped to `team-loop.md` alone: `implementation.md` §3.1 (the agent-neutral source of the scheme) already states the full pattern correctly and needs no change, and Codex/Pi have no equivalent named-teammate spawn instruction (Tier A/B, no team loop) so they are unaffected.

## User Stories
- As the orchestrator (team-lead), I want every spawned teammate to be named with its full `fledge-<role>-<species>` prefix, so that task-list entries, `fledge brood --owner` audit labels, and message routing all carry the role information the naming scheme exists to convey.
- As a developer maintaining fledge's Claude adapter, I want the spawn-tool binding stated explicitly with a correct-vs-incorrect example, so the prefix can't be silently dropped by an orchestrator skimming the cross-reference to `implementation.md` §3.1.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: `team-loop.md`'s "Spawning and addressing teammates" section explicitly states that the spawn tool's `name` argument must be the complete `fledge-<role>-<species>` string, not the bare species — with a concrete correct-vs-incorrect example (e.g. `name: "fledge-brooder-adelie"`, not `name: "adelie"`).
2. FC-2: this explicit binding is stated consistently across every named-spawn callout in `team-loop.md` (the brooder/skua spawn in "Spawning and addressing teammates", and the incubator/forager spawns in "Planning delegation").
3. FC-3: after spawning a teammate, the orchestrator confirms it appears in the team roster under its full prefixed name before proceeding, as a self-check backstop against the prefix being dropped despite the explicit instruction.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [x] AC-1: `team-loop.md`'s "Spawning and addressing teammates" section states the full-string requirement for the spawn tool's `name` argument with a correct/incorrect example (satisfies FC-1).
- [x] AC-2: every named-spawn callout in `team-loop.md` (brooder/skua, incubator, forager) reflects the same explicit binding (satisfies FC-2).
- [x] AC-3: `team-loop.md` states the post-spawn roster self-check (satisfies FC-3).
- [x] AC-4: the `cmd/fledge` txtar fixtures asserting on `team-loop.md`'s scaffolded content (at least `init.txtar`, `init_agents.txtar`, `agents.txtar`) are updated to match, and `go test ./cmd/fledge -run TestScripts` passes.
- [x] AC-5: `fledge init --refresh` regenerates this repo's own scaffold from the new prose and `fledge preen` passes clean afterward.

## Out of Scope
- Any Go behavior change — naming is chosen entirely by prose-following, not code; this plumage is prose-only.
- `implementation.md` §3.1 and `worker-protocols.md` — the agent-neutral naming scheme already states the full pattern correctly; no change needed there.
- Codex and Pi adapters — no named-teammate team loop (Tier A/B), unaffected.
- The species-pool allocation/reuse mechanics themselves (first-unused-species selection, numeric-suffix overflow) — unchanged; this plumage only fixes what string is passed as the spawn name once a species is chosen.
- The force-terminate teardown backstop for forager/incubator — tracked separately in PLM-021.

## Open Questions
None — every branch of this plumage's interrogation was resolved with the user before drafting.
