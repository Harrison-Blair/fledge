# Implementation phase

Executes ready feathers from `pluma/feathers/` with an agent team: ephemeral `fledge-brooder` teammates (one per feather, each in its own git worktree) paired with a small persistent pool of `fledge-skua` teammates. This phase runs in the main session — you are the **team lead and orchestrator**: you dispatch, gate, merge, and triage. You do not implement or review code yourself. Your name is `fledge-orchestrator` — fledge prefix, no species postfix — and it is how teammates address you.

Communication topology is strict: each brooder talks only to its assigned skua and to you; skuas talk only to their current brooder and to you. There are no other peer channels — boundary questions between feathers route through you. Teammates can technically address any teammate by name; this topology is a rule you and they enforce, not a technical limit.

Teammates inherit no conversation history — a spawn prompt is a teammate's entire context and must be fully self-contained.

## 1. Resolve scope

Map the user's request to a feather set:

- "implement PLM-###" → all of that plumage's feathers.
- "implement FTHR-###" (or a list) → exactly those feathers; verify every feather in their `depends_on` closure is either `fledged` or in the set (`fledge vee --json` gives the full dependency data), and surface any that aren't before proceeding.
- bare "implement" → the ready set is `fledge ready --json` (it recomputes readiness from `depends_on` completion — the persisted `pipping`/`egg` field is only an authoring-time hint — and excludes feathers with a held lock). Present the set and run a confirmation gate on it (Accept / Make changes — see the SKILL.md ground rules); "Make changes" adjusts the set and re-presents.

Then gate:

- `fledge preen` passes with no errors (it validates that every feather exists, links to a hatched plumage, has a Tests section, and that the dependency graph is sound). Fix findings before dispatching.
- Context freshness: apply the freshness gate from `planning.md` step 1 (compare `.fledge/nest/index.md` commit to HEAD; ask before regenerating).
- The working tree on main is clean and the full test suite passes (see `.fledge/nest/testing.md` for how). Do not dispatch onto a broken baseline.
- The feather specs, plumages, and `.fledge/nest/` docs are committed — worktrees are created from main and only contain committed files. If they aren't, present the uncommitted paths and ask (AskUserQuestion): commit them now, or stop so the user can handle it.
- tmux precondition: check the session is inside tmux (`test -n "$TMUX"`). If not, warn the user that split-pane teammate display is unavailable and ask (AskUserQuestion): stop and restart inside tmux (recommended), or proceed degraded with in-process teammates. Never silently proceed.
- Permission mode: teammates inherit your permission mode at spawn. Brooders must edit files and run tests unattended in their panes — present the current mode and ask (AskUserQuestion) whether to proceed with it or stop while the user switches to one without per-action prompts (e.g. acceptEdits) before dispatching, or brooder panes will stall awaiting approvals.

## 2. Dispatch loop

Maintain the ready set continuously with `fledge ready` — a feather is ready when every feather in its `depends_on` is `fledged` and no lock is held on it. Dispatch the moment a feather becomes ready — do not wait for sibling feathers ("waves" are reporting language only).

For each feather dispatched:

1. **Oversight gate (during):** if the feather's frontmatter has `oversight: during`, STOP and prompt the user (AskUserQuestion) to confirm they are ready to participate before spawning. Do not dispatch it until they confirm; keep dispatching other ready feathers meanwhile. Because the brooder may message only its skua and you, you are the user's proxy for this feather: instruct the brooder in its spawn prompt to surface decision checkpoints to you rather than deciding autonomously, and relay each one to the user (AskUserQuestion) and their answer back. Without this the "during" participation never happens.
2. Create a worktree: `git worktree add <scratchpad or .fledge/burrows>/FTHR-### -b feather/FTHR-###-<kebab>` from main.
3. Assign a skua round-robin from the pool.
4. Spawn a teammate of type `fledge-brooder`, named per the naming scheme below, whose spawn prompt contains: its own name and feather ID, the feather spec path, the worktree path and branch, its evidence-file path (`.fledge/molt/FTHR-###.md`, written inside the worktree) and the duty to record per-criterion evidence there, the assigned skua's name, your name (`fledge-orchestrator`), and which `.fledge/nest/` docs to read (from the feather's Affected Modules citations).
5. Claim the feather: `fledge brood FTHR-### --owner <teammate-name> --branch feather/FTHR-###-<kebab>`. This atomically creates the lock (failing loudly if another dispatch already holds it) and sets the feather file's `status: hatching` in one step (you run fledge on main; brooders never touch spec files — the assigned skua is the only agent that mutates one, checking AC boxes via `fledge criteria` in the worktree). From this point until the branch merges, do not edit the dispatched feather's spec file on main — the skua's checked boxes ride the branch and a mid-flight edit conflicts at merge. Track the name→feather mapping in your roster.
6. Mirror into the shared team task list: create a team task titled `FTHR-###: <title>`, assigned to that brooder teammate, state in-progress. You are the **sole writer** of the team task list — teammates never create, claim, or update entries. It is a visibility mirror only; spec frontmatter is the source of truth and wins on any disagreement.

**Skua pool:** size is `ceil(active brooders / 3)`, minimum 1. Spawn teammates of type `fledge-skua` (named per the scheme below) as the active brooder count crosses each multiple of 3; skuas persist until the end of the run. A skua idle between review requests is normal — idle is not completion; it stays alive and addressable.

**Naming scheme:** a teammate's name is its agent-definition (purpose/template) name plus a unique identifier drawn from the 18 extant penguin species — `<agent-type>-<species>`, e.g. `fledge-brooder-adelie`, `fledge-skua-emperor`. The name is set at spawn and is how you and other teammates address it. Species identifiers are for spawned teammates only — you are always `fledge-orchestrator` and never take a species. One species per living teammate, shared across roles:

`emperor`, `king`, `adelie`, `chinstrap`, `gentoo`, `little`, `yellow-eyed`, `african`, `humboldt`, `magellanic`, `galapagos`, `fiordland`, `snares`, `erect-crested`, `southern-rockhopper`, `northern-rockhopper`, `royal`, `macaroni`

Assign the first unused species; a species frees for reuse only after its teammate's shutdown is confirmed. If all 18 species are in use (≥14 brooders plus their skua pool can exceed the list), append a numeric suffix to the first species — `fledge-brooder-adelie-2`, then `-3` — so a full pool never blocks dispatch. Report a one-line roster delta to the user whenever it changes (e.g. `+ fledge-brooder-gentoo → FTHR-007`); give the full roster (name → role → feather) on request. Keep the full name→feather mapping internally — species reuse depends on it.

## 3. On approval

A feather is cleared for merge in one of two ways: its skua messages you a pass (having checked every AC box in the worktree and committed that change), or — after a skua's 3rd-rejection escalation (§4), presenting the unresolved findings and cycle history — the user chooses (AskUserQuestion) to ship anyway (waiving the findings) rather than send it back for another cycle. On a user override, record the accepted (waived) findings on the feather file and use `--force` on the criteria-gated commands, so the decision is auditable. Then:

1. **Oversight gate (merge):** if the feather has `oversight: merge`, hold the branch unmerged. Show the user the full diff and the skua's verdict, then run a confirmation gate: Merge / Make changes ("Make changes" routes the feedback to the brooder as findings and re-gates after the fix).
2. Merge the branch to main (prefer a regular merge). On conflict, have the brooder rebase its branch and re-run its tests; because the rebase produces hand-resolved changes the skua never saw, route the rebased diff back through the assigned skua for a lightweight re-check (tests pass + resolution looks right) before you merge.
3. Run the full test suite on main.
   - **Green:** verify the criteria arrived with the merge — `fledge criteria FTHR-### --json` shows every box checked and `.fledge/molt/FTHR-###.md` exists on main — then run `fledge abandon FTHR-### --fledged` (releases the lock and sets `status: fledged`; it refuses while boxes are unchecked). Commit the spec update, and mark the mirrored team task completed yourself (never rely on a teammate to do it). Then remove the worktree (`git worktree remove`), delete the branch, and request graceful shutdown of the brooder teammate by name; its species frees only after shutdown is confirmed.
   - **Red:** the combination broke — the brooder is still alive and its worktree and branch survive (teardown happens only on green). Send it the failure; it fixes in its worktree and commits to the same branch. Route that fix commit through the assigned skua for a lightweight re-check, then merge the fix commit to main and re-run the suite. Loop until green, then proceed to the green teardown above. The fix reaches main only through this merge — never leave it stranded on the (already-merged) branch.
4. Re-evaluate the ready set and dispatch newly unblocked feathers (step 2). Shrink nothing: existing skuas stay for the run.
5. **Plumage closeout:** if that was the last unfinished feather of its plumage, verify each plumage acceptance criterion — citing which feathers and evidence files satisfy it — and present that AC-by-AC accounting through a confirmation gate (Accept / Make changes). On "Accept", check each box with `fledge criteria check PLM-### <n>` on main, run `fledge status PLM-### fledged` (it refuses while boxes are unchecked), and commit the spec update. On "Make changes", the gap goes back into the run (new or reopened feathers) before the plumage can close.

## 4. Escalations

Agents will escalate blockers and disputes to you. Triage by fledge's standing rule — facts belong in the repo, decisions belong to the user:

- **Facts** (what an interface is, where code lives, what a spec sentence means when the spec is actually unambiguous): resolve yourself by reading the spec, context, and code, and answer the agent.
- **Decisions** (genuine spec gaps, contradictions, tradeoffs, a skua's 3rd-rejection escalation): surface to the user with the context they need, then relay their call.

An escalated brooder stays alive and paused; other feathers keep flowing while it waits.

## 5. End of run

When no feathers remain in the set that are unfinished and dispatchable, gracefully shut down each skua teammate by name, then reconcile the team task list: every team task dispatched this run should be completed — complete any stragglers yourself and note discrepancies in the report. Then report:

- feathers completed (merged, suite green) vs. blocked or escalated, with reasons
- merges performed and the final suite status on main
- any feathers newly unblocked outside the run's scope that could be implemented next

## 6. Recovery after resume

`/resume` and `/rewind` do not restore teammates — after a resume, no teammate from the transcript exists, regardless of what your notes say. To recover a run:

1. Treat all remembered teammates as gone; clear the roster.
2. Inventory reality: `git worktree list`, feather branches, `fledge broods` (owner, branch, and pid-alive per held lock), and `fledge vee`. Feathers with a held lock (equivalently `status: hatching`) and a surviving worktree are the resume set. Locks whose feather has no surviving worktree are stale — release them with `fledge abandon FTHR-### --force`, then set their status explicitly (`fledge status FTHR-### pipping --force`) so they re-enter the ready set.
3. For each, respawn a fresh brooder teammate (a new species is fine) into the **existing** worktree and branch. Its spawn prompt must say partial work may exist: inspect commits and the diff before continuing, and re-verify the test-first evidence chain — if the captured failing-test output was lost with the old teammate, re-derive it (revert/stash) or flag the gap to the skua.
4. Respawn the skua pool at the computed size and reassign round-robin.
5. Reconcile the team task list against spec frontmatter (complete or create entries as needed).
6. Report the reconstructed roster to the user before proceeding.
