---
generated: 2026-07-10T20:53:53Z
commit: f28efebd76d6aa135adb0956a3337a40a8d98351
agent: fledge-forager
fledge_version: 0.3.0
---

# Domain

Glossary of fledge's bird-themed domain vocabulary, reconciled across all scout reports. See `README.md` for the canonical decode.

## Spec lifecycle

- **Plumage** (`PLM-###`, `pluma/plumage/PLM-###-*.md`) — a feature/requirement spec: user-facing intent, WHAT and WHY. Status lifecycle: `egg → hatched → fledged`. Sections: Context, User Stories, Functional Criteria (FC-N), Acceptance Criteria (AC-N), Out of Scope, Open Questions.
- **Feather** (`FTHR-###`, `pluma/feathers/FTHR-###-*.md`) — an implementable task under a plumage. Status lifecycle: `egg → pipping → hatching → fledged`. Sections: Description, Affected Modules, Approach, Tests, Acceptance Criteria. Has `depends_on` (blocking feather IDs, cycle-checked) and optional `oversight: merge` (human-review gate, typically on prose/doc feathers).
- **Pipping** — a feather status meaning its dependencies are all satisfied and it's ready to be claimed/worked.
- **Hatching** — a feather actively claimed (brooded) and in progress.
- **Fledged** — terminal status for both plumages and feathers; complete and effectively immutable (append-only) once reached.
- **Acceptance criteria (AC-N)** — checkbox list (`- [x] AC-N: ...`); the only way to check them is `fledge criteria check`, never manual editing; all must be checked before a spec can fledge.
- **Functional criteria (FC-N)** — numbered testable statements in a plumage, traced downstream into feather acceptance criteria for coverage (e.g. "AC-2 satisfies PLM-001 FC-1..3 as pinned by txtar").

## Workflow roles and process nouns

- **Nest** — the distilled repository-knowledge directory, `.fledge/nest/`: 8 concern docs (this document is one) + `index.md` + `raw/` scout reports. Regenerated wholesale by a forager.
- **Forager** — a one-shot Tier B/C agent that orchestrates scouts, then synthesizes their raw reports into the nest concern docs and index. (This document was produced by exactly such a forager.)
- **Scout** — an unnamed, cheap agent spawned by a forager; reads only its assigned module's files and writes exactly one `.fledge/nest/raw/<module>.md` report; self-terminates on a one-line confirmation.
- **Brood** — a claim/lock on a feather held by an agent while working it; stored as `.fledge/broods/<FTHR-ID>.brood` (owner, PID, created, branch). `fledge brood`/`abandon`/`broods` manage it.
- **Brooder** — a Tier C ephemeral teammate that implements one feather test-first in a dedicated git worktree; messages its paired skua and the orchestrator; lives until the feather merges.
- **Skua** — a Tier C ephemeral teammate paired 1:1 with a brooder; reviews the brooder's completed feather against spec (re-runs tests, audits test-first evidence); reports approval to the orchestrator; lives until the feather merges.
- **Orchestrator** — the main session driving planning/implementation; on Claude Code specifically addressed as "team-lead," not "fledge-orchestrator."
- **Preen** — the validation command (`fledge preen`); checks spec health (parse/schema/dangling-ref/cycle/brood-consistency/criteria findings) and scaffold drift together.
- **Molt** — evidence storage: `.fledge/molt/FTHR-###.md`, holding AC-by-AC test-run proof (failing pre-implementation, passing post-implementation) that a skua audits.
- **Vee** — the dependency-graph command/visualization (`fledge vee`); text, dot, or JSON; computes waves and detects cycles.
- **Colony** — the full spec-inventory/status-summary report command (`fledge colony`).
- **Wave** — a topological layer in the dependency graph; feathers in the same wave have no inter-dependencies and could in principle proceed in parallel.
- **Ready** — feathers whose `depends_on` are all fledged and which haven't started; the dispatch hint surfaced by `fledge ready`.

## Bootstrap/adapter vocabulary

- **Harness** — an agent-hosting framework: Claude Code, pi, Codex (and, per `docs/generalization-plan.md`, prospectively Cursor/opencode).
- **Primitive** — one of 6 agent-neutral orchestration capabilities (`confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`) a harness must realize via some concrete mechanism.
- **Tier** — a harness's derived capability level: A (4 primitives, solo planning+implementation), B (+`spawn-worker`, adds fan-out foraging), C (+`message-peer`, full brooder/skua team loop). Never hand-declared — always computed from declared primitive coverage via `DeriveTier()`.
- **Adapter** — a harness's thin, format-only mapping: `manifest.yaml` (detector + primitive map + file list) plus any harness-native template files. Zero Go code needed to add/change a harness.
- **Manifest** — the single YAML source of truth for one adapter (detector marker, tier-primitives map, per-file write policy, piping-file reference).
- **Scaffold** — the set of files `fledge init` writes into a target repo (`.fledge/skills/`, harness adapter files, `pluma/` skeleton, `.gitignore` entries, `.fledge/scaffold.json`).
- **Scaffold stamp** (`.fledge/scaffold.json`) — records fledge version, scaffolded agents, and per-file `{policy, sha256|target|lines}` — the basis for drift detection and refresh preserve/prune decisions.
- **Drift** — the classification of a scaffolded file's on-disk state vs. expected: up-to-date, stale, modified, missing, or obsolete.
- **Concern doc** — one of the 8 synthesized `.fledge/nest/` documents (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain) — as distinct from the unsynthesized per-module `raw/` scout reports.
- **Interrogate skill** (fledge-interrogate) — a decision-forcing, one-question-at-a-time interview protocol used to stress-test a plumage before it's committed.

## Open Questions

None survive synthesis — every domain term encountered across the 7 scout reports and `README.md` is accounted for above.
</content>
