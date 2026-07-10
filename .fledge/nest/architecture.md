---
generated: 2026-07-10T14:50:00Z
commit: 7678344ab9a18730530b9f6edf507ad0c449d352
agent: fledge-forager
fledge_version: 0.2.1
---

# Architecture

How fledge's two layers — the deterministic CLI and the harness-agnostic bootstrap/adapter system — fit together, and how this repo's own `pluma/`/`.fledge/` scaffolding dogfoods that system.

## Two-layer design

fledge is deliberately split into two layers that never leak into each other:

1. **The CLI** (`cmd/fledge/main.go` → `internal/cli`) — deterministic, agent-agnostic spec operations. `cmd/fledge/main.go` is a one-line shim that calls `internal/cli.Run(os.Args[1:])` and exits with its code. `internal/cli/cli.go` holds the command registry: each command file (`agents.go`, `brood.go`, `criteria.go`, `init.go`, `nest.go`, `new.go`, `preen.go`, `ready.go`, `scan.go`, `set.go`, `status.go`, `unfledged.go`, `vee.go`, `version.go`, `colony.go`) registers itself via `init()` calling `register(name, run, usage)`; `commandOrder` in `cli.go` fixes both the `--help` listing and the generated Claude allow-lists (see Q23 in `docs/generalization-plan.md`). Every command supports `--json` via a shared `emitJSON` helper, and returns one of four meaningful exit codes: `ExitOK/Fail/Usage/Env` = 0/1/2/3.
2. **The bootstrap/adapter system** (`internal/bootstrap`) — what `fledge init` scaffolds into a target repo. `internal/bootstrap/bootstrap.go` embeds two trees via `//go:embed core adapters`: `core/skills/fledge-orchestrate/` and `core/skills/fledge-interrogate/` are the single agent-neutral source of the orchestration workflow prose (`SKILL.md`, `planning.md`, `implementation.md`, `foraging.md`, `worker-protocols.md`, and `templates/`); `adapters/<harness>/` (`claude/`, `pi/`, `codex/`) are thin, format-only mappings driven entirely by a `manifest.yaml` per harness (`internal/bootstrap/registry.go` → `Manifest` type). `internal/bootstrap/registry.go` implements `WriteCore` (copies core skills to a repo's `.fledge/skills/`, byte-idempotent via `writeIfChanged`) and `Manifest.WriteAdapter` (renders/copies adapter files per each file's write policy). Adding or changing a harness is purely a manifest edit — zero Go code.

The two layers are joined only by the **6-primitive contract** (`internal/bootstrap/primitives.go`, `PrimitiveOrder`): `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`. Each adapter's `manifest.yaml` declares which mechanism realizes each primitive it supports (its `tier_primitives` map); the adapter's **tier** (A: solo / B: fan-out foraging / C: full team loop) is *derived* from that coverage by `DeriveTier`, never hand-declared. `docs/generalization-plan.md` records this as a locked, 23-decision design (originally specced with 7 primitives including `spawn-pool`; the shipped implementation collapsed to 6 — see Open Questions).

## Manifest file write policies

`ManifestFile` (`internal/bootstrap/registry.go:45`) gives every scaffolded file one of five write policies, which the CLI's own regeneration depends on being right:
- `generate` / `primitive_map` — rendered from a `text/template` at write time (e.g. `fledge-adapter.md`, primitive tables).
- `overwrite` — copied verbatim, rewritten whenever bytes differ (fledge-managed files like generated `settings.local.json` allow-lists).
- `append_if_missing` — an additive line ensured present, never clobbering the rest of the file (e.g. Codex's `AGENTS.md` pointer).
- `symlink` — points a harness-native path (e.g. `.claude/skills/fledge-orchestrate`) at the core skill under `.fledge/skills/`.
- default — copy, **skip-if-exists**, so user edits survive; `fledge init --refresh` re-syncs default-policy files and always repairs `overwrite`-policy ones.

## Dogfooding: this repo's own fledge state

This repository is itself fledge-managed: `pluma/plumage/PLM-###` and `pluma/feathers/FTHR-###` track fledge's own feature work (see `domain.md`, `pluma` module), and `.fledge/skills/`, `.claude/` are the scaffolded output of the exact `internal/bootstrap` code described above — regenerated via `fledge init --refresh` whenever `core/`/`adapters/` embedded content changes (`CLAUDE.md`). `.fledge/nest/` (this document set) is produced by the **forager/scout** protocol defined in `core/skills/fledge-orchestrate/foraging.md` and implemented by `internal/nest` (`nest.go`, `docs.go`) plus the `fledge nest` CLI subcommand (`internal/cli/nest.go`).

## Cross-module relationships

- `internal/cli` commands are thin dispatchers over domain packages: `internal/spec` (frontmatter, IDs, templates, load), `internal/check` (validation = preen), `internal/graph` (dependency graph = vee), `internal/lock` (feather claims = brood), `internal/scan` (module enumeration), `internal/repo` (git root/path resolution), `internal/nest` (context doc scaffolding), and `internal/bootstrap` (init/agents commands).
- `internal/bootstrap` is the only package that embeds prose/config trees (`core/`, `adapters/`) rather than implementing spec logic; `internal/cli/init.go` and `internal/cli/agents.go` are its only callers.
- `cmd/fledge/testdata/*.txtar` acceptance tests assert on the *combined* output of both layers — e.g. `init.txtar`/`init_agents.txtar`/`agents.txtar` assert on bootstrap scaffolding, while `new.txtar`/`status.txtar`/`set.txtar`/etc. assert on pure CLI spec operations.

## Open Questions

- `docs/generalization-plan.md` describes a 7-primitive contract including `spawn-pool` (Tier C keeps N reusable named workers); the shipped `internal/bootstrap/primitives.go` defines only 6 primitives (no `spawn-pool`). Whether this was a deliberate descope or `spawn-pool` was folded into `spawn-worker`/`message-peer` semantics is not resolved by any scout.
- Exact Codex CLI skills location/auto-load behavior and Cursor/opencode adapter formats are marked TBD-pending-verification in `docs/generalization-plan.md` (§7, M3/0.3.0 milestones) — not yet implemented as of HEAD.
