# Fledge and firstmate

**Date:** 2026-10-12. Compared against [firstmate](https://github.com/kunchenguid/firstmate) at its
push of 2026-10-12 and Fledge `dev` at `0b6fc49`. Firstmate moves fast; re-check its README and
`docs/architecture.md` before citing a detail from this note as current.

## Purpose

Record what firstmate does that Fledge does not, and the reverse, so planning can borrow from it
without refetching the repository. The decisions section at the end records what the owner chose
to borrow.

## What each one is

**Firstmate** is an "agent distro": a cloned repository whose `AGENTS.md`, about 30 agent-loaded
skills, and about 200 `bin/fm-*.sh` scripts turn a general coding agent into a "first mate" that
talks to the user (the "captain") and runs a crew of workers. There is no binary. Terminals come
from a pluggable backend: tmux by default, Herdr with its own CI lane, and zellij, Orca, and cmux
as experimental. Worktrees come from a sibling tool, treehouse. MIT, bash and Markdown, created
2026-06-12, about 7.8k stars at the time of writing.

**Fledge** is a tested Go CLI that speaks Herdr's Unix socket directly (protocol 22 pinned). It is a
primitive layer: agent spawn, inspect, read, wait, message, send, pause, stop, cleanup, adopt;
durable tasks with assign, complete, verify, dependencies, subtasks, and TOML proposals; managed
worktrees; repository memory; doctor; self-update. It has no persona and no supervisor loop.

Firstmate is a job description that ships tools. Fledge is a toolbox that ships role briefs.

## Firstmate has, Fledge does not

| Capability | Firstmate | Fledge backlog item |
| --- | --- | --- |
| Zero-token supervision | A bash watcher sleeps on the fleet and wakes the LLM only for actionable events; per-harness turn-end guard hooks stop the primary from ending a turn while work is in flight | #4 event stream, #85 persistent supervisor |
| Delivery modes and merge guards | Per task `no-mistakes`, `direct-PR`, or `local-only`, with an orthogonal `+yolo` merge grant; `fm-pr-merge.sh` merges only after live reads prove the PR open, non-draft, and green at the exact head, then binds the merge to that head; GitHub, GitLab, Forgejo, read-only Gerrit | #57 task diff, #60 review, #61 integration |
| Durable steering inbox | A steer becomes a durable record plus a one-line doorbell in the worker's terminal; the watcher re-rings unacknowledged messages | #40 mailbox, #41 delivery states |
| Multi-project, multi-machine | A `projects/` registry, fleet sync of clones, persistent "secondmates" in isolated homes, locally or over SSH | #74 remote workers |
| Two task shapes | `ship` changes code; `scout` writes `data/<id>/report.md` and never pushes | none |
| Quota-aware dispatch profiles | Natural-language routing rules choose harness and model, ranked by a quota probe | #101 routing |
| Captain-facing surfaces | `/bearings` digest, `/ahoy` recap of unanswered decisions, `/afk` and `/quiet` away modes, `/stow` memory curation, `/updatefirstmate`, Relay for X and Discord, voice, IMAP mail as a wake source | #9 attention queue |
| Keyed decisions | An open `needs-decision` keeps resurfacing in an OPEN DECISIONS section until a record closes it | #9, #47 decision log |
| Session-manager portability | tmux, Herdr, zellij, Orca, cmux | out of scope: Fledge is Herdr-native |

## Fledge has, firstmate does not

- **A typed, tested, versioned binary.** Every command returns one JSON outcome envelope with
  `outcome`, `phase`, and error codes. Go tests with `-race`, a gofmt gate, and an STE-100 prose
  ratchet. Firstmate's contracts live in script header comments and bash behavior tests.
- **Direct socket protocol use.** Readiness polling on spawn, identity-stale detection on `--id`,
  workspace-per-worktree placement, parent tracking. Firstmate's Herdr adapter shells out to the
  `herdr` CLI plus `jq`.
- **A first-class task graph.** Parent and subtask progress, `--after` prerequisites with cycle
  detection under a store lock, `task list --ready`, `task import` of a reviewed TOML proposal.
  Firstmate's backlog is a Markdown file or `tasks-axi` with dependencies as free text.
- **Enforced separate verification.** `task verify` refuses the owner and unregistered callers and
  records the verifier. Firstmate leaves that role to its external no-mistakes pipeline or the
  captain.
- **Measured usage.** `agent usage` reads tokens and cost from the harness session store per agent,
  with a `measured` or `unavailable` basis.
- **Identity-aware fan-out.** Filters such as `--mine --state idle --task X` select recipients for
  message, wait, stop, and usage; the caller is never selected. Each message carries a sender
  header with a reply command.
- **Caller-scoped cleanup.** `agent cleanup` stops only the caller's own spawned workers whose
  tasks are all verified or cancelled, and removes only checkouts whose recorded marker, branch,
  and base still match.
- **Repository memory as a command.** One fact per file, shared by all worktrees, injected into
  profile spawns.
- **Built-in role profiles** in the binary, with repository override by filename.

## Shared ground

Both refuse to equate an agent's `idle` with task success. Both isolate every worker in a
worktree. Both refuse to remove unlanded or unmerged work without an explicit force. Both keep
state on disk so a restart loses nothing. Both forbid adding an agent as a commit co-author.
Firstmate's Herdr adapter also declines to trust Herdr's native `idle`, for the same reason
Fledge documents.

## Where the principles differ

| Axis | Firstmate | Fledge |
| --- | --- | --- |
| Who orchestrates | A fixed persona that never writes to a project | Any agent; the built-in `orchestrator` profile says never edit, the owner's harness skills allow small direct edits |
| Where logic lives | Bash scripts own mechanics, the LLM owns judgment | Go owns mechanics and state transitions; judgment is outside the tool |
| Vendor stance | Harness and session-manager agnostic, adapters per vendor | Herdr is the terminal substrate; harness-agnostic through Herdr's 24 kinds |
| Autonomy | Opt-in per project; `yolo` is an explicit grant | Permission bypass is the spawn default; merge authority is not modeled |
| Primary unit | The project and its PR | The task and its verification record |
| Human interface | Translated outcomes, mandatory "captain" address | None; the orchestrator's own voice |
| Contract size | 9,000-word ceiling on always-loaded prose plus 30 skills | `AGENTS.md` plus short role briefs under an STE-100 ratchet |
| State | `data/`, `state/`, append-only status logs, a wake queue | `.fledge/state/` records under a store lock; no event log |

## Quality of life

- Firstmate is faster to start and slower to understand: clone, run the harness, type one sentence,
  then inherit 673 files, bash plus `jq` plus treehouse plus `gh`, and a vocabulary that needs its
  own translation table.
- Fledge is slower to start and easier to reason about: one binary, inside Herdr, explicit
  `--harness` and `--model` on every spawn, and in return `--json` everywhere, `cleanup --dry-run`,
  `doctor`, and `update --check`.
- Firstmate hides the machinery; Fledge is the machinery. Fledge's open friction entry that
  `blocked` carries no reason is the kind of question firstmate's wake classifier exists to answer.
- Stacking is plausible. Firstmate's Herdr backend reimplements in bash what Fledge does natively.

## Relevant Herdr facts

Herdr 0.9.3 (protocol 22) exposes `events.subscribe`, which keeps one connection open and streams
`{"event":…,"data":…}` lines, and `events.wait`, which blocks until one matching event arrives. A
`pane.agent_status_changed` subscription filters to one pane and optionally one target status.
`pane.output_matched` pushes once per connection and must be re-subscribed. There is no
unsubscribe; close the connection. See [events.md](../herdr/events.md).

## Decisions

Recorded 2026-10-12 by the owner:

- Goal: borrow ideas into Fledge, reprioritizing existing backlog items rather than adding scope.
- Borrow all four: the zero-token watcher and events (#4, #85), the durable steering mailbox
  (#40, #41), the attention queue and digest (#9), and delivery modes with merge guards (#57, #60,
  #61).
- Recast the Fledge orchestrator as a dispatcher, not a contributor, in the spirit of firstmate's
  first hard rule.

- Dispatcher rule: tighten the built-in `orchestrator` profile (never edit, delegate investigation
  to a researcher or scout, never land without the user's word) and remove the direct-edit
  allowance from the owner's `orchestrate` and `fledge-orchestrate` skills. No hook enforcement
  and no firstmate-style outcome-only etiquette.
- Watcher: a background daemon from the start, not a blocking foreground command. Reason: a
  blocking CLI wait makes agents submit extra turns to check on it; a daemon lets the
  orchestrator idle at zero tokens and be woken by a message.
- Mailbox: doorbell always. Every message is stored as a durable record and the recipient's pane
  receives only a one-line doorbell naming the command that reads it.
- Landing: a guarded `task land`. The task records its delivery mode; one command merges after
  verification and the user's word. PR mode depends on `gh`.
- Configuration: the `git config fledge.baseBranch` step goes away. A committed
  `.fledge/config.toml` holds the repository's base branch and Fledge reads it wherever the git
  config was read.
- Daemon lifecycle: automatic. The first spawn or assign in a repository starts the supervision
  daemon under a state lock; it exits when its last watched worker ends; cleanup stops it.
- Wave-one wake classes: `blocked` or pane exited; a worker idle or done while it still owns an
  assigned, uncompleted task; an unacknowledged doorbell past a timeout. Stale-working detection is
  deferred.
- Waves: wave 1 is the dispatcher rule (profile, skills) and the base-branch config file. Wave 2
  is the daemon, wake queue, mailbox, and attention queue as one state design, with `task land`
  running in parallel because it shares no files with them.

Recorded 2026-10-12, wave-two decisions and the wave-one sign-off:

- Ring timing: immediate. The daemon types the doorbell as soon as a wake is actionable, even if
  the orchestrator is mid-turn.
- Local landing uses a merge commit titled `Merge branch '<branch>' (verified task <id>)`.
- `task verify --commit` records the verified head, and `task land` refuses a branch whose head
  differs from it.
- Delivery mode defaults from `delivery` in `.fledge/config.toml`, with a per-task `--delivery`
  override.
- The owner's `orchestrate` and `fledge-orchestrate` skills were edited on 2026-10-12 to remove
  the direct-edit allowance.
- Wave-one sign-off answers:
  - The config file stays at `.fledge/config.toml`, despite the cost of upgrading the nested
    managed `.fledge/.gitignore`.
  - When the local base branch is absent, Fledge falls back to `origin/<name>`.
  - The orchestrator delegates all investigation.

Recorded 2026-10-14, the stack wave (GitHub stacks, the branches table, and the feature flow):

- Decision 1, scope: the wave covers stacks, the `[branches]` table, and the feature-branch flow.
  A `pr_ready` supervisor wake, a promotion command from dev to main, and a command that refreshes
  a feature branch are out of scope.
- Decision 2, carry-forward: a verification carries forward to a new head with an equal patch id.
  `task verify --commit` stores the patch id. `task land` and `task size` compare it.
- Decision 3, the parent is the group: a parent task gets a shape, `stack` or `feature`. There is
  no separate stack record. The proposal `[parent]` gets a `delivery_mode` key.
- Decision 4, one chain: the layers of a stack form one `after` chain. `task create` and
  `task import` refuse a fork.
- Decision 5, branches: a task gets a `branch` at creation. `task size` and `task land` measure
  each stack layer against the branch of the layer below, the layer base.
- Decision 6, landing a stack: a stack lands only in `pr` mode, with a verified parent.
  `task land` pushes the layers, opens one pull request for each layer, and forms a native GitHub
  stack. `task land --merge` merges the stack with the merge-async API. Each run makes the records
  follow GitHub, also after a merge made with the GitHub button. No command deletes a branch.
- Decision 7, the branches table: a `[branches]` table with `main` and `dev` keys replaces the
  `base_branch` config key. `fledge doctor` warns about a missing dev branch. No command makes it.
- Decision 8, the feature flow: a local landing merges in the checkout that has the target checked
  out. The members of a feature parent default to `local` delivery into the feature branch.
- Decision 9, size: `task size` measures a layer against the layer below. For a stack parent, it
  reports the total of the stack. The report stays advisory.
- Decision 10, cleanup: `agent cleanup` judges a stacked layer by the merge commit of its task
  against the stack base.
- Decision 11, limits: each layer is at most 300 source lines, 20 files, and 3 code domains.
- The task ceiling is 300 hand-written source lines (owner decision of 2026-10-14). Files stay at
  20 and code domains at 3. The pull request target of 200 and the pull request ceiling of 300 do
  not change.
- Owner answers to the six questions of the first pass:
  - Q1, patch id mode: `git patch-id --verbatim`, not `--stable`.
  - Q2, a stack landing needs the parent verified. The parent moves to `landed` from `verified`.
  - Q3, mode of a stack: the parent's mode must be `pr`. The layer modes have no effect.
  - Q4, refusal codes for the local landing: the new codes `land_target_checkout_missing` and
    `land_target_checkout_dirty`.
  - Q5, friction status word: `Resolved 2026-10-14`.
  - Q6, a missing default dev branch: fall back to `origin/HEAD`, then `main`, as before.

Further decisions are appended as planning rounds settle them.
