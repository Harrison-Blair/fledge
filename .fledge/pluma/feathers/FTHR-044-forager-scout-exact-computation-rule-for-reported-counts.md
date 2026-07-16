---
id: FTHR-044
title: Forager/scout exact-computation rule for reported counts
plumage: PLM-023
status: fledged
priority: P1
depends_on: []
authored: 2026-07-16T01:55:16Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-044: Forager/scout exact-computation rule for reported counts

## Description
Scouts currently hand-estimate mechanical inventories in `.fledge/nest/`
concern docs — registered-command counts, test-fixture counts, file totals —
and these numbers are wrong today (18 commands reported as 24, 22 txtar
fixtures reported as 23) and redrift on every forager run since nothing
requires an exact source. This feather adds a generic authoring rule to the
core forager/scout prose: any count stated in a synthesized doc must come
from an exact, re-run-able computation (a grep, glob, or line count) executed
at write time, never estimated by eye. This is prose-only and not specific to
this repo — it's a rule scouts/foragers carry into every future run.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` — the
  Scout section (~line 72-80, "Follow the section order...") is where
  scout-authoring conventions live; the new rule is added here.
- `internal/bootstrap/core/skills/fledge-orchestrate/templates/scout-report.md`
  — the schema reference scouts follow; add a one-line pointer to the rule if
  it strengthens discoverability, but the substantive rule lives in
  `foraging.md` (see `.fledge/nest/modules.md` → bootstrap/adapters for how
  `core/` sources scaffold to `.fledge/skills/`).
- `cmd/fledge/testdata/init.txtar` / `init_agents.txtar` (or wherever the
  scaffolded `foraging.md` content is asserted) — must be updated to match
  the new prose.

## Approach
Add a rule statement to `foraging.md`'s Scout section, near the existing
"Follow the section order in `templates/scout-report.md`... exactly" line:
any count, total, or enumerated size reported in a report or synthesized doc
(e.g. "N commands," "N fixtures," "N files in module X") must be derived from
an exact, cited computation performed at write time (a `grep -c`, a `find`/
glob count, `wc -l`, or equivalent) — never recalled or estimated. State that
scouts should show or reference the computation (e.g. cite the command run)
so the count is re-derivable by a future reader, not just asserted. Keep the
wording generic (applies to any fledge-managed repo, not this one's specific
numbers) per this plumage's Out of Scope.

## Tests
- A `cmd/fledge` txtar test (extending an existing init-scaffold fixture, or
  adding a targeted one) asserts the scaffolded `.fledge/skills/fledge-orchestrate/foraging.md`
  contains the new exact-computation rule text after `fledge init`.
  Written first against the *current* scaffolded content and confirmed to
  FAIL (the rule text doesn't exist yet), then the prose is added and
  `fledge init --refresh` regenerates the scaffold until the assertion
  passes (satisfies PLM-023 FC-2, AC-2).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-044.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-023 FC-2"). AC-1 is always:
- [x] AC-1: The tests listed above were observed failing before implementation and pass after; evidence captured verbatim.
- [x] AC-2: `foraging.md`'s core source states the exact-computation-for-counts
  rule as a generic authoring requirement (satisfies PLM-023 FC-2, AC-2).
- [x] AC-3: `fledge init --refresh` regenerates this repo's scaffolded copy to
  match, and `go test ./cmd/fledge -run TestScripts` passes.
