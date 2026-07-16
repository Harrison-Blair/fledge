---
id: FTHR-058
title: Repoint core-doc and CLAUDE.md references to per-role files
plumage: PLM-027
status: fledged
priority: P1
depends_on: [FTHR-057]
authored: 2026-07-16T16:16:43Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-058: Repoint core-doc and CLAUDE.md references to per-role files

## Description
Repoint every reference to `worker-protocols.md §Incubator`/`§Brooder`/`§Skua` in the core orchestration docs and in `CLAUDE.md` to the new per-role files created by FTHR-057 (satisfies PLM-027 FC-4, the core-docs half).

## Affected Modules
Per `.fledge/nest/modules.md` (internal-bootstrap-core) and `.fledge/nest/architecture.md`:
- `internal/bootstrap/core/skills/fledge-orchestrate/planning.md` (2 references, lines 9 and 12/19 in the current file)
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` (1 reference, line 28)
- `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` (1 reference, line 9)
- `CLAUDE.md` (repo root, 1 reference, line 123 — mentions `worker-protocols.md` in the list of core files under `core/`)

## Approach
Grep-confirmed exact reference sites (current line numbers, will shift after FTHR-057 lands but the phrasing is stable):
- `planning.md:9` — "...per the Incubator protocol in `worker-protocols.md`." → "...per the Incubator protocol in `incubator.md`."
- `planning.md:12` — "(envelope in `worker-protocols.md` §Incubator)" → "(envelope in `incubator.md`)"
- `planning.md:19` — "...per `worker-protocols.md` §Incubator when delegated." → "...per `incubator.md` when delegated."
- `implementation.md:28` — "...see `planning.md` §0 and `worker-protocols.md` §Incubator." → "...see `planning.md` §0 and `incubator.md`."
- `foraging.md:9` — "`planning.md` §2 and `worker-protocols.md` point here." → "`planning.md` §2 and `incubator.md` point here." (foraging is read by whichever role is the commissioner — currently only the orchestrator/incubator ever act as commissioner per `foraging.md`'s own text, so `incubator.md` is the correct single target, not all three role files).
- `CLAUDE.md:123` — the file list "(planning.md, implementation.md, worker-protocols.md, templates/)" → "(planning.md, implementation.md, worker-protocols.md, incubator.md, brooder.md, skua.md, templates/)" (worker-protocols.md still exists as the stub, so it stays in the list; the three new files are added).

Do NOT touch `worker-protocols.md` itself (owned by FTHR-057) or the Claude adapter agent files (owned by FTHR-059).

## Tests
- `TestCoreDocsRepointToRoleFiles` (new, `internal/bootstrap`, alongside the doc-structure tests from FTHR-057): reads the embedded `planning.md`, `implementation.md`, `foraging.md` and asserts none contain the stale pattern `worker-protocols.md §` (case-sensitive substring), and that `planning.md`/`implementation.md`/`foraging.md` each contain `incubator.md`.
- `TestClaudeMdReferencesRoleFiles` (new, `internal/doctest`, following the existing `readRoot` helper pattern used for README/RELEASING.md): reads root `CLAUDE.md` and asserts it contains `incubator.md`, `brooder.md`, and `skua.md`.
- Implementation order: write both tests against the unchanged repo (they fail — the strings aren't there yet / the stale pattern still is), then make the edits, confirm both pass.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `planning.md`, `implementation.md`, `foraging.md` (embedded core) contain zero occurrences of `worker-protocols.md §` and reference `incubator.md` at each of the sites listed in Approach (satisfies PLM-027 FC-4, AC-3 — core-docs half).
- [x] AC-3: `CLAUDE.md` lists `incubator.md`, `brooder.md`, `skua.md` alongside `worker-protocols.md` in its core-files description.
- [x] AC-4: `go test ./internal/bootstrap/... ./internal/doctest/...` passes.
