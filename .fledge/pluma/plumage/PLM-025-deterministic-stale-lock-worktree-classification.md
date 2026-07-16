---
id: PLM-025
title: Deterministic stale-lock worktree classification
status: fledged
priority: P2
authored: 2026-07-16T01:37:30Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# PLM-025: Deterministic stale-lock worktree classification

## Context
A `/code-review med` pass over recent commits found that the implementation
workflow's crash/resume recovery step (`implementation.md` §6) hand-correlates
two separate command outputs to decide whether a feather's brood lock is
still live or safe to force-release: it runs `fledge broods` and
`git worktree list` and cross-references them itself, because the lock
`Record` (`internal/lock/lock.go`) stores `{Task, Owner, PID, Created,
Branch}` — no worktree path — and `broods --json` reports only `pid_alive`.
Neither command can answer "does this claim's worktree still exist" on its
own; the correlation falls to the LLM at the most fragile moment (right after
a crash, with partial work at stake). Mis-pairing a lock with the wrong or
already-removed worktree can force-release a live feather's claim, or resume
work into a worktree that's already gone.

The fix stores the worktree path in the lock record at brood time and has
`fledge broods` report worktree existence directly, so classification is a
CLI query instead of an LLM cross-reference — this review's core interest
applied to the resume path specifically.

Two design decisions were settled during interrogation:
- **Read-only reporting only.** `broods` gains a `worktree_exists` field and a
  `--stale` filter, but performs no release itself; the orchestrator still
  runs `fledge abandon FTHR-### --force` per stale lock, one at a time, as it
  does today. `broods` stays a pure query command, consistent with its
  current contract.
- **Legacy locks classify as stale.** Locks already on disk before this ships
  have no stored worktree path (the field is added going forward, not
  backfilled). Rather than a third "unknown" state, an empty/missing path is
  treated as `worktree_exists: false` — the simpler two-state classification.
  Because release stays manual (previous point), this doesn't auto-release
  anything; the recovery prose must say explicitly that a lock surfaced as
  stale for lacking a path (as opposed to one whose worktree was actually
  checked and is gone) should be re-checked against `git worktree list`
  before the operator force-releases it.

## User Stories
- As an orchestrator recovering from a crash or resuming a session, I want
  `fledge broods` to tell me directly whether each held lock's worktree still
  exists, so I don't have to hand-correlate it against `git worktree list`
  myself.
- As an orchestrator recovering from a crash, I want a `--stale` filter that
  narrows `fledge broods` to locks whose worktree is gone, so I can find the
  force-release candidates in one query instead of scanning the full list.
- As an orchestrator encountering a pre-upgrade lock with no stored worktree
  path, I want the recovery guidance to tell me to double-check it against
  `git worktree list` before force-releasing, so a legacy record isn't
  force-released on a false "stale" classification.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: `lock.Record` gains a worktree-path field, populated by `fledge
   brood` at claim time (the path passed to/derived the same way the command
   already derives `Branch` — from the worktree the caller is operating in,
   or an explicit flag/positional if the caller must supply it since `brood`
   itself doesn't create the worktree).
2. FC-2: `fledge broods --json` reports a `worktree_exists` boolean per
   record, computed by checking whether the stored path exists on disk (and,
   where feasible, is a worktree git still recognizes). A record with no
   stored path (legacy lock predating this change) reports
   `worktree_exists: false`.
3. FC-3: `fledge broods --stale` filters output (text and `--json`) to
   records where `worktree_exists` is `false`, leaving `fledge broods` with
   no flag showing the full list as today.
4. FC-4: `implementation.md` §6 recovery prose is rewritten to use
   `fledge broods --stale` for classification instead of hand-correlating
   `fledge broods` against `git worktree list`, and explicitly instructs: for
   any lock reported stale that has no stored worktree path (a legacy
   record), re-check it against `git worktree list` before running
   `fledge abandon FTHR-### --force`, since an empty path is classified stale
   by default rather than verified.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [x] AC-1: `lock.Record` has a worktree-path field; `fledge brood` populates
  it at claim time; a unit test in `internal/lock` covers a record written
  with and without the field (round-trip through `Acquire`/`Get`/`List`)
  (FC-1).
- [x] AC-2: `fledge broods --json` includes `worktree_exists` per record,
  `true` when the stored path exists on disk, `false` when it doesn't or the
  path is empty; a `cmd/fledge` txtar test covers both a live-path record and
  a missing-path (legacy, empty-path) record (FC-2).
- [x] AC-3: `fledge broods --stale` filters to `worktree_exists: false`
  records only, in both text and `--json` output; a txtar test asserts a
  mixed set of stale and live records is filtered correctly (FC-3).
- [x] AC-4: `implementation.md`'s §6 recovery step (core source in
  `internal/bootstrap/core/skills/fledge-orchestrate/`) is rewritten per FC-4,
  including the legacy-record caveat; this repo's scaffolded copy is
  refreshed to match.
- [x] AC-5: `go test ./...` passes and `fledge preen` reports the scaffold
  healthy after `fledge init --refresh`.

## Out of Scope
- Auto-releasing stale locks (e.g. a combined `--stale --release` mutation on
  `broods`) — declined during interrogation; release stays a manual,
  per-lock `fledge abandon FTHR-### --force` call by the orchestrator.
- Backfilling the worktree path onto locks that already exist on disk before
  this ships — they're classified stale by convention (FC-2) rather than
  migrated.
- The worker roster/species allocator (F6) — separate plumage.
- Any change to `fledge abandon`'s own force-release semantics — this
  plumage only changes how a stale candidate is *identified*, not how it's
  released.

## Open Questions
None — both design forks (read-only reporting vs. a release-performing
`--stale`; legacy-lock classification as stale vs. unknown) were resolved
during interrogation.
