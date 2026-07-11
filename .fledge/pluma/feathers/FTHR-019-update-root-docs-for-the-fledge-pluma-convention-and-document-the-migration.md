---
id: FTHR-019
title: Update root docs for the .fledge/pluma/ convention and document the migration
plumage: PLM-011
status: fledged
priority: P2
depends_on: []
authored: 2026-07-11T02:27:04Z
agent: fledge-orchestrate/planning
fledge_version: 0.3.4
---

# FTHR-019: Update root docs for the .fledge/pluma/ convention and document the migration

## Description
Update the four root-level documentation files that reference the old `pluma/` location — `README.md`, `CLAUDE.md`, `docs/generalization-plan.md` — to reflect `.fledge/pluma/`, and add a new "Migrating a fledge 0.3.x repo to 0.4.0" section to `MIGRATION.md` documenting the manual `git mv pluma .fledge/pluma` step repos need to take (superseding its current two notes that `pluma/` doesn't move, at lines 30 and 75). Pure prose, no code or file-overlap dependency on FTHR-017/FTHR-018 — runs in parallel with them.

## Affected Modules
Per `.fledge/nest/modules.md` (`<root>` module) and the reference surface mapped during planning:
- `README.md` (3 references)
- `CLAUDE.md` (3 references)
- `docs/generalization-plan.md` (1 reference)
- `MIGRATION.md` — currently states at line 30 ("Everything else is unchanged: `.fledge/nest/`, `pluma/`, spec files...") and line 75 ("Nothing else moves: `.fledge/nest/`, `pluma/`, and all spec files are untouched...") that `pluma/` does NOT move; both become stale and need a new migration section instead.

## Approach
1. Grep `README.md`, `CLAUDE.md`, `docs/generalization-plan.md` for `pluma/plumage`/`pluma/feathers` references and record the count — "before" evidence.
2. Update each reference to `.fledge/pluma/...`, prose only.
3. In `MIGRATION.md`, add a new top section "Migrating a fledge 0.3.x repo to 0.4.0" (following the file's existing per-version section pattern) documenting: what changed (spec directory convention moved from root `pluma/` to `.fledge/pluma/`), and the manual steps (`git mv pluma .fledge/pluma`, rebuild/reinstall fledge, `fledge init --refresh`, commit). Remove or correct the two now-false statements (lines 30, 75) that claim `pluma/` doesn't move.
4. Re-grep the three docs files and confirm zero stale `pluma/plumage`/`pluma/feathers` references remain (excluding `.fledge/pluma/...`) — "after" evidence.

## Tests
- Grep-based before/after check across `README.md`, `CLAUDE.md`, `docs/generalization-plan.md` (per Approach steps 1 and 4) — pins FC-4 for this surface. Capture "before" (non-zero matches) as FAILING evidence and "after" (zero matches) as passing evidence.
- Manual read-through of `MIGRATION.md`'s new section against the actual steps the repo-migration feather will perform, confirming they match — recorded as evidence, not an automated test (this is documentation, not executable behavior).

## Acceptance Criteria
- [x] AC-1: The grep-based check was observed showing stale references before the edit and zero stale references after, in README.md/CLAUDE.md/docs/generalization-plan.md.
- [x] AC-2: None of the three docs contain a `pluma/plumage` or `pluma/feathers` path reference — satisfies PLM-011 FC-4 for this surface.
- [x] AC-3: `MIGRATION.md` contains a new section documenting the manual `git mv pluma .fledge/pluma` migration step, and no longer claims `pluma/` is unaffected — satisfies PLM-011 AC-4.
