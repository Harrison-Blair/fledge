# Project Context

_Generated at 2026-07-25 20:27:55 UTC._

## Project Overview

Fledge is a zero-inference Go orchestrator for multi-agent coding sessions built on three integrations (Claude, Codex, Pi) hosted in visible Herdr panes. Its core invariant is that the Go CLI is the sole state authority: a per-flock daemon (internal/daemon) maintains an append-only journal as ground truth, journaling every state transition before acknowledging the client, and rebuilding roster/messages/placements/readiness by replaying that journal on startup. The daemon never performs LLM inference itself — all reasoning happens inside operator-visible agent panes launched and tracked through the herdr socket API (internal/herdrwire) and CLI (internal/herdr). Agent identity, model routing, and launch configuration are governed by portable Markdown definitions (internal/agentcfg), which are synchronized into deterministic JSON indexes and enforce strict namespace and integration-specific field rules. Supporting packages provide workspace/session identity (internal/workspace, internal/flock), file discovery and ignore filtering (internal/scan, internal/ignore), sandboxed RPC fallback (internal/filebridge via internal/client), model discovery (internal/catalog), and directory scaffolding (internal/scaffold). The cmd/fledge CLI implements hand-rolled flag parsing with globally-unique short flags, dispatches flock/agent/context/daemon subcommands, and drives interactive flows (start, restart, clear) with extensive dependency-injection test seams. A self-hosted Forager/Analyzer mechanism (internal/contextdoc, validated by internal/daemon's context_message_test) partitions the codebase into file groups, dispatches structured-analysis requests to spawned analyzer agents, validates replies, and renders the published project.md context document — this very run is an instance of that mechanism. Legacy Stage 0 design docs (docs/) record now-completed research (authority-split model, screen-detection precedence, reliable input injection, no practical concurrency ceiling) that is carried forward into the current architecture but is not itself a live plan.

## Routing

- `.github` → `project-meta`: CI/CD workflows: PR validation (lint/test/build/version-check), post-merge release automation, and release badge publishing.
- `scripts` → `project-meta`: Build, install, reinstall, and release-version-check shell scripts.
- `README.md` → `project-meta`: Authoritative command/flag reference, .fledge/ tree layout, and portable agent format documentation.
- `CLAUDE.md` → `project-meta`: Developer guide: architecture invariants, CLI flag conventions, versioning, and re-verification rules for fast-moving integrations.
- `AGENTS.md` → `project-meta`: Repository guidelines for single-user project conventions.
- `docs` → `docs`: Completed Stage 0 legacy design/decision/experiment record and immutable 2026-07-17 reference snapshots; historical, not a live roadmap.
- `cmd/fledge/main.go` → `cmd-fledge-core`: CLI entrypoint, command dispatch, flock/daemon lifecycle, agent spawn/messaging, interactive pickers.
- `cmd/fledge/context.go` → `cmd-fledge-core`: Context subcommands: workspace scan, directory graph, analyzer request/worksheet composition.
- `cmd/fledge/help.go` → `cmd-fledge-core`: Embedded help topic map and lookup/fallback resolution.
- `cmd/fledge/watch.go` → `cmd-fledge-core`: Daemon log watcher and non-critical watcher-pane installation.
- `cmd/fledge/agent_definitions_test.go` → `cmd-fledge-tests`: Full CLI test suite: agent lifecycle, flock management, context pipeline, parser robustness, interactive startup, daemon spawn/restart.
- `internal/daemon/daemon.go` → `daemon-core-a`: Core daemon state machine, message routing, identity auth.
- `internal/daemon/journal.go` → `daemon-core-a`: Append-only event journal: atomic writes and full-state replay/recovery.
- `internal/daemon/placement.go` → `daemon-core-a`: Workspace/tab placement resolution, creation latching, crash recovery.
- `internal/daemon/ready_signal.go` → `daemon-core-a`: Socket and file-based readiness token authentication.
- `internal/daemon/ready_test.go` → `daemon-core-a`: Readiness handshake and orchestrator-special-case tests.
- `internal/daemon/spawn.go` → `daemon-core-b`: Agent spawn lifecycle: reserve/launch/journal/readiness loop for Claude/Pi/Codex.
- `internal/daemon/startup_assets.go` → `daemon-core-b`: Generated Claude plugin / Pi extension / Codex bootstrap startup automation assets.
- `internal/daemon/inbox_notify.go` → `daemon-core-b`: Durable orchestrator mailbox wake-notification worker with backoff and coalescing.
- `internal/daemon/isolation_test.go` → `daemon-core-b`: Per-flock isolation guarantees: journal, socket, roster, species pool.
- `internal/daemon/boundary_test.go` → `daemon-tests-misc`: Daemon initialization guard tests: scaffold, name/socket validation, malformed-request tolerance.
- `internal/daemon/context_message_test.go` → `daemon-tests-misc`: Managed Forager/Analyzer message schema validation before journaling.
- `internal/daemon/delivery_order_test.go` → `daemon-tests-misc`: Message durability under append failure and restart-replay correctness.
- `internal/daemon/e2e_test.go` → `daemon-tests-misc`: Broad integration coverage: roster, correlation, restart recovery, journal corruption tolerance.
- `internal/daemon/forager_test.go` → `daemon-tests-misc`: Dedicated-workspace prompt-based launch and readiness-timeout rollback for Forager.
- `internal/daemon/reply_test.go` → `daemon-tests-misc`: Structured reply correlation and inbox-claim identity checks.
- `internal/daemon/serve_test.go` → `daemon-tests-misc`: Serve loop accept-error handling and shutdown/ownership-transfer semantics.
- `internal/daemon/socket_test.go` → `daemon-tests-misc`: Socket path resolution, collision avoidance, concurrent-election tests.
- `internal/daemon/watch_test.go` → `daemon-tests-misc`: Session liveness probing and window-title branding tests.
- `internal/agentcfg` → `internal-agentcfg`: Agent profile/definition loading, fixed-prefix model routing, Markdown parsing, atomic index synchronization.
- `internal/catalog` → `internal-misc-a`: Model discovery from installed integration binaries with deterministic collision resolution.
- `internal/client` → `internal-misc-a`: Unified daemon RPC: Unix socket primary, filebridge fallback.
- `internal/contextdoc` → `internal-misc-a`: Analyzer request/reply schema validation, template composition, and project.md rendering — the machinery this very Forager run uses.
- `internal/filebridge` → `internal-misc-a`: Sandboxed workspace-local file-based RPC transport with PID-liveness tracking.
- `internal/flock` → `internal-misc-a`: Flock naming/validation and workspace-prefixed session identity derivation.
- `internal/herdr` → `internal-misc-b`: Herdr CLI wrapper for session lifecycle (ensure/recreate/stop/delete).
- `internal/herdrwire` → `internal-misc-b`: Direct Herdr Unix-socket wire client for pane/workspace/tab operations.
- `internal/ignore` → `internal-misc-b`: gitignore-style .fledgeignore pattern matching with #include support.
- `internal/protocol` → `internal-misc-b`: Daemon wire-format Request/Response/Agent/Message types and Op constants.
- `internal/scaffold` → `internal-misc-b`: .fledge/ directory tree initialization, template seeding, managed-definition sync.
- `internal/scan` → `internal-misc-b`: Ignore-aware workspace file walking used by context scan.
- `internal/species` → `internal-misc-b`: 18-slug penguin species pool allocation per agent type.
- `internal/version` → `internal-misc-b`: Embedded semantic version source of truth with dev-build suffix.
- `internal/workspace` → `internal-misc-b`: Canonical workspace root discovery and deterministic hash/slug derivation for socket namespacing.

## Cross-Group Flows

- `cmd-fledge-core` → `daemon-core-a`: CLI commands (start, agent spawn/ready, flock lifecycle) dial the daemon socket via internal/client and drive placement/readiness state transitions.
- `cmd-fledge-core` → `daemon-core-b`: runAgentSpawn and interactive start build OpSpawn requests that the daemon's spawn() lifecycle (reserve/launch/journal/ready) fulfills.
- `daemon-core-a` → `daemon-core-b`: acquirePlacement() (placement.go) resolves the workspace/tab a spawned agent lands in before spawn.go's launch() calls Herdr to start the pane.
- `daemon-core-b` → `internal-misc-b`: spawn.go's launch() and startup_assets.go call into internal/herdrwire for pane/workspace operations and internal/herdr for session ensure/recreate.
- `internal-agentcfg` → `daemon-core-b`: agentcfg.Config (Integration/Model/Argv/Env) resolved via Route()/Load() supplies the launch argv and role instructions spawn.go passes to Herdr.
- `cmd-fledge-core` → `internal-misc-a`: context.go's compose/validate commands and runInit's model discovery call into internal/contextdoc and internal/catalog respectively.
- `internal-misc-a` → `internal-misc-b`: internal/contextdoc's scan/render pipeline consumes internal/scan file listings (filtered by internal/ignore) and internal/scaffold directory constants.
- `internal-misc-a` → `daemon-tests-misc`: The Forager/Analyzer request/reply schemas validated in internal/contextdoc are enforced a second time server-side in the daemon's managed-context message path (context_message_test.go), which this run's dispatch/reply traffic exercises.
- `daemon-core-b` → `daemon-tests-misc`: spawn.go's readiness and inbox-notify machinery is exercised end-to-end by e2e_test.go and delivery_order_test.go for restart-replay and once-only delivery guarantees.
- `docs` → `daemon-core-a`: The zero-inference and journal-as-truth invariants recorded as resolved ADRs in docs/DECISIONS.md are directly encoded in daemon.go/journal.go's append-before-ack design.
- `docs` → `internal-misc-b`: EXP1/EXP2 findings (native Claude screen-detection precedence, reliable pane.send_input with real Enter) shape how internal/herdrwire and internal/herdr are used by the daemon, favoring metadata-only reporting for Claude panes.

## Global Invariants

- The append-only per-flock journal is the sole state authority; every operation is fsync'd and journaled before the client is acknowledged, and daemon state is rebuilt purely by journal replay on startup.
- The orchestrator performs zero inference: it only issues Herdr/socket commands, consumes events as input signals, advances a deterministic state machine, and writes its journal.
- All agent work happens in visible, operator-controllable Herdr panes; Herdr itself is never treated as durable state and loses metadata across restarts.
- Liveness differs by agent kind: self-registered agents are judged by PID (signal-0 probe on the session leader), while spawned agents change state only on an observed journaled event, never by PID.
- Workspace/session identity is canonical and deterministic: FindRoot + EvalSymlinks + SHA-256 hash ensures the CLI and daemon always agree on socket namespace and session naming.
- Model routing is a fixed prefix table with no inference or fallback; an unrecognized model id is always a hard error with a remedy hint, never a guess.
- Namespace enforcement is strict: user agent/profile names may never use the reserved fledge-* prefix, and fledge-orchestrator is the sole exempt singleton name.
- Analyzer/Forager request and reply traffic is validated against strict JSON schemas (DisallowUnknownFields, duplicate-key rejection, exact correlation) both client-side (internal/contextdoc) and daemon-side (context_message_test), before anything is journaled or acted on.
- Flock-level isolation is total: each flock has its own journal, socket, roster, and species pool with no cross-flock visibility.
- Destructive or irreversible interactive operations (flock clear, deinit) require a real terminal and explicit confirmation; there is no --force flag for scripted bypass.

## Subsystem: cmd-fledge-core

CLI frontend implementing command dispatch, flag parsing, flock/daemon lifecycle, agent spawning, interactive workflows, and context analysis. Hand-rolled flag parsing (takeFlag, takeBoolFlag, rejectFlags) with global short-flag uniqueness. Flock lifecycle spans start (interactive orchestrator + watcher), restart, stop, clear. Daemon spawning via setsid re-exec with readiness polling. Agent commands for register, spawn, ready, stop, list, models, types, and async messaging. Interactive UI with terminal detection, picker menus, confirmation prompts. Context commands for workspace scanning, directory graphs, and analyzer request composition. 32 help topics with fallback resolution.

**Purpose:** CLI entrypoint, flag parsing, and core commands (main, context, help, watch) in cmd/fledge

### Entry Points

- `cmd/fledge/main.go`: main() → run() CLI entrypoint and top-level command router (init, start, restart, stop, flock, agent, daemon, context, watch, version, help)
- `cmd/fledge/main.go`: runStart (590) - Interactive flock startup with herdr session, daemon spawn, orchestrator selection, and watcher pane installation
- `cmd/fledge/main.go`: runFlock (370) - Flock subcommand dispatcher (clear, stop, list, status)
- `cmd/fledge/main.go`: runAgent (1511) - Agent subcommand dispatcher (register, spawn, ready, stop, list, models, types, msg)
- `cmd/fledge/main.go`: runInit (2209) - Create/refresh .fledge directory with model discovery and agent config synchronization
- `cmd/fledge/main.go`: runContext (83) - Context subcommand dispatcher (scan, graph, compose, validate, render-project)
- `cmd/fledge/watch.go`: runWatch (23) - Stream daemon.log with polling and interrupt handling
- `cmd/fledge/context.go`: runContextCompose (19) - Analyzer request/reply composition dispatcher
- `cmd/fledge/help.go`: printHelp (531) - Lookup and print help topic from embedded helpPages map (32 topics)

### Key Symbols

- `takeFlag` in `cmd/fledge/main.go` (function): Extract flag with value from args; rejects leading - in values (346-358)
- `takeBoolFlag` in `cmd/fledge/main.go` (function): Extract valueless flag from args (873-884)
- `rejectFlags` in `cmd/fledge/main.go` (function): Reject leftover flag-shaped args (360-368)
- `spawnDaemon` in `cmd/fledge/main.go` (function): Re-exec self as daemon in setsid session with env vars (966-996)
- `waitSpawnDaemonReady` in `cmd/fledge/main.go` (function): Poll OpStatus until daemon session matches expected; 5s timeout (1003-1013)
- `stopFlock` in `cmd/fledge/main.go` (function): End flock by stopping its herdr session; managed sessions also deleted (1183-1222)
- `attachHerdr` in `cmd/fledge/main.go` (function variable): Spawn herdr UI process; stubbed for testing (909-932)
- `startAfterAttach` in `cmd/fledge/main.go` (function variable): Async orchestrator spawn goroutine; stubbed for testing (957-959)
- `managedOrchestratorRequest` in `cmd/fledge/main.go` (function): Build OpSpawn request for fledge-orchestrator with profile selection (803-828)
- `awaitSpawn` in `cmd/fledge/main.go` (function): Resolve interactive start outcome with buffered channel and rollback handling (778-801)
- `runAgentSpawn` in `cmd/fledge/main.go` (function): Spawn daemon-managed agent with agent/profile/model selection (1613-1753)
- `runAgentReady` in `cmd/fledge/main.go` (function): Authenticate readiness token; wait for first message unless --no-wait (1755-1816)
- `pickOrchestratorConfig` in `cmd/fledge/main.go` (function): Two-level interactive menu: Claude/Codex direct, Pi via submenu (2899-2958)
- `helpPages` in `cmd/fledge/main.go` (map): 32 help topics covering all commands and subcommands (32-497 in help.go)
- `scanContext` in `cmd/fledge/context.go` (function): Build workspace file list with ignore filtering and optional scope (201-251)
- `buildContextGraph` in `cmd/fledge/context.go` (function): Build directory tree with recursive sizes and file counts (312-345)
- `watchDaemonLog` in `cmd/fledge/watch.go` (function): Poll daemon.log, copy appended entries, stop when daemon down (54-94)
- `installWatcherPane` in `cmd/fledge/watch.go` (function): Split shell pane, start watcher command, focus orchestrator (101-126)

### Dependencies

- Internal `internal/agentcfg`: Load/sync agent definitions, model routing, profile configuration
- Internal `internal/catalog`: Model discovery from claude/pi/codex CLIs, catalog file writing
- Internal `internal/client`: Daemon IPC via Do() and Running() liveness probe
- Internal `internal/contextdoc`: Analyzer request/reply validation and composition
- Internal `internal/daemon`: Socket path computation, readiness signal publishing
- Internal `internal/flock`: Flock naming (SessionName, SessionPrefix), environment vars, directory layout
- Internal `internal/herdr`: Session lifecycle: Ensure, Recreate, Stop, Delete, Find, List, Up
- Internal `internal/herdrwire`: Pane operations: Create, PaneClose, PaneSplit, SendInput, PaneFocus, TabRename
- Internal `internal/ignore`: File ignore pattern matching (.fledgeignore)
- Internal `internal/protocol`: Request/Response/Agent/Message types, OpCodes (OpSpawn, OpRegister, OpReady, etc.)
- Internal `internal/scaffold`: .fledge directory structure constants and template names
- Internal `internal/scan`: File listing with ignore pattern filtering
- Internal `internal/version`: Version string for display and daemon verification
- Internal `internal/workspace`: Workspace root discovery via git-style walk and EvalSymlinks
- External `encoding/json`: JSON marshaling for --json output and daemon IPC
- External `os`: File I/O, executable detection, signal handling, environment vars
- External `os/exec`: Daemon spawn (setsid), herdr UI attachment, git risk checking
- External `os/signal`: NotifyContext for watch command interrupt handling
- External `bufio`: Interactive prompt reading from stdin
- External `fmt`: Formatted output and error messages
- External `strings`: String manipulation (TrimSpace, HasPrefix, Split, ReplaceAll)
- External `time`: Timeouts (5s daemon ready, 10s flock stop, 2s herdr attach), polling intervals
- External `path/filepath`: Path manipulation, symlink resolution (EvalSymlinks), directory walking
- External `sort`: Sorting agents, configs, files, help topics
- External `strconv`: String to int conversion for picker selections and ports
- External `syscall`: getsid syscall for session leader detection
- External `math`: Overflow checking for file size totals (MaxInt64)
- External `io`: io.Copy for daemon log streaming
- External `io/fs`: fs.DirEntry for .fledge tree walking
- External `errors`: Error wrapping and joining
- External `context`: Signal-driven context cancellation

### Data Flows

- `runStart entry` → `spawnDaemon`: Interactive start flows: workspace validation → session create/ensure → daemon spawn (setsid) → readiness poll
- `spawnDaemon` → `waitSpawnDaemonReady`: Daemon spawn triggers 5-second readiness poll; OpStatus must report matching session
- `runStart` → `managedOrchestratorRequest`: After daemon ready, build OpSpawn request for orchestrator with profile selection
- `attachHerdr` → `startAfterAttach`: Herdr UI attaches; once terminal owned, spawn orchestrator in background goroutine
- `startAfterAttach goroutine` → `awaitSpawn`: Orchestrator spawn result reported on buffered channel; awaitSpawn resolves with rollback guard
- `installWatcherPane` → `herdrwire operations`: Non-critical watcher setup: split shell (50% down), start watcher command, refocus orchestrator
- `runAgentSpawn` → `pickAgentDefinition/pickAgentConfig`: Bare spawn with terminal shows numbered menu; selection (by number or name) passed to OpSpawn
- `runAgentReady` → `client.Do(OpReady)`: Agent authenticates token; if --no-wait returns; else waits for OpReceive message
- `runAgentMsg*` → `agentMsgRequest (client.Do)`: send/reply/inbox/wait all route through injected agentMsgRequest function
- `runContextScan` → `scanContext`: Workspace scan → ignore filter → file list; optional scope narrows to subtree
- `runContextGraph` → `buildContextGraph`: Scan workspace → build tree structure with recursive size measurement
- `runContextCompose*` → `contextdoc validators`: Compose request/worksheet → validate JSON → atomic write via temp+rename
- `runWatch` → `watchDaemonLog`: Signal context + polling loop: initial io.Copy, then poll tail + check liveness

### Invariants

- Short flags are globally unique across entire CLI (per CLAUDE.md); -P for --pid, -D for --provider, preventing subcommand-level collisions
- Workspace root discovery is canonical via filepath.EvalSymlinks; client and daemon must agree for socket path hash and deterministic session selection
- Flag parsing exact-match only; takeFlag rejects leading - in values so -H in positional context is always the help flag, never a value
- Daemon socket lies outside workspace in $XDG_RUNTIME_DIR/fledge/<hash>/ because NFS cannot bind unix sockets; hash supports multiple concurrent workspaces
- Managed herdr sessions are recreated on every start (not reused) to prevent stale orchestrator pane identity collisions; operator-named sessions (--session) are reused
- Interactive start requires terminal stdout; non-terminal stdout skips orchestrator, attach, watcher; pre-daemon failures roll back; post-daemon failures preserve flock
- Destructive operations (flock clear, deinit, fresh init) require terminal I/O; no --force flag; scripted users use rm -rf or manual commands
- Agent spawn catalog works offline (models/types); agent register/ready/stop/msg all require FLEDGE_FLOCK environment and running daemon
- Analyzer requests and replies are strictly validated against JSON schemas; unknown fields, missing required fields, unsafe paths are rejected and correlated
- Help page resolution is exact-match with fallback; runHelp walks up command path to deepest match, falls back to nearest valid topic instead of root

### Tests

- `cmd/fledge/main.go`: Testable seams: stdoutIsTerminal, stdinIsTerminal, attachHerdr, startAfterAttach, stopHerdrSession function vars enable stubbing of terminal detection and interactive flow
- `cmd/fledge/main.go`: Flock clear testable via injected: clearFlockRunning, clearFlockSession, clearFlockRemoveAll, clearFlockOrphans, clearOrphanSession; allows testing without real daemon/herdr
- `cmd/fledge/main.go`: Daemon operations testable via injected: daemonStatusForCLI, spawnDaemonStatus, spawnDaemonSleep, restartDaemonStatus, restartDaemonShutdown, restartDaemonRunning, restartSpawnDaemon, restartSleep
- `cmd/fledge/main.go`: Agent operations testable via injected: agentSpawnRequest, agentMsgRequest; allows testing without running daemon
- `cmd/fledge/watch.go`: Watch testable via injected: runLogWatcher (watchDaemonLog), watchPollInterval; allows testing polling and log tail without real daemon
- `cmd/fledge/main.go`: Related tests in agent_definitions_test.go, workspace_test.go (from git status); likely test agent definitions and workspace discovery respectively

### Files

#### cmd/fledge/context.go

_text._ 432-line context analysis commands: analyzer request/worksheet composition (39-189), workspace scanning with ignore filtering and optional scope (201-251), directory graph generation with recursive size measurement (281-375), text rendering of tree structures (386-431). Helper types local to file (scannedContext, graphNode, graphDir). No external validation; contextdoc package handles schema checks. Self-contained; could be moved independently.

#### cmd/fledge/help.go

_text._ 560-line help system: rootHelp ASCII logo (8-30), 32 hard-coded help topics in helpPages map (32-497), usageError type with help context (499-516), help lookup and printing (518-559). Entire help text embedded; no external localization support. Adding a command requires manual map entry. Help resolution uses exact-match with fallback to nearest valid topic.

#### cmd/fledge/main.go

_text._ 2982-line command router, flag parser, flock/daemon lifecycle manager, agent spawner, interactive UI coordinator, context scanner. Sections: top-level dispatch (45-81), context subcommands (83-207), helpers (234-338), flock commands (370-550), interactive start (590-763), lifecycle guards (778-871), daemon ops (966-1127), flock stop/list/status (1153-1310), agent commands (1511-1753), agent models/types (1850-1882), agent messaging (1916-2056), init/deinit (2209-2391), model rendering (2540-2637), interactive pickers (2690-2958). Large file; could be split by domain. Heavy use of function-var seams for testing.

#### cmd/fledge/watch.go

_text._ 142-line daemon log watcher and pane integration: runWatch entry point (23-48), watchDaemonLog polling loop (54-94), watcher pane installation and split (101-126), error recovery with shell quoting (128-141). Self-contained; non-critical setup preserves flock on failure. 50% down pane split; no configuration for ratio.

## Subsystem: cmd-fledge-tests

Test suite for cmd/fledge CLI package covering agent lifecycle (registration, spawning, messaging, readiness), flock management (creation, listing, status, stopping, clearing), context analysis (scanning, graphing, composition, validation, rendering), CLI parser robustness (flag handling, validation, help pages), interactive orchestrator startup and placement, daemon spawn/restart/supervision, and terminal-based interaction (deinit, clear, stop).

**Purpose:** Test suite for the cmd/fledge CLI package covering agent definitions, behavior, clear, context pipeline, graph, parser, restart, scan, stop, watch, and workspace commands

### Entry Points

- `cmd/fledge/agent_definitions_test.go`: Agent types, definitions, registration, readiness protocol, socket vs fallback signal
- `cmd/fledge/behavior_test.go`: CLI operation tests: roster formatting, agent operations, messaging, flock listing
- `cmd/fledge/clear_test.go`: Flock state cleanup, orphan session management, interactive confirmation
- `cmd/fledge/context_pipeline_test.go`: Analyzer request/reply validation, worksheet composition, project rendering
- `cmd/fledge/graph_test.go`: File tree graphing, ignore semantics, scope navigation
- `cmd/fledge/main_test.go`: Help pages, profile pickers, init discovery, startup orchestration, agent rows
- `cmd/fledge/parser_test.go`: Flag parsing, value extraction, malformed input detection
- `cmd/fledge/restart_test.go`: Daemon replacement, liveness polling, verification, error guidance
- `cmd/fledge/scan_test.go`: File walking, ignore semantics, workspace discovery
- `cmd/fledge/stop_test.go`: Daemon shutdown, session cleanup, terminal confirmation
- `cmd/fledge/watch_test.go`: Log polling, pane layout, orchestrator attach, watcher command
- `cmd/fledge/workspace_test.go`: Daemon spawning, session binding, interactive start, placement rollback

### Key Symbols

- `captureRun` in `cmd/fledge/main_test.go` (function): Captures stdout/stderr from CLI run, returns output and error
- `takeFlag` in `cmd/fledge/main_test.go` (function): Extracts flag value from args, rejects flag-shaped values except stdin marker
- `pickAgentConfig` in `cmd/fledge/main_test.go` (function): Interactive profile selection via numbered menu or name
- `scaffoldedWorkspace` in `cmd/fledge/main_test.go` (function): Creates temp workspace with .fledge tree and subdirectory
- `stubStdinTerminal` in `cmd/fledge/main_test.go` (function): Toggles stdin TTY detection for interactive tests
- `startDaemon` in `cmd/fledge/main_test.go` (function): Runs in-process daemon with cleanup
- `interactiveStart` in `cmd/fledge/main_test.go` (function): Wires fake herdr session for interactive start test
- `liveSocket` in `cmd/fledge/main_test.go` (function): Fake herdr protocol responder recording requests
- `fakeHerdr` in `cmd/fledge/main_test.go` (function): Stub herdr CLI recording session launches and operations
- `pickOrchestratorConfig` in `cmd/fledge/main_test.go` (function): Orchestrator-only profile menu with pi browser

### Dependencies

- Internal `internal/agentcfg`: Profile catalog, index versioning, reserved agent names (fledge-orchestrator)
- Internal `internal/client`: RPC calls to daemon (Do, Running), liveness checks
- Internal `internal/daemon`: Daemon lifecycle (New, RunBound, SocketPath, ReadySignalPath)
- Internal `internal/flock`: Flock directory paths, listing, session names, prefixes
- Internal `internal/protocol`: Request/Response structures, operations, agent names, env vars
- Internal `internal/scaffold`: Directory tree initialization, .gitignore entries
- Internal `internal/workspace`: Git-style root discovery with EvalSymlinks
- Internal `internal/contextdoc`: Analyzer request/reply schemas, scan/render documents, validation
- Internal `internal/version`: Version.Get() for daemon restart verification
- External `encoding/json`: Marshaling profiles, requests, responses, graph structures
- External `os`: File I/O, pipes, environment, Stat, Mkdir, WriteFile
- External `path/filepath`: Path manipulation, symbolic link resolution with EvalSymlinks
- External `strings, bufio`: Text processing, line reading, field splitting
- External `net`: Unix socket listening for fake herdr protocol
- External `time`: Polling intervals, timeouts, sleep mocking in tests
- External `io`: Pipe reading for stdout/stderr capture
- External `sync`: Mutex-protected request recording in wireRecorder
- External `context`: Cancellation for log watcher polling
- External `errors`: Error wrapping and identity checks (Is)
- External `strconv`: PID and duration string conversion
- External `slices`: Slice equality assertions (Equal, Contains)

### Data Flows

- `CLI args` → `takeFlag, takeBoolFlag, rejectFlags`: Parser validates all flags up front, rejects flag-shaped values
- `Parsed args → run()` → `Command dispatch → handler`: Routes to agent types, spawn, stop, clear, init, etc.
- `Handler → client.Do() → daemon socket` → `RPC request`: Commands go through protocol to running daemon
- `Daemon journal → client.Running()` → `Liveness check`: Self-registered and spawned agent liveness via PID probe or event state
- `Interactive start → herdr session socket` → `wire protocol`: Orchestrator placement: agent.start → pane.swap → pane.focus
- `Watchdog poller → log file → watch output` → `Streaming lines`: Poll intervals yielded until context cancelled
- `Flock state → manifest files` → `Cleanup phases`: Session deletion before state removal (ordering invariant)
- `Analyzer request → response files` → `Workflow inputs`: File list, paths, sizes feed into analyzer assignment
- `Scan walk → ignore patterns → graph nodes` → `Tree construction`: Patterns applied bottom-up; ignored dirs fully pruned

### Invariants

- Flag parsing precedes command dispatch: flag-shaped values reject at parse time, not silently consumed
- Placed daemons not outside running roster: client.Running() uses journal and signal-0 probes
- Ready handshake order enforced: register → ready → receive → ack sequence exact
- Clear/stop only delete running-false targets: re-probed liveness before removal
- Session cleanup precedes state removal: clearFlocks() deletes session first, preserves on failure
- Managed sessions scoped to workspace: clearFlockOrphans() filters by hash prefix + saved flock list
- Interactive start attaches before spawn: orchestrator launch waits for Herdr UI live
- Workspace labels deterministic: fixed IDs in test fixtures (w1:p1, w1:p2)
- Ignore patterns applied at scan time: fully ignored subtrees yield single root with FileCount=0
- Orchestrator profile fallback: missing orchestrator-profile shows general picker menu

### Tests

- `cmd/fledge/agent_definitions_test.go`: Agent registration, metadata carrying, readiness protocol, socket/fallback signal handling (6 tests)
- `cmd/fledge/behavior_test.go`: Roster formatting, flock ops, human output, messaging correlation (10 tests)
- `cmd/fledge/clear_test.go`: Flock cleanup: confirmation, liveness, orphan sessions, ordering, partial failure (14 tests)
- `cmd/fledge/context_pipeline_test.go`: Analyzer request/reply validation, worksheet composition, graph rendering (13 tests)
- `cmd/fledge/graph_test.go`: File tree graphing, ignore patterns, scope handling (6 tests)
- `cmd/fledge/main_test.go`: Help pages, profile pickers, init discovery, startup orchestration, agent rows (60+ tests)
- `cmd/fledge/parser_test.go`: Flag parsing robustness, value extraction, malformed input (7 tests)
- `cmd/fledge/restart_test.go`: Daemon replacement, liveness polling, verification, error guidance (12 tests)
- `cmd/fledge/scan_test.go`: File walking, ignore semantics, workspace discovery (4 tests)
- `cmd/fledge/stop_test.go`: Daemon shutdown, session cleanup, terminal confirmation (8 tests)
- `cmd/fledge/watch_test.go`: Log polling, pane layout, orchestrator attach, watcher command (8 tests)
- `cmd/fledge/workspace_test.go`: Daemon spawning, session binding, interactive start, placement rollback (25+ tests)

### Files

#### cmd/fledge/agent_definitions_test.go

_text._ Tests agent definition loading, registration metadata carrying, readiness protocol with socket and fallback signal paths

#### cmd/fledge/behavior_test.go

_text._ Tests CLI operation: roster listing, agent register/list/stop, flock status/list, human output formatting, messaging

#### cmd/fledge/clear_test.go

_text._ Tests flock cleanup: confirmation flow, liveness re-checks, orphan session management, cleanup ordering, partial failures

#### cmd/fledge/context_pipeline_test.go

_text._ Tests analyzer request/reply validation, worksheet composition, graph rendering, context commands

#### cmd/fledge/graph_test.go

_text._ Tests file tree graphing with ignore semantics, scope selection, human and JSON output

#### cmd/fledge/main_test.go

_text._ Tests help pages, profile pickers, init discovery, startup orchestration, agent row formatting, ~1500 lines

#### cmd/fledge/parser_test.go

_text._ Tests CLI flag parsing: takeFlag robustness, flag-shaped value rejection, unknown flag detection

#### cmd/fledge/restart_test.go

_text._ Tests daemon replacement: readiness polling, verification, error guidance for spawn/version/session failures

#### cmd/fledge/scan_test.go

_text._ Tests file scanning: git-style workspace discovery, ignore pattern application, subtree listing

#### cmd/fledge/stop_test.go

_text._ Tests daemon shutdown: terminal confirmation, session cleanup, managed session deletion

#### cmd/fledge/watch_test.go

_text._ Tests log watcher: file polling, daemon shutdown notice, pane layout, orchestrator attach

#### cmd/fledge/workspace_test.go

_text._ Tests daemon spawning, session binding, interactive start flow, placement rollback, workspace discovery

## Subsystem: daemon-core-a

The daemon-core subsystem manages journal durability, agent placement in Herdr workspaces/tabs, and readiness handshaking. The journal is the append-only source of truth; every operation is journaled before client ack. Placement resolves workspace/tab labels, creates tabs if absent, and uses latches to serialize concurrent creates. Readiness authenticates spawned agents via one-use tokens, supporting both socket and file-based (sandbox fallback) paths.

**Purpose:** Daemon core: journal, placement, and readiness lifecycle in internal/daemon

### Entry Points

- `internal/daemon/journal.go`: replay(path) reconstructs daemon state from journal file for startup/recovery
- `internal/daemon/daemon.go`: Daemon.append(e event) atomically writes events with fsync before client ack
- `internal/daemon/placement.go`: Daemon.acquirePlacement() main entry for placing agents into workspace/tab with concurrent create serialization
- `internal/daemon/ready_signal.go`: WriteReadySignal() and consumeReadySignal() handle authenticated readiness via socket and file-based paths

### Key Symbols

- `state` in `internal/daemon/journal.go` (struct): Full daemon state reconstructed from journal: agents, messages, tokens, owned tabs, workspace/tab closures
- `event` in `internal/daemon/journal.go` (struct): Union type for all journal events (agent.registered, agent.ready, tab.created, msg.sent, etc.)
- `resolvedPlacement` in `internal/daemon/placement.go` (struct): Exact workspace/tab address with IDs and labels after label resolution
- `ownedTab` in `internal/daemon/placement.go` (struct): Metadata for tabs Fledge created, including RootPaneID for ephemeral shell cleanup
- `tabCreateLatch` in `internal/daemon/placement.go` (struct): Synchronization primitive serializing concurrent tab creates with the same label; waiters converge on one result

### Dependencies

- Internal `internal/herdrwire`: Socket API calls to Herdr (WorkspaceList, TabList, TabCreate, TabClose, WorkspaceClose, etc.)
- Internal `internal/protocol`: Message, Agent, Request, Response types; journal event serialization
- Internal `internal/agentcfg`: Agent configuration, ReservedOrchestrator constant, agent definition lookups
- Internal `internal/flock`: Flock directory paths, window title formatting
- Internal `internal/scaffold`: .fledge layout structure, catalog, agents directory names
- External `os`: File I/O for journal, ready signals, directory creation/permissions
- External `encoding/json`: Event marshaling/unmarshaling for journal and ready signals
- External `bufio`: Scanner for journal replay line-by-line reading
- External `sync`: Mutex for daemon state lock, WaitGroup for goroutine coordination
- External `crypto/sha256`: Hashing readiness tokens for credential storage

### Data Flows

- `Daemon.New()` → `replay(journalPath)`: Reconstruct roster, messages, tokens, owned tabs, closures from journal at startup
- `replay()` → `Daemon.recoverOwnedTabs()`: After state reconstruction, clean up crash-left ephemeral tabs and creation intents
- `Daemon.spawn()` → `Daemon.acquirePlacement()`: Resolve workspace/tab labels, create tab if absent, record placement for agent start
- `acquirePlacement()` → `Daemon.append(evTabCreateIntent/evTabCreated)`: Journal tab creation intent and final ownership atomically for crash recovery
- `spawned agent` → `Daemon.ready(token) or WriteReadySignal(token)`: Agent signals readiness via socket (normal) or file (sandbox fallback)
- `Daemon.ready()` → `Daemon.append(evReady)`: Validate token hash, journal readiness, convert token to credential
- `Daemon.stop(OwnsWorkspace=true)` → `Daemon.stopWorkspaceOwner()`: Cascade stop to nested agents, journal workspace.closing, then workspace.close RPC
- `Daemon.cleanupOwnedTab()` → `Daemon.append(evTabClosing/evTabClosed)`: Journal tab closure intent before RPC, then completion after idempotent Herdr close
- `recoverOwnedTabs()` → `Daemon.recoverTabCreateIntent()`: On startup, converge incomplete tab creations: 0 matches→resolve, 1 match→rollback, N>1→preserve

### Invariants

- Journal-before-ack: every state change fsync'd before client ack; torn final line tolerable, earlier lines authoritative
- Idempotent recovery: tab/workspace close checks Herdr inventory, not local state; already-closed treated as success
- Token single-use: readiness token→credential at evReady journal; replayed tokens rejected
- Ownership tracking: owned tabs journaled at create (evTabCreated) and close (evTabClosed) atomically
- Latch serialization: concurrent acquirePlacement() with same workspace/tab label converge via tabCreateLatch on one creation
- Workspace closure cascade: once evWorkspaceClosing journaled, all agents in workspace blocked from messaging
- Creation intent attribution: temporary unique label (fledge-create-<hex>) used to identify incomplete creates; recovery handles ambiguity safely

### Tests

- `internal/daemon/journal_test.go`: 8 tests: replay correctness (missing journal, roster rebuild, pending filtering, send order, legacy delivery), registration reuse, launch lifecycle (incomplete→orphaned, complete→starting)
- `internal/daemon/placement_test.go`: 25+ tests: label/ID resolution and ambiguity, targeted spawn (reuse/create/close tabs), concurrent create convergence, external creator races, validation, crash recovery (orphaned tabs, pending intents, idempotent closes), workspace cascade stop and messaging lockdown
- `internal/daemon/ready_test.go`: 20+ tests: token authentication (valid/invalid/replayed), spawned identity messaging, stopped identity lifecycle, parked wait delivery, file-based readiness signal, early readiness blocking, concurrent stop during launch, readiness timeout, orchestrator special handling (managed vs raw), lifecycle input suppression, bootstrap prompt injection

### Files

#### internal/daemon/daemon.go

_text._ Core daemon state machine (45KB): Daemon struct, socket/file request dispatch, agent lifecycle (register/stop), message routing (send/reply/inbox/wait/receive/ack), readiness waiter, identity auth (tokens/credentials), inboxNotification lifecycle. Manages both self-registered agents (Unix socket boundary) and spawned agents (launch credential). No inference; deterministic state advancement via journal.

#### internal/daemon/journal.go

_text._ Append-only event log (14KB): append()/appendAll() for atomic fsync writes, replay() for full state reconstruction from line-by-line journal. Handles torn final line gracefully (truncate+re-terminate). Tracks agents, messages, pending deliveries, tokens, owned tabs, workspace/tab closures. Supports legacy pane-delivery events and pi subprocess agents. Orphaned inference for incomplete launches and pane-less spawned agents.

#### internal/daemon/journal_test.go

_text._ Journal replay verification (5.5KB): fixture-based tests for empty journal, roster rebuild, pending filtering, send order preservation, legacy delivery finality, registration reuse, launch lifecycle (orphaned/starting state). Validates state reconstruction correctness.

#### internal/daemon/placement.go

_text._ Workspace/tab placement and lifecycle (26KB): acquirePlacement() resolves labels→creates tabs if absent→latches concurrent creates. Tab creation intent tracking for crash recovery. Owned tab tracking with ephemeral shell cleanup (finishOwnedTabSetup). Workspace owner stop with nested cascade. Idempotent tab/workspace close via Herdr inventory polling. Recovery: crash-left tab cleanup, creation intent convergence (0/1/N matches), tab close retry.

#### internal/daemon/placement_test.go

_text._ Placement and lifecycle tests (31KB): label/ID resolution (priority, ambiguity), targeted spawn (reuse existing, create/close owned, prevent duplicates), concurrent create convergence via latch, external creator races (rollback only Fledge tab), tab ownership lifecycle, workspace cascading stop, crash recovery (orphaned tabs, pending intents, idempotent closes), edge cases (external same-label, ambiguous temporary label, workspace closure races). Comprehensive concurrent scenarios with fake Herdr.

#### internal/daemon/ready_signal.go

_text._ Readiness authentication (3.7KB): WriteReadySignal() atomic write of hash(token) to workspace-local directory (sandbox fallback), consumeReadySignal() validation+removal, consumeReadySignals() batch resume after restart. Token→credential conversion at evReady. Legacy plain-digest format support.

#### internal/daemon/ready_test.go

_text._ Readiness handshake tests (30KB): token single-use+credential re-delivery, spawned identity messaging auth, stopped identity lifecycle, parked wait delivery (before/after ready), file-based signal without socket, early readiness blocking until spawn journaled, concurrent stop during launch, readiness timeout and species reuse, orchestrator special cases (managed=plugin+append-system-prompt, raw=message-wait), lifecycle input suppression, bootstrap prompt injection. Orchestrator readiness only at startup, managed variant no waiter.

## Subsystem: daemon-core-b

Daemon agent lifecycle subsystem (internal/daemon): coordinates spawning pane-hosted agents (Claude, Pi, Codex), validating readiness via one-use token, generating startup automation for managed orchestrators, and delivering durable inbox notifications. Five files implement the full lifecycle from reservation through teardown, plus flock-level isolation (separate journals, sockets, species pools).

**Purpose:** Daemon agent lifecycle: spawn, startup assets, inbox notifications, and isolation in internal/daemon

### Entry Points

- `internal/daemon/spawn.go`: spawn() entry point: reserve name, launch via Herdr, journal states, await readiness
- `internal/daemon/spawn.go`: ready()/readyDigest(): authenticate readiness token, transition to running, arm inbox delivery
- `internal/daemon/startup_assets.go`: orchestratorStartupArgs(): generate Claude plugins/Pi extensions for startup automation
- `internal/daemon/inbox_notify.go`: startInboxNotifier()/runInboxNotifier(): background worker for durable inbox notifications
- `internal/daemon/spawn.go`: Flock isolation boundary: each daemon has own journal, socket, roster, species pool

### Key Symbols

- `spawn` in `internal/daemon/spawn.go` (func): Main lifecycle: reserve name → launch Herdr pane/workspace → journal states → readiness loop
- `launchLatch` in `internal/daemon/spawn.go` (type): Synchronization primitive separating slow Herdr launch from spawn response
- `reserve` in `internal/daemon/spawn.go` (func): Claim name with reservedPID (-1) placeholder before lock-free launch
- `launch` in `internal/daemon/spawn.go` (func): Herdr calls (agent.start, workspace.create, pane ops) without d.mu held
- `readyDigest` in `internal/daemon/spawn.go` (func): State machine: token verify → launch await → ready event → inbox arm → running
- `markStopped` in `internal/daemon/spawn.go` (func): Journal stopped, clear readiness latches, cancel waiters
- `orchestratorStartupArgs` in `internal/daemon/startup_assets.go` (func): Returns argv for Claude/Pi orchestrators with startup automation files
- `writeRuntimeFile` in `internal/daemon/startup_assets.go` (func): Atomic write via temp-then-rename; creates parent dirs 0700
- `runInboxNotifier` in `internal/daemon/inbox_notify.go` (func): Main loop: dequeue task → wait readyAt → notifyInboxAgent() → retry on error
- `queueInboxWakeLocked` in `internal/daemon/inbox_notify.go` (func): Coalesce multiple sends into one task per agent; preserve backoff
- `notifyInboxAgent` in `internal/daemon/inbox_notify.go` (func): Extract pending messages, invoke wake callback, journal on success
- `retryInboxNotifyLocked` in `internal/daemon/inbox_notify.go` (func): Exponential backoff 25ms → 5s, cap at 31 attempts

### Dependencies

- Internal `internal/agentcfg`: Config, Workspace, ReservedOrchestrator, definition/profile loading
- Internal `internal/herdrwire`: Pane lifecycle (AgentStart, WorkspaceCreate, PaneClose, ProcessInfo, ReportMetadata, PaneSwap)
- Internal `internal/species`: Pick() for kebab-case suffix generation
- Internal `internal/flock`: Env constant, Dir(), journal path helpers
- Internal `internal/protocol`: Request, Response, Agent, Message, event marshaling
- External `filebridge`: Fallback RPC for sandboxed agents without socket access
- External `crypto/sha256, crypto/subtle`: Token hashing, constant-time compare
- External `context`: Cancellation for inbox notifier
- External `os, io, filepath`: File I/O, atomicity via temp-rename
- External `testing`: Fake Herdr server, test utilities

### Data Flows

- `spawn()` → `reserve()`: Claim name with reservedPID (-1) placeholder while holding d.mu
- `spawn()` → `launch()`: Call Herdr without d.mu; slow pane/workspace creation
- `launch()` → `journal(evSpawned)`: Journal after Herdr succeeds; spawned event is second atomic write
- `spawn()` → `readiness loop`: Await readiness signal via env-injected token (50ms polling)
- `ready()` → `readyDigest()`: Token verify → launch latch await → ready event → inbox arm
- `send()` → `queueInboxWake()`: Orchestrator only: queue/coalesce inbox notification task
- `runInboxNotifier()` → `notifyInboxAgent()`: Dequeue oldest task, invoke wake callback with target+metadata
- `notifyInboxAgent()` → `journal(evInboxNotified)`: Journal success; mark notified in-memory
- `retryInboxNotifyLocked()` → `queueInboxWakeLocked()`: Reschedule failed task with exponential backoff
- `replayInboxNotifications()` → `queueInboxWakeLocked()`: On restart: queue durable unnotified messages
- `stop()` → `herdrwire.PaneClose()`: Close spawned agent's pane
- `stop()` → `stopWorkspaceOwner()`: Close workspace if agent owns it
- `flock isolation` → `journal`: Each flock maintains own journal; replay filters by flock name
- `flock isolation` → `species pool`: Each flock type has independent species pool

### Invariants

- State atomicity: registered+launching are one pre-launch journal write; spawned is second post-launch
- Readiness one-use: token hash stored, deleted after verify, second call fails
- Name liveness: self-registered agents judged by PID; spawned agents by State (never PID for spawned)
- Species reuse: only when State ∈ {stopped, orphaned} or (self-registered && !alive(pid))
- Orchestrator naming: always bare (fledge-orchestrator, no species), one per flock
- Inbox body-free: wake callback never receives message bodies (metadata only: ID, From, To, ReplyTo)
- Message atomicity: delivered ≤1 time per mailbox (journaled inbox.notified ids + in-memory map)
- Flock isolation: roster, journal, socket, species pool all per-flock; no cross-flock visibility
- Workspace ownership: only spawned agents can own; closed on stop or launch failure
- Launch coordination: launchLatch ensures readiness/stop calls wait for slow Herdr.launch() to complete

### Tests

- `internal/daemon/spawn_test.go`: 60+ integration tests covering placement, orchestrator, launchers, concurrency, atomicity, readiness, orphans, mailbox
- `internal/daemon/startup_assets_test.go`: 7 tests validating Claude/Pi asset generation, JSON validity, script perms, no-trigger, Codex bootstrap
- `internal/daemon/inbox_notify_test.go`: 11 tests validating readiness+arming atomicity, body-free metadata, coalescing, retry, worker join, replay
- `internal/daemon/isolation_test.go`: 4 tests validating flock isolation: species pools, message boundaries, daemon death, restart replay

### Files

#### internal/daemon/inbox_notify.go

_text._ Durable orchestrator mailbox notifications (283 lines): exponential backoff (25ms → 5s, 31 attempts), coalescing, body-free metadata wake, journal recovery. Daemon close cancels flights and joins worker.

#### internal/daemon/inbox_notify_test.go

_text._ Inbox lifecycle tests (455 lines, 13k bytes): readiness+arming atomicity, body-free metadata, coalescing, retry, worker join, replay recovery, degraded mode.

#### internal/daemon/isolation_test.go

_text._ Flock isolation validation (146 lines): separate species pools, message boundaries, daemon death isolation, restart replay. Confirms complete flock-level separation.

#### internal/daemon/spawn.go

_text._ Core lifecycle (956 lines): reserve → launch Herdr panes/workspaces → readiness token validation. Orchestrator special-cased for bare naming, workspace ownership, inbox arm. All Herdr calls run without d.mu. Readiness token injected via env, hashed SHA256, constant-time verified. Launch latch synchronization for readiness/stop. Supports Claude/Pi/Codex with role-specific argv. Message delivery via mailbox only.

#### internal/daemon/spawn_test.go

_text._ Comprehensive integration tests (2289 lines, 77k bytes): 60+ scenarios for placement, orchestrator, launchers, concurrency, atomicity, readiness, orphans, isolation, replay. Uses fake Herdr server to intercept calls.

#### internal/daemon/startup_assets.go

_text._ Generates startup automation (157 lines): Claude plugin.json + hooks.json + ready.sh (0700); Pi readiness.ts (TypeScript, session_start, no triggerTurn, nextTurn); Codex positional bootstrap. Atomic temp-then-rename writes.

#### internal/daemon/startup_assets_test.go

_text._ Validates asset generation (199 lines): JSON validity, script perms, hooks structure, Pi extension details, Codex bootstrap, write failure prevention.

## Subsystem: daemon-tests-misc

Nine test files covering daemon lifecycle, robustness, and message durability: initialization and validation (boundary); managed context orchestration (context_message); message durability under append failures (delivery_order); comprehensive integration testing (e2e); dedicated workspace spawning (forager); structured reply validation (reply); serve loop error handling and shutdown (serve); socket path resolution and concurrent election (socket); session liveness and window branding (watch). Together these tests ensure the daemon is bulletproof in startup, messaging, restart recovery, corruption tolerance, and graceful shutdown.

**Purpose:** Remaining daemon test coverage: boundary, context messages, delivery order, e2e, forager, reply, serve, socket, and watch in internal/daemon

### Entry Points

- `internal/daemon/boundary_test.go`: Daemon initialization guards: scaffold validation, flock name format, socket path size limit, malformed client handling, scaffolding requirement
- `internal/daemon/context_message_test.go`: Managed context (analyzer) message lifecycle: request schema validation, reply correlation, traffic rejection before journal entry
- `internal/daemon/delivery_order_test.go`: Message durability under failures: delivery append failure recovery, retry-ability, restart replay, once-only delivery guarantee
- `internal/daemon/e2e_test.go`: End-to-end roster and messaging: species assignment, exact correlation, restart recovery, abandoned waiter detection, journal corruption tolerance
- `internal/daemon/forager_test.go`: Dedicated workspace spawn: prompt-based launch without pane input, readiness timeout with workspace rollback
- `internal/daemon/reply_test.go`: Structured reply validation: inbox claim requirement, inbound identity verification, correlation derivation
- `internal/daemon/serve_test.go`: Serve loop robustness: accept error handling, response write deadline protection, shutdown semantics, ownership transfer ordering
- `internal/daemon/socket_test.go`: Socket path resolution and collision avoidance: concurrent election, permission hardening, deep workspace support, path stability
- `internal/daemon/watch_test.go`: Session liveness and window branding: probe-based exit, title landing caching, client attachment retry logic, graceful goroutine shutdown

### Key Symbols

- `daemon.New` in `internal/daemon/boundary_test.go` (function): Daemon constructor: validates scaffold, socket path, binds socket, elects winner on concurrent calls
- `daemon.Run` in `internal/daemon/boundary_test.go` (function): Standalone daemon runner: requires scaffolding before serving
- `register` in `internal/daemon/e2e_test.go` (function): Test helper: registers agent with type, returns assigned name (type-species format)
- `workspace` in `internal/daemon/e2e_test.go` (function): Test helper: creates scaffolded temp workspace
- `start` in `internal/daemon/e2e_test.go` (function): Test helper: starts daemon in background, returns stop func
- `registerManagedContextPair` in `internal/daemon/context_message_test.go` (function): Test helper: registers forager and analyzer agents for context flow
- `failDeliveredJournal` in `internal/daemon/delivery_order_test.go` (type): Injectible journal wrapper: fails msg.delivered writes to simulate append failure
- `scriptedListener` in `internal/daemon/serve_test.go` (type): Mock listener for Serve error testing: returns scripted accept() results
- `TestConcurrentNewElectsOneWinner` in `internal/daemon/socket_test.go` (test): Concurrent New() calls: exactly one winner (stale socket reclaim serialized)
- `TestWatchSessionSetsWindowTitleOnce` in `internal/daemon/watch_test.go` (test): Window title lands on attached client; cached and not retried

### Dependencies

- Internal `internal/client`: Client-side socket operations (Do, Running, ErrNotRunning)
- Internal `internal/protocol`: Request/Response/Agent/Message types, Op constants (OpRegister, OpSend, OpWait, OpReply, OpInbox, OpReceive, OpAck, OpList, OpStatus, OpShutdown), event type constants
- Internal `internal/scaffold`: Workspace initialization, .fledge tree structure (DirName), Ensure helper
- Internal `internal/filebridge`: Sandboxed client fallback (Submit, Await, Cleanup, Awaiting, liveness polling)
- Internal `internal/flock`: Flock name validation and constants (MaxName)
- Internal `internal/version`: version.Get() for status reporting
- Internal `internal/agentcfg`: Agent catalog, config structures, writeCatalog test helper
- Internal `internal/herdr`: Herdr CLI session lifecycle (workspace.create, agent.start via serveHerdr mock)
- External `net`: Unix socket listener, dialer, pipe, error types (ErrClosed)
- External `encoding/json`: Request/response marshaling and unmarshaling
- External `os`: File operations (permissions, PID, env), exec for helper process spawning
- External `sync`: Mutex, atomic types (Bool, Int32), waitgroup for concurrency
- External `time`: Ticker, timeout, deadline, sleep for timing control
- External `syscall`: Signal 0 probe for liveness, EINVAL and other error codes
- External `io`: Reader/Writer interfaces, Discard sink

### Data Flows

- `daemon startup` → `socket bind`: New() probes XDG_RUNTIME_DIR path, checks scaffold, tightens permissions, binds socket; concurrent calls race to bind (winner = first successful bind)
- `journal replay` → `daemon state`: On startup, replay agent.registered events rebuild roster; replay msg.sent events rebuild pending messages
- `send request` → `pending messages`: send() journals msg.sent atomically, adds to pending slice; journals before return (durability invariant)
- `pending messages` → `delivery`: Parked wait/receive/ack handlers wake, attempt delivery, journal msg.delivered on success; if journal append fails, message stays pending
- `wait request` → `delivery or timeout`: wait() parks with exact correlation (From, ReplyTo); wakes on message match or timeout; skipped messages remain pending
- `shutdown request` → `listener close`: dispatch(OpShutdown) closes listener, which closes accept loop; parked waiters released with error; active requests must drain before ownership transfer
- `watch probe` → `serve exit`: WatchSession calls probe() on tick; if probe returns true (session gone), close listener and Serve exits
- `title landing` → `title cache`: Window title set request lands (changed=true) and daemon caches titled=true, stops retrying

### Invariants

- Once-only delivery: messageDelivered map ensures each message ID delivered at most once, survives retries and restarts
- Append-only journal: msg.sent journals before send() returns, establishing durability; corrupted final lines (torn, unterminated) are recovered on replay by truncation or re-termination
- Exact correlation strictness: wait(As, From, ReplyTo) matches sender and replyTo exactly; partial matches rejected; uncorrelated messages remain pending for later matching
- Socket uniqueness per flock: concurrent New() calls serialize stale socket reclaim (probe, unlink, bind), electing exactly one winner
- Permission hardening: flock directory and journal upgraded from 0o755 to 0o700 and 0o644 to 0o600 on startup if stale
- Species pool per type: each agent type has its own 18-slot pool; species override allows explicit request; dead PID releases slot for reclaim
- No pane input on prompt launch: dedicated workspace with prompt instructions uses instructions only, no pane.send_input calls
- Filebridge liveness polling: daemon probes bridge waiter pid every ~250ms; dead process means waiter dropped, message not swallowed
- Structured reply validation: reply JSON validated against schema before journal entry; group_id must match original dispatch
- Shutdown ownership transfer: active requests (beginRequest/endRequest refcount) must drain before listener release; filebridge drain completes before ownership passes to new daemon
- Watch graceful exit: Close() stops watch goroutine via done channel, no leak

### Tests

- `internal/daemon/boundary_test.go`: TestNewRejectsMissingScaffold: New() fails if .fledge/ missing
- `internal/daemon/boundary_test.go`: TestNewRejectsInvalidFlockNames: rejects empty, uppercase, dash-containing, >32-char names
- `internal/daemon/boundary_test.go`: TestNewRejectsOversizedSocketPath: rejects path >103 bytes
- `internal/daemon/boundary_test.go`: TestUnknownOperationReturnsDaemonError: unknown Op returns protocol error
- `internal/daemon/boundary_test.go`: TestMalformedClientRequestIsDroppedWithoutStoppingDaemon: non-JSON request dropped; daemon continues
- `internal/daemon/boundary_test.go`: TestRunRequiresScaffoldingBeforeServing: Run() fails if scaffold absent
- `internal/daemon/context_message_test.go`: TestManagedContextSendRejectsInvalidTrafficBeforeJournal: malformed JSON, missing instructions_before, mismatched group_id rejected before journal
- `internal/daemon/context_message_test.go`: TestManagedContextStructuredReplyValidatesBeforeJournal: malformed reply JSON rejected before journal entry
- `internal/daemon/delivery_order_test.go`: TestSendToParkedWaiterDeliveryAppendFailureRemainsRetryable: delivery append failure leaves message pending; parked waiter and retry waiter both get it
- `internal/daemon/delivery_order_test.go`: TestReplyToParkedWaiterDeliveryAppendFailurePreservesCorrelation: failed reply append keeps reply_to correlation intact
- `internal/daemon/delivery_order_test.go`: TestWaitDeliveryAppendFailureLeavesPendingForRetry: failed wait append leaves message pending for retry
- `internal/daemon/delivery_order_test.go`: TestDeliveryAppendFailureReplaysDurableSendAsPending: restart recovers durable send from before delivery append, replays as pending
- `internal/daemon/e2e_test.go`: TestDaemonDownIsHardError: no daemon running returns ErrNotRunning
- `internal/daemon/e2e_test.go`: TestRegisterAssignsSpeciesPerType: engineer-emperor, engineer-king per type; reviewer-emperor; species override honored
- `internal/daemon/e2e_test.go`: TestRegisterRejectsBadType: rejects empty, leading dash, double dash, uppercase, underscore
- `internal/daemon/e2e_test.go`: TestDeadAgentReleasesItsName: PID 0 registration born dead; name reclaimed by next registration
- `internal/daemon/e2e_test.go`: TestPoolExhaustsAtNineteen: 18 registrations succeed; 19th fails; different type unaffected
- `internal/daemon/e2e_test.go`: TestSendToUnknownAgentFails: rejects unknown to or from agent
- `internal/daemon/e2e_test.go`: TestSendThenWaitDelivers: single delivery; second wait times out (once-only)
- `internal/daemon/e2e_test.go`: TestWaitBlocksUntilSend: wait parks; wakes on send
- `internal/daemon/e2e_test.go`: TestWaitReplyToAndSenderRequireExactCorrelation: uncorrelated messages skip; wrong sender rejected; correlated delivers; skipped messages remain pending
- `internal/daemon/e2e_test.go`: TestWaitTimesOut: timeout honored; pending message remains
- `internal/daemon/e2e_test.go`: TestWaitAsUnknownAgentFails: unknown agent rejected
- `internal/daemon/e2e_test.go`: TestRestartReplaysRosterAndPending: roster and undelivered messages survive daemon restart
- `internal/daemon/e2e_test.go`: TestRestartDoesNotRedeliver: delivered message not redelivered after restart
- `internal/daemon/e2e_test.go`: TestSecondDaemonRefusesLiveSocket: concurrent New() blocks until first releases socket
- `internal/daemon/e2e_test.go`: TestReplayToleratesTornFinalLine: torn final line ignored; earlier events replayed
- `internal/daemon/e2e_test.go`: TestTornTailTruncatedAcrossRestart: torn tail truncated on replay; next append does not fuse corrupted bytes
- `internal/daemon/e2e_test.go`: TestUnterminatedFinalLineReterminated: unterminated valid event re-terminated; next append does not fuse
- `internal/daemon/e2e_test.go`: TestAbandonedWaiterDoesNotSwallowMessages: socket waiter abandoned (connection closed) means waiter dropped; live wait gets message
- `internal/daemon/e2e_test.go`: TestAbandonedBridgeWaiterDoesNotSwallowMessages: filebridge waiter abandoned (Cleanup called) means waiter dropped; live wait gets message
- `internal/daemon/e2e_test.go`: TestKilledBridgeWaiterDoesNotSwallowMessages: filebridge waiter killed (pid dies) means liveness probe detects; waiter dropped; live wait gets message
- `internal/daemon/forager_test.go`: TestDedicatedWorkspaceUsesLaunchPromptWithoutPaneInput: prompt-based launch does not send pane.send_input
- `internal/daemon/forager_test.go`: TestDedicatedWorkspaceReadinessTimeoutClosesWorkspace: readiness timeout means workspace.close and agent stopped
- `internal/daemon/reply_test.go`: TestStructuredReplyDerivesClaimedInboundSenderAndCausality: reply before inbox claim fails; after claim, derives from/to/replyTo
- `internal/daemon/reply_test.go`: TestStructuredReplyRejectsMessageInboundToAnotherIdentity: reply from non-recipient agent rejected
- `internal/daemon/serve_test.go`: TestServeReturnsNonTemporaryAcceptError: non-temporary accept error terminates Serve with error
- `internal/daemon/serve_test.go`: TestServeRetriesTemporaryAcceptError: temporary error (EMFILE) retried; clean close returns nil
- `internal/daemon/serve_test.go`: TestHandleWriteDeadlineFreesBlockedWriter: response write deadline frees blocked handler (non-reading client)
- `internal/daemon/serve_test.go`: TestStatusReportsDaemonProcessAndVersion: status Op returns daemon PID and version
- `internal/daemon/serve_test.go`: TestFileBridgeStatusReportsDaemonProcessAndVersion: filebridge status Op returns daemon PID and version
- `internal/daemon/serve_test.go`: TestFileBridgeShutdownRespondsBeforeClose: filebridge shutdown Op responds before listener close
- `internal/daemon/serve_test.go`: TestShutdownReleasesParkedWaiters: shutdown Op releases parked waiters with error
- `internal/daemon/serve_test.go`: TestShutdownHoldsOwnershipUntilActiveRequestsDrain: shutdown waits for active requests to drain before ownership passes to replacement
- `internal/daemon/serve_test.go`: TestFileBridgeDrainCompletesBeforeOwnershipPasses: filebridge request drain completes before ownership transfer; old daemon writes no journal after replacement binds
- `internal/daemon/socket_test.go`: TestConcurrentNewElectsOneWinner: burst of 12 concurrent New() calls means exactly 1 daemon comes up
- `internal/daemon/socket_test.go`: TestFlockDirAndJournalPermissions: 0o755 to 0o700 flock dir, 0o644 to 0o600 journal on startup
- `internal/daemon/socket_test.go`: TestSocketPathsDifferPerWorkspace: different roots hash to different socket paths
- `internal/daemon/socket_test.go`: TestSocketPathIsStableAcrossEquivalentRoots: relative and symlink resolution produce same socket path
- `internal/daemon/socket_test.go`: TestDeepWorkspaceRunsDaemon: workspace >100 chars works end-to-end
- `internal/daemon/socket_test.go`: TestWorstCaseSocketPathFitsDarwinLimit: max flock name + deep workspace fits 103-byte darwin sun_path limit
- `internal/daemon/watch_test.go`: TestWatchSessionExitsWhenSessionGone: probe returns true means Serve exits with nil
- `internal/daemon/watch_test.go`: TestWatchSessionKeepsServingWhileSessionUp: probe returns false means Serve keeps running
- `internal/daemon/watch_test.go`: TestWatchSessionSetsWindowTitleOnce: title lands (changed=true) means cache titled=true, no retries
- `internal/daemon/watch_test.go`: TestWatchSessionRetriesWindowTitleUntilAttached: no foreground client means keep retrying title
- `internal/daemon/watch_test.go`: TestWatchSessionUnboundSetsNoTitle: unbound daemon (no session) means sets no title
- `internal/daemon/watch_test.go`: TestWatchSessionStopsOnClose: Close() stops watch goroutine, no leak

### Files

#### internal/daemon/boundary_test.go

_text._ Boundary and initialization guard tests: daemon.New() validates scaffold presence, flock name format (lowercase, no dash, at most 32 chars), socket path size (at most 103 bytes); daemon.Run() requires scaffolding; malformed client requests (non-JSON) are dropped without crashing; unknown operations return protocol error.

#### internal/daemon/context_message_test.go

_text._ Managed context orchestration tests: registers forager and analyzer agents; sends analyzer requests with validation of JSON schema and required fields (instructions_before, instructions_after); validates replies before journaling (schema, group_id correlation); tests rejection of invalid traffic before journal entry to preserve causality.

#### internal/daemon/delivery_order_test.go

_text._ Message durability and recovery tests: injects delivery-append failures to test retry-ability of messages and exact recovery of reply_to correlation; verifies messages stay pending when delivery journals fail; confirms durable sends survive daemon restart and replay as pending; tests receive/ack idempotence and once-only delivery guarantee.

#### internal/daemon/e2e_test.go

_text._ Comprehensive integration tests (largest suite, 22.9k): species assignment per type with pool exhaustion; exact send/wait correlation with partial-match rejection and uncorrelated pending retention; restart replay of roster and pending; reduplicate prevention; abandoned waiter detection (socket connection close and filebridge cleanup/pid liveness); journal corruption tolerance (torn final line, unterminated line).

#### internal/daemon/forager_test.go

_text._ Dedicated workspace spawn tests: verifies prompt-based launch path uses instructions without pane.send_input calls; tests readiness timeout handling with workspace rollback (workspace.close called on timeout).

#### internal/daemon/reply_test.go

_text._ Structured reply validation tests: requires inbox claim before reply (claim derives inbound sender); verifies from/to/replyTo correlation; rejects replies from wrong identity (message inbound to different agent).

#### internal/daemon/serve_test.go

_text._ Serve loop robustness tests: scripted listener for error injection; non-temporary accept errors terminate with error; temporary errors (EMFILE) retried; response write deadline protection for non-reading clients; status Op reports daemon PID/version; filebridge path support; shutdown semantics (releases waiters, drains active requests before ownership transfer).

#### internal/daemon/socket_test.go

_text._ Socket path resolution and collision avoidance tests: concurrent New() serializes stale socket reclaim to one winner; permission hardening (0o700 dir, 0o600 journal); path stability across equivalent roots (relative, symlink resolution); deep workspace support (over 100 chars); worst-case path (max flock name in deep workspace) fits darwin 103-byte sun_path limit.

#### internal/daemon/watch_test.go

_text._ Session liveness and window branding tests: WatchSession probes session, exits Serve when gone, keeps serving while up; window title set once on client attachment and cached (no retries); retries before client attaches (no foreground client); unbound daemon (no session) sets no title; Close() stops watch goroutine gracefully (no leak).

## Subsystem: docs

Legacy Stage 0 design documentation: complete specification of Fledge orchestrator authority model, zero-inference rule, integration surfaces (Herdr/Pi/Claude), and three resolved experiments. All docs carry forward verified findings; Stage 1 deferred. No Stage 1 placeholder packages created. Invariants: Go CLI is state authority; Herdr is UI/pane bus; Pi has native Herdr lifecycle authority; Claude panes use metadata-only to preserve screen-manifest blocked detection. All three experiments executed successfully on 2026-07-18; ADR-012/013/014 resolved.

**Purpose:** Legacy Stage 0 design docs and fixed reference snapshots under docs/

### Entry Points

- `docs/ARCHITECTURE.md`: Authority-split invariants, zero-inference rule, data/event flow diagram (3 paths), staged roadmap Stage 0–4.
- `docs/DECISIONS.md`: 17-entry ADR log (newest first); all statuses resolved. Key: ADR-012 EXP1 (native detection overrides), ADR-013 EXP2 (3/3 reliable), ADR-014 EXP3 (no practical cap).
- `docs/EXPERIMENTS.md`: Three supervised experiments with raw results: EXP1 (2026-07-18 02:46 UTC, custom report ineffective), EXP2 (02:48 UTC, 3/3 sends), EXP3 (n=2,3 no throttle observed).
- `docs/INTEGRATION-CONTRACTS.md`: Pinned version snapshot (2026-07-17): Herdr v0.7.4 protocol 16, Pi v0.80.x, Claude Code ≥v2.1.212. Surface details, examples, soft spots. Herdr verified live 2026-07-18.
- `docs/reference/integration-surfaces.md`: Immutable research snapshot (2026-07-17): four workstreams (Herdr socket API, Pi RPC/extensions, Claude hooks/headless, prior art + Go libraries), authority-split architecture fit, staged Stage 0–4 recommendations.
- `docs/reference/ai-sdlc-scan.md`: Immutable research snapshot (2026-07-17): tooling evolution June 15–July 17 2026 (Claude Code, Codex, Herdr, Pi versions), model releases (Sonnet 5, GPT-5.6, Fable 5), recommendations.

### Key Symbols

- `Zero-inference invariant` in `docs/ARCHITECTURE.md` (principle): Go CLI may only: issue Herdr socket commands, consume Herdr/agent events, advance deterministic FSM, write event log. Must never: make LLM API calls (all inference in agent panes), treat Herdr as durable truth, hand-drive agents invisibly.
- `Authority-split (Go/Herdr/Pi/Claude)` in `docs/ARCHITECTURE.md` (principle): Go CLI is state authority (SQLite log is truth). Herdr is UI/pane plumbing (events are input signals). Pi panes: Herdr's bundled extension is lifecycle authority (idle/working/blocked). Claude panes: no lifecycle authority in Herdr; use metadata-only to preserve screen-manifest blocked detection.
- `pane.report_agent --source custom:*` in `docs/INTEGRATION-CONTRACTS.md` (operation): Herdr socket call that seizes lifecycle authority on a pane. On Claude panes, native screen-manifest detection takes precedence (EXP1 verdict); custom report accepted but ineffective. Must be paired with cleanup (pane.clear_agent_authority/pane.release_agent).
- `pane.report_metadata` in `docs/ARCHITECTURE.md` (operation): Herdr socket call for display-only metadata (title, display_agent, state_labels, tokens). Does NOT seize authority. Recommended method for Claude panes under metadata-only strategy.
- `pane.send_input {text, keys:[]}` in `docs/INTEGRATION-CONTRACTS.md` (operation): Herdr socket call to inject text + encoded keypresses in one request. EXP2 verified 3/3 reliable for interactive Claude panes. Real Enter keypress required (Ink TUI does not treat programmatic \r as submit).
- `Rate-limit handling` in `docs/INTEGRATION-CONTRACTS.md` (concern): Claude Code subscription pooled: 5-hour rolling window + weekly cap shared across all Claude Code sessions and Claude chat on account. Parallel subagent fan-out documented exhaustion cause. Handle reactively via StopFailure/rate_limit hook (no pre-fixed concurrent-pane cap per EXP3).
- `Worktree isolation` in `docs/reference/integration-surfaces.md` (pattern): Each agent gets isolated git worktree with shared .git object store. One-branch-at-a-time merge prevents .git/index.lock contention. File ownership declared upfront; shared files (lockfiles, migrations, config) sequenced, never parallelized.
- `Cross-vendor review` in `docs/reference/ai-sdlc-scan.md` (pattern): Planner/writer from one vendor (Pi), reviewer from different vendor (Claude/Codex), Go orchestrator gates merge. Omnigent 'Polly' reference implementation (June 13 open-source).
- `File-lock coordinator` in `docs/reference/integration-surfaces.md` (pattern): Out-of-process Go relay: FSM routing, SQLite event log + task/ownership tables, gofrs/flock namespace locks. Enforces one-agent-per-file-namespace; matches practitioner consensus pattern.
- `ADR-012 verdict (EXP1)` in `docs/DECISIONS.md` (finding): Custom pane.report_agent on Claude pane does NOT suppress screen detection; native Herdr screen-manifest detection takes precedence. Custom report accepted but ineffective. Metadata-only rule (ADR-004) confirmed safe. Resolved 2026-07-18 02:46 UTC.
- `ADR-013 verdict (EXP2)` in `docs/DECISIONS.md` (finding): pane.send_input {text, keys:["enter"]} submits reliably 3/3 times to interactive Claude pane. Ink TUI limitation (bare \r does not submit) confirmed overcome by real Enter keypress. Claude workers can run in visible panes. Resolved 2026-07-18 02:48 UTC.
- `ADR-014 verdict (EXP3)` in `docs/DECISIONS.md` (finding): Concurrent Claude panes at n=2 and n=3 sustained showed no throttle signal before operator stopped. No practical concurrency ceiling found for Fledge's needs. Rate limits handled reactively via StopFailure/rate_limit hook, not pre-capping fan-out. Resolved 2026-07-18 ~03:14 UTC.

### Dependencies

- Internal `docs/DECISIONS.md`: Incorporates experiment results from EXPERIMENTS.md; ADRs reference handoff-stage0 commission + ground rules.
- Internal `docs/EXPERIMENTS.md`: Procedures defined in handoff-stage0 §6; run results resolve experiment flip thresholds (ADR-012/013/014).
- Internal `docs/INTEGRATION-CONTRACTS.md`: API surface details sourced from reference/integration-surfaces.md; verification status updated post-EXP1/EXP2.
- Internal `docs/reference/integration-surfaces.md`: Foundation for all three integration-contract sections (Herdr, Pi, Claude); immutable research input (2026-07-17 snapshot).
- Internal `docs/reference/ai-sdlc-scan.md`: Environmental context used by handoff and distilled docs; immutable snapshot (2026-07-17) documenting tooling evolution June 15–July 17 2026.
- External `Herdr v0.7.4`: Socket protocol v16; schema dumped to internal/herdrclient/herdr-schema.json (committed); types generated. Verified live 2026-07-18 via EXP1/EXP2.
- External `Pi v0.80.x`: RPC mode (JSONL), extension event bus, Herdr lifecycle integration v2. Pinned in INTEGRATION-CONTRACTS; not yet live-verified in Stage 0.
- External `Claude Code v2.1.212+`: 30+ hook events, stream-json output, session resume by ID. Pinned in INTEGRATION-CONTRACTS; verified via EXP1/EXP2 runs.

### Data Flows

- `Go Orchestrator` → `Herdr server`: Socket commands: agent.start, pane.split, pane.send_input (text+keys), pane.report_metadata (display-only), events.subscribe setup. One request per connection; newline-delimited JSON.
- `Agent panes` → `Go Orchestrator`: Claude: hooks POST event JSON to relay HTTP endpoint (Stop, Notification, PermissionRequest, StopFailure/rate_limit). Pi: RPC events stream to relay (subprocess JSONL, LF-framed).
- `Herdr server` → `Go Orchestrator`: session.snapshot (one-time bootstrap: version/protocol/pane/agent records), events.subscribe (pane.agent_status_changed, pane.output_matched, worktree.*, layout.updated as input signals).
- `Go Orchestrator` → `SQLite event log`: Write: agent lifecycle events, task state, file ownership, flock acquisitions. Read: source of truth for orchestration state. Log survives Herdr restarts.
- `Go Orchestrator` → `gofrs/flock`: Acquire/release namespace locks per file; enforces one-writer-per-file-namespace. Non-blocking TryLock()/TryRLock() for concurrent tasks.

### Invariants

- Go CLI is the state authority. Herdr events and agent hook/RPC events are input signals only. SQLite event log + tasks are truth.
- Pi panes: trust Herdr's native bundled-extension lifecycle authority (idle/working/blocked). Never report custom state onto Pi panes.
- Claude panes: use pane.report_metadata (display-only) to preserve Herdr's screen-manifest blocked detection for permission prompts. Do NOT seize authority with pane.report_agent --source custom:* (unless deliberately taking over, paired with cleanup).
- Zero-inference on Go CLI: issue Herdr socket commands, consume Herdr/agent events, advance deterministic FSM/workflow, write event log, acquire/release locks. NEVER: make LLM API calls (all inference in visible agent panes), treat Herdr as durable truth, hand-drive agents invisibly.
- Authority seizure (if any): always pair pane.report_agent --source custom:* with pane.clear_agent_authority / pane.release_agent on exit to restore native detection.
- Rate limits are pooled (5h rolling + weekly cap shared across all Claude Code sessions + Claude chat on account) and parallel-hostile. Handle reactively via StopFailure/rate_limit hook. No pre-fixed concurrent-pane cap.
- Worktree isolation: each agent gets own git worktree with shared .git; one-branch-at-a-time merge; file ownership declared upfront; enforce with gofrs/flock per-namespace locks.

### Tests

- `docs/EXPERIMENTS.md`: EXP1 (2026-07-18 02:46 UTC): supervised test of pane.report_agent authority override on Claude pane. Verdict: native detection takes precedence; custom report ineffective. Resolved ADR-012.
- `docs/EXPERIMENTS.md`: EXP2 (2026-07-18 02:48 UTC): supervised test of pane.send_input {text, keys:["enter"]} reliability on interactive Claude pane. Verdict: 3/3 gated sends submitted successfully. Resolved ADR-013.
- `docs/EXPERIMENTS.md`: EXP3 (2026-07-18 ~03:14 UTC): sustained-load test at n=2,3 concurrent Claude panes. Verdict: no throttle observed; no practical ceiling for orchestrator concurrency. Handle limits reactively. Resolved ADR-014.

### Files

#### docs/ARCHITECTURE.md

_text._ Distilled Stage 0 design: authority-split invariants (Go/Herdr/Pi/Claude), zero-inference rule, data/event flow (3 paths: Go→Herdr, Agent→Go, Herdr→Go), staged roadmap (Stage 0–4). Carries forward core findings; Stage 1 explicitly deferred.

#### docs/DECISIONS.md

_text._ ADR log (17 entries, newest first): accepted decisions (Stage 0 scope, Go authority, Pi/Claude splits, reference immutability, no Stage 1 packages) and three experiment flip thresholds, all now resolved with run verdicts (ADR-012/013/014 2026-07-18).

#### docs/EXPERIMENTS.md

_text._ Three supervised experiments: EXP1 (authority override, custom report ineffective, native detection overrides), EXP2 (interactive input 3/3 reliable), EXP3 (n=2,3 sustained no throttle). Raw observations, operator confirmations, verdicts. All complete 2026-07-18.

#### docs/INTEGRATION-CONTRACTS.md

_text._ Pinned version snapshot (2026-07-17, Herdr verified live 2026-07-18): three sections (Herdr v0.7.4 protocol 16, Pi v0.80.x, Claude Code ≥v2.1.212) with surface details, invocation examples, version/stability caveats, soft spots flagged.

#### docs/handoff-stage0.md

_text._ Commission brief (now historical): mission (skeleton, docs, harnesses, types), ground rules (HEAD authoritative, reference immutable, re-verify claims, zero-inference, experiments in throwaway session), authority invariants, repo layout, referential docs requirements, experiment procedures, type-gen script, definition of done (all 8 items complete).

#### docs/reference/ai-sdlc-scan.md

_text._ Immutable research snapshot (2026-07-17, flagged never-edit): tooling evolution June 15–July 17 2026. Claude Code v2.1.181→212, Codex v0.138→0.144.5, Herdr v0.7.0→0.7.4, Pi v0.80.x, models (Sonnet 5 tokenizer inflation 1.28–1.4x, GPT-5.6, Fable 5). Watch list, recommendations.

#### docs/reference/integration-surfaces.md

_text._ Immutable research snapshot (2026-07-17, flagged never-edit): four workstreams (Herdr socket API lifecycle/authority, Pi RPC/extensions, Claude hooks/headless, prior art patterns + Go libraries). Authority-split architecture fit. Staged Stage 0–4 recommendations. Data flow diagram.

## Subsystem: internal-agentcfg

Package agentcfg provides configuration management for Fledge agents: loading/validating named agent profiles, routing models to integrations via fixed prefix table, parsing portable agent definitions from Markdown, and synchronizing generated JSON indexes atomically. It is the authority on what agents can be launched and how, enforcing namespace rules, integration-specific field isolation, and preventing profiles from overriding Fledge's orchestration control.

**Purpose:** Agent configuration, model routing, and definition parsing in internal/agentcfg

### Entry Points

- `internal/agentcfg/agentcfg.go`: Load() reads and merges resolved profiles; Route() maps models to integrations via static table; Validate/ValidateFields() enforce cross-checks; CommandArgv/LaunchArgv assemble launch commands; NewSessionID() generates UUIDs
- `internal/agentcfg/definitions.go`: ParseDefinition() parses .agent.md files; Synchronize() rebuilds indexes atomically from Markdown; LoadDefinitions/FindDefinition() query agents and profiles; MigrateLegacyGenerated() handles legacy index reorganization

### Key Symbols

- `Config` in `internal/agentcfg/agentcfg.go` (type): Launchable agent configuration struct with Integration, Model, Provider, Cwd, PermissionMode, Sandbox, Argv, Env fields; validated to ensure provider/permission_mode/sandbox are only used with appropriate integrations
- `Route` in `internal/agentcfg/agentcfg.go` (func): Maps model id to integration + provider via fixed prefix table; never guesses; returns error with remedy (fledge.profile reference or routable model prefix) for unknown models
- `Definition` in `internal/agentcfg/definitions.go` (type): Parsed portable agent definition with Name, Description, Tools, Model, Profile, Prompt (authoritative body), Source, Managed flag, Launch config, and optional Workspace placement
- `ParseDefinition` in `internal/agentcfg/definitions.go` (func): Parses .agent.md files: extracts YAML frontmatter with name/description/tools/model/fledge (profile/launch/workspace/worktree); validates workspace labels/tabs are trimmed/single-line; returns Definition with authoritative prompt body
- `Synchronize` in `internal/agentcfg/definitions.go` (func): Scans user/ and fledge/ directories for definitions; parses and validates each; derives profiles by routing Model and merging Launch fields; writes user-agents.json and managed-agents.json atomically with version and deterministic layout
- `AgentRecord` in `internal/agentcfg/definitions.go` (type): Deterministic projection of Definition for generated indexes: Source, Description, Tools, Profile, Workspace, PromptHash (SHA-256); lets consumers detect changes without duplicating prompt body
- `Index` in `internal/agentcfg/definitions.go` (type): Generated index shape: Version (1), Agents (map[string]AgentRecord), Profiles (map[string]Config); loaded from user, managed, and catalog sources in order; coalesces identical declarations, rejects conflicts

### Dependencies

- Internal `internal/scaffold`: DirName constant (.fledge), Ensure() for directory creation; used by Load/Synchronize to locate .fledge/agents/ and .fledge/agents/fledge/
- External `github.com/goccy/go-yaml`: YAML unmarshaling for frontmatter; chosen for embedded YAML slice/map support in definitions.go ParseDefinition
- External `crypto/rand`: Random number generation for RFC-4122 v4 UUID in NewSessionID()
- External `encoding/json`: JSON marshaling/unmarshaling for Index and Config serialization in Load and writeIndexAtomic
- External `encoding/hex`: Hex encoding for SHA-256 PromptHash in Synchronize and AgentRecord

### Data Flows

- `Markdown definitions (.agent.md files)` → `Definition struct`: ParseDefinition() extracts YAML frontmatter + prompt body, validates name/description/workspace, returns Definition with authoritative Prompt field
- `Definition + Model` → `Config (profile)`: deriveProfile() routes Model via Route(), then merges explicit Launch fields using mergeConfig(); validates result with validateProfile()
- `Definition` → `AgentRecord`: Synchronize() projects Definition into AgentRecord: copies Source/Description/Tools/Profile/Workspace, computes SHA-256(Prompt) as PromptHash
- `User + Managed + Catalog indexes` → `Profiles map`: Load() reads three Index files in order; first occurrence of a name wins (user shadows); identical declarations coalesce; differing conflicts are errors
- `Config + sessionID + instructions + bootstrap` → `Process argv`: CommandArgv() builds integration-native base; LaunchArgv() adds profile argv + Fledge's instruction option + startup assets + bootstrap; profile argv placed before instructions so Fledge's identity always wins
- `Model string` → `Integration + Provider or Error`: Route() uses fixed prefix table (claude*, gpt*/codex*/o-series, opencode* variants); unknown models return error with remedy suggestion

### Invariants

- Markdown is authoritative for definitions: generated indexes rebuilt wholesale from .agent.md files; never merged back from generated JSON
- Namespace enforcement: user agents/profiles forbidden from fledge-* prefix; managed agents required to use fledge-* prefix; fledge-orchestrator is the single reserved name exempt from naming rules
- Fixed model routing: Route() uses static prefix table; no inference or defaults; unknown model is always an error, never a fallback
- Profile coalescing over shadowing: Load reads user → managed → catalog; first occurrence of a name is kept; later occurrences ignored; differing declarations across sources are validation errors
- Integration-specific field isolation: provider is pi-only; permission_mode is claude-only; sandbox is codex-only; cross-checks prevent invalid combinations
- Interactive argv validation: profile argv cannot contain -- or integration flags that replace session/instruction ownership (--print, --resume, --session-id, --mode, etc.); prevents profiles from seizing control from Fledge orchestration
- Profile argv placement: profile argv placed before Fledge's native instruction option so Fledge's assigned identity and role always win; profile cannot override
- Atomic index writes: writeIndexAtomic uses temp file + fsync + atomic rename to ensure valid index on disk at all times; torn final line tolerated; malformed earlier line is corruption
- Prompt body immutability: prompt hash computed from body; Definition holds full Prompt; AgentRecord holds only PromptHash for change detection without duplicating body
- Path/name matching in definitions: folder + filename (without .agent.md) + frontmatter name must all be equal; enforced in scanDefinitions; rejected in Synchronize

### Tests

- `internal/agentcfg/agentcfg_test.go`: 14 test functions covering Route (model→integration routing with all prefix variants and error cases), Validate (integration cross-checks), CommandArgv (argv assembly for each integration), LaunchArgv (profile argv + instructions + bootstrap ordering), Load (file loading and shadowing), portable name validation (kebab-case enforcement), and NewSessionID (RFC-4122 v4 UUID uniqueness)
- `internal/agentcfg/definitions_test.go`: 10 test functions covering ParseDefinition (YAML + prompt parsing, workspace validation), Synchronize (deterministic index rebuild, path/name matching, namespace enforcement, profile derivation, conflict detection), workspace indexing, legacy migration (legacy→managed directory reorganization, invalid canonical replacement), and reserved profile namespace enforcement

### Files

#### internal/agentcfg/agentcfg.go

_text._ Core configuration types and operations (~320 lines): Config struct with Integration/Model/Provider/PermissionMode/Sandbox/Argv/Env; Load() for reading three-source merged indexes; Route() for model→integration mapping via static prefix table; Validate()/ValidateFields() for cross-checks; CommandArgv/LaunchArgv for process assembly; NewSessionID() for UUID generation; validName/validPortableName for kebab-case enforcement; validateInteractiveArgv for preventing session/instruction control override

#### internal/agentcfg/agentcfg_test.go

_text._ Configuration tests (~380 lines): TestRoute (all prefix variants + error cases), TestRouteErrorPointsToWorkingRemedy (error message guidance), TestValidate (integration combinations and cross-checks), TestCommandArgv (argv assembly per integration), TestLaunchArgv (instruction ordering), TestValidateRejectsArgvDelimiter (-- not allowed), TestValidateRejectsProfileFlags (bans session/instruction flags), TestLoad* (file loading, shadowing, merging), TestNewSessionID (uniqueness), TestPortableNames (kebab-case enforcement)

#### internal/agentcfg/definitions.go

_text._ Markdown definition parsing and synchronization (~555 lines): ParseDefinition() extracts YAML frontmatter + prompt, validates workspace; Definition/AgentRecord/Index types; Synchronize() scans user/ and fledge/ directories, parses definitions, derives profiles by routing Model and merging Launch, writes indexes atomically with deterministic layout; LoadDefinitions() and FindDefinition() for querying; MigrateLegacyGenerated() for one-time legacy index reorganization; helper functions for profile derivation, atomic JSON writing, and validation

#### internal/agentcfg/definitions_test.go

_text._ Parsing and synchronization tests (~366 lines): TestParseDefinition (YAML extraction, workspace/launch/tools parsing), TestParseDefinitionValidatesWorkspace (label/tab trimmed/single-line), TestParseDefinitionRejectsUnsupportedWorktree (worktree future-proofs), TestSynchronize* (deterministic rebuild, path/name matching, namespace rules, profile derivation, conflict detection), TestMigrateLegacy* (index reorganization, invalid canonical replacement), TestSync*Profiles (reserved namespace enforcement)

## Subsystem: internal-misc-a

Four tightly coupled packages supporting daemon orchestration and context analysis. Catalog discovers models from installed integrations (claude, codex, pi) with deterministic collision resolution. Client provides unified RPC via Unix socket (primary) with transparent file-bridge fallback for sandboxed agents. Filebridge implements workspace-local atomic request/response handoff with client PID tracking for signal-0 liveness verification. Contextdoc validates analyzer requests/replies with strict JSON enforcement (no unknown fields, no duplicates), composes instructions from templates, and renders artifacts to project.md. Flock manages session identity derivation (workspace-prefixed) and flock name validation/selection for session isolation.

**Purpose:** Model catalog, socket client, context document rendering/validation, file bridge, and flock lifecycle packages

### Entry Points

- `internal/catalog/catalog.go`: Discover() orchestrates exec-and-parse model discovery from three integrations with collision resolution and atomic catalog write
- `internal/client/client.go`: Do() provides unified request/response RPC with socket priority and transparent file-bridge fallback
- `internal/contextdoc/validate.go`: ValidateAnalyzerRequest() and ValidateAnalyzerReply() enforce strict JSON validation for request/reply correlation
- `internal/contextdoc/render.go`: RenderProject() loads, validates, and atomically publishes analyzer artifacts to project context document
- `internal/filebridge/filebridge.go`: Submit/Await/Take/Respond implement atomic file-based RPC transport with client PID tracking and orphan sweeping
- `internal/flock/flock.go`: FromEnv() selects flock from environment; SessionName() derives workspace-prefixed herdr session identity

### Key Symbols

- `Discover` in `internal/catalog/catalog.go` (function): Executes each integration binary in fixed order, parses outputs into entries, resolves collisions by source ordering, returns configs and notes
- `Write` in `internal/catalog/catalog.go` (function): Atomically writes catalog to .fledge/agents/fledge/catalog.json via temp+rename with Chmod(0o644), producing deterministic bytes
- `Do` in `internal/client/client.go` (function): Sends one request to daemon via socket (primary) or file bridge (fallback), blocks for response, returns error if not running
- `Running` in `internal/client/client.go` (function): Probes daemon liveness via socket dial or file bridge Available() check
- `ValidateAnalyzerRequest` in `internal/contextdoc/validate.go` (function): Validates request schema, group_id (kebab-case), purpose (nonempty), files (1-50, ≤256KB total, oversized singleton allowed), total_size match
- `ValidateAnalyzerReply` in `internal/contextdoc/validate.go` (function): Validates reply status ('ok' or 'error'), correlates group_id with request, checks all arrays present, path references assigned, content_kind text/non-text
- `ParseRequestTemplate` in `internal/contextdoc/compose.go` (function): Extracts instruction_before and instruction_after from XML-delimited template sections
- `ComposeAnalyzerRequest` in `internal/contextdoc/compose.go` (function): Substitutes {group_id}, {purpose}, {worksheet_path} placeholders into template sections
- `RenderProject` in `internal/contextdoc/render.go` (function): Loads and validates all analyzer artifacts, renders markdown from scan/requests/replies/synthesis, atomically publishes to project.md
- `Submit` in `internal/filebridge/filebridge.go` (function): Generates random exchange id, publishes {id, request, pid} atomically to inbox directory
- `Take` in `internal/filebridge/filebridge.go` (function): Scans inbox, validates each (id safety via validID, pid > 0), atomically moves to accepted marker with pid stamped
- `Respond` in `internal/filebridge/filebridge.go` (function): Publishes response atomically, sweeps orphan if Awaiting(id) returns false (client abandoned)
- `Awaiting` in `internal/filebridge/filebridge.go` (function): Checks if accepted marker exists and client pid is alive (signal-0 probe)
- `SessionName` in `internal/flock/flock.go` (function): Derives 'fledge-' + workspace.Slug(root) + '-' + name to prevent cross-workspace session collisions
- `Validate` in `internal/flock/flock.go` (function): Enforces flock name rules: lowercase alphanumeric, 1-32 chars, rejects traversals and special characters
- `FromEnv` in `internal/flock/flock.go` (function): Reads FLEDGE_FLOCK, validates name, returns selected flock or hard error

### Dependencies

- Internal `internal/agentcfg`: Config types, validation rules, Index format for catalog merge
- Internal `internal/scaffold`: Directory structure constants (DirName, CatalogName, AgentsDir)
- Internal `internal/daemon`: SocketPath() helper for Unix socket location
- Internal `internal/protocol`: Request and Response wire types for RPC exchange
- Internal `internal/workspace`: Slug() for workspace identity in session naming; EvalSymlinks for canonical path
- External `encoding/json`: Marshal/Unmarshal, DisallowUnknownFields decoder, duplicate-key rejection via manual Token walk
- External `os/exec`: LookPath and CommandContext for integration binary discovery with timeouts
- External `net`: Unix socket transport (Dial, Listen, Conn)
- External `syscall`: Kill(pid, 0) for signal-0 liveness probes in filebridge; chmod/permission enforcement
- External `crypto/rand`: Random bytes for exchange id generation (16 bytes, hex encoded)
- External `time`: Deadlines and timeouts in discovery and file RPC polls
- External `regexp`: Kebab-case pattern validation for group_id
- External `io/fs`: ValidPath() for path safety checks in request/reply
- External `os`: File operations, atomic temp+rename pattern, directory traversal (WalkDir)

### Data Flows

- `catalog.Discover()` → `os/exec`: Executes claude --version, codex debug models, pi --list-models in fixed order
- `os/exec output` → `parseClaudeVersion/parseCodexModels/parsePiModels`: Parses stdout into entry structs
- `entries` → `catalog.Discover collision logic`: Applies sourceSuffix and slugName to resolve collisions deterministically
- `config map` → `catalog.Write`: MarshalIndent config map and atomically publishes to catalog.json
- `user request` → `client.Do`: Sends protocol.Request to daemon via socket (primary) or file bridge
- `client.Do` → `net unix socket or filebridge`: Encodes request as JSON, sends to daemon, blocks for response
- `filebridge.Submit` → `.rpc/inbox`: Writes {id, request, pid} atomically
- `daemon` → `filebridge.Take`: Scans inbox, claims requests, atomically moves to accepted marker with pid
- `daemon work` → `filebridge.Respond`: Writes response atomically, checks Awaiting for client liveness, sweeps orphans
- `analyzer request template` → `ComposeAnalyzerRequest`: Substitutes {group_id}, {purpose}, {worksheet_path} into instruction sections
- `composed request` → `analyzer`: Delivers request with both instruction fields required nonempty
- `analyzer reply` → `ValidateAnalyzerReply`: Correlates with request, validates all arrays present, path references assigned
- `validated artifacts` → `RenderProject`: Loads scan, requests, replies, synthesis; renders markdown; atomically publishes
- `FLEDGE_FLOCK env` → `flock.FromEnv`: Reads and validates flock name; derives session name from workspace slug and flock name

### Invariants

- Catalog discovery writes deterministic bytes: exec order fixed, collision survivor by first source, MarshalIndent output stable
- Client RPC prioritizes socket, always has file-bridge fallback; transparent to caller
- Filebridge atomicity: Submit→Take→Respond→Cleanup form handoff; Respond sweeps orphans if Awaiting(id) false
- Validation strictness: JSON decoders reject unknown fields and duplicate keys before unmarshal
- Flock sessions isolated: SessionName includes workspace.Slug(root); two workspaces with same flock name get different herdr sessions
- Path safety: fs.ValidPath checks; filebridge ids are 16 random bytes; request/reply paths must be in assigned file set
- Liveness tracking: Client PID stored in filebridge accepted marker; signal-0 probe distinguishes live from dead clients
- Instruction requirement: Composed analyzer requests must have both instruction fields nonempty before dispatch
- All reply arrays present: ValidateAnalyzerReply requires EntryPoints, KeySymbols, Dependencies, DataFlows, Invariants, Tests, Files to never be nil

### Tests

- `internal/catalog/catalog_test.go`: Comprehensive discovery: all sources, missing/failing binaries, timeouts, collision detection, user-profile non-collision, stable writes
- `internal/client/client_test.go`: Socket RPC lifecycle, file-bridge fallback, error propagation, Running() detection
- `internal/contextdoc/compose_test.go`: Template parsing (duplicate/missing tags, empty sections), placeholder substitution, worksheet composition
- `internal/contextdoc/contextdoc_test.go`: Request validation (schema, group_id, purpose, files, grouping bounds); reply validation (status, group_id match, path refs)
- `internal/filebridge/filebridge_test.go`: Lifecycle, orphan sweep, path-traversal id rejection, concurrent submit/take with race detector, daemon-stop abort, pre-pid rejection
- `internal/flock/flock_test.go`: Name validation (empty, too long, uppercase, special chars, traversals), list sorting, minting (gap filling), session derivation (workspace isolation), env selection

### Files

#### internal/catalog/catalog.go

_text._ Model discovery orchestrator. Executes three integration binaries in fixed order, parses outputs into entries, resolves collisions by source ordering and suffix rules. Atomically writes merged catalog to .fledge/agents/fledge/catalog.json via temp+rename with deterministic MarshalIndent output.

#### internal/catalog/catalog_test.go

_text._ Comprehensive discovery tests using fake binaries on PATH. Validates all sources, missing/failing binaries, timeouts, collision detection, suffix application, user-profile non-collision, and stable writes across re-init.

#### internal/client/client.go

_text._ Unified RPC dispatcher to daemon. Do() attempts socket dial (primary); on failure, falls back to filebridge. Running() probes both transports. Transparent to caller: same function signature regardless of transport.

#### internal/client/client_test.go

_text._ Tests socket RPC lifecycle (JSON exchange), error propagation, malformed/closed connections, Running() detection via socket and file bridge, file fallback for sandboxed agents.

#### internal/contextdoc/compose.go

_text._ Template composition for analyzer requests. ParseRequestTemplate extracts <instructions_before> and <instructions_after> sections. ComposeAnalyzerRequest substitutes {group_id}, {purpose}, {worksheet_path} placeholders. ComposeWorksheet stamps file checklist.

#### internal/contextdoc/compose_test.go

_text._ Tests template parsing (duplicate/missing tags, empty sections, unknown placeholders), placeholder substitution with required worksheet_path, worksheet file list composition with byte counts.

#### internal/contextdoc/contextdoc_test.go

_text._ Tests request validation (schema version, group_id kebab-case, purpose nonempty, file limits 1-50 and ≤256KB, total_size match) and reply validation (status ok/error, group_id match, path references assigned, all arrays present).

#### internal/contextdoc/render.go

_text._ Artifact loading and rendering. Validates all files before atomic publication via temp+rename. Renders markdown from scan metadata, analyzer requests/replies, synthesis, and provenance. Returns RenderResult with SHA256 and warnings for best-effort cleanup failures.

#### internal/contextdoc/types.go

_text._ Wire contracts for context analysis. Defines Scan (file manifest), AnalyzerRequest (group_id, purpose, files, instructions), AnalyzerReply (ok: summary/entry_points/symbols/deps/flows/invariants/tests/files, or error: errors array), Synthesis (routing/cross-group flows), and Provenance (traceability).

#### internal/contextdoc/validate.go

_text._ Strict JSON validation. decodeExact uses DisallowUnknownFields and manual duplicate-key rejection via Token walk. Validates schema version, group_id (kebab-case), purpose (nonempty), file limits (1-50, ≤256KB, oversized singleton). Reply validation checks status, group_id match, all arrays present, path references assigned by request, content_kind text/non-text.

#### internal/filebridge/filebridge.go

_text._ File-based RPC transport for sandboxed clients. Submit publishes {id, request, pid} to .rpc/inbox atomically. Take claims inbox, validates id (16 random bytes hex) and pid > 0. Await polls accepted marker then response file. Respond writes response atomically and sweeps if !Awaiting (client abandoned). Cleanup removes all exchange files.

#### internal/filebridge/filebridge_test.go

_text._ Tests request/response lifecycle, orphan sweep (client abandonment), path-traversal id validation, concurrent submit/take with race detector, daemon-stop detection, pre-pid binary upgrade window.

#### internal/flock/flock.go

_text._ Flock lifecycle and session identity. Dir returns .fledge/flocks/<name> state path. SessionName derives 'fledge-' + workspace.Slug(root) + '-' + name for session isolation. Validate enforces name rules (lowercase alphanumeric, ≤32 chars). List returns sorted flocks. Mint finds lowest free flockN. FromEnv reads FLEDGE_FLOCK, validates, returns.

#### internal/flock/flock_test.go

_text._ Tests name validation (empty, too long, uppercase, special chars, dots, traversals, reserved), list sorting, minting (gap filling), session derivation (workspace isolation, stable across path spellings), env selection (hard error on unset, rejects bad names).

## Subsystem: internal-misc-b

Infrastructure for workspace management, session coordination, file filtering, and scaffolding. Layered system: foundation (version, species, workspace, protocol), file operations (ignore, scan), Herdr integration (herdr CLI wrapper, herdrwire socket API), setup (scaffold). All systems converge on workspace identity via FindRoot→EvalSymlinks→Hash to ensure daemon and CLI agree on socket namespace and session identity.

**Purpose:** Herdr CLI/wire integration, ignore rules, wire protocol, scaffold templates, workspace scanning, species pool, versioning, and workspace root discovery

### Entry Points

- `internal/workspace/workspace.go`: FindRoot(dir): Walk up to .fledge, return canonical absolute path via EvalSymlinks
- `internal/herdr/herdr.go`: Ensure(name, env, dir): Attach to or start named Herdr session with environment and working directory
- `internal/herdrwire/herdrwire.go`: AgentStart(socket, name, cwd, argv, env, split): Launch pane-hosted agent via Herdr socket API
- `internal/ignore/ignore.go`: ParseFile(path, root): Load .fledgeignore patterns with #include directive support
- `internal/scan/scan.go`: Files(root, matcher): Walk directory tree, return non-ignored files with sizes in lexical order
- `internal/scaffold/scaffold.go`: Ensure(root): Initialize or refresh .fledge directory structure and templates
- `internal/version/version.go`: Get(): Return semantic version with optional -dev suffix based on build tag
- `internal/species/species.go`: Pick(taken, requested): Select or validate agent species slug from 18-element penguin pool

### Key Symbols

- `Session` in `internal/herdr/herdr.go` (struct): Represents a Herdr session: Name, Running, Default, SocketPath
- `Ensure` in `internal/herdr/herdr.go` (function): Attach to existing or start new Herdr session with environment and working directory
- `Call` in `internal/herdrwire/herdrwire.go` (function): Single request/response over unix socket, newline-delimited JSON with fixed timeouts
- `Matcher` in `internal/ignore/ignore.go` (struct): Holds compiled patterns from .fledgeignore, supports Match(path, isDir) with negation
- `ParseFile` in `internal/ignore/ignore.go` (function): Load .fledgeignore patterns, resolving #include directives relative to root
- `Ensure` in `internal/scaffold/scaffold.go` (function): Initialize or refresh .fledge directory: create subdirs, seed templates, replace managed definitions atomically
- `Request` in `internal/protocol/protocol.go` (struct): Daemon-bound command: Op plus context-specific fields (agent, message, spawn, etc.)
- `FindRoot` in `internal/workspace/workspace.go` (function): Walk up to .fledge directory, return canonical absolute path via EvalSymlinks
- `Hash` in `internal/workspace/workspace.go` (function): Deterministic workspace identity: SHA256(abs(root))[:12] for socket namespace key
- `Slugs` in `internal/species/species.go` (var): 18 penguin species in fixed order: emperor to southernrockhopper

### Dependencies

- Internal `internal/ignore`: ignore.Matcher used by scaffold and scan for .fledgeignore pattern matching
- Internal `internal/protocol`: Protocol constants (Op*, Env*, JournalName, LogName) used throughout for daemon communication
- External `stdlib`: encoding/json, os, filepath, net, strings, regexp, time, sync/atomic, crypto/sha256, syscall, io, bufio, errors

### Data Flows

- `command invocation` → `workspace.FindRoot`: Command runs in dir → FindRoot walks up → stat .fledge → EvalSymlinks → canonical root
- `workspace.FindRoot` → `workspace.Hash`: Canonical root → Hash(root) → 12-char hex for socket namespace key
- `herdr.Ensure` → `herdr.start`: Ensure checks for existing session, if missing: start detached herdr CLI → poll Find+Up until ready
- `herdr.start` → `herdrwire.Call`: Once session socket is up, Herdr API calls use herdrwire.Call for pane/workspace ops
- `scaffold.Ensure` → `ignore.ParseFile`: Scaffold loads .fledgeignore to check gitignore coverage during EnsureGitignore
- `scan.Files` → `ignore.Matcher.Match`: Directory walk: match each rel path → SkipDir if ignored dir → append non-ignored files
- `ignore.ParseFile` → `ignore.compile`: Pattern parsing: includeTarget directives resolved → parseLine → compile globs to regexps
- `scaffold.Ensure` → `scaffold.replaceManagedDefinitions`: Managed definitions staged in temp files → sync → atomic Rename to avoid partial updates

### Invariants

- Workspace identity is stable: FindRoot(p1)==FindRoot(p2) iff canonical absolute paths are equal after EvalSymlinks
- Socket namespace is deterministic: Hash(root)=hex(SHA256(abs(root)))[:12]; same workspace always gets same hash
- Ignore semantics mirror gitignore: directories are pruned (fs.SkipDir), negation cannot re-include under ignored parent, last pattern wins
- Herdr socket protocol: exactly one request/response per connection, newline-delimited JSON, server closes after response
- Managed definitions are immutable during session: atomic staging replaces via temp file → sync → close → rename
- User edits are preserved: .fledgeignore and user-agents.json written only if absent (writeIfAbsent)
- Species slug pool is exhaustible: 18 fixed slugs, auto-assignment walks order, error when all taken
- Pane liveness is definitive: Herdr removes socket when session ends, dial failure = down (no fallback polling)
- Versions follow strict semver: MAJOR.MINOR.PATCH with no leading zeros, except bare 0; dev builds append -dev
- File paths in .fledgeignore are relative to workspace root: slash-separated, matched via filepath.ToSlash(filepath.Rel)

### Tests

- `internal/herdr/herdr_test.go`: Session list, find, up probe, ensure reuse/start, recreate stop+delete+start, remove idempotent
- `internal/herdrwire/herdrwire_test.go`: Call envelope (unique id, mandatory params), one-shot per connection, agent/workspace/tab/pane ops, error handling, timeout enforcement
- `internal/ignore/ignore_test.go`: 60+ pattern match cases, include directives with cycle detection, edge cases (escapes, bracket classes, POSIX, negation)
- `internal/scan/scan_test.go`: Directory walk, file size reporting, prune ignored dirs, negation, cannot re-include under pruned parent
- `internal/species/species_test.go`: Pick auto/requested/exhausted/unknown, verifies 18 unique slugs
- `internal/version/version_test.go`: Strict semver format validation (MAJOR.MINOR.PATCH with no leading zeros)
- `internal/workspace/workspace_test.go`: FindRoot walks up, prefers nearest, canonicalizes symlinks, hash stability, slug sanitization
- `internal/scaffold/scaffold_test.go`: Tree creation, template seeding, refresh managed definitions, remove obsolete, preserve edits, gitignore coverage

### Files

#### internal/herdr/herdr.go

_text._ Session lifecycle CLI wrapper. Exported: SessionEnv, Session struct, List, Find, Up, Ensure, Recreate, Remove, Stop, Delete. Private: start (detach via setsid), detach. Verified on herdr 0.7.4/protocol 16. Protocol 16 emits no session-lifecycle events; probing socket is the only mechanism.

#### internal/herdr/herdr_test.go

_text._ Tests session operations via stubHerdr fake CLI. Tests list/find, up probe, ensure reuse/start, recreate stop+delete+start, remove idempotent, error wrapping.

#### internal/herdrwire/herdrwire.go

_text._ Unix socket client for Herdr API. Call function handles one request/response per connection with timeouts. Exported: Call, AgentStart, Workspace*, Tab*, Pane* ops, ProcessInfo, SendInput, AgentAlive, AgentStatus, ReleaseAgent, ReportMetadata, WindowTitleSet.

#### internal/herdrwire/herdrwire_test.go

_text._ Tests via fakeHerdr mock server. Tests Call envelope, one-shot per connection, agent/workspace/tab/pane operations, error handling, timeout enforcement, placement options.

#### internal/ignore/ignore.go

_text._ gitignore-style pattern matcher with #include support. Matcher holds compiled patterns. ParseFile loads .fledgeignore (missing ok), Parse from reader. compile translates globs to regexp with *, ?, [], **, escapes, bracket class handling. Match respects dir-only, negation, last pattern wins.

#### internal/ignore/ignore_test.go

_text._ Tests Match with 60+ cases (patterns, anchoring, dir-only, negation, **, escapes, bracket classes). ParseFile missing→empty, IncludeDirective with cycle detection, errors.

#### internal/protocol/protocol.go

_text._ Wire format: Op/Env constants, Request/Response structs, Agent struct (spawned+self-registered fields), Message struct, JournalName/LogName constants.

#### internal/scaffold/agents_test.go

_text._ External test verifying cross-package constant sync: scaffold.AgentsName==agentcfg.FileName, GitignoreEntries includes managed index, seeded stub parses under agentcfg.

#### internal/scaffold/scaffold.go

_text._ Initialize/refresh .fledge: Ensure creates subdirs, seeds templates (request, worksheet), stubs agents index, replaces managed defs atomically via staging, removes obsolete dirs, migrates legacy profiles. EnsureGitignore appends GitignoreEntries if not covered.

#### internal/scaffold/scaffold_test.go

_text._ Tests Ensure: tree creation, template seeds, refresh managed defs, remove obsolete, preserve edits, idempotent, gitignore coverage, legacy profile migration.

#### internal/scan/scan.go

_text._ Directory walk. File struct {Path, Size}. Files(root, matcher) returns non-ignored files in lexical order, prunes ignored directories. No re-include under pruned parent.

#### internal/scan/scan_test.go

_text._ Tests Files: walk behavior, size reporting, prune ignored dirs, negation, cannot re-include under pruned parent.

#### internal/species/species.go

_text._ 18 penguin species slugs in fixed order. Pick(taken, requested) auto-picks first free or validates requested. Exported: Slugs, Pick.

#### internal/species/species_test.go

_text._ Tests Pick auto/requested/exhausted/unknown. Verifies 18 unique slugs.

#### internal/version/VERSION

_text._ Embedded version string: 0.0.1

#### internal/version/suffix.go

_text._ Build-tag !dev: const suffix = ""

#### internal/version/suffix_dev.go

_text._ Build-tag dev: const suffix = "-dev"

#### internal/version/version.go

_text._ Get() returns embedded VERSION trimmed + conditional dev suffix. Exported: Get.

#### internal/version/version_test.go

_text._ Tests Get returns strict MAJOR.MINOR.PATCH (no leading zeros except bare 0).

#### internal/workspace/workspace.go

_text._ FindRoot(dir) walks up to .fledge, returns canonical absolute via EvalSymlinks. Hash(root)=hex(SHA256(abs))[:12] for socket namespace. Slug(root)=sanitized basename + 6-char hash suffix. Exported: FindRoot, Hash, Slug, ErrNotFound.

#### internal/workspace/workspace_test.go

_text._ Tests FindRoot: walk up, canonicalize symlinks, skip .fledge file. Hash: stable, distinct, 12-char. Slug: sanitization, hash suffix.

## Subsystem: project-meta

Fledge is a zero-inference Go orchestrator for launching and coordinating multi-agent coding sessions. It maintains three invariants: (1) the append-only journal is authoritative state, not Herdr; (2) the orchestrator itself performs no LLM inference; and (3) all visible coding work happens in Herdr panes the operator can watch and interrupt. The system spans a CLI, per-flock daemon, portable agent definitions, and integration contracts (Claude, Codex, Pi). CI/CD pipelines validate formatting, test coverage, semantic versioning, and produce static Linux binaries. The project is architected for minimal dependencies (stdlib + goccy/go-yaml) and Unix-only operation.

**Purpose:** Top-level project metadata: README, CLAUDE.md, licensing, CI workflows, and build/install scripts

### Entry Points

- `README.md`: fledge init — scaffolds .fledge/ tree, generates agent indexes, discovers/catalogs installed integrations
- `README.md`: fledge start — brings up a new flock with Herdr UI, launches orchestrator, optionally spawns watcher pane
- `README.md`: fledge agent spawn — launches a named agent definition into the roster with resolved profile/model

### Key Symbols

- `Flock` in `README.md` (concept): Isolated orchestration session: own daemon, roster, journal, socket, and Herdr session state
- `FLEDGE_FLOCK` in `README.md` (env-variable): Environment variable selecting ambient flock for scoped commands; exported by fledge start into session panes
- `Agent` in `README.md` (concept): Named process with species-based naming; one exception: fledge-orchestrator (no species suffix)
- `Species` in `README.md` (concept): 18 penguin slugs allocated per agent type; reused across instances
- `Integration` in `README.md` (concept): Launch harness (Claude, Codex, Pi); all agents run in visible Herdr panes with instruction injection
- `Orchestrator` in `README.md` (concept): The managed fledge-orchestrator agent; always present after flock start; receives role via native instruction
- `Forager` in `README.md` (managed-agent): fledge-forager: coordinates multi-agent file analysis; spawns analyzers, validates requests/replies, publishes project.md
- `Analyzer` in `README.md` (managed-agent): fledge-analyzer: reads assigned file subset, fills worksheet, returns structured findings

### Dependencies

- Internal `cmd/fledge`: CLI: hand-rolled dispatch, flag parsing
- Internal `internal/daemon`: Per-flock server; spawning, journal, session watch, state machine
- Internal `internal/protocol`: Request/response types for daemon socket
- Internal `internal/client`: CLI dials the daemon socket
- Internal `internal/flock`: Flock naming, layout, socket paths, FLEDGE_FLOCK resolution
- Internal `internal/herdr`: Shells out to herdr CLI for session lifecycle
- Internal `internal/herdrwire`: Speaks herdr socket API directly for pane operations
- Internal `internal/agentcfg`: .agent.md parsing, index synchronization, profiles, model routing
- Internal `internal/catalog`: Model discovery from installed integrations
- Internal `internal/species`: 18-slug penguin pool management
- Internal `internal/scaffold`: Creates .fledge/ tree structure
- Internal `internal/ignore`: .fledgeignore matching rules
- Internal `internal/scan`: Workspace file scanning and filtering
- External `github.com/goccy/go-yaml`: v1.19.2 — YAML frontmatter parsing for .agent.md files
- External `herdr`: CLI tool, protocol 16 (verified against 0.7.4); pane system, Herdr UI, session management
- External `claude`: CLI for Claude Code integration; optional if not spawning Claude agents
- External `codex`: CLI for Codex integration; optional if not spawning Codex agents
- External `pi`: CLI for Pi integration (multi-provider: openai-codex, opencode, opencode-go); optional if not spawning Pi agents

### Data Flows

- `fledge init discovery` → `catalog.json`: Probe installed integrations, regenerate model catalog
- `fledge start` → `daemon + journal`: Mint flock, start Herdr session, spawn daemon, launch orchestrator
- `Agent spawn` → `journal.jsonl`: Parse definition, resolve profile/model, journal lifecycle events
- `Forager orchestration` → `Analyzer instances`: Partition files, compose requests with instructions, dispatch, validate replies
- `Analyzer` → `project.md + provenance.json`: Synthesize validated findings into published outputs

### Invariants

- Append-only journal is authoritative state; every operation journaled before client acknowledgment
- Zero inference in orchestrator; fledge issues commands, consumes events, advances state machine, writes journal
- All visible work in panes; operators watch and control agents through Herdr UI
- Roster derived from journal replay; daemon rebuilds state by replaying journal on startup
- Unix-only operation; sockets, setsid, signal-0 probes
- Version is embedded source of truth (internal/version/VERSION)
- Managed namespace reserved; fledge-* names owned by system
- Species pool per type; 18 penguin slugs reused across agent types
- Request/reply validation strict; correlation checks before delivery
- Assigned files only; analyzers read only listed files

### Tests

- `.github/workflows/pull-request.yml`: PR validation: lint (gofmt, vet), test (coverage), build (amd64/arm64), version check
- `.github/workflows/release.yml`: Post-merge release: gate, lint, test, build+package, release creation, tag management

### Files

#### .github/workflows/pull-request.yml

_text._ PR validation workflow: gofmt/vet, test with coverage report, build static binaries (linux amd64/arm64), semantic version check

#### .github/workflows/release-badge.yml

_text._ Release badge workflow runs after release succeeds; computes latest tag, publishes JSON badge to orphan badges branch

#### .github/workflows/release.yml

_text._ Post-merge release automation: gate check, lint/test, build tar.gz+SHA256SUMS, create/resume release, tag management

#### .gitignore

_text._ Excludes build output, test binaries, coverage, vendor, .fledge runtime state (locks, flocks, context runs, managed agents, indexes)

#### AGENTS.md

_text._ Repository guidelines: single-user project, breaking changes acceptable; structure, build/test commands, style (gofmt, hand-rolled CLI), testing, commits, architecture

#### CLAUDE.md

_text._ Developer guide: structure, CLI conventions (hand-rolled flags with unique short forms), testing, commits, architecture invariants, versioning, verification

#### LICENSE

_text._ GNU Affero General Public License v3.0 full text

#### README.md

_text._ Authoritative command reference and architecture: CLI commands/flags, configuration, agent definitions, Forager/Analyzer orchestration, .fledgeignore, requirements, development

#### go.mod

_text._ Go 1.26 module with single dependency: github.com/goccy/go-yaml v1.19.2 (YAML frontmatter for agents)

#### go.sum

_text._ Checksum for goccy/go-yaml v1.19.2

#### scripts/build.sh

_text._ Build script: go build -o bin/fledge ./cmd/fledge

#### scripts/check-release-version.sh

_text._ Semantic version validator: strict MAJOR.MINOR.PATCH, must exceed existing tags; 'new' (PR) and 'release' (post-merge) modes

#### scripts/install.sh

_text._ Install script: build with -tags dev, install to GOBIN/GOPATH/bin (BINDIR override), verify PATH and shadowing

#### scripts/reinstall.sh

_text._ Convenience wrapper: build.sh + install.sh; no arguments; BINDIR override
