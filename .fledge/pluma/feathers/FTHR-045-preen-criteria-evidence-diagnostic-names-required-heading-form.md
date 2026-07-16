---
id: FTHR-045
title: Preen criteria-evidence diagnostic names required heading form
plumage: PLM-023
status: pipping
priority: P1
depends_on: []
authored: 2026-07-16T01:55:16Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-045: Preen criteria-evidence diagnostic names required heading form

## Description
`fledge preen`'s criteria-evidence check (`checkCriteriaEvidence` in
`internal/check/check.go`) requires a checked acceptance criterion's evidence
section to be a line exactly equal to `## AC-N` (`hasSection` does an exact
line match) — a labeled variant like `## AC-1: failing test capture` does
NOT satisfy it. This gotcha was previously documented in nest prose and got
dropped on regeneration; a brooder who writes a labeled heading gets every
checked criterion reported missing evidence and the feather stalls. Rather
than relying on prose to carry this fact, this feather makes the diagnostic
self-explaining: its message names the exact required heading form directly.

## Affected Modules
- `internal/check/check.go` — `checkCriteriaEvidence` (~line 267-292), the
  `add(t.Path, "criteria-evidence", Warning, ...)` message format string
  (see `.fledge/nest/modules.md` → internal/check).

## Approach
Edit the `Warning` message built in `checkCriteriaEvidence` to state the
required form explicitly, e.g.: `checked criteria missing evidence sections
in %s: %s (heading must be the bare form "## AC-N", not "## AC-N: <label>")`.
No change to `hasSection`'s matching behavior — only the diagnostic text.

## Tests
- A unit test in `internal/check` (extending existing check tests) exercises
  a feather whose evidence file has a labeled heading (`## AC-1: failing test
  capture`) for a checked criterion `AC-1`, and asserts the resulting
  `criteria-evidence` finding's message contains the required bare-heading
  form text. Written first against the unchanged message format and
  confirmed to FAIL (message doesn't yet name the form), then the message is
  updated until it passes (satisfies PLM-023 FC-4, AC-4).

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-045.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-023 FC-2"). AC-1 is always:
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after; evidence captured verbatim.
- [ ] AC-2: `checkCriteriaEvidence`'s emitted message names the exact required
  `## AC-N` heading form as part of its text (satisfies PLM-023 FC-4, AC-4).
- [ ] AC-3: `go test ./internal/check/...` passes.
