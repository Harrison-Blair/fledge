---
id: FTHR-083
title: fledge roost command with unit eligibility and idempotent relocation
plumage: PLM-032
status: egg
priority: P2
depends_on: [FTHR-082]
authored: 2026-07-17T03:32:42Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-083: fledge roost command with unit eligibility and idempotent relocation

## Description

The command itself: `fledge roost` relocates completed work out of the active spec
directories into a `fledged/` subdirectory of each, one plumage-unit at a time.

Depends on FTHR-082 because relocation is only safe once discovery and ID allocation are
location-independent. Moving a spec before that lands makes it invisible and causes ID
reuse — FTHR-082 is what makes this feather's writes harmless.

Delivers the eligibility rule and the move, and nothing else. The `preen` advisory
(FTHR-084), the phase-close prose step (FTHR-085), and this repository's own migration
(FTHR-086) all build on the predicate this feather exports.

The eligibility rule, decided during interrogation and load-bearing for all three
downstream feathers:

- The **unit** is a plumage plus every feather whose `plumage:` field names it. Members are
  never separated.
- A unit is eligible only when the plumage is `fledged` **and** every feather in it is
  `fledged`. One straggling feather holds the whole unit back.
- A feather whose `plumage:` does not resolve (an orphan) is **never** eligible, in any
  circumstance.
- Ineligible units are reported with the reason, naming the feather responsible.

Satisfies PLM-032 FC-1, FC-3, FC-4, FC-5, FC-6, FC-9, FC-10, FC-11.

## Affected Modules

See `.fledge/nest/modules.md` → `internal/cli`, `internal/spec`;
`.fledge/nest/data-model.md` (status lifecycles: plumage `egg→hatched→fledged`, feather
`egg→pipping→hatching→fledged`; `Set.Reqs`/`Set.Tasks`; orphans modelled at
`colony.go:84-85`); `.fledge/nest/conventions.md` (command registration, exit codes,
`--json` on every command).

- `internal/cli/roost.go` — **new file**, the whole command.
- `internal/cli/cli.go` — one line in `commandOrder`. See Approach re: the scaffold stamp.
- `internal/repo/repo.go` — a read-only accessor for the destination path, alongside
  `RequirementsDir()`/`TasksDir()` (`repo.go:35-36`).
- `cmd/fledge/testdata/roost.txtar` — **new file** (not an edit to an existing fixture).

## Approach

**Export the eligibility predicate.** FTHR-084 and FTHR-086 both need it; duplicating the
rule in three places is how the three drift apart. Put it in `roost.go` as an exported
function over an already-loaded `*spec.Set` — e.g. returning eligible units and skipped
units with reasons — so callers pass a `Set` and get a decision without touching the
filesystem. Keep it pure: no I/O, no globals. That is the seam the tests hook into, and it
is why FTHR-084 can depend on this feather rather than reimplementing the rule.

**Grouping.** Build plumage-ID → feathers from `set.Tasks` by the `Requirement` field (YAML
key `plumage`). `colony.go:84-85` already models the orphan case — a feather whose plumage
reference does not resolve to a loaded plumage — and this feather must reach the same
conclusion for orphans: not eligible, never moved. Reuse that reasoning; do not invent a
second notion of "orphan".

**The move.** For each eligible unit, move the plumage file into
`<RequirementsDir()>/fledged/` and each of its feathers into `<TasksDir()>/fledged/`,
creating each destination with `os.MkdirAll` at first need (FC-10 — **not** at init; nothing
in `init.go` changes, which is what keeps this feather disjoint from FTHR-087).

Use `os.Rename`. It is atomic within a filesystem and preserves bytes exactly (FC-11); git
detects the rename at commit time from content identity, so `git mv` buys nothing here and
would drag a git dependency into a command that has none. Do **not** rewrite frontmatter:
`Path` is a load-time field (`data-model.md`), not persisted state, so nothing inside the
file refers to its own location.

**Idempotence (FC-9).** A second run must find nothing eligible and succeed. This falls out
of the predicate if it reads status rather than location: an already-roosted unit is still
fledged, but its files are already at the destination. Guard the no-op explicitly — if
source and destination resolve to the same path, skip without error. Report "nothing to
roost" and exit 0.

**Failure atomicity.** A unit is all-or-nothing per FC-3. If a member's move fails midway,
do not leave the unit split: attempt to restore the members already moved, and fail with a
non-zero exit naming the unit. Report the partial state rather than swallowing it.

**Scaffold stamp — read this before dispatching.** Adding `roost` to `commandOrder`
regenerates scaffolded content: `ExpectedFiles(m, commandOrder)` (`stamp.go:144`) feeds the
command list into per-adapter generated allow-lists, so their hashes change and
`.fledge/scaffold.json` is rewritten. This feather must therefore run `fledge init --refresh`
and commit the resulting stamp. It is the **first** of three feathers that rewrite that
stamp (FTHR-085 and FTHR-087 follow); they are serialized by `depends_on` precisely so they
never collide at merge. Do not dispatch them concurrently.

