# Project Context

_Generated at 2026-07-24 01:45:47 UTC._

## Provenance

- Forager: `fledge-forager-emperor` (profile `fledge-context-sonnet-auto`, model `claude-sonnet-5`)
- Analyzer count: 10
- Group count: 10
- File count: 94 (1203040 bytes)
- Distinct profiles: 2 (`fledge-context-haiku-auto`, `fledge-context-sonnet-auto`)
- Distinct models: 2 (`claude-haiku-4-5`, `claude-sonnet-5`)
- Run created at: 2026-07-24 01:45:25 UTC
- Analyzer `agentcfg-catalog-scaffold`: `fledge-analyzer-gentoo` (profile `fledge-context-haiku-auto`, model `claude-haiku-4-5`)
- Analyzer `cmd-fledge-1`: `fledge-analyzer-adelie` (profile `fledge-context-haiku-auto`, model `claude-haiku-4-5`)
- Analyzer `cmd-fledge-2`: `fledge-analyzer-chinstrap` (profile `fledge-context-haiku-auto`, model `claude-haiku-4-5`)
- Analyzer `contextdoc`: `fledge-analyzer-little` (profile `fledge-context-haiku-auto`, model `claude-haiku-4-5`)
- Analyzer `daemon-core`: `fledge-analyzer-african` (profile `fledge-context-haiku-auto`, model `claude-haiku-4-5`)
- Analyzer `daemon-spawn-readiness`: `fledge-analyzer-galapagos` (profile `fledge-context-haiku-auto`, model `claude-haiku-4-5`)
- Analyzer `docs`: `fledge-analyzer-king` (profile `fledge-context-haiku-auto`, model `claude-haiku-4-5`)
- Analyzer `herdr-integration`: `fledge-analyzer-humboldt` (profile `fledge-context-haiku-auto`, model `claude-haiku-4-5`)
- Analyzer `root-meta`: `fledge-analyzer-emperor` (profile `fledge-context-haiku-auto`, model `claude-haiku-4-5`)
- Analyzer `support-libs`: `fledge-analyzer-magellanic` (profile `fledge-context-haiku-auto`, model `claude-haiku-4-5`)

## Project Overview

