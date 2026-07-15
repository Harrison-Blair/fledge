---
id: PLM-015
title: fledge update command discoverability and scaffold completion
status: fledged
priority: P1
authored: 2026-07-15T14:52:25Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# PLM-015: fledge update command discoverability and scaffold completion

## Context
`fledge update` (PLM-014) is a shipped, fully-implemented command — it downloads the
latest release, checksum-verifies it, and atomically swaps the running binary. But it
was registered without being added to the CLI's ordered command list, so it is
effectively invisible to the audience fledge is built for:

- it never appears in `fledge`'s top-level usage/help output;
- it is omitted from the generated Claude allow-list, so an agent that runs
  `fledge update` hits a permission prompt instead of the pre-approval every other
  command enjoys;
- it is absent from the README command reference and the "Upgrading" guidance.

The omission shipped green because nothing tests that the ordered command list stays in
sync with the set of registered commands — a manual invariant enforced only by a code
comment. Because this repo dogfoods fledge, its own generated allow-list
(`.claude/settings.local.json`) is likewise missing the `fledge update` entry and must be
regenerated once the command is added. (The scaffold version *stamp* here is already
current — this is only about the missing allow-list entry.)

This plumage makes the `update` command fully discoverable across every surface, adds an
automated guard so this class of drift cannot recur silently, and — as a preventive
measure — documents the scaffold-refresh step in the release process so the dogfood
scaffold is kept in sync on future version bumps.

## User Stories
- As an agent operating a fledge repo, I want `fledge update` to be pre-approved in my
  allow-list and listed in help, so that I can self-update without hitting a permission
  prompt or having to discover the command out-of-band.
- As a human user reading the README, I want `fledge update` documented in the command
  reference and the upgrading guidance, so that I know how to update the binary itself.
- As a fledge maintainer, I want a test that fails the moment a registered command is
  missing from the ordered command list (or vice-versa), so that a shipped command can
  never again be silently absent from help and the generated allow-list.
- As a maintainer cutting a release, I want the scaffold-refresh step to be part of the
  documented release steps, so that the dogfood scaffold cannot drift out of sync on a
  future version bump.

## Functional Criteria
1. FC-1: `fledge update` appears in the top-level usage/command listing emitted when
   `fledge` is run with no command.
2. FC-2: The generated agent allow-list includes a pre-approval entry for
   `fledge update`, on par with every other command.
3. FC-3: A test fails whenever the registered command set and the ordered command list
   diverge in either direction — a registered command absent from the ordered list, or
   an ordered-list entry that resolves to no registered command.
4. FC-4: This repository's own generated allow-list (`.claude/settings.local.json`) is
   regenerated to include the `fledge update` pre-approval entry once the command is added
   to the ordered list.
5. FC-5: The README documents `fledge update` in its command reference and in the
   upgrading guidance.
6. FC-6: The release process documentation includes the scaffold-refresh-and-commit step
   so the dogfood scaffold is kept in sync on every version bump (preventive).

## Acceptance Criteria
- [x] AC-1: `fledge` with no command lists `update` among its commands, and the
  generated allow-list contains a `fledge update` pre-approval entry.
- [x] AC-2: A test asserts bidirectional parity between the registered command set and
  the ordered command list, and fails if either side gains or loses a command without
  the other; verified failing before the fix (with `update` absent) and passing after.
- [x] AC-3: This repository's own `.claude/settings.local.json` is regenerated to contain a
  `Bash(fledge update *)` entry (the scaffold stamp is already current, so the operative
  change is the added allow-list entry).
- [x] AC-4: The README's command reference and upgrading section both cover
  `fledge update`.
- [x] AC-5: The release process documentation states the scaffold-refresh-and-commit
  step required on a version bump.
- [x] AC-6: `fledge preen` passes and the full test suite is green after the changes.

## Out of Scope
- Any change to what `fledge update` *does* (download, checksum, swap logic) — the
  command's behavior is correct and stays as-is here. (The missing HTTP timeout on the
  update fetch is tracked separately under the concurrency/robustness plumage.)
- Redesign of the command-registration or usage-generation mechanism beyond adding the
  missing entry and the parity guard.

## Open Questions
None.
