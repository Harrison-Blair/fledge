---
id: FTHR-082
title: Location-independent spec discovery and ID allocation
plumage: PLM-032
status: pipping
priority: P2
depends_on: []
authored: 2026-07-17T03:30:40Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-082: Location-independent spec discovery and ID allocation

## Description

The tracer bullet for PLM-032, and the feather that makes every later one safe.

Two functions in `internal/spec` read exactly one directory level and ignore what is below
it. `specFiles` (`load.go:63`) skips any entry where `e.IsDir()`. `NextID` (`ids.go:17`)
allocates the next ID by regex-matching **filenames** from a single `os.ReadDir`. Neither
descends.

Relocating a completed spec under the current code therefore does two things, one merely
bad and one catastrophic:

- The spec becomes invisible to every command that loads the spec set — `preen`, `vee`,
  `colony`, `ready`, `unfledged`, `status`, `criteria`, and the `--plumage`/`--depends-on`
  existence checks in `new`.
- **`NextID` reissues IDs that already belong to merged historical specs.** With this
  repository's 73 fledged feathers relocated, the next `fledge new feather` returns
  `FTHR-006` — silently colliding with real, closed work.

This feather makes location presentational: both functions see the whole tree, so a spec
file works identically wherever it sits. It ships **no relocation** and no new command —
FTHR-083 does that, on top of this. That ordering is deliberate: the ID hazard must be dead
before any code exists that can move a file.

It is nonetheless a real, verifiable end-to-end slice. After this feather alone, a
hand-created `fledged/` subdirectory with a `git mv`'d spec inside it is fully functional:
every command reports it, and allocation still counts it.

Satisfies PLM-032 FC-7, FC-8; enables FC-1 and everything downstream.

## Affected Modules

See `.fledge/nest/modules.md` → `internal/spec`, `internal/cli`;
`.fledge/nest/data-model.md` (spec directory layout, `Set`, per-directory `.alloc.lock`);
`.fledge/nest/conventions.md` (ID allocation rules).

- `internal/spec/load.go` — `specFiles` (`load.go:63`), called by `Load` (`load.go:27`) for
  both the plumage and feather directories.
- `internal/spec/ids.go` — `NextID` (`ids.go:17`); `AllocateAndCreate` (`ids.go:62`) and
  `lockAllocDir` (`ids.go:88`) are **read-only context** here and must not change.
- `internal/cli/specload.go` — **read only.** The shared loader every spec-reading command
  funnels through. Deliberately untouched: see Approach.

## Approach

