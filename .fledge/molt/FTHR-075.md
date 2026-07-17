# FTHR-075 evidence

## AC-1

Guard tests were written first, in `internal/bootstrap/ledger_handoff_test.go`,
before any prose file was touched. Ran against the unmodified prose (this
worktree at 4edc49e):

```
$ go test ./internal/bootstrap/... -run 'TestAwaitGuardHelpersDetectDeadlockingForm|TestNoAwaitKindVerdictOrEscalationWithoutExists|TestNoAwaitKindStatusWithExists|TestNoAwaitWithoutTimeout|TestNoIndefiniteWaitLanguage|TestWorkerProtocolsDescribesHeartbeatDiscipline|TestWorkerProtocolsDescribesPulseRecovery|TestWorkerProtocolsRescopesMessagePeer|TestSkuaDescribesVerdictCommand|TestBrooderDescribesEscalateCommand|TestForagingDescribesAwaitStatusDoneSignal|TestForagingStatesAwaitIsNotPolling|TestSevenFilesNamePulseAsRecovery|TestSevenFilesUseLedgerVocabulary|TestImplementationAndPlanningReferenceRewrittenSections' -v
=== RUN   TestAwaitGuardHelpersDetectDeadlockingForm
--- PASS: TestAwaitGuardHelpersDetectDeadlockingForm (0.00s)
=== RUN   TestNoAwaitKindVerdictOrEscalationWithoutExists
--- PASS: TestNoAwaitKindVerdictOrEscalationWithoutExists (0.00s)
=== RUN   TestNoAwaitKindStatusWithExists
--- PASS: TestNoAwaitKindStatusWithExists (0.00s)
=== RUN   TestNoAwaitWithoutTimeout
--- PASS: TestNoAwaitWithoutTimeout (0.00s)
=== RUN   TestNoIndefiniteWaitLanguage
--- PASS: TestNoIndefiniteWaitLanguage (0.00s)
=== RUN   TestWorkerProtocolsDescribesHeartbeatDiscipline
    ledger_handoff_test.go:174: worker-protocols.md must name fledge heartbeat
    ledger_handoff_test.go:177: worker-protocols.md must instruct heartbeat periodically during long operations, not just before
    ledger_handoff_test.go:182: worker-protocols.md must state the before-and-during heartbeat instruction in that order
    ledger_handoff_test.go:185: worker-protocols.md must declare --expect for a single blocking call with no seam
--- FAIL: TestWorkerProtocolsDescribesHeartbeatDiscipline (0.00s)
=== RUN   TestWorkerProtocolsDescribesPulseRecovery
    ledger_handoff_test.go:199: worker-protocols.md must contain "fledge pulse" as part of the pulse recovery pattern
    ledger_handoff_test.go:199: worker-protocols.md must contain "Not stalled" as part of the pulse recovery pattern
    ledger_handoff_test.go:199: worker-protocols.md must contain "Stalled" as part of the pulse recovery pattern
    ledger_handoff_test.go:199: worker-protocols.md must contain "No status record" as part of the pulse recovery pattern
    ledger_handoff_test.go:203: worker-protocols.md must forbid comparing timestamps by hand
--- FAIL: TestWorkerProtocolsDescribesPulseRecovery (0.00s)
=== RUN   TestWorkerProtocolsRescopesMessagePeer
    ledger_handoff_test.go:214: worker-protocols.md must still mention message-peer
--- FAIL: TestWorkerProtocolsRescopesMessagePeer (0.00s)
=== RUN   TestSkuaDescribesVerdictCommand
    ledger_handoff_test.go:230: skua.md must describe fledge verdict as the verdict mechanism
--- FAIL: TestSkuaDescribesVerdictCommand (0.00s)
=== RUN   TestBrooderDescribesEscalateCommand
    ledger_handoff_test.go:239: brooder.md must describe fledge escalate for blockers
--- FAIL: TestBrooderDescribesEscalateCommand (0.00s)
=== RUN   TestForagingDescribesAwaitStatusDoneSignal
    ledger_handoff_test.go:261: foraging.md Commissioner section must contain "fledge await"
    ledger_handoff_test.go:261: foraging.md Commissioner section must contain "--kind status"
    ledger_handoff_test.go:261: foraging.md Commissioner section must contain "--timeout"
--- FAIL: TestForagingDescribesAwaitStatusDoneSignal (0.00s)
=== RUN   TestForagingStatesAwaitIsNotPolling
    ledger_handoff_test.go:279: foraging.md must state fledge await is a deterministic replacement for event re-invocation, not hand-rolled polling
--- FAIL: TestForagingStatesAwaitIsNotPolling (0.00s)
=== RUN   TestSevenFilesNamePulseAsRecovery
    ledger_handoff_test.go:290: worker-protocols.md: must name fledge pulse as the exit-4 recovery for its wait site(s)
    ledger_handoff_test.go:290: incubator.md: must name fledge pulse as the exit-4 recovery for its wait site(s)
    ledger_handoff_test.go:290: brooder.md: must name fledge pulse as the exit-4 recovery for its wait site(s)
    ledger_handoff_test.go:290: skua.md: must name fledge pulse as the exit-4 recovery for its wait site(s)
    ledger_handoff_test.go:290: foraging.md: must name fledge pulse as the exit-4 recovery for its wait site(s)
    ledger_handoff_test.go:290: implementation.md: must name fledge pulse as the exit-4 recovery for its wait site(s)
    ledger_handoff_test.go:290: planning.md: must name fledge pulse as the exit-4 recovery for its wait site(s)
--- FAIL: TestSevenFilesNamePulseAsRecovery (0.00s)
=== RUN   TestSevenFilesUseLedgerVocabulary
    ledger_handoff_test.go:309: worker-protocols.md: describes no ledger command (heartbeat/await/verdict/escalate/pulse/ledger read)
    ledger_handoff_test.go:309: incubator.md: describes no ledger command (heartbeat/await/verdict/escalate/pulse/ledger read)
    ledger_handoff_test.go:309: brooder.md: describes no ledger command (heartbeat/await/verdict/escalate/pulse/ledger read)
    ledger_handoff_test.go:309: skua.md: describes no ledger command (heartbeat/await/verdict/escalate/pulse/ledger read)
    ledger_handoff_test.go:309: foraging.md: describes no ledger command (heartbeat/await/verdict/escalate/pulse/ledger read)
    ledger_handoff_test.go:309: implementation.md: describes no ledger command (heartbeat/await/verdict/escalate/pulse/ledger read)
    ledger_handoff_test.go:309: planning.md: describes no ledger command (heartbeat/await/verdict/escalate/pulse/ledger read)
--- FAIL: TestSevenFilesUseLedgerVocabulary (0.00s)
=== RUN   TestImplementationAndPlanningReferenceRewrittenSections
    ledger_handoff_test.go:323: implementation.md must not describe merge clearance as a bare skua pass message
    ledger_handoff_test.go:326: implementation.md must describe fledge verdict as how merge clearance is learned
    ledger_handoff_test.go:329: implementation.md must describe fledge escalate for worker escalations
    ledger_handoff_test.go:334: planning.md must not duplicate foraging.md's full commissioner-wait paragraph verbatim; it should reference foraging.md's Commissioner section instead
--- FAIL: TestImplementationAndPlanningReferenceRewrittenSections (0.00s)
FAIL
```

