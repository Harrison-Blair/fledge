# Fix Plan: Code-Review Findings (dev vs main, 2026-08-11)

Source: high-effort multi-agent review of `git diff main...HEAD` (208 files). 34 findings
survived adversarial verification; the 10 most severe correctness defects are planned here.
Roughly twenty additional confirmed cleanup/maintainability findings fell below the report
cap and are out of scope for this plan (candidates for a follow-up `/simplify` pass).

Full review output: `/tmp/claude-1000/-home-penguin-source-fledge/ef759c25-8b04-4c0b-89dc-a6b6900cfd8f/tasks/wn8frl06l.output`

## Delegation & sequencing

The work is split into five workstreams with non-overlapping file surface area so they can
be delegated to independent agents per the AGENTS.md delegation model. Workstreams 1–5 are
mutually independent and can run in parallel. Within a workstream, items are ordered.

| Workstream | Files owned | Findings |
|---|---|---|
| 1. Messaging store: reply authorization + durability | `internal/messaging/store.go`, `internal/messaging/coordination.go` (append path), `internal/lifecycle/messaging.go` | F1, F3 |
| 2. Dispatcher & supervision resilience | `internal/watchproc/dispatcher.go`, `internal/messaging/coordination.go` (start-evidence path) | F4, F5, F7 |
| 3. Session stop & confirmation UX | `internal/lifecycle/manager.go` (Stop path), `internal/tui/confirm.go`, `cmd/stop.go` | F6, F8, F9 |
| 4. Filesystem symlink hardening | `internal/fsutil/`, `internal/project/profile.go` | F2 |
| 5. Installer | `scripts/install.sh` | F10 |

Caution on the shared files: workstreams 1 and 2 both touch `internal/messaging/coordination.go`
but in disjoint functions (append/write path vs `markAgentTaskStarted`/start-evidence). If that
is judged too risky for parallel agents, run workstream 2 after workstream 1 lands.

Every workstream must ship regression tests with the fix (see per-finding notes), keep logic
in pure functions with IO injected at the edges, and leave `./scripts/build.sh` and the full
race-enabled test suite green.

---

## Workstream 1 — Messaging store: reply authorization + durability

### F1. In-session user replies always rejected (`internal/lifecycle/messaging.go:92`, `internal/messaging/store.go:579`)

**Problem.** Messages addressed to `user` are stored with `RecipientPane ""` (the store
enforces "user recipient must not have a pane"), but `paneCaller` resolves the control-shell
user to a caller with the non-empty `HERDR_PANE_ID`. `ReplyMessage`'s ownership check
(`original.RecipientPane != active.caller.paneID`) and `authorize()` both re-enforce the
mismatch, so a user inside the session can never reply to any message thread; the identical
command works only from a terminal outside Herdr.

**Fix.** Make ownership comparison identity-based rather than pane-string-based for the user
caller: when the caller resolves to the user identity, a message whose recipient is `user`
(RecipientPane `""`) belongs to the caller regardless of the caller's paneID. Apply the same
rule in both enforcement points — `ReplyMessage` in `internal/lifecycle/messaging.go` and
`authorize()` in `internal/messaging/store.go` — ideally by extracting a single shared
`callerOwnsMessage(caller, msg)` helper so the two paths cannot drift again. Do not weaken
agent-to-agent pane checks.

**Tests.**
- Promote the scratch test `internal/lifecycle/zz_verify_control_pane_reply_test.go` into a
  real regression test with assertions (it currently asserts nothing): user replies from the
  control pane (HERDR_PANE_ID set, no authority token) succeed; replies from outside Herdr
  still succeed; an agent replying to a message it does not own is still rejected.