**Why no command changes.** All 11 spec-reading commands (`brood`, `colony`, `criteria`,
`new`, `preen`, `ready`, `set`, `status`, `unfledged`, `vee`, and `preen`'s scaffold path)
load through `internal/cli/specload.go`, which calls `spec.Load(reqDir, taskDir)`. Fixing
discovery inside `spec.Load` therefore fixes every command at once, with no call-site edits.
This is what keeps the tracer thin and the later feathers parallel-safe — do not push
location awareness up into the CLI layer.

**`specFiles` → recursive.** Replace the flat `os.ReadDir` with a walk (`filepath.WalkDir`)
rooted at `dir`, collecting `.md` files at any depth. Preserve three existing properties:

- **Missing dir yields nil, not an error** (`load.go:64-67`). `Load`'s contract is that a
  missing directory is an empty set; `WalkDir` surfaces the root's `ENOENT` through the
  callback, so swallow it exactly as today.
- **Deterministic order.** `Set`'s slices are documented as "in filename order"
  (`load.go:17`); `WalkDir` walks lexically, which preserves that for a flat directory and
  extends it sensibly. Do not sort differently.
- **Skip the dotfiles.** `.alloc.lock` is already excluded by the `.md` suffix filter
  (`ids.go:47` documents this dependency explicitly — the lock is a dotfile with no `.md`
  suffix precisely so this scan ignores it). Keep the suffix filter; do not replace it with
  a name check that would break that contract.

**`NextID` → recursive.** Same change of shape: walk instead of `ReadDir`, match the regex
against **base names** (`filepath.Base`), not full paths — the pattern is anchored at `^`
and a path prefix would break it. The `os.IsNotExist` early return (`ids.go:20-22`) must
survive: a missing directory still allocates `-001`.

**Do not touch allocation locking.** `AllocateAndCreate` (`ids.go:62`) and its `.alloc.lock`
flock are correct as-is and are covered by an existing 20-goroutine × 5-round concurrency
test. `NextID` is called *inside* the lock; making it walk more files lengthens the critical
section slightly but changes no semantics. Resist any refactor here — the concurrency
guarantee is the reason ID allocation is trustworthy.

**Constraint: no behavior change for a flat repository.** Every existing fixture must pass
untouched. A repo with no subdirectories must produce byte-identical output from every
command. That is what AC-6 pins.

## Tests

Unit tests beside their package (`internal/spec`), plus one acceptance-level txtar proving
the end-to-end result. This matches `.fledge/nest/testing.md`: `internal/spec` already has
`load_test.go` and `ids_test.go` with exactly this shape.

Run: `go test ./internal/spec`, `go test ./cmd/fledge -run TestScripts/roost_discovery`,
then `go test ./...`.

**`internal/spec/load_test.go`**
- *specs in a subdirectory are loaded* — a plumage and a feather placed in `<dir>/fledged/`
  are both present in the returned `Set`, with `Path` reflecting the nested location →
  AC-2.
- *nested and flat specs load together* — a set split across `<dir>/` and `<dir>/fledged/`
  returns all of them, in deterministic order → AC-2.
- *missing directory still yields an empty set, not an error* — pins the `Load` contract
  through the walk rewrite → AC-5.
- *`.alloc.lock` is not loaded as a spec* — the lock file at any depth is ignored → AC-5.

**`internal/spec/ids_test.go`**
- *`NextID` counts specs in subdirectories* — the highest ID lives in `<dir>/fledged/`;
  allocation returns highest+1 and **does not reissue it**. This is the ID-reuse regression
  and the single most important test in the feather → AC-3.
- *`NextID` on a missing directory allocates the first ID* — pins the `os.IsNotExist` path
  → AC-3.
- *`NextID` matches base names, not paths* — a nested file's ID is parsed correctly (guards
  the `^`-anchored regex against a path prefix) → AC-3.
- *concurrent allocation with nested specs stays unique* — extends the existing
  20-goroutine × 5-round flock test to a tree containing a `fledged/` subdirectory; every
  allocated ID distinct, none colliding with a nested spec → AC-3, AC-4.

**`cmd/fledge/testdata/roost_discovery.txtar`** (new file — deliberately not an edit to an
existing fixture, so this feather stays merge-disjoint from the others)
- *a spec relocated by hand is fully functional* — fixture repo with a fledged plumage and
  its feathers under `fledged/` subdirectories and live specs at top level. Assert
  `fledge vee`, `fledge unfledged`, `fledge colony`, and `fledge preen` all report the
  nested specs exactly as if flat, and `fledge new feather --plumage <nested PLM>` resolves
  the nested parent and allocates a non-colliding ID → AC-2, AC-3, AC-6.

Test-first order is fixed: write these, run them against unchanged code and observe them
FAIL for the expected reason (nested specs invisible; `NextID` returning an already-used
ID), then implement until they pass.

## Acceptance Criteria

- [ ] AC-1: The tests listed above were observed failing before implementation and pass
      after.
- [ ] AC-2: A spec file in a subdirectory of its spec directory is loaded, and every command
      that loads the spec set reports it identically to a flat one, differing only in the
      path reported. Satisfies PLM-032 FC-7, AC-2.
- [ ] AC-3: ID allocation accounts for specs at any depth. With the highest existing ID held
      by a spec in a subdirectory, the next allocated ID follows it and collides with
      nothing. Satisfies PLM-032 FC-8, AC-3.
- [ ] AC-4: Concurrent allocation remains collision-free with nested specs present — the
      existing flock guarantee is preserved, not weakened.
- [ ] AC-5: `Load`'s existing contracts survive the rewrite: a missing directory yields an
      empty set rather than an error, and `.alloc.lock` is never loaded as a spec.
- [ ] AC-6: A repository with no subdirectories behaves byte-identically to before, and
      `go test ./...` passes with every existing fixture unmodified.