**Why the negative guards (`TestNoAwaitKindVerdictOrEscalationWithoutExists`,
`TestNoAwaitKindStatusWithExists`, `TestNoAwaitWithoutTimeout`,
`TestNoIndefiniteWaitLanguage`) pass vacuously here, as expected:** the
current (unrewritten) prose contains zero `fledge await` invocations at all —
it is still 100% message-peer based, confirmed by `git log` showing none of
the seven files were touched by the CLI-side ledger feathers
(FTHR-073/074/088/089/090/092). A guard checking "no line violates shape X"
cannot fail against text that never mentions the shape. `TestAwaitGuardHelpersDetectDeadlockingForm`
is the test-first proof that the checker functions themselves are correct
(each flags exactly the deadlocking/backwards form and passes the corrected
form) — written and run red→green before any prose was rewritten, so the
negative guards are known-good instruments before they're pointed at real
prose. The positive guards above (heartbeat discipline, pulse recovery,
verdict/escalate mentions, ledger vocabulary, ...) are what actually fail
against the current prose, for the expected reason: the CLI-side commands
this feather teaches don't appear in it yet. Post-implementation run
(after the rewrite) is captured below.

Post-implementation, the same test set (plus `TestCoreSkillsNoPIDLiveness` and
`TestCoreNeutral`, AC-9/AC-10) all pass:

