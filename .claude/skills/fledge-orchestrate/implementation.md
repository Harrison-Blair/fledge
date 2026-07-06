# Implementation phase

Executes approved tasks from `spec/tasks/` with an agent team: ephemeral `fledge-implementor` teammates (one per task, each in its own git worktree) paired with a small persistent pool of `fledge-reviewer` teammates. This phase runs in the main session — you are the **team lead and orchestrator**: you dispatch, gate, merge, and triage. You do not implement or review code yourself.

Communication topology is strict: each implementor talks only to its assigned reviewer and to you; reviewers talk only to their current implementor and to you. There are no other peer channels — boundary questions between tasks route through you. Teammates can technically address any teammate by name; this topology is a rule you and they enforce, not a technical limit.

Teammates inherit no conversation history — a spawn prompt is a teammate's entire context and must be fully self-contained.

## 1. Resolve scope

Map the user's request to a task set:

- "implement REQ-###" → all of that requirement's tasks.
- "implement TASK-###" (or a list) → exactly those tasks; verify every task in their `depends_on` closure is either `done` or in the set (`fledge graph --json` gives the full dependency data), and surface any that aren't before proceeding.
- bare "implement" → the ready set is `fledge ready --json` (it recomputes readiness from `depends_on` completion — the persisted `ready`/`blocked` field is only an authoring-time hint — and excludes tasks with a held lock). Confirm the set with the user.

Then gate:

- `fledge check` passes with no errors (it validates that every task exists, links to an approved requirement, has a Tests section, and that the dependency graph is sound). Fix findings before dispatching.
- Context freshness: apply the freshness gate from `planning.md` step 1 (compare `.fledge/context/index.md` commit to HEAD; ask before regenerating).
- The working tree on main is clean and the full test suite passes (see `.fledge/context/testing.md` for how). Do not dispatch onto a broken baseline.
- The task specs, requirements, and `.fledge/context/` docs are committed — worktrees are created from main and only contain committed files. If they aren't, ask the user to commit (or for permission to).
- tmux precondition: check the session is inside tmux (`test -n "$TMUX"`). If not, warn the user that split-pane teammate display is unavailable and offer a choice: stop and restart inside tmux (recommended), or proceed degraded with in-process teammates. Never silently proceed.
- Permission mode: teammates inherit your permission mode at spawn. Implementors must edit files and run tests unattended in their panes — confirm with the user that the session's mode allows this without per-action prompts (e.g. acceptEdits) before dispatching, or implementor panes will stall awaiting approvals.

## 2. Dispatch loop

Maintain the ready set continuously with `fledge ready` — a task is ready when every task in its `depends_on` is `done` and no lock is held on it. Dispatch the moment a task becomes ready — do not wait for sibling tasks ("waves" are reporting language only).

For each task dispatched:

1. **Oversight gate (during):** if the task's frontmatter has `oversight: during`, STOP and prompt the user (AskUserQuestion) to confirm they are ready to participate before spawning. Do not dispatch it until they confirm; keep dispatching other ready tasks meanwhile. Because the implementor may message only its reviewer and you, you are the user's proxy for this task: instruct the implementor in its spawn prompt to surface decision checkpoints to you rather than deciding autonomously, and relay each one to the user (AskUserQuestion) and their answer back. Without this the "during" participation never happens.
2. Create a worktree: `git worktree add <scratchpad or .fledge/worktrees>/TASK-### -b task/TASK-###-<kebab>` from main.
3. Assign a reviewer round-robin from the pool.
4. Spawn a teammate of type `fledge-implementor`, named per the naming scheme below, whose spawn prompt contains: its own name and task ID, the TASK spec path, the worktree path and branch, the assigned reviewer's name, and which `.fledge/context/` docs to read (from the task's Affected Modules citations).
5. Claim the task: `fledge lock TASK-### --owner <teammate-name> --branch task/TASK-###-<kebab>`. This atomically creates the lock (failing loudly if another dispatch already holds it) and sets the TASK file's `status: in-progress` in one step (you run fledge on main; agents never touch spec files). Track the name→task mapping in your roster.
6. Mirror into the shared team task list: create a team task titled `TASK-###: <title>`, assigned to that implementor teammate, state in-progress. You are the **sole writer** of the team task list — teammates never create, claim, or update entries. It is a visibility mirror only; spec frontmatter is the source of truth and wins on any disagreement.

**Reviewer pool:** size is `ceil(active implementors / 3)`, minimum 1. Spawn teammates of type `fledge-reviewer` (named per the scheme below) as the active implementor count crosses each multiple of 3; reviewers persist until the end of the run. A reviewer idle between review requests is normal — idle is not completion; it stays alive and addressable.

