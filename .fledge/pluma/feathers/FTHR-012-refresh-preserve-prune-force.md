---
id: FTHR-012
title: "Refresh preserve/prune + --force"
plumage: PLM-009
status: fledged
priority: P1
depends_on: [FTHR-009]
authored: 2026-07-10T15:01:02Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# FTHR-012: Refresh preserve/prune + --force

## Description
Makes `fledge init --refresh` safe and self-healing. Preserve: refresh no
longer clobbers user-edited default-policy/core files — a file is rewritten
only when its disk hash matches the old stamp's hash (provably unedited);
edited files are kept and reported, and a new `--force` flag restores the old
overwrite behavior. Prune: files present in the old stamp but absent from the
new expected tree are deleted only when disk content still matches the
recorded hash (or a symlink still points at its recorded target); everything
else is reported, never deleted; paths absent from the stamp are never
touched. A stampless refresh is the adoption path: preserve everything, prune
nothing, write a fresh stamp.

## Affected Modules
- **`internal/bootstrap/registry.go`** — thread `WriteOpts{Refresh, Force
  bool; Old *Stamp}` through `WriteCore`/`WriteAdapter`/`writeFileEntry`
  (replacing the bare `refresh bool`); implement the preserve decision for
  default/core policies; new `preserved` outcome bucket. `generate`,
  `primitive_map`, `overwrite`, and `symlink` policies remain always-managed.
- **`internal/cli/init.go`** — `--force` flag; prune pass after writes
  (delete provably-owned unmodified obsolete paths, then remove now-empty
  parent dirs under `.fledge/skills/` and `.claude/`); report lines
  `kept <path> (user-edited; use --force)`, `removed <path>`,
  `obsolete <path> (user-edited — remove manually)`; update the `--refresh`
  help text and the refresh note (init.go ~150) for the semantics change.
- **`cmd/fledge/testdata/refresh_scaffold.txtar`** (new) — dedicated fixture;
  disjoint from FTHR-010/011 fixtures.

## Approach
- Preserve decision (refresh, default/core policy, file exists, disk ≠ new
  shipped bytes): disk hash == old stamp hash → rewrite; otherwise (edited or
  no stamp entry) → keep + report; `--force` → rewrite regardless.
- Prune set = old stamp paths − new expected paths; the stamp itself is
  implicitly owned. Deletion is conservative by construction — hash or
  symlink-target proof required.
- The new stamp written at the end reflects only the new expected tree
  (obsolete-but-kept files leave the manifest; preen then reports them as
  unowned leftovers if fledge-shaped — documented behavior).
- Docs touched here are the CLI-facing strings only; CLAUDE.md/README churn
  belongs to FTHR-013.

## Tests
Written test-first: (1) write; (2) observe FAIL against unchanged code for the
expected reason; (3) implement until green.
- **`internal/bootstrap/registry_test.go`** — preserve decision table
  (unedited→rewritten, edited→kept, no-stamp-entry→kept, force→rewritten);
  prune decision table (hash match→delete, mismatch→keep, missing→keep,
  symlink repointed→keep, path not in stamp→never considered).
- **`cmd/fledge/testdata/refresh_scaffold.txtar`**
  - edit a default-policy file → `--refresh` keeps it + reports `kept`;
    `--refresh --force` overwrites it.
  - inject a fake stamp entry whose on-disk file matches its hash → refresh
    prunes it (`removed`); with mismatched content → kept + `obsolete` report.
  - delete a core file → refresh recreates it.
  - `rm .fledge/scaffold.json` then `--refresh` → adopts: nothing pruned,
    edited files kept, fresh stamp written.
- Whole `go test ./...` + `go vet ./...` green.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: Refresh rewrites provably-unedited files, keeps and reports user-edited ones, and `--force` restores overwrite behavior; always-managed policies are unaffected (satisfies PLM-009 FC-4).
- [x] AC-3: Refresh prunes only stamp-proven, content-matching obsolete files (or symlinks at their recorded target), reports everything else without deleting, never touches paths outside the stamp, and the stampless path adopts without pruning (satisfies PLM-009 FC-5).
- [x] AC-4: `--refresh` help text and the init refresh note describe the new preserve/prune semantics; `go test ./...` and `go vet ./...` pass.
