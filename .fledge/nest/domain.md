---
generated: 2026-07-16T02:20:48Z
commit: 407b91e70b53764944447dae5829d2076fb852c5
agent: fledge-forager
fledge_version: 0.5.5
---

# Domain

Glossary of fledge's bird-themed domain vocabulary, reconciled across scout reports.

- **Plumage** (`PLM-###`) — a requirement/feature-intent spec. Status lifecycle: `egg` → `hatched` → `fledged`. Holds user stories, Functional Criteria (FC-N), and Acceptance Criteria checkboxes.
- **Feather** (`FTHR-###`) — an implementable task under a plumage. Status lifecycle: `egg` → `pipping` → `hatching` → `fledged`. May `depends_on` other feathers; may declare `oversight` (`merge` or `during`).
- **Functional Criteria (FC-N)** — numbered behavioral statements in a plumage's body; feathers reference them as evidence the requirement is satisfied.
- **Acceptance Criteria (AC-N)** — checkbox lines (`- [ ] AC-N: text`) under a `## Acceptance Criteria` heading; the *only* sanctioned way to check one is `fledge criteria check`.
- **Pipping** — a feather's computed "ready to work" state: all its dependencies are fledged and it is not currently brooded.
- **Hatching** — a feather actively being worked (a brood claim is held on it).
- **Fledged** — the terminal, complete state (feather or plumage). A spec set is "fledged" when everything reaches this state.
- **Brood** — the claim/lock an agent holds on a feather while working it: `{Task, Owner, PID, Created, Branch}`, stored as `.fledge/broods/<FTHR-ID>.brood`. Acquiring one flips status to `hatching`; releasing one (via `abandon`) optionally flips to `fledged`.
- **Preen** — the validation command (`fledge preen`); runs `internal/check`'s ~20 rules plus scaffold-drift comparison; exits nonzero on errors.
- **Molt** — the evidence directory (`.fledge/molt/`) holding per-feather markdown documenting acceptance-criteria verification. (Also used loosely elsewhere for "acceptance criteria" — the CLAUDE.md convention list calls molt headings `## AC-N`; the authoritative referent is the evidence directory per `internal/check`'s criteria-evidence rule.)
- **Nest** (`.fledge/nest/`) — the distilled repository-knowledge document set this very file belongs to: eight concern docs (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain) plus `index.md`, synthesized from scout reports under `raw/`.
- **Scout** / **Context Scout** — an ephemeral worker that examines an assigned module's files and writes one raw report to `.fledge/nest/raw/<module>.md`; never modifies source, never synthesizes.
- **Forager** — the worker that orchestrates scouts and synthesizes their raw reports into the nest's concern docs and index (this document set is the forager's output).
- **Brooder** — the worker that implements one feather end-to-end (test-first, evidence, commit) and hands off to a paired skua.
- **Skua** — the reviewer paired 1:1 with a brooder; re-runs tests, audits test-first evidence, checks AC boxes; never modifies code or merges.
- **Incubator** — the delegated planning worker: owns context-gathering, interrogation, spec drafting, and planning-phase CLI mutations end-to-end.
- **Scaffold** — the fledge-owned file tree (`​.fledge/skills/`, harness adapter files, agent specs) written by `fledge init`; recorded in `.fledge/scaffold.json` (the **stamp**) with per-file write policy and content hash.
- **Drift** — the on-disk classification of a scaffolded file relative to the stamp: `up-to-date`, `stale` (binary moved, unedited — refresh-safe), `modified` (user-edited — needs confirmation), `missing`, or `obsolete` (no longer shipped).
- **Primitive** — one of 6 orchestration capabilities an agent harness may provide: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`.
- **Tier** — a harness adapter's derived capability level (A/B/C), computed from which primitives it provides — never declared directly. A = {confirm-gate, read-only-shell, write-file, run-fledge}; B adds spawn-worker; C adds message-peer.
- **Harness** — the agent runtime consuming fledge's scaffolded output (Claude Code, pi, Codex; docs/generalization-plan.md also names Cursor and opencode as target/aspirational harnesses).
- **Adapter** — the thin, harness-specific mapping (a `manifest.yaml` + files) that realizes core prose in one harness's native primitives; adding a harness means adding a manifest, never Go code.
- **Manifest** — an adapter's single source of truth: detector marker, `tier_primitives` map, file list with write policies, optional piping file.
- **Core** — the agent-neutral orchestration skill content (`fledge-orchestrate`, `fledge-interrogate`) embedded once and scaffolded into every harness identically.
- **Wave** — a topological batch of feathers from `internal/graph.Waves()`: all feathers in wave N have every dependency satisfied by waves 1..N-1; used for parallel-dispatch planning (`fledge vee`).
- **Vee** — the dependency-graph command/visualization (`fledge vee`): waves, cycle detection, dot/JSON/text output.
- **Colony** — the high-level completion report command (`fledge colony`): feather counts by status, per-plumage completion %, orphans, blocked feathers, active broods, parse errors.
- **Unfledged** — spec items (plumages or feathers) not yet in the `fledged` state; also the name of the command listing them.

## Open Questions

- `docs/generalization-plan.md` refers to a `spawn-pool` primitive (7-primitive contract) not present in shipped code (6 primitives) — unclear whether dropped, renamed, or deferred.
- Exact scope of "molt": `internal/check`'s evidence rule and CLAUDE.md's "molt headings" convention both use the term, but for two related-but-distinct things (an evidence directory vs. an AC-checkbox heading style); the canonical definition wasn't settled by any single scout.
