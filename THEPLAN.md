# THEPLAN — fledge zero-token watcher: handoff document

**Written 2026-08-04 by the orchestrating session after stopping all agents mid-Phase-5-fix-round.**
Read this top to bottom before touching anything. The full approved plan lives at
`/home/penguin/.claude/plans/https-github-com-kunchenguid-firstmate-i-eager-planet.md` (keep it in sync if scope changes).
Background research (how firstmate does this) is published at https://claude.ai/code/artifact/6bc3092e-959b-460d-b8c5-8236ef742f6d ; the firstmate clone used as reference is at
`/tmp/claude-1000/-home-penguin-source-fledge/14ebbae1-727b-44d8-816e-e202fc842c3d/scratchpad/firstmate` (scratchpad — may be gone; re-clone kunchenguid/firstmate if needed).

## 1. What is being built

A firstmate-style **zero-token watcher** for fledge: a detached daemon that watches workers via
(a) herdr native push events (`events.subscribe` → `pane.agent_status_changed`; `blocked` = actionable) with snapshot-poll fallback, and
(b) per-worker append-only status files taught via the spawn prompt (verbs `working|done|needs-decision|blocked|failed|paused`),
enqueues actionable findings to a durable wake ledger, and wakes the orchestrator by **Fledge message injection** (existing `agent prompt` path, sender identity `watcher`).

**User-locked decisions:** wake channel = message injection (no harness hook files in v1) · scope = fuller port incl. status files · process model = auto-start daemon (`fledge start`/`spawn` launch it, `fledge stop` tears it down) · **`fledge watch` = attached live monitor** streaming the decision log; `fledge watch --daemon` (hidden) is the background mode.

Verified environment facts: herdr 0.8.0, protocol 19; socket at `~/.config/herdr/sessions/<session>/herdr.sock`; `session list --json` carries `socket_path`; `api snapshot` carries `agent_status` per pane/agent.

## 2. Current tree state (exact, at handoff)

HEAD `b2c48e7` on `dev`. **Nothing committed by this effort. Do not commit without the user's say-so; NEVER add a Co-Authored-By trailer.**

- **Pre-existing uncommitted work by ANOTHER agent (~19 ` M` files):** README, harness/args*, lifecycle/{manager,manager_test,messaging_test,opencode,opencode_test}, messaging/{lock_unix,lock_windows,store,store_test}, project/{profile,profile_test,project,project_test}, statedir/{statedir,statedir_test}. This is statedir tmp/ + generated-orchestrator-prompt work. **Do not touch or revert it. The user decided it must LAND (be committed) before Phase 6 starts.** Only exception: `internal/herdr/client.go` — its ` M` status includes OUR additive struct tags (see below) on top of whatever was there.
- **This effort's files:**
  - `internal/statedir/watch.go` + `watch_test.go` (new) — `WatchSession`, `StatusDir`, `StatusFile` under `.fledge/tmp/<session>/`.
  - `internal/herdr/status.go` + `status_test.go` (new) + additive edits in `client.go` — `Session.SocketPath`, `Pane.AgentStatus`, `Agent.AgentStatus`, `Client.Protocol(ctx)` (`herdr status --json` → `.client.protocol`).
  - `internal/wake/` (new pkg, 9 files) — **APPROVED, FROZEN, GREEN.**
  - `internal/watch/` (new pkg, 8 files) — classify/config/events APPROVED; engine (engine.go + engine_test.go) **mid-fix-round, currently RED** (see §4).

Gates: `go build ./...` green. `go test ./internal/wake/...` green (-race, 84.6% cov). `go test ./internal/watch/...` **FAILS** — 2 failures from the interrupted fix round:
1. `TestCycleKeepsMarkersWhenTheLedgerRefusedTheWake` — markers advanced when the ledger refused the wake (enqueue-before-suppress invariant violated or the new test landed before its fix).
2. `TestCycleDoesNotHotSpinOnEventFailures` — nil-pointer panic at engine_test.go:1147 → engine.go:488/214 (a new test whose fake/wiring is half-landed).
These are almost certainly the stopped engineer's in-progress work on findings #1/#2 of the engine review, not regressions in approved code. First job: make them green (finish the fix or temporarily revert the half-landed test+code pair to the last green engine state, then redo the fix red-first).

## 3. What is DONE and approved (do not reopen)

