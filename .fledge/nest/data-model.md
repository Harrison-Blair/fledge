---
generated: 2026-07-11T01:58:32Z
commit: 96a3ac38bc843217824d6d6886c49906053bf686
agent: fledge-forager
fledge_version: 0.3.4
---

# Data Model

Core types and on-disk schemas across the codebase, organized by the domain they model rather than by source file.

## Spec types (`internal/spec/types.go`)

- **`Requirement`** (a plumage/PLM spec): `ID`, `Title`, `Status` (`egg`|`hatched`|`fledged`), `Priority`, `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body` (raw preserved bytes).
- **`Task`** (a feather/FTHR spec): `ID`, `Title`, `Requirement` (parent PLM ref), `Status` (`egg`|`pipping`|`hatching`|`fledged`), `Priority` (`P1`|`P2`), `DependsOn` ([]string of FTHR IDs), `Oversight` (optional, e.g. `merge`), `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body`.
- **`Criterion`** — one acceptance-criteria checkbox: `N`, `Label` (`AC-N`), `Checked`, `Text`, `boxOff` (byte offset used for single-byte state mutation via `SetCriterion`).
- **`Set`** — bulk-load container: `Reqs []*Requirement`, `Tasks []*Task`, `Errors []FileError`, `UnknownFields map[string][]string`. Built by `Load(reqDir, taskDir)`; collects per-file errors instead of failing the whole load.
- **`FileError`** — `{Path, Err}` pair for one failed parse in a `Set`.
- Frontmatter is YAML, bounded by `---` delimiters, fixed key order, rendered/parsed in `internal/spec/frontmatter.go` (`SplitFrontmatter`, `Frontmatter()`, `Render()`).

## Dependency & lock types

- **`Graph`** (`internal/graph/graph.go`) — wraps `tasks []*spec.Task` + `byID map[string]*spec.Task`; methods `Cycle()` (DFS cycle detection), `Waves()` (topological layers), `Ready()` (unstarted tasks with satisfied `DependsOn`).
- **`Record`** (`internal/lock/lock.go`) — a brood claim: `Task`, `Owner`, `PID` (int), `Created` (RFC3339), `Branch`. JSON-encoded into `.fledge/broods/*.brood` files.
- **`HeldError`** — wraps an existing `Record`; returned by `Acquire()` on lock contention.

## Nest/context doc types

- **`Doc`** (`internal/nest/nest.go`) — `Kind` (`"concern"`|`"scout"`), plus `Generated`/`Commit` (concern docs) or `Module`/`Authored`/`Agent` (scout reports), `FledgeVersion`, `Body`.
- `ConcernDocs` (`internal/nest/docs.go`) — the closed, ordered set of 9 known concern docs: architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain, index. `IsKnownDoc()`/`Title()` back this registry; scout reports (module-named) are an open set by contrast.

## Validation & scan types

- **`Finding`** (`internal/check/check.go`) — `{File, Rule, Severity, Message}`; `Severity` is `"error"`|`"warning"`.
- **`Module`** / **`Result`** (`internal/scan/scan.go`) — `Module{Name, Files []string, Count, Bytes int64}`; `Result{Commit, ShortCommit, Modules []Module}` — this is the schema `fledge scan --json` emits and the forager's authoritative work list.
- **`Repo`** (`internal/repo/repo.go`) — `{Root string}`, with derived path accessors (`FledgeDir`, `LocksDir`, `ContextDir`, `EvidenceDir`, `RequirementsDir`, `TasksDir`, `ScanIgnorePath`).

## CLI-layer report types (`internal/cli`)

These are output-shaping structs, not persisted state: `reportCounts` (colony status counts), `reqCompletion` (per-plumage completion), `orphanTask`/`blockedTask`/`lockEntry` (colony report items), `adapterInfo` (agents command), `criterionJSON`, `unfledgedReport`, `readyTask`, `graphNode` (vee), `scaffoldJSONOut`/`scaffoldEntry` (preen drift), `initJSON`, `lockOut`.

## Scaffold/bootstrap types (`internal/bootstrap`)

- **`Manifest`** (`registry.go`) — `{Name, Detector{Exists}, TierPrimitives map[string]string, Files []ManifestFile, PipingFile, dir}` — one per harness adapter, the single source of truth for what gets scaffolded.
- **`ManifestFile`** — `{Src, Dst, Generate, PrimitiveMap, Overwrite, AppendIfMissing, Symlink}` — six write-policy booleans, classified by `filePolicy()`.
- **`Stamp`** / **`StampEntry`** (`stamp.go`) — `Stamp{FledgeVersion, Agents []string, Files map[string]StampEntry}`; `StampEntry{Policy, Sha256, Target, Lines}` (exactly one of Sha256/Target/Lines populated depending on policy). Persisted as `.fledge/scaffold.json`.
- **`DriftStatus`** enum (`drift.go`) — `StatusUpToDate | StatusStale | StatusModified | StatusMissing | StatusObsolete`; **`Drift`** — `{Path, Status, Policy}`.

## Open Questions

None observed — all data types above were directly read from source across scouted modules.
