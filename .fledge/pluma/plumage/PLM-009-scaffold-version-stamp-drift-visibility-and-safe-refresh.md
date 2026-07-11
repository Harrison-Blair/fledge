---
id: PLM-009
title: "Scaffold version stamp, drift visibility, and safe refresh"
status: fledged
priority: P1
authored: 2026-07-10T14:54:16Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# PLM-009: Scaffold version stamp, drift visibility, and safe refresh

## Context
fledge writes no version information into repos it scaffolds and has no drift
detection. The version exists only in the binary (`internal/cli/version.go`,
ldflags from `VERSION`); a scaffolded repo carries nothing to compare against.
Real-world consequence (observed in a sibling fledge-managed repo): skills
content silently drifted from the current embed on 6 files, nest docs stamped
`fledge_version: unknown`, and legacy directories from older layouts
(`.fledge/locks/`, `.fledge/burrows/`) survive every `init --refresh` because
refresh only adds/updates and never removes or reports — re-init feels
"spotty". The user cannot tell at a glance which fledge version scaffolded a
repo, gets no feedback when it is behind the installed binary, and has no safe
way to bring a repo current. Decisions were settled in the 2026-07-10
interrogation: warning-only (no automatic sync, no global symlinks — vendored
copies stay; updating the binary plus one explicit per-repo
`fledge init --refresh` is the "global update" story).

## User Stories
- As a fledge user with several fledge-managed repos, I want every fledge
  command to tell me when a repo's scaffold is behind my installed binary, so
  that staleness is impossible to miss without me remembering to check.
- As a fledge user, I want a git-tracked stamp recording which fledge version
  scaffolded the repo and exactly which files fledge wrote, so that I (and
  collaborators) can answer "what version is this repo on?" at a glance.
- As a fledge user running `fledge preen`, I want a per-file scaffold drift
  report (modified / missing / stale / obsolete), so that "spotty re-init" is
  diagnosable instead of mysterious.
- As a fledge user running `fledge init --refresh`, I want obsolete
  fledge-owned files pruned when provably unmodified and my hand-edited files
  preserved and reported (with a `--force` escape hatch), so that refresh is
  safe, self-healing, and never destroys my work.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: `fledge init` (and `--refresh`) writes a deterministic, git-tracked
   stamp file `.fledge/scaffold.json` recording the fledge binary version, the
   scaffolded agents/harnesses, and a manifest of every file fledge wrote:
   path → write policy plus sha256 of the rendered bytes as shipped (symlink
   entries record their target; append entries record their ensured lines).
   The stamp excludes itself and is byte-idempotent across repeated runs.
2. FC-2: Every fledge command except `init` and `version`, when run in a repo
   whose stamp version differs from the binary version, prints a one-line
   warning to stderr naming both versions and suggesting
   `fledge init --refresh`. stdout is untouched (`--json` output stays valid).
   A repo with no stamp produces no per-command warning.
3. FC-3: `fledge preen` reports scaffold drift: per-file status vs the stamp
   and the embedded tree — up-to-date, stale (unedited but embed is newer),
   modified (user-edited), missing, obsolete (in stamp, no longer shipped) —
   as warning-severity findings in human output and a structured `scaffold`
   object in `--json`. A repo with no stamp yields a single warning finding
   directing the user to adopt via `fledge init --refresh`.
4. FC-4: `fledge init --refresh` preserves user-edited default-policy/core
   files (disk hash ≠ old stamp hash), reporting them as kept; `--force`
   restores overwrite behavior. Always-managed policies (generate,
   primitive_map, overwrite, symlink) remain always-managed.
5. FC-5: `fledge init --refresh` prunes obsolete files only when the old stamp
   proves fledge wrote them and the disk content still matches the recorded
   hash (or a symlink still points at the recorded target); everything else is
   reported, never deleted. Paths absent from the stamp are never touched.
   A no-stamp refresh adopts: prunes nothing, preserves existing files, writes
   a fresh stamp.
6. FC-6: The release version is bumped (VERSION and `binaryVersion` to 0.3.0)
   and this repository is re-initialized so it carries its own stamp.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [x] AC-1: A fresh `fledge init` produces `.fledge/scaffold.json` with the binary version and a manifest entry (policy + hash/target/lines) for every scaffolded file, and a second run changes zero bytes.
- [x] AC-2: With a stamp version older than the binary, every command except `init` and `version` emits the one-line stderr mismatch warning while `--json` stdout remains parseable; with a matching or absent stamp, commands are silent.
- [x] AC-3: `fledge preen` (human and `--json`) correctly classifies seeded up-to-date, stale, modified, missing, and obsolete scaffold files, and reports the no-stamp adoption warning when the stamp is absent.
- [x] AC-4: `fledge init --refresh` updates unedited files, keeps and reports user-edited files (overwriting them only with `--force`), prunes provably-owned unmodified obsolete files, reports — without deleting — everything else, and never touches paths outside the stamp manifest.
- [x] AC-5: Automated tests cover AC-1..AC-4 (unit tests for stamp/drift/prune decisions; txtar acceptance tests including the no-stamp adoption path), each observed failing before its implementation, and the full suite passes.
- [x] AC-6: VERSION and `binaryVersion` read 0.3.0, and this repo's own `fledge init --refresh` produces a clean stamp with no pruning and no kept-as-edited reports.

## Out of Scope
- Automatic refresh / self-healing writes on version mismatch (explicitly
  deferred by the user — warning only).
- Global symlink mode (`.fledge/skills` pointing at a machine-global share).
- Pruning or migrating pre-stamp legacy directories in existing repos
  (`.fledge/locks/`, `.fledge/burrows/`, `.fledge/molt/` archives) beyond
  reporting them as unowned.
- Windows-native symlink support (existing limitation; drift check must
  degrade gracefully, not crash).
- A dedicated `fledge doctor` command (drift reporting lives in `preen`).

## Open Questions
None — all decisions resolved during the 2026-07-10 interrogation.
