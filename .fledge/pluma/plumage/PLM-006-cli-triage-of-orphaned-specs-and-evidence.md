---
id: PLM-006
title: CLI triage of orphaned specs and evidence
status: hatched
priority: P2
authored: 2026-07-08T06:13:52Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# PLM-006: CLI triage of orphaned specs and evidence

## Context
When an agent edits or removes a spec, it can leave dangling artifacts that nothing
currently surfaces. fledge validates the forward direction — a checked feather must have
matching evidence, and plumage/`depends_on` references must resolve (`colony` and `preen`
already report orphan feathers and dangling refs) — but there is no reverse check. A
feather removed or renamed leaves its `.fledge/molt/FTHR-###.md` evidence file and any
`.fledge/broods/FTHR-###.brood` lock behind with no owner, and a plumage whose feathers
were all removed sits childless — all silently. This is not hypothetical: the most recent
commit on this repository reconstructed an FTHR-004 evidence file that had gone missing.
These left-behind artifacts should be triageable through the CLI so an agent or developer
can see what a spec edit orphaned and decide what to do about it.

## User Stories
- As an agent that just edited or removed a spec, I want the CLI to show me what artifacts
  that change orphaned, so that I can triage them instead of leaving silent cruft.
- As a developer, I want orphaned evidence and locks surfaced during routine validation, so
  that I notice them without hunting through `.fledge/` by hand.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: The CLI detects and surfaces evidence files with no owning feather, brood locks
   with no owning feather, and hatched/fledged plumages with no referencing feathers.
2. FC-2: Detection is read-only — surfacing an orphan never modifies or deletes any file.
3. FC-3: Surfacing never breaks a validation/merge gate: the observer report exits 0, and
   the left-behind-file cases appear as validation warnings, not errors.

## Acceptance Criteria
- [ ] AC-1: `fledge colony` reports an orphaned-artifacts section listing evidence files under `.fledge/molt/` with no matching feather, brood locks with no matching feather, and hatched/fledged plumages with zero referencing feathers; exit stays 0; the section is empty/absent when there are none; `--json` includes the structured data.
- [ ] AC-2: `fledge preen` emits a Warning (not an Error) for each orphaned evidence file and each orphaned brood lock, and these do not by themselves change preen's exit code.
- [ ] AC-3: Childless plumages are surfaced by `colony` only, never by `preen`.
- [ ] AC-4: Detection is strictly read-only — no code path deletes or modifies any evidence file, lock, or spec.
- [ ] AC-5: Automated tests (txtar covering each anomaly, the clean/empty case, and `--json` shape for both `colony` and `preen`) cover AC-1..AC-4 and the full suite passes.

## Out of Scope
- Any remediation or pruning (no auto-deletion of evidence, locks, or specs).
- Making the orphan cases hard `preen` errors.
- Orphan feather (dangling plumage) and dangling `depends_on` — already handled by existing
  `colony`/`preen`.
- Childless `egg` plumages (a normal mid-drafting state).
- A dedicated `fledge triage` command (folded into `colony`/`preen`).

## Open Questions
None — all decisions resolved during the 2026-07-08 interrogation. Follow-up to investigate
separately: whether `fledge abandon --force` can release a lock whose feather is already gone.