Fledge is a zero-inference Go orchestrator for a multi-agent coding stack (Herdr pane-bus, Pi, Claude Code, Codex). One binary runs as both the CLI and a per-flock daemon (re-exec'd under setsid), meeting only over a unix socket speaking newline-delimited JSON (internal/protocol). A flock is one isolated orchestration session with its own daemon, agent roster, append-only journal, socket, and Herdr session; the journal is the sole state authority, written before any client is ack'd, and is replayed on restart. Agents are pane-hosted across three integrations (claude/codex/pi) with kebab-case type-species names drawn from a fixed penguin pool, launched via portable Markdown definitions (internal/agentcfg) synchronized into versioned JSON indexes, with models routed by a fixed prefix table and discovered into a per-machine catalog (internal/catalog). Herdr integration is three-layered: CLI shell-out for session lifecycle (internal/herdr), direct socket wire calls for pane/workspace control (internal/herdrwire), and a sandboxed file-bridge fallback (internal/filebridge) for clients that cannot open the daemon socket. A managed context subsystem (internal/contextdoc, this forager/analyzer workflow) lets a forager agent partition a workspace scan into subsystem groups, dispatch them to analyzer agents, validate their structured replies, and atomically render a canonical Markdown project document. The orchestrator itself performs no LLM inference: it issues socket commands, consumes observed events, advances deterministic state, and journals — all inference happens in visible, operator-interactable panes. docs/ is a completed, frozen 'Stage 0' research/experiment record that informs but does not govern current design.

## Routing

- `.github` → `root-meta`: CI/CD workflow definitions (PR checks, release automation); gates on internal/version/VERSION.
- `scripts` → `root-meta`: Build/install/reinstall/release-version shell scripts; no build system beyond these.
- `go.mod` → `root-meta`: Module declaration: github.com/Harrison-Blair/fledge, Go 1.26, single dependency goccy/go-yaml.
- `go.sum` → `root-meta`: Pinned dependency hash for goccy/go-yaml.
- `README.md` → `root-meta`: Authoritative user-facing command reference, quickstart, concepts, and .fledge/ layout documentation.
- `CLAUDE.md` → `root-meta`: Project instructions for Claude Code: architecture invariants, conventions, commands.
- `AGENTS.md` → `root-meta`: Repository guidelines for portable .agent.md definitions and namespacing.
- `LICENSE` → `root-meta`: AGPLv3 license text.
- `.gitignore` → `root-meta`: Excludes build artifacts and gitignored generated .fledge state (locks, flocks, catalog.json, agents.json).
- `docs` → `docs`: Frozen Stage 0 design/decision record (ADRs, experiment findings, integration contracts, immutable reference snapshots). Historical input, not current instruction; do not resurrect from docs/handoff-stage0.md or treat docs/reference/* as current.
- `cmd/fledge/agent_definitions_test.go` → `cmd-fledge-1`: CLI command dispatcher, context inspection (scan/graph), flock lifecycle, agent lifecycle/message routing.
- `cmd/fledge/behavior_test.go` → `cmd-fledge-1`: See cmd-fledge-1.
- `cmd/fledge/clear_test.go` → `cmd-fledge-1`: See cmd-fledge-1.
- `cmd/fledge/context.go` → `cmd-fledge-1`: `context scan/graph` command implementation.
- `cmd/fledge/context_pipeline_test.go` → `cmd-fledge-1`: See cmd-fledge-1.
- `cmd/fledge/graph_test.go` → `cmd-fledge-1`: See cmd-fledge-1.
- `cmd/fledge/help.go` → `cmd-fledge-1`: Embedded help-page text and dispatch for --help/usage errors.
- `cmd/fledge/main.go` → `cmd-fledge-1`: Top-level command dispatcher and hand-rolled flag parser (takeFlag/takeBoolFlag/rejectFlags).
- `cmd/fledge/main_test.go` → `cmd-fledge-2`: CLI entrypoint tests and remaining commands: parser/restart/scan/stop/watch/workspace.
- `cmd/fledge/parser_test.go` → `cmd-fledge-2`: See cmd-fledge-2.
- `cmd/fledge/restart_test.go` → `cmd-fledge-2`: In-place restart/install-handoff command tests.
- `cmd/fledge/scan_test.go` → `cmd-fledge-2`: See cmd-fledge-2.
- `cmd/fledge/stop_test.go` → `cmd-fledge-2`: See cmd-fledge-2.
- `cmd/fledge/watch.go` → `cmd-fledge-2`: `fledge watch` command, execs as a non-agent root-shell tab process.
- `cmd/fledge/watch_test.go` → `cmd-fledge-2`: See cmd-fledge-2.
- `cmd/fledge/workspace_test.go` → `cmd-fledge-2`: See cmd-fledge-2.
- `internal/agentcfg` → `agentcfg-catalog-scaffold`: Portable agent/profile definitions, model routing, index synchronization.
- `internal/catalog` → `agentcfg-catalog-scaffold`: Model discovery from installed integration binaries; regenerates catalog.json.
- `internal/scaffold` → `agentcfg-catalog-scaffold`: Creates and maintains the .fledge/ directory tree on init.
- `internal/contextdoc` → `contextdoc`: Context document rendering/validation: analyzer-request/reply schemas, project.md synthesis and atomic publish. Directly governs this forager/analyzer workflow.
- `internal/daemon/boundary_test.go` → `daemon-core`: Per-flock state machine: journal, placement, message delivery, isolation.
- `internal/daemon/context_message_test.go` → `daemon-core`: Managed context (forager/analyzer) message validation tests.
- `internal/daemon/daemon.go` → `daemon-core`: Core daemon server and state machine.
- `internal/daemon/delivery_order_test.go` → `daemon-core`: Message delivery ordering tests.
- `internal/daemon/e2e_test.go` → `daemon-core`: End-to-end daemon lifecycle tests.
- `internal/daemon/forager_test.go` → `daemon-core`: Forager agent type tests.
- `internal/daemon/inbox_notify.go` → `daemon-core`: Wake/inbox-notify delivery for armed agents.
- `internal/daemon/inbox_notify_test.go` → `daemon-core`: See inbox_notify.go.
- `internal/daemon/isolation_test.go` → `daemon-core`: Cross-flock isolation tests.
- `internal/daemon/journal.go` → `daemon-core`: Append-only journal read/write/replay; torn-line tolerance.
- `internal/daemon/journal_test.go` → `daemon-core`: See journal.go.
- `internal/daemon/placement.go` → `daemon-core`: Workspace/tab placement and Herdr session coordination, latching.
- `internal/daemon/placement_test.go` → `daemon-core`: See placement.go.
- `internal/daemon/ready_signal.go` → `daemon-spawn-readiness`: Agent spawn/readiness lifecycle: one-use token auth, launch/stop coordination.
- `internal/daemon/ready_test.go` → `daemon-spawn-readiness`: See ready_signal.go and spawn.go.
- `internal/daemon/reply_test.go` → `daemon-spawn-readiness`: Reply/wait-correlation tests.
- `internal/daemon/serve_test.go` → `daemon-spawn-readiness`: Socket serve-loop tests.
- `internal/daemon/socket_test.go` → `daemon-spawn-readiness`: Socket lifecycle/reclaim tests.
- `internal/daemon/spawn.go` → `daemon-spawn-readiness`: Agent spawn implementation: journaling, launch metadata, readiness latches.
- `internal/daemon/spawn_test.go` → `daemon-spawn-readiness`: See spawn.go.
- `internal/daemon/watch_test.go` → `daemon-spawn-readiness`: Watch-related daemon tests.
- `internal/herdr` → `herdr-integration`: Herdr CLI shell-out wrapper for session lifecycle (no socket API for this).
- `internal/herdrwire` → `herdr-integration`: Direct Herdr socket wire protocol (pinned protocol 16 / herdr 0.7.4) for pane ops.
- `internal/filebridge` → `herdr-integration`: Sandboxed request/response file-bridge fallback transport when the daemon socket is unavailable.
- `internal/protocol` → `support-libs`: Daemon<->client wire request/response contract types.
- `internal/scan` → `support-libs`: Workspace file scanning with ignore-rule application.
- `internal/species` → `support-libs`: Fixed penguin species pool for agent naming.
- `internal/version` → `support-libs`: Embedded VERSION single source of truth; build-tag dev suffix.
- `internal/workspace` → `support-libs`: Git-style walk-up root discovery, symlink canonicalization.
- `internal/client` → `support-libs`: Daemon unix-socket client.
- `internal/flock` → `support-libs`: Flock name validation and identity.
- `internal/ignore` → `support-libs`: .fledgeignore gitignore-syntax matching with #include support.

## Cross-Group Flows

- `cmd-fledge-1` → `daemon-spawn-readiness`: CLI agent/spawn commands issue socket requests that the daemon journals as registered/launching/spawned before Herdr placement.
- `cmd-fledge-1` → `support-libs`: CLI commands resolve the workspace root and flock identity via internal/workspace and internal/flock before any daemon dial.
- `cmd-fledge-2` → `daemon-core`: restart/stop/watch commands query and mutate daemon state (status, shutdown, roster) via the socket client.
- `agentcfg-catalog-scaffold` → `daemon-spawn-readiness`: Resolved agent definitions, profiles, and routed models feed spawn's launch metadata and instruction-hash computation.
- `daemon-core` → `herdr-integration`: Placement logic and message delivery drive Herdr workspace/tab/pane creation via herdrwire, and session lifecycle via herdr.
- `daemon-spawn-readiness` → `herdr-integration`: Spawn launches agents into Herdr panes and tears them down on failure; readiness is authenticated independently of Herdr's pane events.
- `daemon-spawn-readiness` → `herdr-integration`: Sandboxed agents unable to reach the daemon socket fall back to internal/filebridge for spawn/ready/message round-trips.
- `daemon-core` → `contextdoc`: The managed context protocol (forager request/reply messages) is schema-validated against internal/contextdoc types before being journaled as sent.
- `contextdoc` → `support-libs`: Context run-directory and published artifact paths are pinned and canonicalized via internal/workspace root discovery.
- `root-meta` → `cmd-fledge-1`: CI workflows and build/install scripts build and gate cmd/fledge as the single binary entrypoint.
- `docs` → `herdr-integration`: Frozen Stage 0 experiment findings (EXP1/EXP2/EXP3) are encoded as live invariants in herdr/herdrwire behavior (native screen detection wins; keys:["enter"] required; no pre-set concurrency cap).

## Global Invariants

- The Go CLI (daemon) is the sole state authority; Herdr and agent events are input signals only, never durable state.
- Zero inference in the orchestrator: it issues socket commands, consumes events, and advances deterministic state; all LLM inference happens inside visible, operator-interactable panes.
- The journal is append-only and written before any client is acknowledged; replay reconstructs exact state; a torn final line is tolerated but any earlier malformed line is corruption.
- Anything not journaled must not be left running: a failed spawn is torn down immediately (no dangling Herdr panes/workspaces).
- Workspace root discovery walks up git-style to the nearest .fledge/ directory then resolves symlinks, so client and daemon always agree on the canonical path that keys the socket namespace.
- Model routing is a fixed prefix table, never guessed; unknown models are a hard error.
- Agent names are kebab-case <type>-<species> drawn from a fixed species pool, one pool per flock; fledge-orchestrator is the sole no-species exception; the fledge-* namespace is reserved from user definitions.
- Herdr pane authority is native/screen-detection-first: Fledge's socket reporting is metadata-only and never seizes agent status authority on Claude panes (EXP1).
- Unix sockets live under $XDG_RUNTIME_DIR outside the workspace (108-byte sun_path cap; NFS cannot bind unix sockets there).
- Sandboxed clients that cannot open the daemon socket fall back to a workspace-local file-bridge transport carrying the full protocol.
- docs/ is a completed, frozen historical record (Stage 0); git history is not design input; only current HEAD and distilled docs/CLAUDE.md govern design.

## Subsystem: agentcfg-catalog-scaffold

The agent configuration subsystem handles portable agent definitions, launch profiles, and model catalog discovery. It synchronizes Markdown-based agent definitions into versioned JSON indexes (user-agents.json, managed-agents.json, catalog.json), routes model IDs to integrations (claude/pi/codex), validates launch configurations, and regenerates the model catalog from installed integration binaries. The scaffold module creates and maintains the .fledge directory tree and managed agent definitions.

**Purpose:** Agent configuration subsystem: agent/profile definitions, model catalog discovery, and portable Markdown agent scaffolding.

### Entry Points

- `internal/agentcfg/agentcfg.go`: Configuration loading and validation: Load() merges user/managed/catalog profiles in order; Route() maps model IDs to integrations; Validate() checks profiles; CommandArgv()/LaunchArgv() assemble launch commands
- `internal/agentcfg/definitions.go`: Markdown definition parsing and synchronization: ParseDefinition() reads frontmatter and prompt bodies; Synchronize() rebuilds indexes atomically; LoadDefinitions()/FindDefinition() provide runtime access to agent definitions and their profiles
- `internal/catalog/catalog.go`: Model discovery from installed binaries: Discover() queries claude/codex/pi for available models; Write() persists the catalog; integration-specific parsers handle version, models, and models output formats
- `internal/scaffold/scaffold.go`: Workspace initialization: Ensure() creates .fledge tree and refreshes managed definitions; EnsureGitignore() maintains runtime-state ignore patterns

### Key Symbols

- `Config` in `internal/agentcfg/agentcfg.go` (struct): Launchable agent configuration with integration, model, provider, permission_mode, sandbox, argv, env, and cwd fields
- `Route` in `internal/agentcfg/agentcfg.go` (function): Routes model ID to integration+provider by prefix table; never guesses; claude*, gpt*/codex*/o-series/opencode* are valid; others error with a working remedy hint
- `Load` in `internal/agentcfg/agentcfg.go` (function): Loads and merges Config profiles in deterministic order: user-agents.json, managed-agents.json, catalog.json; user entries shadow managed/catalog
- `Definition` in `internal/agentcfg/definitions.go` (struct): Parsed portable agent definition: name, description, tools, model, profile, prompt text, source, managed flag, launch Config, and workspace placement
- `ParseDefinition` in `internal/agentcfg/definitions.go` (function): Parses .agent.md file: validates YAML frontmatter (name, description required; tools, model, fledge.profile/launch/workspace optional); returns prompt body untrimmed
- `Synchronize` in `internal/agentcfg/definitions.go` (function): Rebuilds user and managed indexes atomically from Markdown definitions under agents/user and agents/fledge; derives profiles from model+fledge.profile; validates cross-source conflicts; writes JSON atomically with tmp+rename
- `Index` in `internal/agentcfg/definitions.go` (struct): Versioned JSON index shape: version, agents map (name→AgentRecord), profiles map (name→Config)
- `Discover` in `internal/catalog/catalog.go` (function): Queries installed integration binaries in fixed order (claude --version, codex debug models, pi --list-models); assembles named Config entries; skips missing/failing binaries gracefully; returns Notes for every skip/error
- `Write` in `internal/catalog/catalog.go` (function): Atomically replaces catalog.json with discovery results (all Config entries under an Index); deterministic because json.MarshalIndent sorts keys
- `Ensure` in `internal/scaffold/scaffold.go` (function): Creates .fledge directory tree, seeded agents stub, managed definitions (orchestrator, forager, analyzer, context profiles), and .fledgeignore; migrates legacy context profiles; preserves user edits to .fledgeignore and stub

### Dependencies

- Internal `internal/scaffold`: Provides DirName constant (.fledge), AgentsName stub filename, GitignoreEntries for runtime state, and Ensure() to initialize workspace
- Internal `internal/ignore`: Parses .fledgeignore and .gitignore patterns; used by EnsureGitignore() to check coverage before appending entries
- External `encoding/json`: Marshals and unmarshals Index, Config, and catalog JSON; MarshalIndent ensures deterministic key sort order
- External `github.com/goccy/go-yaml`: Parses YAML frontmatter in .agent.md files; unmarshals into frontMatter struct for validation
- External `crypto/sha256`: Hashes prompt bodies; PromptHash in AgentRecord lets consumers detect definition changes without storing the full prompt
- External `os/exec`: Spawns integration binaries (claude, codex, pi) for model discovery; context.WithTimeout bounds each execution

### Data Flows

- `.fledge/agents/user/**/*.agent.md` → `internal/agentcfg/definitions.go:ParseDefinition`: User definitions on disk are parsed; frontmatter extracted; profile references validated
- `internal/agentcfg/definitions.go:Synchronize` → `.fledge/agents/fledge/user-agents.json`: Parsed user definitions indexed deterministically; atomically written with tmp+rename; profile derivations included
- `internal/agentcfg/definitions.go:Synchronize` → `.fledge/agents/fledge/managed-agents.json`: Parsed managed definitions (from .fledge/agents/fledge/) indexed; managed-only profiles included
- `internal/catalog/catalog.go:Discover` → `installed binaries (claude, codex, pi)`: Spawns each integration with model-list arguments; parses stdout; builds named Config entries from discovered models
- `internal/catalog/catalog.go:Discover` → `.fledge/agents/fledge/catalog.json`: Generated machine-specific model catalog; reflects currently-installed integrations and their available models
- `internal/agentcfg/agentcfg.go:Load` → `daemon, CLI commands`: Reads all three indexes in order; returns merged map of launchable profiles; user entries shadow generated ones
- `internal/agentcfg/agentcfg.go:Route` → `spawn/agent commands`: CLI passes a model ID; Route looks it up in fixed prefix table; returns integration (claude/pi/codex) and provider (pi-only)
- `CLI spawn flags` → `internal/agentcfg/agentcfg.go:Validate`: Validates permission_mode (claude), sandbox (codex), provider (pi), argv restrictions, and integration-specific combinations

### Invariants

- Model routing is deterministic prefix-match-only; no guessing or defaults; unknown models error with a working remedy hint (fledge.profile reference)
- User definitions and profiles shadow identically-named managed and catalog entries; no merging or conflict resolution within a name
- Profile derivation requires both a model (routable) and fledge.profile name if launch fields are present; missing either is an error
- Managed agent names use the reserved fledge-* namespace; user agents must not; namespace violation is caught at parse time
- Index writes are atomic (tmp file + rename) to survive crashes; malformed earlier lines in journal are corruption; final torn line is tolerated
- Generated indexes (user-agents.json, managed-agents.json, catalog.json) live in .fledge/agents/fledge/; only user definitions live in .fledge/agents/user/
- Portable agent definition paths must be agents/<source>/<name>/<name>.agent.md; folder name, filename, and frontmatter name must all match
- Profile conflicts (same name, different Config) across sources are errors; Synchronize hard-fails before writing any index
- Model discovery runs in fixed order (claude, codex, pi) with 30s timeout per integration; missing/failing binaries are noted, not errors
- Discovered launcher names are always suffixed (cl, cx, pi, oc, og, or provider slug) so the same model doesn't change names when re-discovered
- CommandArgv and LaunchArgv are deterministic; profile argv is option-only and never contains --; Fledge appends integration identity and instruction after profile argv
- Workspace metadata (label, tab) must be single trimmed lines with no newlines/nulls; non-text files treated as metadata-only, contents never invented
- Permission_mode, sandbox, and provider are integration-specific; cross-checks reject mismatched integration+field combinations

### Tests

- `internal/agentcfg/agentcfg_test.go`: TestRoute: covers all model prefixes and rejects unknown models with a working remedy hint
- `internal/agentcfg/agentcfg_test.go`: TestValidate: covers valid configs, name formats, integration combinations, and field isolation (provider on pi only, etc.)
- `internal/agentcfg/agentcfg_test.go`: TestCommandArgv: assembles launch commands correctly for each integration; verifies option ordering and session ID placement
- `internal/agentcfg/agentcfg_test.go`: TestLoadValid, TestLoadCatalogOnly, TestLoadUserShadowsCatalog: merges indexes in order; user entries shadow generated ones
- `internal/agentcfg/definitions_test.go`: TestSynchronizeDerivesProfileAndWritesDeterministicIndex: parses definitions, derives profiles, writes deterministic JSON; ensures rewrite produces identical bytes
- `internal/agentcfg/definitions_test.go`: TestSynchronizeValidatesPathAndNamespaces: rejects mismatched paths, reserved namespace in user definitions, managed agents outside fledge-* namespace
- `internal/agentcfg/definitions_test.go`: TestSynchronizeRejectsConflictingProfileDeclarations: hard-fails when two definitions declare the same profile with different Configs
- `internal/catalog/catalog_test.go`: TestDiscoverNamesEverySourceAlways: discovers models from all three integrations; verifies every entry validates and is suffixed
- `internal/catalog/catalog_test.go`: TestDiscoverSkipsMissingBinary, TestDiscoverSkipsFailingBinary: gracefully skips missing/failing integrations; other sources still contribute; Notes explain every skip
- `internal/catalog/catalog_test.go`: TestRunDeadlineReportsTimeout: 30s timeout per binary; context.DeadlineExceeded is caught and reported separately from generic failures
- `internal/scaffold/agents_test.go`: TestAgentsNameMatchesAgentcfg: verifies scaffold.AgentsName constant equals agentcfg.FileName (no drift in file paths)
- `internal/scaffold/agents_test.go`: TestAgentsStubIsEmptyAndLoads: seeded agent stub loads as empty Config map
- `internal/scaffold/scaffold_test.go`: TestEnsureCreatesTree: creates all subdirs, .fledgeignore, agent stubs, and managed definitions
- `internal/scaffold/scaffold_test.go`: TestEnsureRefreshesEveryManagedDefinition: overwrites stale managed definitions on re-run
- `internal/scaffold/scaffold_test.go`: TestEnsureMigratesKnownLegacyContextProfiles: removes legacy context-haiku-auto and context-sonnet-auto when their bytes match the old templates

### Files

#### internal/agentcfg/agentcfg.go

_text._ Public API for loading and validating launch configurations; defines Config struct, Load/Route/Validate functions, CommandArgv/LaunchArgv builders, and ReservedOrchestrator constant; no inference or defaults

#### internal/agentcfg/agentcfg_test.go

_text._ Tests for Route, Validate, CommandArgv, LaunchArgv, Load semantics, and name validation; covers all integration combinations and field isolation rules

#### internal/agentcfg/definitions.go

_text._ Markdown parsing and index synchronization; ParseDefinition extracts YAML frontmatter and prompt; Synchronize rebuilds user/managed indexes atomically; LoadDefinitions and FindDefinition provide runtime lookup; legacy migration helper

#### internal/agentcfg/definitions_test.go

_text._ Tests for definition parsing, synchronization atomicity, profile derivation, namespace/path validation, conflict detection, and legacy index migration

#### internal/catalog/catalog.go

_text._ Model discovery from installed integration binaries; Discover queries claude/codex/pi in fixed order; gracefully handles missing/failing binaries; generates named Config entries with source suffixes; deterministic Write with tmp+rename

#### internal/catalog/catalog_test.go

_text._ Tests for parsing codex JSON and pi fixed-width output; Discover integration with fake binaries; timeout and failure handling; collision resolution; determinism; and cross-module collision-freedom with user profiles

#### internal/scaffold/agents_test.go

_text._ Cross-package test verifying scaffold.AgentsName matches agentcfg.FileName constant and that generated stub loads as empty; ensures .gitignore covers fledge/agents/fledge/

#### internal/scaffold/scaffold.go

_text._ Workspace initialization: Ensure creates .fledge tree, seeded stubs, and managed definitions (orchestrator, forager, analyzer, context profiles); handles legacy migration; EnsureGitignore appends runtime-state entries idempotently

#### internal/scaffold/scaffold_test.go

_text._ Tests for directory creation, managed definition refresh, legacy profile migration (with modification detection), .fledgeignore template (with .gitignore include logic), and gitignore entry management

## Subsystem: cmd-fledge-1

CLI entrypoint system implementing fledge command dispatcher, context inspection (scan/graph), workspace orchestration (flock lifecycle), and agent lifecycle management with message routing. Comprehensive command parsing and help system.

**Purpose:** CLI entrypoint core: context/help/main/watch command implementations and their direct tests.

### Entry Points

- `cmd/fledge/main.go`: Main entry point and command dispatcher; routes all fledge CLI commands through run() function
- `cmd/fledge/help.go`: Centralized help system with helpPages map covering all commands and help routing logic
- `cmd/fledge/context.go`: Context command implementations for workspace scanning and directory graph visualization

### Key Symbols

- `run` in `cmd/fledge/main.go` (function): Core command dispatcher routing all top-level subcommands (init, start, context, agent, etc.)
- `runContext` in `cmd/fledge/main.go` (function): Context subcommand dispatcher routing to scan, graph, validate, and render-project
- `scanContext` in `cmd/fledge/context.go` (function): Resolves workspace root and applies ignore rules to produce scannedContext view
- `buildContextGraph` in `cmd/fledge/context.go` (function): Transforms scanned files into hierarchical directory graph with size/count aggregation
- `runAgentSpawn` in `cmd/fledge/main.go` (function): Agent spawn orchestrator handling profile selection, model routing, workspace placement
- `runFlockClear` in `cmd/fledge/main.go` (function): Interactive flock state cleanup with managed session teardown and orphan session removal
- `helpPages` in `cmd/fledge/help.go` (variable): Centralized map of all command help text organized by command path
- `usageErrorf` in `cmd/fledge/main.go` (function): Creates usageError with contextual help page for command-line syntax errors
- `takeFlag` in `cmd/fledge/main.go` (function): Extracts flag and value from args list, returns value and remaining args
- `flockArg` in `cmd/fledge/main.go` (function): Resolves flock name from explicit argument or FLEDGE_FLOCK environment variable

### Dependencies

- Internal `internal/workspace`: Workspace root discovery (FindRoot) using git-style walk-up
- Internal `internal/scaffold`: .fledge directory management and initialization
- Internal `internal/agentcfg`: Agent definitions, profiles, and configuration synchronization
- Internal `internal/catalog`: Model catalog discovery and management for spawning
- Internal `internal/client`: Socket client for daemon communication (register, spawn, list, msg ops)
- Internal `internal/daemon`: Daemon lifecycle (spawn, status, shutdown) and journal management
- Internal `internal/flock`: Flock naming, validation, listing, and directory management
- Internal `internal/herdr`: Herdr session lifecycle (create, stop, delete, find, up checks)
- Internal `internal/herdrwire`: Herdr wire protocol for workspace/tab creation and pane operations
- Internal `internal/protocol`: Socket protocol definitions (Request/Response types and operation codes)
- Internal `internal/scan`: Workspace file scanning with ignore rules
- Internal `internal/ignore`: Ignore file parsing for .fledgeignore matching
- Internal `internal/contextdoc`: Context document validation and project rendering
- Internal `internal/version`: Version information retrieval
- External `os`: Process and environment manipulation, filesystem operations
- External `os/exec`: External command execution (herdr CLI, daemon spawn)
- External `syscall`: Low-level syscall for setsid (process group) and getsid
- External `encoding/json`: JSON encoding/decoding for API responses and config
- External `path/filepath`: Path manipulation and traversal
- External `bufio`: Buffered I/O for stdin prompts and log tailing
- External `time`: Duration parsing and deadline management
- External `fmt`: Formatted output and error messages
- External `strings`: String manipulation and parsing

### Data Flows

- `CLI args` → `run()`: Command-line arguments parsed and dispatched to subcommand handlers
- `workspace root` → `context.go:scanContext`: Workspace directory discovered, ignore rules loaded, files scanned
- `scanned files` → `buildContextGraph`: Files aggregated into hierarchical directory structure with size/count calculations
- `agent definitions` → `runAgentSpawn`: Loaded definitions checked for profile requirements, user prompted for selection if needed
- `spawn request` → `client.Do()`: Protocol request sent to daemon socket for agent launch
- `flock list` → `runFlockClear`: Flock states enumerated, liveness checked, interactive confirmation obtained
- `agent ready` → `daemon.WriteReadySignal`: Readiness token and session ID written as fallback when daemon socket unavailable
- `agent msg ops` → `acknowledgeMessage`: Received messages acknowledged via OpAck after JSON output succeeds
- `start attach callback` → `startAfterAttach goroutine`: Orchestrator spawn initiated asynchronously after herdr terminal handoff
- `daemon status` → `awaitSpawn`: Spawn outcome resolved against attach error to determine start success/failure

### Invariants

- All flock names validated through flock.Validate before use; invalid names rejected with descriptive error
- Help pages always accompany syntax/usage errors through usageError wrapper; bare runtime failures report directly
- Flag parsing completed before positional argument checking; rejectFlags catches leftover flags for unified error handling
- Interactive commands (clear, stop, deinit) require both stdin and stdout terminals; non-terminal stdin/stdout causes immediate refusal
- Flock argument resolution: explicit positional > FLEDGE_FLOCK environment > error requiring one of both
- Start rollback: half-finished start always tears down flock to avoid stranded daemons; only partial failures (watcher) are non-critical
- Ready token lifecycle: one-use token validated before wait; fallback signal written only when socket unavailable, never exposes token
- Message acknowledgment: only called after successful JSON output to ensure interrupted output preserves message delivery
- Graph building: ignored files permanently pruned; selecting ignored directory still produces empty root node, never reactivates pruned items
- Agent liveness probed by PID signal-0; session leader used by default (not immediate parent) to survive transient sh -c processes
- Managed herdr sessions (fledge- prefix) deleted post-stop; operator-named sessions only stopped, never deleted
- Spawn placement: --workspace and --tab required together; incompatible with pi profiles; validated before daemon request
- Configuration loading: agentcfg.Synchronize called before any config-dependent operation to ensure indexes match definitions
- Workspace root: git-style walk-up to .fledge directory via workspace.FindRoot; canonicalized via EvalSymlinks for consistent socket namespace

### Tests

- `cmd/fledge/agent_definitions_test.go`: Agent definition registration, readiness authentication, token fallback signal, and metadata carrying tests
- `cmd/fledge/behavior_test.go`: Help system, flock listing/status, agent registration, agent messaging (send/wait/reply correlation), duration parsing tests
- `cmd/fledge/clear_test.go`: Flock clear lifecycle: terminal checks, running skip, liveness recheck before deletion, orphan session removal, partial failure handling
- `cmd/fledge/graph_test.go`: Context graph human/JSON output, scope filtering, recursive size/count calculation, ignore semantics, empty scope handling
- `cmd/fledge/context_pipeline_test.go`: Spawn placement selectors, message send/reply correlation, inbox claiming, ready no-wait, analyzer request/reply validation, project rendering

### Files

#### cmd/fledge/agent_definitions_test.go

_text._ Tests agent type listing, definition registration with metadata carrying, ready authentication sequence with token validation, fallback ready signal, and --no-wait name output with manual delivery warning

#### cmd/fledge/behavior_test.go

_text._ Tests human-readable help output, flock list/status with up/down distinction, agent registration and listing with JSON, agent message send/wait/reply correlation with ID tracking, duration parsing, and environment variable requirements

#### cmd/fledge/clear_test.go

_text._ Tests flock clear syntax, terminal requirement, running flock skipping with racecheck, orphan session enumeration and removal, partial failure aggregation, and session cleanup before state removal

#### cmd/fledge/context.go

_text._ Context command dispatcher routing to scan/graph/validate/render-project; scanContext resolving workspace and applying ignores; buildContextGraph constructing hierarchy with recursive aggregation; printContextGraph rendering ASCII tree

#### cmd/fledge/context_pipeline_test.go

_text._ Tests agent spawn placement selectors (workspace/tab), message send with body file/stdin, reply body handling, wait filtering by sender/reply-to, inbox non-blocking claiming, analyzer request/reply validation with correlation, and project rendering

#### cmd/fledge/graph_test.go

_text._ Tests context graph human tree formatting, JSON schema with totals and containment edges, default/explicit scope filtering, ignore semantics with negation, empty and fully-ignored scopes, and help routing

#### cmd/fledge/help.go

_text._ Help pages map covering all commands with detailed usage, flags, and subcommand routing; usageError wrapper carrying contextual help; runHelp falling back to deepest valid page; printHelp and isHelpFlag utility functions

#### cmd/fledge/main.go

_text._ 2690-line main command implementation: run() dispatcher, context/flock/agent subcommand handlers, workspace initialization (init/deinit), start/stop/restart/watch orchestration, agent spawn/ready/msg protocol implementation, flag parsing (takeFlag, takeBoolFlag, rejectFlags), interactive terminal prompts, help system integration, and daemon lifecycle management

## Subsystem: cmd-fledge-2

CLI entrypoint tests and remaining commands subsystem covering main parser, restart, scan, stop, watch, and workspace functionality. The module validates command routing, flag parsing, help text generation, daemon lifecycle management, and interactive terminal flows.

**Purpose:** CLI entrypoint tests and remaining commands: main/parser/restart/scan/stop/watch/workspace test suites.

### Entry Points

- `cmd/fledge/watch.go`: Log watcher implementation for monitoring flock daemons; runWatch entry point

### Key Symbols

- `run` in `cmd/fledge/main_test.go` (function): Routes command arguments to subcommand handlers; core CLI dispatcher
- `captureRun` in `cmd/fledge/main_test.go` (function): Test helper that captures stdout from run() for assertion
- `helpPages` in `cmd/fledge/main_test.go` (variable): Map of command path to help text strings used for validation
- `agentRows` in `cmd/fledge/main_test.go` (function): Formats agent list output with parity for spawned/unserialized agents
- `pickerRows` in `cmd/fledge/main_test.go` (function): Renders agent picker menu grouped by provider
- `modelRows` in `cmd/fledge/main_test.go` (function): Renders model catalog grouped by provider
- `pickAgentConfig` in `cmd/fledge/main_test.go` (function): Interactive agent selection by number or name
- `guardedBringUp` in `cmd/fledge/main_test.go` (function): Manages transactional session creation and rollback on daemon spawn failure
- `awaitSpawn` in `cmd/fledge/main_test.go` (function): Waits for async daemon spawn with attach failure handling
- `takeFlag` in `cmd/fledge/parser_test.go` (function): Hand-rolled flag parser rejecting flag-shaped values
- `takeBoolFlag` in `cmd/fledge/parser_test.go` (function): Boolean flag parser for flags without values
- `rejectFlags` in `cmd/fledge/parser_test.go` (function): Validates remaining args contain no unknown flags
- `restartDaemonStatus` in `cmd/fledge/restart_test.go` (function): Queries daemon status; stubbed during testing
- `restartSpawnDaemon` in `cmd/fledge/restart_test.go` (function): Spawns replacement daemon; stubbed during testing
- `waitSpawnDaemonReady` in `cmd/fledge/restart_test.go` (function): Polls daemon until exact session binding or unbound readiness
- `contextdoc.Scan` in `cmd/fledge/scan_test.go` (type): JSON-serializable result of context scan with root and file list
- `watchDaemonLog` in `cmd/fledge/watch_test.go` (function): Polls daemon log file and emits appended content
- `installWatcherPane` in `cmd/fledge/watch_test.go` (function): Replaces CLI pane with fledge watch command in existing session
- `runWatch` in `cmd/fledge/watch.go` (function): Entry point for watch command; validates args and starts log watcher
- `scaffoldedWorkspace` in `cmd/fledge/workspace_test.go` (function): Creates temp workspace with .fledge tree for testing
- `interactiveStart` in `cmd/fledge/workspace_test.go` (function): Simulates interactive start flow with fake session and herdr
- `fakeHerdr` in `cmd/fledge/workspace_test.go` (function): Installs stub herdr CLI recording session operations
- `liveSocket` in `cmd/fledge/workspace_test.go` (function): Listening socket replaying herdr wire protocol for testing

### Dependencies

- Internal `internal/agentcfg`: Agent configuration, catalog, and profile management
- Internal `internal/daemon`: Daemon process lifecycle and socket communication
- Internal `internal/protocol`: Client-daemon wire protocol and types
- Internal `internal/scaffold`: Workspace tree initialization
- Internal `internal/client`: CLI-side daemon communication
- Internal `internal/flock`: Flock directory and environment management
- Internal `internal/herdrwire`: Herdr socket protocol for pane operations
- Internal `internal/contextdoc`: Context scan result types
- Internal `internal/workspace`: Workspace root discovery
- Internal `internal/version`: Daemon and CLI version strings
- External `testing`: Go standard test package
- External `os`: Process, signal, and file I/O
- External `path/filepath`: Path manipulation
- External `strings`: String operations
- External `encoding/json`: JSON marshaling
- External `errors`: Error wrapping
- External `io`: I/O operations
- External `net`: Unix socket networking
- External `context`: Context cancellation
- External `time`: Timing and polling
- External `bufio`: Buffered I/O
- External `sync`: Concurrency primitives

### Data Flows

- `run()` → `route subcommands`: Main dispatcher interprets command arguments and routes to specific command handlers
- `takeFlag/takeBoolFlag` → `rejectFlags`: Flag parsing is two-phase: consume known flags, then reject unknown ones
- `restartDaemonStatus` → `version verification`: Restart validates daemon PID changed and version matches before completing
- `scaffoldedWorkspace` → `test setup`: Test helper creates isolated workspace trees avoiding side effects
- `interactiveStart` → `agent spawn flow`: Orchestrates fake herdr session, daemon, and CLI to test interactive startup
- `watchDaemonLog` → `installWatcherPane`: Log watcher runs as CLI pane shell command; pane install sets up the flow
- `liveSocket/wireRecorder` → `test assertions`: Fake herdr socket records all wire calls for method/param inspection

### Invariants

- Flag parsing must reject flag-shaped positionals before they consume option values
- Restart must validate both PID change and version match to confirm successful replacement
- Daemon spawn failures during session creation trigger automatic session cleanup via guardedBringUp
- Interactive start attaches the UI before spawning the orchestrator agent
- Watch command reuses the existing CLI pane via herdr wire protocol
- Context scan walks up git-style to find workspace root and applies ignore patterns
- Agent list formatting maintains backward compatibility for pre-spawn registrations
- Test flock names follow FLEDGE_FLOCK environment or explicit positional argument
- Scaffolded workspaces include a pre-initialized .fledge tree with agent catalog
- Help text is embedded in helpPages map and served by run() on help requests

### Tests

- `cmd/fledge/main_test.go`: root help, nested help routing, bare groups, agent rows, picker rows, model rows, config picking, deinit interactive flows, agent type listing, init discovery and catalog generation
- `cmd/fledge/parser_test.go`: takeFlag flag-shaped value rejection, takeBoolFlag short/long forms, rejectFlags validation, agent register species validation, agent msg wait timeout validation, msg send flag-shaped positional handling
- `cmd/fledge/restart_test.go`: spawn daemon readiness waiting, restart status/shutdown/spawn lifecycle, environment/explicit flock selection, down restart error, legacy shutdown guidance, replacement failure handling, post-spawn verification
- `cmd/fledge/scan_test.go`: context scan workspace root resolution from subdir, dir arg limiting, ignore patterns, workspace error handling, JSON output contract validation
- `cmd/fledge/stop_test.go`: stop terminal requirements, no-flock noop, scoped stop, decline behavior, partial failure continuation, confirmed stop with real daemon
- `cmd/fledge/watch_test.go`: watch flock selection, validation and syntax, running daemon requirement, history plus append ordering, daemon shutdown reporting, watcher pane installation and failure recovery, orchestrator layout ordering
- `cmd/fledge/workspace_test.go`: flock status/agent list from subdirectory, start reuses running flock, stale managed session recreation, herdr/daemon execution from workspace root, managed session deletion on stop, operator session preservation, workspace creation at root, init nested workspace warning, interactive start flow with picker/placement/rollback, scripted start without orchestrator, workspace and tab labeling

### Files

#### cmd/fledge/main_test.go

_text._ Comprehensive CLI tests: root/nested help routing, agent/model list formatting with launch column parity, interactive agent picker with menu grouping and selection by number/name, deinit interactive flow with terminal check and confirm/decline paths, init discovery and catalog writing, help page consistency, start/deinit/stop terminal requirements, guardedBringUp session lifecycle, awaitSpawn spawn failure handling. Tests are integration-level, exercising the full run() dispatch and output capture.

#### cmd/fledge/parser_test.go

_text._ Flag parsing tests: takeFlag with flag-shaped value rejection (the core safety invariant), short/long form handling, stdin marker '-' acceptance, takeBoolFlag present/absent cases, rejectFlags unknown flag detection. End-to-end repro for agent register species injection attack and agent msg wait timeout validation. Tests isolate the parser layer before it reaches the daemon.

#### cmd/fledge/restart_test.go

_text._ Restart lifecycle tests: daemon status query stub, spawn stub with failure tracking, waitSpawnDaemonReady exact session or unbound matching, environment vs explicit flock selection, down daemon error handling, legacy shutdown compatibility, spawn failure with log guidance, post-spawn status/PID/version/session verification failures. Covers all verification failure modes and guidance text.

#### cmd/fledge/scan_test.go

_text._ Context scan tests: workspace root git-style walk-up from subdirectory, dir arg subtree limiting, ignore pattern application, JSON output schema contract validation (schema_version, file_count, total_size derivation), outside-workspace error, empty result handling. Tests validate the scan coordination with .fledgeignore matching.

#### cmd/fledge/stop_test.go

_text._ Stop command tests: terminal requirement (both stdin and stdout), no-flock noop without prompt, scoped stop (FLEDGE_FLOCK env) preview and single-flock execution, decline paths (n/eof/enter/nope), stopFlocks partial failure continuation with per-flock error reporting, confirmed stop with real daemon teardown and managed session deletion. Uses stubStdinTerminal and fakeHerdr for integration.

#### cmd/fledge/watch.go

_text._ Watch command implementation: runWatch entry point with arg validation and help routing, watchDaemonLog append-only polling via file descriptor, installWatcherPane pane.send_input and pane.focus via herdrwire, warnWatcherFailure recovery hint, shellQuote POSIX escaping for error messages. Single 100ms poll interval as module-level var for testing override.

#### cmd/fledge/watch_test.go

_text._ Watch tests: flock selection (explicit arg > FLEDGE_FLOCK env), validation/syntax error routing, running daemon requirement, history emission plus append ordering with concurrent log writes, daemon shutdown detection, watcher pane installation with shell reuse, failures keep primary layout and emit manual recovery warnings. Uses wireRecorder to inspect herdr protocol calls and verify no workspace/tab creation during watcher setup.

#### cmd/fledge/workspace_test.go

_text._ Workspace/start tests: flock status and agent list work from subdirectories via workspace root resolution, start reuses running flock without creating workspaces, stale managed session recreation via stop/delete/start sequence, herdr/daemon exec from workspace root with session binding, managed session deletion vs operator session preservation, workspace creation at root before attach, init nested workspace warning, interactive start with attach-first then spawn, orchestrator placement (left via pane.swap), watcher pane installation, picker cancellation/empty catalog rollback, scripted start skips orchestrator and picker, workspace/tab labeling. Largest test file with fake herdr and wire recording.

## Subsystem: contextdoc

Context document rendering and validation subsystem provides facilities for agents to describe analyzed project subsystems (JSON wire format, schema validation) and synthesizes them into a deterministic, structured Markdown artifact (.fledge/context/project.md) that serves as the project's current canonical context.

**Purpose:** Context document rendering and validation: builds and validates the published project.md context artifact.

### Entry Points

- `internal/contextdoc/render.go`: RenderProject validates all artifacts below a run directory, atomically replaces the workspace context document, and removes consumed JSON; starts the full render pipeline
- `internal/contextdoc/validate.go`: ValidateAnalyzerRequest, ValidateAnalyzerReply provide in-memory validation entry points for the daemon to validate analyzer requests and correlate replies with requests

### Key Symbols

- `AnalyzerRequest` in `internal/contextdoc/types.go` (struct): Wire contract for agents to describe their assigned file group: schema version, group ID, purpose, file list with sizes, total size; validated and cached
- `AnalyzerReply` in `internal/contextdoc/types.go` (struct): Normalized analyzer reply capturing subsystem analysis: status, group ID, summary, entry points, symbols, dependencies, data flows, invariants, tests, files, error list
- `Synthesis` in `internal/contextdoc/types.go` (struct): Cross-group synthesis summary: project overview, routing table (path prefixes to group IDs), cross-group data flows, global invariants
- `Provenance` in `internal/contextdoc/types.go` (struct): Agent execution provenance: forager identity, per-group analyzer identities (names, profiles, models), creation timestamp
- `validRelativePath` in `internal/contextdoc/validate.go` (function): Validates path safety: filesystem.ValidPath, no backslashes, normalized via path.Clean; enforces safe relative paths across all path-valued fields
- `preflightRun` in `internal/contextdoc/render.go` (function): Preflight security: resolves workspace root via EvalSymlinks, verifies run directory is canonical (no symlinks) and within runs/ tree, pins all directories and artifact file handles before content read
- `loadContextRun` in `internal/contextdoc/render.go` (function): Loads and cross-validates scan, all requests, matching replies, synthesis, and provenance; validates file ownership uniqueness and internal-dependency scanned-path matching
- `renderMarkdown` in `internal/contextdoc/render.go` (function): Generates deterministic Markdown document: provenance section (timestamps, agent identities, profile/model counts), project overview, routing table, cross-group flows, global invariants, per-subsystem sections with entry points/symbols/dependencies/flows/invariants/tests/file summaries
- `decodeExact` in `internal/contextdoc/validate.go` (function): Strict JSON decoder: rejects duplicate object keys, requires exact shape match (no unknown fields, no null values for non-optional), strict number parsing; validates both structural and semantic exactness
- `writeAtomic` in `internal/contextdoc/render.go` (function): Atomic document publication: creates temporary file in target directory root, writes data, syncs, atomically renames to project.md, syncs directory for durability

### Dependencies

- Internal `internal/scaffold`: DirName constant (.fledge) used for context document path construction
- Internal `internal/workspace`: FindRoot function to locate canonical workspace root when validating run directories
- External `crypto/sha256`: SHA-256 hashing for document integrity verification in RenderResult
- External `encoding/json`: JSON encoding/decoding for all wire contracts and validation workflows
- External `regexp`: kebab-case pattern for group_id validation

### Data Flows

- `AnalyzerRequest (agent)` → `Scan (forager)`: Agents describe file groups they've analyzed; requests must reference files present in the scan (file ownership is exclusive across all requests)
- `AnalyzerReply (agent)` → `AnalyzerRequest (same agent)`: Replies correlate to requests by group_id; all assigned files must have summaries, entry points/symbols/tests must reference text-content files only
- `Synthesis (orchestrator)` → `AnalyzerRequest set (all agents)`: Synthesis provides routing (path prefixes to groups), cross-group flows, and global invariants; routing must cover scanned paths and respect file ownership
- `Provenance (orchestrator)` → `Request + Reply set`: Provenance records forager and per-analyzer identities (name, profile, model); every request must have a corresponding analyzer in provenance
- `contextRun (loaded)` → `project.md document`: Renders synthesized Markdown containing provenance, project overview, routing, cross-group flows, global invariants, and per-subsystem analysis sections
- `project.md (published)` → `run JSON artifacts (cleanup)`: After successful publication, all consumed JSON files and directories (scan, requests, replies, synthesis, provenance) are removed from the runs/ tree

### Invariants

- Run directory must be canonical (no symlink components) and strictly contained within .fledge/context/runs/ — validated and pinned before any artifact read
- All scanned files are partitioned exactly once across request groups (no file is unassigned or owned by multiple groups)
- Every request has a matching reply with the same group_id; reply status must be 'ok' (error replies cause immediate failure)
- All entry points, key symbols, and tests reference text-content files only (content_kind='text'); non-text files may only be referenced in file summaries
- All internal dependencies must match a scanned path or directory prefix; path traversal (../) is rejected
- Routing table covers all scanned paths and respects file ownership: each path_prefix must match files owned by its designated group_id
- Cross-group flows reference only defined groups; all fields are semantic (non-empty/trimmed)
- Provenance agent names are globally unique; every request has an analyzer with matching group_id
- Document publication is atomic: written to temporary file in pinned context root, synced, then renamed; post-publication failures become warnings, not rollbacks
- JSON wire format is exact: duplicate keys rejected, all required fields present, unknown fields rejected, no null values where not explicitly optional

### Tests

- `internal/contextdoc/contextdoc_test.go`: TestValidateAnalyzerRequestRejectsMalformedAndDuplicateData validates request decoder rejects malformed JSON, duplicate fields, missing required fields, unsafe paths, size mismatches
- `internal/contextdoc/contextdoc_test.go`: TestValidateAnalyzerRequestEnforcesGroupingBounds validates file and byte limits (50 files, 256KB oversized singleton rule)
- `internal/contextdoc/contextdoc_test.go`: TestValidateAnalyzerReplyCorrelatesRequestAndNonText validates reply cross-checks group_id, file coverage exactness, content_kind restrictions (text-only for symbols/entry points)
- `internal/contextdoc/contextdoc_test.go`: TestValidateAnalyzerReplyAllowsSafeUnassignedInternalDependency validates internal dependencies can reference unassigned project paths but must be safe/normalized
- `internal/contextdoc/contextdoc_test.go`: TestValidateAnalyzerErrorReply validates error-status replies with error code/message and optional assigned paths
- `internal/contextdoc/contextdoc_test.go`: TestValidateAnalyzerReplyRejectsEmptySemanticFields validates all description/name/summary fields are non-empty and trimmed
- `internal/contextdoc/contextdoc_test.go`: TestRenderProjectRendersAllSectionsAndCleansRun validates full render pipeline: loads all artifacts, generates deterministic Markdown, publishes atomically, cleans run directory
- `internal/contextdoc/contextdoc_test.go`: TestRenderProjectCorrelatesInternalDependenciesWithScan validates internal dependencies match scanned paths; absent dependencies cause failure
- `internal/contextdoc/contextdoc_test.go`: TestRenderProjectRejectsUnconfinedAndSymlinkedInputs validates symlink detection, directory confinement, and preflight rejection on open
- `internal/contextdoc/contextdoc_test.go`: TestRenderProjectPinnedRunCannotBeRedirectedAfterOpen validates TOCTOU protection: directories/files pinned before content read; post-open mutations cause cleanup refusal warnings
- `internal/contextdoc/contextdoc_test.go`: TestRenderProjectPublicationUsesPinnedContextRoot validates publication targets the opened context root, immune to symlink or directory replacement attacks
- `internal/contextdoc/contextdoc_test.go`: TestRenderProjectPostPublicationFailuresAreWarnings validates directory sync and cleanup failures post-publication are warnings, not rollbacks
- `internal/contextdoc/contextdoc_test.go`: TestRenderProjectFailuresPreserveDocumentAndArtifacts validates validation failures leave prior document and all run artifacts unchanged

### Files

#### internal/contextdoc/contextdoc_test.go

_text._ Comprehensive test suite: malformed JSON and duplicate key rejection, grouping bounds (50 files, 256KB), reply correlation and content_kind restrictions, internal dependency scanned-path validation, symlink + confinement rejection, TOCTOU protection, file ownership synthesis validation, semantic field emptiness, post-publication failure handling, artifact preservation on error

#### internal/contextdoc/render.go

_text._ Document rendering and publication pipeline: preflight (security validation + handle pinning), load + cross-validate all artifacts, generate Markdown (provenance/overview/routing/flows/invariants/subsystems), atomic write, cleanup. TOCTOU protection via os.Root and file handle pinning; all paths validated as safe normalized relative paths

#### internal/contextdoc/types.go

_text._ JSON wire contracts and normalized types: SchemaVersion constant, File/Scan (input metadata), AnalyzerRequest/Reply/Error (agent output), Synthesis/Routing/CrossGroupFlow/Provenance (orchestrator summary), RenderResult (publication outcome)

#### internal/contextdoc/validate.go

_text._ Strict JSON validation and exact decoding: ValidateAnalyzerRequest/Reply entry points, schema shape validation (no unknown fields, required fields, exact structure), duplicate key rejection, safe path validation, file grouping uniqueness, content kind restrictions, semantic field (non-empty/trimmed) validation

## Subsystem: daemon-core

Fledge daemon is a per-flock state machine that binds agents, distributes messages, and manages agent lifecycle (spawning, placement, stopping) via durable append-only journal. Core invariant: every state-changing operation is journaled before acknowledgment; replay from journal reconstructs exact state after restart. Message delivery uses acknowledged wait + eager-delivery paths; placement logic coordinates Herdr session for workspaces/tabs; managed context protocol (forager↔analyzer) validates schema before send.

**Purpose:** Daemon core state machine: main daemon loop, journal replay, placement logic, and message delivery/isolation tests.

### Entry Points

- `internal/daemon/daemon.go`: Run/RunBound starts daemon, New initializes from replayed journal, Serve accepts socket connections, dispatch routes operations
- `internal/daemon/journal.go`: replay reconstructs state from journal; append/appendAll write events durably with fsync before ack
- `internal/daemon/placement.go`: acquirePlacement resolves workspace/tab selectors and creates owned tabs; recoverOwnedTabs cleans up crash-left ephemeral tabs
- `internal/daemon/inbox_notify.go`: runInboxNotifier delivers inbox wake signals to armed orchestrator agents with exponential backoff on failure

### Key Symbols

- `Daemon` in `internal/daemon/daemon.go` (struct): Per-flock daemon state: socket listener, journal writer, mutex-protected agent roster, pending/delivered messages, placement state, inbox notifier tasks
- `Run/RunBound` in `internal/daemon/daemon.go` (function): Entry points: Run starts unbound daemon; RunBound binds to Herdr session and blocks until session ends or listener closes
- `dispatch` in `internal/daemon/daemon.go` (function): Routes OpRegister/OpList/OpStatus/OpSend/OpReply/OpInbox/OpWait/OpPeek/OpReceive/OpAck/OpSpawn/OpReady/OpStop/OpShutdown to handlers
- `send/reply` in `internal/daemon/daemon.go` (function): sendReq creates protocol.Message, authorizes actor, journals evSent, matches waiting receiver or enqueues pending, triggers inbox notify
- `wait/receive` in `internal/daemon/daemon.go` (function): wait blocks on incoming message; acknowledge=false eager-delivers (legacy); receive leaves message pending until explicit ack
- `authorizeMessageActorLocked/validateMessageRecipientLocked` in `internal/daemon/daemon.go` (function): Authenticates identity token hash; checks agent state, stopping, workspace-closing for messaging authority
- `replay` in `internal/daemon/journal.go` (function): Rebuilds state from journal.jsonl: agents, pending/delivered messages, tokens, owned tabs, workspace/tab closures; tolerates torn final line and re-terminates short-written lines
- `event` in `internal/daemon/journal.go` (struct): Journal line: union of all event fields (evStarted/evRegistered/evLaunching/evPlaced/evSpawned/evReady/evStopped/evSent/evDelivered/evInboxNotified/tab/workspace lifecycle)
- `acquirePlacement` in `internal/daemon/placement.go` (function): Resolves workspace/tab selectors via Herdr wire API; creates owned tab if needed with unique temporary label; latches concurrent same-label requests
- `resolveWorkspace/resolveTab` in `internal/daemon/placement.go` (function): Match by ID first, then by label; error on ambiguous label; tab selector that looks like ID (w\d+:t\d+) fails if not found
- `recoverOwnedTabs` in `internal/daemon/placement.go` (function): Startup recovery: closes pending workspace closures; rolls back crash-left tab creation intents; closes stale owned tabs without active agents
- `stopWorkspaceOwner` in `internal/daemon/placement.go` (function): Workspace owner stop journalsWorkspaceClosing with nested placed agents and owned tabs; closes workspace; journals nested agent stops and tab closures
- `runInboxNotifier` in `internal/daemon/inbox_notify.go` (function): Background goroutine delivering inbox wake signals to orchestrator; exponential backoff on failure; coalesces concurrent messages
- `inboxNotifyEligibleLocked` in `internal/daemon/inbox_notify.go` (function): Agent is eligible for inbox wake if: not closing, not stopping, workspace not closing, inboxWake func set, inbox delivery armed

### Dependencies

- Internal `internal/protocol`: Request/Response/Agent/Message structures; operation codes (OpRegister, OpSend, etc.)
- Internal `internal/herdrwire`: Socket API to Herdr: workspace/tab/pane CRUD, window title; protocol version 16
- Internal `internal/agentcfg`: Agent definitions, profiles, configuration parsing; species pool
- Internal `internal/contextdoc`: Schema validation for managed context protocol (analyzer request/reply)
- Internal `internal/flock`: Flock directory layout, window title formatting
- Internal `internal/filebridge`: Sandbox-compatible request/response file-based RPC bridge for agents without socket access
- Internal `internal/version`: Daemon version for status/compatibility reporting
- External `net`: Unix socket listener and connection I/O
- External `sync`: Mutex protecting roster, messages, and state; channels for notifications and synchronization
- External `crypto/sha256`: Token hash for identity authentication
- External `os`: File I/O for journal, lock files, directory creation
- External `syscall`: flock(2) for exclusive ownership locking; signal 0 liveness probe

### Data Flows

- `dispatch` → `send/reply`: OpSend/OpReply → create message → journal evSent → match waiter or enqueue pending → queue inbox wake if eligible
- `send/reply` → `matchWaiter`: Find first live waiter matching message; if found, offer (acknowledge=true) or deliver (acknowledge=false)
- `deliver` → `append`: Journal evDelivered, mark messageDelivered, drop waiter, send via channel before returning to client
- `wait/receive` → `append`: On immediate match, deliver (wait, acknowledge=false) or return without mark (receive, acknowledge=true); else park waiter and block on channel
- `send/reply` → `queueInboxWakeLocked`: If shouldNotifyInboxLocked (eligible + not already notified), queue inbox notify task or update existing task backoff
- `runInboxNotifier` → `notifyInboxAgent`: Dequeue next eligible task; call inboxWake callback with orchestrator identity and pending message metadata; journal evInboxNotified on success
- `dispatch` → `spawn`: OpSpawn resolves launch config, acquires placement, starts pane in Herdr, journals evLaunching/evSpawned
- `acquirePlacement` → `herdrwire`: Call Herdr to list workspaces/tabs, resolve selectors, create tab if needed (atomic journal → create → rename → re-list for ambiguity)
- `recoverOwnedTabs` → `herdrwire`: List workspace/tab inventory; match crash-left intents by temporary label; rollback unattributable tabs; close stale owned tabs
- `replay` → `append`: On startup, replay journal events into state; if final line torn, truncate before reopening for O_APPEND
- `validateManagedContextMessageLocked` → `contextdoc`: Before send, validate analyzer request schema and reply schema + correlation for managed context protocol
- `authorizeMessageActorLocked` → `authenticateIdentityLocked`: Verify agent identity via constant-time token hash comparison; accept bare credential for launched agents

### Invariants

- Journal-first: every state-changing operation (send, deliver, spawn, stop, placement, inbox notify) is appended to journal with fsync before responding to client
- Replay idempotence: restart replays journal and rebuilds exact in-memory state (agents, pending messages, placement, closures) without loss or duplication
- Torn-line tolerance: final line of journal can be torn; replay detects and truncates before reopening; non-final malformed line is corruption (hard fail)
- Message delivery atomicity: evSent + evDelivered or evSent + pending are the only valid end states; delivery failure leaves message pending for retry
- Waiter matching: a waiter parked before evSent blocks until delivery; a sender finding a matching waiter journals delivery before releasing sender or waiter
- Species pool isolation: each flock has independent species pools; same type/pool in different flocks each hand out first species (e.g., worker-emperor)
- Placement latching: concurrent spawn requests for same workspace+tab label converge on single tab creation via latch; later requesters journal their own placement event
- Owned tab lifecycle: tabs created by Fledge are journaled as owned; crash-left tabs without active agents are cleaned up on recovery; stop cleans owned tabs
- Workspace closure durability: evWorkspaceClosing journals owner name + nested agent stops + owned tabs to close; evWorkspaceClosed marks completion
- Inbox notify eligibility: agent must be armed (inbox_notify_armed + ready journaled), running, not stopping, workspace not closing for wake delivery
- Managed context validation: forager→analyzer request rejected if schema invalid before evSent; analyzer→forager reply rejected if schema/correlation invalid before evSent
- Message actor authorization: sender authenticated via launch credential; lifecycle checked (alive/running/not-stopping/workspace-not-closing) before send allowed
- Socket path limit: unix socket path ≤ 103 bytes (darwin stricter); socket lives in XDG_RUNTIME_DIR/fledge/workspace-hash/, not workspace (NFS incompatible)

### Tests

- `internal/daemon/delivery_order_test.go`: TestSendToParkedWaiterDeliveryAppendFailureRemainsRetryable: delivery failure leaves message pending for retry; parked waiter error is propagated
- `internal/daemon/delivery_order_test.go`: TestReplyToParkedWaiterDeliveryAppendFailurePreservesCorrelation: reply failure preserves reply_to correlation and remains pending
- `internal/daemon/delivery_order_test.go`: TestWaitDeliveryAppendFailureLeavesPendingForRetry: wait delivery failure retains message pending; retry receives same message
- `internal/daemon/delivery_order_test.go`: TestDeliveryAppendFailureReplaysDurableSendAsPending: daemon restart replays undelivered send as pending; retry receives same message once
- `internal/daemon/delivery_order_test.go`: TestReceiveRequiresAckAndCanBeRetriedAfterOutputLoss: receive leaves message pending until ack; multiple receives return same message; ack is idempotent
- `internal/daemon/placement_test.go`: TestTargetedSpawnResolvesLabelsAndReusesTab: spawn resolves workspace/tab labels via Herdr; reuses existing tab without creating
- `internal/daemon/placement_test.go`: TestTargetedSpawnCreatesAndClosesOwnedTab: spawn creates owned tab with temporary label, renames to requested label, closes initial shell; stop closes owned tab
- `internal/daemon/placement_test.go`: TestExternalTabCreateRaceRollsBackOnlyFledgeTab: external creator races with Fledge rename; Fledge detects ambiguity and rolls back only its tab
- `internal/daemon/placement_test.go`: TestConcurrentTargetedSpawnsConvergeOnCreatedTab: two concurrent spawns for same label latch; single tab creation; both agents placed in same tab
- `internal/daemon/placement_test.go`: TestRecoverOwnedTabsClosesOnlyUnreferencedAuthority: startup recovery closes stale owned tabs without active agents; keeps tabs with active placements
- `internal/daemon/placement_test.go`: TestRecoverTabCreateIntentBeforeCreateLeavesExternalSameLabel: recovery with intent before create resolves intent without closing external same-label tab
- `internal/daemon/context_message_test.go`: TestManagedContextSendRejectsInvalidTrafficBeforeJournal: malformed request/reply rejected before evSent; invalid schema or missing correlation rejected
- `internal/daemon/inbox_notify_test.go`: Inbox notifier arms/disarms agents; retries with exponential backoff on failure; journals evInboxNotified for successful delivery
- `internal/daemon/isolation_test.go`: TestFlocksHaveSeparateSpeciesPools: two flocks in same workspace have independent species pools; each hands out first species per type
- `internal/daemon/isolation_test.go`: TestMessagesDoNotCrossFlocks: message sent in one flock is never visible in another; isolated journals and rosters
- `internal/daemon/isolation_test.go`: TestRestartReplaysOnlyItsOwnFlock: daemon restart replays only its own journal; roster does not leak agents from other flocks

### Files

#### internal/daemon/boundary_test.go

_text._ Test boundary conditions for request validation and error cases

#### internal/daemon/context_message_test.go

_text._ Managed context protocol validation: analyzer request/reply schema check before send; correlation validation

#### internal/daemon/daemon.go

_text._ Core state machine: Daemon struct with agent roster, messages, placement state; New/Run/RunBound/Serve/Close lifecycle; dispatch routing; send/reply/wait/inbox/ack/spawn/ready/stop handlers

#### internal/daemon/delivery_order_test.go

_text._ Message delivery durability and retry: parked waiter + eager delivery paths; delivery append failure handling; receive + ack flow

#### internal/daemon/e2e_test.go

_text._ End-to-end spawn, message exchange, agent lifecycle tests with fake Herdr integration

#### internal/daemon/forager_test.go

_text._ Context forager request/reply protocol tests

#### internal/daemon/inbox_notify.go

_text._ Inbox notifier: runInboxNotifier background goroutine; task queuing with exponential backoff; eligibility checks; integration wake callback

#### internal/daemon/inbox_notify_test.go

_text._ Inbox notifier tests: arming/disarming, retry backoff, journal atomicity, coalescing

#### internal/daemon/isolation_test.go

_text._ Multi-flock isolation: separate species pools, message isolation, independent restarts

#### internal/daemon/journal.go

_text._ Journal replay and append: event struct; replay reconstructs state from journal.jsonl; append writes with fsync; torn-line tolerance and re-termination

#### internal/daemon/journal_test.go

_text._ Journal replay tests: event ordering, message pending state, delivery markers, tab lifecycle

#### internal/daemon/placement.go

_text._ Placement logic: acquirePlacement resolves workspace/tab via Herdr; creates owned tabs with latching for concurrent requests; recovery closes crash-left tabs

#### internal/daemon/placement_test.go

_text._ Placement tests: label resolution, tab creation/reuse, external race handling, concurrent spawn convergence, recovery

## Subsystem: daemon-spawn-readiness

Daemon-driven agent lifecycle: spawn, readiness authentication via one-use tokens with SHA-256 digest validation, socket/serve handling with deadline-bounded writes, and comprehensive readiness/launch/stop coordination across socket and sandboxed file-bridge channels.

**Purpose:** Daemon agent lifecycle: spawn/readiness signaling, socket/serve handling, and related tests.

### Entry Points

- `internal/daemon/spawn.go`: Main spawn entry point: agent reservation, launch preparation, readiness orchestration, and failed rollback
- `internal/daemon/ready_signal.go`: Sandboxed readiness fallback: atomic file-based digest storage and consumption for agents whose sandbox denies Unix sockets
- `internal/daemon/serve_test.go`: Socket serve harness: listener error handling (non-temporary fails fast, temporary retries), write deadlines, shutdown sequencing

### Key Symbols

- `spawn` in `internal/daemon/spawn.go` (func): Orchestrates spawn lifecycle: resolves config, reserves name, writes atomic launch intent, launches Herdr pane, journals spawned event, polls readiness token and file-bridge signal
- `ready` in `internal/daemon/spawn.go` (func): Authenticates one-use token via SHA-256 constant-time comparison, awaits spawn event if launch still in flight, transitions agent to running, delivers queued inbox messages
- `reserve` in `internal/daemon/spawn.go` (func): Allocates name from species pool before launch (with reservedPID=-1 placeholder), serializes against concurrent spawns via d.mu
- `launch` in `internal/daemon/spawn.go` (func): Calls Herdr to create pane/workspace, reports metadata, probes shell PID; runs unlocked to avoid stalling flock on socket round-trips
- `failLaunching` in `internal/daemon/spawn.go` (func): Atomically journals stopped event and releases launch/ready latches after unrecoverable pre-launch failure
- `rollbackStarting` in `internal/daemon/spawn.go` (func): On readiness timeout: tears down pane or workspace, journals stopped, clears readiness latches
- `stop` in `internal/daemon/spawn.go` (func): Stops spawned agent: awaits launch latch, closes pane/workspace via Herdr, journals stopped, cancels parked message waiters
- `WriteReadySignal` in `internal/daemon/ready_signal.go` (func): Atomically publishes hashed token (never raw) in .fledge/flocks/<name>/.ready/<agentName> for sandboxed launch paths
- `consumeReadySignal` in `internal/daemon/ready_signal.go` (func): Validates and removes file-bridge readiness signal; tolerates legacy plain-digest format; invalid/stale signals are consumed without agent transition
- `consumeReadySignals` in `internal/daemon/ready_signal.go` (func): Resumes ready signals left while daemon was restarting; only processes signals corresponding to journaled tokens
- `launchLatch` in `internal/daemon/spawn.go` (type): Synchronization primitive: readable-once channel that spawn() parks early-ready calls on while Herdr launch resolves
- `spawnResolution` in `internal/daemon/spawn.go` (type): Resolved spawn configuration: agent name, profile, config entry, prompt, workspace (if dedicated), source path

### Dependencies

- Internal `internal/protocol`: RPC request/response envelopes, operation codes, environment variable keys
- Internal `internal/agentcfg`: Config resolution (Load, LoadDefinitions), reserved orchestrator name, session ID generation, validation
- Internal `internal/species`: Species pool management: Pick() allocates unique suffixes
- Internal `internal/flock`: Flock directory path helpers, environment variable for pane inheritance
- Internal `internal/herdrwire`: Socket API for pane/workspace ops: AgentStart, WorkspaceCreate, PaneClose, ReportMetadata, ProcessInfo
- Internal `internal/herdr`: Session state object with socket path; only imported for session binding checks
- Internal `internal/scaffold`: Workspace initialization (.fledge directory structure)
- Internal `internal/filebridge`: Fallback RPC for sandboxed agents: Submit, Await
- External `crypto/sha256`: One-use token digest for readiness authentication
- External `encoding/hex`: Hex encoding of SHA-256 digests
- External `os`: File I/O for ready signals, process info queries
- External `sync`: Mutex (d.mu) protecting roster, pending messages, readiness state
- External `time`: Readiness timeout (default 2 minutes), ticker for file-signal probes, deadline-bounded writes
- External `net`: Listen/Accept error classification (temporary vs. non-temporary), deadline on response writes

### Data Flows

- `spawn()` → `d.appendAll() [registered + launching]`: Atomic pre-launch journal: encodes type, species, config, model, instruction hash, token hash
- `spawn() to launch()` → `herdrwire.AgentStart`: Launches pane with env vars: agent name, readiness token, flock name for inheritance
- `launch()` → `d.append() [spawned event]`: Journals PID, pane ID, workspace metadata from Herdr response; triggers ready-waiter notification
- `ready()` → `readyDigest()`: One-use token to SHA-256 digest, constant-time comparison against stored hash
- `ready() to d.consumeReadySignals()` → `File-bridge fallback: .fledge/flocks/<flock>/.ready/<name>`: Validates and atomically deletes file-based digest for sandboxed agents
- `spawn() readiness loop` → `d.readyWaiters[name]`: Channel closed on ready() success; spawn() unblocks when agent reaches stateRunning
- `spawn()` → `d.rollbackStarting()`: On readiness timeout: one-way transition from stateStarting to stateStopped, pane teardown
- `stop()` → `d.cancelAgentWaitersLocked()`: Stopped agent releases any message.wait() calls parked on its behalf

### Invariants

- Spawn journals (registered, launching, spawned) are all-or-nothing atomically, so a clean journal replay never loses a launch intent
- A one-use token is hashed before storage and before spawn returns it to the agent; raw token never persists in journal or filesystem
- A stopped agent retains its identity token for message authentication but rejects all lifecycle operations (send, wait, reply, inbox)
- Early readiness (before spawned event is journaled) waits for launch completion without holding d.mu, so stop can abort launch while readiness waits
- A readiness timeout transitions only if current state is stateStarting; a concurrent stop that already transitioned is a no-op
- File-bridge ready signals older than journaled tokens are consumed and discarded; unrelated signals are never journaled
- Launched agents never create dangling Herdr resources: a spawn failure closes the pane/workspace immediately; a journal failure rolls back before returning
- Concurrent spawns serialize through d.mu at reservation time, ensuring each type-species pair is issued exactly once until its previous holder stops

### Tests

- `internal/daemon/ready_test.go`: Token validation (valid/invalid/replayed), identity authentication for send/wait/reply, stopped-agent authorization failure, message delivery ordering (before/after ready), file-bridge signal consumption
- `internal/daemon/spawn_test.go`: Spawn lifecycle: config/model/agent resolution, claude/codex/pi integrations, dedicated workspaces, launch failure rollbacks, journal atomicity, concurrent spawns, failed-launch species reuse, orchestrator readiness, bootstrap prompt injection
- `internal/daemon/reply_test.go`: Structured reply: claimed message identity validation, wrong-identity reply rejection
- `internal/daemon/serve_test.go`: Socket serve: listener error handling (temp vs non-temp), write deadline enforcement, shutdown sequencing, file-bridge drain ordering
- `internal/daemon/socket_test.go`: Socket path stability, deep workspace handling, daemon concurrency election (one winner per flock), file/journal permissions, manifest flock directory tightening

### Files

#### internal/daemon/ready_signal.go

_text._ Sandboxed readiness fallback: WriteReadySignal() atomically publishes SHA-256 digest in .fledge/flocks/<flock>/.ready/; consumeReadySignal() validates and removes; journal-replayed token map is authoritative for which signals may be consumed

#### internal/daemon/ready_test.go

_text._ Readiness tests: valid/invalid/replayed tokens, send/wait/reply authentication, stopped-agent lifecycle rejection, parked wait cancellation on stop, delivery ordering (queued-before-ready vs. sent-after-ready-wait-parks), file-bridge signal validation, spawn readiness timeout rollback, orchestrator readiness recovery

#### internal/daemon/reply_test.go

_text._ Structured reply: derives sender and causality from claimed message ID, rejects reply from non-inbound identity, enforces claim-before-reply

#### internal/daemon/serve_test.go

_text._ Serve harness: non-temporary Accept error stops Serve cleanly, temporary error retries, write deadline frees blocked writers, status reports daemon PID/version, file-bridge submit/await, shutdown sequences with active-request drain and file-bridge completion barriers

#### internal/daemon/socket_test.go

_text._ Socket lifecycle: concurrent New() elects single winner per flock, stale-socket reclaim is atomic, socket path stable across equivalent roots, deep workspace support, file/journal permissions tightened on startup

#### internal/daemon/spawn.go

_text._ Spawn orchestration: reserveSpawn allocates name (type-species), launch() calls Herdr without d.mu, failLaunching() journals stopped on pre-spawn failure, ready() validates one-use token then transitions to running, rollbackStarting() on readiness timeout closes pane/workspace, stop() awaits launch latch then closes pane, file-bridge ready signal polling (50ms tick), orchestrator skips role-prompt delivery

#### internal/daemon/spawn_test.go

_text._ Spawn tests: config/model/profile/agent resolution, integration routing (claude/codex/pi), dedicated workspace creation/cleanup, launch failure rollback cascades (placement, journal), concurrent spawns get distinct names, failed launch releases slug, orphaned agent frees species, file-bridge spawn/message dispatching, message delivery doesn't depend on post-launch Herdr, readiness timeout and species reuse, orchestrator role injection, pi/claude role injection, bootstrap prompt, structured reply identity, pending message delivery exactly-once

#### internal/daemon/watch_test.go

_text._ Session watch: probes liveness, retries title-set until foreground client attaches, stops on daemon close, unbound daemon sends no Herdr requests

## Subsystem: docs

Design and decision documentation for Fledge Stage 0 (completed experiment: zero-inference Go orchestrator for Herdr/Pi/Claude-Code multi-agent coding stack). Four referential documents distilled from research snapshots (2026-07-17), three ADR logs recording architecture decisions and experiment findings, and comprehensive integration contracts for each surface. Core invariants: Go CLI as state authority (SQLite event log); Herdr as UI/pane plumbing (socket API); Pi as fully programmable lifecycle-aware GPT harness; Claude Code driven via hooks and screen-manifest detection, never as a lifecycle authority. Two hard unknowns (EXP1: authority override; EXP2: interactive input) de-risked and resolved via supervised experiments; one rate-limit ceiling (EXP3) measured but awaiting operator-driven re-run. All Stage 0 scope complete; Stage 1 (relay core) deliberately deferred to a new commissioned session.

**Purpose:** Design and decision documentation: architecture notes, ADR-style decisions, experiment logs, integration contracts, and reference snapshots.

### Entry Points

- `docs/ARCHITECTURE.md`: Zero-inference architectural invariants: Go CLI state authority, Herdr as plumbing, Pi/Claude lifecycle authority split, data/event flow paths (Figure 1), staged roadmap Stages 0–4
- `docs/DECISIONS.md`: ADR log (newest first): ADR-017 (Herdr protocol v16 reconciliation, ADR-015/016 superseded), ADR-014 (no concurrent-pane cap, reactive rate-limit handling), ADR-013 (interactive pane submission reliable 3/3), ADR-012 (screen detection precedence over custom report), ADR-010 (git history not design input), ADR-001 (Stage 0 scope only)
- `docs/EXPERIMENTS.md`: Experiment harness protocols and results: EXP1 (authority override — run 2026-07-18, screen detection unchanged despite custom report), EXP2 (interactive input — 3/3 gated submits reliable, 2026-07-18), EXP3 (rate limits — harnesses built, n=3 sustained load showed no throttle, operator-executed only, 2026-07-18)
- `docs/INTEGRATION-CONTRACTS.md`: Three integration surfaces pinned and verified: Herdr v0.7.4/protocol 16 (socket API, authority arbitration, pane/agent lifecycle), Pi v0.80.x (RPC mode, Herdr extension integration, lifecycle authority), Claude Code v2.1.212 (hooks, headless -p, interactive input caveats, pooled rate limits)
- `docs/handoff-stage0.md`: Stage 0 commission (historical): mission (skeleton + docs + three harnesses + type generation), ground rules (git history not design input, reference docs immutable, re-verify version claims), repo layout, definition of done — all completed except EXP3 operator execution
- `docs/reference/ai-sdlc-scan.md`: Research snapshot 2026-07-17: TL;DR (Sonnet 5 + GPT-5.6 tokenizer shifts, runaway-cost governors, Herdr active + maintained), key findings (model layer biggest lever, multi-agent fan-out is first-class problem, encrypted Codex delegation audit risk, cross-vendor review consensus), version-bump table, recommendations, caveats
- `docs/reference/integration-surfaces.md`: Deep research (2026-07-17): TL;DR (Herdr socket API complete orchestration surface, Pi most programmable, Claude constrained by rate limits and interactive input caveat), capability matrix, three workstreams (Herdr authority split, Pi programmatic driving, Claude Code hooks/headless), prior art (file-lock coordinators, worktree isolation substrate), Go building blocks, architecture fit mapping, staged recommendations

### Key Symbols

- `ADR-017` in `docs/DECISIONS.md` (decision): Herdr protocol v16 reconciliation (2026-07-17): live client vs. v15 research target, wire/shape drift corrected, ADR-015/016 superseded. Key findings: params mandatory, one-request-per-connection, results wrapped by kind, field/enum drift (argv vs. command, recent_unwrapped vs. recent-unwrapped), screen_detection_skipped boolean. Tests: snapshot, agent.start, pane.read, agent.get pass; explain payload + streaming not yet verified.
- `ADR-012` in `docs/DECISIONS.md` (decision): EXP1 outcome (2026-07-17 resolved): authority override does NOT suppress screen detection on Claude panes. Verdict: native detection takes precedence, custom report accepted but ineffective on Claude. Conclusion: metadata-only safe; rule may be relaxed but state remains overridden by native detection. Side-finding: clear_agent_authority vs. release_agent semantics need Stage-1 verification.
- `ADR-013` in `docs/DECISIONS.md` (decision): EXP2 outcome (2026-07-17 resolved): pane.send_input {text, keys:["enter"]} reliably submits (3/3 gated sends). Verdict: Claude workers run in visible panes, no fallback to -p/stream-json needed. Bare \r does not submit (Ink limitation); explicit Enter keypress required.
- `ADR-014` in `docs/DECISIONS.md` (decision): EXP3 outcome (2026-07-18 operator-reported): no fixed concurrent-pane cap. Verdict: rate limits handled reactively (StopFailure/rate_limit hook authoritative) rather than pre-limited. n=3 sustained load showed no throttle; ceiling above practical needs. Revisit if sustained throttling observed in real use.
- `EXP1 Result` in `docs/EXPERIMENTS.md` (experiment): Run 2026-07-18 02:46 UTC: spawned Claude pane, confirmed baseline screen_detection_skipped=false + blocked via screen manifest, issued pane.report_agent {custom:test, working, seq:1}, pane held screen_detection_skipped=false (unchanged), operator confirmed sidebar still blocked. Verdict: custom report does NOT suppress native screen detection on Claude. Control (shell pane): same call flipped agent from null→probe, status unknown→working, proving authority seizure on non-detected panes.
- `EXP2 Result` in `docs/EXPERIMENTS.md` (experiment): Run 2026-07-18 02:48 UTC: spawned Claude pane, three gated rounds of pane.send_input {text, keys:["enter"]}, operator confirmed each submitted (3/3 success). Pane tail samples show prompt echoed and agent responses ('● pong'). Verdict: reliable submission; interactive Claude workers viable.
- `EXP3 Result` in `docs/EXPERIMENTS.md` (experiment): Three runs (one invalid, two valid). First (2026-07-18 03:02): false positive (repo text matched throttle regex, no actual StopFailure hook). n=2 (2026-07-18 ~03:14, manual): no throttle, real work in neutral cwd (lru.go+test, sudoku.py+test), lower bound only (finite tasks, not sustained). n=3 --sustain (2026-07-18): no throttle under sustained load, operator aborted early. Verdict: no practical cap needed; ≥3 concurrent safe for fledge's needs. Revisit if sustained throttling observed in production.
- `Herdr Surface` in `docs/INTEGRATION-CONTRACTS.md` (interface): v0.7.4 protocol 16 (verified live 2026-07-17). Methods: agent.start, pane.split/send_input/read/report_agent/clear_agent_authority/release_agent, session.snapshot, events.subscribe. Authority arbitration: one pane, one authority; custom report overrides built-in detection; native detection suppressed when authority seized. Caveat: clear vs. release semantics undocumented in live docs.
- `Pi Surface` in `docs/INTEGRATION-CONTRACTS.md` (interface): v0.80.x (not yet live-verified). RPC mode stdin/stdout JSONL (LF-framed); commands: prompt/steer/follow_up/abort/session ops/tools/bash; events: agent_start/settled/turn/message/tool; Herdr integration v2: lifecycle authority (idle/working/blocked), native session restore. Extension API: tool registration, context rewrite, UI redraw.
- `Claude Code Surface` in `docs/INTEGRATION-CONTRACTS.md` (interface): v2.1.212 (verified 2026-07-17). Hooks: Stop/Notification/PreToolUse/PermissionRequest (30+ events). Headless: -p mode, --output-format json|stream-json. Session: --session-id (first), --resume <id>, must resume from same cwd. Interactive: send_text + real Enter keypress (Ink limitation). Rate limits: 5-hour rolling + weekly cap, pooled across all Claude sessions + chat, shared account.

### Dependencies

- Internal `docs/ARCHITECTURE.md`: References invariants from §3 and data-flow Figure 1, carried into every design doc
- Internal `docs/DECISIONS.md`: Cross-references: ADR-017 (wire facts), ADR-012/013/014 (experiment thresholds), ADR-004 (Claude metadata-only rule), ADR-003/ADR-002 (authority-split invariants)
- Internal `docs/EXPERIMENTS.md`: Results sections (EXP1/EXP2/EXP3) populated from supervised runs; flip thresholds mirror DECISIONS.md entries
- Internal `docs/INTEGRATION-CONTRACTS.md`: Three sections (Herdr/Pi/Claude) distilled from reference/* snapshots; Last verified lines updated per experimental findings (ADR-017, EXP1, EXP2)
- Internal `docs/handoff-stage0.md`: Commission document for Stage 0; defines scope, ground rules, definition of done — all addressed in completed docs
- Internal `docs/reference/ai-sdlc-scan.md`: Research input (immutable): model releases (Sonnet 5, GPT-5.6), harness releases (Claude Code, Pi, Herdr in-window), cost governors, cross-vendor review pattern. Cited by INTEGRATION-CONTRACTS recommendations.
- Internal `docs/reference/integration-surfaces.md`: Research input (immutable): 150-page deep integration study (Herdr/Pi/Claude), prior-art survey, Go building blocks, staged roadmap. Primary source for authority-split invariants, capability matrix, workstreams, EXP recommendations.
- External `Herdr v0.7.4`: Terminal multiplexer / pane bus; socket API (protocol v16, verified 2026-07-17); pre-1.0, solo-maintained, AGPL-3.0
- External `Pi v0.80.x`: Programmable GPT harness; RPC mode (JSONL, LF-framed), extension SDK, Herdr integration v2; open-core, MIT + Fair Source
- External `Claude Code v2.1.212`: Anthropic sub; 30+ hook events, headless -p mode, interactive panes; v2.1.212 has background /fork, subagent caps (200), rate-limit pooling
- External `Claude Sonnet 5`: Default model (June 30, 2026); 1M context, adaptive thinking on-by-default, new tokenizer (~1.28–1.4x tokens for English/Python vs. prior baseline)
- External `GPT-5.6 (Sol/Terra/Luna)`: OpenAI model family (GA July 9, 2026); Sol/Terra use encrypted MultiAgentV2 (local audit visibility lost); Luna stays MultiAgentV1
- External `Omnigent Polly`: Reference architecture (open-sourced June 13): multi-agent orchestrator, cross-vendor review (different vendor writes vs. reviews), git worktree isolation per agent

### Data Flows

- `Go Orchestrator (CLI)` → `Herdr server (socket)`: (1) Commands: agent.start, pane.split, pane.send_input, report_metadata (display only)
- `Herdr server (socket)` → `Go Orchestrator (CLI)`: (3) Event subscriptions: pane.agent_status_changed, pane.output_matched, worktree.*, layout.updated, session.snapshot bootstrap
- `Agent panes (Claude Code hooks, Pi RPC)` → `Go Orchestrator (CLI)`: (2) Callbacks: Claude hooks POST event JSON to relay HTTP endpoint; Pi RPC events read from subprocess stream
- `Go Orchestrator (CLI)` → `SQLite event log`: Deterministic FSM routing, task state, namespace locks, ownership declared before dispatch
- `SQLite event log` → `Go Orchestrator (CLI)`: Source of truth for orchestration state; survives Herdr restarts (Herdr does not restore token metadata across restart)
- `Herdr session.snapshot + events.subscribe` → `Go Orchestrator in-memory cache`: Local mirror of pane/agent/workspace state; recoverable but not authoritative (Go CLI's SQLite log is truth)

### Invariants

- The Go CLI is the state authority. Herdr events and agent hook/RPC events are input signals only. Herdr's own store is never relied on for durable orchestration state (Herdr does not restore token metadata across server restart).
- Zero-inference in the orchestrator: CLI issues socket commands, consumes events, advances FSM, writes event log; all LLM inference happens inside visible, interactable panes (Pi/Claude/Codex), never in the orchestrator.
- Pi panes: Herdr's native Pi lifecycle authority (bundled extension v2) is trusted as the state source (idle/working/blocked). CLI reads this as input signal; does not report custom state onto Pi panes.
- Claude panes: Claude Code is intentionally not a lifecycle authority in Herdr. Blocked/working come from screen-manifest detection. CLI uses pane.report_metadata (display-only) for sidebar updates; does NOT seize authority with pane.report_agent --source custom:* on Claude panes, preserving built-in blocked detection for permission prompts and human-escalation routing.
- Herdr authority model: each pane has exactly one status authority. Custom socket report (pane.report_agent --source custom:*) overrides built-in detection and suppresses competing sources. Must be paired with pane.clear_agent_authority or pane.release_agent on exit to restore fallback.
- Git history is not design input. Current HEAD is authoritative; prior iterations in git history are not excavated or resurrected for design purposes (ADR-010, ground rule 1).
- Reference docs (docs/reference/*) are immutable research snapshots (2026-07-17). Version-specific claims must be re-verified against live binaries at build time. Corrections and re-verified facts go in distilled docs or DECISIONS.md.
- Stage 0 scope only: skeleton, distilled docs, three experiment harnesses, Herdr type generation. Stage 1 (relay core: SQLite event log, FSM/workflow engine, Claude hook HTTP endpoint, Pi RPC subprocess manager) deliberately deferred to a new commissioned session.
- EXP1 finding (ADR-012): custom pane.report_agent does NOT suppress screen detection on Claude panes. Native detection takes precedence. Metadata-only rule safe; can be relaxed but state remains overridden.
- EXP2 finding (ADR-013): pane.send_input {text, keys:["enter"]} reliably submits (3/3 verified). Claude workers run in visible panes; no fallback to -p/stream-json needed.
- EXP3 finding (ADR-014): no fixed concurrent-pane cap. Rate limits handled reactively (StopFailure/rate_limit hook). n=3 sustained load showed no throttle; no pre-limit warranted.
- Herdr protocol v16 (not v15 as researched). Params mandatory on every method. One request per connection; no multiplexing. Results wrapped by kind. Wire shapes reconciled live (ADR-017).
- Claude Code subscriptions: 5-hour rolling window + weekly cap, pooled across all Claude sessions + Claude chat on one account. Parallel subagent fan-out is documented cause of premature exhaustion. Rate limits are primary scaling risk; reactive backoff required (StopFailure hook).

### Tests

- `docs/EXPERIMENTS.md`: EXP1 (authority override) — supervised run 2026-07-18: spawned Claude pane, verified baseline screen_detection_skipped=false, issued pane.report_agent {custom:test, working}, re-checked explain payload (unchanged), operator confirmed sidebar still blocked, cleared authority, confirmed restoration. Verdict: native detection precedence confirmed.
- `docs/EXPERIMENTS.md`: EXP2 (interactive input) — supervised run 2026-07-18: spawned Claude pane, three gated pane.send_input {text, keys:["enter"]} rounds, operator confirmed each submitted (3/3 success). Verdict: reliable submission confirmed.
- `docs/EXPERIMENTS.md`: EXP3 (rate limits) — operator-run n=2 (2026-07-18 ~03:14): no throttle detected, real work completed (lru.go, sudoku.py with tests) in neutral cwd. n=3 --sustain (2026-07-18): no throttle under sustained re-fed load, operator aborted early. Verdict: ≥3 concurrent safe; no ceiling found.
- `docs/INTEGRATION-CONTRACTS.md`: Herdr v0.7.4 live verification (2026-07-17): scripts/gen-herdr-types.sh run against live binary, schema dump committed, herdrclient types reconciled, ADR-017 wire-shape discrepancies documented. Verified methods: session.snapshot, agent.start, pane.read, agent.get, pane.close, multi-call reuse. Not yet verified: agent.explain success payload (needs Herdr-detected Claude pane), events.subscribe streaming.

### Files

#### docs/ARCHITECTURE.md

_text._ Zero-inference invariants (Go CLI state authority, Herdr plumbing, Pi/Claude authority split), zero-inference rule (what CLI may/must-not do), data/event flow Figure 1 (three paths: commands, callbacks, events), staged roadmap Stages 0–4 (Stage 0 marked in progress, Stage 1 relay core deferred).

#### docs/DECISIONS.md

_text._ ADR log newest-first. Accepted: ADR-010 (git history not design), ADR-007 (generated types committed), ADR-006 (reference docs immutable), ADR-004 (Claude metadata-only pending EXP1, EXP1 outcome resolved), ADR-003 (Pi lifecycle trusted), ADR-002 (Go CLI state authority), ADR-001 (Stage 0 scope). Open→Resolved: ADR-017 (Herdr v16 reconciliation, ADR-015/016 superseded), ADR-014 (no concurrent cap, reactive rate limits), ADR-013 (interactive input reliable), ADR-012 (screen detection precedence).

#### docs/EXPERIMENTS.md

_text._ EXP1 (authority override): procedure, preconditions, flip threshold. Results: 2026-07-18 run, screen_detection_skipped remained false after custom report, operator confirmed sidebar blocked throughout. Verdict: native detection precedence, metadata-only safe. EXP2 (interactive input): procedure, flip threshold. Results: 2026-07-18 run, 3/3 gated send_input submissions succeeded. Verdict: reliable, Claude workers in visible panes viable. EXP3 (rate limits): procedure, flip threshold. Results: n=2 no throttle (lower bound), n=3 sustained no throttle (upper bound). Verdict: reactive rate-limit handling, no pre-cap needed.

#### docs/INTEGRATION-CONTRACTS.md

_text._ Herdr v0.7.4 protocol 16 (Last verified 2026-07-17, ADR-017): socket API, methods, authority arbitration, pane lifecycle, sequencing, protocol versioning, soft spots (clear/release semantics undocumented, 32-source cap scope unknown). Pi v0.80.x (not live-verified): RPC mode, commands, events, Herdr integration v2 lifecycle authority, version caveats. Claude Code v2.1.212 (verified 2026-07-17): hooks, headless -p, interactive input caveat (real Enter), rate limits (pooled, parallel-hostile), version/stability caveats.

#### docs/handoff-stage0.md

_text._ Stage 0 commission document (historical): mission (skeleton + docs + harnesses + type gen, Stage 1 deferred), ground rules (current HEAD authoritative, reference docs immutable, re-verify claims, never run EXP3, pause before live sessions, zero-inference applies to harnesses), authority-split invariants, repo layout, five referential docs per §5, three experiments per §6, Herdr types script per §7, definition of done checklist.

#### docs/reference/ai-sdlc-scan.md

_text._ Research snapshot 2026-06-15 to 2026-07-17. Key findings: Sonnet 5 (June 30, new tokenizer, 1M context, adaptive thinking on-by-default); GPT-5.6 (July 9, Sol/Terra/Luna, `ultra` multi-agent mode, encryption on Sol/Terra); runaway-delegation governors (Claude Code subagent caps 200, OpenCode nested subagents off, Codex token budgets); Herdr v0.7.x active (socket API, Pi detection); Omnigent Polly reference architecture; Codex encrypted delegation audit risk (July 14). Version-bump table. Recommendations: immediately bump + re-baseline Sonnet 5 tokens; adopt runaway-cost governors; build cross-vendor review; drive Herdr socket API; avoid Sol/Terra for auditability; September spend review (intro pricing expires Aug 31). Caveats: pre-1.0 surfaces, vendor-reported benchmarks, secondary sourcing on practitioner patterns.

#### docs/reference/integration-surfaces.md

_text._ Deep integration research (150+ pages, 2026-07-17): TL;DR (Herdr complete surface, Pi most programmable, Claude constrained by rate limits and interactive caveat). Workstream 1 — Herdr: transport (NDJSON socket, one-request-per-conn), bootstrap (snapshot+subscribe), lifecycle (agent.start/pane.*/events), custom state authority (seizes on custom report; overrides built-in detection), giving authority back (clear/release). Workstream 2 — Pi: RPC mode + SDK + extensions, Herdr lifecycle integration v2 (native authority), version caveats. Workstream 3 — Claude Code: hooks (30+ events), headless vs. interactive (pane.send_input caveat), session threading (cwd-bound resume), rate limits (pooled, parallel-hostile). Workstream 4 — Prior art + Go building blocks (gofrs/flock, looplab/fsm, go-workflows, SQLite). Architecture fit: authority split mapping, planner/reviewer loop, parallel worktree isolation, zero-inference data flow Figure 1, staged recommendations (Stages 0–4). Caveats: pre-1.0, fast-moving, undocumented Herdr behavior, rate limits unpublished, secondary sourcing on orchestrators.

