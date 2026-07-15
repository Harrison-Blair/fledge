---
id: PLM-015
title: fledge update command discoverability and scaffold completion
status: hatched
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
comment. Separately, this repo dogfoods fledge, and its own scaffold version stamp was
never refreshed across the `0.4.0 → 0.5.4` bumps, so every `fledge` invocation here
prints a stamp-mismatch warning. Regenerating the scaffold to pick up the new allow-list
entry also clears that stale stamp.

This plumage makes the `update` command fully discoverable across every surface, adds an
automated guard so this class of drift cannot recur silently, and codifies the scaffold
refresh into the release process so the dogfood stamp stays current.

## User Stories
- As an agent operating a fledge repo, I want `fledge update` to be pre-approved in my
  allow-list and listed in help, so that I can self-update without hitting a permission
  prompt or having to discover the command out-of-band.
- As a human user reading the README, I want `fledge update` documented in the command
  reference and the upgrading guidance, so that I know how to update the binary itself.
- As a fledge maintainer, I want a test that fails the moment a registered command is
  missing from the ordered command list (or vice-versa), so that a shipped command can
  never again be silently absent from help and the generated allow-list.
- As a maintainer cutting a release, I want the scaffold-stamp refresh to be part of the
  documented release steps, so that the dogfood repo never drifts to a stale stamp again.

## Functional Criteria
1. FC-1: `fledge update` appears in the top-level usage/command listing emitted when
   `fledge` is run with no command.
2. FC-2: The generated agent allow-list includes a pre-approval entry for
   `fledge update`, on par with every other command.
3. FC-3: A test fails whenever the registered command set and the ordered command list
   diverge in either direction — a registered command absent from the ordered list, or
   an ordered-list entry that resolves to no registered command.
4. FC-4: Running any `fledge` command in this repository emits no scaffold
   stamp-mismatch warning (the repo's own scaffold stamp matches the current binary
   version).
5. FC-5: The README documents `fledge update` in its command reference and in the
   upgrading guidance.
6. FC-6: The release process documentation includes the scaffold-refresh-and-commit step
   so the dogfood stamp is kept current on every version bump.

## Acceptance Criteria
- [ ] AC-1: `fledge` with no command lists `update` among its commands, and the
  generated allow-list contains a `fledge update` pre-approval entry.
- [ ] AC-2: A test asserts bidirectional parity between the registered command set and
  the ordered command list, and fails if either side gains or loses a command without
  the other; verified failing before the fix (with `update` absent) and passing after.
- [ ] AC-3: No `fledge` command run in this repository prints a scaffold stamp-mismatch
  warning.
- [ ] AC-4: The README's command reference and upgrading section both cover
  `fledge update`.
- [ ] AC-5: The release process documentation states the scaffold-refresh-and-commit
  step required on a version bump.
- [ ] AC-6: `fledge preen` passes and the full test suite is green after the changes.

## Out of Scope
- Any change to what `fledge update` *does* (download, checksum, swap logic) — the
  command's behavior is correct and stays as-is here. (The missing HTTP timeout on the
  update fetch is tracked separately under the concurrency/robustness plumage.)
- Redesign of the command-registration or usage-generation mechanism beyond adding the
  missing entry and the parity guard.

## Open Questions
None.
