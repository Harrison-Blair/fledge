---
generated: 2026-07-11T01:58:32Z
commit: 96a3ac38bc843217824d6d6886c49906053bf686
agent: fledge-forager
fledge_version: 0.3.4
---

# Architecture

How fledge is put together: a deterministic CLI over spec files, plus a bootstrap/adapter layer that scaffolds an agent-neutral orchestration workflow into any harness. The two layers are deliberately separated and talk to each other only through `internal/bootstrap`'s exported API.

## Two layers

**1. The CLI** (`cmd/fledge/main.go` → `internal/cli`) — deterministic, agent-agnostic spec operations. `main()` (`cmd/fledge/main.go`) calls `cli.Run(os.Args[1:])` (`internal/cli/cli.go:Run`), which dispatches to one of 17 registered subcommands (`commandOrder`, `internal/cli/cli.go`). Each command file (`internal/cli/new.go`, `status.go`, `preen.go`, `nest.go`, …) calls `register(name, run, usage)` from its own `init()` — there is no central switch statement. Exit codes are shared and meaningful: `ExitOK/Fail/Usage/Env` = 0/1/2/3 (`internal/cli/cli.go`).

**2. The bootstrap/adapter system** (`internal/bootstrap/`) — what `fledge init` scaffolds into a target repo:
- `bootstrap.go` embeds two trees via `//go:embed core adapters`, exposed as `FS`.
- `core/` is the single agent-neutral source of the `fledge-orchestrate` and `fledge-interrogate` skills (`planning.md`, `implementation.md`, `foraging.md`, `worker-protocols.md`, `templates/`) — written to a target repo's `.fledge/skills/` by `WriteCore()` (`internal/bootstrap/registry.go`).
- `adapters/<harness>/` (claude, codex, pi) is a thin format-only mapping, each driven entirely by its `manifest.yaml` (`registry.go:Manifest`): a detector, a `tier_primitives` map, and a file list with per-file write policies (`ManifestFile`: `primitive_map`/`generate`, `overwrite`, `append_if_missing`, `symlink`, default copy-skip-if-exists). Adding a harness means writing a manifest — zero Go code.
- The 6 primitives (`internal/bootstrap/primitives.go:PrimitiveOrder`) — `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer` — are capability contracts an adapter declares coverage for; its tier (A/B/C) is *derived* from that coverage via `DeriveTier()`, never declared directly.
- `stamp.go` persists what was written (`.fledge/scaffold.json`, `Stamp`/`StampEntry`) so `drift.go`'s `DriftReport()` can classify every scaffolded file as up-to-date/stale/modified/missing/obsolete on a later `fledge init --refresh` or `fledge preen`.

## Cross-module relationships

- `internal/cli` is the sole consumer of `internal/bootstrap` (agents/init/preen commands), `internal/spec` (all spec-mutating commands), `internal/check` (preen), `internal/graph` (vee, ready), `internal/lock` (brood/abandon/broods), `internal/scan` (scan), and `internal/nest` (nest new/scaffold/scout/stamp). None of those domain packages import each other or `internal/cli` — `internal/cli` is purely an orchestration/formatting layer over independent, focused packages.
- `internal/spec` is the foundational data layer: `Requirement`/`Task`/`Set` (`internal/spec/types.go`) are consumed by `internal/check` (validation), `internal/graph` (dependency structure), and `internal/nest` (`spec.YAMLScalar`, `spec.SplitFrontmatter` for frontmatter safety).
- `internal/repo` (`repo.go`) is the shared root-finder: `Find()` locates the git root and derives `.fledge/` subpaths (`FledgeDir`, `LocksDir`, `ContextDir`, `RequirementsDir`, `TasksDir`, `EvidenceDir`) used across `internal/cli` command implementations.
- This repo dogfoods fledge itself: `.fledge/` holds its own nest and broods, `pluma/` holds its own plumage/feather specs, and `.claude/agents/*.md` (except the incubator) are symlinks into `internal/bootstrap/adapters/claude/agents/` — editing behavior means editing the Go-embedded source, not the scaffolded copy.

## Open Questions

- Migration behavior when a manifest's file list or write policy changes between releases isn't fully visible from `internal/bootstrap` alone — `PruneObsolete` (`internal/bootstrap/drift.go`) removes obsolete stamp entries on refresh, but the full cross-version migration story is presumed to live in `MIGRATION.md` (root), which was out of scope for every scout this round.
