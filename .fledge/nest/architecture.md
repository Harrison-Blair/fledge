---
generated: 2026-07-15T23:53:12Z
commit: a4d02e8187c64ef9f3f1231052990b282207420b
agent: fledge-forager
fledge_version: 0.5.5
---

# Architecture

How fledge's two layers (deterministic spec CLI vs. manifest-driven bootstrap/adapter system) fit together, and how the pieces in `internal/` compose to implement each `fledge` subcommand end to end.

## Two-layer design

fledge is deliberately split into two layers that don't leak into each other (`CLAUDE.md`, `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md`):

1. **The CLI** (`internal/cli` + focused domain packages) — deterministic, agent-agnostic spec operations. Command dispatch lives in `internal/cli/cli.go`: every command file (`init.go`, `new.go`, `nest.go`, `preen.go`, `brood.go`, `vee.go`, `colony.go`, `status.go`, `set.go`, `criteria.go`, `ready.go`, `unfledged.go`, `scan.go`, `agents.go`, `update.go`, `version.go`) has an `init()` that calls `register(name, run, usage)`; `commandOrder` fixes usage/allow-list order (`internal/cli/cli.go`, per `raw/cli.md`). `Run(args []string) int` is the sole entry point (called from `cmd/fledge/main.go`); every command shares the exit code scheme `ExitOK/Fail/Usage/Env = 0/1/2/3` and supports `--json`.
2. **The bootstrap/adapter system** (`internal/bootstrap`) — what `fledge init` scaffolds into a target repo. `bootstrap.go` embeds two trees via `//go:embed core adapters`. `core/` is the single agent-neutral source of the `fledge-orchestrate` and `fledge-interrogate` skills (workflow prose + templates), written to `.fledge/skills/` by `WriteCore`. `adapters/<harness>/` is a thin format-only mapping per harness (claude, codex, pi), driven entirely by that harness's `manifest.yaml` (`registry.go` → `Manifest` struct). Adding or changing a harness is editing a manifest, not Go code (`raw/bootstrap-go.md`, `raw/bootstrap-adapters.md`).

The CLI layer never depends on the bootstrap layer's prose content; the bootstrap layer only *writes* the prose and manifests, it doesn't interpret spec files. `internal/cli/init.go`, `internal/cli/agents.go`, and `internal/cli/preen.go` are the seams where CLI commands call into `internal/bootstrap` (`LoadAdapters`, `WriteCore`, `Manifest.WriteAdapter`, `DriftReport`, `LoadStamp`) to perform scaffolding and drift-checking (`raw/cli.md`, `raw/bootstrap-go.md`).

## Domain packages behind the CLI

Each CLI command is a thin wrapper over one or more small, single-purpose `internal/` packages (`raw/internal-misc.md`, `raw/spec.md`, `raw/nest.md`):

- `internal/spec` — frontmatter parsing/rendering, ID allocation (flock-serialized), acceptance-criteria checkbox parsing/mutation, embedded plumage/feather markdown templates. The domain model every other package builds on.
- `internal/check` — validation engine behind `fledge preen`; consumes a `spec.Set` plus locked-task and evidence-dir info, emits `Finding` structs (error/warning severity).
- `internal/graph` — dependency-graph analysis behind `fledge vee` and readiness computation: cycle detection, topological wave layering, ready-set filtering.
- `internal/lock` — feather claim locking behind `fledge brood`; atomic `.brood` file acquire/release via `os.Link` (O_EXCL semantics).
- `internal/scan` — module enumeration behind `fledge scan`; groups `git ls-files` output by top-level directory, filtered through `.fledgeignore` via `git check-ignore`.
- `internal/repo` — repo-root detection (`git rev-parse --show-toplevel`) and path helpers (`FledgeDir`, `LocksDir`, `ContextDir`, etc.) used by every other package.
- `internal/nest` — schema/rendering/completeness-checking for `.fledge/nest/` documents (concern docs + scout reports); the package that defines what "done" means for a forager run (`raw/nest.md`).
- `internal/ciconfig`, `internal/doctest`, `internal/hooktest` — test-only packages with no production code; they assert structural properties of `.github/workflows/*.yml`, root docs (README/RELEASING), and `scripts/hooks/pre-commit` respectively, keeping CI config and documentation from silently drifting from what the code actually does (`raw/internal-misc.md`).

## Cross-module relationships

- `internal/cli/*` imports `internal/spec`, `internal/check`, `internal/graph`, `internal/lock`, `internal/scan`, `internal/repo`, `internal/nest`, and `internal/bootstrap` — it is the only layer that composes all of them.
- `internal/check` imports `internal/spec` and `internal/graph` (cycle detection reused for validation).
- `internal/nest` imports `internal/spec` for `SplitFrontmatter`/`YAMLScalar` helpers — nest documents use the same frontmatter conventions as plumage/feather specs.
- `internal/bootstrap` is self-contained (embeds its own template data); the CLI depends on it, not vice versa.
- `cmd/fledge` is a 2-file shim: `main.go` calls `cli.Run(os.Args[1:])` and propagates the exit code; `main_test.go` plus `testdata/*.txtar` (23 files) are the CLI's black-box acceptance suite, driven via `github.com/rogpeppe/go-internal/testscript` (`raw/cmd.md`).

## Repo dogfooding loop

This repo runs fledge on itself: `.fledge/pluma/` holds its own plumage/feather specs, `.fledge/broods/` holds active claims, `.fledge/nest/` (this directory) holds the synthesized context you're reading, and `.claude/` is a scaffolded Claude adapter (five agents symlinked into `internal/bootstrap/adapters/claude/agents/`, per user memory). Changing embedded `core/`/`adapters/` content requires regenerating this repo's own scaffold (`fledge init --refresh`) and reviewing `git status`, and updating the txtar fixtures that assert on scaffolded output (`init.txtar`, `init_agents.txtar`, `agents.txtar`) — see `CLAUDE.md` and `raw/root.md`.

## Open Questions

- Exact algorithm `DetectAdapters` uses to pick among multiple present harness markers (`.claude/`, `.pi/`, `.codex/`) when more than one exists — implemented in `internal/bootstrap` but not directly observed by the `cli` scout (`raw/cli.md`).
- Whether `internal/bootstrap`'s manifest schema supports conditional file entries (include a file only if a primitive is provided), or all `Files` entries are always written regardless of primitive coverage (`raw/bootstrap-go.md`).
