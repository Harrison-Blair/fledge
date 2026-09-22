# Fledge enhancement ideas

**The logical next step is to make Fledge manage work from assignment through verified results.** From an agent's perspective, the biggest benefits would be knowing who owns a task, whether they actually started it, where their result is, and how to recover when something gets interrupted.

Fledge already has the foundation: spawning (including an initial assignment), listing, inspecting, messaging, stopping, model discovery, and worktree placement. The `doctor` command also diagnoses the environment. Its current messaging command confirms submission, while stopping closes the agent's pane. Those leave several useful gaps to fill. [Current behavior](../../README.md#agents)

The suggested division of responsibilities is to let **Herdr own the live terminals and agent processes**, while **Fledge owns assignments, dependencies, coordination, and evidence of completion**. One important distinction: Herdr's `idle` and `done` states describe readiness and whether output has been seen; neither proves that an assigned task succeeded. [Herdr agent semantics](https://herdr.dev/docs/agent-automation/)

This checklist tracks **110 enhancement ideas**, starting with the first priorities, followed by broader capabilities and more experimental possibilities. Command names for unchecked items are illustrative proposals. Some ideas build on existing Herdr APIs; others would require persistent Fledge state, a running coordinator, or support from individual harnesses. This remains an ideas backlog, not a committed roadmap.

A checked item means its main capability is implemented; accompanying notes record implementation decisions and any remaining gaps in the original idea. Completed items stay in their original categories. When checking an item, add its implementation commit, author, author date from Git, and a short implementation note. Record the rationale where documented, without guessing why a choice was made.

## First priorities

