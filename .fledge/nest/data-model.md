---
generated: 2026-07-08T01:03:26Z
commit: e44524d1f089dcfe1c1f313f819ec18d9a42eceb
agent: fledge-forager
fledge_version: 0.2.1
---

# Data Model

Core types across the CLI/domain layer and the spec files they represent on disk.

## Spec types (`internal/spec/types.go`)

- **`Requirement`** (a plumage, `pluma/plumage/PLM-###.md`) — `id`, `title`, `status` (`egg|hatched|fledged`), `priority` (`P0`–`P3`), `authored` (RFC3339), `agent`, `fledge_version`, `path`, `body`.
- **`Task`** (a feather, `pluma/feathers/FTHR-###.md`) — `id`, `title`, `plumage` (parent requirement ID), `status` (`egg|pipping|hatching|fledged`), `priority`, `depends_on` (`[]string` of feather IDs, acyclic), `oversight` (`merge|during|""`), `authored`, `agent`, `fledge_version`, `path`, `body`.

Frontmatter keys are fixed-order on serialize; optional keys are omitted when empty. Unknown keys are tracked per file rather than rejected, surfacing as a warning in `fledge preen` output (`internal/spec/load.go:Set.UnknownFields`).

## Load result (`internal/spec/load.go`)

- **`Set`** — `Reqs`, `Tasks`, `Errors` (`[]FileError`: path + parse error), `UnknownFields` (`map[path][]keys`). `spec.Load(reqDir, taskDir)` builds a `Set`; `Set.Req(id)` / `Set.Task(id)` do lookup.

## Criteria (`internal/spec/criteria.go`)

- **`Criterion`** — `N` (int), `Label` (`AC-N` string), `Checked` (bool), `Text`, `boxOff` (byte offset of the checkbox character in the file). Parsed by regex, scoped strictly to content under a `## Acceptance Criteria` heading. `SetCriterion` flips exactly the one byte at `boxOff` — the rest of the file, including surrounding prose, is untouched.

## Validation findings (`internal/check/check.go`)

- **`Severity`** — `Error | Warning`.
- **`Finding`** — `file`, `rule`, `severity`, `message` (all JSON-tagged for `--json` output). Rules include: parse, unknown-field, duplicate-id, schema (required fields/enum/format), id-filename, dangling-ref, unhatched-plumage, tests-section, required-sections, stale-pipping-hint, cycle, brood-consistency, criteria-format, criteria-incomplete, criteria-evidence.

## Dependency graph (`internal/graph/graph.go`)

- **`Graph`** — built over `[]*Task`, backed by a `byID` map. Methods: `Cycle()` (detect + report a cycle path), `Waves()` (topological layers for parallel-work planning), `Ready()` (feathers whose dependencies are all `fledged` and which are not currently brooded). Dangling `depends_on` references are tolerated by the graph (reported separately by `check`) but excluded from wave/cycle computation.

## Brood / lock records (`internal/lock/lock.go`)

- **`Record`** — `Task` (FTHR-ID), `Owner`, `PID`, `Created` (RFC3339), `Branch`. Serialized as JSON to `.fledge/broods/<FTHR-ID>.brood`.
- **`HeldError`** — wraps the existing `Record` when `Acquire()` loses a race; acquisition is atomic via `O_EXCL` so exactly one caller wins under concurrent contention (tested with 16 concurrent acquirers in `internal/lock/lock_test.go`).

## Repo paths (`internal/repo/repo.go`)

- **`Repo`** — wraps `Root`; exposes `FledgeDir()`, `LocksDir()` (`.fledge/broods`), `ContextDir()` (`.fledge/nest`), `EvidenceDir()` (`.fledge/molt`), `RequirementsDir()` (`pluma/plumage`), `TasksDir()` (`pluma/feathers`), plus `RequireFledge()`, `Version()`, `Head()`.

## Scan result (`internal/scan/scan.go`)

- **`Module`** — `Name`, `Files`, `Count`, `Bytes`.
- **`Result`** — `Commit` (full sha), `ShortCommit`, `Modules` (grouped by top-level directory, `.fledgeignore`-filtered via `git check-ignore`).

## Bootstrap/adapter types (`internal/bootstrap/registry.go`, `primitives.go`)

- **`Manifest`** — `Name`, `Detector` (marker path for auto-detect), `TierPrimitives` (primitive → mechanism map), `Files` (`[]ManifestFile`), `PipingFile`, plus an internal `dir`.
- **`ManifestDetector`** — `Exists` (path checked to auto-detect this harness is in use).
- **`ManifestFile`** — `Src`, `Dst`, and mutually-describing write-policy fields: `Generate`, `PrimitiveMap`, `Overwrite`, `AppendIfMissing`, `Symlink`.
- **`primitiveRow`** / **`renderContext`** — template-rendering data structures (adapter name, tier, provided/not-provided primitive rows, `PipingFile`, `CommandOrder`) used when generating each adapter's `fledge-adapter.md`.
- **`PrimitiveOrder`** — canonical ordered list of the 7 primitives; **`primitiveDesc`**, **`primitiveTier`**, **`TierPrimitives`** — maps from primitive name to description / minimum tier / full per-tier primitive set.

## Relationships

```
Requirement (PLM-###) 1---N Task (FTHR-###)     [Task.plumage → Requirement.id]
Task N---N Task                                  [Task.depends_on → other Task.id, acyclic]
Task 1---0..1 Record                             [lock.Record.Task → Task.id, while hatching]
Task 1---0..1 evidence file                       [.fledge/molt/<FTHR-ID>.md, per-criterion evidence]
Manifest 1---N ManifestFile                       [scaffolded file write policies]
Manifest N---N primitive                          [TierPrimitives: which of the 7 this harness realizes]
```

## Open Questions
- Full JSON schema/shape for `--json` output of each command is described in prose (functional criteria) but not published as a formal schema document.
- Are `.fledge/molt/<FTHR-ID>.md` evidence files generated automatically by the CLI or authored manually by the implementing agent? Full semantics of the `criteria-evidence` check rule were not resolved from assigned source files.
