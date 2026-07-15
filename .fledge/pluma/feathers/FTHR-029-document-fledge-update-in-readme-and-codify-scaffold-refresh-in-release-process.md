---
id: FTHR-029
title: Document fledge update in README and codify scaffold refresh in release process
plumage: PLM-015
status: hatching
priority: P2
depends_on: []
authored: 2026-07-15T14:58:06Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-029: Document fledge update in README and codify scaffold refresh in release process

## Description
Make `fledge update` discoverable in the human-facing docs and give the release process a
single documented home that includes the scaffold-refresh step, so the dogfood scaffold
stays in sync on future version bumps. This is preventive — the scaffold is currently in
sync; the step codifies the practice so it can't drift. Independent of the source change —
touches only docs — so it can be implemented in parallel with FTHR-028.

Satisfies PLM-015 FC-5 (README) and FC-6 (release-process doc).

## Affected Modules
- `README.md` — the Commands table (§Commands, lines ~69–82, currently ending at
  `fledge version`) and the Upgrading section (§Upgrading, lines ~87–98, which today only
  covers `fledge init --refresh` for scaffold files, never binary self-update).
- `RELEASING.md` — new file at repo root (none exists today; release steps are currently
  tribal knowledge). See `.fledge/nest/modules.md → <root>` for where root docs live.

## Approach
1. **README Commands table.** Add a `fledge update` row describing binary self-update
   (dry-run by default surfacing the available version/notes; `--yes` to apply;
   `--json` supported), matching the table's existing style and column layout.
2. **README Upgrading section.** Add a bullet distinguishing the two kinds of upgrade:
   scaffold files (`fledge init --refresh`, already documented) vs. the binary itself
   (`fledge update`), so a reader knows how to update each.
3. **New `RELEASING.md`.** Document the actual release steps as they exist today plus the
   refresh step: bump the `VERSION` file and the other version-stamped locations that must
   move with it (enumerate them from the current release history — VERSION plus the
   hardcoded `binaryVersion` and any fixture that pins the version), tag/push to trigger
   `.github/workflows/release.yml`, and — the new requirement — run `fledge init --refresh`
   in the repo and commit the regenerated `.fledge/scaffold.json` so the dogfood stamp
   tracks the new version. Cross-link it from the README Upgrading section.

Keep it factual and current — verify the exact set of files a version bump touches against
the repo before writing them down, rather than guessing. Scope is documentation only: no
code or workflow changes.

## Tests
Docs are verified with a small structural test, written test-first (consistent with the
repo's existing structural tests for CI YAML under `internal/ciconfig`):
- `TestReadmeDocumentsUpdateCommand` — pins FC-5: asserts `README.md` references
  `fledge update` in the Commands section and that the Upgrading section mentions binary
  self-update. Fails before the edit.
- `TestReleasingDocCoversScaffoldRefresh` — pins FC-6: asserts a `RELEASING.md` exists at
  repo root and contains the `fledge init --refresh` scaffold-stamp step. Fails before the
  file is created.

Place these in a small doc-structural test (e.g. `internal/doctest/docs_test.go`),
reading the files from the repo root; keep assertions minimal (substring/section checks),
not full-content snapshots, so ordinary prose edits don't break them.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `README.md`'s Commands table includes a `fledge update` row and the Upgrading section covers binary self-update (FC-5).
- [x] AC-3: `RELEASING.md` exists at the repo root and documents the version-bump steps plus the `fledge init --refresh` + commit-stamp requirement (FC-6).
- [x] AC-4: `fledge preen` passes and `go test ./...` is green.
