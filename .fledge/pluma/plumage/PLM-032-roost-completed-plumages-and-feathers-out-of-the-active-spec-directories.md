---
id: PLM-032
title: Roost completed plumages and feathers out of the active spec directories
status: hatched
priority: P2
authored: 2026-07-17T03:16:51Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# PLM-032: Roost completed plumages and feathers out of the active spec directories

## Context

fledge keeps every plumage and feather it has ever produced as a flat file in one of two
directories. Those directories accumulate without bound: nothing has ever left them, and
in a repository that has been managed by fledge for any length of time, finished work
comes to dominate them. In this repository, at the time of authoring, 73 of 81 feathers
and 25 of 31 plumages are fledged — roughly 88% of what a reader sees is work that is
already merged, verified, and closed.

The cost is borne by the human. Agents never browse these directories; they query them
through the CLI, which filters by status and is unaffected by volume. A person opening the
feathers directory in an editor or file tree sees eighty-one entries and must filter the
handful of live ones out of them by eye, on every visit. The signal — what is being
drafted, and what is ready to implement — is the small minority of what is displayed.

The remedy is to make the directory itself carry the distinction that the status field
already carries: completed work is relocated into a dedicated subdirectory of each spec
directory, named for the terminal status it holds, leaving the active directory listing
only work that is drafting or ready to implement. The agreed vocabulary is a `roost`
operation moving completed units into a `fledged/` subdirectory of each spec directory —
"fledged" rather than a new word, because it is exactly the status the eligibility rule
reads, so the directory name means one thing and needs no glossary.

Two properties of the existing design make this more than a file move, and they are the
substance of this plumage. Spec discovery and ID allocation both read a single directory
level and neither descends into subdirectories; allocation in particular derives the next
ID from the *filenames* it can see. Relocating completed specs without addressing this
would make finished work invisible to every command that loads the spec set, and — far
worse — would cause the allocator to reissue IDs that already belong to merged historical
specs, silently. Location must therefore become presentational: something the tooling
is indifferent to, and the reader benefits from. That inversion, not the move, is the
feature's real content.

Relocation is requested explicitly rather than triggered by a status change. Completion is
recorded at merge time, and relocating a file at that moment would place a rename inside
the window where a feather's branch is being merged and its status flipped — a known sharp
edge in this repository. Keeping the operation out of that window is deliberate; the cost
is that tidiness drifts until the operation is run, which the phase-close and health-check
behaviors below exist to counter.

See `.fledge/nest/data-model.md` (spec file layout, frontmatter, the plumage and feather
status lifecycles), `.fledge/nest/conventions.md` (ID and lifecycle rules), and
`.fledge/nest/modules.md` (`internal/spec`, `internal/cli`).

## User Stories

- As a fledge user opening a spec directory, I want to see only work that is drafting or
  ready to implement, so that I can find the live items at a glance instead of filtering
  finished work out by eye every time.
- As a fledge user, I want a completed plumage and its feathers to be filed together as a
  unit, so that a finished piece of work stays readable as one story rather than being
  scattered.
- As a fledge user, I want relocation to happen only when I ask for it, so that files never
  move underneath an in-flight merge.
- As a fledge user, I want every command to behave identically regardless of where a spec
  file sits, so that tidying my directories can never change what the tooling reports.
- As a fledge user, I want newly allocated IDs to remain unique after tidying, so that
  relocating finished work can never cause a new spec to collide with a historical one.
- As a fledge user, I want to be told when finished work is waiting to be tidied, so that
  the distinction the directories are meant to carry does not quietly decay.

## Functional Criteria

1. FC-1: An explicit, user-requested operation relocates completed work out of each active
   spec directory into a dedicated subdirectory of it, named for the terminal status.
2. FC-2: No status transition relocates anything as a side effect. Relocation happens only
   when the operation is requested.
3. FC-3: The unit of relocation is a plumage together with every feather belonging to it.
   The members of a unit are never separated by the operation.
4. FC-4: A unit is eligible only when the plumage is fledged and every feather belonging to
   it is fledged. A fledged plumage with any non-fledged feather is not eligible, and
   nothing belonging to it moves.
5. FC-5: The operation reports each ineligible unit it skipped and the reason it was
   skipped, so that a plumage held back by a straggling feather is visible rather than
   silently ignored.
6. FC-6: A feather whose parent plumage does not resolve is never eligible and is never
   relocated, in any circumstance.
