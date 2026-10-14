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
fledge task create --title "Parser stack" --file stack.md --shape stack --delivery-mode pr   # see Stacks
fledge task import --file .fledge/tmp/plans/5d6ab497.toml --dry-run   # see Proposals
fledge task depend --id 9c0d1e2f --after 1a2b3c4d --remove 5e6f7a8b
fledge task list --ready                                        # created and unblocked
fledge task assign --id 1a2b3c4d --name worker
fledge task complete --id 1a2b3c4d --summary "Fixed in abc123"  # run by the owner
fledge task verify --id 1a2b3c4d --commit HEAD --summary "Tests pass"  # run by another agent
fledge task land --id 1a2b3c4d                                  # after the user's word
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

A task moves `created` → `assigned` → `completed` → `verified` → `landed`.
`task cancel [--reason TEXT]` ends a `created`, `assigned`, or `completed` task as `cancelled`.
A `created` or `assigned` parent can also go straight to `verified` once its subtasks are finished (see `verify` below).
A verified task can only be verified again or landed with `task land` (see [Landing](#landing)).
Cancelled and landed tasks are final, and other out-of-order transitions fail with `task_invalid_state`.

There are no progress updates within a task and no recorded checks.

A task can be a **subtask** of a parent and can run **after** prerequisite tasks.
Subtasks and prerequisites are independent.
A parent keeps its own lifecycle and does not wait for its subtasks.
A subtask is not a prerequisite of its parent.

- **Subtasks.** `create --parent TASK` fixes the parent at creation.
  It cannot change later.
  The parent must exist and be neither verified, landed, nor cancelled (`task_not_found` or `task_invalid_state`).
  Nesting depth is unlimited and cycles cannot form because a parent always predates its subtasks.
  `list` and `get` show the progress of direct subtasks as `2/3 verified`, adding `, 1 cancelled` when any were cancelled: cancelled subtasks are shown but not counted toward the total.
  A `landed` subtask counts as verified in the `verified` count and the `2/3 verified` text.
  `verify` refuses a parent whose direct subtasks are not all verified, landed, or cancelled with `task_open_subtasks`.
  `--force` verifies a completed parent anyway, records `forced: true`, and lists the open subtasks.
  Cancelling a parent leaves its subtasks unchanged.
- **Dependencies.** A prerequisite is satisfied once it is `verified` or `landed`.
  A `cancelled` prerequisite also counts as satisfied but stays on its dependents and is shown with its state and reason, never dropped.
  Declare prerequisites with repeatable `create --after TASK`.
  Change them later with `depend --id TASK --after PREREQ --remove PREREQ`.
  Both flags are repeatable.
  Removals apply first.
  Adding a present prerequisite or removing an absent one changes nothing.
  `depend` refuses verified, landed, or cancelled tasks with `task_invalid_state` and unknown prerequisites with `task_not_found`.
  It refuses additions that would form a cycle, including self-dependencies, with `task_dependency_cycle`.
  The error names the chain.
  `assign` refuses a task with unmet prerequisites with `task_dependencies_unmet`, naming them, before it contacts the agent.
  `--force` assigns it anyway and records the unmet prerequisites as `unmet_at_assign` (null otherwise), separately from verify's `forced`.
  `cancel` names the created tasks it left ready.
  These checks and the changes they guard run under the state store lock, so concurrent commands cannot form a cycle or lose an edit.

- `create` requires a single-line `--title` and a brief (`--body`, or `--file` with `-` for stdin), and takes optional `--parent` and repeatable `--after`.
  `--delivery-mode` and `--base-branch` set how and where the task lands (see [Delivery modes](#delivery-modes)).
  `--shape` and `--branch` set the shape of a parent and the branch of a task (see [Stacks and features](#stacks-and-features)).
  `created_by` is the caller's agent record, or null when the caller is unregistered.
- `assign --id TASK` takes exactly one of `--name`, `--pane`, or `--agent-id`.
  The agent must be registered.
  Otherwise it fails with `agent_unregistered`: the agent must register itself with `fledge agent adopt` in its own pane, or be started with `fledge agent spawn`.
  Fledge first stores the `created` or `assigned` task as assigned to the agent.
  It then stores the brief in the owner's [inbox](agents.md#inbox) as an `assignment` record and rings its doorbell, which adds ` · task: <id> · <title>`.
  The stored body is these parts in order:

  1. `task: <id> · title: <title> · complete with: fledge task complete --id <id> --summary "..."`.
  2. The brief.

  Fledge records delivery (`message_id`, the inbox record id; `pane`; `delivered_at`; `error`; `uncertain`) in a second step.
  A doorbell that did not ring leaves the task assigned with `delivery.error` set and a `partial` outcome.
  If Herdr cannot determine whether the doorbell rang, `delivery.uncertain` is true and the outcome is `unknown`.
  The brief itself is never typed; the supervision daemon, which assign starts, may ring the doorbell again.
  If the task changes between lookup and the locked update, for example through another assignment, assign fails with `task_state_changed`.
- `complete --id TASK` (`--summary` or `--file`) requires an `assigned` task and a caller whose live agent record is the owner (`task_not_owner` otherwise).
  `--force` overrides the owner check.
  The result is stored on the task first.
  Then a short notice, without the report body, is stored in the inbox of the distinct registered creator as a `message` record with `task` set, and its doorbell is rung: `task completed: <id> · title: <title> · read result: fledge task get --id <id> · verify with: fledge task verify --id <id> --summary "..."`.
  Read the full result with `task get`.
  An unregistered creator or a creator completing its own task needs no notification.
  A stale creator or acknowledged delivery failure returns `partial`.
  An uncertain delivery returns `unknown`.
  The task remains completed and the notification outcome is recorded, with the inbox record id as `message_id`.
  The notice itself is never typed; the supervision daemon may ring its doorbell again.
- `verify --id TASK [--summary TEXT | --file PATH] [--commit REV]` requires a `completed` or `verified` task and a registered caller other than the owner.
  The owner is refused with `task_self_verification` and an unregistered caller with `caller_unregistered`.
  `--force` overrides both and records `forced: true`.
  This is a workflow guard, not a security boundary.
  Verifying an already-verified task, for example after fixes, applies the same checks and replaces `verifier` (null for an unregistered `--force`), `verification_note` (null when omitted), `forced`, `verified_at`, and `verified_commit`.
  Only the latest verification is kept, and the result, completion, and owner are unchanged.
  `--commit` binds the verification to a commit (see [Verified commit](#verified-commit)).
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
It also holds `delivery_mode`, `branch`, `base_branch`, `verified_commit`, `landed_at`, `landed_by`, `merge_commit`, `target_branch`, `pr_url`, and `land_step`.
It also holds `shape`, `verified_patch_id`, `carried_from`, and `merge_request`.
A record without `shape` loads with an empty shape.
Records written before subtasks and dependencies load with a null `parent`, `after`, and `unmet_at_assign`.
Records written before landing load with `delivery_mode` `none` and null landing fields.
A completion notification records its `recipient`, `message_id`, optional `pane`, `delivered_at`, `error`, and `uncertain` state.
Every task command supports `--json` with the same outcome envelope as the agent commands.

JSON results add these derived fields:

- List rows: `progress` (`verified`, `total`, `cancelled`, or null) and `waiting`.
- Get: `progress` and `dependencies` (`id`, `title`, `status`, `cancel_reason`, `satisfied`).
- Verify: `open_subtasks` and `warnings`.
- Land: `mode`, `warnings`, `review_pending`, and `size`, beside the record's `land_step`.
- Cancel: `unblocked`.
- Template: `kind` and `text`.

`task list`, `task get`, `task depend`, `task template`, and `task size` never contact Herdr.

## Verified commit

`verify --commit REV` resolves `REV` with `git rev-parse --verify --end-of-options REV^{commit}` in the caller's checkout.
It stores the 40-hex commit as `verified_commit`.
A revision that names no commit is invalid input, and the task is unchanged.
Without `--commit`, `verified_commit` is null, and a later verification replaces it like the other verification fields.
When the caller's checkout has staged, unstaged, or untracked changes, verify prints a warning that the bound commit is not exactly what was tested, and continues.
`task get` prints `verified commit: <hash>`.
`task land` lands only the verified commit.

### Patch id carry-forward

`verify --commit` also stores `verified_patch_id`.
This value is the output of `git patch-id --verbatim` for the diff `<base>...<commit>`.
The base is the base ref of the task, as [Size](#size) finds it. For a stack layer, the base is the layer base.
When the base does not resolve, or the diff is empty, verify stores a null patch id and prints a warning.
Verify never refuses because of a patch id.
Each verification clears `carried_from`.
A verification without `--commit` stores a null patch id.

A rebase changes the commit but can keep the patch.
When the branch head is not `verified_commit`, `task land` and `task size` compute the patch id of the head against the current base:

- An equal patch id carries the verification forward.
  The command stores the head as `verified_commit` and the old commit as `carried_from`.
- Another patch id is `land_head_mismatch`, and the record does not change.

`--verbatim` keeps whitespace, so a change only in whitespace gives another patch id.
`task get` prints `verified patch id:` and `carried from:` when the record holds them.
A carry-forward does not push.
In `pr` mode, push the rebased head yourself first, else the push of `task land` is `land_push_rejected`.

## Delivery modes

A task's `delivery_mode` decides how `task land` delivers its branch:

- `local` merges the branch into the target branch in the checkout that has the target checked out. Fledge verification is the only review.
- `pr` pushes the branch, opens a GitHub pull request with `gh`, and waits for its checks.
  Then `task land` stops with the pull request open for the owner's read (the `review` step).
  `task land --merge` merges it later, on the owner's word.
  This mode gives each pull request an agent review (the verifier) and a human review.
- `auto` pushes, opens, checks, and merges a pull request in one command, with no review stop.
  The verifier is the only review.
- `none` refuses to land the task (`task_no_delivery_mode`).

Every mode lands only a `verified` task with a non-null `verified_commit` (see [Landing](#landing)).

`create --delivery-mode MODE` sets the mode.
Without it, the mode is `delivery_mode` in `.fledge/config.toml`, else `none`.
This repository's `.fledge/config.toml` sets `delivery_mode = "pr"`.
`task land --mode local|pr|auto` overrides the mode for a landing and stores it on the task.
`create --base-branch BRANCH` names the branch the task lands into instead of the repository integration branch.
The branch must be nonblank and a single line, must not start with `-`, and must pass `git check-ref-format refs/heads/BRANCH`.
Otherwise it is invalid input.
A proposal entry takes the same `delivery_mode` and `base_branch` keys, and `import` checks them in the same way.
A proposal entry also takes a `branch` key, as `create --branch` does.

## Landing

`task land --id TASK [--branch BRANCH] [--mode local|pr|auto] [--merge] [--json]` merges a verified task's branch into its target branch.
Then the task moves to `landed`.
The caller must be a registered agent other than the owner (`caller_unregistered`, `task_self_landing`, no `--force`).
The task must be `verified` (`task_invalid_state`) with a non-null `verified_commit` (`land_unverified_commit`).

The branch is `--branch`, else the branch that an earlier `task land` stored, else the branch checked out in the owner's recorded checkout, else the branch its spawn created (`land_branch_unknown`).
The target is the task's `base_branch`, else the repository integration branch, reduced to a short name.
No integration branch, or a failed lookup of it, is `land_target_missing`.
`land` refuses a target whose short name is not a valid branch name with `land_target_invalid`, before any git or gh command runs.
`land` stores `--branch` and `--mode` even when it refuses the landing.

One `task land` runs at a time per repository: it holds an exclusive lock on `.fledge/state/land.lock` for the whole command.
Before it merges or pushes, it requires all of these:

- The branch is valid (`land_branch_invalid`) and differs from the target (`land_same_branch`).
- `refs/heads/<branch>` is the verified commit, or has the verified patch id (`land_head_mismatch`, see [Patch id carry-forward](#patch-id-carry-forward)).
- No checkout of the branch has staged or unstaged changes (`land_dirty`).
- The target exists: `refs/heads/<target>` in local mode, `refs/remotes/origin/<target>` in pr and auto mode (`land_target_missing`).
- The target does not already contain the verified commit (`land_already_merged`, with nothing recorded).
  A pr task with a stored `pr_url` skips this check, because `land` checks that pull request instead (see below).

In **local** mode, `land` merges in the checkout that has the target checked out.
That checkout is the primary checkout or a managed worktree under `.fledge/worktrees`.
For example, a member of a feature parent lands in the feature checkout that has the feature branch checked out.
When no such checkout has the target checked out, the refusal is `land_target_checkout_missing`.
An unmanaged checkout of the target does not count.
When that checkout has staged or unstaged changes, the refusal is `land_target_checkout_dirty`.
Untracked files are allowed.
`land` runs `git merge --no-ff -m "Merge branch '<branch>' (verified task <id>)" <verified_commit>` there.
It never stashes, resets, or checks out.
When the merge fails, `land` aborts it and requires `HEAD` and the full `git status` output to match the state before the merge, byte for byte (`land_restore_failed` otherwise).
A conflict is `land_merge_conflict`.
Git's refusal to overwrite untracked files, or a failed hook, is `land_merge_refused` with git's message.
A merge overwrites ignored files that the branch starts to track, but git refuses to overwrite untracked files.

In **pr** and **auto** mode, `land` requires `gh` on `PATH` (`land_gh_missing`) and an `origin` remote (`land_target_missing`).
It passes every value to `gh` as `--flag=value`; a failed `gh` command is `land_gh_failed` with gh's message.
It runs these steps, each recorded in `land_step`:

1. `push`: if `origin/<branch>` is not the verified commit, push the verified commit to the branch on origin (`land_push_rejected`).
2. `pr`: find the open pull request from the branch into the target, or open one with the task title and a body that names the task, commit, verifier, note, and owner, and the size report.
   More than one open pull request is `land_pr_ambiguous`.
   `land` records `pr_url` as soon as the pull request exists, so a failed lookup after `gh pr create` keeps it.
3. `checks`: refuse a draft (`land_pr_draft`), a `CONFLICTING` pull request (`land_pr_not_mergeable`), an `UNKNOWN` one (`land_mergeable_unknown`, retry later), or another head (`land_head_mismatch`).
   Refuse checks that are still running or a status context that is `PENDING` or `EXPECTED` (`land_checks_pending`).
   Refuse a check run that concluded other than `SUCCESS`, `NEUTRAL`, or `SKIPPED`, or a status context other than `SUCCESS` (`land_checks_failed`).
   Zero checks pass.
4. `review` (pr mode only): stop with the pull request open for the owner's read.
   The task stays `verified`, with `land_step` `review` and `pr_url` set.
   The command succeeds, and the JSON result has `review_pending` true.
   The text output names the pull request and the next command, `fledge task land --id TASK --merge`.
   auto mode skips this step.
5. `merge`: `gh pr merge --merge --match-head-commit=<verified_commit>` (`land_merge_failed` with gh's message).
6. `confirm`: only a `MERGED` pull request with a merge commit is recorded as landed (`land_unconfirmed` otherwise, for example under a merge queue).
   Then `land` runs `git fetch origin <target>` so that `agent cleanup` sees the merge.

`task land --id TASK --merge` continues a pr task after the review stop.
It runs every guard and step 1 to 3 again, then merges and confirms only while the pull request head is the verified commit.
`--merge` applies only to pr mode: with `--mode local` or `--mode auto`, or a task in another mode, it is invalid input.
A pr task with no `pr_url` is `land_review_not_open`: run `task land` without `--merge` first.

The owner can also merge a review-stage pull request with the GitHub button.
Each `task land` of a pr task with a stored `pr_url`, with or without `--merge`, first reads that pull request:

- `MERGED` at the verified commit: `land` records the landing with GitHub's merge commit (`land_step` `confirm`) and fetches the target.
  After a squash or rebase merge, the verified commit is not in the target history, and `land` warns that `agent cleanup` will hold the checkout.
- `MERGED` at another head: `land_pr_merged_other_head`.
- `CLOSED` without a merge: `land_pr_closed`; reopen it, then run `task land` again.
- `MERGED` with no merge commit reported: `land_unconfirmed`.
- Open: `land` continues with the steps above.

Every step can run again, so run `task land` again after a refusal.
A refusal in pr or auto mode after step 1 leaves the pushed branch, the pull request, `pr_url`, and `land_step`.
`land` decodes `gh` JSON strictly, and a missing field is `land_gh_output_invalid`.

Before it records the landing, `land` checks under the store lock that the task is still `verified` with the same `verified_commit` (`task_state_changed`).
It records `merge_commit`, `target_branch`, `landed_at`, `landed_by`, and status `landed`.
Every refusal is `rejected` with its code, and the JSON result shows the `land_step` reached.
`land` never removes a checkout, stops an agent, or deletes a branch.
`agent cleanup` does that and counts a `landed` task as finished.

### Size report

Every `task land` that passes the head check measures the verified commit against the target, as [`task size`](#size) does, with the same base ref and the limits of the mode.
The report prints after the outcome, also on a refusal and at the review stop, and is the JSON `size` field (null when the head check was not reached or the measure failed).
A pull request that `land` opens carries the report in its body.
After a landing, each measure over a limit is a `review size:` warning.
The review stop and refusals print the report without warnings.
The size is advisory: it never refuses a landing.

## Stacks and features

A parent task can have a **shape**: `stack` or `feature`.
The direct subtasks of a `stack` parent are its **layers**.
The direct subtasks of a `feature` parent are its **members**.
There is no separate stack record. The parent is the group.

- `create --shape stack|feature` stores the shape in the `shape` field. Any other value is invalid input.
- `task get` prints `shape: <value>` when the shape is not empty.
- `create --branch BRANCH` stores the branch of a task in the `branch` field.
  The branch must pass the same check as `--base-branch`.
- The proposal `[parent]` table takes the keys `shape`, `branch`, `base_branch`, and `delivery_mode` (see [Proposals](#proposals)).
  `import` checks the parent `delivery_mode` as it checks the `delivery_mode` of a task.
  Without the key, the parent gets the repository default, as a task does.

### The chain rule

The layers of a stack form one `after` chain.
Only the `after` edges between the layers of one stack count:

- Exactly one layer, the bottom, has no prerequisite among the layers.
- Each other layer has exactly one prerequisite among the layers.
- No layer is the prerequisite of more than one layer.
- Every layer is in the chain.

Cancelled layers stay in the chain. A prerequisite outside the stack does not count.
`task import` refuses a stack whose tasks fork as invalid input: `stack layers must form one after chain: ...`.
`task import --parent <stack parent>` checks the existing layers and the new tasks together.
`task create --parent <stack parent>` refuses with `task_stack_fork` when the new task is not `after` the top layer and no other layer.
The first layer names no layer.
`task depend` can still make a fork later.
Then `task size` refuses with `size_stack_fork`, and `task land` refuses with `land_stack_fork`.
A `feature` parent puts no rule on the order of its members.

### The layer base

The **layer base** is the branch that `task size` and `task land` compare a layer with:

- For the bottom layer, the **stack base**: the parent's `base_branch`, else the [`[branches] dev`](worktrees.md#integration-branch) branch.
- For any other layer, the stored `branch` of the layer below.
- For a layer above a `landed` layer, the stack base. GitHub moves the base of that pull request to the stack base.

A layer below with no stored `branch` is `size_branch_unknown` or `land_branch_unknown`, and the message names that layer.
`verify --commit` uses the layer base for the patch id of a layer (see [Patch id carry-forward](#patch-id-carry-forward)).

### Landing a stack

A stack lands only when the parent's delivery mode is `pr`. Any other parent mode is `land_stack_mode`.
The delivery modes of the layers have no effect.
`--branch` and `--mode` are invalid input for a stack parent or a layer, because the records store both.
The parent must be `verified`. It moves to `landed` from `verified`, as each task does.

Each `task land` on a stack parent or a layer does these steps:

1. **Reconcile.** The run reads the pull request of each layer that has a `pr_url`, one time for each layer.
   It records each layer that GitHub reports merged and that is not `landed`.
   This includes a merge made with the GitHub button.
   The reconcile always runs first, with or without `--merge`, also for a `landed` parent.
2. **Push.** `task land --id <parent>` checks every layer and pushes the layer branches to origin, bottom first.
3. **Open.** The same run opens one pull request for each layer, from the layer branch into the layer base.
4. **Link.** The same run forms one native GitHub stack from the layer pull requests, then stops at `review`.
5. **Merge.** `task land --id <parent> --merge` requests the GitHub merge of the top layer, on the owner's word.
   `task land --id <layer> --merge` requests the merge of the unlanded layers up to that layer.

**Push.** Each layer must be `verified` with a `verified_commit` (`land_stack_layer_unverified`, which names the layer).
The run reads `git ls-remote --heads origin`, then runs `git fetch` only for the layer branches that exist on origin.
It runs the local guards for every layer before the first push.
The target check of a layer runs after the push of the layer below, because that branch is its target.
Each push is `git push origin <commit>:refs/heads/<branch>`, without force.
Each layer stores its `branch`. Each layer and the parent store `land_step` `push`.

**Open and link.** The run finds or opens the pull request of each layer and stores its `pr_url`.
A layer with a stored `pr_url` uses that pull request and never opens another.
The current head of a layer with a pull request is the head of that pull request on GitHub.
Its patch id base is the base of that pull request.
GitHub rebases the upper layers after a lower merge, and an equal patch id lets such a head pass with no push.
The run checks each pull request with the rules of the `checks` step in [Landing](#landing).
Then it reads each layer pull request with `GET repos/{owner}/{repo}/pulls/{n}`:

- No layer pull request is in a stack: the run sends `POST repos/{owner}/{repo}/stacks` with the pull request numbers, bottom first.
  GitHub takes the stack base from the bottom pull request, which targets the stack base.
- All layer pull requests are in one stack of the layer count, each at its layer number: the run sends nothing.
- Any other state, or an HTTP 422 answer to the POST, is `land_stack_mismatch`. The message names each pull request with its stack and position.

Every layer and the parent then store `land_step` `review`.
The JSON result has `review_pending` true, a `layers` list (`id`, `branch`, `head`, `pr_url`, `land_step`), and the GitHub `stack` number.
Fledge never uses the `gh stack` extension.

**Merge.** Each layer from the lowest unlanded layer up to the selected layer must be at `review` with a `pr_url` (`land_stack_not_open`).
Each of them must be verified at its current head, with green checks.
The run sends `PUT repos/{owner}/{repo}/pulls/{n}/merge-async` with `merge_method=merge` and the current head of the selected layer.
The selected layer stores the `uuid` of a pending answer as `merge_request`.
The run then polls `GET repos/{owner}/{repo}/pulls/{n}/merge-async/{uuid}` every 2 seconds for at most 120 seconds:

- `pending` or `enqueued` at the end is `land_merge_pending`. The `uuid` stays stored.
  The next `task land` on the parent or on any layer continues the poll and sends no new request.
- `failed` is `land_merge_failed` with GitHub's message, and the run clears the `uuid`.
- `merged` runs the reconcile again. Each merged layer gets the one merge commit of the stack merge.
- An HTTP 404 answer clears the `uuid` and reads the pull request again. This rule is an assumption, because no probe saw the answer to an expired `uuid`.

A pull request that still reads open after a `merged` answer gets 3 more reads, 2 seconds apart.
Then the refusal is `land_unconfirmed`, and the next `task land` records the merge.

**Records.** The reconcile writes in one locked step, bottom first, after every check passes.
A merged layer at another head with another patch id is `land_pr_merged_other_head`. A closed layer pull request is `land_pr_closed`.
A refusal for one layer gives no write on any layer.
Each merged layer gets `landed`, the merge commit of its pull request, `target_branch` set to the stack base, and `land_step` `confirm`.
When every layer is `landed`, the parent gets `landed` with the merge commit of the top layer.
When only some layers are `landed`, the parent stays `verified` at `review`.
A failed write is `land_record_incomplete`. The message names the written records and the command to run again.
A run with no change on GitHub writes nothing and exits 0.
After a reconcile that writes a layer, `land` runs `git fetch origin <stack base>`, so that `agent cleanup` sees the merge commit. A failed `git fetch` is a warning.
The text output then names the recorded layers, for example `Recorded a GitHub merge of layers 1 to 2.`

**Layer runs.** `task land --id <layer>` before the parent reached `review` is `land_stack_not_open`, and the message names `fledge task land --id <parent>`.
Without `--merge`, a layer never lands alone.
A `landed` parent with a layer that is not `landed` is `task_invalid_state`.

Nothing in `task land` or `agent cleanup` deletes a branch, because a deleted lower branch closes every upper pull request.
`agent cleanup` judges the checkout of a layer by the merge commit of its task (see [Cleanup](agents.md#cleanup)).

### Features

A `feature` parent collects the members of one feature on one feature branch.

- A `feature` parent must have a `branch`, in a proposal and in `create --shape feature`.
- A member without a delivery mode gets `local`.
- A member without a base branch gets the parent's `branch`.
- Another explicit mode of a member is `task_feature_member_mode` in `create`, and invalid input in `import`.
- Another explicit base branch of a member is `task_feature_member_base` in `create`, and invalid input in `import`.

A member thus lands with a local merge into the feature branch, in the checkout that has the feature branch checked out (see [Landing](#landing)).
The parent keeps its own delivery mode.
It lands as a pull request from its `branch` into its target, with the `pr` steps of [Landing](#landing).
It needs a `verified_commit` at the head of the feature branch.

No command refreshes a feature branch from the dev branch.
To refresh it, run `git merge <dev>` by hand in the feature checkout.
Then verify the parent again with `fledge task verify --id <parent> --commit HEAD`.

## Size

`task size --id TASK [--json]` reports how large a task's branch is for review, as `task land` measures it.
It does not contact Herdr.
It writes only a carry-forward (see [Patch id carry-forward](#patch-id-carry-forward)).

The diff is `<base>...<head>`, from the merge base of the two:

- `head` is the head of the task's branch.
  The branch is found as in [Landing](#landing) (`size_branch_unknown`).
  A task without a `verified_commit` whose branch does not exist is `size_branch_missing`.
  A task with a `verified_commit` uses that commit in three cases:
  the branch is at it, the branch does not exist, or the task is not `verified`.
  For a `verified` task, a branch head with the verified patch id gets a carry-forward. A branch head with another patch id is `land_head_mismatch`.
- The target is the task's `base_branch`, else the integration branch (`size_target_missing`, `size_target_invalid`).
  For a stack layer, the target is the [layer base](#the-layer-base).
- `base` is `refs/heads/<target>` for modes `local` and `none`, and `refs/remotes/origin/<target>` for `pr` and `auto`.
  When that ref is missing, the other one is used and the output says so (`base_missing` in JSON).
  When both are missing, the refusal is `size_target_missing`.

A layer thus counts only its own lines, not the lines of the layers below it.
For a stack parent, the diff is `<stack base>...<head of the top layer>`, so the report shows the total of the stack.
A `feature` parent measures its own `branch`, as any other task does.

The measures follow [Review units](../plan/review-units.md):

- **source lines**: added plus deleted lines of hand-written source: non-test Go, built-in profiles (`internal/lib/profiles/builtin/*.md`), `.github/scripts/*.py` except `*_test.py`, and `.github/workflows/*.yml`; files under any `testdata/` directory are excluded.
- **files**: changed files, except files under a `testdata/` directory and Herdr raw captures (`reference/herdr/raw/`).
- **code domains**: domains of changed source and test files; fixtures under a `testdata/` directory add no domain.
  A domain is a command (`cmd/<name>/` with `internal/<name>/`), each `internal/lib/<name>/`, the command root (`cmd/`), the module root, or `.github/`.
  Documentation areas (`reference/<area>/`) are listed in JSON as `doc_areas` but not counted.
- **total read**: added plus deleted lines of every changed file.

Modes `local` and `none` compare with the task limits, and `pr` and `auto` with the pull request target and ceiling:

| Measure | Task limit | Pull request target | Pull request ceiling |
| --- | --- | --- | --- |
| Source lines | 300 | 200 | 300 |
| Files | 20 | 20 | 30 |
| Code domains | 3 | 3 | 5 |
| Total read | 750 | 750 | 750 |

Each measure has a status: `within`, `over target` (over the target, within the ceiling), or `over ceiling`.
A measure with one limit is `within` or `over ceiling`.
The text output names the branch, target, mode and limits, base, and head, then one line per measure and the code domains.
The JSON result holds `branch`, `target`, `base`, `base_missing`, `head`, `mode`, and `size` (`kind`, one `value`/`target`/`ceiling`/`status` object per measure, `domains`, and `doc_areas`).
An over-limit size never makes the command fail.

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
shape = "stack"              # optional: stack or feature
delivery_mode = "pr"         # optional: the parent's own mode

[[tasks]]
key = "brief-lib"            # names the task within this file
title = "Add brief template validation"
brief = '''
## Objective
...
'''
after = []                   # local keys or existing 8-hex task ids
delivery_mode = "local"      # optional: local, pr, auto, or none
base_branch = "dev"          # optional: the branch it lands into
branch = "feat/brief-lib"    # optional: the branch of the task

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
The `[parent]` table also takes `shape`, `branch`, `base_branch`, and `delivery_mode` (see [Stacks and features](#stacks-and-features)).
A `[parent]` with `shape = "stack"` needs tasks that form one `after` chain.
A `[parent]` with `shape = "feature"` needs a `branch`, and its tasks get the member defaults.

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
It must be neither verified, landed, nor cancelled, and it conflicts with a `[parent]` in the file.

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
