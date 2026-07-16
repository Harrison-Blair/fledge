---
id: PLM-023
title: "Deterministic nest & release-fact checks"
status: fledged
priority: P1
authored: 2026-07-16T00:49:27Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# PLM-023: Deterministic nest & release-fact checks

## Context
A `/code-review med` pass over recent commits (`da33651` nest regeneration,
`6e1259e` PLM-021/022 planning) found that this repo's own dogfooded
`.fledge/nest/*.md` docs and release checklist carry facts an LLM has to
remember correctly every time — and they've already drifted:

- `conventions.md`'s "Versioning & release" section lists only two
  must-move-together files (`VERSION`, `internal/cli/version.go`
  `binaryVersion`), dropping `cmd/fledge/testdata/stamp_warning.txtar`, which
  also pins the current binary version. A release agent trusting the doc
  bumps two files, CI fails on the stale fixture, and the version number is
  burned (failed releases can't be reused).
- `entry-points.md`, `modules.md`, `index.md`, `testing.md`, and
  `dependencies.md` state hand-counted inventories ("24 commands", "23 txtar
  fixtures") that are wrong (ground truth: 18 commands in
  `internal/cli/cli.go`'s `commandOrder`, 22 `*.txtar` files in
  `cmd/fledge/testdata/`) and redrift on every forager run because scouts
  estimate them by eye rather than deriving them exactly.
- `data-model.md` dropped a previously-documented gotcha: `fledge preen`'s
  criteria-evidence check requires molt-evidence headings in the bare form
  `## AC-N`, and silently rejects a labeled variant like
  `## AC-1: failing test capture` — a brooder who doesn't know this loses
  time to a stalled feather.

This plumage is this review's most direct expression of its stated interest:
replace facts an agent must recall correctly with mechanisms that either
enforce the fact automatically (a test) or make it impossible to get wrong by
construction (an exact, cited computation instead of an eyeballed estimate;
a diagnostic that states its own requirement). Two prior gates already
settled the shape: all three fixes are narrow/root-cause, not the broader
"forager emits deterministic inventories for scouts to annotate" feature
floated during review, and the count-fix is a generic authoring rule added
to the forager/scout prose, not new fledge-specific CLI code.

## User Stories
- As a release engineer running the version-bump checklist, I want an
  automated check that the version string agrees across every file that must
  move together, so that a bump can't silently miss one and burn a release.
- As a forager/scout regenerating this repo's context docs, I want a rule
  that any count I write must come from an exact, re-derivable computation,
  so hand-authored inventories stop drifting from ground truth every run.
- As a brooder or skua running `fledge preen`, I want a criteria-evidence
  finding to state the exact required heading form itself, so I don't need to
  remember an undocumented gotcha to unblock a stalled feather.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: A committed automated test asserts that the version string in
   `VERSION`, the CLI's binary-version constant, and every committed txtar
   fixture that pins the current binary version (at minimum
   `stamp_warning.txtar`) all agree, and fails when any of them diverges.
2. FC-2: The forager/scout core-skill prose states, as a general authoring
   rule (not specific to any one repo), that any count reported in a
   synthesized doc — registered-command counts, test-fixture counts, file
   totals, and similar — must be produced by an exact, re-run-able
   computation (e.g. a grep, a glob, a line count) at write time, never
   estimated by eye.
3. FC-3: Once FC-2 lands and this repo's nest is regenerated, its concern
   docs report the correct counts (18 commands, 22 txtar fixtures) wherever
   they currently state a wrong one.
4. FC-4: `fledge preen`'s criteria-evidence finding message, emitted when a
   checked acceptance criterion's evidence section is missing, names the
   exact required heading form (bare `## AC-N`, not a labeled variant) as
   part of the message text itself.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [x] AC-1: The version-consistency test (FC-1) exists, was confirmed to fail
  when one of the three version strings is deliberately diverged, and passes
  at rest; it runs under `go test ./...`.
- [x] AC-2: The forager/scout core-skill prose (e.g. `foraging.md` and/or
  `templates/scout-report.md`) states the exact-computation-for-counts rule
  (FC-2), and this repo's scaffolded copies are refreshed to match
  (`fledge init --refresh`).
- [x] AC-3: This repo's own `.fledge/nest/` is regenerated and
  `entry-points.md`, `modules.md`, `index.md`, `testing.md`, and
  `dependencies.md` show the correct counts (19 commands, 25 txtar
  fixtures — the ground truth moved from the 18/22 believed at authoring
  time once FTHR-054 added the `roster` command and this plumage's own run
  added acceptance fixtures), confirmed by an independent recount against
  ground truth (FC-3).
- [x] AC-4: `fledge preen`'s criteria-evidence diagnostic names the required
  `## AC-N` heading form in its message text (FC-4), confirmed by a test that
  exercises the missing/mislabeled-evidence case and asserts on the message.
- [x] AC-5: `conventions.md`'s "Versioning & release" section lists all three
  must-move-together files once regenerated, consistent with FC-1's test.

## Out of Scope
- The broader "forager emits deterministic inventories for scouts to
  annotate" feature (per-module file lists, go.mod versions,
  exported-symbol lists) — declined during review gating as scope creep
  beyond what's broken today.
- Any new `fledge` CLI command or `fledge scan` extension to compute
  fledge-specific facts (e.g. a command-registry count) — the fix is a
  generic authoring rule in forager/scout prose, not new product code.
- Stale-lock worktree classification (F5) and the worker roster/species
  allocator (F6) — separate plumages.
- PLM-021's FC-2 wording tightening (F9) — handled as a direct edit to that
  existing plumage, not part of this one.

## Open Questions
None — all forks (F1/F2/F3 root-cause-narrow scope, F2's mechanism as a
generic prose rule rather than new CLI code, priority) were resolved during
interrogation.
