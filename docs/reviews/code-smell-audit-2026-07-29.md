# Fledge Code-Smell Audit

Adversarial code-smell audit of the fledge repository (~13,100 lines of Go across 56 files in 10 packages), run 2026-07-29. Method: nine territory-scoped adversarial reviewers (six package territories plus three cross-package completeness sweeps) produced candidate findings; every candidate was then attacked by a per-finding skeptic that tried to refute it against [the code-smells reference](../reference/code-smells.md) and the actual code. Of 61 raw findings, 16 were refuted (26% refutation rate) and 45 confirmed. After skeptic severity adjustment the confirmed set is 0 high, 19 medium, and 26 low; every original high-severity finding was downgraded on verification (typically because the duplication was in-sync, test-only, or fails loudly).

Severities below use the skeptic-adjusted rating; entries note when the verifier downgraded the original rating.

## High

None. Four findings were originally rated high; all were downgraded to medium on adversarial verification.

## Medium

### internal/fledge

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/messaging.go:97`, `ReplyMessage / AckMessage / RetryMessage / CancelMessage` *(downgraded from high)*
  Four message commands repeat the same ~20-line precondition chain nearly verbatim: FindMessage + messageLookupError, `!run.Active` -> "archived messages cannot be X", activeMessaging, `run.ID != active.runID` -> "message is not part of the active run", then an actor-authorization check (`active.actor != "user" && active.actor != message.Sender` appears identically at lines 183 and 215). Blocks differ only in the verb and the terminal action — a change to the run/actor rules must be applied four times.
  *Fix:* Extract a shared resolver (e.g. `resolveActiveMessage(ctx, messageID)` returning the message plus activeMessagingContext after the FindMessage/run-active/run-match checks) and a small authorization helper for the sender-or-user rule, leaving each command with only its distinctive state check and event append.

- **[Data Clumps](../reference/code-smells.md#data-clumps)** — `internal/fledge/messaging.go:435`, `deliver`
  The cluster (runID, recipient/name, paneID, activationID) travels together through deliver, deliverLocked, drainAgentMessages, activateMessagingAgent, prepareMessagingActivation (returns two of them), DeliverActivation, and deactivateMessagingAgent — identical argument groups repeated across seven signatures. Removing any one member (e.g. activationID) leaves the rest meaningless as a delivery target.
  *Fix:* Promote the recurring group to its own type (e.g. an activation or deliveryTarget struct holding runID, agent name, paneID, activationID) and pass that object through the delivery/activation call chain instead of four parallel strings.

- **[Long Parameter List](../reference/code-smells.md#long-parameter-list)** — `internal/fledge/messaging.go:476`, `deliverLocked`
  deliverLocked takes 7 parameters, four of which are consecutive same-typed strings (runID, recipient, paneID, activationID); deliver (line 435) likewise takes 7 with five consecutive strings. Call sites like `s.deliver(ctx, runID, message.ID, name, paneID, activationID, client)` (line 666) are walls of arguments where transposing two strings compiles silently.
  *Fix:* Bundle the co-travelling string arguments into a parameter object (shared with the data-clump fix) so the signatures become ctx + target + client, making misordering impossible.

- **[Long Method](../reference/code-smells.md#long-method)** — `internal/fledge/agent_start.go:139`, `spawnAgentInCurrentPane`
  A ~98-line method mixing abstraction levels: a 45-line WithLocked closure doing pane/workspace validation, name-conflict eviction, and label renaming; then messaging activation and helper launch; then getwd/chdir/exec-syscall mechanics — with the rollbackInPaneSpawn call repeated at five separate exit points. The phases are separated by blank lines and cannot be named by one concise verb.
  *Fix:* Extract each coherent block into its own well-named method (e.g. claimCurrentPaneLocked, activateSpawnMessaging, execIntoHarness), and hoist the repeated rollback into a single deferred/wrapped error path so the top-level method reads as three named steps.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/agent_start.go:418`, `createAgentPane`
  The workspace-resolution fallback chain (prefer s.WorkspaceID, else st.WorkspaceID, else matchingWorkspace with wrapped error, else fallbackWorkspace) in createAgentPane lines 418-434 is a near-verbatim copy of resolveOrCreateOrchestratorWorkspace (workspace.go lines 114-129), and the two-line `s.WorkspaceID` / `st.WorkspaceID` preference fragment repeats again in reusableAgentPane (376-379) and spawnAgentInCurrentPane (162-165). The workspace.create call with identical params is also duplicated between allocateAgentPane and workspace.go:136. The verifier confirmed the copies have already drifted (the orchestrator path falls back via hasWorkspace when s.WorkspaceID is stale; the agent path only checks for empty).
  *Fix:* Extract one resolveWorkspaceID(st, snapshot) helper implementing the preference/fallback chain (and one createProjectWorkspace helper) and call it from both the agent-pane and orchestrator paths, so a change to workspace-selection policy lands in one place.

