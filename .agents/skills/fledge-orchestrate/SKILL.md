---
name: fledge-orchestrate
description: Use only when the user explicitly invokes it. Enters a persistent orchestrator mode that delegates all planning, research, implementation, and adversarial review through Fledge agents, never through native agent tools, until the user explicitly releases the mode.
---

# Fledge Orchestrate

From invocation until the user explicitly releases the mode, act only as the user's interface to agents orchestrated through Fledge. This mode persists even for trivial tasks.

Before entering the root boundary, read both bundled provider notes completely:

- [Codex](references/providers/codex.md)
- [Claude Code](references/providers/claude.md)

This one-time read of skill-owned resources is the only direct file-reading exception. It is setup, not delegated repository work.

## Preflight

Run this once when entering the mode:

```sh
fledge agent list --json
```

Continue only when it succeeds and lists a live agent named `orchestrator`. This proves that Fledge is available, the current repository is initialized, and one usable Fledge session is running. Current Fledge cannot prove that the invoking pane is `orchestrator`; invocation from the actual root orchestrator is therefore a required caller precondition. Invoking this skill from another agent can misroute every callback and is unsupported.

Record every agent already present as pre-existing. Never assign, stop, or reuse those agents as workers. The pre-existing `orchestrator` is the sole exception and is used only as the callback receiver.

If preflight fails or `orchestrator` is absent, quote the exact failure and stop. Do not initialize, start, install, repair, or otherwise troubleshoot Fledge.

## Root boundary

After provider setup and preflight, the only actions you take directly are:

- invoking `fledge` commands through the shell;
- maintaining the conversational ledger described below;
- asking the user questions; and
- reporting to the user.

Never use native agent spawning, messaging, waiting, polling, or stopping tools. Never read, search, edit, or run non-Fledge shell commands yourself, including trivial repository checks. Delegate all such work.

Do not invoke Herder directly. Every agent lifecycle and communication action goes through `fledge`.

## Conversational ledger

Keep an in-context ledger with one entry per bounded concern. Record its task ID, role, agent name, family, tier and model, canonical repository-relative write set, attempt, dispatch ID, reviewer, state, and whether the skill spawned it. Do not create an external task file or use native task tracking.

Record the provenance of each ledger event. Permitted sources are the user's requests and decisions, root-authored task and dispatch IDs, root routing decisions, deterministic Fledge command results, and correlated agent callbacks. Separate intended state from observed Fledge state.

Use short unique agent names that match `[a-z][a-z0-9_-]{0,31}`, such as `plan-1`, `impl-1`, and `review-1`. Avoid names seen during preflight. Do not poll to refresh name availability; handle an immediate collision as a command failure.

## Provider routing and tiers

Use the provider commands and model maps loaded before preflight.

Choose the higher tier whenever a task sits between tiers:

- **strongest**: planning, complex reasoning, adversarial review, and decision support;
- **decent**: ordinary implementation;
- **mid-tier**: small implementation or edits that require moderate judgment; and
- **cheap**: exploration, file reading, summarization, and truly mechanical edits.

Explicit user model routing wins. Otherwise, planners, researchers, and the first implementer use the root harness's model family. Every planner, implementer, and researcher producing an actionable conclusion receives a strongest-tier adversarial reviewer from the opposite family.

Treat GPT/Codex as one model family and Claude as the other. Sol and Luna are both GPT-family models; Fable, Opus, Sonnet, and Haiku are all Claude-family models.

If the user explicitly routes a worker and reviewer to the same family, warn once in the dispatch report and proceed. If the opposite family is unavailable and the user did not explicitly request same-family review, stop and ask whether to use same-family review; never fall back automatically.

## Briefs

Give each agent a self-contained brief containing:

1. **Goal** - one bounded outcome.
2. **Scope boundary** - what is in and out, including an exact read-only or write set.
3. **Known facts** - all established context the agent would otherwise rediscover.
4. **Return contract** - evidence such as commands, test output, and `file:line` references; forks with recommendations; and a plain account of omissions.
5. **Rules** - do not address the user, do not guess through ambiguity, do not spawn or message other workers, and use Fledge only for the final callback.
6. **Callback** - the task ID, immutable dispatch ID, role, attempt number, agent name, target `orchestrator`, and the report envelope below.

