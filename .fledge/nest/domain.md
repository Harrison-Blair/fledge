---
generated: 2026-07-10T14:50:00Z
commit: 7678344ab9a18730530b9f6edf507ad0c449d352
agent: fledge-forager
fledge_version: 0.2.1
---

# Domain

Glossary of fledge's bird-themed business/domain vocabulary, reconciled across all scouted modules — the terminology every layer (specs, CLI, prose, agent roles) shares.

## Spec model

- **Plumage** (`PLM-###`): a feature-intent spec — WHY/WHAT at feature level. Status lifecycle: `egg → hatched → fledged`. Stored at `pluma/plumage/PLM-###-*.md`.
- **Feather** (`FTHR-###`): an implementable-task spec under a plumage — HOW, with tests/evidence/dependencies. Status lifecycle: `egg → pipping → hatching → fledged`. Stored at `pluma/feathers/FTHR-###-*.md`.
- **Status states** (feather, most granular): `egg` = blocked/unstarted (deps unmet); `pipping` = ready (all deps satisfied, unclaimed); `hatching` = in progress (claimed, brood exists); `fledged` = complete (all AC checked, immutable).
- **Acceptance Criteria (AC-N)**: numbered checkbox list in a spec body; must all be checked (via `fledge criteria check`, never hand-edited) before a feather/plumage can reach `fledged`.
- **Functional Criteria (FC-N)**: numbered behavior statements in a plumage body; referenced by a feather's AC-N to trace requirement → implementation.
- **Orphan feather**: a feather referencing a plumage that doesn't exist; excluded from plumage-progress counts.
- **Dangling ref**: a feather's `depends_on` pointing at a nonexistent task, or a reference to a missing plumage.
- **Priority**: P0–P3 scheme, used for sort order in `ready`/`unfledged` output.
- **Oversight**: a merge/review gate on a feather (e.g. `oversight: merge`) requiring human sign-off before closure.

## Operations (CLI commands as domain verbs)

- **Preen**: validate a spec set (`fledge preen`); reports `Finding`s (errors/warnings).
- **Vee**: dependency-graph analysis/visualization (`fledge vee`); waves, cycle detection, DOT output.
- **Colony**: aggregate progress report (`fledge colony`) — counts, per-plumage completion, orphans, blocked tasks, locks, issues.
- **Brood** (verb, on a feather): claim a feather for work (`fledge brood FTHR-### --owner`), creating a lock file and setting status to `hatching`.
- **Molt**: the per-feather evidence directory/file (`.fledge/molt/FTHR-###.md`) capturing proof for each acceptance criterion (test runs, diffs, commands).

## Infrastructure nouns

- **Nest**: the `.fledge/nest/` directory — this document set (8 concern docs + `index.md` + `raw/` scout reports), synthesized repo knowledge for downstream planning agents.
- **Brood** (noun): a lock file at `.fledge/broods/<FTHR-ID>.brood`, recording `Task, Owner, PID, Created, Branch` for an in-progress feather claim.
- **Skill**: an Agent Skills-format directory (`SKILL.md` + supporting files); fledge ships two core skills (`fledge-orchestrate`, `fledge-interrogate`).
- **Adapter**: a thin, harness-specific binding (`internal/bootstrap/adapters/<harness>/`), driven entirely by its `manifest.yaml`.
- **Manifest**: the YAML schema (`internal/bootstrap.Manifest`) declaring a harness's detector, primitive→mechanism mappings, and file scaffolding policy.
- **Core skill**: the single agent-neutral orchestration workflow source (`internal/bootstrap/core/skills/fledge-orchestrate/`), scaffolded to a repo's `.fledge/skills/`.
- **Primitive**: one of the 6 agent-neutral capability contracts (`confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`) — see `architecture.md`.
- **Tier**: a harness's derived capability level — A (solo, 4 primitives), B (fan-out foraging, +`spawn-worker`), C (full team loop, +`message-peer`). Always derived from primitive coverage, never hand-declared.

## Agent roles

- **Forager**: the orchestrating agent spawned during planning that fans out scouts and synthesizes this document set (Tier B+). One-shot: after its final message it has no further work in harnesses where it doesn't persist.
- **Scout**: a cheap, read-only, unnamed (no species) context-gathering agent spawned by a forager; examines an assigned module/file-list and writes exactly one report to `.fledge/nest/raw/<module>.md`; self-terminates on its one-line final message.
- **Brooder**: an agent that claims and implements a feather (Tier C team loop); works only in its own worktree; test-first; hands off to a paired skua.
- **Skua**: a reviewer agent paired 1:1 with a brooder; audits tests/diff/scope, checks AC boxes; never modifies code itself.
- **Orchestrator**: the lead agent/session dispatching feathers and reviewing work; role name `fledge-orchestrator`, aliased per harness (on Claude Code: `team-lead`).

## Terms specific to the generalization design (`docs/`)

- **Behavioral identity**: the M0 exit criterion for the bootstrap refactor — files present (modulo location), exit codes, and JSON shape must match; not byte-identical.
- **Trust-git backup model**: re-initializing with `--upgrade-core`/`--refresh` may overwrite the core skill; user edits are recoverable via git history rather than fledge preserving them itself.
- **Capability-conditional prose**: core skill prose that branches on which primitives are declared ("if you provide `spawn-worker`, you may…") rather than on tier labels directly.

## Open Questions

- Whether **spawn-pool** (a 7th primitive in `docs/generalization-plan.md`'s original design — keep N reusable named workers alive) was deliberately descoped from the shipped 6-primitive contract, or folded into `spawn-worker`/`message-peer` semantics. Not resolved by any scout report.
