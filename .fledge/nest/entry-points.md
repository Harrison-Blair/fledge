---
generated: 2026-07-15T18:14:39Z
commit: 5728c29953a7c218c923ce20333dbffebb00623f
agent: fledge-forager
fledge_version: 0.5.4
---

# Entry Points

How to build, run, and invoke fledge, plus its public CLI/Go-API surfaces.

## Build & install

```sh
go build ./...                    # build everything
go build -o fledge ./cmd/fledge   # build the CLI binary
go install ./cmd/fledge           # install to $(go env GOPATH)/bin
scripts/install.sh                # build+install+verify (optional --refresh)
```

After changing CLI or `internal/bootstrap/...` code: `go install ./cmd/fledge && hash -r && command -v fledge && fledge version` — verify the installed binary's version matches the `VERSION` file (root scout, CLAUDE.md). If `internal/bootstrap/core/`/`adapters/` content changed, also run `fledge init --refresh` in this repo and review `git status` (this repo dogfoods itself).

## Binary entry point

`cmd/fledge/main.go` — 12 lines: `os.Exit(cli.Run(os.Args[1:]))`. All real logic lives in `internal/cli.Run`.

## CLI commands (`internal/cli/cli.go` dispatch table)

16 subcommands, every one supporting `--json`: `init`/`agents`, `scan`, `new`, `preen`, `ready`, `vee`, `colony`, `status`/`set`/`criteria`, `brood`/`abandon`/`broods`, `version`, `update`, plus `nest` (sub-verbs: `scaffold`, `scout --module <name>`, `new`, `stamp <file>`). Exit codes: `ExitOK=0, ExitFail=1, ExitUsage=2, ExitEnv=3`.

Key commands for context/spec work:
- `fledge scan [--json]` — authoritative module/file inventory (filtered by `.fledgeignore`); the forager's step-1 work list.
- `fledge nest scaffold` — clears and recreates `.fledge/nest/` (including `raw/`) with stamped empty stubs.
- `fledge nest scout --module <name>` — creates `.fledge/nest/raw/<module>.md` with correct frontmatter for a scout to fill.
- `fledge nest stamp <file>` — refreshes a nest doc's frontmatter (generated/commit/authored) while preserving its body.
- `fledge new plumage|feather ...`, `fledge status`, `fledge set`, `fledge criteria check|uncheck` — the only sanctioned way to allocate IDs and mutate spec frontmatter/state.
- `fledge preen [--strict]` — validates specs (schema, dangling refs, criteria completeness) and scaffold drift together.
- `fledge init [--refresh] [--force] [--agent <name>]` — scaffolds `.fledge/skills/`, per-harness adapter files, and `.fledge/scaffold.json`.
- `fledge agents` — lists adapter detectors, derived tiers, scaffolded status.

## Testing entry points

```sh
go test ./...                                        # everything
go test ./cmd/fledge -run TestScripts                 # all txtar acceptance tests
go test ./cmd/fledge -run TestScripts/init -v          # one script, verbose trace
go test ./internal/spec -run TestAllocateID            # a single unit test
go vet ./...
```

## Orchestration entry point (agent-facing, not CLI)

`.fledge/skills/fledge-orchestrate/SKILL.md` is the entry point agents load first — it routes to `planning.md` (new feature/spec/breakdown requests) or `implementation.md` (implement/run-feathers requests). The Claude-specific primitive map is `.claude/fledge-adapter.md` (generated, points to `.claude/team-loop.md` for Claude runtime specifics). This is not invoked by a CLI command — it's loaded by the agent's own skill-loading mechanism (symlinked into `.claude/skills/fledge-orchestrate` on Claude, referenced by pointer on Pi, auto-loaded via `AGENTS.md` on Codex).

## Public Go API (internal, consumed by `cmd/fledge` and `internal/cli` only — not an external package)

- `internal/bootstrap`: `LoadAdapters()`, `FindAdapter(name)`, `DetectAdapters(root)`, `Manifest.Tier()`, `Manifest.WriteAdapter(root, commandOrder, opts)`, `WriteCore(root, opts)`, `PruneObsolete(...)`, `ExpectedFiles(...)`, `DriftReport(...)`, `EditedOnRefresh(...)`, `DeriveTier(provided)`, `CheckDuplicateSkills(root)`.
- `internal/spec`: `Load`, `ParseRequirementFile`, `ParseTaskFile`, `NextID`, `AllocateAndCreate`, `SetCriterion`, `ParseCriteria`, `WriteFileAtomic`.
- `internal/check`: `Run(set) []Finding`.
- `internal/graph`: `New(tasks)`, `.Cycle()`, `.Waves()`, `.Ready()`.
- `internal/lock`: `Acquire`, `Release`, `Get`, `List`.
- `internal/nest`: `Doc`, `ConcernBody`, `IndexBody`, `ScoutBody`, `RefreshDoc`, `ClearNest`, `ConcernDocs`, `IsKnownDoc`.
- `internal/repo`: `Find()`, `Repo` accessor methods.
- `internal/scan`: `Run()`.

## Open Questions

None observed.