| Piece | State | Review |
|---|---|---|
| statedir helpers + herdr decodes + Protocol() | green, mutation-verified | reviewed inline (Phase 1 report) |
| `internal/wake` ledger+markers+Compact | green, frozen | **APPROVED** (2 rounds + consolidated re-check) |
| `internal/watch` classify.go / config.go / events.go | green | **APPROVED** (2 rounds) |
| `internal/watch` engine.go | built, review findings partly applied | 1 must-fix + 5 should-fix OPEN (§4) |

Key already-fixed bugs worth knowing: wake's resurrection blocker (superseded queued entries came back with stale reasons → fixed via `Record.IDs`); config's discard-on-partial-error bug (corrupt neighbour field silently overrode `"enabled":false` → per-field `map[string]json.RawMessage` decode, case-insensitive keys).

## 4. OPEN work item: engine fix round (finish this first)

From watch-reviewer's task #16 review; the stopped engineer was mid-implementation:

1. **MUST-FIX: empty-snapshot mass-departure storm.** A successful snapshot with zero agents (or missing the orchestrator) must be treated as an untrustworthy read — skip departure detection that cycle, log a suspect-read line. Repro: two known workers, one empty snapshot read → must NOT emit a dead wake. (Likely what `TestCycleDoesNotHotSpinOnEventFailures`/the panic relates to — verify.)
2. **Two false-pins needing REAL assertions:** (a) the done-grace *backward* look-back test compares the wrong timestamp — deleting the look-back must fail the suite; (b) the fake subscriber ignores its ctx, so the bounded per-cycle ctx (load-bearing constraint) isn't actually pinned — make the fake honor ctx so an unbounded swap fails.
3. **Should-fixes:** restart-window — a watcher restart converts a clean finish into a "vanished" wake (completion look-back scoped to process start; smallest honest fix, no wake-Markers schema change without a reason); a never-acking subscription scores as a healthy cycle when `poll_interval_seconds <= 5` — a no-ack outcome must count toward the 3-strikes event fallback regardless of interval; remaining nits take-or-decline with reasoning.
4. Then: freeze, re-request review, get sign-off.

## 5. Remaining phases (briefs)

### Phase 6 — lifecycle wiring (GATED: user must land the other agent's work first; ask them)
In `internal/lifecycle`:
- `Manager.SendWatcherWake(ctx, dir, body) (messaging.Message, error)` in messaging.go — mirror `SendMessage` (same activeMessageSession flock flow) but FORCE caller identity `watcher` (skip `inferMessageCaller`), recipient fixed `orchestrator`, standard envelope via existing `deliverMessage`.
- Reserve name `watcher` in Spawn exactly like `user` (manager.go ~:582-595).
- Spawn: `MkdirAll(statedir.StatusDir)` + append a status-reporting block to the worker prompt (const beside `agentMessagingContext` ~manager.go:55; abs path from `statedir.StatusFile` substituted). Verb list + "append-only, supplement not replace messaging" wording is in the plan file. Consider stripping a UTF-8 BOM in ParseStatusLine here OR teach the prompt `printf`>`Out-File` — deferred decision from Phase 3 (PowerShell BOM lines are silently ignored today).
- Orchestrator instructions (orchestratorInstructions ~:518): one bullet — watcher messages come from sender `watcher`, act on them, never reply.
- Existing lifecycle prompt-content tests will need updating.

### Phase 7 — `fledge watch` command + daemon + adapters
- `cmd/watch.go` + `Watch(ctx, dir) error` on the `sessionManager` interface (cmd/root.go:16-25) + stub in root_test.
- **Attached monitor is the default mode** (user request): stream `.fledge/logs/<session>/watch.log` — if no daemon holds the flock, run the engine foreground (log to stdout AND file); if a daemon is running, print ~50-line backlog then poll-tail the file. `--daemon` (hidden flag): engine only, log to file, exit 0 if flock held.
- Singleton: flock LOCK_EX|LOCK_NB on `.fledge/tmp/<session>/watch/watch.lock` + `watch.pid` + `beacon` (touch every cycle). Reuse messaging's build-tagged lock pattern (wake has twins too).
- Decision log: `internal/logging`-style O_APPEND writer; **format contract already implemented in the engine's logger calls** — queued lines carry `w-<hex>` ids, delivered lines carry `msg-…` + every retired `w-` id (the E2E smoke greps these).
- Adapters (engine imports nothing concrete — it defines seams): Ledger→`internal/wake` (**MUST map `wake.ErrCorruptLog` → `watch.ErrCorruptLog`** — `errors.Is` keys the whole degradation path; and **MUST call `MarkDelivered(record.IDs...)`, NEVER `record.ID`** — pinned in wake by `delivering_only_the_survivor_ID_leaves_the_superseded_wake_queued`); Waker→`Manager.SendWatcherWake`; CompletionLog→`messaging.Store.List()` filter (sender==worker, recipient==orchestrator, CreatedAt>=since) — **internal/messaging needs NO changes**; Herdr→existing client; dial→`net.Dial("unix", socketPath)` from `session list --json`.
- Engine contracts the daemon must honor: pass a bounded per-cycle ctx to Subscribe (it blocks forever otherwise); Subscribe's `onReady` fires post-ack — take the level-reconcile snapshot then; intervals are engine-clamped ≥1s but don't rely on it.
- Consider a max line length in the event reader (unbounded `ReadBytes` today — deferred nit, matters once a long-lived daemon exists).

