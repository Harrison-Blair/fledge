# Fledge enhancement ideas

**The logical next step is to make Fledge manage work from assignment through verified results.** From an agent's perspective, the biggest benefits would be knowing who owns a task, whether they actually started it, where their result is, and how to recover when something gets interrupted.

Fledge already has the foundation: spawning, listing, messaging, stopping, model discovery, and worktree placement. Its current messaging command confirms submission, while stopping closes the agent's pane. Those leave several useful gaps to fill. [Current behavior](../../README.md#agents)

The suggested division of responsibilities is to let **Herdr own the live terminals and agent processes**, while **Fledge owns assignments, dependencies, coordination, and evidence of completion**. One important distinction: Herdr's `idle` and `done` states describe readiness and whether output has been seen; neither proves that an assigned task succeeded. [Herdr agent semantics](https://herdr.dev/docs/agent-automation/)

These **110 ideas** start with the most obvious additions, followed by broader capabilities and more experimental possibilities. Command names are illustrative proposals. Some ideas build on existing Herdr APIs; others would require persistent Fledge state, a running coordinator, or support from individual harnesses. This is a brainstorming catalog, not a committed roadmap.

## First priorities

1. **Inspect one agent.** An `agent get` command showing its harness, task, working directory, branch, readiness, current state, and last activity.

2. **Read an agent's output.** An `agent read` command so a coordinator can inspect progress and answers without leaving Fledge. Distinguish terminal snapshots from complete conversation records.

3. **Wait for agents.** An `agent wait` command with timeouts, specific states, and "any worker" or "all workers" options. This removes repeated manual polling.

4. **Watch activity as it happens.** An event stream for agent starts, state changes, exits, and blockers, with readable output for people and streaming JSON for automation.

5. **Create durable tasks.** Give each assignment an ID, description, owner, and status that survives the worker exiting or its pane closing.

6. **Report task completion explicitly.** Let workers submit a result tied to a task: what changed, what remains unresolved, and where the deliverables are.

7. **Verify task results.** Associate checks with a task and record their actual outcomes. Distinguish "worker reported complete" from "verification passed" and "review accepted."

8. **Launch with an assignment.** One operation to start an agent and deliver its task, reporting which steps succeeded if launch or delivery fails.

9. **Show everything needing attention.** A single queue of approval dialogs, unanswered questions, failed checks, and stalled assignments, each linked to the relevant agent.

10. **Interrupt without destroying the pane.** Stop the current turn where the harness supports it, preserve the conversation, and allow revised instructions. Today's `stop` closes the pane.

11. **Track who spawned whom.** Record parent agents, child workers, task ownership, and group membership. A coordinator should easily find every worker it owns.

12. **Clean up completed work safely.** Retire owned agents and resources after results are collected, while identifying worktrees with uncommitted or unmerged changes.

13. **Diagnose the environment.** A `doctor` command checking Herdr connectivity, compatibility, harness installations, model discovery, and relevant configuration.

14. **Save reusable agent profiles.** Named configurations such as `reviewer`, `implementer`, and `researcher`, containing the harness, model, launch arguments, and role instructions.

15. **Put a task board inside Herdr.** Show tasks, workers, blockers, and results together, with actions to inspect or focus them. Herdr's plugin API already provides panes, actions, and event hooks that could support this. [Herdr plugin API](https://herdr.dev/docs/socket-api/)

## Task management

16. **A queue with exclusive task claims.** Idle workers claim eligible work, with coordination that prevents two workers accidentally accepting the same assignment.

17. **Task dependencies.** Express "implement after research" or "review after tests pass," then expose which tasks are ready to run.

18. **Parent tasks and subtasks.** Break a larger goal into bounded pieces while preserving a useful overall progress view.

19. **Priorities and deadlines.** Let urgent work move ahead of routine work, and show when a deadline is threatened by unresolved dependencies.

20. **Separate task attempts.** Preserve each attempt's worker, outcome, and artifacts when a task fails or is retried.

21. **Task brief templates.** Standardize the information workers need: objective, constraints, allowed scope, deliverables, and acceptance criteria.

22. **Cancellation that reaches dependent work.** Cancel an assignment and identify which children or downstream tasks should stop, continue, or require reconsideration.

23. **Pause and drain a queue.** Stop dispatching new work while allowing current tasks to finish—a useful distinction from interrupting everything.

24. **Reassign an existing task.** Transfer responsibility to another worker while retaining the task's history, decisions, and previous attempts.

25. **Recover after coordinator interruption.** Reopen a project and reconstruct what was assigned, what is still running, and what requires reconciliation before continuing.

26. **Recognize duplicate assignments.** Warn when an issue, change request, or equivalent task is already being worked on.

27. **Store task artifacts.** Give reports, patches, screenshots, logs, and generated files a durable home associated with the task.

## Agent management

28. **Discover the caller's identity.** An `agent current` command telling a worker its own identity, parent, assignment, and workspace.

