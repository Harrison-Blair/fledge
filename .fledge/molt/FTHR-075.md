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

Post-implementation, full run:

```
$ go test ./internal/bootstrap/... -v
(see below)
```