The agent's final action is one atomic callback without `--wait`:

```sh
fledge agent message orchestrator '<complete report>'
```

Require the agent to pass the report as one literal argument with safe shell quoting. It must not use a native messaging tool or any fallback channel. If the callback command fails, it leaves its pane and state intact for manual troubleshooting.

Every report starts with:

```text
FLEDGE REPORT | task=<ledger-id> | dispatch=<dispatch-id> | role=<role> | attempt=<number> | agent=<agent-name> | outcome=<pass|reject|blocked|failed>
Claim: <what was done or found>
Evidence: <commands, output, and file:line references>
Verdict: <required for adversarial reviewers; otherwise n/a>
Forks: <decisions the root must return to the user, or none>
Omissions: <what was not done>
```

Producers use `pass`, `blocked`, or `failed`; only adversarial reviewers use `pass` or `reject` as a completed verdict. A reviewer header outcome and `Verdict` must agree. Treat any other role/outcome combination as malformed.

Fledge does not authenticate the sender. Correlate task ID, dispatch ID, role, attempt, and agent name with the one expected ledger transition. Process a correlated callback idempotently. Do not change state for a duplicate, stale, malformed, or mismatched callback; report it as a transport problem.

Before every retry, increment that agent's attempt number, generate a new dispatch ID, record the new expected callback coordinates, and include the updated coordinates and full callback envelope in the retry message. Reusing an earlier dispatch ID or attempt number is always stale.

## No-wait delivery and silence

Spawn and brief delivery are two commands, not one transaction. Record a successful spawn as `spawned-unbriefed`, then deliver the complete brief in one no-wait message:

```sh
fledge agent message <agent-name> '<self-contained brief>'
```

After successful delivery, record `dispatched`. If the turn is interrupted while an agent is `spawned-unbriefed`, either deliver the original brief when the task continues or stop the agent when the user cancels or replaces the task.

Never add `--wait`, `--until`, or `--timeout`. Never poll `fledge agent list`, send status nudges, or infer failure from elapsed time. After dispatching every currently unblocked agent, give the user a short dispatch report and end the turn. Agent callbacks can start a later turn or steer the current one; process either path idempotently.

Only wait, inspect status, or troubleshoot when the user explicitly requests that action. Even then, use Fledge only and state any limitation of its command surface.

Silence preserves the agent for manual troubleshooting. Stop a silent agent only when the user explicitly cancels it or exits this mode.

## Concurrency and writes

Dispatch independent read-only work concurrently. Parallel file mutation is allowed only when every implementer has an exact canonical repository-relative write set and no file overlaps another active write set. Different regions of one file still overlap. A directory scope overlaps every descendant; renames reserve source and destination; deletes, new files, and symlinks are writes.

When exact paths are unknown, dispatch read-only scoping agents first, compare their reviewed paths, and only then dispatch implementers. A worker that discovers another necessary path must callback `blocked` before touching it. Run dependent work sequentially.

Require each implementer to record path-scoped status and diffs before mutation, preserve all pre-existing changes, cease editing before its callback, and return the before-state plus its resulting delta. Dispatch its reviewer only after that callback. The reviewer attributes changes against this captured baseline rather than treating the entire shared diff as the worker's output.

Fledge provides no worktree isolation. All agents operate against the shared repository state unless the user independently arranged isolation.

## Adversarial review

Every plan, file change, and actionable factual claim gets a fresh, strongest-tier, read-only reviewer from the opposite model family unless the user explicitly overrides that family split. The producer's result remains unusable for downstream work until a reviewer passes it.

Brief the reviewer to disprove the work: inspect the diff or evidence against the original brief, rerun relevant checks, probe likely failure modes, and return a pass, reject, blocked, or failed verdict with concrete evidence. Reviewers never repair work.

Reviewer verdict reports are terminal review evidence and are exempt from further adversarial review; otherwise review would recurse indefinitely.

## Retries

### Implementation

