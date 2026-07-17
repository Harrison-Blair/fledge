# Worker protocols

The delegated worker roles, agent-neutral: the planning incubator, and the team-loop (Tier C) brooder and skua. These are spawned workers: a spawn prompt is a worker's entire context (it inherits no conversation history) and must be fully self-contained. A `spawn-worker` is fresh, named, addressable, killable, may idle, and returns one final message.

A worker's spawn prompt tells it which protocol file to follow (incubator, brooder, or skua), its name, the orchestrator's name (the harness-assigned name the orchestrator supplies — address the orchestrator by exactly that name; e.g. on Claude Code it is `team-lead`), and its role-specific fields — for brooders and skuas: feather ID, worktree/branch, evidence-file path, and the paired counterpart's name (same species); for the incubator: the user's feature request verbatim.

### Ledger handoffs, not message state

Every state-bearing handoff between a worker and its counterpart — a review verdict, a worker's activity/liveness, an escalated blocker — is a typed ledger record, written and read with `fledge heartbeat`/`await`/`verdict`/`escalate`/`pulse` and `fledge ledger read`, never carried as content inside a message. `message-peer` is not removed and stays available to every role, re-scoped to exactly one job: a **stateless wake-up nudge** — "I wrote a record, go check" — never the carrier of the record's content itself. A one-shot report with no ongoing state behind it (a scout's or forager's final coverage summary, a skua's numbered findings list) is not a ledger handoff and may still travel as message content; the ledger is only for the state a counterpart would otherwise have to infer from message arrival or absence.

### Heartbeat

Any worker running a long operation (a build, a test suite, a synthesis pass, a long shell command) calls `fledge heartbeat <name> [--note "<what I'm doing>"]` **before** starting it, and again **periodically during** it, at an interval comfortably under the lease default — never before-only. A before-only heartbeat goes stale partway through a long stretch of real work and is misread as a stall: this feather's own planning phase measured a forager synthesis run as one unbroken 5m25s silence, and a before-only heartbeat would have gone stale at 5:00 — 25 seconds before the work succeeded, raising a false stall on a healthy worker.

Multi-step work has seams to heartbeat at between steps. A **single blocking call with no seam** to heartbeat at (one long `fledge` invocation, one long build) instead declares its expected duration up front: `fledge heartbeat <name> --expect <duration>`.

### Waiting, and the exit-4 recovery

Whoever blocks on another worker's next handoff runs `fledge await <subject> --kind <kind> --timeout <duration>` — `--timeout` is mandatory, so no wait is ever unbounded. `fledge await` is a deterministic replacement for "wait to be re-invoked by an event," not a return to hand-rolled polling: the poll interval and the timeout are fixed and bounded, and the call runs once inside the waiting agent's own turn.

Two kinds of record, two wait modes, never interchangeable:

- **`verdict` and `escalation` are write-once.** Wait for them with `--exists`: `fledge await <subject> --kind verdict --exists --timeout <duration>` or `fledge await <subject> --kind escalation --exists --timeout <duration>`. Omitting `--exists` here waits for the record to *change* from its value at call time — a record that is written once and never changes again deadlocks that wait.
- **`status` is repeatedly written** (it doubles as the liveness heartbeat and as a done/ready/blocked terminal signal carried in its note). Wait for a change to it with `fledge await <subject> --kind status --timeout <duration>` — no `--exists`: an existence-wait on `status` returns the instant the subject's very first heartbeat lands, long before the terminal value the waiter actually wants.

`fledge await` exits `4` (`ExitTimeout`) on a timeout with no change — a normal, expected outcome, not a failure. On exit 4, the waiter has learned nothing about the subject and must not guess:

1. Run `fledge pulse <subject-name>`.
2. **Not stalled** → the subject is working; `pulse` reports its declared quiet period and the elapsed time, so re-await for the remainder rather than retrying blind.
3. **Stalled** → that classification *is* the stall signal — escalate to the user (`fledge escalate`); never abandon a worker unilaterally on a bare timeout.
4. **No status record** → the subject hasn't heartbeat yet (it's starting up), which is **not** stalled — re-await.

Never compare timestamps by hand to judge liveness: `pulse` exists precisely so this one tested procedure has one home, and every wait site in these protocols defers to it.

Each protocol lives in its own file:

- `incubator.md` — the delegated planner: owns the planning phase end to end; relay envelope, communication rules, drafting, lifecycle.
- `brooder.md` — the feather implementer: test-first protocol, scope discipline, evidence, handoff and fix loop, lifecycle.
- `skua.md` — the paired reviewer: review checks, criteria audit, verdict rules, lifecycle.
