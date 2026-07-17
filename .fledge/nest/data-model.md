---
generated: 2026-07-17T02:54:09Z
commit: e7a6d4969f861ed3f03af7833b750a7cd703a7a8
agent: fledge-forager
fledge_version: 0.5.8
---

# Data Model

Core data types across the repo: spec files (plumage/feather), the scaffold stamp, and the supporting on-disk record formats (brood locks, roster, ledger). Precision matters here — this document backs planning decisions about spec layout and lifecycle.

## Spec files (`internal/spec`) — the on-disk plumage/feather format

Both spec types share the format: `---\n<YAML frontmatter>\n---\n<body>`, with body bytes preserved byte-for-byte except for the single-byte AC checkbox toggle. Detects both LF and CRLF line endings.

### Requirement (plumage, `internal/spec/types.go:27-38`)

| Field | Type | Notes |
|---|---|---|
| `ID` | string | `PLM-###`, CLI-allocated |
| `Title` | string | user-visible name |
| `Status` | string | one of `ReqEgg`="egg", `ReqHatched`="hatched", `ReqFledged`="fledged" |
| `Priority` | string | one of `P0`,`P1`,`P2`,`P3` |
| `Authored` | string | RFC3339 timestamp |
| `Agent` | string | authoring agent id |
| `FledgeVersion` | string | fledge binary version at creation |
| `Path` | string | file path as loaded; empty if unsaved |
| `Body` | []byte | markdown after the closing `---` fence, byte-preserved |

Frontmatter field render order (7 fields, all required): `id, title, status, priority, authored, agent, fledge_version`.

Plumage body sections (per `internal/spec/templates/plumage.md`): Context, User Stories, Functional Criteria (FC-N), Acceptance Criteria (AC-N checkboxes), Out of Scope, Open Questions.

### Task (feather, `internal/spec/types.go:41-55`)

| Field | Type | Notes |
|---|---|---|
| `ID` | string | `FTHR-###`, CLI-allocated |
| `Title` | string | |
| `Requirement` | string | parent plumage ID; YAML key `plumage` |
| `Status` | string | one of `TaskEgg`="egg", `TaskPipping`="pipping", `TaskHatching`="hatching", `TaskFledged`="fledged" |
| `Priority` | string | `P0`–`P3` |
| `DependsOn` | []string | YAML `depends_on`; other FTHR-### IDs blocking this one; may be empty |
| `Oversight` | string | YAML `oversight`, optional: `""`, `"merge"`, or `"during"` |
| `Authored` | string | RFC3339 |
| `Agent` | string | |
| `FledgeVersion` | string | |
| `Path` | string | |
| `Body` | []byte | byte-preserved |

Frontmatter field render order (9 or 10 fields; `oversight` omitted entirely when empty): `id, title, plumage, status, priority, depends_on, [oversight], authored, agent, fledge_version`.

Feather body sections (per `internal/spec/templates/feather.md`): Description, Affected Modules, Approach, Tests (test names mapped to ACs; test-first), Acceptance Criteria (AC-1 always "tests observed FAIL before, pass after"; evidence recorded in `.fledge/molt/FTHR-###.md`).

### Status lifecycle (authoritative, from `internal/spec/types.go`)

- **Plumage**: `egg` (authoring) → `hatched` (ready for feather decomposition) → `fledged` (all feathers done, closeout verified).
- **Feather**: `egg` (authoring/blocked) → `pipping` (all `depends_on` fledged, not yet claimed) → `hatching` (actively claimed/implemented, brood held) → `fledged` (merged, verified, all AC boxes checked).
- These 4 states (feather) / 3 states (plumage) are the *only* CLI-recognized frontmatter values. Runtime sub-states some orchestration prose mentions (claimed, dispatched, in-review, green-on-main) are never written to frontmatter — orchestrator-only bookkeeping.

### Criterion (`internal/spec/criteria.go:10-18`)

One acceptance-criteria checkbox line, parsed from a case-sensitive `## Acceptance Criteria` heading, pattern `- [xX] AC-N: text` at column 0 (uppercase/lowercase `x` both read; write always lowercase):

| Field | Type | Notes |
|---|---|---|
| `N` | int | numeric part of "AC-N" |
| `Label` | string | full "AC-N" string |
| `Checked` | bool | space vs x/X in box |
| `Text` | string | content after the colon |
| `boxOff` | int | internal byte offset for the toggle |

### Set / FileError (`internal/spec/load.go`)

- `Set`: `Reqs []*Requirement`, `Tasks []*Task`, `Errors []FileError`, `UnknownFields map[string][]string` (unrecognized YAML keys keyed by file path — collected, not fatal).
- `FileError`: `{Path string, Err error}`.

## Spec directory layout

- `.fledge/pluma/plumage/PLM-###-<kebab-title>.md`
- `.fledge/pluma/feathers/FTHR-###-<kebab-title>.md`
- ID allocation is per-directory: `.alloc.lock` flock file lives in each dir so plumage and feather allocation never block each other.

## Nest context docs (`internal/nest`, this document set's own schema)

