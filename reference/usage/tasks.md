# Tasks

[README](../../README.md)

A task is a durable brief with an owner and an outcome.
Records live in `.fledge/state/tasks/` beside the agent records, so they survive the owner's pane closing.
Owners and verifiers are Fledge agent record IDs (see [Identity](agents.md#identity)).
Task status changes only through these commands.

Completion is never derived from Herdr idle or done.

```sh
fledge task template > brief.md                               # optional skeleton
fledge task create --title "Fix the parser" --file brief.md   # prints the task ID
fledge task create --title "Add tests" --file tests.md --parent 1a2b3c4d --after 5e6f7a8b
fledge task create --title "Try a probe" --body "one line"
fledge task import --file .fledge/tmp/plans/5d6ab497.toml --dry-run   # see Proposals
fledge task depend --id 9c0d1e2f --after 1a2b3c4d --remove 5e6f7a8b
fledge task list --ready                                        # created and unblocked
fledge task assign --id 1a2b3c4d --name worker
fledge task complete --id 1a2b3c4d --summary "Fixed in abc123"  # run by the owner
fledge task verify --id 1a2b3c4d --summary "Tests pass"         # run by another agent
fledge task list --status completed
fledge task get --id 1a2b3c4d
```

## Briefs

A brief is any nonblank, valid UTF-8 text without NUL.
`create` and `import` store it exactly as given, without trimming, and `assign` delivers it unchanged.
Blank, invalid UTF-8, or NUL-containing text fails before any state is created as invalid input (exit 2, phase `validation`).
There is no required structure and no `--freeform` flag.

The optional **brief template** is a recommended structure: six `## ` headings, in this order:

- `## Objective` – the outcome wanted, in one paragraph.
- `## Acceptance criteria` – concrete, testable checks that decide completion.
- `## Scope` – allowed files or areas, and what is explicitly out.
- `## Known facts` – established facts the worker would otherwise rediscover, with `file:line` where known.
- `## Deliverables` – the return contract: evidence to report, findings or decisions to return, what may remain undone.
- `## Constraints` – rules: authorization, commit policy, who to report to, what to escalate.

`fledge task template` prints the six headings, each followed by an HTML comment hint, as a starting point.
Nothing checks the headings.
A brief may use some, none, or other headings.
Fledge accepts even an unfilled skeleton.

Write the brief before creating the task.
`--proposal` prints a proposal skeleton instead: `schema_version`, a `[parent]`, and one example `[[tasks]]` entry, each brief being the brief skeleton.
`--json` returns `kind` (`brief` or `proposal`) and `text` in the outcome envelope.

`task template` never contacts Herdr or reads task state, so it works outside a repository.

## Lifecycle

A task moves `created` → `assigned` → `completed` → `verified`.
`task cancel [--reason TEXT]` ends a `created`, `assigned`, or `completed` task as `cancelled`.
A `created` or `assigned` parent can also go straight to `verified` once its subtasks are finished (see `verify` below).
A verified task can only be verified again, cancelled tasks are final, and other out-of-order transitions fail with `task_invalid_state`.

There are no progress updates within a task and no recorded checks.

A task can be a **subtask** of a parent and can run **after** prerequisite tasks.
Subtasks and prerequisites are independent.
A parent keeps its own lifecycle and does not wait for its subtasks.
A subtask is not a prerequisite of its parent.

- **Subtasks.** `create --parent TASK` fixes the parent at creation.
  It cannot change later.
  The parent must exist and be neither verified nor cancelled (`task_not_found` or `task_invalid_state`).
  Nesting depth is unlimited and cycles cannot form because a parent always predates its subtasks.
  `list` and `get` show the progress of direct subtasks as `2/3 verified`, adding `, 1 cancelled` when any were cancelled: cancelled subtasks are shown but not counted toward the total.
  `verify` refuses a parent whose direct subtasks are not all verified or cancelled with `task_open_subtasks`.
  `--force` verifies a completed parent anyway, records `forced: true`, and lists the open subtasks.
  Cancelling a parent leaves its subtasks unchanged.
- **Dependencies.** A prerequisite is satisfied once it is `verified`.
  A `cancelled` prerequisite also counts as satisfied but stays on its dependents and is shown with its state and reason, never dropped.
  Declare prerequisites with repeatable `create --after TASK`.
  Change them later with `depend --id TASK --after PREREQ --remove PREREQ`.
  Both flags are repeatable.
  Removals apply first.
  Adding a present prerequisite or removing an absent one changes nothing.
  `depend` refuses verified or cancelled tasks with `task_invalid_state` and unknown prerequisites with `task_not_found`.
  It refuses additions that would form a cycle, including self-dependencies, with `task_dependency_cycle`.
  The error names the chain.
  `assign` refuses a task with unmet prerequisites with `task_dependencies_unmet`, naming them, before it contacts the agent.
  `--force` assigns it anyway and records the unmet prerequisites as `unmet_at_assign` (null otherwise), separately from verify's `forced`.
  `cancel` names the created tasks it left ready.
  These checks and the changes they guard run under the state store lock, so concurrent commands cannot form a cycle or lose an edit.

- `create` requires a single-line `--title` and a brief (`--body`, or `--file` with `-` for stdin), and takes optional `--parent` and repeatable `--after`.
  `created_by` is the caller's agent record, or null when the caller is unregistered.
- `assign --id TASK` takes exactly one of `--name`, `--pane`, or `--agent-id`.
  The agent must be registered.
  Otherwise it fails with `agent_unregistered` and suggests `fledge agent adopt`.
  Fledge first stores the `created` or `assigned` task as assigned to the agent.
  It then submits the brief like `agent message`, with these parts in order:

  1. The sender header.
  2. `task: <id> · title: <title> · complete with: fledge task complete --id <id> --summary "..."`.
  3. The brief.

  Fledge records delivery (`message_id`, `pane`, `delivered_at`, `error`, `uncertain`) in a second step.
  A failed delivery leaves the task assigned with `delivery.error` set and a `partial` outcome.
  If Herdr cannot determine whether the brief arrived, `delivery.uncertain` is true and the outcome is `unknown`.
  The output says the delivery outcome is unknown. Fledge never retries delivery.
  If the task changes between lookup and the locked update, for example through another assignment, assign fails with `task_state_changed`.
- `complete --id TASK` (`--summary` or `--file`) requires an `assigned` task and a caller whose live agent record is the owner (`task_not_owner` otherwise).
  `--force` overrides the owner check.
  The result is stored on the task first.
  Then the distinct registered creator is sent a short notice, without the report body: `task completed: <id> · title: <title> · read result: fledge task get --id <id> · verify with: fledge task verify --id <id> --summary "..."`.
  Read the full result with `task get`.
  An unregistered creator or a creator completing its own task needs no notification.
  A stale creator or acknowledged delivery failure returns `partial`.
  An uncertain delivery returns `unknown`.
  The task remains completed, the notification outcome is recorded, and delivery is never retried automatically.
- `verify --id TASK [--summary TEXT | --file PATH]` requires a `completed` or `verified` task and a registered caller other than the owner.
  The owner is refused with `task_self_verification` and an unregistered caller with `caller_unregistered`.
  `--force` overrides both and records `forced: true`.
  This is a workflow guard, not a security boundary.
  Verifying an already-verified task, for example after fixes, applies the same checks and replaces `verifier` (null for an unregistered `--force`), `verification_note` (null when omitted), `forced`, and `verified_at`.
  Only the latest verification is kept, and the result, completion, and owner are unchanged.
  Verification is not tied to a Git revision, so name the checked commit in `--summary` when it matters.
  A grouping task in `created` or `assigned` can go directly to `verified` under the same caller checks.
  Every direct subtask must be `verified` or `cancelled`, and at least one must be `verified`.
  Its owner, if any, is unchanged and not notified.
  Such a task with open subtasks is refused with `task_open_subtasks`, naming them.
  One whose subtasks were all cancelled, or that has no subtasks, is refused with `task_invalid_state` (cancel the former instead).
  `--force` never overrides these state checks.
- `list [--status STATE] [--owner AGENT_ID] [--parent TASK] [--ready]` prints tasks oldest first.
  Columns include ID, status, owner, parent, waiting prerequisites, subtask progress, and title.
  Owner shows the agent's name while its record is live, otherwise its ID.
  `--parent` keeps direct subtasks only.
  `--ready` keeps created tasks whose prerequisites are all satisfied.
  Filters combine.
  `get --id TASK` prints the full record with its parent, subtask progress, and each prerequisite's state, for example `after: 1a2b3c4d (verified), 5e6f7a8b (cancelled: superseded), 9c0d1e2f (assigned, waiting)`.

Task commands do not collect usage.
`complete` and `verify` read no harness transcripts.
For token and cost figures, ask for them explicitly with [`fledge agent usage`](agents.md#usage), which reads an agent's whole native session on demand.
`assign`, the owner's own `complete`, and `verify` still capture live native session refs on the owner's or verifier's agent record.

Capture is best effort: a failed write produces a `warning` effect.
This lets `agent usage` find the session later.

Each record holds `id`, `title`, `brief`, `parent`, `after`, `owner`, `status`, `result`, `verifier`, `verification_note`, `forced`, `cancel_reason`, `created_at`, `created_by`, `assigned_at`, `unmet_at_assign`, `completed_at`, `completion_notification`, `verified_at`, `cancelled_at`, and `delivery`.
Records written before subtasks and dependencies load with a null `parent`, `after`, and `unmet_at_assign`.
A completion notification records its `recipient`, `message_id`, optional `pane`, `delivered_at`, `error`, and `uncertain` state.
Every task command supports `--json` with the same outcome envelope as the agent commands.

JSON results add these derived fields:

- List rows: `progress` (`verified`, `total`, `cancelled`, or null) and `waiting`.
- Get: `progress` and `dependencies` (`id`, `title`, `status`, `cancel_reason`, `satisfied`).
- Verify: `open_subtasks`.
- Cancel: `unblocked`.
- Template: `kind` and `text`.

`task list`, `task get`, `task depend`, and `task template` never contact Herdr.

## Proposals

A **proposal** is a TOML file describing several tasks at once, typically written by a planner and reviewed before dispatch.
`fledge task template --proposal` prints a skeleton.

```toml
schema_version = 1

[parent]                     # optional: the task the others are created under
title = "Task brief templates and decomposition"
brief = '''
## Objective
...
'''

[[tasks]]
key = "brief-lib"            # names the task within this file
title = "Add brief template validation"
brief = '''
## Objective
...
'''
after = []                   # local keys or existing 8-hex task ids

[[tasks]]
key = "import"
title = "Add task import"
brief = '''...'''
after = ["brief-lib"]
```

`schema_version` must be `1`, and unknown keys anywhere are rejected.
There must be at least one `[[tasks]]` entry.
Each `key` is nonempty, single-line, unique, and never an 8-hex task id.
Each `title` is nonempty and single-line.

Every brief, including the parent's, must be nonblank, valid UTF-8 without NUL after TOML decoding (so an escaped `\u0000` is rejected too), with no required headings.
Each `after` entry names a key in the file or an existing task id.
Local dependencies must not form a cycle.

```sh
fledge task import --file plan.toml --dry-run            # validate and preview
fledge task import --file plan.toml                      # create the tasks
fledge task import --file - --parent 1a2b3c4d < plan.toml
```

`task import --file PATH` (`-` for stdin) checks the whole file before it opens the state store.
Under one store lock, import creates the optional `[parent]` and the tasks in dependency order.
Where dependencies allow, it uses file order.
It resolves each `after` key to the new task's ID and sets every task's `parent`.

`--parent TASK` places the tasks under an existing task instead.
It must be neither verified nor cancelled, and it conflicts with a `[parent]` in the file.

`created_by` is the caller, as for `create`.
Import never assigns.
A real import contacts Herdr's socket only when run inside a Herdr pane (`HERDR_PANE_ID` set), to identify the caller.
Outside a pane it imports without the socket.

`--dry-run` runs the same checks, including that `--parent` and any existing `after` ids exist, and prints the tasks in creation order without creating anything:

```text
Would create 2 tasks under 1a2b3c4d:
  brief-lib  Add brief template validation
  import     Add task import  (after: brief-lib)
```

A real run prints `Created task 9c0d1e2f (brief-lib): Add brief template validation` per task, preceded by the created parent's own line and followed by `Created 2 tasks under parent <id>` when the file had a `[parent]`.
The JSON result is `{"parent": <id or null>, "tasks": [{"key", "id", "title", "after"}], "dry_run": <bool>}`.
`id` is null on a dry run, and `after` holds the file's entries on a dry run and resolved ids otherwise.
Effects list one `created task <id>` per record.

A file or schema problem, a `[parent]` with `--parent`, or a missing `--file` is invalid input (exit 2, phase `validation`).
An invalid brief also fails as invalid input.
Its error names the task key, as in `task "import": brief must not be blank`.
An unknown `--parent` or `after` id fails with `task_not_found`, and a verified or cancelled `--parent` with `task_invalid_state` (phase `task`).

None of these create anything.
The store keeps one file per record, so a failure partway through a real import leaves the earlier records in place.

The outcome is then `partial` and its effects name them.
A dry run never contacts Herdr.

By convention a planner writes its proposal to `.fledge/tmp/plans/<task-id>.toml`, named for the planning task it was assigned.
`.fledge/tmp/` is the agents' scratch directory and is already ignored.
No Fledge command creates it, and `task import` accepts any path.