```
$ go test ./internal/bootstrap/... -v -run 'TestAwaitGuardHelpersDetectDeadlockingForm|TestNoAwaitKindVerdictOrEscalationWithoutExists|TestNoAwaitKindStatusWithExists|TestNoAwaitWithoutTimeout|TestNoIndefiniteWaitLanguage|TestWorkerProtocolsDescribesHeartbeatDiscipline|TestWorkerProtocolsDescribesPulseRecovery|TestWorkerProtocolsRescopesMessagePeer|TestSkuaDescribesVerdictCommand|TestBrooderDescribesEscalateCommand|TestForagingDescribesAwaitStatusDoneSignal|TestForagingStatesAwaitIsNotPolling|TestSevenFilesNamePulseAsRecovery|TestSevenFilesUseLedgerVocabulary|TestImplementationAndPlanningReferenceRewrittenSections|TestCoreSkillsNoPIDLiveness|TestCoreNeutral|TestWorkerProtocolsStub|TestForagingDocDescribesDigestWrite'
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.005s
```

Full package: `go test ./internal/bootstrap/...` → `ok`, all ~65 tests green
(see AC-10/AC-11 below for the complete list).

## AC-2

Guard `TestNoAwaitKindVerdictOrEscalationWithoutExists` / `TestNoAwaitKindStatusWithExists`
scan every inline code span (`` `...` ``) in the seven files for `fledge
await` invocations, flagging `--kind verdict`/`--kind escalation` without
`--exists` (write-once kinds must existence-wait) and `--kind status` *with*
`--exists` (repeatedly-written kind must never existence-wait — the other
AC-2 direction). `TestAwaitGuardHelpersDetectDeadlockingForm` proves the
checker functions themselves correctly flag the deadlocking/backwards form
and pass the corrected form (self-test, run red→green before the checkers
were pointed at real prose).

The actual shapes taught in prose:
- `worker-protocols.md`: `fledge await <subject> --kind verdict --exists --timeout <duration>` and `fledge await <subject> --kind escalation --exists --timeout <duration>` (write-once, existence-wait); `fledge await <subject> --kind status --timeout <duration>` (change-wait, no `--exists`).
- `implementation.md` §3.2: `fledge await FTHR-### --kind verdict --exists --timeout <duration>` for merge clearance.
- `implementation.md` §4: `fledge await <worker-name> --kind escalation --exists --timeout <duration>` for escalation detection.
- `brooder.md`/`skua.md`: `fledge await <own-name> --kind escalation --exists --timeout <duration>`.
- `foraging.md` Commissioner: `fledge await <forager-name> --kind status --timeout <duration>` (no `--exists`).

```
$ go test ./internal/bootstrap/... -run 'TestNoAwaitKindVerdictOrEscalationWithoutExists|TestNoAwaitKindStatusWithExists|TestAwaitGuardHelpersDetectDeadlockingForm' -v
=== RUN   TestAwaitGuardHelpersDetectDeadlockingForm
--- PASS: TestAwaitGuardHelpersDetectDeadlockingForm (0.00s)
=== RUN   TestNoAwaitKindVerdictOrEscalationWithoutExists
--- PASS: TestNoAwaitKindVerdictOrEscalationWithoutExists (0.00s)
=== RUN   TestNoAwaitKindStatusWithExists
--- PASS: TestNoAwaitKindStatusWithExists (0.00s)
PASS
```

## AC-3

