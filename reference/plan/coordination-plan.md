# Fledge coordination plan: output, waiting, identity, tasks, worktrees

**File:** `reference/plan/coordination-plan.md`

**Status:** Agreed 2026-09-21; not implemented. Independently reviewed before agreement; review findings are folded in.

**Review status:** The design review was performed by a pi agent on `openai-codex/gpt-6-astra`. Its report is not checked in; its findings are incorporated below.

## 1. Goal

Make Fledge able to run a delegation loop end to end: send a worker an attributable brief, wait for it, read what it produced, know who spawned whom, keep task ownership and outcomes after panes close, and clean up worktrees safely.

## 2. Backlog coverage

Item numbers refer to [enhancement ideas](enhancement-ideas.md); friction entries refer to the [dogfooding friction log](../dogfood/friction.md).

- **Fully covered:** items 2 (read output), 3 (wait), 5 (durable tasks), 11 (parent/child), 30 (adopt), and the friction entries "Delivered messages carry no sender" and "Workers cannot reply to the agent that messaged them".
- **Partially covered:** item 6 (completion report is free text), item 7 (verified state without recorded checks), item 12 (cleanup without automatic agent retirement), item 52 (inventory shows one owning agent, no owning task).

## 3. Stage A: stateless commands

### Message header

Every body sent by `agent message`, and the first prompt sent by `spawn --prompt/--file`, is prefixed with one line:

```text
ᛉ fledge message from <sender-name> (<sender-pane>) · id m-XXXX · reply: fledge agent message --name <sender-name>
```

- The glyph is U+16C9. It marks the line as Fledge-generated; a human at a keyboard never types it.
- The sender resolves from `HERDR_PANE_ID` through `pane.current` then `agent.get`. A named agent gets the full line. An unnamed agent or non-agent pane gets `from pane <id>` and no reply command. A resolution failure is reported as unknown, never relabelled as human.
- The message id is a short random token for correlation only. No durable reply route exists in Stage A.
- Empty-body validation runs before the header is added.

### `agent read`

```text
fledge agent read --name|--pane [--source visible|recent|recent-unwrapped|detection] [--lines N] [--json]
```

- Wraps Herdr `agent.read`. The hyphenated source maps to the wire spelling. The default source is `recent-unwrapped`.
- Output is labelled as a terminal snapshot. JSON carries source, revision, and truncated. The 1000-row cap is documented. No transcript reading.

### `agent wait`

```text
fledge agent wait --name|--pane ... [--until STATE ...] [--timeout D] [--all|--any] [--json]
```

- A single target wraps Herdr `agent.wait`. The result is decoded as the measured `agent_info` shape.
- An omitted `--timeout` waits indefinitely. This needs a no-deadline transport mode in `internal/lib/herdr` that still honours context cancellation (today every call is capped at 15 s).
- Repeated targets fan out one Herdr call each in parallel. `--all` collects every result and reports per-target outcome, failing if any target errored. `--any` succeeds on the first requested-state match, continues past individual errors while targets remain, then cancels the rest. `agent_not_running` is reported as an error for that target.
- Docs state that a settled state is not proof the assigned work succeeded.

## 4. Stage B: state, identity, tasks

### State store

- Location: `.fledge/state/` under the primary checkout, resolved through git so it works from linked worktrees and offline. One JSON file per record, written atomically via a temp file plus rename.
- A repo-wide lock file is held only around each read-modify-write, never across a Herdr call.
- Ids are 8 hex characters created with exclusive-create; collisions retry.
- A new shared helper writes `.fledge/.gitignore` containing `*`: append-only, with the regular-file and real-directory checks preserved from the current spawn code, verified with `git check-ignore`. The existing `.fledge/worktrees/.gitignore` is left alone.

### Agent identity and adopt

- Record: id, name, pane, harness, Herdr session, terminal id, parent id or null, spawned/adopted time, worktree path, ended time or null.
- Spawn registers the agent through the same step adopt uses. The parent is the caller when the caller's pane is a registered agent.
- `fledge agent adopt --name X` from inside a pane, or `--pane P --name X` from outside. It names an unnamed live agent via `agent.rename`, mints the id, and records the parent.
- Commands accept `--name`, `--pane`, or `--id`. An `--id` action verifies that the live pane's terminal id matches the record and fails closed otherwise.
- Records are never deleted; ended agents keep their record with the ended time set.
- Spawn outcomes distinguish record written, agent started, and prompt delivered; a failure after any of these reports what exists.

