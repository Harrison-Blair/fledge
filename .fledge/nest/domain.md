---
generated: 2026-07-15T18:14:39Z
commit: 5728c29953a7c218c923ce20333dbffebb00623f
agent: fledge-forager
fledge_version: 0.5.4
---

# Domain

Glossary of fledge's bird-themed vocabulary and the underlying spec-driven-development concepts it names. Alphabetical within groups; group by lifecycle relevance.

## Spec artifacts

- **Plumage** (`PLM-###`) — feature-level requirement spec: context, user stories, functional criteria (FC-1, ...), acceptance criteria (unchecked AC-N boxes). Lives at `.fledge/pluma/plumage/`. Lifecycle: `egg` (draft) → `hatched` (user sign-off) → `fledged` (all feathers done, all AC boxes checked).
- **Feather** (`FTHR-###`) — implementable task under a plumage (or standalone): description, affected modules, approach, tests (test-first), acceptance criteria (AC-1 is always the failing→passing test-first cycle). Lives at `.fledge/pluma/feathers/`. Lifecycle: `egg` → `pipping` (dependencies met, ready to start) → `hatching` (claimed/in progress) → `fledged` (merged, criteria verified).
- **Functional criteria (FC-N)** — plumage-level numbered behavior statements.
- **Acceptance criteria (AC-N)** — checkbox list of verifiable conditions; authored unchecked, checked only via `fledge criteria check` (audit-trail, single-byte edit — never hand-edited).
- **Tracer bullet** — feather decomposition strategy: earliest feather(s) build a thin, working end-to-end slice through all layers; later feathers widen it.
- **Molt** — evidence file (`.fledge/molt/FTHR-###.md`) with one `## AC-N` heading per criterion, verbatim command output proving it was satisfied. AC-1's evidence is always the pre-implementation failing-test capture.

## Repository infrastructure

- **Nest** — repository knowledge distilled into `.fledge/nest/`: eight concern docs (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain) plus `index.md`, built from raw scout reports in `.fledge/nest/raw/`.
- **Scout report** — one raw per-module analysis file (`.fledge/nest/raw/<module>.md`), authored by a `fledge-context-scout`; the forager's synthesis input.
- **Concern doc** — one of the eight documents above; organized by topic (not module, except modules.md), every claim file-referenced.
- **Brood** (noun: lock file, verb: to claim) — advisory claim on a feather during active work (`.fledge/broods/<FTHR-ID>.brood`: task, owner, PID, created, branch). Acquired atomically via hard link; PID-liveness checked to detect stale claims.
- **Colony** — full inventory/status report across all specs (`fledge colony` command).
- **Preen** — validation pass over the whole spec set: schema, dangling refs, criteria completeness, and (separately) scaffold drift (`fledge preen`).
- **Scaffold** — the set of files `fledge init` writes into a repo (`.fledge/skills/`, per-harness adapter files like `.claude/`, `.fledge/scaffold.json` stamp).
- **Stamp** (`.fledge/scaffold.json`) — manifest of every scaffolded file's content hash / symlink target / append lines, plus fledge version and agents list; what drift-checking compares against.
- **Dogfooding** — this repo is itself fledge-managed; it must stay in sync with its own tool via rebuild + `fledge init --refresh`.

## Orchestration primitives & tiers

- **Primitive** — one of 6 abstract orchestration capabilities: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`. Adapters map each to a harness-specific mechanism.
- **Tier** — capability level derived (never declared) from primitive coverage: **A** (solo — first 4 primitives; both planning and implementation run in-session, no delegation), **B** (A + `spawn-worker` — fan-out foraging possible), **C** (B + `message-peer` — full team loop with named, communicating workers).
- **Adapter** — thin, harness-specific, manifest-driven mapping realizing the primitive contract on a given tool (Claude Code, Codex, Pi). Adding one is a `manifest.yaml` change, no Go code.
- **Manifest** — an adapter's declarative spec: detector (marker file), `tier_primitives` map, file list with write policies, optional piping file.
- **Core skill** — the single agent-neutral source of orchestration workflow prose (`fledge-orchestrate`, `fledge-interrogate`), embedded once and referenced (never copied/forked) by every adapter.
- **Piping file** — harness-specific runtime procedural doc a manifest points to for behavior too concrete for agent-neutral prose (Claude's `team-loop.md`: tmux display, orchestrator naming, spawn/shutdown mechanics).

## Spawned workers (roles)

- **Orchestrator** — the top-level agent driving a phase; harness-assigned name (`team-lead` on Claude Code) or the audit tag `fledge-orchestrator` in prose.
- **Incubator** — delegated planner (Tier C only): owns planning phase steps 1–4 end-to-end, relays every user decision to the orchestrator, spawns a forager, runs CLI spec mutations. Pure relay — holds no independent implementation state.
- **Forager** — spawned worker (planning) that orchestrates scouts and synthesizes the eight concern docs + index into `.fledge/nest/`. Writes are confined to `.fledge/nest/`; never touches source.
- **Scout** — spawned subagent (planning) that reads an assigned module's file list and writes exactly one raw report; read-only, unnamed, self-terminates.
- **Brooder** — spawned worker (implementation, Tier C) that implements one feather test-first in a dedicated git worktree, hands off to its paired skua, never merges or mutates the spec directly.
- **Skua** — spawned worker (implementation, Tier C), paired 1:1 with a brooder, reviews the feather across cycles. Its `### Reviewing a feather` checklist: tests pass now, tests failed first (AC-1 evidence), diff vs. spec, scope/simplicity, criteria audit. Its `### Verdict`: findings listed to the brooder; a third consecutive rejection escalates; on pass, checks AC boxes via `fledge criteria check` in the worktree and messages the pass to the **orchestrator** (not just the brooder). Stays alive post-pass until merge is verified, then awaits a graceful shutdown request (or force-termination if slow). Never writes code, never merges.
- **Species** — worker-instance identity: 18 penguin species, shared by a brooder+skua pair, reused only once both are *observably* gone (roster absence + pane close on Claude, not just an acknowledged shutdown message).
- **Teammate** vs **subagent** — teammates (incubator, forager, brooder, skua) are long-lived, named, SendMessage-addressable, require explicit `TaskStop` to terminate; subagents (context-scout) are short-lived, unnamed, self-terminating.

## Process states & gates

- **Confirm-gate** — the primitive realizing "show the user material verbatim and get an explicit decision" (full path for large bodies, diff on revisions; Accept/Make-changes or a decision menu).
- **Create-then-gate** — pattern: create the spec file with `fledge new` (real ID, `egg` status) first, write its body, *then* gate on the on-disk draft + summary + diff — never gate on an in-memory draft.
- **GATE / QUESTION / SPAWN-REQUEST / PHASE-CLOSE** — the incubator's relay envelope types for communicating with the orchestrator during delegated planning.
- **Escalation** — triage split between facts (resolved by reading the repo) and decisions (must go to the user); a skua's third rejection is a mandatory escalation trigger, not another silent revise cycle.

## Open Questions

None outstanding beyond what other concern docs carry (primarily around `docs/generalization-plan.md`'s superseded `spawn-pool` vocabulary — not part of the current glossary).