## Subsystem: herdr-integration

Three-layer integration with Herdr pane-bus: herdr package shells out to the CLI for session lifecycle (start/stop/delete); herdrwire speaks the socket API directly for pane control and workspace management; filebridge provides a fallback RPC transport for sandboxed clients via atomic file exchanges.

**Purpose:** Herdr pane-bus integration: CLI shell-out (herdr), socket wire protocol (herdrwire), and sandboxed file bridge.

### Entry Points

- `internal/herdr/herdr.go`: Session lifecycle: List(), Find(), Ensure(), Recreate(), Remove(), Stop(), Delete(). Ensures Herdr server is running and ready before pane operations.
- `internal/herdrwire/herdrwire.go`: Pane and workspace control: AgentStart(), AgentStartInWorkspace(), WorkspaceCreate(), TabCreate(), SendInput(), ProcessInfo(), PaneFocus(), PaneSwap(), AgentAlive(), WindowTitleSet(). One-shot request/response over unix socket per call.
- `internal/filebridge/filebridge.go`: Sandboxed fallback RPC: Submit(), Take(), Await(), Respond(), Cleanup(). Atomic file-based request/response when unix sockets forbidden.

### Key Symbols

- `Session` in `internal/herdr/herdr.go` (struct): Herdr session metadata from CLI list output: Name, Running, Default, SocketPath
- `Ensure` in `internal/herdr/herdr.go` (func): Idempotent session resolution: reuses live session, starts new if missing, returns whether it started
- `Recreate` in `internal/herdr/herdr.go` (func): Replaces a session record with fresh headless server, used for clean recovery on managed sessions
- `Up` in `internal/herdr/herdr.go` (func): Probes session socket liveness; socket removal is only signal for session end
- `Call` in `internal/herdrwire/herdrwire.go` (func): Core transport: dials socket, sends one request line, reads one response, decodes result or *wireError
- `StartedAgent` in `internal/herdrwire/herdrwire.go` (struct): Result of agent.start: PaneID and TerminalID for pane identification
- `CreatedWorkspace` in `internal/herdrwire/herdrwire.go` (struct): Result of workspace.create: WorkspaceID, TabID, RootPaneID all needed for rollback
- `AgentAlive` in `internal/herdrwire/herdrwire.go` (func): Liveness check: distinguishes wireError (pane gone) from transport failure; only liveness-reportable
- `AgentStatus` in `internal/herdrwire/herdrwire.go` (func): Screen-detected status from agent.get: 'unknown' on pane creation, transitions to TUI status as integration runs
- `Pending` in `internal/filebridge/filebridge.go` (struct): Claimed request with ID awaiting dispatch
- `pending` in `internal/filebridge/filebridge.go` (struct): Internal: request + PID for client liveness probing via signal 0
- `Await` in `internal/filebridge/filebridge.go` (func): Client waits for acceptance then response; probes daemon aliveness during wait
- `Respond` in `internal/filebridge/filebridge.go` (func): Daemon responds atomically; sweeps orphans when client liveness fails

