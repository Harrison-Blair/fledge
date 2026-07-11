---
generated: 2026-07-11T01:58:32Z
commit: 96a3ac38bc843217824d6d6886c49906053bf686
agent: fledge-forager
fledge_version: 0.3.4
---

# Domain

Glossary of fledge's bird-themed domain vocabulary, reconciled across all scouted modules. See `README.md` for the canonical decoder.

## Spec artifacts

- **Plumage (`PLM-###`)** — a feature/requirement spec (`.fledge/pluma/plumage/`). Sections: Context, User Stories, Functional Criteria, Acceptance Criteria, Out of Scope, Open Questions. Lifecycle: `egg` (draft) → `hatched` (spec complete) → `fledged` (all feathers done).
- **Feather (`FTHR-###`)** — an implementable task spec under a plumage (`.fledge/pluma/feathers/`). Sections: Description, Affected Modules, Approach, Tests, Acceptance Criteria. Lifecycle: `egg` (unstarted/blocked) → `pipping` (ready to work, deps satisfied) → `hatching` (actively worked, brood held) → `fledged` (complete, all AC checked).
- **Acceptance Criteria (AC-N)** — checkbox items in a spec body; the only supported mutation path is `fledge criteria check`/`uncheck`, never hand-editing.
- **Functional Criteria (FC-N)** — testable requirement statements in feather specs, referenced by ID and pinned by txtar test assertions.
- **Frontmatter** — YAML metadata header (`id`, `title`, `status`, `priority`, `authored`, `agent`, `fledge_version`, plus `plumage`/`depends_on`/`oversight` for feathers) bounded by `---` delimiters; CLI-owned, never hand-edited.
- **Oversight** — optional feather field; `merge` means a human reviews and approves the diff before merge.

## Work lifecycle

- **Brood** — an active claim (lock) on a feather while an agent works it; a JSON record in `.fledge/broods/*.brood` (owner, PID, branch, created time). `fledge brood`/`abandon`/`broods` manage it.
- **Ready** — feathers in `pipping` status with all dependencies `fledged`; the dispatch candidate set (`fledge ready`).
- **Wave** — one parallel batch in the dependency-ordered execution plan (`fledge vee --format text`).
- **Colony** — summary report of plumage/feather status counts, per-plumage completion, orphans, blocked deps, held broods (`fledge colony`/`report`).
- **Orphan** — a feather whose plumage reference doesn't resolve to an existing spec.
- **Dangling ref** — any reference (plumage link, `depends_on` entry) pointing to a spec that doesn't exist.
- **Molt** — scaffold refresh (`fledge init --refresh`): resets fledge-owned files to shipped versions, preserves user edits where policy allows, prunes obsolete files.

## Repo knowledge & orchestration

- **Nest (`.fledge/nest/`)** — this directory: distilled repository knowledge for planning agents. Contains 8 synthesized concern docs + `index.md`, and `raw/` (per-module scout reports).
- **Concern doc** — one of the 9 known synthesis documents (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain, index) — a closed, ordered set (`internal/nest/docs.go:ConcernDocs`).
- **Scout report** — a raw, module-scoped analysis written by a `fledge-context-scout` worker to `.fledge/nest/raw/<module>.md`; the scout's assigned modules form an open set.
- **Forager** — the worker (or the planner itself, if `spawn-worker` is unavailable) that orchestrates scouts and synthesizes concern docs from their raw reports.
- **Scout** — a cheap, narrowly-scoped worker spawned by the forager; reads only its assigned files, writes exactly one raw report.
- **Brooder** — a spawned team worker (implementation phase) that implements one feather test-first, in a dedicated git worktree.
- **Skua** — a spawned team worker paired 1:1 with a brooder; reviews the brooder's completed feather against its spec.
- **Incubator** — a delegated planning worker that owns the planning phase end-to-end (context gathering, interrogation, spec drafting) and relays user decisions to the team lead.

## Scaffolding & harness support

- **Scaffold** — the set of fledge-owned files written by `fledge init` into a target repo (`.fledge/`, `pluma/`, and harness-specific files like `.claude/`).
- **Scaffold stamp (`.fledge/scaffold.json`)** — records every fledge-owned file, its write policy, and a content hash/target/lines-count (depending on policy); the basis for drift detection.
- **Drift** — divergence between a scaffolded file's on-disk state and its expected state; one of up-to-date, stale, modified, missing, or obsolete.
- **Adapter** — a harness-specific file mapping (Claude Code, Codex, pi), declared entirely in one `manifest.yaml`.
- **Primitive** — one of 6 orchestration capability contracts an adapter can provide: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`.
- **Tier (A/B/C)** — a harness's derived capability level based on which primitives its adapter provides: A = solo planning only, B = adds foraging (scout fan-out), C = adds the full team loop (brooder/skua pairs).
- **Write policy** — how a scaffolded file is created/updated: `generate`/`primitive_map` (rendered from a Go template), `overwrite` (always repaired), `append_if_missing` (additive line, never deleted), `symlink` (points into `.fledge/skills/...`), or default (copy, skip-if-exists so user edits survive until `--refresh`).

## Open Questions

- What determines the canonical `agent` value stamped into nest docs (explicit `--agent` override vs. forager-provided vs. hardcoded default) was not resolved by any scout — see `internal/nest/nest.go`.
