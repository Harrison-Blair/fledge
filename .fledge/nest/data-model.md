---
generated: 2026-07-15T18:14:39Z
commit: 5728c29953a7c218c923ce20333dbffebb00623f
agent: fledge-forager
fledge_version: 0.5.4
---

# Data Model

The core Go types and on-disk schemas fledge operates over: specs, locks, drift/scaffold state, and CLI JSON output shapes.

## Spec types (`internal/spec/types.go`)

- `Requirement` (PLM-###, "plumage"): `ID, Title, Status, Priority, Authored, Agent, FledgeVersion, Path, Body`. Status enum: `ReqEgg`, `ReqHatched`, `ReqFledged`.
- `Task` (FTHR-###, "feather"): `ID, Title, Requirement (plumage ref), Status, Priority, DependsOn ([]string), Oversight, Authored, Agent, FledgeVersion, Path, Body`. Status enum: `TaskEgg`, `TaskPipping`, `TaskHatching`, `TaskFledged`. `Oversight` ∈ `{"merge", "during"}`, omitted = fully autonomous.
- Priority constants: `P0, P1, P2, P3`.
- `Criterion` (`internal/spec/criteria.go`): `{N, Label, Checked, Text, boxOff}` — `boxOff` is an internal byte offset enabling single-byte checkbox toggles without full-file rewrite.
- `Set` (`internal/spec/load.go`): `{Reqs []Requirement, Tasks []Task, Errors []FileError, UnknownFields map[path][]string}` — the fully-loaded, error-tolerant view of all specs in a repo.
- Frontmatter is canonical YAML with a fixed key order per doc kind, rendered via `goccy/go-yaml`; body is preserved byte-for-byte after the `---\n` fence.

## Lock / brood (`internal/lock/lock.go`)

- `Record`: `{Task, Owner, PID, Created, Branch}` (JSON) — one `.fledge/broods/<FTHR-ID>.brood` file per active claim.
- `HeldError`: wraps an existing `Record` for user-friendly "already held" messaging.
- Acquired via atomic hard-link creation (race-safe); `Lock.List()` skips corrupt `.brood` files rather than failing (open question: resilience choice, not documented in code).

## Context documents (`internal/nest/`)

- `Doc`: `{Kind, Module/Authored (scout) | Generated/Commit (concern), Agent, FledgeVersion, Body}`. `Kind` ∈ `{"concern", "scout"}` — each kind has a distinct frontmatter key order (concern: `generated, commit, agent, fledge_version`; scout: `module, authored, agent, fledge_version`).
- Eight fixed concern-doc names known to `internal/nest/docs.go` (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain) plus `index.md`.
- `RefreshDoc` updates frontmatter only, preserving body — what `fledge nest stamp <file>` calls.

## Scan (`internal/scan/scan.go`)

- `Module`: `{Name, Files []string, Count int, Bytes int}`.
- `Result`: `{Commit, ShortCommit, Modules []Module}` — the shape emitted by `fledge scan --json`, and the authoritative work list a forager plans scout splits against.

## Scaffold / bootstrap types (`internal/bootstrap/`)

- `Manifest` (`registry.go`): `{Name, Detector ManifestDetector, TierPrimitives map[string]string, Files []ManifestFile, PipingFile string, dir}`. Loaded from `adapters/<harness>/manifest.yaml`.
- `ManifestDetector`: `{Exists string}` — repo-relative marker path (e.g. `.claude/`) used to auto-detect an adapter.
- `ManifestFile`: `{Src, Dst, Generate bool, PrimitiveMap bool, Overwrite bool, AppendIfMissing string, Symlink string}` — six write policies encoded as booleans/strings on one struct.
- `Stamp` (`.fledge/scaffold.json`, `stamp.go`): `{FledgeVersion, Agents []string, Files map[string]StampEntry}`.
- `StampEntry`: `{Policy string ("core"|"default"|"generate"|"primitive_map"|"overwrite"|"symlink"|"append"), Sha256, Target (symlinks), Lines []string (append entries)}`.
- `DriftStatus` (`drift.go`): enum `StatusUpToDate | StatusStale | StatusModified | StatusMissing | StatusObsolete`.
- `Drift`: `{Path, Status DriftStatus, Policy string}`.
- Primitive/tier types (`primitives.go`): `PrimitiveOrder []string` (canonical 6), `TierPrimitives map[string][]string` (tier → required primitives), `primitiveTier map[string]string`, `primitiveDesc map[string]string`.
- Template-rendering context (`registry.go`): `renderContext {Adapter, Tier, Rows []primitiveRow, Provided, NotProvided, PipingFile, CommandOrder}`; `primitiveRow {Name, Desc, Mechanism, Provided bool, Tier}` — feeds the generated `fledge-adapter.md` primitive-map table per harness.

## CLI JSON output shapes (`internal/cli/*.go`)

- `initJSON {Created, Skipped, Updated, Agents, Removed}`.
- `adapterInfo {Name, Tier, Detector, Scaffolded}` (`agents` command).
- `criterionJSON {N, Label, Checked, Text}` (`criteria --json`).
- `graphNode {ID, Title, Status, Requirement, DependsOn}` (`vee --json`).
- `readyTask {ID, Title, Priority, Requirement, Oversight, Path}`.
- `reportCounts, reqCompletion, orphanTask, blockedTask, lockEntry, report` (`colony --json`).
- `unfledgedItem, unfledgedReport` (`unfledged --json`).
- `updateJSON {Current, Latest, UpToDate, Notes}` (`update --json` dry-run).
- `command {run, usage}` — internal dispatch-table entry, not user-facing JSON.

## Validation findings (`internal/check/check.go`)

- `Finding`: `{File, Rule, Severity, Message}` (JSON-marshalled). `Severity` ∈ `{"error", "warning"}`. Rules include parse, duplicate-id, schema, dangling-ref, criteria-incomplete, cycles, brood-consistency.

## Dependency graph (`internal/graph/graph.go`)

- `Graph`: wraps a task slice, exposes cycle detection (DFS), `Waves()` (topological layers — same-wave tasks can run in parallel), and `Ready()` (unstarted tasks whose deps are all fledged). Dangling `depends_on` references are tolerated by the graph (skipped for cycle/wave computation) but flagged separately by `check` — open question on whether this split is deliberate.

## Repository (`internal/repo/repo.go`)

- `Repo`: `{Root string}` — git worktree absolute path, discovered via `git rev-parse --show-toplevel`; accessor methods resolve `.fledge`, `broods`, `nest`, `molt`, `pluma/plumage`, `pluma/feathers`, `.fledgeignore`, `.fledge/scaffold.json`.

## Open Questions

- Is `Criterion.boxOff` (byte-offset cache) load-bearing for performance, or would on-demand computation suffice? (internal-domain scout, unresolved from code alone.)
- Whether `graph`'s tolerance of dangling `depends_on` is deliberate defense-in-depth (since `check` catches it separately) or an oversight is unconfirmed.