7. FC-7: Spec discovery is independent of a spec's location: every command that loads the
   spec set reports identically for a relocated spec and a non-relocated one. Relocation
   changes no command's output other than by the file paths it reports.
8. FC-8: ID allocation accounts for relocated specs. An ID belonging to any spec, relocated
   or not, is never reissued.
9. FC-9: The operation is idempotent. Requested when nothing is eligible, it relocates
   nothing and succeeds.
10. FC-10: The destination subdirectory is created when it is first needed. It is not
    created at repository initialization.
11. FC-11: Each spec's content is unchanged by relocation. Only its location changes.
12. FC-12: The health check reports how much completed work is awaiting relocation. The
    report is advisory: an untidied repository is not a failing one, and the check's exit
    status is unaffected by it.
13. FC-13: The orchestration workflow requests the operation as part of closing a phase, so
    that tidying happens at a point where no merge is in flight.
14. FC-14: The spec files already present in this repository are relocated once under these
    rules, as part of delivering this plumage.

## Acceptance Criteria

- [ ] AC-1: After requesting the operation in a repository containing an eligible unit, the
      active spec directories list only non-fledged work, and the unit's plumage and
      feathers are all present in the destination subdirectories.
- [ ] AC-2: Every command that loads the spec set reports the same information about a
      relocated spec as it did before relocation, with only its reported path changed.
- [ ] AC-3: Allocating a new plumage and a new feather in a repository whose earlier specs
      have all been relocated yields IDs that follow the highest existing ID, colliding
      with no relocated spec.
- [ ] AC-4: A unit whose plumage is fledged but which has a non-fledged feather is not
      relocated — neither the plumage nor any of its feathers — and the operation reports it
      as skipped, naming the feather responsible.
- [ ] AC-5: A feather whose parent plumage does not resolve is not relocated, and remains in
      the active directory.
- [ ] AC-6: Fledging a spec, by any means that records completion, leaves that spec's
      location unchanged.
- [ ] AC-7: Requesting the operation twice in succession leaves the second run reporting
      nothing relocated and succeeding, with the tree byte-identical to after the first.
- [ ] AC-8: Requesting the operation in a repository with nothing eligible creates no
      destination subdirectory.
- [ ] AC-9: A relocated spec's file content is byte-identical to its content before
      relocation.
- [ ] AC-10: The health check on a repository with completed work awaiting relocation
      reports that fact and still succeeds; on a tidied repository it does not report it.
- [ ] AC-11: The phase-close step of the orchestration workflow requests the operation.
- [ ] AC-12: This repository's own fledged plumages and their feathers are relocated, its
      active directories list only non-fledged work, and its full test suite and health
      check pass afterward.

## Out of Scope

- **Removing the `.gitkeep` files fledge ships.** Raised during interrogation and accepted
  as a real cleanup, but it answers a different user story, carries its own blast radius
  (initialization, the scaffold stamp and its pruning behavior in consuming repositories,
  and the fixtures that assert on them), and is authored as its own plumage rather than
  fused into this one.
- Relocating anything on a status transition. Explicitly rejected: it would put a file
  rename inside the merge window (FC-2).
- Changing the status lifecycles themselves, or coupling a plumage's status to its
  feathers' statuses. FC-4 reads both statuses; it does not change how either is written.
  A fledged plumage with an unfledged feather remains a state the tooling permits.
- Splitting a unit so that its fledged members move and its unfledged members stay. This
  was considered and rejected in favor of unit coherence (FC-3).
- Relocating anything other than plumage and feather spec files. Evidence, claims, context
  documents, and the ledger are untouched.
- Any change to how completion is decided or recorded.

## Open Questions

- **Leaving the roost — known gap.** How a unit returns to the active directories if it is
  re-opened after being relocated was not decided. A fledged plumage can be moved back to
  `hatched`, at which point its files sit in a destination whose name no longer describes
  them. The operation as specified only ever moves work in one direction: it would leave
  such a unit misfiled, and FC-9's idempotence means a subsequent request would not correct
  it. This is a deliberate, known gap rather than an oversight — the forward path is
  useful on its own, and re-opening a closed plumage is rare — but it is the most likely
  follow-on work.
- **Whether the health check's advisory report should be promoted under a stricter mode.**
  FC-12 fixes the report as advisory and non-failing. Whether a stricter invocation should
  treat untidied work as a finding was raised and left undecided; nothing in this plumage
  depends on the answer.
