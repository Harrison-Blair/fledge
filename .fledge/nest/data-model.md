---
generated: 2026-07-15T23:53:12Z
commit: a4d02e8187c64ef9f3f1231052990b282207420b
agent: fledge-forager
fledge_version: 0.5.5
---

# Data Model

Core types, schemas, and on-disk structures defined across the codebase, with file references.

## Spec domain (`internal/spec`)

- **`Requirement`** (plumage/PLM file) — `internal/spec/types.go:27`. Fields: `ID`, `Title`, `Status` (`egg`/`hatched`/`fledged`), `Priority`, `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body` (byte-preserved markdown).
- **`Task`** (feather/FTHR file) — `internal/spec/types.go:41`. Fields: `ID`, `Title`, `Requirement` (parent PLM id), `Status` (`egg`/`pipping`/`hatching`/`fledged`), `Priority`, `DependsOn ([]string)`, `Oversight` (`"merge"`/`"during"`/`""`), `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body`.
- **`Set`** — `internal/spec/load.go:18`. Loaded collection: `Reqs ([]*Requirement)`, `Tasks ([]*Task)`, `Errors ([]FileError)`, `UnknownFields (map[path][]string)`.
- **`FileError`** — `internal/spec/load.go:10`. Attributes a parse failure to a file path.
- **`Criterion`** — `internal/spec/criteria.go:11`. One AC checkbox line: `N` (number), `Label` (`"AC-N"`), `Checked (bool)`, `Text`, `boxOff` (byte offset for single-byte mutation).
- Priorities: `P0`–`P3` (valid set in `Priorities`). Oversight: `"merge"` (checked at merge gate) | `"during"` (checked during implementation) | `""` (no gate).

## Validation findings (`internal/check`)

- **`Finding`** — JSON-serializable: `File`, `Rule`, `Severity`, `Message`.
- **`Severity`** — const enum: `Error`, `Warning`.

## Dependency graph (`internal/graph`)

- **`Graph`** — `internal/graph/graph.go`. Tasks slice + `byID` map. Methods: `Cycle()` (DFS), `Waves()` (topological layering), `Ready()` (deps-fledged filter, `egg`/`pipping` only).

## Locking (`internal/lock`)

- **`Record`** — JSON-serialized `.brood` file content: `Task` (feather ID), `Owner`, `PID`, `Created` (RFC 3339), `Branch`.
- **`HeldError`** — wraps a conflicting `Record`; `Error()` formats owner + creation time.

## Repo/scan (`internal/repo`, `internal/scan`)

- **`Repo`** — `Root string` (absolute path); accessors `FledgeDir()`, `LocksDir()`, `ContextDir()`, `ScanIgnorePath()`, `EvidenceDir()`, `RequirementsDir()`, `TasksDir()`.
- **`Result`** (scan) — `Commit`, `ShortCommit`, `Modules ([]Module)`.
- **`Module`** (scan) — `Name` (top-level dir or `"<root>"`), `Files ([]string)`, `Count (int)`, `Bytes (int64)`.

## CLI-layer view types (`internal/cli`)

Thin request/response shapes for `--json` output and internal dispatch — not persisted, just serialized per-command: `command` (`run func([]string) int`, `usage`), `stringListFlag`, `initJSON` (`Created/Skipped/Updated/Agents/Removed []string`), `adapterInfo` (`Name/Tier/Detector/Scaffolded`), `readyTask`, `graphNode`, `criterionJSON`, `reportCounts`, `reqCompletion`, `orphanTask`/`blockedTask`, `lockEntry`, `issues` (`ParseErrors/DanglingRefs`), `unfledgedItem`/`unfledgedReport`, `updateJSON`, `githubRelease`.

## Bootstrap/adapter schema (`internal/bootstrap`)

- **`Manifest`** — `registry.go`. `Name`, `Detector (ManifestDetector)`, `TierPrimitives (map[string]string)`, `Files ([]ManifestFile)`, `PipingFile`, `dir` (embed FS path).
- **`ManifestFile`** — `Src`, `Dst`, `Generate (bool)`, `PrimitiveMap (bool, implies Generate)`, `Overwrite (bool)`, `AppendIfMissing (string)`, `Symlink (string)`.
- **`ManifestDetector`** — `Exists string` (marker path, e.g. `.claude/`).
- **`Stamp`** — `stamp.go`. `FledgeVersion`, `Agents ([]string)`, `Files (map[string]StampEntry)` — persisted as `.fledge/scaffold.json`.
- **`StampEntry`** — `Policy` (one of `core`/`default`/`generate`/`primitive_map`/`overwrite`/`symlink`/`append`), `Sha256`, `Target` (symlink dest), `Lines ([]string)` (for append policy). Policy + payload field is mutually exclusive per entry.
- **`Drift`** — `drift.go`. `Path`, `Status (DriftStatus)`, `Policy`.
- **`DriftStatus`** — string const enum: `up-to-date`, `stale`, `modified`, `missing`, `obsolete`.
- **`PrimitiveOrder`** — the 6 primitives in fixed order: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`.
- **`TierPrimitives`** — map: Tier A → 4 primitives, Tier B → +`spawn-worker`, Tier C → +`message-peer`.

## Nest documents (`internal/nest`)

- **`Kind`** — string const: `Concern` | `Scout`.
- **`Doc`** — `nest.go:29–47`. `Kind`, `Generated`/`Commit` (Concern only), `Module`/`Authored` (Scout only), `Agent`, `FledgeVersion` (shared), `Body ([]byte`, byte-preserved).
- **`StatusResult`** — `nest.go:105–112`. `Complete (bool)`, `IndexCommitMatches (bool)`, `Head`/`IndexCommit (string)`, `MissingDocs`/`StubDocs ([]string`, in `ConcernDocs` order).
- **`ConcernDocs`** — fixed ordered list of the 9 concern-doc names (`architecture`, `modules`, `conventions`, `data-model`, `dependencies`, `entry-points`, `testing`, `domain`, `index`).

## On-disk spec frontmatter shapes

- **Plumage frontmatter:** `id` (PLM-###), `title`, `status` (egg\|hatched\|fledged), `priority` (P0–P3), `authored` (UTC ISO 8601), `agent`, `fledge_version`.
- **Feather frontmatter:** all plumage fields plus `plumage` (parent PLM-###), `depends_on` (list of FTHR-###), `oversight` (optional: merge\|during).
- **Nest concern-doc frontmatter:** `generated`, `commit`, `agent`, `fledge_version` (fixed key order, enforced by `Doc.Frontmatter()`).
- **Nest scout-report frontmatter:** `module`, `authored`, `agent`, `fledge_version`.
- **Evidence file** (`.fledge/molt/FTHR-###.md`, worktree-local): one `## AC-N` section per acceptance criterion; AC-1 always the pre-implementation failing-test capture; commands + verbatim output per criterion.
- **Brood claim** (`.fledge/broods/FTHR-###.brood`): the `lock.Record` JSON shape above.

## Open Questions

None observed — all raw scout reports resolved their module's data types without ambiguity.
