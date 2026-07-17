---
id: PLM-031
title: Dev install mode linking scaffold to local fledge source
status: fledged
priority: P1
authored: 2026-07-17T01:46:28Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# PLM-031: Dev install mode linking scaffold to local fledge source

## Context

fledge's agent and skill definitions ship as content embedded in the binary, and
`fledge init` writes them into a repo as copies. Propagating a change to that prose
therefore costs a rebuild-and-reinstall of the binary followed by a refresh in every
consuming repo — a multi-step process whose outcome is not visible at a glance.

fledge is developed while being used: its author actively works inside two consuming
repositories (`hearth`, `stenographer`) that depend on the same fledge source under
active edit. Those repos need the bleeding-edge definitions, but today they can only
get them by repeating that propagation process, and drift is the normal state between
repetitions rather than the exception. The drift is present as this plumage is authored:
a consuming repo's agent definitions already differ from the working tree they came from.

The cost is not keystrokes. When a propagation step is skipped or applied incompletely,
agents run against stale definitions and the resulting misbehavior is discovered only
after tokens have been spent on it. The requirement is therefore twofold: remove the
propagation step for prose, and make the resulting state observable enough to trust
without inspecting files by hand.

Because the definitions are read from disk by the agent harness, a repo whose scaffold
points directly at the source tree reads the author's edits as they are saved — no
rebuild and no refresh. A symlink write policy already exists in the scaffold system and
is already used for other scaffolded paths, so the mechanism is established; what is
missing is a way to aim it at a working copy of the source instead of at repo-local
content. This plumage adds that as an explicit, opt-in install mode.

See `.fledge/nest/architecture.md` (scaffold write policies, embedded core/adapter
trees, drift detection) and `.fledge/nest/modules.md` (`internal/bootstrap`).

## User Stories

- As a fledge developer working inside a consuming repository, I want that repo's agent
  and skill definitions to resolve to my local fledge working tree, so that edits I save
  take effect immediately without reinstalling the binary or refreshing the repo.
- As a fledge developer, I want to confirm at a glance that a repo is dev-linked and
  where it points, so that I can trust the definitions in play instead of discovering a
  stale one after it has cost me tokens.
- As a fledge developer, I want the machine-specific links themselves to stay out of my
  repositories' history, so that they can never be committed and break a clone or CI.
- As a fledge developer editing skill prose in the fledge repository itself, I want the
  same immediacy there, so that the repo that dogfoods fledge is not the one place that
  still requires a refresh.

## Functional Criteria

1. FC-1: An opt-in dev install mode, requested at init time, points a repository's
   copy-type scaffold at a fledge source tree instead of writing embedded copies.
2. FC-2: The mode accepts an explicit source location. It is never inferred from the
   environment or from a location compiled into the binary.
3. FC-3: Requested without a source location from within a fledge source checkout, the
   mode links that checkout's own scaffold to its own source. Requested without a source
   location from anywhere else, it fails with an error naming what was missing.
4. FC-4: A source location that is not a usable fledge source tree is rejected at
   request time, before any scaffold file is modified.
5. FC-5: The linked set is exactly the scaffold files whose content is copied verbatim
   from the source — the agent definitions and the core skill documents. Files that are
   rendered from templates or merged into files fledge does not own are written as they
   are today.
6. FC-6: Dev-linked paths are excluded from version control by recording them in the
   repository's ignore rules. If any is already tracked when the mode is requested, it
   fails without modifying anything and reports which paths must be untracked first, and
   how. The ignore rules themselves are the one dev-mode change the repository's history
   sees; they name paths rather than machine-specific locations, and so stay valid for
   any clone.
7. FC-7: When the source tree's version differs from the running binary's version, the
   mode reports the mismatch and identifies both versions.
8. FC-8: A dedicated status query reports whether the repository is dev-linked, the
   source it points at, and how many files are linked.
9. FC-9: The status query identifies any dev link whose target no longer resolves,
   naming each broken path.
10. FC-10: A dev-linked repository is not reported as damaged, drifted, or user-edited by
    the health check, and repeating the health check does not alter the dev links.
11. FC-11: A refresh of a dev-linked repository preserves its dev links rather than
    resetting them to copies. Files dev mode does not cover are refreshed as they are
    today, so a refresh remains the way to update them without disturbing dev mode.

## Acceptance Criteria

- [x] AC-1: In a consuming repository, requesting dev mode against a fledge working tree
      causes an edit saved in that tree to be visible through the repository's scaffold
      with no rebuild, reinstall, or refresh performed in between.
- [x] AC-2: Requesting dev mode without a source location succeeds inside a fledge source
      checkout and fails, with an actionable error, outside one.
- [x] AC-3: Requesting dev mode against a path that is not a fledge source tree fails and
      leaves every scaffold file byte-identical to its prior state.
- [x] AC-4: Requesting dev mode in a repository where a to-be-linked path is tracked by
      version control fails, names the tracked paths and the remedy, and leaves the
      working tree unmodified.
- [x] AC-5: After a successful request, no dev-linked path is reported as a change by
      version control.
- [x] AC-6: Files that are rendered or merged rather than copied are still produced, and
      are not links, in a dev-linked repository.
- [x] AC-7: A source/binary version mismatch produces a report naming both versions; a
      match produces no such report.
- [x] AC-8: The status query distinguishes a dev-linked repository from a normally
      scaffolded one, and reports the source path and linked-file count for the former.
- [x] AC-9: With a dev link's target removed, the status query reports that link as
      broken and names it.
- [x] AC-10: The health check on a dev-linked repository reports no findings attributable
      to dev mode, and reports the same result when run twice in succession.
- [x] AC-11: After a refresh of a dev-linked repository, every dev-linked path is still a
      link to the same source, and an edit saved in the source is still visible through
      the repository's scaffold.
- [x] AC-12: A refresh of a dev-linked repository still updates the files dev mode does
      not cover.

## Out of Scope

- Distributing or rebuilding the fledge **binary**. This plumage links prose only; a
  consuming repo's binary is still whatever is installed there. FC-7 exists to surface
  the resulting skew, not to resolve it.
- Files that are rendered from templates or appended into files fledge does not own.
  These cannot be links; changing their sources still requires a refresh.
- Configuring the source location through an environment variable or a persisted user
  config. The source is named explicitly at request time (FC-2).
- Any change to how the embedded definitions are produced, or to the non-dev install
  path, beyond what dev mode requires.
- Multi-machine or shared-checkout use. Dev mode is a single-developer, single-machine
  convenience: the links it creates are deliberately kept out of version control, and the
  ignore rules that keep them out (FC-6) are its only intended git-visible effect.

## Open Questions

- ~~**Refresh interaction.**~~ *Resolved during feather interrogation:* a refresh must
  preserve dev links rather than reset them to copies, and is covered by its own feather.
  Recorded as FC-11.
- **Reverting — known gap, load-bearing.** How a repository leaves dev mode and returns to
  ordinary copies — and whether the paths excluded from version control under FC-6 are then
  restored to being tracked — was not decided. This was a loose end when it was parked; it
  is now load-bearing. FC-11's refresh behavior was resolved as *fail loudly* when the
  recorded source no longer validates, so a developer who moves or deletes their source
  tree gets an erroring refresh whose only sanctioned remedy is re-pointing at a valid
  source. That is a coherent state to ship — re-pointing is a real fix — but the absence of
  an explicit exit from dev mode is a deliberate, known gap and the likely next plumage.
- **Version-mismatch severity.** FC-7 reports the skew. Whether a mismatch should ever
  block the request rather than merely report it was not decided.
