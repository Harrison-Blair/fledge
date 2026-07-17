---
id: FTHR-087
title: Remove gitkeep placeholders from init and this repository
plumage: PLM-033
status: egg
priority: P3
depends_on: [FTHR-085]
authored: 2026-07-17T03:32:48Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-087: Remove gitkeep placeholders from init and this repository

## Description

Stops `fledge init` writing the four `.gitkeep` placeholder files, and removes the three
tracked ones from this repository. The whole of PLM-033 in one feather.

The four placeholders (`nest/raw/`, `broods/`, `pluma/plumage/`, `pluma/feathers/`) exist so
git can carry an empty directory. Nothing depends on them: every consumer creates its
directory on demand — `lock.go:40` before writing a claim, `nest.go:186` before a raw
report, `ids.go:72,93` before a spec — and `specFiles` (`load.go:63-67`) treats a missing
directory as an empty set rather than an error.

**`broods/.gitkeep` was doing real work and is being removed knowingly.** `.fledge/broods/`
is gitignored while that placeholder is tracked — the standard idiom for shipping an empty
directory whose contents are ignored. Removing it means a fresh clone has no
`.fledge/broods/` until the first claim creates it. The user was shown this distinction
explicitly and chose to remove all four anyway. Do not "restore" it as a fix.

## Why this depends on FTHR-085

**The `depends_on: [FTHR-085]` edge is not a logical dependency — it is deliberate
serialization, and it must not be optimized away.**

Nothing in this feather needs anything FTHR-085 produces. The edge exists because **three
feathers rewrite `.fledge/scaffold.json`**: FTHR-083 (adding `roost` to `commandOrder`
regenerates the per-adapter allow-lists via `ExpectedFiles`, `stamp.go:144`), FTHR-085
(core prose changes the hashes of every file copied from it), and this one (init's file
list changes which files the stamp records). Scaffold-refresh feathers dispatched
concurrently all rewrite that shared stamp and collide at merge — a known sharp edge in this
repository.

FTHR-083 → FTHR-085 → FTHR-087 is therefore a chain, so the three never run at once. If a
future reader removes this edge to "parallelize", the collision returns. This paragraph is
the reason it is here.

Satisfies PLM-033 FC-1..FC-6.

## Affected Modules

See `.fledge/nest/modules.md` → `internal/cli`, `internal/bootstrap`;
`.fledge/nest/architecture.md` (scaffold stamp, drift statuses, `obsolete` and pruning);
`.fledge/nest/testing.md` (`init.txtar`, `stamp_warning.txtar`).

- `internal/cli/init.go` — **both** code paths: `init.go:233-237` (base init) and
  `init.go:485-489` (refresh). Four entries each.
- `.fledge/broods/.gitkeep`, `.fledge/pluma/plumage/.gitkeep`,
  `.fledge/pluma/feathers/.gitkeep` — deleted from this repo (`git rm`). The fourth,
  `.fledge/nest/raw/.gitkeep`, is untracked; delete it from disk.
- `cmd/fledge/testdata/init.txtar` — asserts the `created …/.gitkeep` lines
  (`init.txtar:5-9`). **Must** be updated here; unavoidable shared-fixture edit, which the
  serialization above also covers.
- `cmd/fledge/testdata/stamp_warning.txtar` — uses `.gitkeep` paths as fixtures
  (`stamp_warning.txtar:56-57`).
- `.gitignore` — **read only, unchanged** (FC-6).

## Approach

**Remove, don't special-case.** Delete the four entries from both `baseFiles` lists in
`init.go`. No flag, no compatibility shim — they simply stop being written.

**Pruning is the mechanism, not extra work (FC-2).** Once the four leave the manifest,
`ExpectedFiles` no longer lists them, so an existing repo's stamp marks them `obsolete`
(`drift.go:14`) and `fledge init --refresh` prunes them. Refresh already does this — verify
it rather than writing new removal code.

**Consuming repositories are out of scope.** `hearth` and `stenographer` lose their
placeholders on their next refresh, through that same ordinary pruning. Do not reach into
their histories from this feather (PLM-033 Out of Scope).