**Conventions.** Register via `init()` → `register("roost", runRoost, "fledge roost [--json]")`
per `cli.go`'s dispatch pattern, add to `commandOrder` in its correct position
(`command_parity_test.go` fails the build if registration and `commandOrder` disagree), and
support `--json` like every other command. Exit codes: `ExitOK` / `ExitFail` / `ExitUsage`
/ `ExitEnv` per convention.

## Tests

Unit tests for the predicate in `internal/cli/roost_test.go`; acceptance tests for the
command in a new `cmd/fledge/testdata/roost.txtar`. Per `.fledge/nest/testing.md`, this
split matches the repo: pure logic gets focused unit tests, user-visible CLI behavior gets a
txtar.

Run: `go test ./internal/cli -run TestRoost`,
`go test ./cmd/fledge -run TestScripts/roost`, then `go test ./...`.

**`internal/cli/roost_test.go`** (predicate — no filesystem)
- *fully fledged unit is eligible* — plumage fledged, all feathers fledged → eligible →
  AC-2.
- *fledged plumage with one non-fledged feather is not eligible* — the unit is skipped
  whole, and the skip reason names the responsible feather. Neither plumage nor any feather
  is eligible. This is the interrogation's central rule → AC-3.
- *non-fledged plumage is not eligible* — even with every feather fledged (the live
  PLM-030/FTHR-072 shape) → AC-3.
- *orphan feather is never eligible* — a feather whose `plumage:` resolves to nothing is
  never returned as eligible, alone or alongside eligible units → AC-4.
- *plumage with zero feathers is eligible when fledged* — the degenerate unit; pins that
  "all feathers fledged" is vacuously true rather than an error → AC-2.
- *predicate performs no I/O* — operates on an in-memory `Set`; pins the seam FTHR-084 and
  FTHR-086 depend on → AC-2.

**`cmd/fledge/testdata/roost.txtar`** (command)
- *roost relocates an eligible unit* — after `fledge roost`, the plumage and every feather
  are under `fledged/`, the active directories list only non-fledged work → AC-5,
  PLM-032 AC-1.
- *roosted specs remain fully visible* — `vee`, `unfledged`, `colony`, `preen` report
  identically afterwards (leans on FTHR-082) → AC-5, PLM-032 AC-2.
- *roost skips an ineligible unit and says why* — stdout names the unit and the blocking
  feather; nothing moved → AC-3, PLM-032 AC-4.
- *roost is idempotent* — two runs in succession; the second reports nothing relocated,
  exits 0, and the tree is byte-identical to after the first → AC-6, PLM-032 AC-7.
- *roost with nothing eligible creates no directory* — no `fledged/` directory exists
  afterwards → AC-7, PLM-032 AC-8.
- *roosted file content is byte-identical* — compare before/after → AC-8, PLM-032 AC-9.
- *`--json` output* — machine-readable relocated/skipped sets per convention → AC-9.

Test-first order is fixed: write these, run them against unchanged code and observe them
FAIL for the expected reason (no `roost` command registered), then implement until they
pass.

## Acceptance Criteria

- [ ] AC-1: The tests listed above were observed failing before implementation and pass
      after.
- [ ] AC-2: An exported, I/O-free eligibility predicate over a loaded spec set returns the
      units that are fully fledged, including the zero-feather case. Satisfies PLM-032 FC-4.
- [ ] AC-3: A unit whose plumage is fledged but which contains any non-fledged feather is
      not relocated — neither the plumage nor any feather — and is reported as skipped,
      naming the responsible feather. Satisfies PLM-032 FC-4, FC-5, AC-4.
- [ ] AC-4: A feather whose parent plumage does not resolve is never eligible and is never
      relocated. Satisfies PLM-032 FC-6, AC-5.
- [ ] AC-5: Requesting the command relocates every eligible unit's plumage and feathers into
      the `fledged/` subdirectory of their respective spec directories, and every command
      that loads the spec set still reports them. Satisfies PLM-032 FC-1, FC-3, AC-1, AC-2.
- [ ] AC-6: Running the command twice in succession leaves the second run reporting nothing
      relocated and exiting 0, with the tree byte-identical to after the first. Satisfies
      PLM-032 FC-9, AC-7.
- [ ] AC-7: Requesting the command with nothing eligible creates no destination directory.
      Satisfies PLM-032 FC-10, AC-8.
- [ ] AC-8: A relocated spec's content is byte-identical to its content before relocation.
      Satisfies PLM-032 FC-11, AC-9.
- [ ] AC-9: The command supports `--json` and uses the shared exit codes, per repo
      convention.
- [ ] AC-10: `fledge init --refresh` has been run and the resulting `.fledge/scaffold.json`
      committed; `fledge preen` reports the scaffold healthy and `go test ./...` passes.
