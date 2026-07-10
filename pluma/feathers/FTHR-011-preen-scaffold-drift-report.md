---
id: FTHR-011
title: Preen scaffold-drift report
plumage: PLM-009
status: hatching
priority: P1
depends_on: [FTHR-009]
authored: 2026-07-10T15:00:04Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# FTHR-011: Preen scaffold-drift report

## Description
The deep drift diagnosis: `fledge preen` gains a scaffold section comparing the
stamp, the disk, and the embedded tree, classifying every manifest path as
up-to-date, stale (disk matches the stamp hash but the embed has moved — a
refresh will cleanly update it), modified (user-edited), missing, or obsolete
(in the stamp, no longer shipped), with symlink-target and append-line checks.
Findings are warning severity (exit code changes only under `--strict`);
`--json` gains a structured `scaffold` object. A repo with no stamp yields one
warning finding: "no scaffold stamp — run fledge init --refresh to adopt".
This is what makes "spotty re-init" diagnosable.

## Affected Modules
- **`internal/bootstrap`** (new `drift.go`) — `DriftReport(root, stamp,
  expected) []Drift` consuming FTHR-009's `LoadStamp` + `ExpectedFiles`. Drift
  logic lives here so `internal/check` keeps zero bootstrap dependency. See
  `.fledge/nest/architecture.md` → layer separation.
- **`internal/cli/preen.go`** — after `check.Run`: assemble the expected tree
  (same helper init uses), compute drift, convert non-up-to-date entries into
  `check.Finding{Rule: "scaffold-drift", Severity: Warning}` with actionable
  messages; human output adds a one-line summary (counts + stamp/binary
  versions); `--json` adds `"scaffold": {stampVersion, binaryVersion,
  files: [{path, status, policy}]}` including up-to-date entries.
- **`cmd/fledge/testdata/preen_scaffold.txtar`** (new) — dedicated fixture;
  does not touch FTHR-010/012's fixtures.

## Approach
- Statuses: `up-to-date` (disk == expected bytes), `stale` (disk == stamp hash
  ≠ expected — provably unedited, refresh-safe), `modified` (disk ≠ both),
  `missing`, `obsolete` (stamp-only). Symlinks: compare `os.Readlink` to the
  recorded target; a non-symlink where one is expected reports `modified`,
  never errors (Windows degradation). Append entries: line absent → `missing`
  semantics on that entry.
- Message per status embeds the remedy (`run fledge init --refresh`; for
  modified: `user-edited; refresh will preserve it`).
- No writes anywhere — preen stays read-only.

## Tests
Written test-first: (1) write; (2) observe FAIL against unchanged code for the
expected reason; (3) implement until green.
- **`internal/bootstrap/drift_test.go`** — table test over a temp tree seeding
  each status (untouched / edited / deleted / stamp-only entry / repointed
  symlink / removed append line) and asserting the classification; no-stamp →
  distinct sentinel.
- **`cmd/fledge/testdata/preen_scaffold.txtar`**
  - fresh init → preen reports scaffold healthy (no scaffold-drift findings;
    `--json` scaffold.files all up-to-date).
  - edit a default-policy file → `modified` finding, exit 0; `--strict` exit 1.
  - delete a core file → `missing`; add a fake manifest entry with a matching
    on-disk file → `obsolete`.
  - `rm .fledge/scaffold.json` → single "no scaffold stamp" warning finding.
  - `--json` shape assertions for the scaffold object.
- Whole `go test ./...` + `go vet ./...` green.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after.
- [ ] AC-2: `fledge preen` classifies up-to-date, stale, modified, missing, and obsolete scaffold entries correctly in human output and in the `--json` scaffold object, at warning severity (satisfies PLM-009 FC-3).
- [ ] AC-3: A stampless repo produces exactly one adoption warning finding; symlink and append entries are checked without error on any platform.
- [ ] AC-4: `internal/check` gains no bootstrap import; `go test ./...` and `go vet ./...` pass.
