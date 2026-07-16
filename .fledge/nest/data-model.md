---
generated: 2026-07-16T02:20:48Z
commit: 407b91e70b53764944447dae5829d2076fb852c5
agent: fledge-forager
fledge_version: 0.5.5
---

# Data Model

Core types and on-disk schemas: the spec structs, lock/brood records, scaffold stamp, drift classification, and nest doc schemas.

## Spec types (`internal/spec/types.go`)

- **`Requirement`** (a "plumage") — `ID` (`PLM-###`), `Title`, `Status` (`egg`→`hatched`→`fledged`), `Priority`, `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body []byte` (markdown, byte-preserved).
- **`Task`** (a "feather") — `ID` (`FTHR-###`), `Title`, `Requirement` (plumage-ID link), `Status` (`egg`→`pipping`→`hatching`→`fledged`), `Priority`, `DependsOn []string` (FTHR-### IDs, unvalidated at parse time — cycle/dangling-ref checks live in `internal/check` and `internal/graph`), `Oversight` (optional: `"merge"` | `"during"`, absent = none), `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body`.
- **`Set`** (`internal/spec/load.go`) — `Reqs []*Requirement`, `Tasks []*Task`, `Errors []FileError`, `UnknownFields map[string][]string`. Produced by `Load(reqDir, taskDir)`; `Req(id)`/`Task(id)` do an O(n) linear lookup.
- **`FileError`** — `{Path, Err}`, one per file that failed to parse; parse errors surface as data, not abort.
- **`Criterion`** (`internal/spec/criteria.go`) — `{N int, Label string ("AC-1"), Checked bool, Text string, boxOff int}` (byte offset used for single-byte mutation); parsed from `- [ ] AC-N: text` lines under a `## Acceptance Criteria` heading.

## Lock/brood types (`internal/lock/lock.go`)

- **`Record`** — `{Task, Owner, PID, Created, Branch}`, JSON-marshaled to `.fledge/broods/<FTHR-ID>.brood`.
- **`HeldError`** — wraps an existing `Record`; returned by `Acquire()` on conflict; formats owner + created timestamp.

## Check/validation types (`internal/check/check.go`)

- **`Finding`** — `{File, Rule, Severity, Message}`.
- **`Severity`** — string enum `"error"` | `"warning"`.
- Rule names are kebab-case: `parse`, `unknown-field`, `duplicate-id`, `dangling-ref`, `unhatched-plumage`, `cycle`, `tests-section`, `required-sections`, `stale-pipping-hint`, `brood-consistency`, `criteria-incomplete`, `criteria-format`, `criteria-evidence`, `id-filename`, `schema`.

## Graph types (`internal/graph/graph.go`)

- **`Graph`** — private state built over `[]*spec.Task` via each task's `DependsOn`. Methods: `Cycle() []string`, `Waves() ([][]string, error)` (topological levels for parallel dispatch), `Ready() []string` (egg/pipping tasks whose deps are all fledged, dangling deps tolerated as never-done).

## Scaffold/bootstrap types (`internal/bootstrap`)

- **`Manifest`** — `{name, detector (ManifestDetector{Exists}), TierPrimitives map[string][]string, Files []ManifestFile, PipingFile, dir}` — one per harness, loaded from `manifest.yaml`.
- **`ManifestFile`** — `{Src, Dst, Generate, PrimitiveMap, Overwrite, AppendIfMissing, Symlink}` — encodes which of the 6 write policies applies to a given scaffolded file.
- **`Stamp`** (`.fledge/scaffold.json`) — `{FledgeVersion, Agents []string, Files map[string]StampEntry}`.
- **`StampEntry`** — `{Policy, Sha256, Target (symlink dest), Lines (append content)}`.
- **`Drift`** — `{Path, Status (DriftStatus), Policy}`.
- **`DriftStatus`** — string enum: `StatusUpToDate`, `StatusStale`, `StatusModified`, `StatusMissing`, `StatusObsolete`.
- **`renderContext`/`primitiveRow`** — internal template-rendering context for generated files (adapter, tier, per-primitive provided/not-provided rows).

## Nest/context-doc types (`internal/nest`)

- **`Doc`** — `{Kind, Generated, Commit, Module, Authored, Agent, FledgeVersion, Body}` — the schema behind every file in `.fledge/nest/`, including this one.
- **`Kind`** — string enum: `"concern"` | `"scout"` (concern docs use `generated`/`commit` frontmatter; scout reports use `module`/`authored`).
- **`StatusResult`** — `{Complete, IndexCommitMatches, Head, IndexCommit, MissingDocs, StubDocs}` — what `fledge nest status` (the gate this forager run must pass) actually computes.

## Repo/scan types

- **`repo.Repo`** — `{Root}`; accessor methods for `FledgeDir`, `RequirementsDir`, `TasksDir`, `LocksDir`, `EvidenceDir`, `ContextDir`, `Head()` (full HEAD SHA), `Version(fallback)`.
- **`scan.Module`** — `{Name, Files, Count, Bytes}`.
- **`scan.Result`** — `{Commit, ShortCommit, Modules}` — what `fledge scan` emits and this forager's step 1 treats as the authoritative work list.

## CLI output types (`internal/cli`)

- **`command`** — `{run func, usage string}`, keyed by name in the `commands` map.
- Per-command output structs used for `--json`: `initJSON` (created/skipped/updated/agents/removed), `updateJSON` (current/latest/upToDate/notes), `readyTask` (id/title/priority/plumage/oversight/path), `unfledgedReport` (plumage/feathers/issues), `report` (colony: counts/requirements/orphans/blocked/locks/issues), `adapterInfo` (name/tier/detector/scaffolded), `graphNode` (id/title/status/plumage/depends_on), `lockOut` (Record + `pidAlive bool`).
- **`githubRelease`** — `{tag_name, body, assets[]}`, decoded from the GitHub releases API for `fledge update`.