**`.gitignore` stays untouched (FC-6).** The rules excluding `.fledge/nest/raw/` and
`.fledge/broods/` remain correct — they exclude those directories' *contents*, which is
still wanted. Only the placeholders go. AC-8 pins this byte-for-byte, because "cleaning up
gitignore while I'm here" is the obvious way to break it.

**Scaffold stamp.** Run `fledge init --refresh` and commit the resulting
`.fledge/scaffold.json`. This is the **third and last** of the serialized refresh feathers.

## Tests

Acceptance tests via txtar (the established home for init/scaffold behavior per
`.fledge/nest/testing.md`), plus one unit test for the loader contract.

Run: `go test ./cmd/fledge -run TestScripts/init`,
`go test ./cmd/fledge -run TestScripts/gitkeep_removal`, `go test ./internal/spec`, then
`go test ./...`.

**`cmd/fledge/testdata/init.txtar`** (existing fixture — update)
- *init writes no placeholder* — the `created …/.gitkeep` assertions
  (`init.txtar:5-9`) are removed, and the script asserts **no** `.gitkeep` exists anywhere
  under the repo afterwards. The negative assertion is the point: deleting the old lines
  alone would let a regression pass silently → AC-2, PLM-033 AC-1.

**`cmd/fledge/testdata/gitkeep_removal.txtar`** (new file)
- *refresh prunes placeholders written by a previous version* — fixture repo with all four
  present and recorded in its stamp; after `fledge init --refresh`, none remains → AC-3,
  PLM-033 AC-2.
- *claim works with no broods directory* — `fledge brood FTHR-###` in a repo without
  `.fledge/broods/` succeeds and creates it. The `broods/.gitkeep` removal is the riskiest
  of the four; this is its safety net → AC-4, PLM-033 AC-3.
- *scout report works with no raw directory* → AC-4, PLM-033 AC-4.
- *spec creation works with no spec directories* — `fledge new plumage` and
  `fledge new feather` succeed, creating the directories, and allocate `PLM-001`/`FTHR-001`
  → AC-5, PLM-033 AC-5.
- *spec-loading commands succeed with no spec directories* — `preen`, `ready`, `unfledged`,
  `vee`, `colony` each report an empty set and exit 0 → AC-6, PLM-033 AC-6.
- *`.gitignore` is unchanged* — byte-identical after init and after refresh → AC-8,
  PLM-033 AC-8.

**`internal/spec/load_test.go`** (existing file — add one case)
- *missing spec directory yields an empty set* — pins `Load`'s contract that FC-5 depends
  on. Note FTHR-082 adds a case of the same name for the walk rewrite; if both land, keep
  one. → AC-6

Test-first order is fixed: write these, run them against unchanged code and observe them
FAIL for the expected reason (placeholders still written; `init.txtar`'s negative assertion
failing), then implement until they pass.

## Acceptance Criteria

- [ ] AC-1: The tests listed above were observed failing before implementation and pass
      after.
- [ ] AC-2: Initializing a fresh repository produces no `.gitkeep` file anywhere under it.
      Satisfies PLM-033 FC-1, AC-1.
- [ ] AC-3: Refreshing a repository containing placeholders written by a previous version
      leaves none of them present. Satisfies PLM-033 FC-2, AC-2.
- [ ] AC-4: With no claim directory and no raw report directory present, recording a claim
      and writing a scout report each succeed and create their directory. Satisfies
      PLM-033 FC-4, AC-3, AC-4.
- [ ] AC-5: With no spec directories present, creating a plumage and a feather succeeds,
      creates the directories, and allocates the first IDs of each sequence. Satisfies
      PLM-033 FC-4, AC-5.
- [ ] AC-6: With no spec directories present, every command that loads the spec set reports
      an empty set and exits 0. Satisfies PLM-033 FC-5, AC-6.
- [ ] AC-7: The three tracked placeholders are removed from this repository, no `.gitkeep`
      remains anywhere in it, `go test ./...` passes and `fledge preen` reports the scaffold
      healthy. Satisfies PLM-033 FC-3, AC-7.
- [ ] AC-8: `.gitignore` is byte-identical to its prior state. Satisfies PLM-033 FC-6, AC-8.
- [ ] AC-9: `fledge init --refresh` has been run and the resulting `.fledge/scaffold.json`
      committed.
