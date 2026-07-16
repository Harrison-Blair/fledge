---
generated: 2026-07-16T21:27:15Z
commit: a1ed62a38540df7ab1cbdc4c486176a64a762018
agent: fledge-forager
fledge_version: 0.5.8
---

# Domain

Glossary of fledge's bird-themed vocabulary and orchestration concepts. Terms are grouped by theme, not alphabetically, since many are defined relative to each other.

## Spec artifacts and their lifecycle

- **Plumage** (`PLM-###`): a feature/requirement spec — the WHAT and WHY, no implementation detail. Lifecycle: `egg` (authoring) → `hatched` (user sign-off) → `fledged` (all its feathers fledged, boxes checked). Body sections: Context, User Stories, Functional Criteria (FC-N), Acceptance Criteria, Out of Scope, Open Questions.
- **Feather** (`FTHR-###`): one implementable task under a plumage. Lifecycle: `egg` (not started) → `pipping` (all `depends_on` fledged, unclaimed — ready to dispatch) → `hatching` (claimed/in progress) → `fledged` (done: AC boxes checked, merged, suite green). Body sections: Description, Affected Modules, Approach, Tests, Acceptance Criteria (AC-N, AC-1 always test-first evidence).
- **Acceptance Criteria (AC-N)**: checkbox list on a spec; changed *only* via `fledge criteria check|uncheck`, never hand-edited. All must be checked before a feather/plumage can be `fledged`.
- **Functional Criteria (FC-N)**: numbered testable behavior statement in a plumage; feathers' AC entries reference these.
- **Brooding**: claiming a feather for implementation — a lock (`fledge brood`) plus the status transition to `hatching`.
- **Molting**: the evidence file (`.fledge/molt/FTHR-###.md`) recording per-criterion command+output evidence.
- **Fledging**: marking a feather/plumage complete.
- **Depends_on**: array of upstream feather IDs a feather blocks on; must all be `fledged` before the feather becomes `pipping`.
- **Oversight**: user-participation mode on a feather — `"during"` (user confirms readiness before the worker spawns, decisions relayed via orchestrator), `"merge"` (user signs off on diff + verdict before merge), or omitted (fully autonomous).
- **Tracer bullet**: the first feather(s) in a decomposition — a thin, minimal, real, verifiable end-to-end slice through all layers; later feathers widen from there.
- **Colony**: portfolio summary view — counts of plumages/feathers by status, orphans, blocked tasks, held locks (`fledge colony`).

## Repository structures

- **Nest** (`.fledge/nest/`): distilled repository context — the 8 concern docs + `index.md` this forager writes, plus `raw/` scout reports.
- **Scaffold**: the set of fledge-owned files written into a repo by `fledge init` (`.fledge/skills/`, `.claude/` or other harness dirs, `.fledge/scaffold.json`).
- **Stamp** (`.fledge/scaffold.json`): content-hash manifest of every scaffold file fledge owns; enables drift detection and safe pruning on `--refresh`.
- **Preen**: the validation command (`fledge preen`) — checks spec schema, frontmatter, dependency graph, acceptance-criteria state, brood consistency, and scaffold drift.
- **Roster**: persistent name↔assignment mapping for worker species (`fledge roster assign|release`).
- **Species**: one of 18 canonical penguin names (adelie, emperor, gentoo, …) used as the second half of a worker's identity.

## Orchestration roles

- **Orchestrator**: the main-session dispatcher; role name `fledge-orchestrator`, harness-specific address (`team-lead` on Claude Code).
- **Forager**: orchestrates scouts, synthesizes the 8 concern docs + index from their reports. Write-confined to `.fledge/nest/`; never touches source code. (This document was written by one.)
- **Scout** (`fledge-context-scout`): reads only its assigned file list, writes exactly one raw report to `.fledge/nest/raw/<module>.md`. Unnamed, self-terminating.
- **Incubator**: delegated planner — owns the planning phase end-to-end when spawned (interrogation, spec drafting, planning-phase CLI mutations), relays every user decision through the orchestrator.
- **Brooder**: implementer — test-first, works only in its worktree, evidence per criterion, hands off to its paired skua.
- **Skua**: reviewer — audits a brooder's evidence and diff against spec, re-runs tests, red-teams uncovered inputs, checks/unchecks AC boxes on verdict, never merges or modifies code itself.

## Orchestration mechanics

- **Primitive**: one of 6 fixed orchestration capabilities a harness may provide — `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`.
- **Tier**: capability level derived (never declared) from primitive coverage — Tier A (4 primitives, solo workflow only), Tier B (+`spawn-worker`, enables foraging), Tier C (+`message-peer`, full team loop with brooder/skua pairs). Claude Code is Tier C; Codex and pi are Tier A.
- **Adapter**: per-harness thin mapping layer (`.claude/`, `.codex/`, `.pi/`) realizing the 6 primitives via harness-native mechanisms; fully described by a `manifest.yaml`, zero harness-specific Go code.
- **Manifest**: the YAML file (one per adapter) declaring detector, `tier_primitives` map, and scaffolded-file list with write policies.
- **Detector**: the condition (e.g. `exists:.claude/`) `fledge init`/`fledge agents` uses to decide whether an adapter applies to a repo.
- **Harness**: an agent execution platform — Claude Code, pi, Codex CLI (and design docs mention Cursor/opencode as future targets).
- **Foraging**: the context-gathering phase (this pipeline) — scan, scout fan-out, synthesis, verification.
- **Interrogate**: the `fledge-interrogate` skill — a decision-forcing, one-question-at-a-time interview protocol used during plumage/feather authoring.
- **Relay envelope**: the message format an incubator uses toward its orchestrator — `GATE review`, `GATE decision`, `QUESTION`, `SPAWN-REQUEST`, `PHASE-CLOSE`.
- **Digest**: a phase's close-out summary file (`digest-planning.md`, `digest-implementation.md`, `digest-foraging.md` under `.fledge/scratch/`).
- **Green teardown**: the required Tier-C sequence after a feather's skua approval — merge, run full suite, verify criteria landed, `fledge abandon --fledged`, release locks, remove worktree, delete branch.

## Open Questions

- Whether "Agent Skills standard" (cross-harness skill frontmatter format referenced in `docs/generalization-plan.md`) is an external standard fledge conforms to, or fledge-internal terminology — not independently confirmed against code in this pass.
