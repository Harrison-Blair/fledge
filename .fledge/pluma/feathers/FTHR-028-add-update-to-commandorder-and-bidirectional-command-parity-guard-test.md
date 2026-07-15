---
id: FTHR-028
title: Add update to commandOrder and bidirectional command-parity guard test
plumage: PLM-015
status: hatching
priority: P1
depends_on: []
authored: 2026-07-15T14:58:06Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-028: Add update to commandOrder and bidirectional command-parity guard test

## Description
The tracer slice for PLM-015. Add the already-registered `update` command to the ordered
command list so it appears in usage output and the generated agent allow-list, and add a
test that fails whenever the registered command set and the ordered list diverge in either
direction — so this class of drift can never recur silently. Update the CLI acceptance-test
fixtures that assert on generated scaffold output to reflect the new `update` allow-list
entry.

Satisfies PLM-015 FC-1 (usage listing), FC-2 (allow-list entry), and FC-3 (parity guard).

## Affected Modules
- `internal/cli/cli.go` — `commandOrder` slice (line ~105) and the `commands` map (line
  28, populated by each command file's `init()` via `register`). Usage generation iterates
  `commandOrder` at `cli.go:97-98`. See `.fledge/nest/modules.md → internal/cli` and
  `.fledge/nest/conventions.md` (self-registering command pattern).
- `internal/cli/` — new test file for the parity guard (e.g. `command_parity_test.go`).
- `cmd/fledge/testdata/init.txtar` — the acceptance test that asserts the generated
  `.claude/settings.local.json` allow-list content; adding `update` to `commandOrder`
  changes that generated output (the template ranges over `.CommandOrder`), so the fixture
  must be updated in lockstep. (`agents.txtar` was checked and does not assert the
  per-command allow-list; confirm during implementation and update only the fixtures that
  actually assert it.)

## Approach
1. **Parity test first.** Add a Go unit test in `internal/cli` that reads the `commands`
   registration map and the `commandOrder` slice and asserts *bidirectional* set equality:
   every key in `commands` appears in `commandOrder`, and every entry in `commandOrder`
   resolves to a registered command. It must fail loudly naming the offending command(s)
   in either direction. Because `commands` is package-private, the test lives in package
   `cli` (white-box).
2. Run it against unchanged code and confirm it FAILS reporting `update` present in
   `commands` but absent from `commandOrder`.
3. **Fix.** Add `"update"` to the `commandOrder` slice (append after `"version"`; order
   only affects display/allow-list ordering, not correctness). The parity test now passes.
4. **Fixtures.** Regenerate expectations for `cmd/fledge/testdata/init.txtar` so the
   asserted `settings.local.json` includes the `Bash(fledge update *)` entry in the
   position implied by `commandOrder`. Keep the byte-idempotent `writeIfChanged` behavior
   the txtar tests depend on (see `.fledge/nest/conventions.md`).

Constraints: do not redesign the registration/usage mechanism; the only source change is
the one-line `commandOrder` addition plus the new test. Do not hand-edit generated scaffold
files elsewhere — the dogfood regen of this repo's own `.claude/`/`.fledge/scaffold.json`
is FTHR-030's job, not this feather's.

## Tests
Written test-first, run failing before the fix, passing after:
- `TestCommandOrderMatchesRegistrations` (new, package `cli`) — pins FC-3: asserts every
  registered command is in `commandOrder` and every `commandOrder` entry is registered;
  fails before the fix with `update` missing from `commandOrder`.
- `cmd/fledge/testdata/init.txtar` (existing acceptance test, fixture updated) — pins FC-2:
  the generated allow-list contains `Bash(fledge update *)`. Confirm it fails against the
  pre-fix binary (no update entry) and passes after.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `fledge` run with no command lists `update` among its commands (FC-1) — captured from the built binary.
- [x] AC-3: The generated `.claude/settings.local.json` produced by `fledge init` contains a `Bash(fledge update *)` allow-list entry, asserted by the updated `init.txtar` fixture (FC-2).
- [x] AC-4: `TestCommandOrderMatchesRegistrations` enforces bidirectional parity and fails if either the registered set or `commandOrder` gains/loses a command without the other (FC-3).
- [x] AC-5: `fledge preen` passes and `go test ./...` is green.
