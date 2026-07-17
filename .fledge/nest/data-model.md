---
generated: 2026-07-17T07:00:54Z
commit: ee49464adb830bef7189f94a1d3253927d33fb5f
agent: fledge-forager
fledge_version: 0.6.7
---

# Data Model

Core data types across the codebase: spec types, bootstrap/scaffold types, ledger/lock/nest/roster state types, and CLI report/envelope shapes.

## Spec types (`internal/spec/types.go`)

- **`Requirement`** (a plumage): `ID`, `Title`, `Status` (`ReqEgg`/`ReqHatched`/`ReqFledged`), `Priority`, `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body` (byte-preserved).
- **`Task`** (a feather): `ID`, `Title`, `Requirement` (parent plumage ref), `Status` (`TaskEgg`/`TaskPipping`/`TaskHatching`/`TaskFledged`), `Priority`, `DependsOn ([]string)`, `Oversight` (optional: `"merge"` | `"during"`), `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body` (byte-preserved).
- **`Criterion`** (`internal/spec/criteria.go`): `N` (int), `Label` (e.g. `"AC-1"`), `Checked` (bool), `Text`, `boxOff` (byte offset of the state character, for single-byte in-place flips).
- **`Set`** (`internal/spec/load.go`): `Reqs []*Requirement`, `Tasks []*Task`, `Errors []FileError` (per-file parse failures, non-fatal), `UnknownFields map[string][]string` (unrecognized frontmatter keys by path).
- Priorities: `P0`/`P1`/`P2`/`P3`.

## Ledger types (`internal/ledger/ledger.go`, new — PLM-030)

- **`Record`**: `{Subject, Kind, Timestamp (RFC3339), Payload (json.RawMessage)}`; `Record.Decode(v)` unmarshals the payload into a kind-specific struct.
- **`StatusRecord`**: `{PID, Note, UpdatedAt}` — heartbeat/liveness.
- **`VerdictRecord`**: `{Result (pass|fail), Note}` — write-once review outcome.
- **`EscalationRecord`**: `{Message}` — write-once blocker signal.
- Error types: `NotFoundError{subject, kind}`, `CorruptError{subject, kind, wrapped err}`, `InvalidSubjectError{subject, reason}` — all typed, assertable via `errors.As`, never panic.
- Kinds: `KindStatus`, `KindVerdict`, `KindEscalation`. Storage path: `.fledge/ledger/<subject>.<kind>.json`. Liveness: `ClassifyLiveness` compares PID + lease age against `StaleAfter` (5-minute TTL).

## Lock types (`internal/lock/lock.go`)

- **`Record`**: `{Task, Owner, PID, Created (RFC3339), Branch, Worktree}` — one per claimed feather, stored as `<task>.brood` under `.fledge/broods/`.
- **`HeldError`**: wraps the existing `Record`, reports the current holder to a caller attempting a conflicting claim.
- Backward-compatible: legacy JSON without a `Worktree` field still parses.

## Nest types (`internal/nest/nest.go`, `docs.go`)

- **`Doc`**: `{Kind (Concern|Scout), Generated/Commit (concern-doc fields), Module/Authored (scout-report fields), Agent, FledgeVersion, Body ([]byte, preserved)}` — frontmatter key order is fixed per `Kind`.
- **`StatusResult`** (backs `fledge nest status`): `{Complete, IndexCommitMatches, Head, IndexCommit, MissingDocs, StubDocs}`.
- 9-document `ConcernDocs` set in fixed order: architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain, index.
- `internal/nest/templates/{concern-doc,index,scout-report}.md` are placeholder *skeletons* stamped into newly-scaffolded files by `fledge nest scaffold`/`scout` — distinct from `.fledge/skills/fledge-orchestrate/templates/*.md`, which are the *authoring conventions* a forager/scout reads to know what to write into those skeletons. Confirmed by diffing both file pairs at synthesis time: `internal/nest/templates/scout-report.md` is a fill-in-the-blank module report skeleton, while `.fledge/skills/.../templates/scout-report.md` (identical content to `internal/bootstrap/core/skills/.../templates/scout-report.md`) is a one-paragraph description of the scout-report contract plus a section-order reference — not itself a skeleton to be filled in.

## Roster types (`internal/roster/roster.go`)

- **`Entry`**: `{Species, Members ([]string), Released ([]bool, per member), Feather}`.
- **`Species`**: fixed `[18]string` canonical list (penguin species — adelie, emperor, gentoo, chinstrap, ... northernrockhopper); sequential allocation, numeric-suffix overflow (`-2`, `-3`, ...) once exhausted.

## Bootstrap/scaffold types (`internal/bootstrap/`)

- **`Manifest`** (per adapter): `Name`, `Detector` (marker file for auto-sense), `TierPrimitives (map[string]string)` (primitive → mechanism), `Files ([]ManifestFile)`, `PipingFile` (optional).
- **`ManifestFile`**: `Src`, `Dst`, `Generate`, `PrimitiveMap`, `Overwrite`, `AppendIfMissing`, `Symlink` — policy fields, mutually describing one of: generate, primitive_map, overwrite, append, symlink, default (skip-if-exists).
- **`Stamp`** (`.fledge/scaffold.json`): `FledgeVersion`, `Agents ([]string)`, `Files (map[string]StampEntry)`, `DevSource` (optional, PLM-031).
- **`StampEntry`**: `Policy`, `Sha256`, `Target` (symlink target), `Lines` (required append lines).
- **`Drift`**: `{Path, Status (DriftStatus), Policy}`. **`DriftStatus`** enum: up-to-date, stale, modified, missing, obsolete.
- **`WriteOpts`**: `Refresh (bool)`, `DevSource (absolute path or empty)`, `SelfLink (bool, PLM-031 FC-3)`.

## CLI report/envelope types (`internal/cli/*.go`)

- **`awaitResult`** / **`awaitEnvelope`** (`await.go`): `{record *ledger.Record, timedOut bool}` / JSON shape `{Record, TimedOut}` with `timed_out` omitted on success.
- **`awaitClock`** (`await.go`): `{now func() time.Time, sleep func(time.Duration)}` — injectable time source for deterministic, sleep-free tests.
- **`reportCounts`, `reqCompletion`, `orphanTask`, `blockedTask`, `report`** (`colony.go`) — `fledge colony` status/inventory shapes.
- **`unfledgedItem`, `unfledgedReport`** (`unfledged.go`) — `fledge unfledged` listing shape.
- **`readyTask`** (`ready.go`) — `fledge ready` dispatch-availability shape.

## Check/graph/scan types (`internal/{check,graph,scan}`)

- **`Finding`** (`check/check.go`): `{File, Rule, Severity ("error"|"warning"), Message}` — the unit of `fledge preen` output.
- **`Graph`** (`graph/graph.go`): tasks slice + `byID` map; methods `Cycle() []string`, `Waves() ([][]string, error)`, `Ready() []string`.
- **`Module`** / **`Result`** (`scan/scan.go`): `{Name, Files ([]string), Count, Bytes}` / `{Commit, ShortCommit, Modules ([]Module)}` — the shape behind `fledge scan --json`, used to plan this very forager's scout split.

## Open Questions

- Whether `DevSource` on `Stamp` is written on every `--dev` invocation or only on refresh (`internal-bootstrap` scout report — not confirmed from files read).
