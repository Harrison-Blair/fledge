---
generated: 2026-07-16T04:02:08Z
commit: 154510fc963e7071b2f09297ecfeba2b6710e85e
agent: fledge-forager
fledge_version: 0.5.8
---

# Entry Points

Where execution enters this codebase, and how to build/run/test it.

## Binary entry point

- **`cmd/fledge/main.go`** — sole `func main()`. Takes `os.Args[1:]`, calls `internal/cli.Run(args)`, exits with the returned int. No logic lives here; it exists solely to satisfy Go's `package main` requirement.
- **`internal/cli.Run(args []string) int`** (`internal/cli/cli.go`) — the real dispatcher. Parses `args[0]` as a command name, looks it up in a registry populated by each command file's `init()` → `register(name, run, usage)`, invokes the handler, returns its exit code.

## CLI commands (19 total)

Exact count and order verified via `awk '/commandOrder = /,/^}/' internal/cli/cli.go`:

```
init, agents, scan, new, nest, preen, ready, vee, colony,
unfledged, status, set, criteria, brood, abandon, broods, roster, version, update
```

| Command | File | Purpose |
|---|---|---|
| `init` | init.go | Scaffold/refresh `.fledge/` + harness adapter into a repo. |
| `agents` | agents.go | List available/detected agent harness adapters, tiers. |
| `scan` | scan.go | Discover modules + file/byte counts (context-gathering work list). |
| `new` | new.go | Create a plumage or feather spec with CLI-allocated ID. |
| `nest` | nest.go | Subcommand dispatcher: `new`, `scaffold`, `scout --module`, `stamp`, `status` for `.fledge/nest/`. |
| `preen` | preen.go | Validate spec set + scaffold drift; reports findings. |
| `ready` | ready.go | List feathers in `pipping` state (claimable). |
| `vee` | vee.go | Dependency graph: cycles, topological waves, `--format dot/json`. |
| `colony` | colony.go | Aggregate status counts, per-plumage completion, orphans, blocked tasks. |
| `unfledged` | unfledged.go | List all non-`fledged` specs. |
| `status` | status.go | Read/transition a spec's lifecycle status. |
| `set` | set.go | Mutate a spec frontmatter field (priority, oversight, depends_on, title). |
| `criteria` | criteria.go | List/check/uncheck acceptance-criteria checkboxes. |
| `brood` | brood.go | Claim a feather (creates `.brood` lock file). |
| `abandon` | brood.go | Release a feather claim. |
| `broods` | brood.go | List active claims (`--stale` filters missing worktrees). |
| `roster` | roster.go | Assign/release/list agent species names per feather. |
| `version` | version.go | Print binary version (`binaryVersion` constant). |
| `update` | update.go | Self-update: fetch latest GitHub release, verify, swap binary. |

All 19 support `--json`.

## Build & run

```sh
go build ./...                    # build everything
go build -o fledge ./cmd/fledge   # build the CLI binary
go install ./cmd/fledge           # install to $(go env GOPATH)/bin
```

After install: `hash -r` (drop shell path cache), `command -v fledge` (confirm resolution), `fledge version` (must match `VERSION` file — if not, the installed binary is stale, rerun `go install`).

## Test entry points

```sh
go test ./...                                    # everything
go test ./cmd/fledge -run TestScripts            # all 25 acceptance fixtures
go test ./cmd/fledge -run TestScripts/init       # one fixture (init.txtar)
go test ./cmd/fledge -run TestScripts/init -v     # with script trace
go test ./internal/spec -run TestAllocateID       # one unit test
```

`cmd/fledge/main_test.go`'s `TestMain` registers the CLI function (not a built binary) with `testscript`, so `.txtar` fixtures run `fledge` as an in-process shell command — no separate binary build needed for acceptance tests. `ls cmd/fledge/testdata/*.txtar | wc -l` = 25 fixtures, one per command or cross-cutting behavior.

## Scaffolded entry points (generated into consuming repos by `fledge init`)

- `.fledge/skills/fledge-orchestrate/SKILL.md` — routing entry point agents read first (routes to planning.md/implementation.md).
- `.claude/agents/*.md` — Claude Code Task-tool agent definitions (symlinked from `internal/bootstrap/adapters/claude/agents/` in this dogfooding repo).
- `.pi/fledge-plan`, `.pi/fledge-implement` — pi harness command entry points.
- `.codex`/`AGENTS.md` append — one-line pointer for Codex.

## Open Questions

None observed beyond what's noted in modules.md/architecture.md.
