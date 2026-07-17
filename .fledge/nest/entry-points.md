---
generated: 2026-07-17T02:54:09Z
commit: e7a6d4969f861ed3f03af7833b750a7cd703a7a8
agent: fledge-forager
fledge_version: 0.5.8
---

# Entry Points

How to build, run, and invoke fledge, and the full surface of its CLI commands and public package interfaces.

## Build & install

```sh
go build ./...                    # build everything
go build -o fledge ./cmd/fledge   # build the CLI binary to repo root
go install ./cmd/fledge           # install to $(go env GOPATH)/bin
```

Binary entry point: `cmd/fledge/main.go:main()` → `internal/cli.Run(os.Args[1:]) int` → `os.Exit(code)`. No logic lives in `cmd/fledge` itself.

`scripts/install.sh` wraps `go install` with `-ldflags` version injection, PATH verification, and an optional `--refresh` to re-sync `.fledge/skills/` and the harness adapter.

## CLI command surface (19 commands, `internal/cli/cli.go` dispatch)

All support `--json`. Exit codes: `0` ExitOK, `1` ExitFail, `2` ExitUsage, `3` ExitEnv.

| Command | Purpose | Key flags |
|---|---|---|
| `fledge init` | Scaffold `.fledge/skills/` + harness adapter into the repo | `--agent <name>...`, `--dev[=<path>]`, `--refresh`, `--force`, `--list-agents` |
| `fledge agents` | List available adapters, tier, detector, scaffolded status | |
| `fledge scan` | Enumerate repo modules + file lists/sizes (authoritative work list for foraging) | |
| `fledge new plumage\|feather` | Allocate ID + create spec file | `--title`, `--priority`, `--plumage` (feather), `--depends-on`, `--oversight`, `--force` |
| `fledge status <ID> [<new-status>]` | Get/set spec status with legal-transition validation | `--force` |
| `fledge set <ID> <field> <value>` | Update a mutable frontmatter field | |
| `fledge criteria <ID>` / `check\|uncheck <ID> <AC-N>` | List/toggle acceptance criteria | |
| `fledge preen` | Validate specs + scaffold drift | `--strict` |
| `fledge vee [PLM-###]` | Dependency graph (waves, cycles) | `--format text\|dot\|json` |
| `fledge ready` | List dispatchable (pipping, unlocked) feathers | |
| `fledge colony` | Full status report: counts, orphans, blocked, locks, parse errors | |
| `fledge unfledged` | List unfinished specs | `--plumage`, `--feathers` |
| `fledge brood FTHR-### --owner <name>` | Acquire a feather claim, flip status to hatching | `--branch`, `--worktree` |
| `fledge abandon FTHR-###` | Release a claim | `--fledged`, `--force` |
| `fledge broods` | List active claims | `--stale` |
| `fledge nest new\|scaffold\|scout\|stamp\|status` | Manage `.fledge/nest/` context docs | `--module <name>` (scout) |
| `fledge roster [assign\|release\|list]` | Worker species-name allocation | `--feather`, `--pair`, `--for <purpose>` |
| `fledge heartbeat <name>` | Record agent liveness/status to ledger | `--note <text>` |
| `fledge version` | Print binary version | |
| `fledge update` | Self-update from GitHub Releases | `--yes` |

## Nest command detail (context regeneration — the process that produced this document)

- `fledge scan` — modules + file lists, input to the forager's scout-split plan
- `fledge nest scaffold` — clear + recreate `.fledge/nest/` (all `.md` files + `raw/`), stamping frontmatter to HEAD
- `fledge nest scout --module <name>` — create `.fledge/nest/raw/<module>.md` with correct frontmatter + section skeleton for a scout to fill
- `fledge nest status [--json]` — authoritative completeness check: all 8 concern docs present and non-stub, plus `index.md` stamped to current HEAD

## Programmatic entry points (selected public package APIs)

- `internal/spec.Load(reqDir, taskDir string) (*Set, error)` — batch-load all specs
- `internal/spec.AllocateAndCreate(dir, prefix string, build func(id string) (path, content []byte)) (id, path string, err error)` — race-safe ID allocation
- `internal/check.Run(set *spec.Set, lockedTasks []string, evidenceDir string) []Finding` — validation
- `internal/graph.New(tasks []*spec.Task) *Graph` — dependency graph; `.Cycle()`, `.Waves()`, `.Ready()`
- `internal/lock.Acquire/Release/Get/List(dir, ...)` — brood claim management
- `internal/repo.Find() (*Repo, error)` — locate git root + `.fledge/` paths
- `internal/bootstrap.LoadAdapters() []*Manifest`, `Manifest.WriteAdapter(root, commandOrder, opts)`, `bootstrap.WriteCore(root, opts)` — scaffolding
- `internal/nest.Status(contextDir, head string) StatusResult` — nest completeness check (backs `fledge nest status`)

## CI entry points

- `.github/workflows/pr-check.yml` — on PR to main: `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`
- `.github/workflows/release.yml` — on push to main: safety-net (same lint/build/test) → detect-version (diff `VERSION` against prior commit) → conditionally build+release 4 platform binaries with version-injected `-ldflags`

## Optional local git hook

```sh
git config core.hooksPath scripts/hooks
```
Enables `scripts/hooks/pre-commit`, mirroring CI's `gofmt -l .` + `go vet ./...` before every local commit.
