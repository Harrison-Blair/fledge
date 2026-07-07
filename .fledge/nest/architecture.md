---
generated: 2026-07-06T23:33:05Z
commit: b701cf5a12a99b5adf9538e83f51178d4dead0c2
agent: fledge-context-gatherer
fledge_version: 0.1.0
---

# Architecture

fledge is a single-binary Go CLI for spec-driven development. It manages a repository's requirements and tasks as markdown files with YAML frontmatter under `.fledge/`, and provides deterministic operations over them (create, validate, graph, lock, scan) that agents and humans both drive. The codebase is a thin `cmd` entry point over an `internal` library organized as a command layer plus focused core packages.

## Layering

Execution flows top-down through three tiers (`cmd/fledge/main.go`, `internal/cli/*`, `internal/{spec,check,graph,lock,repo,scan}`):

- **Entry** — `cmd/fledge/main.go:main` is a bootstrap that calls `cli.Run(os.Args[1:])` and returns its exit code to the OS. It holds no logic.
- **Command layer** — `internal/cli` implements every subcommand (one file per command), argument parsing, output formatting (text + `--json`), exit-code discipline, and dispatch via an init-time registry (`internal/cli/cli.go:commands`). It orchestrates the core packages but owns no domain algorithms itself.
- **Core packages** — each `internal/*` package owns one concern and is independently testable:
  - `internal/spec` — the data model: `Requirement` and `Task` structs, frontmatter parse/render with byte-for-byte body preservation, ID allocation, template scaffolding, atomic writes (`internal/spec/types.go`, `frontmatter.go`, `ids.go`, `load.go`, `templates.go`).
  - `internal/check` — validation rules engine; `check.Run` returns `[]Finding` rather than errors (`internal/check/check.go`).
  - `internal/graph` — task dependency DAG: cycle detection, wave (topological layer) computation, ready-set calculation (`internal/graph/graph.go`).
  - `internal/lock` — advisory per-task file locks under `.fledge/broods/` with atomic `O_EXCL` acquisition (`internal/lock/lock.go`).
  - `internal/repo` — repo root discovery via git and resolution of `.fledge/`, spec dirs, VERSION, HEAD (`internal/repo/repo.go`).
  - `internal/scan` — file inventory grouped into modules for context gathering, honoring `.fledgeignore` (`internal/scan/scan.go`).

## Dependency direction

`cmd` → `internal/cli` → core packages. Within core, `internal/check` depends on `internal/graph` and `internal/spec`; `internal/graph` depends on `internal/spec`; `spec`, `lock`, `repo`, and `scan` depend only on the standard library (plus `goccy/go-yaml` in `spec`). There are no import cycles; the domain model (`spec`) sits at the bottom and everything above consumes it.

## Cross-cutting design principles

- **Determinism** — operations are byte-reproducible. `internal/spec/frontmatter.go` renders frontmatter in a fixed key order with canonical scalar quoting and never rewrites the markdown body (`SplitFrontmatter` returns the body unchanged; `TestTaskRoundTrip` guards this). `internal/scan` output is documented as byte-compatible with a retired `.fledge/scripts/scan` bash script.
- **Atomic mutation** — every spec and lock write goes through temp-file + rename (`spec.WriteFileAtomic`, `lock.Acquire` with `O_EXCL`), so concurrent writers collide safely instead of corrupting files.
- **Consistency invariants across packages** — lock and status are kept coherent by the command layer: `fledge brood` acquires the lock then sets status to in-progress, rolling the lock back if the status write fails; `fledge abandon --done` writes done status *before* removing the lock (`internal/cli/lock.go`; `lock_test.go:TestLockRollsBackOnStatusWriteFailure`).
- **Findings vs errors** — validation surfaces domain problems as `check.Finding` values with a rule and severity, distinct from Go `error`s used for I/O and environment failures. This maps onto the CLI's exit-code taxonomy (see `entry-points.md`).

## Relationship to the agent layer

The repository also contains `.claude/` agents and skills (excluded from scan via `.fledgeignore`) that orchestrate this CLI: a planning/orchestration skill authors PLM/FTHR specs, and an implementation loop locks tasks, works them in git worktrees, and unlocks on completion. The CLI is the deterministic substrate those agents call; it does not itself contain agent logic.
