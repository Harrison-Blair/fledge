---
generated: 2026-07-16T04:02:08Z
commit: 154510fc963e7071b2f09297ecfeba2b6710e85e
agent: fledge-forager
fledge_version: 0.5.8
---

# Domain

Glossary of fledge's bird-themed domain vocabulary, reconciled across all scouted modules.

## Spec hierarchy

- **Plumage** (`PLM-###`) — top-level feature/requirement spec (WHAT and WHY). Lifecycle: `egg` (drafted) → `hatched` (user-approved) → `fledged` (all its feathers fledged + all its own acceptance criteria checked). Body: Context, User Stories, Functional Criteria (FC-N), Acceptance Criteria (AC-N checkboxes), Out of Scope, Open Questions.
- **Feather** (`FTHR-###`) — one implementable task under a plumage. Lifecycle: `egg` (drafted/blocked) → `pipping` (dependencies fledged, claimable) → `hatching` (claimed/in progress) → `fledged` (merged + verified). Body: Description, Affected Modules, Approach, Tests, AC-1…AC-N checkboxes. Has `depends_on` (other feather IDs) and optional `oversight` (`merge` = PR-reviewed, `during` = async no-gate).
- **Acceptance Criteria (AC-N)** — checkbox line `- [x] AC-N: text` under a `## Acceptance Criteria` heading; the only sanctioned mutation path is `fledge criteria check`. A feather can't reach `fledged` with unchecked boxes.

## Repository state & artifacts

- **Nest** (`.fledge/nest/`) — distilled repository knowledge: 8 concern docs (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain) + `index.md` + `raw/` (regenerable scout reports). This document set.
- **Brood** — an agent's claim on a feather while working it; a `.fledge/broods/FTHR-###.brood` JSON lock file (owner, PID, branch, worktree, created). Prevents concurrent claims; excludes the feather from `ready`.
- **Molt** — evidence proving a feather's acceptance criteria are actually satisfied (files under `.fledge/molt/` per some conventions; acceptance-criteria headings referenced as "molt headings" in preen diagnostics).
- **Roster** — persistent allocation state (`.fledge/roster/roster.json`) of live agent-species assignments; 18 canonical bird species, pair-aware release semantics, numeric-suffix overflow (`-2`, `-3`, …).
- **Scaffold** — the fledge-owned file set written by `fledge init` (`.fledge/skills/`, `.claude/` or other harness dirs, `.gitignore` patches); tracked by `.fledge/scaffold.json` (the **Stamp**) with per-file content hashes and the fledge version that wrote them.
- **Colony** — aggregate repo-wide status report (`fledge colony`): counts by status, per-plumage completion, orphan feathers (no valid plumage), blocked tasks.
- **Vee** — the feather dependency graph (`fledge vee`): cycle detection, topological **waves** (parallelizable batches), `ready` (unblocked, unstarted feathers).

## Process operations

- **Preen** — validation command (`fledge preen`); runs 13 rules (parse, unknown-field, duplicate-id, schema, ID/filename agreement, dangling-ref, unhatched-plumage, cycle, tests-section, required-sections, stale-pipping-hint, criteria-complete, criteria-evidence) producing `Finding{File, Rule, Severity, Message}`.
- **Forage / Foraging** — the context-regeneration pipeline this very document set is produced by: a **forager** scans the repo, fans out **scouts** in parallel (one per module), then synthesizes their raw reports into the 8 concern docs + index.
- **Scout** — cheap, unnamed subagent spawned by a forager; examines one assigned file list and writes exactly one raw report to `.fledge/nest/raw/<module>.md`; never modifies source.
- **Interrogate** — the planning-phase stress-testing protocol (`fledge-interrogate` skill) applied to a plumage or feather before it's committed — surfaces edge cases, ambiguities, and untested assumptions via targeted questioning.

## Orchestration roles (Tier-C, Claude Code only)

- **Brooder** (`fledge-brooder-<species>`) — implements one feather test-first in a dedicated git worktree; hands off to its paired skua; lives until the feather merges.
- **Skua** (`fledge-skua-<species>`) — reviews its paired brooder's completed feather (re-runs tests, audits AC evidence, red-team pass); escalates on a 3rd rejection; lives until the feather is merged and verified.
- **Incubator** (`fledge-incubator-<species>`) — owns the planning phase end-to-end when delegated (context gathering, interrogation, spec drafting); relays every user decision through the team lead.
- **Forager** (`fledge-forager-<species>`) — one-shot; produces the nest document set (this synthesis).
- **Orchestrator / team-lead** — holds the spawn-worker/kill primitive; commissions workers, receives their final messages, force-terminates non-responsive workers.

## Capability model

- **Primitive** — one of 6 fixed orchestration capabilities: `confirm-gate` (user decision), `read-only-shell` (inspection), `write-file` (create/edit), `run-fledge` (CLI mutation), `spawn-worker` (create a subagent/teammate), `message-peer` (async by-name messaging).
- **Tier** — capability level (A/B/C) *derived* from which primitives a harness adapter provides, never declared: A = base 4 (solo work), B = A + spawn-worker (fan-out foraging), C = B + message-peer (full team loop with brooder/skua pairs and incubator delegation). Claude Code = Tier C; Codex and pi = Tier A each (confirmed via `registry_test.go`'s `TestPrimitiveCoverage`).
- **Harness / Adapter** — the agent tool fledge scaffolds into (Claude Code, Codex, pi); an adapter is a thin, manifest-driven, format-only mapping from the 6 primitives to that harness's concrete mechanisms.

## Open Questions

- Whether "molt" as evidence-files-under-`.fledge/molt/` and "molt" as acceptance-criteria-heading terminology (seen in preen diagnostic wording, per CLAUDE.md context) are the same concept described two ways, or two distinct usages — not disambiguated by any single scout report.
