---
id: PLM-033
title: Stop shipping gitkeep placeholder files
status: hatched
priority: P3
authored: 2026-07-17T03:24:21Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# PLM-033: Stop shipping gitkeep placeholder files

## Context

Initializing a repository writes four `.gitkeep` placeholder files: into the raw scout
report directory, the feather claim directory, and both spec directories. They exist for
the single reason such files ever exist — version control cannot represent an empty
directory, so a placeholder is committed to make the directory survive a clone.

That reason no longer holds here. Every directory they guard is created on demand by the
code that writes into it: claims, raw reports, and spec files all create their parent
directory before writing, and loading a spec set treats a missing directory as an empty one
rather than an error. The placeholders are inert. They occupy listings, they are carried in
the scaffold's record of owned files, and they assert nothing the code does not already
guarantee.

The four are not in identical situations, and the difference was examined before deciding.
One is pure residue: its directory's contents are excluded from version control and the
placeholder itself was never tracked, so it has only ever occupied disk. One is genuinely
doing work today: its directory's contents are excluded from version control while the
placeholder itself is tracked — the established idiom for shipping an empty directory whose
contents are ignored. The remaining two matter only in a freshly initialized repository,
where the spec directories are briefly empty. Removing all four is a deliberate choice made
with that distinction visible, not an assumption that none of them did anything.

The tradeoff being accepted: after this change, a fresh clone will not contain those
directories until something creates them. Nothing breaks — the first claim, the first scout
report, and the first spec each create what they need — but the layout is no longer visible
in a clone before first use. That cost is judged smaller than the cost of four permanent
files asserting a constraint the code already enforces.

This change reaches beyond this repository by design. The scaffold records which files it
owns; files it no longer ships become obsolete in that record, and a refresh prunes them.
Consuming repositories will therefore lose these placeholders the next time each is
refreshed. That propagation is the mechanism working as intended and must not be read as
drift damage.

See `.fledge/nest/architecture.md` (the scaffold's ownership record, write policies, and
drift detection) and `.fledge/nest/modules.md` (`internal/cli`, `internal/bootstrap`).

## User Stories

- As a fledge user, I want the tool to stop committing placeholder files into my repository
  that serve no purpose, so that what fledge owns in my history is only what fledge
  actually needs.
- As a fledge developer, I want the scaffold's record of owned files to list only files
  that do something, so that reading it tells me what fledge is responsible for rather than
  what it once was.

## Functional Criteria

1. FC-1: Initializing a repository writes no `.gitkeep` placeholder files.
2. FC-2: Refreshing a repository writes no `.gitkeep` placeholder files, and removes any
   that a previous version wrote.
3. FC-3: The placeholders currently committed to this repository are removed from it.
4. FC-4: Every directory that previously held a placeholder is still created by the code
   that writes into it, at the moment it is first needed.
5. FC-5: A repository with no spec directories present loads as an empty spec set rather
   than failing.
6. FC-6: The version control rules that exclude the contents of the raw report and claim
   directories are unchanged. Only the placeholders are removed; those directories'
   contents remain excluded.

## Acceptance Criteria

- [ ] AC-1: Initializing a fresh repository produces no `.gitkeep` file anywhere under it.
- [ ] AC-2: Refreshing a repository that contains placeholders written by a previous
      version leaves none of them present afterward.
- [ ] AC-3: In a repository with no claim directory, recording a feather claim succeeds and
      creates the directory.
- [ ] AC-4: In a repository with no raw report directory, scaffolding context and writing a
      scout report succeeds and creates the directory.
- [ ] AC-5: In a repository with no spec directories, creating a plumage and a feather
      succeeds and creates the directories, and the IDs allocated are the first of their
      sequence.
- [ ] AC-6: In a repository with no spec directories, every command that loads the spec set
      reports an empty set and succeeds.
- [ ] AC-7: This repository contains no `.gitkeep` file, and its full test suite and health
      check pass afterward.
- [ ] AC-8: The version control rules excluding the raw report and claim directories'
      contents are byte-identical to their prior state.

## Out of Scope

- Refreshing the consuming repositories (`hearth`, `stenographer`). They will lose the
  placeholders through the scaffold's ordinary obsolescence-and-prune behavior the next
  time each is refreshed. Reaching into their histories from this plumage is deliberately
  not done.
- The version control rules that exclude the raw report and claim directories. They remain
  correct — they exclude those directories' *contents*, which is still wanted (FC-6).
- Any change to how directories are created, or to any other file initialization writes.
- Any change to the scaffold's ownership and drift mechanism itself. This plumage changes
  only which files that mechanism records.
- Spec relocation and the roosting of completed work (PLM-032). Both plumages touch
  initialization's file list, but they answer different user stories and are kept apart.

## Open Questions

None. Every question raised during interrogation was resolved.
