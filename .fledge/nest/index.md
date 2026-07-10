---
generated: 2026-07-10T14:50:00Z
commit: 7678344ab9a18730530b9f6edf507ad0c449d352
agent: fledge-forager
fledge_version: 0.2.1
---

# Context Index

## architecture.md
Explains fledge's two-layer split (deterministic `internal/cli` vs. the `internal/bootstrap` embed/manifest scaffolding system), the 6-primitive contract that joins them, manifest file write policies, and how this repo dogfoods its own fledge state (`pluma/`, `.fledge/skills/`, `.claude/`).
Read this when: you need to understand why the CLI and bootstrap layers are separated, how a harness adapter's tier is derived, or how `fledge init --refresh` regenerates this repo's own scaffolding.

## modules.md
Repo map organized by `fledge scan` module (root+scripts, cmd, docs, internal-bootstrap, internal-cli, internal-domain, pluma) — purpose, key files, and a "Look here for" pointer per module.
Read this when: you're orienting in the repo for the first time, or need to find which module/file owns a given concern before diving into `architecture.md` or `data-model.md`.

## conventions.md
Naming, error-handling, and idiom conventions reconciled across the CLI (`register`/`commandOrder`, `--json`/exit codes), spec/file handling (byte preservation, atomic writes, ID allocation), and the bootstrap/manifest system (write policies, byte idempotence).
Read this when: writing new CLI commands, editing spec-parsing code, or touching `internal/bootstrap` and need to match existing patterns exactly.

## data-model.md
Core types: the spec model (`spec.Requirement`, `spec.Task`, frontmatter schema for plumages/feathers), validation/graph/lock types (`check.Finding`, `graph.Graph`, `lock.Record`), nest doc types, CLI `--json` output structs, and the bootstrap `Manifest`/`ManifestFile` schema.
Read this when: you need exact field names/types for a spec, a `--json` output shape, or the manifest schema before writing code against them.

## dependencies.md
Deduplicated external dependencies: the two direct Go deps (`goccy/go-yaml`, `rogpeppe/go-internal`/testscript), heavy stdlib usage, the runtime `git` dependency (repo root, scan, brood branch), and a note that `docs/google_ai_mode_response.md`'s LLM-provider mentions are unrelated example content, not real fledge dependencies.
Read this when: auditing what fledge actually depends on, adding a new dependency, or investigating the `git` subprocess overhead open question.

## entry-points.md
Build/test/run/install commands, the binary's single entry point (`cmd/fledge/main.go` → `internal/cli.Run`), the CLI dispatcher and full command surface, each domain package's public Go API, and the agent-facing orchestration entry points (`SKILL.md`, `foraging.md`).
Read this when: you need the exact command to build/test/install fledge, or need to find which function is the entry point into a specific package.

## testing.md
Test frameworks (stdlib `testing` + `testscript`/txtar), how to run them, per-package unit test coverage, and a full breakdown of all 18 `cmd/fledge/testdata/*.txtar` acceptance fixtures by concern area.
Read this when: writing or debugging a test, deciding whether a change needs a new txtar fixture, or checking what an existing fixture already asserts before duplicating coverage.

## domain.md
Glossary of fledge's bird-themed vocabulary: spec-model terms (plumage, feather, status lifecycles, AC/FC, oversight), operation verbs (preen, vee, colony, brood, molt), infrastructure nouns (nest, skill, adapter, manifest, primitive, tier), and agent roles (forager, scout, brooder, skua, orchestrator).
Read this when: you encounter unfamiliar fledge terminology anywhere in the repo or its prose and need the precise definition.

## Open Questions

- Whether the 7th primitive **`spawn-pool`** described in `docs/generalization-plan.md`'s original design was deliberately descoped from the shipped 6-primitive contract (`internal/bootstrap/primitives.go`) or folded into `spawn-worker`/`message-peer` — unresolved across all scouts (see `architecture.md`, `domain.md`).
- No caching strategy observed for repeated `git` subprocess calls (`rev-parse`, `ls-files`, `check-ignore`) in `internal/scan`/`internal/repo` — potential overhead on large repos, not measured (see `dependencies.md`).
- Exact Codex CLI skills/auto-load layout, Cursor `.cursor/rules/*.mdc` format, and opencode config layout are marked TBD-pending-verification in `docs/generalization-plan.md` (§7) — not yet implemented as of this commit.