- `Doc` struct: `Kind` (`Concern` | `Scout`), plus kind-specific fields — Concern: `Generated`, `Commit`; Scout: `Module`, `Authored` — plus shared `Agent`, `FledgeVersion`, `Body []byte`.
- Frontmatter key order fixed per kind: Concern = `generated, commit, agent, fledge_version`; Scout = `module, authored, agent, fledge_version`.
- `ConcernDocs`: 9 known doc names (the 8 concern docs + `index`).
- `StatusResult`: `Complete`, `IndexCommitMatches` bools; `Head`, `IndexCommit` strings; `MissingDocs`, `StubDocs []string`.

## Scaffold stamp (`internal/bootstrap`) — `.fledge/scaffold.json`

- `Stamp` (`stamp.go:20`): `FledgeVersion string`, `Agents []string` (accretive list of every adapter scaffolded so far), `Files map[string]StampEntry`, `DevSource string` (empty when not dev-linked).
- `StampEntry` (`stamp.go:36`): `Policy string`, plus exactly one of `Sha256` (content hash, for copied/generated/overwrite files), `Target` (symlink target), `Lines` (append_if_missing content) — set per the file's write policy.
- Marshaled as 2-space-indented JSON with a trailing newline, deterministic (Go map keys sorted) — `marshalStamp`.
- `DriftStatus` (`drift.go:14`): one of `"up-to-date"`, `"stale"` (unedited, binary moved — refresh-safe), `"modified"` (user-edited, differs from both stamp and expected), `"missing"`, `"obsolete"` (in stamp, no longer shipped).
- `Drift` (`drift.go:33`): `{Path, Status, Policy}`.
- `Manifest` (`registry.go:34`): `Name`, `Detector` (`{Exists string}`), `TierPrimitives map[string]string`, `Files []ManifestFile`, `PipingFile`, `dir` (embed path).
- `ManifestFile` (`registry.go:58`): `Src`, `Dst`, `Generate`, `PrimitiveMap`, `Overwrite`, `AppendIfMissing`, `Symlink` bools — mutually-exclusive policy flags.
- `WriteOpts` (`registry.go:25`): `Refresh bool`, `DevSource string` (absolute path, PLM-031 dev-install mode).

## Brood (feather claim) record (`internal/lock/lock.go`)

- `.fledge/broods/<FTHR-ID>.brood` — JSON file, written atomically via `os.Link` (temp file + hardlink; `EEXIST` on Link gives exclusivity).
- `Record`: `{Task, Owner, PID int, Created, Branch, Worktree string}` — JSON-serializable; `Worktree` field added later, backward-compatible (absent key → empty string on unmarshal).
- `HeldError`: `{Existing Record}` — returned by `Acquire` when already claimed.

## Roster entry (`internal/roster/roster.go`)

- `Entry`: `{Species string, Members []string, Released []bool, Feather string}`.
- `Species`: fixed `[18]string` array of canonical bird-name tokens (adelie, emperor, gentoo, ... northernrockhopper); overflow uses numeric suffixes (`adelie-2`).
- State persisted to a flock-guarded `roster.json` in `.fledge/roster/`.

## Ledger record (`internal/ledger/ledger.go`, new package)

- `Record`: `{Subject, Kind string, Timestamp time.Time (RFC3339), Payload json.RawMessage}`.
- Three kinds, one file per `(subject, kind)` pair — `.fledge/ledger/<subject>.<kind>.json`:
  - `StatusRecord`: `{PID int, Note string, UpdatedAt time.Time}` — liveness lease; stalled if PID dead or `UpdatedAt` older than `StaleAfter` (5 minutes).
  - `VerdictRecord`: `{Result, Note string}`.
  - `EscalationRecord`: `{Message string}`.
- Errors: `NotFoundError{Subject, Kind}`, `CorruptError{Subject, Kind, Err}`, `InvalidSubjectError{Subject, Reason}`.
- Latest-value-only semantics: no history, atomic temp-file+rename writes.

## Scan result (`internal/scan/scan.go`)

- `Module`: `{Name string, Files []string, Count int, Bytes int64}`.
- `Result`: `{Commit, ShortCommit string, Modules []Module}`.
- Modules are grouped by top-level path component; root-level files group under the literal name `"<root>"`.

## Manifest-driven CLI output types (`internal/cli`, selected)

- `adapterInfo` (agents.go): `{Name, Tier, Detector, Scaffolded bool}`.
- `readyTask` (ready.go): `{ID, Title, Priority, Requirement, Oversight, Path}`.
- `graphNode` (vee.go): `{ID, Title, Status, Requirement, DependsOn []string}`.
- `report`/`reportCounts` (colony.go): status counts keyed `egg`/`pipping`/`hatching`/`fledged` in JSON.
- `scaffoldJSONOut`/`scaffoldEntry` (preen.go): `{StampVersion, BinaryVersion string, Files []{Path, Status DriftStatus, Policy string}}`.

## Open Questions

- Whether the ledger's shared `(subject, kind)` namespace is coordinated at the CLI layer to prevent two agents writing semantically-incompatible verdicts for the same subject is not addressed in the `internal/ledger` source (scout-flagged).