- **[Large Class](../reference/code-smells.md#large-class)** — `internal/fledge/service.go:20`, `Service`
  Service carries ~45 methods (verifier counted 82) spread across 9 files covering four distinct concerns — herdr session lifecycle (lifecycle.go, stop.go), pane/agent orchestration (agent*.go), durable messaging (messaging.go, 881 lines), and orchestrator window layout (workspace.go) — and its field list mixes unrelated hooks (LaunchStopCleanup, ExecAgent, LaunchDeliveryHelper, MessageStore, CallerPaneID). Any given method uses only a small subset of that state.
  *Fix:* Split along responsibilities: pull the messaging operations (which only need messageStore, Store, and actor inference) into their own cohesive type held by Service, and likewise separate the orchestrator-layout duties, leaving Service as the session/agent coordinator.

### internal/fledge (tests)

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/service_fixture_test.go:94`, `fakeBinary` *(downgraded from high)*
  Four separate helpers hand-roll near-identical fake-herdr shell scripts: fakeBinary (service_fixture_test.go:94), fakeBinarySessions (service_fixture_test.go:140), the inline script in TestResolveSessionAlwaysDerivesNameWithoutListingSessions (resolver_test.go:41-54), and newStoppedService (stop_test.go:257-276). The 5-line RequiredMethods-to-schema fragment and the '--version' / 'api schema' shell branches are verbatim copies in three of them, and 'herdr 0.7.5' / protocol 17 are repeated constants. A protocol or schema-shape change requires parallel edits in every copy.
  *Fix:* Extract one script-builder helper (e.g. writeFakeHerdr(t, opts) taking the session-list payload and optional invocation log) that owns the schema JSON, version string, and shell skeleton, and have all four call sites parameterize only what differs.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/stop_test.go:22`, `TestStopDeletesStoppedSessionAndClearsMappings`
  The same seed block populating all seven disposable-state fields (StopGeneration, Socket, WorkspaceID, OrchestratorTabID/PaneID/Initialized, Agents) is copied verbatim in stop_test.go:22-33, stop_test.go:112-123, stop_test.go:164-175, and stop_behavior_test.go:85-96, and the matching 7-condition 'state was cleared' assertion is copied in stop_test.go:49-53, 155-159, 189-193, stop_behavior_test.go:141-145, and integration_test.go:269-274. Adding a field to state.Session requires parallel edits in eight places.
  *Fix:* Extract a shared seedDisposableState(t, service) helper and an assertDisposableStateCleared(t, st, wantGeneration) helper so the field list lives in one place per direction.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/service_fixture_test.go:60`, `newFakeLifecycle`
  The unix-listener setup block (net.Listen on TempDir socket, EPERM sandbox t.Skip, t.Cleanup close, goroutine Accept loop, per-conn JSON decode) is copied four times across three packages: herdr fakeSocket (client_test.go:18-42) already encapsulates it, but fledge newFakeLifecycle (60-81) and cli fakeCoordinatedStopBinary (121-165) and fakeStartSocket (253-291) re-inline it. The copies have drifted: the herdr version checks only errors.Is(err, syscall.EPERM) while the other three also check os.IsPermission — the classic parallel-bug-fix sign.
  *Fix:* Promote herdr's fakeSocket(t, handler) shape into the shared test helper and have all four sites use it, so the sandbox-skip condition exists in exactly one place.

- **[Alternative Classes with Different Interfaces](../reference/code-smells.md#alternative-classes-with-different-interfaces)** — `internal/fledge/service_fixture_test.go:22`, `fakeLifecycle`
  fledge fakeLifecycle (instrumented struct with a serve method and 30+ recording fields) and cli fakeStartSocket (closure-based server recording to log files) do the same job — a fake herdr JSON-RPC server over a unix socket — under incompatible shapes, so neither package can reuse the other's and cli grew a third variant inside fakeCoordinatedStopBinary. Callers pick one arbitrarily based on which package they sit in, which is exactly why the duplication in the switch bodies exists.
  *Fix:* Converge on a single fake-server type in the shared helper whose instrumentation (recorded calls/params) satisfies both fledge's field-based assertions and cli's log-file assertions, then remove the closure variants.

### internal/cli

- **[Long Method](../reference/code-smells.md#long-method)** — `internal/cli/agent_commands.go:71`, `runAgentSpawn`
  ~110-line function whose body is a sequence of blank-line-separated blocks at mixed abstraction levels: flag validation, TTY detection, harness picker flow, model picker flow, name input flow, service construction, spawn call, and result printing. Each block is a coherent sub-task that could be named on its own.
  *Fix:* Extract each coherent block into a well-named helper — selectHarness(env, installed), selectModel(cmd, env, harness), promptAgentName(env) — leaving runAgentSpawn as a short orchestration of validation, selection, and the SpawnAgent call.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/cli/fixtures_test.go:71`, `fakeStartBinary`
  The herdr schema-building fragment (loop over herdr.RequiredMethods building `{"method":{"const":...}}` plus the protocol-17 fmt.Sprintf) is copied four times — fixtures_test.go:71-75 and 189-193, project_commands_test.go:34-38 and 575-579 — even though a fakeHerdrSchema() helper already exists at project_commands_test.go:417. The inline fake-herdr shell script in TestStopJSONDeletesStoppedDeterministicSession (project_commands_test.go:40-59) is also a near-verbatim copy of fakeStoppedStopBinary's script (313-344), differing only in the delete-log line.
  *Fix:* Point all four sites at the existing fakeHerdrSchema() helper, and replace the inline script in TestStopJSONDeletesStoppedDeterministicSession with a call to fakeStoppedStopBinary.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/cli/fixtures_test.go:78`, `fakeStartBinary` (cross-package) *(downgraded from high)*
  Four fixture builders across two packages generate near-identical fake-herdr shell scripts: cli fakeStartBinary (line 78) and fakeCoordinatedStopBinary (line 197) vs fledge fakeBinary (service_fixture_test.go:112) and fakeBinarySessions (:152). The '--version' -> 'herdr 0.7.5', 'api schema', and marker-file-driven 'session list'/'session delete' branches match line-for-line, and the 5-line RequiredMethods->schema JSON construction is copied verbatim in all four.
  *Fix:* Extract a shared internal test-helper package (e.g. internal/herdrtest) with one script-rendering function taking options (session name, running/exists markers, attach behavior, delete support); both packages' fixtures call it instead of re-embedding the heredoc.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/cli/fixtures_test.go:305`, `fakeStartSocket` *(downgraded from high)*
  The JSON-RPC method switch in cli fakeStartSocket (lines 305-409) reimplements fledge fakeLifecycle.serve (service_fixture_test.go:205-390) case by case: tab.create, tab.rename, pane.rename, and pane.split bodies are near line-for-line identical (same find-by-ID loops, same synthetic 'pane-right'/'p-right' PaneInfo construction), the injected-failure error envelope is byte-identical ('injected ' + method + ' failure'), and pane.send_input performs the same 3-param/keys-length-1 validation in both. A third partial copy lives in fakeCoordinatedStopBinary (lines 167-185).
  *Fix:* Extract one shared configurable fake herdr socket server (in the same internal/herdrtest helper) that owns the snapshot mutation logic per method; let fledge keep its call-count/arg instrumentation via callbacks or exposed state rather than a second full switch.

### internal/messaging

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/messaging/store.go:39`, `Store.WithLifecycleLock / Store.Append / Store.readEvents`
  The same seven-line advisory-lock block (os.OpenFile of a .lock path, syscall.Flock LOCK_EX, defer Close, defer LOCK_UN with the identical //nolint:errcheck comment) is copy-pasted three times in this file at lines 39-47, 129-137, and 247-255, differing only in lock-file suffix and error wording. A fourth near-identical copy exists in internal/state/store.go:75-83, showing the pattern is already spreading; a bug fix (e.g. handling EINTR) would need parallel edits in every copy.
  *Fix:* Extract a shared helper such as withFlock(path string, fn func(*os.File) error) error in the messaging package and have WithLifecycleLock, Append, and readEvents call it; optionally lift it somewhere internal/state can reuse it too.

### internal/state

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/state/store.go:156`, `writeAtomic` *(downgraded from high)*
  state.writeAtomic (lines 156-195) and project.writeConfig (project.go lines 159-198) are near line-for-line copies of the same ~40-line atomic JSON write: MarshalIndent, append '\n', CreateTemp with dot-prefixed pattern, tmpName/ok cleanup defer, Chmod, Write, Sync, Close, Rename, then the identical directory-fsync tail. They differ only in file mode (0o600 vs 0o644) and error-message nouns; any durability bug fix must be applied in both packages.
  *Fix:* Extract a single shared atomic-write helper (e.g. internal/fsutil.WriteFileAtomic(path string, data []byte, perm os.FileMode)) and have state.writeAtomic and project.writeConfig call it; the mode and marshalled payload become parameters.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/state/store.go:75`, `Store.WithLocked` *(downgraded from high)*
  The flock acquisition idiom — os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600), defer lock.Close(), syscall.Flock(LOCK_EX), defer Flock(LOCK_UN) with the same //nolint:errcheck comment — appears verbatim here (lines 75-83) and three more times in internal/messaging/store.go (WithLifecycleLock 39-47, Append 129-137, readEvents 247-255), differing only in lockfile suffix and error wrapping. Parallel bug fixes (e.g. lock-fd inheritance, stale-lock cleanup) are needed in two packages.
  *Fix:* Extract one shared helper, e.g. withFlock(path string, fn func() error) error in a small internal package, and have all four sites in state and messaging call it; error wrapping stays at the call site.

### internal/picker

- **[Long Method](../reference/code-smells.md#long-method)** — `internal/picker/picker.go:263`, `selectModel.View`
  View is ~70 lines containing two complete rendering algorithms separated by an early return (collapsible-tree render at 269-294, flat grouped render at 296-331), mixing abstraction levels: cursor-prefix math, collapse-indicator glyph selection, group-header tracking, and footer text all inline. The method cannot be named more precisely than "View" because it does two different things.
  *Fix:* Extract each coherent block into its own well-named method: renderCollapsibleRows and renderFlatRows (plus a shared footer/no-matches helper), leaving View as a short dispatcher.

### internal/agentspawn

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/agentspawn/catalog.go:275`, `normalizeAndSort`
  The sort closure duplicates the maker/name comparison logic in its two branches with divergent behaviour: the provider branch uses compareMakers (which forces "Other" last, lines 280-284), while the no-provider branch inlines strings.ToLower(Maker) comparison (287-289) so an unrecognized Codex/Claude model's "Other" maker sorts alphabetically instead of last; the lowercase name tiebreak is also copied at lines 285 and 290. compareProviders and compareMakers additionally hand-roll the same partition-then-lexicographic shape. The copies have already drifted: the verifier confirmed ParseOpenCodeModels and ParseCodexModels never set Provider, so their models sort "Other" mid-alphabet, contradicting the pi path's Other-last ordering.
  *Fix:* Have both sort branches call compareMakers and one shared name comparator so the Other-last rule and case folding live in exactly one place.

## Low

### internal/fledge

- **[Primitive Obsession](../reference/code-smells.md#primitive-obsession)** — `internal/fledge/messaging.go:401`, `inferActor` *(downgraded from medium)*
  The reserved actor/mailbox identity is the raw literal "user" repeated 12 times across messaging.go and lifecycle.go, including authorization checks (`active.actor != "user"` at lines 183, 215, 307); agent states are likewise scattered raw literals ("stopped", "unknown") re-typed in agent.go, agent_control.go, and lifecycle.go's emptyCounts. No named constant exists. The verifier noted a typo in the "user" comparisons fails closed, so this is a maintainability issue rather than a weakened access-control check.
  *Fix:* Declare a `const userMailbox = "user"` (or a small Actor type with an IsUser method) and a named AgentState string type with constants for the closed status set, replacing every literal.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/messaging.go:361`, `appendActiveRunEvent`
  The identical error-classification fragment — `var serviceErr *Error; if errors.As(err, &serviceErr) { return err }; return messageStoreError(err)` — appears four times in this file (lines 361-367, 466-472, 731-737, 769-776) as the epilogue of every WithLifecycleLock call.
  *Fix:* Extract the fragment into one helper (e.g. asServiceOrStoreError(err)) and call it from all four lock epilogues.

- **[Speculative Generality](../reference/code-smells.md#speculative-generality)** — `internal/fledge/agent_start.go:37`, `StartAgent`
  StartAgent is a two-line wrapper that sets NewTab=true and forwards to SpawnAgent; its comment says it is retained "for callers that do not participate in interactive pane takeover", but the only production caller in the repo (internal/cli/agent_commands.go:165) uses SpawnAgent — StartAgent is referenced solely by tests. Indirection with no second case behind it.
  *Fix:* Delete StartAgent and have tests call SpawnAgent with NewTab: true (or keep a single entrypoint), removing the unused flexibility.

- **[Dead Code](../reference/code-smells.md#dead-code)** — `internal/fledge/workspace.go:263`, `orchestratorTab`
  orchestratorTab has zero production references — the live selection path in selectOrCreateOrchestratorTab uses tabInWorkspace and firstTabInWorkspace directly; the only caller anywhere is integration_test.go:96. It also encodes a label-based lookup subtly different from the production recovery logic, inviting drift.
  *Fix:* Delete the function (version control can recover it) and have the integration test assert via the same helpers production uses (tabInWorkspace plus a label scan), or an exported inspection API.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/agent_control.go:216`, `stoppedView`
  The full 8-field AgentView literal from a state.Agent is written out three times: viewFromInfo (agent_control.go:76-79), stoppedView (agent_control.go:217-220, identical except State is fixed to "stopped"), and listWithClient (agent.go:65-68). Adding a field to AgentView requires editing all three literals in step.
  *Fix:* Extract one baseView(name, managed) constructor and derive the others from it — stoppedView becomes baseView with State set to "stopped", viewFromInfo overlays the live status.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/agent.go:164`, `managed`
  managed (158-171) and listWithClient (33-43) repeat the same preamble verbatim: client.Snapshot, then s.Store.WithLocked(s.Project.Session, s.Project.Root, ...) whose closure first calls reconcileMappings(st, snapshot, s.Project.Root, s.Project.Session, s.WorkspaceID) — the blocks differ only in what they extract from the locked state. The five-argument reconcile call must be kept in sync at both sites.
  *Fix:* Extract a Service helper (e.g. withReconciledState(ctx, client, fn)) that snapshots, locks, and reconciles once, so both callers pass only the closure that reads the reconciled state.

- **[Long Method](../reference/code-smells.md#long-method)** — `internal/fledge/agent.go:32`, `listWithClient`
  55 lines mixing four abstraction levels in blank-line-separated blocks: snapshot+locked reconcile, a side-effecting loop deactivating messaging for exited agents (48-57), pending-count fetch, and view assembly whose nested state resolution repeats the same 'if agent-pointer is nil then State = stopped' fragment twice (72-79). The method is hard to name honestly — it lists agents and mutates messaging activation state.
  *Fix:* Extract each coherent block into a named method — deactivateExitedMessagingAgents(st, live) and resolveAgentState(pane, live) — leaving listWithClient as a readable sequence.

### internal/fledge (tests)

- **[Dead Code](../reference/code-smells.md#dead-code)** — `internal/fledge/agent_test.go:258`, `TestModelArgumentsGeneratedForEveryHarness` *(downgraded from medium)*
  The loop ranges over four harness names but the harness variable is used only as the subtest name — the body calls modelArgs("provider/model", ...) identically every iteration, so the test runs the exact same assertion four times. The per-harness coverage the name promises does not exist; the loop is a vestige with no live effect.
  *Fix:* Either collapse to a single modelArgs assertion with an honest name, or make the loop real by feeding harness into the code under test if per-harness argument generation actually varies.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/stop_behavior_test.go:27`, `TestFailedServerStopDoesNotAdvanceGeneration` *(downgraded from medium)*
  The LaunchStopCleanup closure that builds a worker Store, constructs a detached worker Service from the request fields, and launches FinalizeStop in a goroutine is duplicated nearly line-for-line at stop_behavior_test.go:27-41 and 99-113, differing only in the timeout argument (150ms vs request.Timeout).
  *Fix:* Extract a shared helper, e.g. launchTestCleanupWorker(t, done chan error, timeout ...) returning the LaunchStopCleanup func, and parameterize the timeout.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/integration_test.go:36`, `TestLocalHerdrLifecycle`
  Both integration tests repeat the same preamble and teardown: the FLEDGE_INTEGRATION skip guard plus herdr LookPath (24-30 vs 168-174), the git init + project.Init setup (43-48 vs 190-195), and an identical three-command t.Cleanup running herdr server stop / session stop / session delete (36-42 vs 205-211).
  *Fix:* Extract shared helpers: requireIntegrationHerdr(t) for the skip guard, newIntegrationRepo(t) for the git/project setup, and registerHerdrCleanup(t, herdrPath, session) for the teardown block.

- **[Long Method](../reference/code-smells.md#long-method)** — `internal/fledge/integration_test.go:24`, `TestLocalHerdrLifecycle`
  One 138-line test function walks through skip guards, repo setup, session resolution, server start, orchestrator-layout verification, optional agent lifecycle, stop, message-run archiving, and restart isolation — distinct phases separated by blank lines with mixed abstraction levels. The opening comment enumerates the multiple things it verifies because the function cannot be named for one.
  *Fix:* Extract each coherent phase into a well-named helper (assertOrchestratorLayout(t, snapshot, workspaceID), runOptionalAgentLifecycle(t, service), assertRunIsolationAfterRestart(t, ...)) while keeping the single sequential lifecycle in the top-level test.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/fledge/messaging_test.go:35`, `TestMessageSendInjectAckAndLinkedReply`
  The identical five-line arrange fragment `if _, err := service.StartAgent(t.Context(), AgentStartOptions{Name: "worker", Kind: "codex", Timeout: 30 * time.Second}); err != nil { t.Fatal(err) }` appears six times in messaging_test.go alone (35, 107, 151, 168, 183, 210) and roughly ten more times across agent_test.go and stop_behavior_test.go. This is not table-driven repetition — it is a copy-pasted setup block whose option values would need parallel edits everywhere.
  *Fix:* Extract a mustStartAgent(t, service, name) test helper (with t.Helper()) wrapping the StartAgent call and fatal check, and use it at every arrange site.

### internal/cli

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/cli/agent_commands.go:103`, `runAgentSpawn` *(downgraded from medium)*
  The picker-result handling block — errors.Is(err, picker.ErrCancelled) -> print "Cancelled." and return nil, else non-nil err -> fledge.Wrap("picker_failed", ...) — appears three times nearly verbatim (lines 103-109, 129-135, 143-149) for the harness picker, model picker, and name input.
  *Fix:* Extract a shared helper (e.g. handlePickerResult(env, err) (cancelled bool, wrapped error)) so the cancellation message and picker_failed wrapping live in one place.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/cli/project_commands.go:253`, `confirmStop` *(downgraded from medium)*
  confirmStop (project_commands.go:253-267) and confirmPrune (sessions_commands.go:119-127) share an identical confirmation-reading tail: bufio.NewReader(env.in).ReadString('\n'), the same EOF-tolerant error check wrapped as fledge.Wrap("input_failed", "read confirmation: ..."), and the same TrimSpace + EqualFold("y")/EqualFold("yes") acceptance logic. Only the printed prompt differs.
  *Fix:* Extract a shared confirm(env *environment, prompt string) (bool, error) helper that prints the prompt and performs the read/EOF/y-or-yes logic once; both callers keep only their distinct prompt text.

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/cli/cli.go:174`, `environment.service` *(downgraded from medium)*
  service() (lines 171-178) and auditService() (lines 202-208) duplicate the project.Discover error-translation block verbatim (ErrNotInitialized -> project_not_initialized, else project_discovery_failed), and service() and messagingService() both duplicate the state.New + fledge.Wrap("state_unavailable", ...) fragment.
  *Fix:* Extract a discoverProject(cwd) (project.Info, error) helper holding the error translation, and a newStore() helper for the state.New wrapping; the three service constructors then compose these instead of repeating the conditionals.

- **[Long Parameter List](../reference/code-smells.md#long-parameter-list)** — `internal/cli/fixtures_test.go:41`, `fakeStartBinary` *(downgraded from medium)*
  Seven parameters including two adjacent booleans and an int produce call sites like fakeStartBinary(t, root, session, true, true, 0) and fakeStartBinary(t, root, session, false, false, 0, "pane.split") across start_test.go — the reader cannot tell which flag is running vs workspacePresent vs attachExit without checking the signature.
  *Fix:* Bundle the configuration into a parameter object (e.g. fakeStartOptions{Running, WorkspacePresent bool; AttachExit int; SetupFailure string}) so each call site names what it varies.

- **[Primitive Obsession](../reference/code-smells.md#primitive-obsession)** — `internal/cli/agent_commands.go:425`, `validateStates`
  The agent lifecycle-state vocabulary exists only as scattered string literals: the ad-hoc validity map in validateStates ("idle","working","blocked","done","unknown"), the same list restated in two --until help strings (lines 337, 419), and the list plus "stopped" hard-coded again in the status printer (project_commands.go:173-175). No package defines these as shared constants, so adding or renaming a state requires hunting down each literal set. The verifier found the scatter is wider still (lifecycle.go:357 counts map, agent_control.go:202).
  *Fix:* Introduce named state constants (or a validated small type) in the fledge package with one canonical slice for validation and help text, and have both validateStates and the status output iterate that single definition.

### internal/messaging

- **[Primitive Obsession](../reference/code-smells.md#primitive-obsession)** — `internal/messaging/reconstruct.go:59`, `DeliveryAttempt.Outcome`
  Delivery-attempt outcomes are bare string literals — "attempted" (line 59), "injected" (line 87), "failed" (line 92), "uncertain" (line 97) — assigned to the untyped string field DeliveryAttempt.Outcome, unlike the sibling event-type and status codes in model.go which at least get named constants; a typo in a new outcome would compile silently and consumers have no canonical value set to switch on.
  *Fix:* Declare Outcome constants (or a typed string `type Outcome string` with named values) in model.go next to the Status constants, and use them at every assignment site.

- **[Shotgun Surgery](../reference/code-smells.md#shotgun-surgery)** — `internal/messaging/store.go:129`, `Store.Append` *(downgraded from medium)*
  A single conceptual change to the locking discipline (e.g. shared locks for reads, LOCK_NB with timeout, lockfile naming or stale-lock cleanup) must be edited at four flock sites across state and messaging; a change to the durability discipline (fsync policy, directory-sync-after-rename) touches state.writeAtomic, project.writeConfig, and messaging StartRun/Append/readAndRepair across three packages. Missing one site ships a half-applied on-disk contract. The verifier noted the packages lock/write independent files, so a missed site yields inconsistent policy rather than a broken shared contract.
  *Fix:* Pull the scattered locking and durable-write behaviour back into one owning component (a shared persistence helper package) so that lock and fsync policy changes land in one place; the three packages keep only their format-specific logic.

### internal/herdr

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/herdr/binary.go:42`, `Binary (Inspect, Sessions, DeleteSession, StartServer, Attach, AttachSession)` *(downgraded from medium)*
  The identical fallback block `path := b.Path; if path == "" { path = "herdr" }` is copy-pasted at the top of all six Binary methods (lines 42-45, 99-102, 132-135, 146-149, 174-177, 188-191). A change to the default binary name or resolution logic requires six parallel edits.
  *Fix:* Extract a shared method, e.g. `func (b Binary) path() string`, that returns b.Path or the "herdr" default, and call it from each method so the fallback logic exists in exactly one place.

### internal/picker

- **[Duplicate Code](../reference/code-smells.md#duplicate-code)** — `internal/picker/picker.go:296`, `selectModel.View / selectModel.applyFilter` *(downgraded from medium)*
  The group/subgroup walk using the `lastGroup := "\x00"; lastSubgroup := "\x00"` sentinel pattern is implemented twice: applyFilter's else-branch (lines 199-238) emits header rows for collapsible mode, and View's fallback branch (lines 296-326) re-derives the same group/subgroup headers and indentation at render time for flat mode. A change to grouping or nesting semantics must be applied in both walks or the two modes diverge.
  *Fix:* Make applyFilter always emit header rows (marking flat-mode headers non-interactive) so View renders one row list, or extract a shared walkGroups helper that both call.

- **[Temporary Field](../reference/code-smells.md#temporary-field)** — `internal/picker/picker.go:108`, `selectModel.visible`
  The `visible []Item` field is written at the top of applyFilter (line 190) and read only inside that same method (lines 193, 201); no other method or the View ever consults it. Between calls its value is meaningless to readers, who cannot tell when it is valid.
  *Fix:* Make `visible` a local variable of applyFilter and delete the struct field, so the model's state is always meaningful.

- **[Primitive Obsession](../reference/code-smells.md#primitive-obsession)** — `internal/picker/picker.go:376`, `subgroupPath / selectModel.collapsed`
  Collapse identity is encoded as a NUL-joined string (`group + "\x00" + subgroup`) used as a pseudo-structured key into the `collapsed map[string]bool`, and the same "\x00" magic byte doubles as an impossible-sentinel initial value for lastGroup/lastSubgroup in two functions. Go map keys can be comparable structs, so the string encoding is a primitive standing in for a domain concept.
  *Fix:* Introduce a comparable `type nodePath struct{ group, subgroup string }` as the map key and row path, removing both the NUL-separator encoding and the NUL sentinel initializers.

- **[Speculative Generality](../reference/code-smells.md#speculative-generality)** — `internal/picker/picker.go:372`, `groupPath`
  `func groupPath(group string) string { return group }` is an identity function — indirection with no second case behind it; every one of its call sites would read identically with the bare `group` value.
  *Fix:* Replace groupPath(x) with x at call sites and delete the function (subgroupPath keeps the only real key construction). If the nodePath struct-key fix is applied, this disappears for free.

- **[Long Method](../reference/code-smells.md#long-method)** — `internal/picker/picker.go:189`, `applyFilter`
  55 lines doing three jobs — filtering, grouped-row/header construction with collapse skipping, and cursor clamping — with the header-emission branch nested four deep and driven by "\x00" sentinel lastGroup/lastSubgroup values plus interleaved collapse-skip continues (203-238). The grouped-row block reads as a separate algorithm embedded in the else arm.
  *Fix:* Extract the grouped-row construction into buildGroupedRows(visible) and name the collapse-skip conditions with explaining variables, leaving applyFilter as filter + rows + clamp.

### internal/buildinfo

- **[Dead Code](../reference/code-smells.md#dead-code)** — `internal/buildinfo/buildinfo.go:29`, `Current`
  The struct literal sets `Development: true`, but line 41 unconditionally reassigns `out.Development = out.Revision == "" || out.Modified` on every path, so the initial assignment is never read and can never affect the result.
  *Fix:* Delete the unreachable initial `Development: true` assignment, leaving the single computed assignment as the only writer.

## Coverage Appendix

### Territory results

| Territory | Found | Refuted | Confirmed |
|---|---|---|---|
| fledge-core | 12 | 1 | 11 |
| fledge-tests | 9 | 2 | 7 |
| cli | 9 | 2 | 7 |
| messaging-state | 3 | 1 | 2 |
| herdr-spawn | 3 | 2 | 1 |
| leaf-pkgs | 7 | 1 | 6 |
| sweep: cross-package store duplication | 5 | 2 | 3 |
| sweep: cross-package test-fixture duplication | 5 | 1 | 4 |
| sweep: harness/provider shotgun surgery | 8 | 4 | 4 |
| **Total** | **61** | **16** | **45** |

### Sweep gaps examined

Three completeness sweeps targeted cross-cutting concerns that per-package reviewers cannot see from inside a single territory:

- **Cross-package store duplication** — persistence idioms (flock acquisition, atomic write + fsync) duplicated across internal/state, internal/messaging, and internal/project; confirmed 3 findings including the writeAtomic/writeConfig twin and the four-site flock idiom.
- **Cross-package test-fixture duplication** — fake-herdr scripts, fake JSON-RPC socket servers, and unix-listener setup blocks duplicated between internal/cli and internal/fledge test suites (with a canonical copy already existing in internal/herdr); confirmed 4 findings including one interface-divergence smell.
- **Harness/provider shotgun surgery** — harness/provider vocabulary and sort/reconcile logic scattered across internal/agentspawn and internal/fledge; 4 of 8 candidates survived, including a confirmed behavioral drift in normalizeAndSort's maker ordering.

### Agent accounting

Inferable from the stats: 9 territory audit agents (6 package-scoped reviewers + 3 cross-package sweep agents) produced 61 candidate findings; each candidate received its own adversarial skeptic verification pass (61 verifications, 16 refutations); 1 synthesizer produced this report — 71 agent runs in total.