29. **Filter and select agents.** Find workers by project, owner, role, harness, task, or state, and reuse those selections in other commands.

30. **Adopt manually launched agents.** Give an existing agent a Fledge identity and assignment without requiring it to be restarted.

31. **Resume native conversations.** Preserve harness session references and expose resume operations where supported, with clear capability reporting.

32. **Move work between harnesses.** Transfer the task, artifacts, and a handoff summary when switching tools or models; native conversation state may not be portable.

33. **Retire an agent after its current task.** Mark a worker to finish, report its result, and exit without accepting more work.

34. **Limit resource consumption.** Cap simultaneous workers and expensive activities such as builds, browser sessions, or GPU jobs.

35. **Maintain a small reusable worker pool.** Keep selected agents ready for repeated tasks, with an explicit choice between reusing context and starting fresh.

36. **Expose harness capabilities.** Report which harnesses support resume, interruption, structured results, lifecycle hooks, and other operations.

37. **Explain model discovery.** Show where model entries came from, when they were last observed, and whether discovery failed. A cached model name should not imply confirmed availability.

38. **Measure usage per task.** Collect elapsed time, tokens, and cost where available, distinguishing measurements from estimates.

39. **Apply budgets.** Limit time, spending, or attempts per task or project. State clearly whether a limit is enforced by the runtime or merely communicated to a worker.

## Communication and shared context

40. **A durable agent mailbox.** Preserve messages until they are consumed, including messages sent while the recipient is restarting or temporarily unavailable.

41. **Meaningful delivery states.** Distinguish queued, submitted to the harness, acknowledged by the worker, and answered—where each can actually be observed.

42. **Messages linked to tasks and requests.** Make every reply attributable to the question or assignment it answers.

43. **Queue follow-up prompts.** Offer "send after the current turn" so routine follow-ups do not accidentally steer active work.

44. **Broadcast to a selected group.** Send a changed requirement to every worker on a task, with individual delivery outcomes.

45. **Generate handoff briefs.** Capture current progress, important files, decisions, failed approaches, and remaining work before transferring an assignment.

46. **Package relevant context.** Assemble the task brief, repository instructions, selected files, and supporting references into a bounded input package.

47. **Maintain a decision log.** Record settled choices so subsequent agents do not repeatedly reopen the same questions.

48. **Attach sources to shared findings.** Preserve the file, command output, test run, or document behind a claim.

49. **Send context changes incrementally.** Tell workers what changed since their last update instead of repeatedly sending the entire project history.

50. **Check result completeness.** Validate that a submitted result contains required deliverables, such as a patch, a test report, or answers to specified questions.

51. **Let workers request help.** A worker can ask for a specialist, report a dependency, or escalate uncertainty through the task system.

## Git, worktrees, and integration

52. **A worktree inventory.** List managed checkouts with their branches, owning tasks, active agents, dirty state, and merge status.

53. **Repeatable worktree setup.** Prepare dependencies and project configuration before a worker starts, using project-defined setup steps.

54. **Isolate development services.** Allocate separate ports, databases, and temporary directories so parallel workers can run the application independently.

55. **Declare editing ownership.** Let tasks reserve files or areas of the repository and warn about overlap. Identify advisory reservations as advisory.

56. **Detect likely conflicts early.** Compare active changes and notify coordinators when workers are converging on the same code.

57. **Inspect the diff for a task.** Show the changes associated with an assignment, anchored to its starting revision.

58. **Link commits to task history.** Preserve which assignment produced a commit and which verification results accompanied it.

59. **Checkpoint unfinished work.** Save a recoverable version of changes before interruption, handoff, or an experimental rewrite.

60. **Request independent review.** Launch a reviewer against a specific revision, with the task's acceptance criteria and previous findings.

61. **Queue integration of completed changes.** Bring approved branches together in a controlled order and verify the combined result.

62. **Support dependent branches.** Allow one task to build on another task's pending changes, while making the dependency visible.

63. **Coordinate changes across repositories.** Connect related changes and their checks without implying that multiple repository merges are atomic.

## Herdr user interface

64. **Jump directly to the relevant worker.** Commands such as "focus the agent for this task" or "show the next blocked worker."

65. **Display task metadata beside agents.** Put task IDs, roles, priorities, or verification state into Herdr's available metadata surfaces.

66. **Useful notifications.** Notify on events that require action, with grouping and deduplication when many workers finish together.

67. **Common actions in Herdr's menus.** Expose "assign task," "request review," "read result," and "retire worker" through plugin actions.

68. **Reusable workspace arrangements.** Open a familiar project layout containing implementation, review, tests, and task status.

69. **Create assignments from the current context.** Start a task from selected text, the current diff, a failure log, or an open issue.

70. **Capture a project snapshot.** Save task state, agent identities, resource references, and layout information for later inspection or supported restoration.

71. **An activity timeline.** Explain what happened across workers: assignment, start, question, response, commit, check, review, and completion.

