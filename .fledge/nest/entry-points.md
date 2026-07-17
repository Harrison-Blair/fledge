---
generated: 2026-07-17T07:00:54Z
commit: ee49464adb830bef7189f94a1d3253927d33fb5f
agent: fledge-forager
fledge_version: 0.6.7
---

# Entry Points

Where execution enters this codebase, and how to build/run/install it.

## Binary entrypoint

`cmd/fledge/main.go` — a single-function `func main()` that calls `internal/cli.Run(os.Args[1:])` (`cmd/fledge/main.go`) and exits with the returned process exit code. All CLI behavior is implemented behind `internal/cli.Run`.

## CLI dispatch

`func Run(args []string) int` in `internal/cli/cli.go` — dispatches to one of 25 registered subcommands by name (init, agents, scan, new, nest, preen, ready, vee, colony, unfledged, status, set, criteria, brood, abandon, broods, heartbeat, await, verdict, escalate, ledger, roster, version, update, dev). Each command file registers itself via `init() { register(name, runFunc, usage) }`; `commandOrder` in `cli.go` controls both `--help` ordering and generated allow-lists (e.g. Claude Code's `settings.json` permission list, generated from this order).

Every command supports `--json` for machine-readable output alongside human text.

## Build, test, run (from `CLAUDE.md`, verified against `go.mod`)

```sh
go build ./...                         # build everything
go build -o fledge ./cmd/fledge        # build the CLI binary
go test ./...                          # run all tests
go vet ./...

go test ./cmd/fledge -run TestScripts               # all CLI acceptance tests (37 txtar fixtures)
go test ./cmd/fledge -run TestScripts/init           # one script (init.txtar)
go test ./cmd/fledge -run TestScripts/init -v        # verbose, shows script trace

go test ./internal/spec -run TestAllocateID          # a single unit test
```

Go 1.26.4 (`go.mod`); no Makefile — `go` invoked directly.

## Install / reinstall (dogfooding loop)

```sh
go install ./cmd/fledge        # reinstall to $(go env GOPATH)/bin
hash -r                        # drop shell's cached path to old binary
command -v fledge              # confirm it resolves to the go/bin copy
fledge version                 # must match VERSION in repo root
```

`scripts/install.sh` automates build+install+verify, with an optional `--refresh` flag that also re-syncs `.fledge/skills/` and the `.claude/` adapter (`fledge init --refresh`) after install.

## Scaffold regeneration entrypoint

```sh
fledge init --refresh           # reset fledge-owned files to shipped versions; prunes obsolete ones
git status                      # review what regeneration changed
```

Writes/updates `.fledge/scaffold.json` (the stamp). `fledge preen` reports the scaffold healthy when the stamp is present and consistent.

## Public package APIs (entry points into domain logic, for callers other than the CLI)

- `internal/spec`: `Load(reqDir, taskDir) → Set`, `ParseRequirementFile`/`ParseTaskFile`, `AllocateAndCreate`, `ParseCriteria`/`SetCriterion`, `RequirementBody`/`TaskBody` (template skeletons).
- `internal/check`: `Run(set, lockedTasks, evidenceDir) → []Finding`, `HasErrors(fs) bool`.
- `internal/graph`: `New(tasks) *Graph`, `.Cycle()`, `.Waves()`, `.Ready()`.
- `internal/repo`: `Find() (*Repo, error)`, `Repo.RequireFledge()`, `.Version(fallback)`, `.Head()`, and ~10 `.fledge` path accessors.
- `internal/scan`: `Run(root) (*Result, error)`.
- `internal/ledger`: `Read`/`Write` records for (subject, kind); `ClassifyLiveness`.
- `internal/lock`: `Acquire`/`Release`/`Get`/`List`.
- `internal/nest`: `Status`, `ClearNest`, `RefreshDoc`, `IsStub`, `ConcernBody`/`IndexBody`/`ScoutBody`.
- `internal/roster`: `Assign`/`Release`/`List`.
- `internal/bootstrap`: `LoadAdapters()`, `FindAdapter(name)`, `LoadStamp(root)`, `Stamp.Write(root)`, `WriteCore(root, opts)`, `Manifest.WriteAdapter(root, commandOrder, opts)`, `DriftReport(root, stamp, expected)`, `EditedOnRefresh`, `ExpectedFiles`/`ExpectedFilesDev`, `DeriveTier(provided)`, `ValidateDevSource(path)`.

## Agent-facing entry points (for AI harnesses)

`fledge init` scaffolds one agent-neutral orchestration workflow into whichever harness it detects (Claude Code, pi, Codex) — the entrypoint agents should read first is `.fledge/skills/fledge-orchestrate/SKILL.md` (routing to `planning.md` or `implementation.md`), sourced from `internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md`. The Claude-specific primitive map lives at `.fledge/skills`-adjacent `.claude/fledge-adapter.md` (generated from each adapter's `manifest.yaml`).

## Open Questions

None observed — all entry points are documented consistently across `root`, `cmd`, `internal-cli`, and `internal-bootstrap` scout reports.