Guard `TestNoAwaitWithoutTimeout` scans every inline code span containing
`fledge await` + `--kind` for a missing `--timeout`. Guard
`TestNoIndefiniteWaitLanguage` greps (case-insensitive) all seven files for
"block indefinitely", "wait indefinitely", "indefinitely", and "no timeout" —
none present. Every `fledge await` example in the rewritten prose carries a
concrete `--timeout <duration>`.

```
$ go test ./internal/bootstrap/... -run 'TestNoAwaitWithoutTimeout|TestNoIndefiniteWaitLanguage' -v
=== RUN   TestNoAwaitWithoutTimeout
--- PASS: TestNoAwaitWithoutTimeout (0.00s)
=== RUN   TestNoIndefiniteWaitLanguage
--- PASS: TestNoIndefiniteWaitLanguage (0.00s)
PASS
```

## AC-4

`worker-protocols.md` §"Waiting, and the exit-4 recovery" states the shared
pattern once: on exit 4, run `fledge pulse <subject-name>`; **Not stalled** →
re-await for the remainder `pulse` reports; **Stalled** → escalate to the
user, never abandon unilaterally; **No status record** → not stalled
(starting up), re-await. States explicitly "Never compare timestamps by hand
to judge liveness." Each of the other six files names `fledge pulse` at its
own wait site(s) (guard `TestSevenFilesNamePulseAsRecovery`) rather than
re-deriving the classification by hand — `foraging.md`'s Commissioner section
spells out all three branches again in full (since it's the biggest/most
novel wait site); `incubator.md`, `brooder.md`, `skua.md`, `implementation.md`,
`planning.md` name `fledge pulse` at their wait sites and point back to the
shared pattern rather than repeating all three branches.

```
$ go test ./internal/bootstrap/... -run 'TestWorkerProtocolsDescribesPulseRecovery|TestSevenFilesNamePulseAsRecovery' -v
=== RUN   TestWorkerProtocolsDescribesPulseRecovery
--- PASS: TestWorkerProtocolsDescribesPulseRecovery (0.00s)
=== RUN   TestSevenFilesNamePulseAsRecovery
--- PASS: TestSevenFilesNamePulseAsRecovery (0.00s)
PASS
```

## AC-5

