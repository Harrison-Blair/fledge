---
generated: 2026-07-16T21:27:15Z
commit: a1ed62a38540df7ab1cbdc4c486176a64a762018
agent: fledge-forager
fledge_version: 0.5.8
---

# Data Model

Core types across the spec system, CLI output shapes, the bootstrap/scaffold system, and the nest/context-doc system.

## Spec types (`internal/spec/types.go`)

- **`Requirement`** (plumage, PLM-###): `ID`, `Title`, `Status` (`ReqEgg`|`ReqHatched`|`ReqFledged`), `Priority`, `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body []byte`.
- **`Task`** (feather, FTHR-###): `ID`, `Title`, `Requirement` (parent PLM link), `Status` (`TaskEgg`|`TaskPipping`|`TaskHatching`|`TaskFledged`), `Priority`, `DependsOn []string`, `Oversight`, `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body []byte`.
- **`Criterion`**: `N int`, `Label`, `Checked bool`, `Text`, `boxOff` (byte offset in body — enables in-place byte-preserving mutation via `internal/spec/criteria.go:SetCriterion`).
- **`Set`**: loaded collection — `Reqs`, `Tasks` slices, `Errors []FileError`, `UnknownFields map[path][]string`; lookup via `Set.Req(id)`/`Set.Task(id)`.
- **`FileError`**: `Path`, `Err` — one per file that failed to parse; aggregated, never fail-fast (`internal/spec/load.go:Load`).
- **Constants**: `Priorities = [P0,P1,P2,P3]`; `OversightValues = [merge, during]` — note root `CLAUDE.md`/orchestration prose additionally treat an omitted `oversight` field as a third ("autonomous") mode.

## Lock/claim types (`internal/lock/lock.go`)

- **`Record`** (JSON, one file per held brood under `.fledge/broods/`): `Task`, `Owner`, `PID`, `Created`, `Branch`, `Worktree` (Worktree is a later addition — `lock_test.go` verifies backward compat with pre-Worktree JSON).
- **`HeldError`**: wraps the existing `Record` on an `Acquire()` conflict; implements the `error` interface.

## Validation types (`internal/check/check.go`)

- **`Finding`**: `File`, `Rule`, `Severity`, `Message` — one validation result.
- **`Severity`**: `Error` | `Warning` constants.

## Graph types (`internal/graph/graph.go`)

- **`Graph`**: internal `tasks`/`byID` map; methods `Cycle() []string` (DFS, unvisited/inStack/finished state machine), `Waves() ([][]string, error)` (topological layers, greedy per-layer selection), `Ready() []string` (egg/pipping tasks with all deps fledged, input order preserved).

## CLI output shapes (`internal/cli/*.go`, mostly JSON-marshaled)

- `initJSON` (init.go), `adapterInfo` (agents.go), `readyTask`/`unfledgedItem`/`unfledgedReport` (ready.go/unfledged.go), `reportCounts`/`reqCompletion`/`orphanTask`/`blockedTask`/`lockEntry`/`report` (colony.go — portfolio view), `criterionJSON` (criteria.go), `graphNode` (vee.go), `githubRelease`/`updateJSON` (update.go), `scaffoldJSONOut`/`scaffoldEntry` (preen.go — drift findings).
- `scan.Module`: `Name`, `Files []string`, `Count`, `Bytes`. `scan.Result`: `Commit`, `ShortCommit`, `Modules []Module` — this is the exact shape `fledge scan --json` emits and this forager consumed as its work list.

## Bootstrap/scaffold types (`internal/bootstrap/{registry,primitives,drift,stamp}.go`)

- **`Manifest`**: `Name` (claude|codex|pi), `Detector` (`ManifestDetector{Exists path}`), `TierPrimitives map[string]string` (6 primitive names → mechanism string), `Files []ManifestFile`, `PipingFile` (optional, Claude only).
- **`ManifestFile`**: `Src`, `Dst`, `Generate bool`, `PrimitiveMap bool`, `Overwrite bool`, `AppendIfMissing string`, `Symlink string` — write-policy fields, mutually distinguishing the 6 policies described in conventions.md.
- **`PrimitiveOrder`**: canonical `[]string` of the 6 primitives. **`TierPrimitives`**: `map[Tier][]string` of required primitives per tier.
- **`DriftStatus`**: enum `StatusUpToDate`|`StatusStale`|`StatusModified`|`StatusMissing`|`StatusObsolete`. **`Drift`**: `Path`, `Status`, `Policy`.
- **`Stamp`** (`.fledge/scaffold.json`): `FledgeVersion`, `Agents []string`, `Files map[string]StampEntry`. **`StampEntry`**: `Policy`, `Sha256` (hex), `Target` (symlink), `Lines` (append_if_missing).

## Nest/context-doc types (`internal/nest/{nest,docs}.go`)

- **`Kind`**: string enum `Concern` | `Scout`.
- **`Doc`**: concern fields `Generated` (RFC3339), `Commit` (git SHA); scout fields `Module`, `Authored` (RFC3339); shared `Agent`, `FledgeVersion`, `Body []byte`.
- **`StatusResult`**: `Complete bool`, `IndexCommitMatches`, `Head`, `IndexCommit`, `MissingDocs []string`, `StubDocs []string` — the exact struct `fledge nest status` reports, and what this forager's step 7 gate checks.
- **`ConcernDocs`**: 9-member ordered closed set — `architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain, index`.

## Roster/repo types (`internal/roster/roster.go`, `internal/repo/repo.go`)

- **`roster.Entry`**: `Species`, `Members []string` (pair-share support), `Released []bool` (per-member), `Feather` (originating feather ID reference).
- **`roster.Species`**: fixed `[18]string` — adelie, emperor, gentoo, king, chinstrap, little, african, humboldt, magellanic, galapagos, yelloweyed, fiordland, snares, erectcrested, rockhopper, royal, macaroni, northernrockhopper.
- **`repo.Repo`**: single `Root string` field; accessor methods (`FledgeDir()`, `LocksDir()`, `RosterDir()`, `ContextDir()`, `EvidenceDir()`, `RequirementsDir()`, `TasksDir()`, `ScanIgnorePath()`) derive all `.fledge/` subpaths from it.

## Domain-prose data shapes (not code — conventions asserted in `internal/bootstrap/core/skills/fledge-orchestrate/templates/*.md`)

- **Evidence file** (`.fledge/molt/FTHR-###.md`): `## AC-N` sections, each holding command(s) + verbatim captured output.
- **Relay envelope** (incubator↔orchestrator messages): `GATE review`, `GATE decision`, `QUESTION`, `SPAWN-REQUEST`, `PHASE-CLOSE`.
- **Dispatch status fields** (implementation.md §3.1): `owner`, `branch`, `worktree` path, `worktree_exists` boolean.

## Open Questions

None observed — every type above traces to a concrete struct definition or documented schema in a scout report.