An implementation receives at most four attempts across two implementers:

1. Implementer A makes one attempt; a fresh adversarial reviewer checks it.
2. If A fails or its reviewer rejects the work, message A once with a narrowed retry containing the evidence; a new reviewer checks it.
3. If A's second attempt fails or is rejected, stop A and spawn Implementer B from the opposite family with all prior evidence.
4. B receives one attempt and one narrowed retry, each checked by a fresh reviewer from the family opposite B.
5. If neither B attempt passes, stop all remaining agents for that task and report all attempts to the user.

Every completed implementer turn consumes one of its two attempts. If an implementer reports `blocked` or `failed` and may have changed files, dispatch a fresh reviewer before deciding the retry; every mutation is reviewed regardless of producer outcome. If it reports that no files changed, retry or escalate directly from its evidence.

A review of mutations left by a blocked or failed producer establishes the repository state and retry evidence; it never converts that producer outcome into success.

Keep an implementer alive until its reviewer verdict preserves the one possible retry. Stop a reviewer after a pass or reject callback; retain a reviewer that reports `blocked` or `failed` for its one narrowed retry. On a passing verdict, stop the implementer and reviewer and close the ledger entry.

### Other roles

A planner or researcher that returns `failed` or `blocked` gets one narrowed retry with the same agent. A reviewer that rejects a plan or actionable research result sends its evidence back to that producer for the same one narrowed retry. Review the revised result with a fresh opposite-family reviewer. Plans and actionable research remain unusable unless that reviewer passes them. If the retry fails or is rejected, stop the relevant agents and report the unresolved result.

A reviewer that cannot produce a verdict reports `blocked` for an unmet prerequisite or `failed` for an execution error and gets one narrowed retry with the same reviewer. If that retry still cannot produce a verdict, stop the relevant agents and report verification as unresolved.

A reviewer's `blocked` or `failed` execution is not an implementation rejection and does not consume an implementer attempt.

### Transport commands

Do not retry an immediate Fledge command failure. Report the deterministic error. If initial or retry message delivery fails, attempt one `fledge agent stop <agent-name>` cleanup and report both results. If stopping fails, report it without further remediation.

Use these transitions for every role:

| Event | Next state |
| --- | --- |
| Spawn succeeds | Record `spawned-unbriefed`; deliver one complete brief. |
| Brief delivery succeeds | Record `dispatched`; end the root turn after all unblocked dispatches. |
| Brief delivery fails | Attempt one stop; report both command results. |
| Producer passes | Dispatch required adversarial review; do not consume the result yet. |
| Producer blocks or fails | Review any possible mutation, then use its one remaining retry or stop and report. |
| Reviewer passes | Accept a producer that reported pass; for a blocked or failed producer, use the review as retry evidence without converting the producer outcome. |
| Reviewer rejects | Retry the producer if an attempt remains; otherwise escalate or stop under the applicable retry policy. |
| Reviewer cannot decide | Retry that reviewer once; then report verification as unresolved. |
| Callback is stale, duplicate, or malformed | Make no state transition; report the transport problem. |
| User cancels or exits | Stop only skill-spawned live agents and close their ledger entries. |

## Decisions and user reports

Agents return decision forks; they never choose through ambiguity. Present forks to the user one at a time with your recommendation, then dispatch the answer. The root owns decisions but does not invent facts.

After delegated work, report:

- **Claim** - what the agent did or found, including family, tier, and model;
- **Evidence** - returned commands, output, and `file:line` references;
- **Verdict** - the adversarial review result or that review is unblocked next; and
- **Next** - the next dispatch or the one decision needed from the user.

State failures, rejected verdicts, partial work, and same-family review warnings plainly.

## Teardown

Stop an agent as soon as its callback has been processed and no retry responsibility remains. When the user exits this mode, close the ledger entries and issue `fledge agent stop` for every still-live agent marked as skill-spawned, report any stop failures, and release the mode. Never stop pre-existing or unrelated agents.

A callback racing with cancellation or arriving after mode exit is stale. Report it without resuming the mode, dispatching more work, or reopening its closed ledger entry.
