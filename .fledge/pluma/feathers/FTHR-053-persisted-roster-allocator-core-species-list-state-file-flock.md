---
id: FTHR-053
title: "Persisted roster allocator core (species list, state file, flock)"
plumage: PLM-026
status: pipping
priority: P3
depends_on: []
authored: 2026-07-16T02:06:31Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-053: Persisted roster allocator core (species list, state file, flock)

## Description
Worker species allocation (the `<role>-<species>` naming scheme) is currently
tracked only in the orchestrator's context, with no canonical list of the 18
species anywhere in the repo and no persisted state. This feather lays the
foundation: a new `internal/roster` package holding the canonical species
list (confirmed with the user: adelie, emperor, gentoo, king, chinstrap,
little, african, humboldt, magellanic, galapagos, yelloweyed, fiordland,
snares, erectcrested, rockhopper, royal, macaroni, northernrockhopper) and a
flock-guarded state file recording current assignments, mirroring
`internal/spec/ids.go`'s `.alloc.lock` pattern. This is the tracer-bullet
foundation other feathers in this plumage build the CLI surface on top of.

## Affected Modules
- New package `internal/roster/roster.go` (see `.fledge/nest/modules.md` for
  the sibling pattern in `internal/spec/ids.go`'s `AllocateAndCreate` /
  `lockAllocDir`, and `internal/lock/lock.go`'s `Record`/`Acquire`/`Release`/
  `List` for the persisted-record pattern this mirrors).

## Approach
Define `var Species = [18]string{"adelie", "emperor", "gentoo", "king",
"chinstrap", "little", "african", "humboldt", "magellanic", "galapagos",
"yelloweyed", "fiordland", "snares", "erectcrested", "rockhopper", "royal",
"macaroni", "northernrockhopper"}` (exact order as confirmed) as the single
source of truth. Define an `Entry` type: `{Species string, Members []string,
Feather string}` where `Members` holds the assigned name(s) sharing that
species (one for a solo worker, two for a pair) and each member tracks its
own released state internally (e.g. `Entry.Released map[string]bool` or a
parallel slice) so a species frees only once every member is released.
Persist entries as JSON in a state file under `.fledge/broods/roster.json`
(or a dedicated `.fledge/roster/` dir — pick whichever keeps `broods/`'s
existing scope of "lock records" clean; a separate file is simpler and
matches this plumage's decision that roster state doesn't overload the lock
file). Guard all read-modify-write operations with an exclusive flock on a
sibling `.roster.lock` file, following `internal/spec/ids.go`'s
`lockAllocDir` pattern exactly (open-or-create, `syscall.Flock(LOCK_EX)`,
defer unlock).

Core functions (consumed by FTHR-054's CLI layer, not directly exposed as
commands themselves):
- `Assign(dir, feather string, pair bool) (names []string, err error)` —
  under the flock, scans existing entries for the first unused species (none
  of its members currently unreleased); if all 18 have at least one
  unreleased member, picks the first species whose lowest numeric suffix
  (`-2`, `-3`, ...) is unused. Builds member name(s) as `<role>-<species>`
  is the CALLER's concern (this package returns bare species/suffix tokens
  or full names — decide during implementation whichever keeps role-name
  composition at the CLI layer where the `--pair`/role information lives;
  document the chosen boundary in code comments).
- `Release(dir, name string) error` — marks that member released; if every
  member of its entry is now released, the entry is removed (species
  available again).
- `List(dir) ([]Entry, error)` — returns all live (non-fully-released)
  entries.

## Tests
- Unit tests in `internal/roster` (new `roster_test.go`):
  - `Assign` returns the first unused species; a second `Assign` call
    returns the next unused one; once all 18 are in use, `Assign` returns a
    numeric-suffixed variant of the first species (e.g. `adelie-2`).
  - `Release` on one member of a pair does not free the species; releasing
    the second member frees it (confirmed by a subsequent `Assign` reusing
    it).
  - `List` returns only live entries, omitting fully-released ones.
  - A concurrency test: N goroutines call `Assign` simultaneously against
    the same state dir; assert no two calls ever receive the same species
    (mirrors `internal/spec`'s existing `AllocateAndCreate` race test
    pattern).
Written first against a nonexistent package (compile failure) and confirmed
to fail for that reason, then implemented until all tests pass (satisfies
PLM-026 FC-4, AC-4).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-053.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-026 FC-2"). AC-1 is always:
- [ ] AC-1: The tests listed above were observed failing before implementation
  and pass after; evidence captured verbatim.
- [ ] AC-2: `internal/roster.Species` holds the confirmed 18-species list in
  the confirmed order.
- [ ] AC-3: `Assign`/`Release`/`List` behave as specified, including
  numeric-suffix overflow past 18 and per-member release tracking (satisfies
  PLM-026 FC-1, FC-2, FC-3 at the package level — CLI exposure is FTHR-054).
- [ ] AC-4: The concurrency test demonstrates no double-allocation under
  simultaneous `Assign` calls (satisfies PLM-026 FC-4, AC-4).
- [ ] AC-5: `go test ./internal/roster/...` passes.
