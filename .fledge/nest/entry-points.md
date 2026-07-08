---
generated: 2026-07-08T01:03:26Z
commit: e44524d1f089dcfe1c1f313f819ec18d9a42eceb
agent: fledge-forager
fledge_version: 0.2.1
---

# Entry Points & Public Interfaces

## Binary entry point

`func main()` at `cmd/fledge/main.go:9` — the sole entry point. Delegates immediately to `cli.Run(os.Args[1:])` and calls `os.Exit` with the returned code.

## Build / install / run

```sh
go build ./...                    # build everything
go build -o fledge ./cmd/fledge   # build the CLI binary
go install ./cmd/fledge           # install to $(go env GOPATH)/bin
go vet ./...
```

After changing CLI or `internal/bootstrap/...` code, reinstall and verify the installed binary matches source before relying on it:

```sh
go install ./cmd/fledge
hash -r
command -v fledge
fledge version                    # must match the VERSION file
```

## CLI commands (`internal/cli`, dispatched from `commandOrder`)

16 commands, each in its own `internal/cli/<name>.go`, registered via `register(name, runFunc, usage)`: `init`, `agents`, `scan`, `new`, `preen`, `ready`, `vee`, `colony`, `unfledged`, `status`, `set`, `criteria`, `brood`, `abandon`, `broods`, `version`. Every command supports `--json`.

- `fledge init [--agent] [--list-agents] [--refresh]` — scaffolds `.fledge/skills/` (core, agent-neutral) + harness adapter(s) into a repo; idempotent, additive; `--refresh` re-syncs fledge-owned files to shipped bytes.
- `fledge scan` — file inventory grouped by top-level module, `.fledgeignore`-filtered, with byte/file counts (used by the forager pipeline as the authoritative scout work-list).
- `fledge new` — allocates a new plumage or feather ID and scaffolds its spec file from an embedded template.
- `fledge status` — reads/transitions a spec's lifecycle status, enforcing legal-transition tables (`internal/cli/status.go`); `--force` bypasses legality but not enum validity.
- `fledge set` — updates frontmatter fields (priority, oversight, depends_on, title); rejects edits to CLI-owned/immutable fields.
- `fledge criteria [check|uncheck]` — lists or toggles acceptance-criteria checkboxes by number or `AC-N` label; the only sanctioned way to check a box.
- `fledge preen` — runs the `internal/check` validation engine over the spec set; supports a strict mode where warnings become blocking.
- `fledge ready` — lists feathers eligible to start (deps met, not brooded).
- `fledge vee [PLM-###]` — dependency graph visualization (waves, cycle detection with path reporting, text/dot/json output).
- `fledge brood` / `fledge abandon` / `fledge broods` — claim, release, and list feather locks (`.fledge/broods/`).
- `fledge colony` — repo-wide progress report: counts, per-requirement completion, blocked-task detail, active locks, degraded-data issues.
- `fledge unfledged [--plumage] [--feathers] [--json]` — lists all non-`fledged` plumage/feathers, priority-then-ID ordered, with type filters.
- `fledge agents` — adapter inventory with derived tier and per-repo scaffolding status.
- `fledge version` — prints the CLI version (compare against `VERSION` file).

## Core spec/domain APIs (Go, `internal/spec`, `internal/check`, `internal/graph`, `internal/lock`, `internal/scan`)

- `spec.Load(reqDir, taskDir) *Set` — parses all specs; `Set.Req(id)`, `Set.Task(id)` lookups; `Set.Errors`, `Set.UnknownFields` surface parse/schema problems.
- `check.Run(set, lockedTasks, evidenceDir) []Finding`, `check.HasErrors(findings) bool` — validation entry points.
- `graph.New(tasks) *Graph` — `Cycle()`, `Waves()`, `Ready()`.
- `lock.Acquire(dir, rec) (*HeldError)`, `lock.Release(dir, task)`, `lock.Get(dir, task)`, `lock.List(dir)`.
- `scan.Run(root) (*Result, error)`.

## Bootstrap/adapter public API (`internal/bootstrap`)

- `LoadAdapters() []*Manifest` — all adapter manifests, sorted by name, skipping `_`-prefixed dirs.
- `FindAdapter(name string) *Manifest`.
- `DetectAdapters(root string) []*Manifest` — auto-detects harnesses present in a repo via each manifest's `Detector.Exists` marker path.
- `Manifest.Provides(p string) bool`, `Manifest.Tier() string` (`"A"|"B"|"C"|""`).
- `Manifest.WriteAdapter(root, commandOrder, refresh)` — scaffolds a harness's files; returns created/updated/skipped classification.
- `WriteCore(root, refresh)` — scaffolds `.fledge/skills/` from the embedded `core/` tree.
- `DeriveTier(provided map[string]bool) string`.
- `CheckDuplicateSkills(root string) error` — guard against pre-existing real (non-symlink) skill copies.
- `CoreSkillNames() []string` — currently `fledge-orchestrate`, `fledge-interrogate`.

## Orchestration entry points (agent-facing, not code)

- `.fledge/skills/fledge-orchestrate/SKILL.md` (scaffolded from `internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md`) — the routing entrypoint agents read first; routes to `planning.md` (new feature/spec work) or `implementation.md` (implementing feathers).
- `.fledge/skills/fledge-interrogate/SKILL.md` — plumage/feature design stress-testing interview script.
- `.claude/fledge-adapter.md` (or the equivalent per-harness adapter file) — the generated primitive map for the current repo's detected harness.

## Open Questions
None observed.
