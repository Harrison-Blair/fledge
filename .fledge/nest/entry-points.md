---
generated: 2026-07-16T02:20:48Z
commit: 407b91e70b53764944447dae5829d2076fb852c5
agent: fledge-forager
fledge_version: 0.5.5
---

# Entry Points

Where execution enters fledge, the full command surface, and how to build/run/test it.

## Binary entry point

`cmd/fledge/main.go:main()` — 11 lines, calls `internal/cli.Run(os.Args[1:])` and exits with its return code. All real dispatch logic lives in `internal/cli/cli.go:Run(args []string) int`.

## Build & install

```sh
go build ./...                 # build everything
go build -o fledge ./cmd/fledge   # build the CLI binary
go install ./cmd/fledge        # install to $(go env GOPATH)/bin
```

After changing CLI or `internal/bootstrap` code, reinstall and verify (`hash -r`, `command -v fledge`, `fledge version` must match `VERSION`) — this repo's own `PATH` binary must track source, since the repo dogfoods itself (`scripts/install.sh` automates this: build → install → verify version match → optional `fledge init --refresh`).

## Test

```sh
go test ./...                                       # everything
go test ./cmd/fledge -run TestScripts                # all txtar acceptance tests
go test ./cmd/fledge -run TestScripts/init            # one script (init.txtar)
go test ./cmd/fledge -run TestScripts/init -v         # verbose, shows script trace
go test ./internal/spec -run TestAllocateID           # a package unit test
go vet ./...
```

## The 18 CLI commands (`internal/cli`, dispatched via `commandOrder` in `cli.go`)

`init`, `agents`, `scan`, `new`, `nest` (5 subcommands: new/scaffold/scout/stamp/status), `preen`, `ready`, `vee`, `colony`, `unfledged`, `status`, `set`, `criteria`, `brood`, `abandon`, `broods`, `version`, `update`. Every command accepts `--json`.

- `fledge init [--agent ...] [--refresh] [--force]` — scaffold `.fledge/`, core skills, per-harness adapters; auto-detects harness (`.claude/`, `.pi/`, `.codex/`) or defaults to Claude Code.
- `fledge new plumage|feather` — allocate `PLM-###`/`FTHR-###`, create spec file from template.
- `fledge status <ID> [<new-status>] [--force]` — validate/execute a lifecycle transition.
- `fledge criteria <ID>` / `criteria check|uncheck <ID> <AC-N>` — list/flip acceptance-criteria checkboxes.
- `fledge set <ID> <field> <value>` — mutate frontmatter fields with validation (cycle detection on `depends_on`).
- `fledge brood FTHR-### --owner <name>` / `abandon FTHR-### [--fledged]` / `broods` — claim/release/list feather locks.
- `fledge preen [--strict]` — validate spec set + scaffold drift.
- `fledge vee [--format text|dot|json] [PLM-###]` — dependency-graph waves/cycles, optionally scoped to one plumage.
- `fledge ready [--json]` — list pipping (dispatchable) feathers.
- `fledge unfledged [--plumage] [--feathers]` — non-fledged spec summary.
- `fledge scan [--json]` — module/file/byte inventory (see `modules.md`); this is what the forager pipeline's step 1 consumes.
- `fledge colony [--json]` — high-level completion report (counts, per-plumage %, orphans, blocked).
- `fledge agents [--json]` — list adapter manifests (name/tier/detector/scaffolded).
- `fledge update [--yes] [--json]` — self-update from GitHub releases, checksum-verified atomic binary swap.
- `fledge nest new|scaffold|scout|stamp|status` — the machinery behind this very document set: `scaffold` clears+recreates `.fledge/nest/`, `scout --module <name>` creates one raw report, `status` is the completeness gate (`Complete`, `IndexCommitMatches`, `MissingDocs`, `StubDocs`).

## Exit codes (shared, semantic)

`ExitOK=0`, `ExitFail=1` (domain/validation error), `ExitUsage=2` (CLI misuse), `ExitEnv=3` (not a git repo / missing `.fledge/`) — defined in `internal/cli/cli.go`.

## Local git hooks (optional)

```sh
git config core.hooksPath scripts/hooks
```

Installs `scripts/hooks/pre-commit`, which runs the same `gofmt -l .` + `go vet ./...` gate as CI before allowing a commit. Not installed automatically by `fledge init`.

## Agent-facing entry points (this repo's own dogfooding)

`.fledge/skills/fledge-orchestrate/SKILL.md` is the routing entry point for agents working in *this* repo (or any fledge-managed repo) — it routes feature/spec requests to `planning.md` and implementation requests to `implementation.md`. The Claude-specific primitive map is `.claude/fledge-adapter.md`. Both are generated output of `internal/bootstrap`; edit the Go source, not the scaffolded copies, when changing behavior.