`worker-protocols.md` §Heartbeat: "calls `fledge heartbeat <name> [--note
...]` **before** starting it, and again **periodically during** it, ...
never before-only", citing the measured 5m25s forager-synthesis silence from
this feather's own planning phase as the reason a before-only heartbeat would
have gone stale 25 seconds before success. Multi-step work heartbeats between
steps (seams); "a single blocking call with no seam ... declares its
expected duration up front: `fledge heartbeat <name> --expect <duration>`."
Guard `TestWorkerProtocolsDescribesHeartbeatDiscipline` asserts "fledge
heartbeat", "periodically during", the before-then-during ordering, and
`--expect` are all present.

```
$ go test ./internal/bootstrap/... -run TestWorkerProtocolsDescribesHeartbeatDiscipline -v
--- PASS: TestWorkerProtocolsDescribesHeartbeatDiscipline (0.00s)
```

## AC-6

Guard `TestSevenFilesUseLedgerVocabulary` asserts each of the seven files
contains at least one of `fledge heartbeat`/`fledge await`/`fledge
verdict`/`fledge escalate`/`fledge pulse`/`ledger read`. All seven now
describe their state-bearing handoffs this way: `worker-protocols.md` (all
six), `incubator.md` (await/pulse for the forager wait, heartbeat for its own
liveness), `brooder.md`/`skua.md` (escalate/verdict/await/pulse),
`foraging.md` (heartbeat/await/pulse), `implementation.md` (verdict/escalate/
await/pulse/ledger read), `planning.md` (await/pulse, by reference to
`foraging.md`'s Commissioner section).

```
$ go test ./internal/bootstrap/... -run TestSevenFilesUseLedgerVocabulary -v
--- PASS: TestSevenFilesUseLedgerVocabulary (0.00s)
```

## AC-7

`worker-protocols.md` §"Ledger handoffs, not message state" states
`message-peer` is "re-scoped to exactly one job: a **stateless wake-up
nudge** — 'I wrote a record, go check' — never the carrier of the record's
content itself," while carving out the legitimate exception (a one-shot
report with no ongoing state — a scout/forager final coverage summary, a
skua's numbered findings list — may still travel as message content). Every
rewritten handoff site follows this: skua's verdict/escalation and brooder's
escalation are recorded on the ledger first, with the message reduced to a
stateless nudge ("sends a stateless nudge", "the substance lives in the
escalation record, not the nudge"). Guard `TestWorkerProtocolsRescopesMessagePeer`
checks "message-peer", "stateless", and "nudge" all appear.

```
$ go test ./internal/bootstrap/... -run TestWorkerProtocolsRescopesMessagePeer -v
--- PASS: TestWorkerProtocolsRescopesMessagePeer (0.00s)
```

## AC-8

`foraging.md`'s Commissioner section: "`fledge await` is a deterministic
replacement for 'wait to be re-invoked by an event,' never a return to
hand-rolled polling: the interval and the mandatory `--timeout` are fixed and
bounded, and each call runs once inside your own turn. You still never poll
by hand — never `sleep`, a timed wait-loop, or repeated ad hoc status checks
— `fledge await` *is* the one sanctioned wait." Guard
`TestForagingStatesAwaitIsNotPolling` asserts both the never-poll
prohibition and the "deterministic replacement" phrasing are present.

```
$ go test ./internal/bootstrap/... -run TestForagingStatesAwaitIsNotPolling -v
--- PASS: TestForagingStatesAwaitIsNotPolling (0.00s)
```

## AC-9

`TestCoreSkillsNoPIDLiveness` (pre-existing, from FTHR-090/PLM-035 AC-12)
walks the entire embedded `bootstrap.FS` tree — core and every adapter —
for the string `pid-alive`. This feather's rewrite introduces no PID
reference; the test still passes unmodified.

```
$ go test ./internal/bootstrap/... -run TestCoreSkillsNoPIDLiveness -v
--- PASS: TestCoreSkillsNoPIDLiveness (0.00s)
```

## AC-10

`go test ./internal/bootstrap/...` — full package, all tests green (see
below). One pre-existing test, `TestForagingDocDescribesDigestWrite`, was
intentionally updated: its marker string changed from `"**On the final
message, verify and release.**"` to `"**On done, verify and release.**"`
because this feather changed the Commissioner's gating signal from the
forager's final message to its ledger `status` record reaching done — the
substance the test actually guards (the digest-write duty) is unchanged and
still asserted. One pre-existing acceptance-test fixture,
`cmd/fledge/testdata/forager_contract.txtar`, was intentionally updated:
its "required" grep assertions moved from the old final-message-only phrase
(`"only** signal that it is done is its explicit final message"`) to the new
ledger-driven phrasing (`"the done signal rather than its final message or
any idle/lifecycle notification"`), since that is exactly the invariant this
feather supersedes (PLM-020/FTHR-040's two-input state machine → PLM-030's
ledger-driven wait); its other assertions (no pipeline-stage/failure-mode
leakage, the force-terminate backstop, the exact-computation-for-counts rule)
are untouched and still pass. `TestCoreNeutral` (no harness-specific paths in
`core/`) passes unmodified.

```
$ go test ./internal/bootstrap/... -v 2>&1 | tail -3
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.010s

$ go test ./cmd/fledge -run TestScripts/forager_contract -v 2>&1 | tail -3
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/forager_contract (0.00s)
PASS
```

## AC-11

```
$ go build ./...
(no output)

$ gofmt -l .
(no output)

$ go vet ./...
(no output)

$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.478s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.011s
ok  	github.com/Harrison-Blair/fledge/internal/check	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.024s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/graph	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/ledger	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/lock	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/nest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/repo	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/roster	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/scan	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/spec	(cached)

$ go build -o /tmp/fledge-fthr075 ./cmd/fledge && /tmp/fledge-fthr075 preen
WARN  .fledge/pluma/feathers/FTHR-061-...: checked criteria missing evidence sections ... (pre-existing, unrelated to this feather — FTHR-061's own evidence file heading format, not touched by this branch)
1 warning(s)
$ echo $?
0
```

`fledge preen` exits 0 (warnings only, none introduced by this feather).
