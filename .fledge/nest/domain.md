---
generated: 2026-07-17T02:54:09Z
commit: e7a6d4969f861ed3f03af7833b750a7cd703a7a8
agent: fledge-forager
fledge_version: 0.5.8
---

# Domain

Glossary of fledge's bird-themed domain vocabulary, business concepts, and workflow roles. `README.md` is the canonical public decoder; this document is the exhaustive, source-grounded version.

## Spec artifacts

- **Plumage** (`PLM-###`) — a requirement/feature spec; the top-level unit of feature intent; no implementation detail. Lifecycle: `egg` (authoring) → `hatched` (ready for feather decomposition) → `fledged` (all feathers done, closeout verified). Lives at `.fledge/pluma/plumage/`.
- **Feather** (`FTHR-###`) — one implementable task belonging to exactly one plumage; may `depends_on` other feathers. Lifecycle: `egg` → `pipping` (dependencies met, unclaimed) → `hatching` (claimed, in implementation) → `fledged` (merged + verified). Lives at `.fledge/pluma/feathers/`.
- **Functional Criteria (FC-N)** — numbered, testable behavior statements in a plumage body.
- **Acceptance Criteria (AC-N)** — checkbox list defining verifiable done conditions, on both plumages and feathers; only ever toggled via `fledge criteria check|uncheck`.
- **Frontmatter** — the YAML metadata block bounded by `---` fences at the top of every spec file; CLI-managed, never hand-edited.
- **Oversight** — optional feather field: `"merge"` (hold merge pending user sign-off) or `"during"` (confirm readiness before spawn).

## fledge CLI operations

- **Preen** — `fledge preen`; validates spec frontmatter, references, acceptance-criteria structure, and scaffold drift.
- **Molt** — (a) the evidence directory/file (`.fledge/molt/FTHR-###.md`) capturing per-AC test proof during implementation; (b) generically, an acceptance-criteria/field update.
- **Vee** — `fledge vee`; computes the feather dependency graph (cycles, topological waves).
- **Brood** / **Broods** — a claim (lock) a worker holds on a feather while implementing it (`fledge brood` acquires, `fledge abandon` releases, `fledge broods` lists); stored as `.fledge/broods/<FTHR-ID>.brood`.
- **Colony** — `fledge colony`; a full status report (counts by lifecycle stage, orphans, blocked feathers, active locks, parse errors).
- **Scaffold** — the set of files `fledge init` writes into a target repo (`.fledge/skills/`, harness adapter files); tracked in `.fledge/scaffold.json`.
- **Nest** — `.fledge/nest/`; distilled repository knowledge. Raw scout reports live at `.fledge/nest/raw/<module>.md`; synthesized concern docs (this document set) live at `.fledge/nest/*.md`.
- **Ledger** — `.fledge/ledger/`; deterministic agent-handoff records (status/verdict/escalation), addressed by `(subject, kind)`.
- **Roster** — `.fledge/roster/`; the worker-name allocation pool (18 canonical species tokens).

## Worker roles (spawned, context-free per spawn prompt)

- **Incubator** — delegated planner; owns the planning phase end-to-end (freshness gate, foraging, plumage/feather interrogation, authoring); relays gates/questions/decisions verbatim to the user.
- **Forager** — context-regeneration orchestrator; spawns scouts per module, synthesizes the 8 concern docs + index from their raw reports (this document's own role).
- **Scout** (context-scout) — reads only its assigned module's files, writes one raw report to `.fledge/nest/raw/<module>.md`; unnamed (no species), self-terminates on a one-line confirmation.
- **Brooder** — feather implementer; test-first, evidence file per AC, works in a dedicated git worktree, hands off to its paired skua.
- **Skua** — feather reviewer paired with one brooder; re-runs tests, audits evidence as guilty-until-proven, red-teams for weak tests, checks AC boxes once verified; caps at 3 rejection cycles before escalating.
- **Orchestrator** — the implementation-phase dispatcher (on Claude Code, `team-lead`); sole writer of the team task list, sole holder of the spawn/kill primitive, manages gates and roster.
- **Commissioner** — whichever party (orchestrator or incubator) spawned and waits on a forager, per the forager wait contract in `foraging.md`.

## Workflow phases

- **Planning** — feature request → interrogation → hatched plumages + authored feathers.
- **Foraging** — context regeneration; produces the `.fledge/nest/` document set (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain + index).
- **Implementation** — feathers → merged + verified; solo (Tier A/B, orchestrator implements directly) or team loop (Tier C, brooder/skua pairs).

## Primitives, tiers, adapters, harnesses

- **Primitive** — one of 6 agent-neutral orchestration capabilities: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`.
- **Harness** — the agent execution environment (Claude Code, Codex CLI, pi).
- **Adapter** — a harness's thin, format-only mapping of the 6 primitives to concrete mechanisms, driven by that harness's `manifest.yaml`.
- **Tier** — capability level derived (never declared) from primitive coverage: **A** (solo — `confirm-gate`+`read-only-shell`+`write-file`+`run-fledge`; Codex, pi), **B** (A + `spawn-worker` — forager/scout fan-out), **C** (B + `message-peer` — full team loop; Claude Code only).
- **Species** — a unique worker-name token (e.g. `adelie`, `emperor`) drawn from a fixed pool of 18; brooder+skua pairs share one species.

## Documentation & handoff artifacts

- **Concern doc** — one of the 8 synthesized documents this forager writes (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain).
- **Scout report** — a raw, per-module report a scout writes to `.fledge/nest/raw/<module>.md`, with 9 fixed sections.
- **Digest** — `.fledge/scratch/digest-{planning,implementation,foraging}.md`; inter-phase handoff notes so the next phase doesn't assume conversation history.
- **Scratchpad batching** — the incubator mechanism for batching independent interrogation questions into one file/gate review.

## Open Questions

None observed — domain vocabulary was consistent across all 10 scout reports.
