---
generated: 2026-07-06T23:33:05Z
commit: b701cf5a12a99b5adf9538e83f51178d4dead0c2
agent: fledge-context-gatherer
fledge_version: 0.1.0
---

# Domain Glossary

Business/domain vocabulary embodied by fledge, a spec-driven development tool.

- **Spec-driven development** — the methodology fledge supports: specifications (requirements and tasks) are the primary, versioned artifacts that drive and gate implementation.

- **Requirement (REQ)** — a feature specification stating *what* and *why*, identified `REQ-NNN`. Lifecycle: `draft` → `approved` → `done` (may revert approved → draft). Its body template has sections: Context, User Stories, Functional Criteria (FC-*), Acceptance Criteria (AC-*), Out of Scope, Open Questions. Functional Criteria drive task implementation; Acceptance Criteria verify completion. (`internal/spec/templates/requirement.md`)

- **Task (TASK)** — a unit of work implementing *how*, identified `TASK-NNN`, linked to exactly one parent requirement. Lifecycle: `blocked`/`ready` → `in-progress` → `done`. May depend on other tasks (`depends_on`). Its body template has sections: Description, Affected Modules, Approach, Tests (test-first), Acceptance Criteria — with AC-1 mandated as "tests observed failing before implementation and passing after." (`internal/spec/templates/task.md`)

- **Dependency graph** — the directed acyclic graph of tasks formed by `depends_on` edges. Cycles are errors, detected and reported as a path (`internal/graph`). A task depends on the tasks that must finish before it.

- **Wave** — a topological layer of the dependency graph. Wave 1 tasks have no dependencies; wave N tasks depend only on earlier waves. Used for `graph` layout and reasoning about parallelizable work.

- **Ready task** — a task whose dependencies are all `done` and which is not currently locked. `fledge ready` lists these, sorted by priority then ID. Dangling dependencies count as never-done, so they keep a task out of the ready set.

- **Blocked task** — a task with at least one unfinished dependency; the inverse of ready at creation time.

- **Lock** — an advisory claim on a task, stored as `.fledge/locks/TASK-NNN.lock` (JSON: task, owner, PID, created, branch). Acquiring a lock auto-transitions the task to in-progress and removes it from `ready`. Locks are atomic (`O_EXCL`); a second claimant gets a "held" error. PID liveness in `locks` output is informational only (PIDs recycle).

- **Oversight** — an optional review gate on a task: `merge` (review before merge) or `during` (review while in progress). Absent by default.

- **Finding** — a single validation result from `fledge check`, attributed to a file with a rule ID and a severity (Error or Warning). `--strict` promotes warnings to errors.

- **Priority** — an urgency level `P0`–`P3` (P0 highest) on requirements and tasks; used to order the ready list.

- **Frontmatter** — the YAML metadata block delimited by `---` at the top of every spec file, above the markdown body.

- **Body preservation** — fledge's guarantee that the markdown body below the frontmatter is never modified when it rewrites metadata; edits are byte-for-byte preserving.

- **Scan module** — a grouping of repository files by top-level directory (root-level files grouped as `<root>`), produced by `fledge scan` as the unit of context gathering.

- **Scan-ignore** — a gitignore-syntax file (`.fledge/scan-ignore`) listing paths excluded from `fledge scan` (e.g. `.git/`, `.fledge/`, `.claude/`, vendored/build dirs, lockfiles).

- **Agent** — the authoring identity recorded on a spec (e.g. `fledge-orchestrate/planning`). fledge is designed to be driven by `.claude/` agents that plan specs and run an implementation loop over locked tasks.