72. **Explicit human takeover.** Mark when a person takes control of a worker and when coordination resumes, reducing conflicting inputs.

73. **Select among Herdr sessions.** Address the intended session explicitly and show where each task's workers are running.

74. **Manage remote workers.** Extend task assignment and observation across machines, including connection loss and machine availability.

75. **Links that navigate to the work.** Open a task's agent, worktree, result, or review directly from its ID wherever Herdr supports the interaction.

## Reliability and automation

76. **An operation journal.** Record spawn, delivery, interruption, and cleanup operations with their outcomes and resource identifiers.

77. **Reconcile uncertain outcomes.** Help determine whether a timed-out operation actually created an agent or submitted a prompt before attempting it again.

78. **Duplicate-resistant operations.** Use operation identities and reconciliation to reduce accidental duplicate spawns or assignments, without promising guarantees the underlying interface cannot provide.

79. **Reconnect and repair the state view.** After losing an event connection, refresh live state and identify observation gaps. Herdr's lifecycle subscriptions do not replay earlier events. [Subscription behavior](https://herdr.dev/docs/socket-api/)

80. **Detect crashes and lack of progress.** Distinguish a missing process, unavailable machine, unchanged task, and merely quiet worker before deciding how to respond.

81. **Task-specific timeout and retry policies.** Retry operations only when their outcome and repeatability are understood, and preserve failed attempts.

82. **Check API compatibility.** Discover Herdr capabilities and reject unsupported operations with actionable explanations.

83. **Version the automation contract.** Publish stable JSON shapes, event formats, and error semantics so scripts can survive Fledge upgrades.

84. **Preview operations.** Show the intended harness invocation, resource placement, task assignment, or cleanup targets before execution.

85. **An optional persistent supervisor.** Keep queues and subscriptions active when no coordinating agent is running. Introduce it when those features justify an ongoing process.

86. **Explicit operating permissions.** Give tasks defined access to repositories, commands, and external actions, backed by actual enforcement where available.

87. **Careful handling of secrets.** Supply credentials through appropriate runtime mechanisms and avoid retaining them in task descriptions, logs, or exported artifacts.

## Integrations and extensibility

88. **Portable task export.** Export a task's brief, history, decisions, results, and artifact references in a format a person or another tool can inspect.

89. **A tool interface alongside the CLI.** Expose selected operations through an MCP server or another structured interface for agents that support direct tool calls.

90. **Hooks for task events.** Run configured actions when a task starts, becomes blocked, passes verification, or is accepted.

91. **Issue tracker connections.** Import assignments and synchronize selected status changes, comments, and result links with external trackers.

92. **CI integration.** Associate external build and test results with the exact task attempt and revision they checked.

93. **Small declarative workflows.** Describe repeatable sequences such as implement → test → review, with explicit failure and stopping conditions.

94. **Scheduled and event-triggered work.** Launch routine maintenance or investigation from a schedule or a selected repository event.

95. **A library of workflow recipes.** Provide editable starting points for bug investigation, dependency upgrades, review, documentation, and release preparation.

96. **A simulated Herdr environment.** Test coordination behavior against controlled scenarios such as disconnects, delayed starts, blockers, and partial failures.

97. **Operational metrics.** Measure completion time, blocked time, retry rate, review findings, and human interventions to see whether coordination is improving.

98. **Harness compatibility tests.** Check adapters against supported harness versions so upstream CLI changes are detected early.

99. **Portable agent instructions.** Teach different harnesses how to accept tasks, report progress, submit results, and clean up through the same Fledge workflow.

## Experimental possibilities

These ideas are worth exploring after the basic delegation loop is dependable.

100. **Assisted task decomposition.** Propose a task breakdown, dependencies, and acceptance criteria from a broad request, for review before dispatch.

101. **Routing informed by past results.** Suggest a harness, model, or role based on measured performance on similar tasks.

102. **Adjust team size during execution.** Add workers when independent work becomes available and reduce them when coordination overhead exceeds the benefit.

103. **Compare alternative implementations.** Run bounded approaches in separate worktrees and compare their correctness, complexity, and performance.

104. **Investigate disagreements between reviewers.** Turn conflicting findings into a focused follow-up task that gathers evidence and resolves the specific dispute.

105. **Track evidence behind confidence.** Show why a result appears trustworthy: reproduced behavior, passing checks, independent review, or unresolved assumptions.

106. **Detect drift from the assignment.** Compare a worker's proposed changes with the task's objective and flag scope expansion for review.

107. **Suggest the most useful next action.** Identify the dependency or decision currently delaying the most downstream work.

108. **Curate project lessons.** Preserve verified knowledge about setup, common failures, architecture, and successful approaches, with sources and ways to invalidate stale advice.

109. **Benchmark coordination strategies.** Compare one agent, specialist teams, or staged review on representative tasks using quality, time, and cost.

110. **Make the reasoning behind changes searchable.** Answer "Why does this code exist?" by following links from code and commits to tasks, decisions, evidence, and reviews.
