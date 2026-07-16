---
id: FTHR-054
title: fledge roster assign/release/list commands
plumage: PLM-026
status: egg
priority: P3
depends_on: [FTHR-053]
authored: 2026-07-16T02:06:34Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-054: fledge roster assign/release/list commands

## Description
With `internal/roster`'s allocator core in place (FTHR-053), this feather
exposes it as `fledge roster`, a new multi-subcommand CLI command (following
the `fledge nest new|scaffold|scout|status` dispatch pattern in
`internal/cli/nest.go`): `fledge roster assign --feather FTHR-### [--pair]`,
`fledge roster release <name>`, and `fledge roster [--json]` (list, the
no-subcommand default).

## Affected Modules
- New `internal/cli/roster.go` — `register("roster", runRoster, ...)` plus
  subcommand dispatch, following `internal/cli/nest.go`'s `runNest` pattern
  (see `.fledge/nest/modules.md` → internal/cli).
- `internal/roster` (FTHR-053) — the package this command wraps.

## Approach
`runRoster(args []string) int` dispatches on the first positional argument:
`assign`, `release`, or absent (list). Each subcommand builds its own
`flag.FlagSet` (mirroring `nest.go`'s per-subcommand pattern) and calls into
`internal/roster`'s core functions against `r.LocksDir()`-adjacent or a new
`r.RosterDir()`-style path (repo package, per FTHR-053's Approach).

- `roster assign --feather FTHR-### [--pair]`: calls `roster.Assign`; role
  names are composed here (`--pair` means two names sharing the returned
  species, one per role — the caller conventionally supplies role context via
  the pair flag alone, since `implementation.md`'s usage always assigns
  `fledge-brooder-<species>` + `fledge-skua-<species>` for a pair, or a
  single named role for solo spawns like forager/incubator). Print (or
  `--json` emit) the allocated name(s).
- `roster release <name>`: calls `roster.Release`.
- `roster` (no args) / `roster --json`: calls `roster.List`, printing
  name→feather assignments (text) or the full entry list (`--json`).

## Tests
- `cmd/fledge` txtar tests:
  - `fledge roster assign --feather FTHR-001 --pair --json` returns two names
    sharing a species; a second `assign` call for a different feather returns
    the next unused species; assigning past all 18 in use returns a
    numeric-suffixed species.
  - `fledge roster release <name>` on one member of a pair, followed by
    `fledge roster` (list), still shows the entry (species not yet freed);
    releasing the second member removes it from the list; a subsequent
    `assign` reuses the freed species.
  - `fledge roster --json` with no assignments returns an empty list; with
    assignments, returns them.
Written first against the unregistered `roster` command (usage error /
unknown command) and confirmed to FAIL, then implemented until they pass
(satisfies PLM-026 FC-1, FC-2, FC-3, AC-1/AC-2/AC-3).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-054.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-026 FC-2"). AC-1 is always:
- [x] AC-1: The tests listed above were observed failing before implementation
  and pass after; evidence captured verbatim.
- [x] AC-2: `fledge roster assign --feather FTHR-### --pair` allocates and
  returns species/names as specified, with numeric-suffix overflow past 18
  (satisfies PLM-026 FC-1, AC-1).
- [x] AC-3: `fledge roster release <name>` frees a species only once every
  member sharing it is released (satisfies PLM-026 FC-2, AC-2).
- [x] AC-4: `fledge roster [--json]` lists current name→feather assignments,
  omitting fully-released species (satisfies PLM-026 FC-3, AC-3).
- [x] AC-5: `go test ./internal/cli/... ./cmd/fledge -run TestScripts`
  passes.
