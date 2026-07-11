---
id: FTHR-004
title: wire fledge unfledged into the orchestration skill command surface
plumage: PLM-002
status: fledged
priority: P2
depends_on: [FTHR-003]
oversight: merge
authored: 2026-07-07T21:23:52Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.0
---

# FTHR-004: wire fledge unfledged into the orchestration skill command surface

## Description
Makes `fledge unfledged` visible to orchestrating agents by documenting it in the `fledge-orchestrate` skill's command surface — the half of PLM-002 that turns "the command exists" into "agents actually use it" (satisfies the orchestrating-agent user story). Documentation-only: no Go code, no behavior change. Depends on FTHR-003 so the wording matches the command's shipped flags and output.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate` — the agent-neutral core skill embedded by `internal/bootstrap` and written to `.fledge/skills/` on `init` (see `.fledge/nest/modules.md` → internal/bootstrap). Files: `SKILL.md` (the deterministic-ops command inventory, ~line 36) and, as a usage touchpoint, `implementation.md` (the resume/inventory step) and/or `planning.md` (phase-close survey). No `internal/cli` or Go changes.

## Approach
Prose edits only — match the existing terse, imperative voice of the surrounding bullets; touch only what the addition requires (surgical).
- In `SKILL.md`, extend the deterministic-ops inventory bullet so `fledge unfledged` sits alongside `fledge ready`/`fledge vee` under readiness/structure, framed as "survey all non-fledged plumage and feathers (`fledge unfledged`, `--plumage`/`--feathers` to scope, `--json` for a machine-readable contract)". Keep it one clause; do not restructure the bullet.
- Add exactly one usage touchpoint where an agent surveys outstanding work: either the planning phase-close (`planning.md`, alongside the `fledge ready`/`fledge vee` close-out) or the implementation resume/inventory step (`implementation.md` ~line 133, "Inventory reality"). Prefer whichever reads most naturally; do not add it in multiple places (avoid redundancy).
- Preserve the `fledge ready` vs `fledge unfledged` distinction in the wording: `ready` = dispatchable-now subset; `unfledged` = everything not yet fledged. Do not imply `unfledged` replaces `ready`.
- Do NOT edit any already-materialized copy under `.fledge/skills/` in this repo; the embedded `core/` files are the source of truth (`init --upgrade-core` propagates them). If no such copy exists, nothing else to do.

## Tests
No automated test — this is documentation. Verification is by inspection (this repo has no doc-lint harness; the bootstrap embed is compile-checked by the existing `go:embed` build):
- `go build ./...` still succeeds (the skill files are `go:embed`ded; a missing/renamed file would break the build) — confirms the edits didn't break the embed.
- Manual grep confirms `fledge unfledged` now appears in `SKILL.md`'s command inventory and in exactly one usage touchpoint, and that the `ready` vs `unfledged` distinction is stated.

## Acceptance Criteria
- [x] AC-1: N/A for automated failure-first (documentation feather); instead, `grep -c "fledge unfledged" internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md` was 0 before and ≥1 after.
- [x] AC-2: `SKILL.md`'s deterministic-ops inventory lists `fledge unfledged` with its `--plumage`/`--feathers`/`--json` surface, alongside `ready`/`vee` (satisfies the PLM-002 orchestrating-agent user story).
- [x] AC-3: Exactly one usage touchpoint (planning close **or** implementation inventory) references `fledge unfledged`, and the wording preserves the `ready` (dispatchable-now) vs `unfledged` (all non-fledged) distinction.
- [x] AC-4: `go build ./...` and `go vet ./...` are clean; no `internal/cli` or other Go files changed by this feather.
