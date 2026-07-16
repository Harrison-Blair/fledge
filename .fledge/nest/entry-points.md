---
generated: 2026-07-15T23:53:12Z
commit: a4d02e8187c64ef9f3f1231052990b282207420b
agent: fledge-forager
fledge_version: 0.5.5
---

# Entry Points

Every way execution enters this codebase: the CLI command surface, the binary entry point, and how to build/run/install it.

## Binary entry point

- `cmd/fledge/main.go`: `main()` calls `cli.Run(os.Args[1:])` and propagates the returned exit code via `os.Exit()`. All logic is delegated to `internal/cli`.
- `internal/cli/cli.go`: `Run(args []string) int` is the CLI's own entry point — dispatches `args[0]` to a registered command handler via a `commandOrder`-ordered table built by each command file's `init()` → `register(name, run, usage)`.

## Full command table (24 commands, `internal/cli`)

| Command | Purpose | Implemented in |
|---|---|---|
| `fledge init [--agent]... [--refresh] [--force] [--list-agents] [--json]` | Scaffold `.fledge/` + agent adapters; detect harness; `--refresh` resets to shipped versions and prunes stale files | `init.go` |
| `fledge agents [--json]` | List available adapters (name, tier, detector, scaffolded status) | `agents.go` |
| `fledge scan [--json]` | Enumerate repo modules (top-level dirs), file lists, byte counts, `.fledgeignore`-filtered | `scan.go` |
| `fledge new plumage --title <t> [--priority] [--agent] [--json]` | Allocate a PLM-### and create it from template | `new.go` |
| `fledge new feather --title <t> --plumage PLM-### [--depends-on] [--priority] [--oversight] [--force] [--json]` | Allocate a FTHR-### and create it from template | `new.go` |
| `fledge nest new <doc> [--agent] [--force] [--json]` | Create a single `.fledge/nest/` concern doc | `nest.go` |
| `fledge nest scaffold [--agent] [--json]` | Clear and recreate all of `.fledge/nest/` (concern docs + `raw/`) | `nest.go` |
| `fledge nest scout --module <m> [--agent] [--force] [--json]` | Create `.fledge/nest/raw/<module>.md` scout report skeleton | `nest.go` |
| `fledge nest stamp <file> [--kind] [--agent] [--json]` | Refresh a nest doc's frontmatter (commit, timestamp) | `nest.go` |
| `fledge nest status [--json]` | Report whether nest synthesis is complete (all docs present, non-stub, index at HEAD) | `nest.go` |
| `fledge preen [--strict] [--json]` | Validate specs (`internal/check`) + scaffold drift; report findings | `preen.go` |
| `fledge ready [--json]` | List `pipping` feathers with no lock held, sorted by priority | `ready.go` |
| `fledge vee [--format text\|dot\|json] [--json] [PLM-###]` | Render dependency graph; detect cycles | `vee.go` |
| `fledge colony [--json]` | Aggregate project status: counts by lifecycle, per-plumage completion, orphans, unmet deps, active broods | `colony.go` |
| `fledge unfledged [--plumage] [--feathers] [--json]` | List incomplete plumages/feathers, priority-then-ID order | `unfledged.go` |
| `fledge status <ID> [<new-status>] [--force] [--json]` | Query or transition spec status; enforces legal transitions | `status.go` |
| `fledge set <ID> <field> <value> [--json]` | Mutate priority/oversight/depends_on/title; cycle-checked | `set.go` |
| `fledge criteria <ID> [--json]` | List acceptance-criteria checkboxes | `criteria.go` |
| `fledge criteria check\|uncheck <ID> <AC-N> [--json]` | Toggle a single acceptance-criterion checkbox | `criteria.go` |
| `fledge brood FTHR-### --owner <name> [--branch] [--json]` | Acquire a feather claim lock, transition to `hatching` | `brood.go` |
| `fledge abandon FTHR-### [--fledged] [--force] [--json]` | Release a claim lock, optionally transition to `fledged` | `brood.go` |
| `fledge broods [--json]` | List all currently held locks | `brood.go` |
| `fledge version [--json]` | Print the binary version (build-time `-ldflags`) | `version.go` |
| `fledge update [--yes] [--json]` | Check GitHub for a newer release, prompt, download and swap the binary | `update.go` |

