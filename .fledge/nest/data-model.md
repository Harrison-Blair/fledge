---
generated: 2026-07-16T04:02:08Z
commit: 154510fc963e7071b2f09297ecfeba2b6710e85e
agent: fledge-forager
fledge_version: 0.5.8
---

# Data Model

Core types, schemas, and file formats defined across the codebase, with file references.

## Spec types (`internal/spec/types.go`)

- **Requirement** (plumage) — `ID, Title, Status, Priority, Authored, Agent, FledgeVersion string`; `Path string`; `Body []byte` (byte-preserved markdown). Status: `egg | hatched | fledged`.
- **Task** (feather) — `ID, Title, Requirement (plumage ID), Status, Priority, Oversight, Authored, Agent, FledgeVersion string`; `DependsOn []string` (FTHR IDs); `Path, Body` as above. Status: `egg | pipping | hatching | fledged`. `Oversight` optional: `"merge" | "during"`. Priority: `P0..P3`.
- **Set** (`internal/spec/load.go`) — `Reqs []*Requirement`, `Tasks []*Task` (both sorted by filename), `Errors []FileError`, `UnknownFields map[string][]string`. Produced by `Load(reqDir, taskDir)`.
- **Criterion** (`internal/spec/criteria.go`) — `N int, Label string ("AC-N"), Checked bool, Text string, boxOff int` (byte offset for in-place mutation). Parsed by `ParseCriteria()` from `- [x] AC-N: text` lines under a `## Acceptance Criteria` heading.
- **Frontmatter field sets** (fixed, rendered in this exact order):
  - Requirement (7 fields): `id, title, status, priority, authored, agent, fledge_version`.
  - Task (10 fields): `id, title, plumage, status, priority, depends_on, oversight, authored, agent, fledge_version` (`oversight` omitted when empty; `depends_on` renders `[]` when empty).

## CLI-facing types (`internal/cli/*.go`)

- `command` struct (cli.go) — `run func([]string) int`, `usage string`; the registry entry per command.
- `adapterInfo` (agents.go), `initJSON` (init.go), `readyTask` (ready.go), `graphNode` (vee.go), `unfledgedItem`/`unfledgedReport` (unfledged.go), `reportCounts`/`reqCompletion`/`orphanTask`/`blockedTask`/`lockEntry`/`issues`/`report` (colony.go), `criterionJSON` (criteria.go), `githubRelease`/`updateJSON` (update.go), `scaffoldJSONOut`/`scaffoldEntry` (preen.go) — all `--json` output shapes, one struct family per command.

## Bootstrap/scaffold types (`internal/bootstrap/*.go`)

- **Manifest** (registry.go) — `Name string, Detector ManifestDetector, TierPrimitives map[string]string, Files []ManifestFile, PipingFile string`. One per harness adapter (claude, codex, pi).
- **ManifestFile** — `Src, Dst string, Generate, PrimitiveMap, Overwrite bool, AppendIfMissing, Symlink string`. Encodes the write policy for one scaffolded file.
- **Stamp** (stamp.go) — `FledgeVersion string, Agents []string, Files map[string]StampEntry`. Serialized to `.fledge/scaffold.json`.
- **StampEntry** — `Policy, Sha256, Target string, Lines []string` — exactly one of Sha256/Target/Lines is populated, depending on write policy.
- **DriftStatus** (drift.go) — string enum: `up-to-date | stale | modified | missing | obsolete`.
- **Drift** — `Path string, Status DriftStatus, Policy string`.

## Domain-process records (on-disk formats)

- **Brood claim** (`.fledge/broods/FTHR-###.brood`, JSON, `internal/lock.Record`) — `Feather, Owner, PID, Created, Branch, Worktree` (Worktree added in a later feather; legacy records without it still parse for backward compatibility).
- **Roster entry** (`internal/roster.Entry`, in `.fledge/roster/roster.json`) — `Species string, Members []string, Released []bool, Feather string`. 18-species canonical bird list; pairs share one species across 2 members; species frees only once all members released.
- **Scan result** (`internal/scan.Result`) — `Commit, ShortCommit string, Modules []Module`; `Module{Name string, Files []string, Count int, Bytes int64}`. This is the exact shape `fledge scan` emits and what the forager pipeline treats as its authoritative work list.
- **Graph** (`internal/graph.Graph`) — unexported `tasks []*Task`, `byID map[string]*Task`; methods `Cycle() []string`, `Waves() ([][]string, error)`, `Ready() []string`.
- **Finding** (`internal/check.Finding`) — `File, Rule, Severity, Message string`; `Severity` is `"error" | "warning"`.

## Templates (embedded, byte-identical between two locations)

- `internal/spec/templates/{plumage,feather}.md` — skeletons instantiated by `fledge new`.
- `internal/nest/templates/{concern-doc,index,scout-report}.md` and `internal/bootstrap/core/skills/fledge-orchestrate/templates/{context-doc,plumage,feather,scout-report}.md` — the concern-doc/index/scout-report structural templates this very document set follows.

## Open Questions

- Whether `internal/nest/templates/*.md` and `internal/bootstrap/core/skills/fledge-orchestrate/templates/*.md` are kept in sync by convention/review only, or whether one is generated from the other — not resolved by scouts reading both in isolation.
