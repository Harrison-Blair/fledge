---
generated: 2026-07-08T05:28:12Z
commit: e46c481a047d45ef10bcd79a3326d47932b32868
agent: fledge-forager
fledge_version: 0.2.1
---

# Domain

fledge's vocabulary is bird-themed and load-bearing — command names, package names, agent roles, and lifecycle states all use it, and new code/prose is expected to match. This is the glossary; `README.md` (~lines 8-12) is the shipped decoder.

## Core artifacts

- **plumage** (`PLM-###`) — a requirement/feature spec (the WHAT/WHY). Lifecycle `egg → hatched → fledged`. Lives in `pluma/plumage/`.
- **feather** (`FTHR-###`) — one implementable task under a plumage (the HOW). Lifecycle `egg → pipping → hatching → fledged`. Lives in `pluma/feathers/`.
- **nest** (`.fledge/nest/`) — distilled repository knowledge: eight concern docs + `index.md` (routing) + `raw/` scout reports.
- **brood** — a claim/lock on a feather while an agent works it (`.fledge/broods/FTHR-###.brood`).
- **molt** — the per-feather evidence file (`.fledge/molt/FTHR-###.md`) holding per-criterion (AC-N) proof.
- **pluma/** — the parent directory holding the plumage and feather corpora.

## Lifecycle states

- **egg** — drafted but blocked (plumage not yet hatched / feather has unmet `depends_on`).
- **hatched** (plumage only) — approved; feathers may now be authored against it.
- **pipping** (feather only) — ready to dispatch: all `depends_on` fledged, no brood held.
- **hatching** (feather only) — claimed and in progress (brood held).
- **fledged** — complete: merged, all acceptance criteria checked, evidence captured. Terminal.

## Operations (commands)

- **preen** — validate all specs (`internal/check`): dangling refs, bad frontmatter, unchecked-but-fledged criteria, missing evidence.
- **vee** — the dependency graph (`internal/graph`): waves, cycles, dot output.
- **colony** — repo-wide progress report: status counts, per-plumage completion, orphan feathers, dangling refs, blocked feathers, active broods, degraded-data issues (observer; exits 0).
- **unfledged** — the full slate of non-fledged plumage/feathers (distinct from **ready**, the dispatchable-now subset).
- **scan** — module inventory (drives foraging).

## Agent roles (implementation/planning workflow)

- **forager** (`fledge-forager-<species>`) — self-orchestrating context-gatherer: scans the repo, fans out scouts, synthesizes the nest. One-shot per planning phase.
- **scout** (`fledge-context-scout`) — unnamed worker spawned by the forager per module; reads only its assigned files, writes one `raw/<module>.md`.
- **brooder** (`fledge-brooder-<species>`) — ephemeral implementor: one feather, its own worktree, test-first, hands off to its skua; lives until merge.
- **skua** (`fledge-skua-<species>`) — ephemeral reviewer paired 1:1 with a brooder (same species); re-runs tests, audits test-first evidence, checks the diff against the spec; the only worker that mutates a spec (via `fledge criteria`). Lives until merge.

## Pairing & naming

Worker names are `<role>-<species>` drawn from an 18-penguin-species pool (emperor, king, adelie, chinstrap, gentoo, little, yellow-eyed, african, humboldt, magellanic, galapagos, fiordland, snares, erect-crested, southern-rockhopper, northern-rockhopper, royal, macaroni). **One species per brooder+skua pair** — the pairing is currently maintained by the orchestrator across two separate spawns and encoded only in ephemeral spawn prompts; a species frees for reuse only after both members confirm shutdown. Solo spawns (the forager) take a species of their own. The orchestrator itself takes no species (`fledge-orchestrator`; reached by its harness-assigned name, e.g. `team-lead` on Claude Code).

## Structural concepts

- **primitive** — one of six orchestration capabilities (`confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`); a contract, not a mechanism.
- **tier** — capability level *derived* from primitive coverage (A=solo/4, B=foraging/5, C=team-loop/6), never declared.
- **adapter / harness** — a target agent system (Claude/pi/Codex) and its manifest-driven format mapping.
- **oversight** — optional feather flag: `during` (user participates through implementation) / `merge` (user signs off the reviewed diff) / omitted (autonomous).
- **tracer (bullet)** — a thin end-to-end feather that proves the architecture; later feathers widen it.
- **orphan feather / dangling ref / degraded data** — integrity anomalies surfaced by `colony`: unresolved plumage link, unresolved `depends_on`, unparseable specs.