### Phase 8 — auto-start/teardown + docs + smoke
- `watchLauncher` seam on Manager (default: re-exec `os.Executable()` + `["watch","--daemon"]`, cwd=root, stdio→/dev/null, `Start()`+`Process.Release()` — clone of client.go StartServer:106-131). Call warn-only in `Start` (after initializeOrchestrator, BEFORE Attach) and in `Spawn`. No pre-probe — the child no-ops on a held flock.
- `Stop`: kill watcher (pidfile → verify flock actually held → SIGTERM; Windows Kill) **BEFORE** `removeSessionTemporaryState` (Windows can't delete flock'd files).
- `.fledge/watch.json` (tracked, loose schema — LoadConfig already built): defaults enabled=true, poll 15, idle_poll 60, signal_grace 2, heartbeat 600→7200, wake_min_interval 30, done_grace 90, event_stream true, min_protocol 16, version 1.
- README/docs; then the E2E smoke from the plan file's Verification section — NOTE the smoke greps **watch.log** decision lines, NOT ledger.jsonl (Compact legitimately empties the ledger after drains).

## 6. Engine design facts the next session must not re-derive

- Enqueue-to-ledger BEFORE advancing any marker (crash-replay invariant); drain = Pending → ComposeWakeBody → Deliver → MarkDelivered(IDs...) → best-effort Compact; rate window `wake_min_interval` batches everything into one message.
- Wake ledger retirement = delivered-set membership (a queued entry is retired iff a delivered marker names its ID). Compact drops retired entries wherever they sit (filter, not prefix) and keeps unknown-ID delivered markers (fail-safe; also keeps uniqueID from reissuing).
- ErrCorruptLog policy: ledger stays fail-loud; engine logs loudly, delivers ONE direct notice, then degrades to in-memory dedupe (never crash-loops). In-memory queued lines log "no durable ID".
- done → WakeAfterGrace: absorb iff a worker→orchestrator message exists with CreatedAt >= doneAt − grace (look-back is deliberate: workers message THEN write the status line).
- Dead detection suppression is in-memory by design (documented at-least-once limitation: a restart can cost one duplicate dead-wake).
- Event policy: blocked→wake once per pane (EventEscalated marker), working edge clears it, idle/done deferred; 3 consecutive transport failures disable events for process lifetime (ctx.DeadlineExceeded = clean budget expiry, NOT a strike).
- Master switch: workers = named snapshot agents besides `orchestrator`; zero workers → dormant (idle interval, no subscription, clear departed markers).

## 7. Team process rules (learned the hard way this session)

- **Red-first + mutation verification is mandatory** (user's global rule): every behavior test shown failing against broken code; reviewers independently re-run a sample.
- **Freeze before review**: engineer declares the package stable and stops editing until the verdict. Reviewers snapshot to /tmp (or `go test -overlay=`) and NEVER mutate the live tree; check the task list before calling anything scope creep.
- Package-scoped test runs (`go test ./internal/<pkg>/...`) while multiple actors share the tree; full `./...` gate only at orchestrator checkpoints.
- One package = one owner at a time.
- Never run `fledge start`/`fledge stop` (session lifecycle is the user's). Don't poll idle agents; act on reports.
- Commit only when the user asks; no co-author trailers.

## 8. Immediate next steps, in order

1. Fix the 2 red tests in `internal/watch` (finish the interrupted must-fix/pinning work, §4) → package green.
2. Complete the rest of the engine fix round → freeze → re-review → sign-off.
3. Ask the user to land the other agent's uncommitted change set on `dev` (their explicit chosen gate for Phase 6).
4. Phase 6 → review → Phase 7 → review → Phase 8 → full-suite gate + manual smoke (plan file, Verification section).
