---
generated: 2026-07-10T20:53:53Z
commit: f28efebd76d6aa135adb0956a3337a40a8d98351
agent: fledge-forager
fledge_version: 0.3.0
---

# Data Model

Core Go types across `internal/spec`, `internal/check`, `internal/graph`, `internal/lock`, `internal/nest`, `internal/repo`, `internal/scan`, and the CLI's own JSON output shapes.

## Spec types (`internal/spec/types.go`)

- **`Requirement`** (plumage, `PLM-###`): `ID, Title, Status (egg|hatched|fledged), Priority, Authored, Agent, FledgeVersion, Path, Body` (body preserved byte-for-byte).
- **`Task`** (feather, `FTHR-###`): `ID, Title, Requirement (PLM ref), Status (egg|pipping|hatching|fledged), Priority, DependsOn ([]string), Oversight (merge|during|""), Authored, Agent, FledgeVersion, Path, Body`.
- **`Set`** (`internal/spec/load.go`): `Reqs, Tasks, Errors ([]FileError), UnknownFields (map)` — the parsed universe of specs for a repo.
- **`Criterion`** (`internal/spec/criteria.go`): `N, Label (AC-N), Checked, Text`, plus an internal byte offset used for in-place mutation.
- **`FileError`**: `Path, Err` — one entry per spec file that failed to parse.

## Validation types (`internal/check/check.go`)

- **`Finding`**: `File, Rule, Severity (Error|Warning), Message` (JSON-tagged) — the unit of `preen` output. Rules include parse, unknown-field, duplicate-id, schema, dangling-ref, cycle, brood-consistency, and criteria-related checks.

## Graph types (`internal/graph/graph.go`)

- **`Graph`**: holds `tasks`/`byID` map; methods `Cycle()` (DFS cycle detection), `Waves()` (topological layers — feathers with no inter-dependencies group into the same wave), `Ready()` (unstarted tasks whose `depends_on` are all fledged).
- Dangling `depends_on` references are tolerated (never counted as "done"); no cycle edges are formed through missing tasks.

## Lock/brood types (`internal/lock/lock.go`)

- **`Record`**: `Task (FTHR-ID), Owner, PID, Created, Branch` — one JSON file per claim at `.fledge/broods/<ID>.brood`.
- **`HeldError`**: wraps the existing `Record` when `Acquire` finds a conflicting live claim.

## Nest/context-doc types (`internal/nest/nest.go`, `docs.go`)

- **`Doc`**: `Kind (Concern|Scout)`; concern docs carry `Generated, Commit`; scout docs carry `Module, Authored`; both carry `Agent, FledgeVersion, Body`.
- **`Kind`**: string enum, `"concern"` or `"scout"`.
- **`ConcernDocs`**: the fixed list of 9 known doc names — the 8 concern docs this forager writes plus `index.md`.

## Repo/scan types (`internal/repo/repo.go`, `internal/scan/scan.go`)

- **`Repo`**: `Root` (absolute path to git top-level); methods expose `.fledge/broods`, `.fledge/nest`, `.fledge/molt`, `pluma/plumage`, `pluma/feathers` paths, plus `RequireFledge()`, `Version()`, `Head()` (commit SHA).
- **`Module`** (scan): `Name, Files, Count, Bytes` — one entry per top-level directory (or `<root>`), the unit this very foraging pipeline plans scout assignments around.
- **`Result`** (scan): `Commit, ShortCommit, Modules`.

## Bootstrap/scaffold types (`internal/bootstrap`)

- **`Manifest`** (`registry.go`): `name, detector (Exists marker), tier_primitives (map[primitive]mechanism), files[], piping_file, dir` — one per harness (claude/codex/pi), parsed from `manifest.yaml`.
- **`ManifestFile`**: `src, dst`, plus policy bools `Generate/PrimitiveMap/Overwrite/AppendIfMissing/Symlink` and a symlink target string.
- **`Stamp`** (`stamp.go`): `FledgeVersion, Agents[], Files map[repoPath]StampEntry` — serialized to `.fledge/scaffold.json`.
- **`StampEntry`**: `Policy`, plus exactly one of `Sha256 (hash), Target (symlink), Lines[] (append)`.
- **`Drift`** (`drift.go`): `Path, Status (DriftStatus), Policy`.
- **`DriftStatus`**: string-const enum — `StatusUpToDate | StatusStale | StatusModified | StatusMissing | StatusObsolete`.
- **`primitiveRow`**/**`renderContext`**: template-rendering structs feeding the per-adapter `fledge-adapter.md` primitive-map table (Name, Desc, Mechanism, Provided, Tier / Adapter, Tier, Rows, Provided/NotProvided, PipingFile, CommandOrder).

## CLI JSON output types (`internal/cli/*.go`, one per command)

`adapterInfo` (agents.go: Name/Tier/Detector/Scaffolded); `criterionJSON` (criteria.go); `report`/`reportCounts`/`reqCompletion`/`orphanTask`/`blockedTask`/`lockEntry`/`issues` (colony.go); `initJSON` (init.go: Created/Skipped/Updated/Agents/Kept/Removed/Obsolete); `readyTask` (ready.go); `unfledgedItem`/`unfledgedReport` (unfledged.go); `graphNode` (vee.go: ID/Title/Status/Requirement/DependsOn); `scaffoldJSONOut`/`scaffoldEntry` (preen.go).

## Status transition matrices (`internal/cli/status.go`)

- `taskTransitions`: `egg→hatching`, `pipping→hatching`, `hatching→{fledged,pipping}`.
- `reqTransitions`: `egg→hatched`, `hatched→{fledged,egg}`.
- Both enforced unless `--force`; fledging a requirement additionally checks for unchecked acceptance criteria via `uncheckedCriteria(body)`.

## Open Questions

- `lockEntry` in `internal/cli/colony.go` uses JSON field tag `"feather"` for the Task ID — unclear if intentional vs. a rename candidate for external consumers. (internal-cli scout)
</content>