**Naming scheme:** a teammate's name is its agent-definition (purpose/template) name plus a unique identifier drawn from the 18 extant penguin species — `<agent-type>-<species>`, e.g. `fledge-implementor-adelie`, `fledge-reviewer-emperor`. The name is set at spawn and is how you and other teammates address it. One species per living teammate, shared across roles:

`emperor`, `king`, `adelie`, `chinstrap`, `gentoo`, `little`, `yellow-eyed`, `african`, `humboldt`, `magellanic`, `galapagos`, `fiordland`, `snares`, `erect-crested`, `southern-rockhopper`, `northern-rockhopper`, `royal`, `macaroni`

Assign the first unused species; a species frees for reuse only after its teammate's shutdown is confirmed. If all 18 species are in use (≥14 implementors plus their reviewer pool can exceed the list), append a numeric suffix to the first species — `fledge-implementor-adelie-2`, then `-3` — so a full pool never blocks dispatch. Report a one-line roster delta to the user whenever it changes (e.g. `+ fledge-implementor-gentoo → TASK-007`); give the full roster (name → role → task) on request. Keep the full name→task mapping internally — species reuse depends on it.

## 3. On approval

A task is approved for merge in one of two ways: its reviewer messages you a pass, or — after a reviewer's 3rd-rejection escalation (§4) — the user explicitly directs you to ship it anyway. On a user override, record the accepted (waived) findings on the TASK file before merging, so the decision is auditable. Then:

1. **Oversight gate (merge):** if the task has `oversight: merge`, hold the branch unmerged. Show the user the diff and the reviewer's verdict; merge only on their sign-off.
2. Merge the branch to main (prefer a regular merge). On conflict, have the implementor rebase its branch and re-run its tests; because the rebase produces hand-resolved changes the reviewer never saw, route the rebased diff back through the assigned reviewer for a lightweight re-check (tests pass + resolution looks right) before you merge.
3. Run the full test suite on main.
   - **Green:** run `fledge unlock TASK-### --done` (releases the lock and sets `status: done`), commit the spec update, and mark the mirrored team task completed yourself (never rely on a teammate to do it). Then remove the worktree (`git worktree remove`), delete the branch, and request graceful shutdown of the implementor teammate by name; its species frees only after shutdown is confirmed.
   - **Red:** the combination broke — the implementor is still alive and its worktree and branch survive (teardown happens only on green). Send it the failure; it fixes in its worktree and commits to the same branch. Route that fix commit through the assigned reviewer for a lightweight re-check, then merge the fix commit to main and re-run the suite. Loop until green, then proceed to the green teardown above. The fix reaches main only through this merge — never leave it stranded on the (already-merged) branch.
4. Re-evaluate the ready set and dispatch newly unblocked tasks (step 2). Shrink nothing: existing reviewers stay for the run.

## 4. Escalations

Agents will escalate blockers and disputes to you. Triage by fledge's standing rule — facts belong in the repo, decisions belong to the user:

- **Facts** (what an interface is, where code lives, what a spec sentence means when the spec is actually unambiguous): resolve yourself by reading spec/context/code, and answer the agent.
- **Decisions** (genuine spec gaps, contradictions, tradeoffs, a reviewer's 3rd-rejection escalation): surface to the user with the context they need, then relay their call.

An escalated implementor stays alive and paused; other tasks keep flowing while it waits.

## 5. End of run

When no tasks remain in the set that are unfinished and dispatchable, gracefully shut down each reviewer teammate by name, then reconcile the team task list: every team task dispatched this run should be completed — complete any stragglers yourself and note discrepancies in the report. Then report:

- tasks completed (merged, suite green) vs. blocked or escalated, with reasons
- merges performed and the final suite status on main
- any tasks newly unblocked outside the run's scope that could be implemented next

## 6. Recovery after resume

`/resume` and `/rewind` do not restore teammates — after a resume, no teammate from the transcript exists, regardless of what your notes say. To recover a run:

1. Treat all remembered teammates as gone; clear the roster.
2. Inventory reality: `git worktree list`, task branches, `fledge locks` (owner, branch, and pid-alive per held lock), and `fledge graph`. Tasks with a held lock (equivalently `status: in-progress`) and a surviving worktree are the resume set. Locks whose task has no surviving worktree are stale — release them with `fledge unlock TASK-### --force`, then set their status explicitly (`fledge status TASK-### ready --force`) so they re-enter the ready set.
3. For each, respawn a fresh implementor teammate (a new species is fine) into the **existing** worktree and branch. Its spawn prompt must say partial work may exist: inspect commits and the diff before continuing, and re-verify the test-first evidence chain — if the captured failing-test output was lost with the old teammate, re-derive it (revert/stash) or flag the gap to the reviewer.
4. Respawn the reviewer pool at the computed size and reassign round-robin.
5. Reconcile the team task list against spec frontmatter (complete or create entries as needed).
6. Report the reconstructed roster to the user before proceeding.
