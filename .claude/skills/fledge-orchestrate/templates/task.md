# Task template

This file documents the task format. Files are created with `fledge new task` (the canonical skeleton is embedded in the binary); never instantiate this template by hand — the CLI allocates the ID, links the requirement, computes the initial ready/blocked hint, and stamps the frontmatter.

Tasks live at `spec/tasks/TASK-###-<kebab-name>.md`. IDs are zero-padded and next-sequential within the folder. Every task links to exactly one requirement. `depends_on` forms blocking relationships: a task is `ready` only when every task in `depends_on` is `done`.

```markdown
---
id: TASK-003
title: <task title>
requirement: REQ-001
status: blocked        # blocked | ready | in-progress | done
priority: P1           # P0 | P1 | P2 | P3
depends_on: [TASK-001, TASK-002]   # [] when unblocked from the start (then status: ready)
oversight: merge       # optional; omit for fully autonomous implementation
                       # merge  = implement & review normally, but hold the branch unmerged
                       #          until the user signs off on the diff + reviewer verdict
                       # during = prompt the user to confirm they are ready BEFORE spawning
                       #          the implementor, so they can participate during the work
authored: <UTC ISO 8601>
agent: fledge-orchestrate/planning
fledge_version: <VERSION file contents>
---

# TASK-003: <task title>

## Description
What this task delivers, scoped so one engineer/agent can complete it in a single focused effort.

## Affected Modules
Modules and key files involved, citing the context docs consulted (e.g. "see .fledge/context/modules.md → internal/graph").

## Approach
Implementation guidance: intended shape of the change, constraints, existing code to reuse. Tasks MAY contain implementation detail (unlike requirements). The design must be testable: seams, injectable dependencies, and observable outputs the tests below can hook into.

## Tests
The tests that prove this task's behavior, written test-first:
- Name each test and the behavior it pins down (map to the acceptance criteria below).
- Implementation order is fixed: (1) write the tests; (2) run them against the unchanged code and confirm they FAIL for the expected reason; (3) implement until they pass. A test that has only ever been seen passing proves nothing.

## Acceptance Criteria
Numbered, verifiable. Reference the parent requirement's criteria where applicable (e.g. "satisfies REQ-001 FC-2"). AC-1 is always:
1. AC-1: The tests listed above were observed failing before implementation and pass after.
2. AC-2: …
```
