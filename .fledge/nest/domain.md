---
generated: 2026-07-17T07:00:54Z
commit: ee49464adb830bef7189f94a1d3253927d33fb5f
agent: fledge-forager
fledge_version: 0.6.7
---

# Domain

Glossary of fledge's bird-themed domain vocabulary, spanning specs, orchestration, and the (new) handoff ledger.

## Spec vocabulary

- **Plumage** (`PLM-###`) — a feature/requirement spec: user intent, stories, functional criteria (FC-N), acceptance criteria (AC-N), scope. Lifecycle: egg → hatched → fledged. File: `.fledge/pluma/plumage/PLM-###-<kebab>.md`.
- **Feather** (`FTHR-###`) — an implementable task spec linked to one plumage: approach, tests, acceptance criteria (AC-N checkboxes), `depends_on` other feathers, optional `oversight` timing (`merge` | `during`). Lifecycle: egg → pipping → hatching → fledged. File: `.fledge/pluma/feathers/FTHR-###-<kebab>.md`.
- **Acceptance Criteria (AC-N)** — numbered checkbox verifications under a `## Acceptance Criteria` heading; checked only via `fledge criteria check` (never hand-edited).
- **Functional Criteria (FC-N)** — numbered testable behavior statements in a plumage; a feather's AC references them as "satisfies {{REQ}} FC-N".
- **Pipping** — a feather whose dependencies are all fledged and which is not already claimed — ready to dispatch. `status: egg` in frontmatter does *not* by itself mean not-ready: `fledge ready`/`fledge vee` recompute true dispatchability; frontmatter status is only an authoring hint.
- **Brood** — an active claim on a feather while an agent works it; `fledge brood` allocates, `fledge abandon` releases, `fledge broods` lists. Recorded as `<task>.brood` JSON under `.fledge/broods/` (owner, PID, branch, worktree).
- **Molt** — a spec-section update; more specifically, the evidence directory (`.fledge/molt/FTHR-###.md`) housing per-feather documentation for satisfied criteria.
- **Preen** (`fledge preen`) — validates all specs; `--strict` promotes warnings to failures.
- **Colony** (`fledge colony`) — full spec inventory: status counts, per-plumage completion, orphans.
- **Wave** — a topological layer of feathers computable in parallel (`graph.Waves()`).
- **Kebab** — the hyphenated-lowercase filename slug derived from a spec's title (e.g. `PLM-001-deterministic-cli.md`).

## Orchestration vocabulary

- **Nest** (`.fledge/nest/`) — synthesized repository knowledge: this document set (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain, index) plus `raw/<module>.md` scout reports.
- **Concern doc** — one of the 8 synthesized context documents above (everything except `index.md`).
- **Forager** — the agent that orchestrates scouts, synthesizes their raw reports into the concern docs, and writes `index.md`. One-shot per regeneration.
- **Scout** (`fledge-context-scout`) — a cheap, narrowly-scoped agent that reads exactly one module's assigned files and writes one `raw/<module>.md` report; never modifies source, never synthesizes.
- **Skua** — reviewer agent, paired 1:1 with a brooder; reviews a completed feather against its spec.
- **Brooder** — implementor agent spawned 1:1 with a skua in a dedicated git worktree; implements test-first, hands off for review.
- **Incubator** — delegated planning agent that owns the planning phase end to end (context gathering, interrogation, spec drafting).
- **Roster** — the species-token allocation table (`fledge roster`) giving each worker a unique penguin-species name (18 canonical species, overflow to numeric suffixes).
- **Species** — a worker's identity suffix in role-species naming (e.g. `fledge-forager-gentoo`, `fledge-incubator-emperor`).
- **Digest** — an output summary written by a phase or worker (`digest-planning.md`, `digest-foraging.md`, `digest-implementation.md`) grounding the next phase.
- **Primitive** — one of 6 orchestration-capability abstractions an adapter may provide: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`.
- **Tier** — a harness's derived capability level from its primitive coverage: A (solo: first 4 primitives), B (+`spawn-worker`, fan-out foraging), C (+`message-peer`, full team loop). Never declared directly by an adapter — always computed.
- **Adapter** — a harness-specific realization of the workflow: a manifest (primitives, file map, piping) plus prompts/settings; zero workflow logic of its own.
- **Harness** — an agent platform (Claude Code, pi, Codex); each has exactly one adapter.
- **Manifest** — the YAML single source of truth per adapter (`internal/bootstrap/adapters/<harness>/manifest.yaml`).
- **Scaffold** — the set of files `fledge init` writes to make a repo fledge-managed.
- **Stamp** (`.fledge/scaffold.json`) — the record of what init wrote (content hashes, symlink targets, required append lines, fledge version, scaffolded agents); enables drift detection and refresh preservation.
- **Drift** — on-disk divergence from the expected scaffold state; 5 statuses (up-to-date, stale, modified, missing, obsolete).
- **Dev-link** (PLM-031) — a mode where copy-type scaffold files are symlinked into the fledge source tree instead of copied, so source-tree edits reflect live in a fledge-managed repo without a re-run; a "self-link" variant uses relative symlinks for portability.

## Ledger vocabulary (PLM-030, new)

- **Ledger** — persistent agent handoff records under `.fledge/ledger/<subject>.<kind>.json`; latest-value-only, written atomically.
- **Subject** — the worker name or feather ID a ledger record is keyed to.
- **Kind** — one of `status` (heartbeat/liveness, repeatedly written), `verdict` (pass/fail review outcome, write-once), `escalation` (free-text blocker for the orchestrator, write-once).
- **Heartbeat** (`fledge heartbeat`) — writes a fresh `status` record (alive signal).
- **Await** (`fledge await`) — polls for a ledger record's appearance or change; two distinct modes:
  - **Change-wait** (default; `status` kind) — baseline-sampled at call time, returns on first divergence from the baseline.
  - **Existence-wait** (`--exists`; `verdict`/`escalation` kinds) — no baseline; returns on first read where the record is present.
  `--timeout` is mandatory on both paths; a distinct `ExitTimeout` (4) exit code fires on elapse.
- **Stale** — a `status` record whose lease timestamp exceeds the 5-minute `StaleAfter` TTL, indicating the worker is no longer reporting.

## Terms decoded in `README.md`

Full bird-theme cross-reference (nest, plumage, feather, brood, preen, molt, forager, skua) is authored there; this glossary reconciles it against actual code behavior observed by the scouts, not just the prose description.

## Open Questions

None — domain terms were used consistently across all nine raw reports with no conflicting definitions.
