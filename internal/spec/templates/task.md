# {{ID}}: {{TITLE}}

## Description
What this task delivers, scoped so one engineer/agent can complete it in a single focused effort.

## Affected Modules
Modules and key files involved, citing the context docs consulted (e.g. "see .fledge/context/modules.md → <module>").

## Approach
Implementation guidance: intended shape of the change, constraints, existing code to reuse. Tasks MAY contain implementation detail (unlike requirements). The design must be testable: seams, injectable dependencies, and observable outputs the tests below can hook into.

## Tests
The tests that prove this task's behavior, written test-first:
- Name each test and the behavior it pins down (map to the acceptance criteria below).
- Implementation order is fixed: (1) write the tests; (2) run them against the unchanged code and confirm they FAIL for the expected reason; (3) implement until they pass.

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/evidence/{{ID}}.md`. Reference the parent requirement's criteria where applicable (e.g. "satisfies {{REQ}} FC-2"). AC-1 is always:
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after.
- [ ] AC-2: …