### Spawn picker

- Runs only when `agent spawn` has no flags or native args and stdin and stdout are TTYs.
- Prompts for harness, model (from `internal/lib/models`, filtered by harness, with a harness-default choice; works when discovery is empty), name, and placement: new tab, split, or new worktree.
- Cancel or EOF makes no change. Otherwise it then runs the ordinary spawn path.

### Tasks

- Fields: id, title, brief, owner agent id, status, result text, verifier id, verification note, timestamps.
- States: `created`, `assigned`, `completed`, `verified`, `cancelled`.
- Commands: `task create --title [--body|--file]`, `task assign --id --name|--pane|--id`, `task complete --id --summary`, `task verify --id [--summary]`, `task cancel --id`, `task list`, `task get --id`.
- Assign records the owner, then delivers the brief as a message with the header. Assignment and delivery are recorded separately; a delivery failure leaves the task assigned with delivery marked failed, and uncertain deliveries are not retried automatically.
- Verify by the task's own owner is refused unless `--force`; the verifier and any override are recorded. This is a workflow guard, not a security boundary.
- Task completion is never derived from Herdr idle/done.

## 5. Stage C: worktree commands

### Shared library

- Spawn's worktree creation, managed path rules, and ignore handling move to `internal/lib/worktree`. Spawn and `worktree create` both call it.
- Fix carried in: when invoked from a linked checkout, the create/open source sent to Herdr becomes the resolved primary checkout, not the linked cwd, which Herdr rejects with `linked_worktree_source` ([worktree API](../herdr/api/worktree.md)). Add a contract test for the linked case.

### `worktree list`

- Columns: path, branch, open workspace, owning agent from the state store, dirty (including untracked), merged into the repository default branch, or unknown when git cannot answer.

### `worktree remove`

- Open checkouts go through Herdr `worktree.remove`; closed ones through `git worktree remove`. The primary checkout is never a target.
- Refuses dirty or unmerged checkouts unless `--force`. Merged means the branch head is an ancestor of the default branch; squash merges therefore need `--force`.
- Always refuses while any live agent in the connected Herdr session has a pane in that workspace, rechecked immediately before removal. `--force` never bypasses this. Other Herdr sessions and direct Herdr actions are outside this guard, and the docs say so.
- The branch is kept, matching Herdr.

### `worktree create`

- `fledge worktree create --branch B [--base REF]` creates a managed checkout and opens it, with no agent.

## 6. Delivery order

- **Wave 1**, parallel, each on its own branch off `dev`: plan doc plus friction entries; message header; read and wait; state store and lock lib; worktree lib extraction with the linked-source fix.
- **Wave 2**, after the store and worktree lib merge: identity and adopt, then tasks and picker in parallel, then worktree commands.
- Every branch gets an independent verifier before merge. Merges within a wave happen one at a time, with tests rerun after each.

## 7. Verification expectations

- Test-first per [AGENTS.md](../../AGENTS.md); the [Git-aware formatting check](../../README.md#development), `go vet ./...`, and `go test -race ./...` before any branch is called done.
- Stage A live check: deliver a header to a Claude and a pi worker and confirm the glyph renders and the worker can reply by the printed command.
- Wait: a test that runs past the old 15 s transport cap; fan-out tests for one target erroring, one timing out, and `--any` with an early error.
- Store: a test with two competing writers on one record and one on id creation.
- Worktree lib: a contract test that a linked source is rewritten to the primary, and one live check in a scratch repository.

## 8. Friction entries logged

Both are recorded in the [dogfooding friction log](../dogfood/friction.md):

1. `spawn --file` on the pi harness reported the agent idle, then failed the prompt with `agent_not_ready`; a following `agent message` succeeded. A recurrence of the entry marked resolved 2026-09-19. Observed 2026-09-21 on `dccfdf1`, Herdr 0.9.1.
2. Spawn from a linked worktree sends the linked cwd as the `worktree.create` source, which the Herdr docs reject; the unit test scripts a success for that request. Suspected from code and docs, not yet reproduced live.

## 9. Open items not in this plan

Progress updates on tasks, recorded verification checks, transcript reading, an events stream, automatic agent retirement on cleanup, and per-worktree integration targets.