- Delete the scratch file itself (it is untracked debris; replace, don't keep).

### F3. Fsync removed from the ledger append path (`internal/messaging/store.go:497`, `internal/messaging/coordination.go:86`)

**Problem.** Old `appendLocked` called `file.Sync()` after every event write and the old
`WriteFileAtomic` fsynced the temp file plus its directory. New `appendEvents`/`writeEvents`
never sync, so "durable" events (task complete/fail, wakes) live only in the page cache. A
power loss after a successful `fledge agent task complete` silently loses the completion —
the orchestrator is never woken and the task shows in-progress forever.

**Fix.** Restore durability at the two seams:
- `writeEvents`: `file.Sync()` after the write loop, before returning.
- `fsutil.WriteFileAtomic` (used by the coordination path): fsync the temp file before
  rename, and fsync the containing directory after rename, matching the old semantics.
Keep the sync calls at the IO edge; do not thread sync decisions through business logic.

**Tests.** Unit-test via an injected file abstraction (or a spy wrapping `*os.File`) that
`writeEvents` and `WriteFileAtomic` issue Sync in the right order (write → sync file →
rename → sync dir). A comment on the append path should state the durability contract the
CLI docs promise.

---

## Workstream 2 — Dispatcher & supervision resilience

### F4. Message wake dropped after a single failed delivery (`internal/watchproc/dispatcher.go:578`)

**Problem.** `drainCovered` makes one `PromptAgent` attempt per message wake; on any
non-context error it records a terminal failed outcome and continues. No retry, no failure
notification to the sender, and managed agents cannot query the inbox — so a transient
delivery failure silently stalls the delegation forever.

**Fix.** Two layers, both needed:
1. **Retry with backoff:** on delivery failure, record a failed *attempt* (not a terminal
   outcome) and leave the wake pending so a later drain retries it. Cap attempts (e.g. 5)
   with escalating delay; only after the cap is exhausted record the terminal failure.
2. **Failure notification:** when a wake does go terminal, enqueue a wake to the *sender*
   carrying the delivery failure (mirroring the delivery-failed agent-idle audit that
   task-assigned wakes already get), so the sender never waits on a message the recipient
   never saw.

Keep retry/attempt bookkeeping in the store as pure state transitions; the dispatcher only
supplies attempt results and the clock.

**Tests.** Store-level: wake stays pending after failed attempts below the cap; goes
terminal and emits a sender-notification wake at the cap. Dispatcher-level (existing seams
in `dispatcher_seams_test.go`): a PromptAgent stub failing once then succeeding results in
delivery, not a dropped message.

### F5. Transient Snapshot error kills the dispatcher daemon (`internal/watchproc/dispatcher.go:455`)

**Problem.** The agent-idle deadline branch calls `options.Herdr.Snapshot` on a timer and
returns the error from `runDispatcher` on any failure. The daemon has no supervisor and is
only relaunched as a side effect of a later fledge command, so one transient herdr CLI
failure stops all wake delivery for the whole session.

**Fix.** Treat audit-time Snapshot failure as retryable: log it, reschedule the deadline
(short retry, e.g. the existing deadline granularity), and continue the dispatcher loop.
Reserve fatal returns for context cancellation and genuinely unrecoverable states. Audit
the rest of `runDispatcher` for the same pattern (any other periodic herdr call that
returns fatally) and apply the same policy.

**Tests.** Dispatcher seam test: Snapshot stub errors once at the deadline; assert the
dispatcher stays alive, retries, and completes the audit on the next tick.

### F7. Boot-time "working" status disarms the no-start audit (`internal/messaging/coordination.go:547`)

**Problem.** `markAgentTaskStarted` accepts any "working" observation at or after task
*activation* (assignment commit time) as start evidence. A harness that reports "working"
while booting — captured by the subscription-ready snapshot before the activation wake is
even delivered — sets `startedAt` and permanently disarms the 5-second no-start audit. If
the activation delivery then fails, nobody is ever told.

**Fix.** Gate start evidence on activation-wake *delivery*, not activation commit: record
the delivery time of the activation wake in supervision state, and only accept "working"
observations at/after that delivery time as start evidence. Observations between activation
and delivery are boot noise. If delivery failed, start evidence must remain unset so
`agentIdleDeadline` fires.

**Tests.** Coordination unit tests: (a) "working" recorded between activation and delivery
does not set `startedAt`; (b) "working" after delivery does; (c) with `deliveryFailed=true`
and a pre-delivery "working", the no-start audit still fires at the deadline.

---

## Workstream 3 — Session stop & confirmation UX

These three land as one change since they share the stop flow.

### F6. Live-agents guard removed from stop (`internal/lifecycle/manager.go:1037`)

**Problem.** Old `Stop` enumerated live agents, refused without `--force`
("refusing to stop session while agents are live: <names>"), and gracefully stopped each
agent before stopping the server. New `Manager.Stop` shows a generic "Stop and delete
<session>?" prompt and then kills everything; `stop_behavior_test.go` covering the refusal
was deleted.

**Fix.** Restore the guard in `Manager.Stop`: enumerate live agents; if any and not forced,
refuse with the agent names and a hint. When proceeding (forced or confirmed), gracefully
stop each agent before the server stop. Surface the live-agent names in the interactive
confirmation prompt so the user knows what they are killing.

**Tests.** Re-create the deleted `stop_behavior_test.go` coverage: refusal with live agents,
graceful-stop ordering when forced, prompt text includes agent names.

### F8. No non-interactive stop path (`internal/tui/confirm.go:28`, `cmd/stop.go`)

**Problem.** `Confirmer.Confirm` hard-errors ("confirmation requires an interactive
terminal") when stdin/stdout is not a TTY, and `fledge stop` has no bypass flag — sessions
cannot be stopped from scripts or CI at all.

**Fix.** Add `--force` (skip confirmation, implies overriding the live-agents guard with the
graceful-stop path) and `--yes`/`-y` (skip confirmation only; still refuses on live agents)
to `cmd/stop.go`. With either flag set, `Manager.Stop` never constructs the TTY confirmer.
Restore `--json` output for scripted consumers if the old output contract is still consumed
anywhere (check callers/docs before deciding; if nothing consumes it, note that in the PR
instead of resurrecting it).

**Tests.** Command-level test with piped stdio: `stop --yes` and `stop --force` succeed
without a TTY; bare `stop` without a TTY still errors clearly.

### F9. "yes" treated as decline (`internal/tui/confirm.go:67`)

**Problem.** The prompt accepts only the exact strings `y`/`Y`. "yes", "Yes", or "y " (a
trailing space via KeySpace) return confirmed=false and print "Canceled." — an affirmative
answer silently interpreted as "no".

**Fix.** Normalize the answer before comparing: trim whitespace, lowercase, accept `y` and
`yes`. Everything else declines (keep decline-by-default).

**Tests.** Table test over the confirm model: `y`, `Y`, `yes`, `YES`, `y␠`, `yes␠` confirm;
`n`, `no`, empty, `yep` decline.

---

## Workstream 4 — Filesystem symlink hardening

### F2. `.fledge` writes follow symlinks (`internal/project/profile.go:71`, `internal/fsutil/`)

**Problem.** The deleted `internal/agentprofile` package carried O_NOFOLLOW opens,
`os.Root`-bound directory handles, and owner-uid/hard-link-count verification. Nothing
re-established it: `fsutil.OpenRegular` is a plain `os.OpenFile`, so
`EnsureGeneratedOrchestratorPrompt` (O_CREATE|O_TRUNC + Chmod), the messaging log, and the
opencode runtime writes all follow symlinks. A cloned repo with a committed symlink at a
predictable `.fledge` path (e.g. `.fledge/profiles/generated/orchestrator.md` →
`~/.bashrc`) gets the target truncated and overwritten on `fledge start`.

**Fix.** Rebuild the hardening inside `fsutil` so every `.fledge` writer gets it for free:
- `OpenRegular`: open with `O_NOFOLLOW`; after open, `fstat` and reject non-regular files,
  files not owned by the current uid, and hard-link count > 1 (match the old
  agentprofile checks — consult `git show` of the deleted package for the exact set).
- Resolve paths through an `os.Root` (or `openat`-chain) bound to the `.fledge` directory so
  intermediate path components cannot be symlinked out of the tree.
- `WriteFileAtomic` gets the same treatment for its temp-file and rename destination
  (composes with the fsync work in F3 — coordinate the two edits to this file: land F3's
  sync change first or hand both to the same agent).

**Tests.** Recreate the deleted package's attack tests against `fsutil`: symlink at the leaf
(rejected, target untouched), symlinked intermediate directory (rejected), hard link to a
victim file (rejected), plus the happy path. An integration test that runs
`EnsureGeneratedOrchestratorPrompt` against a planted symlink and asserts the target file is
byte-identical afterwards.

> Note the F3 overlap: `fsutil.WriteFileAtomic` is edited by both workstreams 1 and 4. Either
> sequence them (1 then 4) or assign both `fsutil` edits to workstream 4 and have
> workstream 1 limit itself to `writeEvents`.

---

## Workstream 5 — Installer

### F10. GOPATH list not split (`scripts/install.sh:14`)

**Problem.** The old installer used the first GOPATH element (`${gopath%%:*}/bin`, `;` on
Windows); the rewrite uses the raw value, so `GOPATH=/a:/b` yields the literal directory
`/a:/b/bin` — created, installed into, and never on PATH, while the script prints success.

**Fix.** Restore first-element splitting with the OS-appropriate separator: `:` everywhere
except Windows (`;`), matching `go env GOPATH` semantics. Prefer `go env GOPATH` first
element; keep the existing PATH warning logic pointing at the corrected directory.

**Tests.** shell-level check (bats or a small sh test run from CI/build script) exercising
single-element, multi-element, and empty GOPATH.

---

## Definition of done

1. All ten findings fixed with the regression tests listed above shipped in the same change.
2. Scratch file `internal/lifecycle/zz_verify_control_pane_reply_test.go` removed, replaced
   by the real F1 regression test.
3. `./scripts/build.sh` passes; full test suite passes with `-race`.
4. Each workstream reviewed by a separate review agent before merge (per AGENTS.md).
5. Follow-up (not this plan): triage the ~20 below-cap cleanup findings from the review
   output and schedule a `/simplify` pass for the ones worth keeping.
