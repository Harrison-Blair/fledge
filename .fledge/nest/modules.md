---
generated: 2026-07-08T01:03:26Z
commit: e44524d1f089dcfe1c1f313f819ec18d9a42eceb
agent: fledge-forager
fledge_version: 0.2.1
---

# Modules

Repo map, organized by top-level module as reported by `fledge scan`. One entry per module: purpose, key files, where to look for what.

## root

Repository root: license, versioning, module definition, and the two docs that orient a newcomer.

Key files: `README.md` (quick start, 7-primitive contract, command inventory), `CLAUDE.md` (architecture + build/test/run + conventions), `MIGRATION.md` (0.1.0→0.2.0 upgrade path), `VERSION` (single line, currently `0.2.1`), `go.mod` (Go 1.26.4, two direct deps), `.gitignore` (ignores `.fledge/nest/raw/`, `.fledge/broods/`, `.fledge/burrows/`, built `/fledge` binary).

Look here for: build/install/test commands, license terms, version number, top-level dependency list, upgrade instructions between fledge versions.

## cmd

CLI entry point package. Thin — no domain logic of its own.

Key files: `cmd/fledge/main.go` (9-line `main()`, delegates to `cli.Run`), `cmd/fledge/main_test.go` (testscript harness setup, git determinism), `cmd/fledge/testdata/*.txtar` (17 acceptance-test files, one per command/workflow area: `init`, `init_agents`, `agents`, `new`, `status`, `set`, `criteria`, `check`, `lock`, `graph`, `scan`, `ready`, `report`, `unfledged`, `e2e`).

Look here for: what a full end-to-end CLI invocation of any command looks like, exact stdout/exit-code contracts, acceptance-test coverage for a given command before touching its behavior.

## docs

Design documentation, not shipped code.

Key files: `docs/generalization-plan.md` — the locked design (23 Q&A decisions, milestones M0–M5) for generalizing fledge's orchestration to more harnesses (Cursor, opencode) beyond the current Claude/pi/Codex set.

Look here for: the rationale behind the 7-primitive contract and tier derivation, the manifest-driven adapter design, milestone/release sequencing (0.2.0 vs 0.3.0), unresolved harness-integration verifications (Claude `settings.json` skills array, Codex `AGENTS.md` auto-load, Cursor `.mdc` format, opencode config layout).

## internal/bootstrap

The scaffolding/adapter layer — what `fledge init` writes into a target repo. See `architecture.md` for full design.

Key files: `bootstrap.go` (`//go:embed core adapters`), `primitives.go` (7-primitive contract, tier derivation), `registry.go` (manifest loading, all file-write policies, symlink + duplicate-skill guard — 517 lines, the largest file in the repo), `registry_test.go` (9 tests covering manifest validity, tier derivation, core neutrality, write classification, symlinks, allow-list generation). Plus `core/skills/fledge-orchestrate/{SKILL,planning,implementation,worker-protocols,foraging}.md` + `templates/{scout-report,plumage,feather,context-doc}.md`, `core/skills/fledge-interrogate/SKILL.md`, and `adapters/{claude,pi,codex}/manifest.yaml` + per-harness generated/static files.

Look here for: adding or changing a target-harness adapter (edit `manifest.yaml`, no Go), changing orchestration workflow prose (edit `core/skills/...`, then regenerate this repo's own `.fledge/skills/` via `fledge init --refresh`), the primitive/tier contract, write-policy semantics for scaffolded files.

## internal (domain packages: cli, check, graph, lock, repo, scan, spec)

The deterministic CLI dispatch and spec domain logic. See `architecture.md` for full design.

Key files: `internal/cli/cli.go` (command dispatch, exit codes), `internal/cli/specload.go` (shared `loadSet()` helper), 13 command files under `internal/cli/` (one per command), `internal/spec/{types,frontmatter,load,ids,criteria,templates}.go` (spec data model + parsing), `internal/check/check.go` (validation rules), `internal/graph/graph.go` (dependency waves/cycles/ready-set), `internal/lock/lock.go` (brood claim files), `internal/repo/repo.go` (path resolution), `internal/scan/scan.go` (file inventory).

Look here for: adding a new CLI command, changing spec frontmatter schema or validation rules, dependency-graph or lock-contention logic, exactly which paths under `.fledge/` and `pluma/` the CLI reads/writes.

## pluma

The spec corpus for *this* repository's own features and tasks (fledge is fledge-managed).

Key files: `pluma/plumage/PLM-001-*.md`, `PLM-002-*.md` (both `fledged`), `pluma/feathers/FTHR-001-*.md` through `FTHR-004-*.md` (all `fledged`) — the `fledge colony` and `fledge unfledged` commands were themselves built through this spec workflow.

Look here for: worked examples of real plumage/feather spec bodies (Context, User Stories, Functional Criteria, Acceptance Criteria sections), the frontmatter schema in practice, evidence of test-first discipline (AC-1 pattern) across a completed feature.