1. [x] **Inspect one agent.** An `agent get` command showing its harness, task, working directory, branch, readiness, current state, and last activity.

   **Implemented:** [7211dce](https://github.com/Harrison-Blair/fledge/commit/7211dce8c5d7654733929dfa40692d8e35e264d0) · **Author:** Harrison-Blair · **Author date:** 2026-09-18

   **Implementation decisions and remaining gaps:** `agent get` targets one live agent by `--name` or `--pane`, with text or JSON output. Inspection does not focus the pane or mark output seen. It reports harness, working directory, readiness, current state, and native session details; task, branch, and last-activity fields from the original idea remain unimplemented. [Current behavior](../../README.md#agents)

2. [x] **Read an agent's output.** An `agent read` command so a coordinator can inspect progress and answers without leaving Fledge. Distinguish terminal snapshots from complete conversation records.

   **Implemented:** [f4eb51c](https://github.com/Harrison-Blair/fledge/commit/f4eb51c1151a12beaa061dc2a5ccb374b56d8a5e) · **Author:** Harrison-Blair · **Author date:** 2026-09-22

   **Implementation decisions and remaining gaps:** `agent read` prints a plain-text terminal snapshot of one live agent's pane (`--source visible`, `recent`, `recent-unwrapped` default, or `detection`; `--lines N`), without focusing the pane or marking output seen. JSON adds `source`, `lines`, `text`, `revision`, and `truncated`. A snapshot is the terminal's current contents, not a conversation transcript, so the original idea's distinction from complete conversation records remains unimplemented. [Current behavior](../../README.md#agents)

3. [x] **Wait for agents.** An `agent wait` command with timeouts, specific states, and "any worker" or "all workers" options. This removes repeated manual polling.

   **Implemented:** [f4eb51c](https://github.com/Harrison-Blair/fledge/commit/f4eb51c1151a12beaa061dc2a5ccb374b56d8a5e) · **Author:** Harrison-Blair · **Author date:** 2026-09-22

   **Implementation decisions and remaining gaps:** `agent wait` blocks until agents reach a lifecycle state. Repeatable `--until` chooses among `idle`, `working`, `blocked`, `done`, and `unknown` (default: `idle`/`done`/`blocked`); `--timeout` bounds the wait, otherwise it is indefinite. `--name`/`--pane` are repeatable and mixable; a single target waits directly, two or more require `--all` or `--any`. A settled state means the agent's turn ended, not that its assigned work succeeded. [Current behavior](../../README.md#agents)

4. [ ] **Watch activity as it happens.** An event stream for agent starts, state changes, exits, and blockers, with readable output for people and streaming JSON for automation.

5. [x] **Create durable tasks.** Give each assignment an ID, description, owner, and status that survives the worker exiting or its pane closing.

   **Implemented:** [522bb32](https://github.com/Harrison-Blair/fledge/commit/522bb32ad76a377dd2fffb5cdf9982d8fe6fa798) · **Author:** Harrison-Blair · **Author date:** 2026-09-22

   **Implementation decisions and remaining gaps:** `task create` stores a durable record (`id`, `title`, `brief`, `owner`, `status`, and timestamps) in `.fledge/state/tasks/`, alongside agent records, so it survives the owner's pane closing. Status moves `created` → `assigned` → `completed` → `verified`, with `task cancel` ending it early; other out-of-order transitions fail. There are no progress updates. [Current behavior](../../README.md#tasks)

6. [x] **Report task completion explicitly.** Let workers submit a result tied to a task: what changed, what remains unresolved, and where the deliverables are.

   **Implemented:** [522bb32](https://github.com/Harrison-Blair/fledge/commit/522bb32ad76a377dd2fffb5cdf9982d8fe6fa798) (creator notification added in [a61f7ae](https://github.com/Harrison-Blair/fledge/commit/a61f7ae8879b59adfab892c8ac62483ae75a530e)) · **Author:** Harrison-Blair · **Author date:** 2026-09-22

   **Implementation decisions and remaining gaps:** `task complete --id TASK` (`--summary` or `--file`) requires the owning agent's live record (`--force` overrides) and stores a free-text result on the task. It then notifies the distinct registered creator with the task ID, title, result, and verification command, and records the delivery outcome; an unregistered or self-completing creator needs no notification. The result is one free-text field, not a structured breakdown of what changed, what remains unresolved, and where deliverables are. [Current behavior](../../README.md#tasks)

7. [ ] **Verify task results.** Associate checks with a task and record their actual outcomes. Distinguish "worker reported complete" from "verification passed" and "review accepted."

   **Existing support:** `task verify --id TASK [--summary TEXT]` requires a `completed` task and a registered caller other than the owner, moving it to `verified` and so distinguishing that from merely `completed`. `--force` overrides both checks and records `forced: true`. There are no recorded checks, so associating specific checks with a task and recording their individual outcomes remains open.

8. [x] **Launch with an assignment.** One operation to start an agent and deliver its task, reporting which steps succeeded if launch or delivery fails.

   **Implemented:** [180abb1](https://github.com/Harrison-Blair/fledge/commit/180abb1b6b23fb29b55f240855fbc752e27abcf6) · **Author:** Harrison-Blair · **Author date:** 2026-09-19

   **Implementation decisions and remaining gaps:** `agent spawn --prompt TEXT` or `--file PATH|-` submits the first prompt after readiness. Input is validated before resources are created, and these flags cannot be combined with `--no-wait`. Outcomes report the failing phase and known resource effects if launch or delivery fails. Success confirms prompt submission, not completion of the assigned work; durable task tracking and result verification remain separate ideas. [Current behavior](../../README.md#agents)

9. [ ] **Show everything needing attention.** A single queue of approval dialogs, unanswered questions, failed checks, and stalled assignments, each linked to the relevant agent.

10. [x] **Interrupt without destroying the pane.** Stop the current turn where the harness supports it, preserve the conversation, and allow revised instructions. Today's `stop` closes the pane.

    **Implemented:** [0ab6108](https://github.com/Harrison-Blair/fledge/commit/0ab6108344446d1820375d8bcf6e44d7ca061f15) · **Author:** Harrison-Blair · **Author date:** 2026-09-21

    **Implementation decisions and remaining gaps:** `agent pause` sends one harness-specific default interrupt sequence to the resolved pane, optionally waits for idle/done on the same terminal, and reports delivery separately from settlement. It preserves the pane and conversation; revised instructions use `agent message`. It does not freeze processes, undo work, drain queued prompts, or provide a persistent pause. Unknown harnesses and blocked/unknown/launch-pending agents are refused. Untested mappings remain best effort. [Current behavior](../../README.md#agents)

11. [x] **Track who spawned whom.** Record parent agents, child workers, task ownership, and group membership. A coordinator should easily find every worker it owns.

    **Implemented:** [ff7c96c](https://github.com/Harrison-Blair/fledge/commit/ff7c96c46f137256b38c1bd0032e36181c0fda6c) (parent recording itself landed earlier, in [75ec269](https://github.com/Harrison-Blair/fledge/commit/75ec26936e8eb964f16dbe627db028d6f493cca3) "register agent identity on spawn and add agent adopt") · **Author:** Harrison-Blair · **Author date:** 2026-09-22

    **Implementation decisions and remaining gaps:** A record's `parent` is the caller's live record at spawn or adopt time, or null when the caller is unregistered. `agent list` adds `PARENT` and `--parent ID` (direct children of that ID) and `--mine` (the caller's own direct children; mutually exclusive with `--parent`, and `caller_unregistered` for an unregistered caller). Task ownership is tracked on tasks (`owner`), not as a lineage field; there is no group membership and no recursive subtree listing, and the parent name shown is the name recorded on the parent's record at spawn/adopt time, which can differ from its current live name. [Current behavior](../../README.md#identity)

12. [ ] **Clean up completed work safely.** Retire owned agents and resources after results are collected, while identifying worktrees with uncommitted or unmerged changes.

13. [x] **Diagnose the environment.** A `doctor` command checking Herdr connectivity, compatibility, harness installations, model discovery, and relevant configuration.

    **Implemented:** [5ff1f37](https://github.com/Harrison-Blair/fledge/commit/5ff1f37a8218aa8b114bac79fb280d5a8df714e4) · **Author:** Harrison-Blair · **Author date:** 2026-09-20

    **Implementation decisions and remaining gaps:** `doctor` checks Herdr connectivity, protocol compatibility, harness installations, local model discovery, and configuration. A protocol mismatch is a warning, not a failure. Model discovery reads only local caches for `pi`, `codex`, and `claude` so diagnostics stay read-only; command-based discovery for `opencode` and `cursor` remains available through `agent models`. Reports support text, `--verbose`, and JSON output. [Current behavior](../../README.md#doctor)

14. [ ] **Save reusable agent profiles.** Named configurations such as `reviewer`, `implementer`, and `researcher`, containing the harness, model, launch arguments, and role instructions.

15. [ ] **Put a task board inside Herdr.** Show tasks, workers, blockers, and results together, with actions to inspect or focus them. Herdr's plugin API already provides panes, actions, and event hooks that could support this. [Herdr plugin API](https://herdr.dev/docs/socket-api/)

## Task management

16. [ ] **A queue with exclusive task claims.** Idle workers claim eligible work, with coordination that prevents two workers accidentally accepting the same assignment.

17. [x] **Task dependencies.** Express "implement after research" or "review after tests pass," then expose which tasks are ready to run.

    **Implemented:** [5a4719f](https://github.com/Harrison-Blair/fledge/commit/5a4719fe3ca885d8035f1334bce50f98f9e3d6fe) · **Author:** Harrison-Blair · **Author date:** 2026-09-22

    **Implementation decisions and remaining gaps:** Prerequisites are declared with repeatable `task create --after ID` and changed with `task depend --id ID --after ID --remove ID`, which rejects unknown ids, verified or cancelled tasks, and cycles (including a task after itself), naming the chain; all of these run under the state store lock. A prerequisite is satisfied when `verified`; a `cancelled` one also counts as satisfied but stays listed with its state and reason. `task assign` refuses unmet prerequisites with `task_dependencies_unmet` unless `--force`, which records them as `unmet_at_assign` rather than reusing verify's `forced`. `task list --ready` shows created tasks with every prerequisite satisfied, `list` adds a `WAITING` column, `get` shows each prerequisite's state, and `task cancel` names the created tasks it left ready. Only task verification is modelled: "review after tests pass" can be expressed only as a task for the tests, not as a recorded check. `task verify` does not report the tasks it unblocks, and there is no automatic assignment of ready tasks. [Current behavior](../../README.md#tasks)

18. [x] **Parent tasks and subtasks.** Break a larger goal into bounded pieces while preserving a useful overall progress view.

    **Implemented:** [2804518](https://github.com/Harrison-Blair/fledge/commit/2804518bbb1b9c7728dc027efb4380218a3302ac) · **Author:** Harrison-Blair · **Author date:** 2026-09-22

    **Implementation decisions and remaining gaps:** `task create --parent ID` fixes a parent at creation; there is no re-parenting, and a verified or cancelled parent cannot take new subtasks. Nesting is unlimited and acyclic by construction. The parent keeps its own lifecycle, and subtasks are not its prerequisites. `task list` and `task get` show the progress of direct subtasks as `2/3 verified, 1 cancelled` (cancelled subtasks are excluded from the total), `list` adds a `PARENT` column, and `task list --parent ID` lists direct subtasks. `task verify` refuses a parent with open subtasks unless `--force`, which is recorded and lists them; cancelling a parent does not cascade. Progress counts direct subtasks only; there is no recursive tree view or rolled-up progress across deeper levels. [Current behavior](../../README.md#tasks)

19. [ ] **Priorities and deadlines.** Let urgent work move ahead of routine work, and show when a deadline is threatened by unresolved dependencies.

20. [ ] **Separate task attempts.** Preserve each attempt's worker, outcome, and artifacts when a task fails or is retried.

21. [ ] **Task brief templates.** Standardize the information workers need: objective, constraints, allowed scope, deliverables, and acceptance criteria.

22. [ ] **Cancellation that reaches dependent work.** Cancel an assignment and identify which children or downstream tasks should stop, continue, or require reconsideration.

23. [ ] **Pause and drain a queue.** Stop dispatching new work while allowing current tasks to finish—a useful distinction from interrupting everything.

24. [ ] **Reassign an existing task.** Transfer responsibility to another worker while retaining the task's history, decisions, and previous attempts.

25. [ ] **Recover after coordinator interruption.** Reopen a project and reconstruct what was assigned, what is still running, and what requires reconciliation before continuing.

26. [ ] **Recognize duplicate assignments.** Warn when an issue, change request, or equivalent task is already being worked on.

27. [ ] **Store task artifacts.** Give reports, patches, screenshots, logs, and generated files a durable home associated with the task.

## Agent management

28. [x] **Discover the caller's identity.** An `agent current` command telling a worker its own identity, parent, assignment, and workspace.

    **Implemented:** [ff7c96c](https://github.com/Harrison-Blair/fledge/commit/ff7c96c46f137256b38c1bd0032e36181c0fda6c) · **Author:** Harrison-Blair · **Author date:** 2026-09-22

    **Implementation decisions and remaining gaps:** `agent current` shows the caller's own live record: ID, name, pane, workspace, harness, worktree path, parent ID and the parent's recorded name, plus the tasks it owns in the `assigned` state. It fails with `caller_unregistered` when the caller's pane hosts no registered agent. Workspace is reported as an id, not a resolved name, and there is no list of the caller's own children (use `agent list --mine`). [Current behavior](../../README.md#identity)

29. [ ] **Filter and select agents.** Find workers by project, owner, role, harness, task, or state, and reuse those selections in other commands.

30. [x] **Adopt manually launched agents.** Give an existing agent a Fledge identity and assignment without requiring it to be restarted.

    **Implemented:** [75ec269](https://github.com/Harrison-Blair/fledge/commit/75ec26936e8eb964f16dbe627db028d6f493cca3) · **Author:** Harrison-Blair · **Author date:** 2026-09-22

    **Implementation decisions and remaining gaps:** `agent adopt` registers an already-running agent in the caller's own pane, or in `--pane`; an unnamed agent needs `--name` (a named agent keeps its name, and a different `--name` is refused), and a terminal that already has a live record is refused with `agent_already_registered`. Adoption gives the agent a Fledge identity but does not itself deliver an assignment; use `agent message` or `task assign` afterward. [Current behavior](../../README.md#identity)

31. [ ] **Resume native conversations.** Preserve harness session references and expose resume operations where supported, with clear capability reporting.

32. [ ] **Move work between harnesses.** Transfer the task, artifacts, and a handoff summary when switching tools or models; native conversation state may not be portable.

33. [ ] **Retire an agent after its current task.** Mark a worker to finish, report its result, and exit without accepting more work.

34. [ ] **Limit resource consumption.** Cap simultaneous workers and expensive activities such as builds, browser sessions, or GPU jobs.

35. [ ] **Maintain a small reusable worker pool.** Keep selected agents ready for repeated tasks, with an explicit choice between reusing context and starting fresh.

36. [ ] **Expose harness capabilities.** Report which harnesses support resume, interruption, structured results, lifecycle hooks, and other operations.

37. [ ] **Explain model discovery.** Show where model entries came from, when they were last observed, and whether discovery failed. A cached model name should not imply confirmed availability.

38. [ ] **Measure usage per task.** Collect elapsed time, tokens, and cost where available, distinguishing measurements from estimates.

39. [ ] **Apply budgets.** Limit time, spending, or attempts per task or project. State clearly whether a limit is enforced by the runtime or merely communicated to a worker.

## Communication and shared context

40. [ ] **A durable agent mailbox.** Preserve messages until they are consumed, including messages sent while the recipient is restarting or temporarily unavailable.

41. [ ] **Meaningful delivery states.** Distinguish queued, submitted to the harness, acknowledged by the worker, and answered—where each can actually be observed.

42. [ ] **Messages linked to tasks and requests.** Make every reply attributable to the question or assignment it answers.

43. [ ] **Queue follow-up prompts.** Offer "send after the current turn" so routine follow-ups do not accidentally steer active work.

44. [ ] **Broadcast to a selected group.** Send a changed requirement to every worker on a task, with individual delivery outcomes.

45. [ ] **Generate handoff briefs.** Capture current progress, important files, decisions, failed approaches, and remaining work before transferring an assignment.

46. [ ] **Package relevant context.** Assemble the task brief, repository instructions, selected files, and supporting references into a bounded input package.

47. [ ] **Maintain a decision log.** Record settled choices so subsequent agents do not repeatedly reopen the same questions.

48. [ ] **Attach sources to shared findings.** Preserve the file, command output, test run, or document behind a claim.

49. [ ] **Send context changes incrementally.** Tell workers what changed since their last update instead of repeatedly sending the entire project history.

50. [ ] **Check result completeness.** Validate that a submitted result contains required deliverables, such as a patch, a test report, or answers to specified questions.

51. [ ] **Let workers request help.** A worker can ask for a specialist, report a dependency, or escalate uncertainty through the task system.

## Git, worktrees, and integration

52. [x] **A worktree inventory.** List managed checkouts with their branches, owning tasks, active agents, dirty state, and merge status.

    **Implemented:** [f116a62](https://github.com/Harrison-Blair/fledge/commit/f116a6245cf8897c5fe78e9b1c87022d364c6fd2) (owning agent added in [088c003](https://github.com/Harrison-Blair/fledge/commit/088c0033e56b3eb76c55fa5401fc0e65cabe5ea0)) · **Author:** Harrison-Blair · **Author date:** 2026-09-22

    **Implementation decisions and remaining gaps:** `worktree list` shows every checkout, primary first, with its branch (or detached), the open Herdr workspace, whether it is dirty (including untracked files), whether it is merged into the repository's integration branch, and whether it is managed under `.fledge/worktrees`. `OWNER` names the earliest live registered agent whose spawn created or opened that checkout, with a `+N` count for others sharing it. There is no owning-tasks column; task-to-worktree association is not tracked. [Current behavior](../../README.md#worktrees)

53. [ ] **Repeatable worktree setup.** Prepare dependencies and project configuration before a worker starts, using project-defined setup steps.

54. [ ] **Isolate development services.** Allocate separate ports, databases, and temporary directories so parallel workers can run the application independently.

55. [ ] **Declare editing ownership.** Let tasks reserve files or areas of the repository and warn about overlap. Identify advisory reservations as advisory.

56. [ ] **Detect likely conflicts early.** Compare active changes and notify coordinators when workers are converging on the same code.

57. [ ] **Inspect the diff for a task.** Show the changes associated with an assignment, anchored to its starting revision.

58. [ ] **Link commits to task history.** Preserve which assignment produced a commit and which verification results accompanied it.

59. [ ] **Checkpoint unfinished work.** Save a recoverable version of changes before interruption, handoff, or an experimental rewrite.

60. [ ] **Request independent review.** Launch a reviewer against a specific revision, with the task's acceptance criteria and previous findings.

61. [ ] **Queue integration of completed changes.** Bring approved branches together in a controlled order and verify the combined result.

62. [ ] **Support dependent branches.** Allow one task to build on another task's pending changes, while making the dependency visible.

63. [ ] **Coordinate changes across repositories.** Connect related changes and their checks without implying that multiple repository merges are atomic.

## Herdr user interface

64. [ ] **Jump directly to the relevant worker.** Commands such as "focus the agent for this task" or "show the next blocked worker."

65. [ ] **Display task metadata beside agents.** Put task IDs, roles, priorities, or verification state into Herdr's available metadata surfaces.

66. [ ] **Useful notifications.** Notify on events that require action, with grouping and deduplication when many workers finish together.

67. [ ] **Common actions in Herdr's menus.** Expose "assign task," "request review," "read result," and "retire worker" through plugin actions.

68. [ ] **Reusable workspace arrangements.** Open a familiar project layout containing implementation, review, tests, and task status.

69. [ ] **Create assignments from the current context.** Start a task from selected text, the current diff, a failure log, or an open issue.

70. [ ] **Capture a project snapshot.** Save task state, agent identities, resource references, and layout information for later inspection or supported restoration.

71. [ ] **An activity timeline.** Explain what happened across workers: assignment, start, question, response, commit, check, review, and completion.

72. [ ] **Explicit human takeover.** Mark when a person takes control of a worker and when coordination resumes, reducing conflicting inputs.

73. [ ] **Select among Herdr sessions.** Address the intended session explicitly and show where each task's workers are running.

74. [ ] **Manage remote workers.** Extend task assignment and observation across machines, including connection loss and machine availability.

75. [ ] **Links that navigate to the work.** Open a task's agent, worktree, result, or review directly from its ID wherever Herdr supports the interaction.

## Reliability and automation

76. [ ] **An operation journal.** Record spawn, delivery, interruption, and cleanup operations with their outcomes and resource identifiers.

77. [ ] **Reconcile uncertain outcomes.** Help determine whether a timed-out operation actually created an agent or submitted a prompt before attempting it again.

78. [ ] **Duplicate-resistant operations.** Use operation identities and reconciliation to reduce accidental duplicate spawns or assignments, without promising guarantees the underlying interface cannot provide.

79. [ ] **Reconnect and repair the state view.** After losing an event connection, refresh live state and identify observation gaps. Herdr's lifecycle subscriptions do not replay earlier events. [Subscription behavior](https://herdr.dev/docs/socket-api/)

80. [ ] **Detect crashes and lack of progress.** Distinguish a missing process, unavailable machine, unchanged task, and merely quiet worker before deciding how to respond.

81. [ ] **Task-specific timeout and retry policies.** Retry operations only when their outcome and repeatability are understood, and preserve failed attempts.

82. [ ] **Check API compatibility.** Discover Herdr capabilities and reject unsupported operations with actionable explanations.

    **Existing support:** `doctor` reports Herdr's protocol and capabilities and warns on a protocol mismatch. Capability-based rejection of unsupported operations remains unimplemented, so this idea stays open.

83. [ ] **Version the automation contract.** Publish stable JSON shapes, event formats, and error semantics so scripts can survive Fledge upgrades.

84. [ ] **Preview operations.** Show the intended harness invocation, resource placement, task assignment, or cleanup targets before execution.

85. [ ] **An optional persistent supervisor.** Keep queues and subscriptions active when no coordinating agent is running. Introduce it when those features justify an ongoing process.

86. [ ] **Explicit operating permissions.** Give tasks defined access to repositories, commands, and external actions, backed by actual enforcement where available.

87. [ ] **Careful handling of secrets.** Supply credentials through appropriate runtime mechanisms and avoid retaining them in task descriptions, logs, or exported artifacts.

## Integrations and extensibility

88. [ ] **Portable task export.** Export a task's brief, history, decisions, results, and artifact references in a format a person or another tool can inspect.

89. [ ] **A tool interface alongside the CLI.** Expose selected operations through an MCP server or another structured interface for agents that support direct tool calls.

90. [ ] **Hooks for task events.** Run configured actions when a task starts, becomes blocked, passes verification, or is accepted.

91. [ ] **Issue tracker connections.** Import assignments and synchronize selected status changes, comments, and result links with external trackers.

92. [ ] **CI integration.** Associate external build and test results with the exact task attempt and revision they checked.

93. [ ] **Small declarative workflows.** Describe repeatable sequences such as implement → test → review, with explicit failure and stopping conditions.

94. [ ] **Scheduled and event-triggered work.** Launch routine maintenance or investigation from a schedule or a selected repository event.

95. [ ] **A library of workflow recipes.** Provide editable starting points for bug investigation, dependency upgrades, review, documentation, and release preparation.

96. [ ] **A simulated Herdr environment.** Test coordination behavior against controlled scenarios such as disconnects, delayed starts, blockers, and partial failures.

97. [ ] **Operational metrics.** Measure completion time, blocked time, retry rate, review findings, and human interventions to see whether coordination is improving.

98. [ ] **Harness compatibility tests.** Check adapters against supported harness versions so upstream CLI changes are detected early.

99. [ ] **Portable agent instructions.** Teach different harnesses how to accept tasks, report progress, submit results, and clean up through the same Fledge workflow.

## Experimental possibilities

These ideas are worth exploring after the basic delegation loop is dependable.

100. [ ] **Assisted task decomposition.** Propose a task breakdown, dependencies, and acceptance criteria from a broad request, for review before dispatch.

101. [ ] **Routing informed by past results.** Suggest a harness, model, or role based on measured performance on similar tasks.

102. [ ] **Adjust team size during execution.** Add workers when independent work becomes available and reduce them when coordination overhead exceeds the benefit.

103. [ ] **Compare alternative implementations.** Run bounded approaches in separate worktrees and compare their correctness, complexity, and performance.

104. [ ] **Investigate disagreements between reviewers.** Turn conflicting findings into a focused follow-up task that gathers evidence and resolves the specific dispute.

105. [ ] **Track evidence behind confidence.** Show why a result appears trustworthy: reproduced behavior, passing checks, independent review, or unresolved assumptions.

106. [ ] **Detect drift from the assignment.** Compare a worker's proposed changes with the task's objective and flag scope expansion for review.

107. [ ] **Suggest the most useful next action.** Identify the dependency or decision currently delaying the most downstream work.

108. [ ] **Curate project lessons.** Preserve verified knowledge about setup, common failures, architecture, and successful approaches, with sources and ways to invalidate stale advice.

109. [ ] **Benchmark coordination strategies.** Compare one agent, specialist teams, or staged review on representative tasks using quality, time, and cost.

110. [ ] **Make the reasoning behind changes searchable.** Answer "Why does this code exist?" by following links from code and commits to tasks, decisions, evidence, and reviews.
