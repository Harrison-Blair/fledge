---
generated: 2026-07-06T23:33:05Z
commit: b701cf5a12a99b5adf9538e83f51178d4dead0c2
agent: fledge-context-gatherer
fledge_version: 0.1.0
---

# Data Model

The core types fledge operates on. The authoritative definitions live in `internal/spec/types.go` and `internal/spec/load.go`; lock and scan types live in their respective packages.

## Requirement (`internal/spec/types.go:Requirement`)
A feature specification ("what and why").

| Field | Type | Notes |
|---|---|---|
| `ID` | string | `PLM-NNN`, e.g. `PLM-001` |
| `Title` | string | |
| `Status` | string | `draft` \| `approved` \| `done` |
| `Priority` | string | `P0`–`P3` |
| `Authored` | string | RFC 3339 timestamp |
| `Agent` | string | authoring agent, e.g. `fledge-orchestrate/planning` |
| `FledgeVersion` | string | fledge version at creation, e.g. `0.1.0` |
| `Path` | string | file path when loaded; `""` if unsaved |
| `Body` | []byte | markdown body, byte-preserved |

Frontmatter key order (all mandatory): `id, title, status, priority, authored, agent, fledge_version` (`internal/spec/frontmatter.go`).

## Task (`internal/spec/types.go:Task`)
A unit of work implementing one requirement ("how").

| Field | Type | Notes |
|---|---|---|
| `ID` | string | `FTHR-NNN` |
| `Title` | string | |
| `Requirement` | string | parent `PLM-NNN` |
| `Status` | string | `blocked` \| `ready` \| `in-progress` \| `done` |
| `Priority` | string | `P0`–`P3` |
| `DependsOn` | []string | blocking `FTHR-NNN` IDs |
| `Oversight` | string | `""` when absent; `merge` or `during` when set |
| `Authored` | string | RFC 3339 |
| `Agent` | string | |
| `FledgeVersion` | string | |
| `Path` | string | |
| `Body` | []byte | byte-preserved |

Frontmatter key order (mandatory except `oversight`): `id, title, requirement, status, priority, depends_on, oversight, authored, agent, fledge_version`. `depends_on` always renders (as `[]` when empty); `oversight` is omitted when empty (`internal/spec/frontmatter.go`; `TestTaskFrontmatterOversightOptional`).

## Set (`internal/spec/load.go:Set`)
Result of loading all specs from the requirements and tasks directories.

| Field | Type | Notes |
|---|---|---|
| `Reqs` | []*Requirement | |
| `Tasks` | []*Task | |
| `Errors` | []FileError | aggregated per-file parse failures (`FileError{Path, Err}`) |
| `UnknownFields` | map[string][]string | file path → unrecognized frontmatter keys |

Lookups: `Set.Req(id)` and `Set.Task(id)` return the matching spec or nil. `Load` tolerates missing directories (returns an empty set, no error).

## Constants (`internal/spec/types.go`)
- Requirement statuses: `ReqDraft="draft"`, `ReqApproved="approved"`, `ReqDone="done"`.
- Task statuses: `TaskBlocked="blocked"`, `TaskReady="ready"`, `TaskInProgress="in-progress"`, `TaskDone="done"`.
- `Priorities = ["P0","P1","P2","P3"]`.
- `OversightValues = ["merge","during"]`.

## check.Finding (`internal/check/check.go`)
A single validation result: `{File, Rule, Severity, Message}`, where `Severity` is `Error` or `Warning`. Returned as `[]Finding` from `check.Run`; `HasErrors` reports whether any are errors.

## Graph (`internal/graph/graph.go`)
Internal representation of the task dependency DAG: the task slice plus a `byID` index. Not persisted; constructed on demand via `graph.New(tasks)` for `Cycle`/`Waves`/`Ready`.

## lock.Record (`internal/lock/lock.go`)
JSON content of a `.fledge/broods/FTHR-NNN.lock` file: `{Task, Owner, PID, Created, Branch}`. Acquisition contention returns a `HeldError` wrapping the existing `Record`.

## scan types (`internal/scan/scan.go`)
- `Module{Name, Files, Count, Bytes}` — files grouped by top-level directory (`<root>` for root-level files).
- `Result{Commit, ShortCommit, Modules}` — inventory plus commit stamp (`ShortCommit == "none"` when there are no commits).

## Open Questions
- Whether the spec layer validates that a Task's `Requirement` and `DependsOn` IDs actually exist is a higher-level responsibility: existence/dangling-ref checks are enforced in `internal/check`, not in `internal/spec` parsing.