Every command supports `--json` (`emitJSON()` helper) and shares the exit-code scheme `ExitOK(0)/Fail(1)/Usage(2)/Env(3)`.

## Package-level public APIs (called by `internal/cli`, not directly by users)

- **`internal/spec`**: `ParseRequirementFile`, `ParseTaskFile`, `Load`, `NextID`, `AllocateAndCreate`, `SplitFrontmatter`, `(*Requirement).Save/Render/Frontmatter`, `(*Task).Save/Render/Frontmatter`, `ParseCriteria`, `SetCriterion`, `RequirementBody`, `TaskBody`, `Kebab`, `YAMLScalar`.
- **`internal/check`**: `Run(set, lockedTasks, evidenceDir) → []Finding`; `Finding.HasErrors()`.
- **`internal/graph`**: `New`, `(*Graph).Cycle()`, `(*Graph).Waves()`, `(*Graph).Ready()`.
- **`internal/lock`**: `Acquire`, `Release`, `Get`, `List` (all backing `fledge brood`/`abandon`/`broods`).
- **`internal/repo`**: `Find()`, `(*Repo).RequireFledge()`, `.FledgeDir()`, `.LocksDir()`, `.ContextDir()`, `.ScanIgnorePath()`, `.EvidenceDir()`, `.RequirementsDir()`, `.TasksDir()`, `.Version()`, `.Head()`.
- **`internal/scan`**: `Run(root) → *Result`.
- **`internal/nest`**: `Status(contextDir, head)`, `RefreshDoc(...)`, `ClearNest(contextDir)`, `ConcernBody/IndexBody/ScoutBody`, `IsStub`, `IsKnownDoc`, `Title`.
- **`internal/bootstrap`**: `LoadAdapters()`, `FindAdapter(name)`, `WriteCore(root, opts)`, `(*Manifest).WriteAdapter(root, commandOrder, opts)`, `(*Manifest).Provides(p)`, `(*Manifest).Tier()`, `(*Manifest).Scaffolded(root)`, `CheckDuplicateSkills(root)`, `LoadStamp(root)`, `(*Stamp).Write(root)`, `ExpectedFiles(m, commandOrder)`, `DriftReport(root, stamp, expected)`, `EditedOnRefresh(...)`, `DeriveTier(provided)`, `PruneObsolete(...)`.

## Skill/workflow entry points (agent-facing, not CLI)

- `.fledge/skills/fledge-orchestrate/SKILL.md` — main router; the file every agent-harness is told to load first (`> fledge: load and follow .fledge/skills/fledge-orchestrate/SKILL.md`, per this repo's own `CLAUDE.md`).
- `.fledge/skills/fledge-interrogate/SKILL.md` — standalone interrogation skill, triggered directly or from within planning.
- Phase entry points inside `fledge-orchestrate`: `planning.md` (feature-request handling, spec drafting), `implementation.md` (feather dispatch, review-gate, merge), `foraging.md` (this protocol — context regeneration).

## Build, test, run

```sh
go build ./...                              # build everything
go build -o fledge ./cmd/fledge             # build the CLI binary
go test ./...                               # run all unit + package tests
go vet ./...
go install ./cmd/fledge                     # install to $(go env GOPATH)/bin

go test ./cmd/fledge -run TestScripts       # all CLI acceptance (.txtar) tests
go test ./cmd/fledge -run TestScripts/init  # one script
go test ./cmd/fledge -run TestScripts/init -v  # verbose, shows script trace

go test ./internal/spec -run TestAllocateID # a single unit test
```

After changing embedded `core/`/`adapters/` content, regenerate this repo's own scaffold and review the diff:

```sh
fledge init --refresh
git status
```

Local git hooks (opt-in, one-time per clone): `git config core.hooksPath scripts/hooks`.

## Open Questions

None observed — the CLI command surface and build/run instructions were consistently confirmed across `cmd.md`, `cli.md`, and `root.md` scout reports and this repo's own `CLAUDE.md`.
