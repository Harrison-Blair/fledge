---
id: PLM-002
title: "fledge unfledged: list incomplete plumage and feathers"
status: fledged
priority: P2
authored: 2026-07-07T21:18:11Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.0
---

# PLM-002: fledge unfledged: list incomplete plumage and feathers

## Context
Orchestrating agents (and humans) need to see, at a glance, what work remains in a fledge repo: which plumage and which feathers are not yet `fledged`. Today that answer is scattered — `fledge ready` shows only the dispatchable subset of feathers (pipping, deps met, unlocked), `fledge colony` is a fixed repo-wide *summary*, and `fledge status <ID>` is single-item. None gives a plain list of "everything still open," across both spec types. `fledge unfledged` fills that gap: one command that lists every non-complete plumage and every non-complete feather. Because both spec types share `fledged` as their sole terminal status, "non-complete" has a single, exact meaning — `status != fledged` — spanning plumage `{egg, hatched}` and feathers `{egg, pipping, hatching}`. Both consumers are first-class: the human text view and the `--json` view carry the same information, consistent with every existing command's dual-output convention.

## User Stories
- As a developer working in a fledge repo, I want one command that lists all plumage and feathers that are not yet fledged, so that I can see what remains without correlating `ready`, `colony`, and `status`.
- As an orchestrating agent, I want the same list of incomplete plumage and feathers as machine-readable JSON, so that I can survey outstanding work from a single stable contract before deciding what to dispatch.

## Functional Criteria
1. FC-1: `fledge unfledged` lists all non-complete plumage and feathers, where non-complete means `status != fledged` — plumage in `{egg, hatched}` and feathers in `{egg, pipping, hatching}`. Fledged items never appear.
2. FC-2: The human text view has two sections in fixed order — a **Plumage** section then a **Feathers** section. Each plumage line shows `ID · status · priority · title`; each feather line shows `ID · status · priority · title · (plumage <PLM-ID>)`.
3. FC-3: Within each section, items are ordered by priority, then by ID (identical to `fledge ready`).
4. FC-4: The report is status-only: it does not compute or annotate feather readiness (blocked vs. dispatchable). Determining what can be started now remains `fledge ready`'s responsibility.
5. FC-5: `--plumage` restricts output to the Plumage section only; `--feathers` restricts output to the Feathers section only. Passing neither flag, or both flags, shows both sections.
6. FC-6: `--json` emits the same information as a single JSON document, generated from the same computed value as the text view. Feather entries include their `plumage` link; both types include `path` and, where present, `oversight`.
7. FC-7: Degraded spec data does not abort the report: unparseable spec files are summarized in a dedicated issues section (present in both text and JSON) while everything that parses is listed normally. The command exits 0 in this case — it is an observer, not a validator (`fledge preen` remains the validator).
8. FC-8: An empty or spec-less repository produces a valid report (empty sections), exit 0.
9. FC-9: Exit codes follow the existing taxonomy: 0 success (including degraded-data reports), 2 usage error (e.g. an unknown flag), 3 environment error (e.g. not a fledge repo). No report condition produces exit 1.

## Acceptance Criteria
- [x] AC-1: Tests written first and observed failing against the unchanged code, then passing after implementation (per-repo test-first convention).
- [x] AC-2: A txtar e2e suite covers: populated repo (both sections, correct fields, priority-then-ID order, fledged items absent), `--plumage` and `--feathers` scoping, `--json` output shape, empty repo, and a degraded repo (unparseable file surfaced in the issues section, exit 0).
- [x] AC-3: Running `fledge unfledged` in this repository after implementation reflects its own specs accurately (human-verified against `status`/`colony`).
- [x] AC-4: `fledge preen` reports no findings for the spec set after authoring.

## Out of Scope
- Feather readiness annotation (blocked vs. ready) — that is `fledge ready`'s job.
- Grouping feathers beneath their parent plumage — the two sections are flat; `fledge colony` owns the per-plumage grouped view.
- Status-value filtering (e.g. `--status egg`) or priority filtering — only the `--plumage`/`--feathers` type filters are supported.
- Dependency/lock detail per item — surfaced by `ready`/`colony`, not here.
- Markdown/HTML export formats beyond text and `--json`.

## Open Questions
None — all decisions resolved in interrogation (2026-07-07).
