---
generated: 2026-07-06T23:33:05Z
commit: b701cf5a12a99b5adf9538e83f51178d4dead0c2
agent: fledge-context-gatherer
fledge_version: 0.1.0
---

# Entry Points & Public Interfaces

How execution enters fledge and the full command surface it exposes.

## Process entry
- **`cmd/fledge/main.go:main`** — the only executable entry point. It calls `internal/cli.Run(os.Args[1:])` and exits with the returned code.
- **`internal/cli.Run(args []string) int`** — top-level dispatcher. Looks up the subcommand in the init-time registry (`internal/cli/cli.go:commands`); on an unknown command it prints usage and returns `ExitUsage`.

## Build & run
- Build: `go build ./cmd/fledge` (produces `/fledge`, git-ignored). Version can be stamped at build time: `-ldflags "-X github.com/Harrison-Blair/fledge/internal/cli.binaryVersion=x.y.z"` (`internal/cli/version.go`).
- Run without building: `go run ./cmd/fledge <command>`.
- Test: `go test ./...` (runs both the txtar e2e suite and per-package unit tests).

## Exit-code taxonomy (`internal/cli/cli.go`)
- `0` `ExitOK` — success.
- `1` `ExitFail` — domain failure: check findings, lock held, illegal status transition, dependency cycle, file I/O error.
- `2` `ExitUsage` — usage error: unknown command, bad flag syntax.
- `3` `ExitEnv` — environment error: not in a git repo, or `.fledge/` missing where required.

## Command surface
All commands accept `--json` for structured output alongside human-readable text.

| Command | Args | Key flags | Purpose |
|---|---|---|---|
| `init` | — | — | Scaffold `.fledge/` (spec dirs, default `scan-ignore`) and append per-run intermediates to `.gitignore`. Idempotent (reports "exists" on re-run). |
| `scan` | — | — | Inventory repo files grouped into modules, honoring `.fledge/scan-ignore`; output byte-compatible with the retired scan script. |
| `new req` | — | `--title`*, `--priority`, `--agent` | Create a requirement; allocates next `REQ-NNN`; status defaults to `draft`; default agent `fledge-orchestrate/planning`. |
| `new task` | — | `--title`*, `--req`*, `--depends-on`, `--priority`, `--oversight`, `--force` | Create a task; allocates next `TASK-NNN`; status computed from deps (ready if all deps done, else blocked). Linking to a `draft` requirement requires `--force`. |
| `check` | — | `--strict` | Run all validation rules; `--strict` promotes warnings to errors. Findings → `ExitFail`. |
| `ready` | — | — | List tasks with all deps done and not locked, sorted by priority then ID. |
| `graph` | `[REQ-NNN]` | `--format {text\|dot\|json}` | Render the dependency graph; optional requirement filter limits to that requirement's subgraph plus dependency closure; detects cycles (→ `ExitFail`). |
| `status` | `ID [new-status]` | `--force` | View or transition a requirement/task status; enforces legal transitions unless `--force`. |
| `set` | `ID field value` | — | Update a mutable field (`priority`, `oversight`, `depends_on`, `title`); rejects immutable fields; cycle-checks `depends_on`. |
| `lock` | `TASK-NNN` | `--owner`*, `--branch` | Acquire an exclusive lock; auto-sets status to in-progress; branch defaults to current git branch; rolls back on status-write failure. |
| `unlock` | `TASK-NNN` | `--done`, `--force` | Release a lock; `--done` sets status to done first; `--force` skips status changes and tolerates a missing lock. |
| `locks` | — | — | List active locks with owner, timestamp, branch, and PID-liveness indicator. |
| `version` | — | — | Print the binary version. |

(*) required flag.

## Library public interfaces (for internal callers)
- `internal/spec`: `Load`, `ParseRequirementFile`, `ParseTaskFile`, `SplitFrontmatter`, `NextID`, `Kebab`, `RequirementBody`, `TaskBody`, and `Requirement`/`Task` methods `Frontmatter`/`Render`/`Save`.
- `internal/check`: `Run(set, lockedTasks) []Finding`, `HasErrors`.
- `internal/graph`: `New(tasks)`, then `Cycle`, `Waves`, `Ready`.
- `internal/lock`: `Acquire`, `Release`, `Get`, `List`.
- `internal/repo`: `Find`, plus path resolvers for `.fledge/`, spec dirs, VERSION, HEAD.
- `internal/scan`: `Run(root) (*Result, error)`.

## Graph output formats (`internal/cli/graph.go`)
- **text** — waves numbered 1..N, tasks per wave annotated with status.
- **dot** — Graphviz `rankdir=LR`; done nodes filled lightgray, in-progress lightyellow.
- **json** — `{nodes:[{id,title,status,requirement,depends_on}], waves:[[...]]}`.
