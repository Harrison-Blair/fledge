---
generated: 2026-07-17T02:54:09Z
commit: e7a6d4969f861ed3f03af7833b750a7cd703a7a8
agent: fledge-forager
fledge_version: 0.5.8
---

# Architecture

fledge is a Go CLI that keeps feature intent (plumages) and implementable tasks (feathers) as validated markdown specs on disk, and scaffolds one agent-neutral orchestration workflow into any coding-agent harness. This document covers the two-layer system design and how its major packages relate.

## Two-layer design

1. **CLI layer** (`internal/cli` + domain packages) — deterministic, agent-agnostic spec operations. `internal/cli/cli.go:Run` dispatches to 19 registered subcommands (`commandOrder`), each delegating to a focused domain package:
   - `internal/spec` — spec file I/O: frontmatter parse/render, ID allocation, criteria checkboxes, templates (`internal/spec/frontmatter.go`, `ids.go`, `criteria.go`, `templates.go`, `types.go`)
   - `internal/check` — spec validation, backs `fledge preen` (`internal/check/check.go:Run`)
   - `internal/graph` — dependency graph/cycle detection/waves, backs `fledge vee` (`internal/graph/graph.go:New`)
   - `internal/lock` — feather claim (`.brood`) files, backs `fledge brood`/`abandon`/`broods` (`internal/lock/lock.go:Acquire`)
   - `internal/scan` — repo file/module enumeration, backs `fledge scan` (`internal/scan/scan.go:Run`)
   - `internal/repo` — git-root resolution and standard `.fledge/` subdirectory paths (`internal/repo/repo.go:Find`)
   - `internal/nest` — schemas/templates/status for `.fledge/nest/` context docs, backs `fledge nest` (`internal/nest/nest.go`, `internal/nest/docs.go`)
   - `internal/roster` — worker species-name allocation, backs `fledge roster` (`internal/roster/roster.go`)
   - `internal/ledger` — agent handoff/liveness records under `.fledge/ledger/`, backs `fledge heartbeat` (`internal/ledger/ledger.go`)

2. **Bootstrap/adapter layer** (`internal/bootstrap`) — what `fledge init` scaffolds into a target repo.
   - `internal/bootstrap/bootstrap.go` embeds two trees via `//go:embed core adapters`.
   - `internal/bootstrap/core/skills/` is the single agent-neutral source of the orchestration workflow prose (`fledge-orchestrate`, `fledge-interrogate` skills — planning.md, implementation.md, foraging.md, incubator.md, brooder.md, skua.md, worker-protocols.md, templates/). Written to a repo's `.fledge/skills/` by `WriteCore`.
   - `internal/bootstrap/adapters/<harness>/` (claude, codex, pi) are thin, format-only per-harness mappings, each driven entirely by a `manifest.yaml` (detector, `tier_primitives` map, file list with write policies). Adding a harness is a manifest edit, not Go code.
   - `internal/bootstrap/primitives.go` defines the 6 primitives (`confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`) and derives an adapter's tier (A/B/C) from its declared coverage — tier is never declared directly.
   - `internal/bootstrap/registry.go` (`Manifest`, `WriteCore`, `WriteAdapter`) implements the file write policies: `generate`/`primitive_map` (render a `text/template`), `overwrite` (always rewritten), `append_if_missing` (additive), `symlink` (managed repoint, e.g. `.claude/skills/...` → `.fledge/skills/...`), and default (copy, skip-if-exists so user edits survive; `init --refresh` re-syncs).
   - `internal/bootstrap/stamp.go` (new, PLM-031 dev install mode) implements `Stamp`/`StampEntry` and `.fledge/scaffold.json` — the record of every fledge-owned file, its content hash/symlink-target/append-lines, and (new) an optional `DevSource` for `--dev` symlink-based installs. `internal/bootstrap/drift.go` classifies on-disk state against that stamp (up-to-date/stale/modified/missing/obsolete) for `fledge preen`.

## Cross-module relationships

- `cmd/fledge/main.go` is the sole binary entry point; it calls `internal/cli.Run(os.Args[1:])` and does nothing else (see `entry-points.md`).
- Every `internal/cli/*.go` command file is a thin adapter between argument parsing and one (or a few) domain packages; domain packages never import `internal/cli` (one-directional dependency).
- `internal/spec` is the shared data model: `internal/check`, `internal/graph`, and `internal/cli` (new/set/criteria/status/brood) all operate on `spec.Requirement`/`spec.Task` loaded via `spec.Load`.
- `internal/lock` (broods) and `internal/spec` (status field) are coordinated by `internal/cli/brood.go`: acquiring a lock also flips feather status to `hatching`; releasing flips to `fledged` before unlocking, so a crash mid-transition leaves a detectable stale-brood state that `fledge preen`/`fledge colony` surface.
- `internal/nest` (context doc schemas) is used both by `fledge nest` (CLI command, scaffolds/status-checks `.fledge/nest/`) and — conceptually — by the `fledge-forager`/`fledge-context-scout` workflow roles documented in `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md`, which is the process this very document set was produced by.
- `internal/bootstrap` and `internal/cli/init.go` are tightly coupled: `init.go` calls `bootstrap.LoadAdapters`, `bootstrap.WriteCore`, `Manifest.WriteAdapter`, and `bootstrap.Stamp.Write`; `internal/cli/preen.go` calls `bootstrap.DriftReport` for scaffold health.
- The **core/** skill prose (bootstrap-core) is agent-neutral and describes worker roles (incubator, forager/scout, brooder/skua) and phases (planning, foraging, implementation) that only become concrete via a harness's primitive coverage (adapters). Tier A harnesses (Codex, pi) run planning/implementation solo, in-session; Tier C (Claude Code) can spawn workers and run the team loop.

## Open Questions

- Whether feather implementation across multiple plumages can dispatch in parallel, or is strictly sequenced, is not fully specified in `planning.md`/`implementation.md` prose (see `bootstrap-core` scout report).
- No machinery is described for detecting broken cross-references when a concern doc or skill section is renamed/removed — relies on editorial discipline during synthesis (this document included).