### Dependencies

- Internal `internal/flock`: Directory layout: filebridge builds .rpc paths under flock root
- Internal `internal/protocol`: Shared request/response enums (OpSpawn, OpList, OpSend, OpWait) and types for filebridge marshaling
- External `encoding/json`: Marshal/unmarshal request/response and Session metadata
- External `net`: Unix socket dial, listen, connection lifecycle for herdrwire
- External `os/exec`: Shell out to herdr CLI for session lifecycle (list, start, stop, delete)
- External `syscall`: Setsid for session detach; signal-0 probe for filebridge PID liveness

### Data Flows

- `herdr.Ensure()` → `herdrwire.Call()`: Session resolved and socket obtained; herdrwire operations target that socket
- `herdrwire.AgentStart()` → `internal/protocol`: Response unwrapped to StartedAgent; pane IDs used for subsequent pane ops
- `herdrwire.WorkspaceCreate()` → `herdrwire.TabRename(), herdrwire.TabClose()`: CreatedWorkspace IDs enable label-and-setup or rollback workflow
- `filebridge.Submit()` → `filebridge.Take()`: Client atomically writes inbox request with PID; daemon Take() claims and moves to accepted/
- `filebridge.Respond()` → `filebridge.Awaiting()`: Daemon checks client liveness before writing response; orphan sweep on failure
- `herdr.start()` → `herdr.Up()`: Daemon startup loop: polls socket until it appears or deadline expires
- `herdrwire.nextID atomic counter` → `herdrwire.Call()`: Process-wide request ID generation, unique per connection

