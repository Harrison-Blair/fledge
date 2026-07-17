---
generated: 2026-07-17T17:48:26Z
commit: 1c9011d6e6a06f72f96bc98e3b2bd99c408ab79e
agent: fledge-forager
fledge_version: 0.6.10
---

# Data Model

All persisted and in-memory data types across the CLI, spec files, ledger, and scaffold system.

## Spec types (`internal/spec/types.go`)

- **`Requirement`** (plumage, PLM-###): `ID`, `Title`, `Status`, `Priority`, `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body ([]byte)`. Status: `ReqEgg` | `ReqHatched` | `ReqFledged`. Methods: `Frontmatter()`, `Render()`, `Save()`.
- **`Task`** (feather, FTHR-###): `ID`, `Title`, `Requirement` (plumage link), `Status`, `Priority`, `DependsOn ([]string)`, `Oversight` (optional), `Authored`, `Agent`, `FledgeVersion`, `Path`, `Body`. Status: `TaskEgg` | `TaskPipping` | `TaskHatching` | `TaskFledged`. Same method set.
- **`Set`**: `Reqs []*Requirement`, `Tasks []*Task`, `Errors []FileError`, `UnknownFields map[string][]string`. Methods: `Req(id)`, `Task(id)`.
- **`Criterion`**: `N` (AC number), `Label` (e.g. "AC-3"), `Checked bool`, `Text`, plus an unexported `boxOff` byte offset used for single-byte mutation.
- **`FileError`**: `Path`, `Err`.
- Priority levels: `P0`–`P3`. Oversight values: `"merge"` | `"during"` (feathers only, optional).

## Spec body structure (prose, not Go types)

- **Plumage sections**: Context (WHY), User Stories (As-a/I-want/so-that), Functional Criteria (`FC-N`, testable), Acceptance Criteria (`AC-N` checkboxes, unchecked at authoring), Out of Scope, Open Questions.
- **Feather sections**: Description (WHAT), Affected Modules (files + context citations), Approach (HOW), Tests (test-first list mapped to AC), Acceptance Criteria (`AC-1`…`AC-N`; AC-1 always "tests observed failing before implementation, passing after").
- Templates: `internal/spec/templates/{plumage,feather}.md`; documented for authors in `core/skills/fledge-orchestrate/templates/{plumage,feather}.md`.

## Frontmatter fields

Common to both: `id`, `title`, `status`, `priority`, `authored` (UTC ISO-8601), `agent` (e.g. `"fledge-orchestrate/planning"`), `fledge_version`. Feather-only: `plumage` (links to PLM-###), `depends_on` (list of FTHR-###), `oversight` (optional).

`YAMLScalar()` (internal/spec/frontmatter.go) canonically quotes scalars (booleans, numeric strings, empty strings, unsafe chars) idempotently across parse/render cycles.

## Ledger records (`internal/ledger/ledger.go`)

- **`Record`**: `Subject`, `Kind`, `Timestamp`, `Payload any`. Kinds: `KindStatus`, `KindVerdict`, `KindEscalation`.
- **`StatusRecord`**: `Note`, `Expect`, `UpdatedAt` — repeatedly written; heartbeat + liveness + terminal "done" signal all ride the `Note` field.
- **`VerdictRecord`**: `Result` (pass/fail), `Note` — write-once per feather.
- **`EscalationRecord`**: `Message` — write-once, blocker text from worker to orchestrator.
- Error types: `NotFoundError`, `CorruptError`, `InvalidSubjectError`.
- `StaleAfter = 5 * time.Minute` — TTL constant used by `ClassifyLiveness(lastUpdated, expect, now) (bool, string)`.
- Stored as JSON under `.fledge/ledger/<subject>.<kind>.json`, atomic write via temp-file + `os.Rename`.

## Lock / brood records (`internal/lock/lock.go`)

- **`Record`**: `Task`, `Owner`, `Created`, `Branch`, `Worktree`. Error type: `HeldError`.
- Stored as `.fledge/broods/FTHR-###.brood` (JSON), created via `os.Link` for exclusive-creation semantics. `List()` skips and separately reports corrupt brood files.

## Roster (`internal/roster/roster.go`)

- **`Entry`**: `Species`, `Members`, `Released`, `Feather`.
- `Species` constant: 18 canonical penguin names (adelie, emperor, gentoo, … northernrockhopper), stored at `.fledge/roster/roster.json`. Pair release frees only when both brooder+skua release; overflow issues numeric suffixes.

## Nest / context-doc types (`internal/nest/nest.go`, `internal/nest/docs.go`)

- **`Doc`**: `Kind` (`Concern` | `Scout`), `Generated`, `Commit`, `Module`, `Authored`, `Agent`, `FledgeVersion`, `Body`. Methods: `Frontmatter()`, `Render()`.
- **`StatusResult`** (JSON): `Complete`, `IndexCommitMatches`, `Head`, `IndexCommit`, `MissingDocs`, `StubDocs` — what `fledge nest status` reports and this pipeline gates on.
- **`ConcernDocs`**: 9-entry string array (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain, index) — the canonical doc set this very file set belongs to.

## Scaffold / stamp types (`internal/bootstrap/stamp.go`, `registry.go`, `primitives.go`)

- **`Manifest`**: adapter's single source of truth — name, detector marker, `TierPrimitives map[string]string` (primitive→mechanism), file list, piping file, native skills dir, embed-FS dir.
- **`ManifestFile`**: source→target with write policy (`generate`, `primitive_map`, `overwrite`, `append_if_missing`, `symlink`, default).
- **`Stamp`**: fledge version, agents list, `map[string]StampEntry`, optional dev-source path — serialized as `.fledge/scaffold.json`.
- **`StampEntry`**: policy label + exactly one of `Sha256` (content hash), `Target` (symlink target), `Lines` (append lines).
- **`Drift`**: path, `DriftStatus` (`up-to-date` | `stale` | `modified` | `missing` | `obsolete`), policy.
- **`WriteOpts`**: `Refresh bool`, `DevSource string`, `SelfLink bool`.

## Evidence file (`.fledge/molt/FTHR-###.md`)

One `## AC-N` markdown section per acceptance criterion; each holds the commands run and verbatim captured output proving that criterion. AC-1's section must contain pre-implementation failing-test output (test-first proof). Written incrementally by the brooder, audited section-by-section by the paired skua before any box is checked.

## CLI-side output structs (`internal/cli/*.go`, JSON via `--json`)

`graphNode` (vee.go: id, title, status, plumage, depends_on), `readyTask` (ready.go), `unfledgedItem` (unfledged.go), `adapterInfo` (agents.go: name, tier, detector, scaffolded), `criterionJSON` (criteria.go), `pulseReport` (pulse.go: name, stalled, reason, expect, elapsed), `awaitResult` (await.go: record, timedOut), `scaffoldEntry` (preen.go, wraps bootstrap.Drift), plus nested `report`/`reportCounts`/`orphanTask`/`blockedTask`/`lockEntry`/`issues` structs for `colony.go`'s aggregated report.

## Scratch digests (transient, not versioned data types — markdown files)

`.fledge/scratch/digest-planning.md`, `digest-implementation.md`, `digest-foraging.md` — phase-close summaries written by the orchestrating agent, read by whoever resumes.
