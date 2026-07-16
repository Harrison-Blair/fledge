---
generated: 2026-07-15T23:53:12Z
commit: a4d02e8187c64ef9f3f1231052990b282207420b
agent: fledge-forager
fledge_version: 0.5.5
---

# Domain

Glossary of fledge's bird-themed vocabulary and the workflow/role concepts it encodes — reconciled across every scouted module.

## Spec types & lifecycle

- **Plumage (`PLM-###`)** — a feature/requirement spec: WHAT and WHY, never implementation detail. Contains Context, User Stories, Functional Criteria (`FC-N`), Acceptance Criteria, Out of Scope. Lifecycle: **egg** (authored, not yet approved) → **hatched** (user sign-off, ready for feather decomposition) → **fledged** (all linked feathers fledged and every acceptance-criteria box checked).
- **Feather (`FTHR-###`)** — an implementable task under a plumage: Description, Affected Modules, Approach (implementation detail, testable design), Tests (written test-first), Acceptance Criteria. `depends_on` links create ordering. Lifecycle: **egg** (created, awaiting dependencies) → **pipping** (all dependencies fledged, no lock held — ready to dispatch) → **hatching** (claimed under active implementation, brood lock held) → **fledged** (merged, all AC boxes checked).
- **AC / Acceptance Criteria** — numbered checkboxes `- [ ] AC-N: …`, authored unchecked, checked only via `fledge criteria check FTHR-### <n>`, never hand-edited. Feather AC map to implementation and typically reference a parent plumage's FC ("satisfies PLM-### FC-N").
- **FC / Functional Criteria** — numbered behavioral requirements at the plumage level (`FC-1`, `FC-2`, …), referenced by feather acceptance criteria.
- **Oversight** — feather verification mode: `merge` (branch held unmerged until user signs off on diff + verdict) or `during` (user prompted before the brooder is even spawned, to participate in decisions); empty = no extra gate.
- **Tracer bullet** — the first feather(s) in a plumage: a thin, end-to-end working slice through every architectural layer, proving the design before later feathers widen it with more cases/robustness/polish. Root of the `depends_on` graph.

## Storage layout

- **Nest (`.fledge/nest/`)** — the repository-context knowledge store: `index.md` (routing) + eight concern docs (this file is one of them) + `raw/` (scout reports). Distilled by foraging.
- **Concern doc** — one of the eight synthesized documents in `.fledge/nest/` (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain); topic-organized (except `modules.md`, module-organized), every claim references its source file.
- **Scout report** (`.fledge/nest/raw/<module>.md`) — raw per-module findings written by a scout; the unsynthesized input to concern docs.
- **Molt (`.fledge/molt/`)** — evidence directory; holds `FTHR-###.md` per-feather evidence files (one `## AC-N` section per criterion; AC-1 always the pre-implementation failing-test capture).
- **Brood / broods (`.fledge/broods/`)** — active claim/lock on a feather: owner, PID, created timestamp, branch, stored per-feather as `FTHR-###.brood`. `fledge brood` atomically creates the lock and sets status to `hatching`.
- **Scaffold (`.fledge/scaffold.json`)** — the stamp of fledge-owned files: policy + content hash (or symlink target, or append-lines) per file, written by `fledge init --refresh`; `fledge preen` validates its presence/consistency.
- **Burrows (`.fledge/burrows/`)** — an alternate worktree location (parallel to a scratchpad dir), bird-themed.

## CLI verbs (bird-themed, decoded in `README.md`)

- **preen** — validate specs + scaffold consistency (`fledge preen`).
- **molt** — regenerate/refresh fledge-owned scaffold files (`fledge init --refresh`); also the name of the evidence directory (`.fledge/molt/`) — same root metaphor (shedding/renewing), two distinct uses.
- **brood / abandon / broods** — acquire, release, and list feather claim locks.
- **hatch / hatching** — status transition indicating active work: for a feather, actively claimed and implemented; for a plumage, user has approved it moving past `egg`.
- **pipping** — feather status: ready for dispatch (all dependencies fledged, no lock held).
- **fledged** — fully complete: every acceptance-criteria box checked, and (for feathers) merged.
- **colony** — aggregated project status report (`fledge colony`): counts by lifecycle state, per-plumage completion, orphan feathers, unmet dependencies, active broods.
- **vee** — dependency-graph command (`fledge vee`): named for the V-formation birds fly in; renders waves/cycles in text, dot, or JSON.
- **unfledged** — lists incomplete plumages/feathers.

## Worker roles (orchestration/foraging protocol)