### Invariants

- Session socket is the ONLY signal of session lifecycle: liveness probed by dial, absence means session ended; protocol 16 emits no session events
- herdrwire.Call() is one-shot per connection: dial fresh for every operation, close after response
- filebridge Await()/Respond() are deterministic state machines: accepted marker must appear before response, client removes marker only after reading
- filebridge PID liveness probe (signal 0) is mandatory for Awaiting(); killed client cleanup is signal-0 only
- Workspace/Tab/Pane IDs returned by create operations are stable and usable immediately for placement or rollback
- ReportMetadata() never seizes agent authority: screen detection native to pane wins (EXP1)
- SendInput with pressEnter=true sends keys:["enter"] which submits in TUI; bare \r does not (EXP2)

### Tests

- `internal/herdr/herdr_test.go`: Session lifecycle: List/Find/Ensure/Recreate/Remove/Stop/Delete with stubbed herdr CLI and unix socket probes
- `internal/herdrwire/herdrwire_test.go`: Wire protocol: one-shot request/response envelope, params unwrapping, error handling, socket timeout, workspace/tab/pane placement
- `internal/filebridge/filebridge_test.go`: File bridge atomicity: submit/take/respond lifecycle, orphan sweep, PID validation, unsafe ID rejection, daemon-stop abort, concurrent submit/take

