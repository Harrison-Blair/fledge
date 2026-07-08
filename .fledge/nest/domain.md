---
generated: 2026-07-08T01:03:26Z
commit: e44524d1f089dcfe1c1f313f819ec18d9a42eceb
agent: fledge-forager
fledge_version: 0.2.1
---

# Domain Terms

Glossary of fledge's bird-themed vocabulary and orchestration concepts, reconciled across all scouted modules. `README.md` is the canonical decoder for the metaphor; this glossary adds file-level grounding.

## Spec corpus

- **Plumage** (`PLM-###`) — a feature/requirement spec, `pluma/plumage/PLM-###.md`. Lifecycle: `egg → hatched → fledged`. Body sections: Context, User Stories, Functional Criteria (FC-N), Acceptance Criteria, Out of Scope, Open Questions.
- **Feather** (`FTHR-###`) — an implementable task under a plumage, `pluma/feathers/FTHR-###.md`. Lifecycle: `egg → pipping → hatching → fledged`. Body sections: Description, Affected Modules, Approach, Tests, Acceptance Criteria.
- **Fledged** — terminal status for both plumages and feathers; all acceptance criteria checked, work complete.
- **Unfledged** — anything not yet `fledged`; also the name of the `fledge unfledged` reporting command.
- **Tracer task** — a feather scoped to implement a whole feature end-to-end (e.g. FTHR-001, FTHR-003 in this repo's own spec history), as opposed to follow-on feathers that widen or wire in an already-tracered feature.
- **Depends_on** — a feather's list of prerequisite feather IDs; acyclic, validated at creation and on `set`.
- **Oversight** — an optional feather flag (`merge` or `during`) that gates completion behind review at a specific point.
- **Functional Criteria (FC-N)** — numbered, testable statements in a plumage body, defining what the feature must do.
- **Acceptance Criteria (AC-N)** — checkbox list (`- [ ] AC-N: text`) in a spec body, verifiable conditions for task completion; checked only via `fledge criteria check`.

## Repo-local state (`.fledge/`)

- **Nest** (`.fledge/nest/`) — repository knowledge store; this document set. Raw scout reports live under `.fledge/nest/raw/`.
- **Brood** (`.fledge/broods/<FTHR-ID>.brood`) — an advisory lock/claim file recording who is actively working a feather (owner, PID, created time, branch). "Brooding" a feather = claiming it.
- **Molt** (`.fledge/molt/`) — evidence directory storing per-acceptance-criterion verification output for a feather (test-first: failing run pre-implementation, passing run post-implementation).
- **Colony** — the `fledge colony` command; a repo-wide progress report (counts, per-plumage completion, blocked-task detail, active locks, degraded-data issues).

## CLI verbs (bird-themed commands)

- **Preen** — `fledge preen`; validates the spec set against the check-rule engine, surfacing errors/warnings.
- **Vee** — `fledge vee`; dependency graph visualization (named for the "V" shape of a bird formation / upstream-downstream shape), supports waves, cycle detection, dot/JSON output.
- **Scan** — `fledge scan`; file inventory grouped by top-level module, `.fledgeignore`-filtered; the authoritative work-list for forager scout assignment.
- **Ready** — feathers eligible to start: dependencies met (all `fledged`), not currently brooded.

## Orchestration roles (agent-facing, Tier B/C)

- **Forager** — the planning-phase worker (needs `spawn-worker`) that orchestrates scouts and synthesizes their reports into `.fledge/nest/`. One-shot: no further work after its final message.
- **Scout** — a cheap, unnamed forager subagent assigned one module and an explicit file list; writes exactly one raw report to `.fledge/nest/raw/<module>.md`, then self-terminates.
- **Brooder** — an ephemeral team-loop (Tier C) worker, one per feather, one dedicated git worktree; implements test-first and hands off to its assigned skua.
- **Skua** — a persistent team-loop worker that reviews a brooder's completed feather (re-runs tests, audits test-first evidence, reports approval/findings to the orchestrator).
- **Orchestrator** — the user-proxying role driving the implementation phase; never given a species name — uses whatever identity the harness assigns (e.g. `team-lead` on Claude Code).
- **Species** — the unique per-worker identifier assigned on spawn: a penguin name (emperor, king, adelie, ...) with a numeric suffix once the 18-name base list is exhausted.

## Harness/adapter concepts

- **Harness** — a target agent execution environment: currently Claude Code, pi, Codex (0.2.0); Cursor, opencode planned for 0.3.0.
- **Primitive** — one of 7 canonical orchestration capabilities a harness may provide: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `spawn-pool`, `message-peer`.
- **Tier** — capability level (A = solo, B = adds fan-out foraging, C = adds team loop) *derived* from an adapter's declared primitive coverage, never hand-declared.
- **Adapter** — a manifest + scaffolded files mapping fledge's primitives to one harness's actual mechanisms; format-only, zero Go code per new harness.
- **Core skill** — agent-neutral workflow prose (`fledge-orchestrate`, `fledge-interrogate`), written identically to `.fledge/skills/` regardless of harness.
- **Manifest** — the YAML file that is the single source of truth for one adapter (detector, primitive coverage, file write policies).
- **Interrogate** — the `fledge-interrogate` skill; a structured, one-question-at-a-time interview used to stress-test a plumage/feature design before it's written.
- **Piping file** — adapter-specific prose describing a harness's runtime mechanics (tmux display, `/resume` recovery, permission inheritance) — a Tier C concern.
- **Duplicate guard** — the refusal mechanism (`CheckDuplicateSkills`) preventing `fledge init` from scaffolding over a pre-existing, non-symlinked skill copy at a harness's native path.

## Open Questions
None outstanding — all terms surfaced by scouts (including "skua", initially unresolved by the root-module scout) were subsequently grounded in `internal-bootstrap`/`internal-domain`/`docs` reports.
