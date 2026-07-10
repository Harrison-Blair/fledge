---
generated: 2026-07-10T14:50:00Z
commit: 7678344ab9a18730530b9f6edf507ad0c449d352
agent: fledge-forager
fledge_version: 0.2.1
---

# Data Model

Core types and schemas that fledge operates on: the spec model (plumages/feathers), CLI JSON-output types, and the bootstrap/manifest schema.

## Spec model (`internal/spec/types.go`)

- **`spec.Requirement`** (a plumage/PLM): `ID, Title, Status, Priority, Authored, Agent, FledgeVersion, Path, Body`. Status: `egg → hatched → fledged`.
- **`spec.Task`** (a feather/FTHR): `ID, Title, Requirement, Status, Priority, DependsOn[], Oversight, Authored, Agent, FledgeVersion, Path, Body`. Status: `egg → pipping → hatching → fledged`. `DependsOn` references other FTHR/PLM IDs; validated for dangling refs and cycles.
- **`spec.Set`**: loaded spec collection — `Reqs[], Tasks[], Errors[], UnknownFields map`. `Load()` never aborts on a single parse error; accumulates all in `Set.Errors` (`internal/spec/load.go`).
- **`spec.Criterion`**: a parsed AC checkbox — `N, Label, Checked, Text`, plus an internal `boxOff` byte-offset for in-place mutation (`internal/spec/criteria.go`).
- **`spec.FileError`**: a parse error attributed to a file path.

**Spec frontmatter (YAML), on disk** (`pluma/plumage/PLM-*.md`, `pluma/feathers/FTHR-*.md`):
- Plumage: `id, title, status, priority, authored, agent, fledge_version`.
- Feather: `id, title, plumage` (parent ID), `status, priority, depends_on` (list), `oversight` (merge gate), `authored, agent, fledge_version`.
- Prose body sections — Plumage: Context, User Stories, Functional Criteria (FC-N), Acceptance Criteria (AC-N), Out of Scope, Open Questions. Feather: Description, Affected Modules, Approach, Tests, Acceptance Criteria (checkboxes).

## Validation & graph types (`internal/check`, `internal/graph`, `internal/lock`)

- **`check.Finding`**: `File, Rule, Severity, Message` — JSON-serializable validation result. **`check.Severity`**: `"error" | "warning"`. Rules (`internal/check/check.go`): parse, unknown-field, duplicate-id, schema, id-filename, dangling-ref, unhatched-plumage, tests-section, required-sections, criteria-format, criteria-incomplete, criteria-evidence, stale-pipping-hint, cycle, brood-consistency.
- **`graph.Graph`**: depends-on DAG over tasks; tolerates dangling refs (treated as never-done by `Ready`, skipped by `Cycle`/`Waves`).
- **`lock.Record`** (a brood): `Task, Owner, PID, Created, Branch` — JSON persisted at `.fledge/broods/<FTHR-ID>.brood`. **`lock.HeldError`**: acquisition conflict distinguishing "held by someone else" from corruption.

## Nest doc types (`internal/nest`)

- **`nest.Doc`**: unified context-doc representation — `Kind` (`concern`|`scout`), plus `Generated`/`Commit` (concern) or `Module`/`Authored` (scout), `Agent, FledgeVersion, Body`.
- **`nest.Kind`**: `"concern" | "scout"`. `internal/nest/docs.go`'s `ConcernDocs` is the closed set of the 8 concern-doc names plus `index.md`.

## CLI JSON output types (`internal/cli`)

Types serialized for `--json` flags, one per command: `adapterInfo` (agents), `criterionJSON` (criteria), `reportCounts`/`reqCompletion`/`orphanTask`/`blockedTask`/`lockEntry`/`issues`/`report` (colony), `readyTask` (ready), `unfledgedItem`/`unfledgedReport` (unfledged), `graphNode` (vee), `initJSON` (init: `Created[], Skipped[], Updated[], Agents[]`), `nestDoc` (nest, mirrors `internal/nest.Doc`).

## Bootstrap/manifest schema (`internal/bootstrap`)

- **`bootstrap.Manifest`**: `Name, Detector (ManifestDetector), TierPrimitives map[string]string, Files []ManifestFile, PipingFile string`.
- **`bootstrap.ManifestFile`**: `Src, Dst string`, plus policy flags `Generate, PrimitiveMap, Overwrite bool`, `AppendIfMissing, Symlink string` (see `conventions.md` for policy semantics).
- **`bootstrap.ManifestDetector`**: `Exists string` — marker path for harness auto-detection.
- **`bootstrap.primitiveRow`** / **`renderContext`**: template-time structs for rendering the primitive table into generated adapter files (`CommandOrder` here is how `internal/cli`'s `commandOrder` reaches generated allow-lists).

## Repo/scan types (`internal/repo`, `internal/scan`)

- **`repo.Repo`**: resolved git root; exposes `FledgeDir, LocksDir, ContextDir, RequirementsDir, TasksDir, EvidenceDir, Head(), Version()`.
- **`scan.Module`**: `Name, Files[], Count, Bytes` — one row of `fledge scan`'s module grouping. **`scan.Result`**: `Commit, ShortCommit, Modules[]`.