### Files

#### internal/filebridge/filebridge.go

_text._ Sandboxed fallback RPC: atomic file transport inbox/accepted/responses dirs. Submit() publishes with PID, Take() claims and checks liveness, Await() polls with daemon-stop abort, Respond() sweeps orphans. ID validation guards path traversal.

#### internal/filebridge/filebridge_test.go

_text._ File bridge lifecycle, orphan sweep on client abandonment, PID<=0 rejection for upgrades, unsafe ID rejection (traversal/separators/non-hex), concurrent submit/take, daemon-stop abort in await.

#### internal/herdr/herdr.go

_text._ Session resolution via herdr CLI. Exports Session struct from list output, Ensure() for idempotent startup, Recreate() for recovery, Remove() for cleanup with stop/delete sequencing and liveness polling.

#### internal/herdr/herdr_test.go

_text._ Stubbed-CLI tests for session list/find/ensure/recreate/remove/stop/delete. Tests directory/environment passing on start, polling retry logic, error wrapping.

#### internal/herdrwire/herdrwire.go

_text._ Herdr socket API: Call() core one-shot transport, AgentStart/WorkspaceCreate/TabCreate for placement, SendInput/ProcessInfo for pane interaction, AgentAlive/AgentStatus for liveness and screen detection, PaneFocus/PaneSwap/PaneClose for layout.

