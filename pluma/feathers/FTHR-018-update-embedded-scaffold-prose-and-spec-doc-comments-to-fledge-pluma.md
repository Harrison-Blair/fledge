---
id: FTHR-018
title: Update embedded scaffold prose and spec doc comments to .fledge/pluma/
plumage: PLM-011
status: hatching
priority: P2
depends_on: []
authored: 2026-07-11T02:25:28Z
agent: fledge-orchestrate/planning
fledge_version: 0.3.4
---

# FTHR-018: Update embedded scaffold prose and spec doc comments to .fledge/pluma/

## Description
Update the `pluma/` path references baked into fledge's embedded, agent-neutral scaffold prose — the `fledge-orchestrate` skill files and templates under `internal/bootstrap/core/` — plus the doc comments in `internal/spec/types.go`, so both read `.fledge/pluma/plumage`/`.fledge/pluma/feathers` consistent with the new convention. This is pure prose/comment editing (no Go logic changes) and has no file overlap or functional dependency on the code change in FTHR-017, so it can be implemented in parallel with it.

## Affected Modules
Per `.fledge/nest/architecture.md` (`internal/bootstrap/core/` is the single agent-neutral source written to a target repo's `.fledge/skills/` by `WriteCore()`) and the reference surface mapped during planning:
- `internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md`
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md`
- `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md`
- `internal/bootstrap/core/skills/fledge-orchestrate/templates/plumage.md`
- `internal/bootstrap/core/skills/fledge-orchestrate/templates/feather.md`
- `internal/spec/types.go` — doc comments at lines 26 and 40 referencing the `pluma/` path.

## Approach
1. Grep these 6 files for `pluma/plumage` and `pluma/feathers` path-string references (excluding any already-correct `.fledge/pluma/...` occurrences) and record the count — this is the "before" evidence.
2. Replace each reference with the `.fledge/pluma/...` equivalent, prose only — do not change section structure, wording unrelated to the path, or any other content.
3. Re-grep and confirm zero stale `pluma/plumage`/`pluma/feathers` references remain in these 6 files (excluding correct `.fledge/pluma/...` occurrences) — the "after" evidence.
4. Run `go test ./internal/bootstrap` to confirm the embedded-content tests (`TestCoreNeutral`, `TestSkillFrontmatter`, `TestAdapterManifests`, etc.) still pass — these tests check structural/frontmatter properties of the embedded files, not the specific path prose, so they should be unaffected; a failure here would indicate an unintended structural change.

## Tests
- Grep-based before/after check across the 6 files (per Approach steps 1 and 3) — pins FC-4 for this surface. Capture the "before" grep output (non-zero matches) as the FAILING evidence, and the "after" grep output (zero matches) as the passing evidence.
- `go test ./internal/bootstrap` — confirms no structural regression; run before and after to show it was green throughout (this feather doesn't change Go code, so this isn't a fail→pass test, but its output is still captured as evidence that nothing else broke).

## Acceptance Criteria
- [ ] AC-1: The grep-based check was observed showing stale references before the edit and zero stale references after.
- [ ] AC-2: None of the 6 affected files contain a `pluma/plumage` or `pluma/feathers` path reference (all read `.fledge/pluma/...`) — satisfies PLM-011 FC-4 for this surface.
- [ ] AC-3: `go test ./internal/bootstrap` passes unchanged.