- **Forager** — context orchestrator; spawned by the incubator (or the orchestrator, if planning inline) at the start of context regeneration. Scans the repo, plans a scout split, fans out scouts in parallel, synthesizes the eight concern docs + index from their raw reports. One-shot per regeneration; writes are confined to `.fledge/nest/`.
- **Scout (`fledge-context-scout`)** — cheap subagent spawned by the forager with one module name + an exact file list. Reads only those files, writes exactly one raw report (`.fledge/nest/raw/<module>.md`) following a fixed 9-section skeleton. Unnamed (no species); self-terminates on a one-line final message.
- **Incubator** — delegated planning-phase orchestrator, spawned by the main orchestrator at the start of planning. Owns context gathering (spawning a forager), plumage/feather interrogation, spec creation, and every planning-phase CLI mutation; communicates upward via a structured relay envelope (`GATE`, `QUESTION`, `SPAWN-REQUEST`, `PHASE-CLOSE`), which the orchestrator relays to the user verbatim.
- **Brooder** — feather implementer, spawned at dispatch time in a dedicated git worktree. Writes tests first (from the feather's Tests section), captures a failing run as evidence, implements until green, records per-criterion evidence, and hands off to its paired skua. Named `fledge-brooder-<species>`.
- **Skua** — feather reviewer, spawned paired 1:1 with a brooder. Re-runs tests, diffs the change against the spec, red-teams, audits each criterion's evidence (treating ambiguous/summary-only evidence as *not* proof — "guilty until proven"), checks AC boxes (the skua's only permitted spec write), and reports either a pass or a numbered findings list to the orchestrator (never to the brooder directly). Third rejection of the same feather escalates to the orchestrator/user.
- **Orchestrator** — the main session during implementation: dispatches feathers, gates reviews, merges branches, runs the full test suite, triages escalations, owns the team task list. Never assigned a species. Called "team-lead" specifically on Claude Code.
- **Commissioner** — whoever spawns and waits on a forager: the incubator during delegated planning, or the orchestrator for a standalone context regeneration.

## Naming mechanics

- **Species** — a penguin-species identifier (18 available: emperor, king, adelie, chinstrap, gentoo, little, yellow-eyed, african, humboldt, magellanic, galapagos, fiordland, snares, erect-crested, southern-rockhopper, northern-rockhopper, royal, macaroni) assigned at spawn time to make a worker uniquely addressable, e.g. `fledge-brooder-adelie`. A brooder+skua pair shares one species; solo spawns (forager, incubator) take their own. A species is only reusable once both members of a pair are confirmed shut down (not just acknowledged). If more than 18 concurrent pairs are needed, a numeric suffix is appended (`adelie-2`).

## Orchestration architecture

- **Primitive** — one of 6 orchestration capabilities fledge's workflow prose is written against: `confirm-gate` (user decision/review), `read-only-shell` (inspect without mutating), `write-file`, `run-fledge` (execute the CLI deterministically), `spawn-worker` (spawn an ephemeral addressable sub-session), `message-peer` (async by-name messaging).
- **Harness** — an agent platform fledge scaffolds into: Claude Code, Codex, pi (README also names Cursor, opencode as future targets).
- **Adapter** — a thin, harness-specific mapping of the 6 primitives to concrete mechanisms, defined entirely by a `manifest.yaml`; adding a harness means writing a manifest, not Go code.
- **Manifest** — the YAML config (`manifest.yaml`) declaring an adapter's detector, `tier_primitives` map, and scaffolded file list with write policies.
- **Tier (A/B/C)** — capability level derived (never declared) from primitive coverage: Tier A = solo (4 primitives, no `spawn-worker`/`message-peer` — Codex, pi); Tier B = + `spawn-worker` (not yet present in any shipped adapter); Tier C = + `message-peer`, full team-loop (Claude Code).
- **Write policy** — per-file scaffolding rule in a manifest: `generate`/`primitive_map` (text/template render), `overwrite` (always repaired), `append_if_missing` (additive line), `symlink` (repoint into `.fledge/skills/`), or the default (copy once, skip-if-exists, so user edits survive).
- **Core** — the single agent-neutral skill tree (`internal/bootstrap/core/skills/`) with no harness-specific path strings, shipped identically to every adapter; symlinked (Claude) or copied into each harness's native skill location.
- **Drift** — a scaffold file's classified state relative to what fledge expects: `up-to-date`, `stale` (matches old stamp, shipped version moved — refresh-safe), `modified` (user-edited), `missing`, `obsolete` (in the stamp but no longer shipped).

## Dogfooding

- **Dogfood** — this repo (`fledge` itself) uses `fledge` on itself: it has its own `.fledge/pluma/` specs, `.fledge/broods/` claims, `.fledge/nest/` (this document set), and a scaffolded `.claude/` adapter.

## Origin note (from `docs/generalization-plan.md`)

The docs module's `generalization-plan.md` is the design doc that produced this core/adapters split (fledge 0.1.0 → 0.2.0): it defines the 6/7-primitive contract, the manifest format, and named "forager"/"brooder"/"skua" roles before they existed in code — useful background if a term's *rationale* (not just its current behavior) is needed. `docs/google_ai_mode_response.md` and `docs/research_prompt.md` are unrelated exploratory infrastructure notes and use their own separate vocabulary (tiers-as-cost-hierarchy, not tiers-as-primitive-coverage) — don't conflate the two "tier" concepts.

## Open Questions

None observed — role, lifecycle, and primitive/tier terminology were consistent and mutually confirming across every scout report that touched them (`bootstrap-core.md`, `cli.md`, `bootstrap-adapters.md`, `cmd.md`, `internal-misc.md`).