#### internal/herdrwire/herdrwire_test.go

_text._ Fake Herdr server tests for request envelope, one-shot per connection, agent/workspace/tab/pane params validation, wireError vs transport error, timeout on silent socket, concurrent ID uniqueness.

## Subsystem: root-meta

Fledge is a zero-inference Go orchestrator for multi-agent coding sessions. It manages isolated flocks (orchestration sessions), spawns agents into Herdr panes, carries messages between them, and maintains an authoritative append-only journal. The root-meta assignment covers repository infrastructure: CI/CD workflows, build/install scripts, module definitions, and high-level documentation.

**Purpose:** Repo-level metadata: CI workflows, top-level docs, license, module files, and build/install/release scripts.

### Entry Points

- `scripts/build.sh`: Build entry point: compiles cmd/fledge to bin/fledge; repo root discovery via dirname and go build
- `scripts/install.sh`: Installation entry point: builds with -tags dev (version suffix -dev) and installs to GOBIN or GOPATH/bin or override BINDIR
- `README.md`: User-facing entry point: full command reference, quickstart (fledge init / start), concepts, configuration (.fledge layout, portable definitions), and development guide

### Key Symbols

- `fledge` in `README.md` (project): Zero-inference orchestrator for multi-agent coding. Launches agents into visible Herdr panes, maintains append-only journal as authoritative state, performs no LLM inference
- `Flock` in `CLAUDE.md` (concept): One isolated orchestration session: own daemon, agent roster, journal, unix socket, and Herdr session. State in .fledge/flocks/<name>/
- `Agent` in `CLAUDE.md` (concept): Named process tracked by daemon: <type>-<species> where species drawn from fixed penguin-slug pool. Self-registered or spawned. Exception: fledge-orchestrator (no species suffix)
- `Journal` in `CLAUDE.md` (concept): Append-only journal.jsonl: written before client ack. Daemon rebuilds state by replay. Core invariant: nothing durable unless journaled
- `Integration` in `CLAUDE.md` (concept): How agents launch and are talked to: claude, codex, pi. All pane-hosted in Herdr. Direct-message delivery via durable mailbox
- `check-release-version.sh` in `scripts/check-release-version.sh` (script): Validates semantic version in internal/version/VERSION against git tags; gates both PR checks and release workflow

### Dependencies

