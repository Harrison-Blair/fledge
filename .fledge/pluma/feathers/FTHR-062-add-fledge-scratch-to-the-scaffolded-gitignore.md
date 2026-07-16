---
id: FTHR-062
title: Add .fledge/scratch/ to the scaffolded gitignore
plumage: PLM-028
status: fledged
priority: P2
depends_on: []
authored: 2026-07-16T16:24:56Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-062: Add .fledge/scratch/ to the scaffolded gitignore

## Description
Add `.fledge/scratch/` (the new gitignored directory for scratchpad and digest files) to fledge's scaffolded `.gitignore` output, following the exact existing pattern for `.fledge/roster/` (satisfies PLM-028 FC-1). Root feather for this plumage — independent of the PLM-027 doc split, pure Go CLI change.

## Affected Modules
Per `.fledge/nest/modules.md` (internal/cli) and `.fledge/nest/data-model.md`:
- `internal/cli/init.go` — `gitignoreLines` (line 30): `var gitignoreLines = []string{".fledge/nest/raw/", ".fledge/broods/", ".fledge/roster/", ".alloc.lock"}`
- `cmd/fledge/testdata/init.txtar` — line 28 asserts `grep '.fledge/roster/' .gitignore`; add a parallel assertion for `.fledge/scratch/`.

## Approach
Add `".fledge/scratch/"` to the `gitignoreLines` slice in `internal/cli/init.go:30`, in the same position/style as the existing `.fledge/roster/` entry (order within the slice doesn't matter functionally — append it, matching the existing trailing-slash directory-entry convention). No other code path needs to change: `gitignoreLines` is already consumed uniformly by both the write path (line ~398) and the `--refresh`/drift-check path (line ~471).

## Tests
- Extend `cmd/fledge/testdata/init.txtar`: after the existing `grep '.fledge/roster/' .gitignore` line, add `grep '.fledge/scratch/' .gitignore`.
- Implementation order: add the new `grep` line to `init.txtar` first and run `go test ./cmd/fledge -run TestScripts/init` against the unchanged `gitignoreLines` — it fails (the pattern isn't in `.gitignore` yet) — then add the entry to `gitignoreLines` and confirm the test passes.

## Acceptance Criteria
- [x] AC-1: The test listed above was observed failing before implementation and passes after.
- [x] AC-2: `fledge init` on a fresh repo writes a `.gitignore` containing `.fledge/scratch/` (satisfies PLM-028 FC-1, AC-1).
- [x] AC-3: `go test ./cmd/fledge -run TestScripts/init` passes.