- Internal `internal/daemon`: Per-flock server: spawning, journal, session watch, readiness validation
- Internal `internal/protocol`: Request/response types for daemon socket contract
- Internal `internal/client`: Daemon socket client
- Internal `internal/agentcfg`: .agent.md parsing, index sync, portable definition routing
- Internal `internal/catalog`: Model discovery from installed integrations (claude, pi, codex)
- Internal `internal/herdr`: Herdr CLI wrapper for session lifecycle (no socket API)
- Internal `internal/herdrwire`: Direct Herdr socket API speaking (protocol 16)
- Internal `internal/version`: Embedded version authority: internal/version/VERSION single source of truth
- Internal `internal/workspace`: Root discovery: git-style walk to .fledge/, then EvalSymlinks
- Internal `internal/scaffold`: Creates .fledge/ tree on init
- Internal `internal/ignore`: .fledgeignore matching (gitignore syntax + #include)
- Internal `internal/scan`: Workspace file scanning with ignore rules
- External `github.com/goccy/go-yaml`: YAML frontmatter parsing for .agent.md portable definitions
- External `herdr`: CLI on PATH; protocol 16 verified at 0.7.4; session lifecycle and pane management
- External `claude`: Claude Code CLI on PATH; optional, only needed for claude integration spawns
- External `pi`: Pi CLI on PATH; optional, only needed for pi integration spawns (supports openai-codex, opencode, opencode-go providers)
- External `codex`: Codex CLI on PATH; optional, only needed for codex integration spawns

### Data Flows

- `.github/workflows/pull-request.yml` → `cmd/fledge, internal/version/VERSION`: PR validation: lint, vet, test coverage, and version check on every PR open/sync/reopen
- `.github/workflows/release.yml` → `scripts/check-release-version.sh, cmd/fledge, internal/version/VERSION`: Release gate: version validation, build (amd64/arm64), archive creation, GitHub release with generated notes
- `go.mod, go.sum` → `scripts/build.sh, scripts/install.sh`: Go module versioning: go build uses pinned goccy/go-yaml v1.19.2
- `AGENTS.md` → `internal/agentcfg, internal/scaffold`: Repository guidelines for portable .agent.md definitions, namespacing, and profile layout
- `.gitignore` → `build artifacts, .fledge state`: Excludes: *.exe, bin/, vendor/, .fledge/locks/, .fledge/flocks/, generated agents.json and catalog.json
- `README.md` → `end users, developers`: Authoritative command reference, quickstart, concepts, configuration, and development guidelines

### Invariants

- Fledge's append-only journal is authoritative state. Herdr and agent events are input signals, never source of truth.
- Zero inference in the orchestrator: no LLM calls in fledge code itself. All inference happens in visible agent processes.
- Internal version/VERSION is single source of truth; bumped in every releasable PR; gates both PR checks and release workflow.
- CI/CD workflow: PR runs lint/test/build independently; merge into main tags and publishes GitHub Release with amd64/arm64 archives.
- Go 1.26+ required. Unix-only: unix sockets, setsid, signal-0 liveness probes. No third-party deps except goccy/go-yaml.
- Portable .agent.md files are user-authored and tracked; generated JSON indexes and catalog are gitignored per-machine.
- .fledgeignore: gitignore syntax + #include directive; defaults exclude dot-dirs except .github/ and .fledge/agents/user/
- Flock naming: deterministic hash keyed on workspace path; socket under $XDG_RUNTIME_DIR (108-byte sun_path limit, NFS-safe).
- Agent species: fixed penguin-slug pool, one per type, exception fledge-orchestrator (no species, no suffix).
- Herdr protocol pinned to 16 (0.7.4); live-verified quirks documented inline; fast-moving pre-1.0 surface.

### Tests

_None._

### Files

#### .github/workflows/pull-request.yml

_text._ PR validation workflow: lint (gofmt/vet), test with coverage, build (amd64/arm64), version check. Runs on open/sync/reopen.

#### .github/workflows/release.yml

_text._ Release automation: gate (version check), lint/test if release, build amd64/arm64, create/resume GitHub Release with archives (fledge_<version>_linux_*.tar.gz + SHA256SUMS).

#### .gitignore

_text._ Excludes binaries (*.exe, *.so, *.dylib), build outputs (bin/), Go artifacts, vendor/, .fledge/{locks,flocks,agents/fledge,agents/fledge-agents.json,agents/catalog.json}, and .env

#### AGENTS.md

_text._ Repository guidelines for portable agent definitions: single user so breaking changes ok, structure under internal/, colocation tests, hand-written parsing, reserved fledge-* namespace, gitignore portable .agent.md but generated indexes.

#### CLAUDE.md

_text._ Project-level instructions for Claude Code: architecture (CLI authority, zero inference), flock/daemon/journal concepts, integration shapes, portable definition schema, context scanning, and development commands/conventions.

#### LICENSE

_text._ GNU Affero General Public License v3.0

#### README.md

_text._ Authoritative user-facing documentation: requirements, install, quickstart, concepts (Flock/Agent/Integration/Journal), full command surface with flags, configuration (.fledge layout, portable definitions, .fledgeignore, forager workflow), development (test/build/layout), and license.

#### go.mod

_text._ Module declaration: github.com/Harrison-Blair/fledge, Go 1.26, single dependency: github.com/goccy/go-yaml v1.19.2

#### go.sum

_text._ Pinned hash for goccy/go-yaml v1.19.2

#### scripts/build.sh

_text._ Builds cmd/fledge to bin/fledge; repo root discovery and go build -o invocation

#### scripts/check-release-version.sh

_text._ Semantic version validator: reads internal/version/VERSION, gates new PR and release modes; ensures MAJOR.MINOR.PATCH format and monotonic tag progression

#### scripts/install.sh

_text._ Dev install script: builds with -tags dev (version suffix -dev), installs to GOBIN/GOPATH/bin, warns if not on PATH or shadowed

#### scripts/reinstall.sh

_text._ Convenience wrapper: calls build.sh then install.sh; accepts no arguments, BINDIR override only

## Subsystem: support-libs

Foundational infrastructure libraries providing wire protocol definitions, workspace/flock discovery and identity, file scanning with ignore-pattern matching (gitignore semantics), species-based agent naming, versioning with build-tag suffixes, and socket-based client communication with workspace-local fallback for sandboxed access.

**Purpose:** Foundational internal support libraries: wire protocol types, socket client, workspace root discovery, ignore-pattern matching, species name pool, versioning, and flock config.

### Entry Points

- `internal/protocol/protocol.go`: Newline-delimited JSON wire format for CLI-daemon communication over Unix socket; defines Request/Response structs and operation opcodes
- `internal/client/client.go`: Client library for exchanging protocol requests with the daemon; uses Unix socket when available, falls back to workspace-local file bridge for sandboxed agents
- `internal/workspace/workspace.go`: Workspace root discovery (git-style walk to .fledge/), canonicalization via EvalSymlinks, hash identity, and human-readable slug generation
- `internal/flock/flock.go`: Flock naming, validation, enumeration, minting, session name derivation with workspace scoping, and environment-based flock selection

### Key Symbols

- `Request` in `internal/protocol/protocol.go` (type): Client request: operation, agent identity, message routing, spawn config/model, ready token, stop command with optional workspace/tab placement
- `Response` in `internal/protocol/protocol.go` (type): Daemon reply: error, agent name/id, agents list, point-to-point message, spawn pane id, daemon status and version
- `Agent` in `internal/protocol/protocol.go` (type): Agent registry entry: name, type, species, pid, alive status, spawned metadata (integration, model, config, pane/workspace/tab ids, state)
- `Message` in `internal/protocol/protocol.go` (type): Point-to-point message: id, from/to agents, body, optional reply_to correlation
- `Do` in `internal/client/client.go` (function): Send one request to daemon, receive response; blocks for long operations like wait; attempts Unix socket then workspace file bridge fallback
- `Running` in `internal/client/client.go` (function): Probe whether daemon is listening; checks socket and file-bridge liveness
- `FindRoot` in `internal/workspace/workspace.go` (function): Walk up from dir to nearest .fledge directory, return canonical-absolute path with symlinks resolved; error instructs to run fledge init
- `Hash` in `internal/workspace/workspace.go` (function): SHA-256 hash of workspace absolute path, first 12 hex chars; keys socket namespace and session names so client/daemon agree on identity
- `Slug` in `internal/workspace/workspace.go` (function): Human-readable workspace identity: sanitized basename (lowercase, dashes, 16-char max) plus 6-char Hash suffix for collision handling
- `Dir` in `internal/flock/flock.go` (function): State directory for one flock: .fledge/flocks/<name>
- `SessionName` in `internal/flock/flock.go` (function): Herdr session name for a flock's default managed session: fledge-<workspace-slug>-<flock-name>; globally scoped in Herdr to prevent cross-workspace collisions
- `Validate` in `internal/flock/flock.go` (function): Flock name validation: non-empty, ≤32 chars, lowercase alphanumerics only; usable as path segment and env var
- `List` in `internal/flock/flock.go` (function): Enumerate flock state directories, sorted; returns empty list for missing workspace (not error)
- `Mint` in `internal/flock/flock.go` (function): Auto-mint lowest flockN name with no state; always fresh (never resumes stale journal)
- `FromEnv` in `internal/flock/flock.go` (function): Get flock from FLEDGE_FLOCK environment variable; hard error if unset or invalid (flock-scoped commands are useless without it)
- `Files` in `internal/scan/scan.go` (function): Scan directory tree respecting .fledgeignore (gitignore semantics); prunes ignored dirs, returns lexically sorted File structs with path and size
- `ParseFile` in `internal/ignore/ignore.go` (function): Parse .fledgeignore file; missing file yields empty matcher (graceful for un-initialized trees); #include directives resolve paths against root
- `Parse` in `internal/ignore/ignore.go` (function): Parse patterns from io.Reader (one per line); gitignore semantics with #include support, last-match-wins negation, dir-only markers
- `Matcher.Match` in `internal/ignore/ignore.go` (function): Test whether a slash-separated path is ignored; handles anchoring, wildcards (*, ?, [...]), deep wildcards (**), escaping, and negation
- `Pick` in `internal/species/species.go` (function): Assign penguin species slug: if requested is non-empty it must be known and un-taken; otherwise auto-pick first free in Slugs order; error if all 18 taken
- `Slugs` in `internal/species/species.go` (variable): Fixed list of 18 extant penguin species as lowercase slugs in auto-assignment order
- `Get` in `internal/version/version.go` (function): Return fledge binary version: embedded VERSION file trimmed + build-tag suffix ("-dev" if built with dev tag, empty otherwise)

### Dependencies

- Internal `internal/daemon`: Socket path derivation for client connections (daemon.SocketPath); sandbox-aware fallback coordination
- Internal `internal/scaffold`: Workspace structure constants (.fledge directory name) and initialization
- Internal `internal/filebridge`: Workspace-local request/response file bridge for sandboxed agents that cannot access runtime-directory sockets
- External `github.com/goccy/go-yaml`: YAML parsing for agent definitions and catalog (used indirectly by daemon/agentcfg)
- External `regexp`: Pattern compilation for .fledgeignore glob-to-regex translation
- External `crypto/sha256`: Workspace hash identity via SHA-256
- External `encoding/json`: Wire protocol marshaling (Request/Response/Agent/Message)
- External `net`: Unix socket dial/listen (client and test mocking)

### Data Flows

- `internal/workspace.FindRoot` → `internal/workspace.Hash + internal/workspace.Slug`: Workspace root determines session namespace identity; both daemon and CLI must canonicalize identically
- `internal/flock.SessionName` → `internal/workspace.Slug`: Flock's managed session name embeds workspace slug to prevent cross-workspace collisions in global Herdr namespace
- `internal/flock.FromEnv` → `internal/client.Do + internal/protocol.Request/Response`: Flock selection from environment gates all flock-scoped operations (register, send, wait, spawn, stop)
- `internal/client.Do` → `internal/protocol.Request + internal/protocol.Response`: Client marshals request to JSON, sends over socket, unmarshals response; blocks for streaming operations
- `internal/client.doFile` → `internal/filebridge`: Sandboxed agents use workspace-local file bridge when socket is unreachable
- `internal/scan.Files` → `internal/ignore.Matcher.Match`: Directory walk filters each path/dir through ignore patterns; pruning directories is how git semantics work
- `internal/ignore.ParseFile` → `internal/ignore.Parse + internal/ignore.compile`: File loading delegates to reader pipeline; patterns compiled lazily to regexps at parse time
- `internal/version.Get` → `internal/protocol.Response.DaemonVersion`: Daemon reports its version at status; CLI uses this to detect mismatched binary after install + restart
- `internal/species.Pick` → `internal/protocol.Agent.Species`: Species assignment happens at spawn time; self-registered agents pick one via this function

### Invariants

- Workspace root discovery must resolve symlinks so that client and daemon agree on hash regardless of spelling used to reach the workspace
- Flock session names embed workspace slug so concurrent workspaces cannot collide in Herdr's global session namespace
- Wire protocol is newline-delimited JSON with one request/response pair per connection; blocks for long operations like wait
- Sandboxed agents try Unix socket first, then fall back to workspace-local file bridge; socket path uses XDG_RUNTIME_DIR to keep it outside workspace (108-byte sun_path limit)
- Ignore patterns use gitignore semantics: last-match-wins for negation, directory pruning prevents re-inclusion of pruned subtrees, ** only spans directories as a whole segment
- Species pool is fixed at 18 penguin species, auto-assigned in order, with explicit request-and-validate fallback; all 18 taken is a hard error
- Version embeds VERSION file at build time with build-tag suffix; no ldflags, single source of truth
- Flock names are ≤32 chars, lowercase alphanumerics only, usable as path segment and environment variable; reserved namespace fledge-* for managed entities
- Agent lifecycle metadata (integration, model, config, pane/workspace/tab ids, state) recorded in protocol and journal at spawn/ready; spawned agents track session_id for Codex resume

### Tests

- `internal/protocol/protocol.go`: No tests in protocol.go itself; wire format is exercised by daemon e2e and client unit tests
- `internal/scan/scan_test.go`: Tests directory scanning: no patterns, size reporting, pruning ignored dirs, single-file ignore, preventing re-inclusion under pruned parent
- `internal/species/species_test.go`: Tests auto-pick first free, auto-pick with gaps, exhaustion, explicit request (free/taken/unknown), pool uniqueness and size (18)
- `internal/version/version_test.go`: Tests Get() returns strict MAJOR.MINOR.PATCH semver (no whitespace, no leading zeros)
- `internal/workspace/workspace_test.go`: Tests FindRoot walks up from nested dir, at root itself, prefers nearest ancestor, errors without .fledge, skips regular .fledge file, canonicalizes symlinks consistently; Hash stability and uniqueness; Slug sanitization (case/punctuation/truncation/collision suffix)
- `internal/client/client_test.go`: Tests request/response JSON exchange, error mapping, malformed/closed responses, Running() socket probe, file-bridge fallback when socket inaccessible
- `internal/flock/flock_test.go`: Tests Validate (valid/invalid names), List (sorted, empty workspace, missing), Mint (lowest free, gaps), FromEnv (unset, malformed), SessionName branding and workspace scoping, Slug stability
- `internal/ignore/ignore_test.go`: Tests Match (comments, bare names, globs, dir-only, anchoring, wildcards, classes, escaping, negation, deep wildcards); ParseFile (missing is empty); includes (nested, cycles, directives, error cases); 66 match scenarios

### Files

#### internal/client/client.go

_text._ Client RPC over Unix socket or file bridge: Do() sends request, unmarshals response, blocks for streaming; Running() probes socket then file-bridge; doFile() fallback for sandboxed agents with timeouts; ErrNotRunning is operator-facing instruction

#### internal/client/client_test.go

_text._ 5 tests: JSON exchange, daemon error mapping, malformed/closed response handling, Running() probe, file-bridge fallback (sandbox scenario with different XDG_RUNTIME_DIR); serveOnce() helper mocks listener

#### internal/flock/flock.go

_text._ Flock state and identity: Env constant FLEDGE_FLOCK, DirName "flocks", MaxName 32 (sun_path constraint); Dir() path to state; SessionName() derives fledge-<workspace-slug>-<name> (globally scoped); SessionPrefix() for cleanup; WindowTitle() for terminal; Validate() checks name constraints; List() enumeration; Mint() lowest free flockN; FromEnv() retrieves with validation

#### internal/flock/flock_test.go

_text._ 7 test groups: Validate (valid/invalid cases), List (sorted, empty, missing), Mint (lowest free, gaps, never existing), FromEnv (unset/malformed), SessionName/WindowTitle branding, workspace-scoped session identity, slug stability; scratch() helper creates scaffolded temp workspace

#### internal/ignore/ignore.go

_text._ Gitignore-style ignore-pattern matching: Matcher struct (patterns array), ParseFile() optional top-level, Parse() from reader, load() recursive includes with cycle detection, read() processes lines; pattern struct (regexp, negate, dirOnly); Match() applies patterns in order (last-match-wins); parseLine() handles negation (!), escaping (\#, \!), dir-only (/), anchoring (/prefix); compile() translates glob to regexp (*, ?, [...], **, literal escapes, gitignore semantics)

#### internal/ignore/ignore_test.go

_text._ 66+ match test cases covering comments/blanks, bare names, globs, wildcards, char classes (positive/negated/posix), escaping, negation/re-inclusion, deep wildcards, leading/interior/trailing **, anchoring, dir-only, separator rules; 4 include tests (nested, cycles, errors, directives); write() helper creates test files

#### internal/protocol/protocol.go

_text._ Newline-delimited JSON wire protocol: env vars (agent name, ready token, credential, codex thread), journal/log filenames, operation opcodes (register/list/status/send/reply/inbox/wait/peek/receive/ack/spawn/ready/stop/shutdown), Request struct with op-specific fields (agent id, send routing, spawn config/model/integration/workspace/tab/split/anchor/orchestrator, ready token/session id), Response struct with error handling, metadata (agent/pane/session), Agent struct (name/type/species/pid/alive + spawned metadata), Message struct (id/from/to/body/reply_to)

#### internal/scan/scan.go

_text._ Directory walker filtering by ignore patterns: File type (path, size), Files() function walks filepath tree, prunes ignored dirs, returns lexically sorted File array; used to report workspace contents

#### internal/scan/scan_test.go

_text._ 4 tests: no patterns, size reporting, directory pruning, single-file re-inclusion logic; helper tree() creates temp dir with paths, helper files() returns just paths for assertions

#### internal/species/species.go

_text._ Penguin species assignment: Slugs array (18 species in order), Pick() function validates requested slug or auto-picks first free, known() helper checks membership

#### internal/species/species_test.go

_text._ 6 Pick test cases + uniqueness/size test; takenSet() helper builds a predicate from slug list

#### internal/version/VERSION

_text._ Single line: 0.1.0 (semantic versioning, embedded at build time)

#### internal/version/suffix.go

_text._ Build-tag !dev variant: suffix constant empty string (production builds)

#### internal/version/suffix_dev.go

_text._ Build-tag dev variant: suffix constant "-dev" (local installs via scripts/install.sh)

#### internal/version/version.go

_text._ Package version reports fledge binary version: embeds VERSION file, Get() returns trimmed raw + build-tag suffix; no ldflags, single source of truth

#### internal/version/version_test.go

_text._ 1 test: Get() matches strict MAJOR.MINOR.PATCH regex (numeric, no leading zeros, no surrounding whitespace)

#### internal/workspace/workspace.go

_text._ Workspace root discovery and identity: FindRoot() walks up to .fledge dir, canonicalizes symlinks, ErrNotFound instructs fledge init; Hash() SHA-256 of absolute path (12 hex chars, keys socket namespace); Slug() basename sanitization + 6-char hash suffix; slugBase() handles case, punctuation, truncation, collapse runs, trim trailing dashes

#### internal/workspace/workspace_test.go

_text._ 8 FindRoot tests (nested walk, at root, nearest ancestor shadowing, hard error, regular file skip, symlink canonicalization); Hash tests (stability, uniqueness, length); Slug tests (sanitization cases, hash suffix format); helper mark() creates .fledge dir
