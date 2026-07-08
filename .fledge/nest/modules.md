---
generated: 2026-07-08T05:28:12Z
commit: e46c481a047d45ef10bcd79a3326d47932b32868
agent: fledge-forager
fledge_version: 0.2.1
---

# Modules

Repo map organized by `fledge scan` module. One entry per module: purpose, key files, and where to look for what.

## root

Repository root: licensing, versioning, orientation docs, and the dogfooding install path.

Key files: `README.md` (quick start, 6-primitive contract, command inventory, terminology decoder at lines ~8-12), `CLAUDE.md` (architecture + build/test/run + conventions + the install/verify discipline), `MIGRATION.md` (0.1.0→0.2.0: skills moved `.claude/` → `.fledge/skills/`), `VERSION` (single line, currently `0.2.1`), `go.mod` (Go 1.26.4), `scripts/install.sh` (build → `go install` with version ldflags → `hash -r` → PATH check → `fledge version` matches VERSION → optional `--refresh`), `.gitignore` (ignores `.fledge/nest/raw/`, `.fledge/broods/`, `.fledge/burrows/`, built `/fledge`).

Look here for: build/install/verify commands, version number, terminology decoder, top-level dependencies, how the dogfooded binary is kept in sync.

## cmd

CLI entry point and the acceptance-test harness. Thin — no domain logic.

Key files: `cmd/fledge/main.go` (~11-line `main()`, delegates to `cli.Run`), `cmd/fledge/main_test.go` (testscript harness; deterministic git env), `cmd/fledge/testdata/*.txtar` (16 acceptance files, one per command/area: agents, check, criteria, e2e, graph, init, init_agents, lock, nest, new, ready, report, scan, set, status, unfledged).

Look here for: the exact stdout/exit-code contract of any command, and acceptance coverage before touching a command's behavior.

## internal/cli

The command dispatcher and all 17 commands. Deterministic, agent-agnostic.

Key files: `cli.go` (`Run`, `register`, `commandOrder`, exit codes, `emitJSON`, `loadSet`), plus one file per command — `init.go`, `agents.go`, `scan.go`, `new.go`, `nest.go`, `preen.go`, `ready.go`, `vee.go`, `colony.go`, `unfledged.go`, `status.go`, `set.go`, `criteria.go`, `brood.go` (brood/abandon/broods), `version.go`. `specload.go` holds the shared `loadSet()` loader. `fledgeignore.default` is the embedded default scan-ignore list.

Look here for: adding/altering a command, the frontmatter-mutation entry points, orphan/dangling-ref detection (`colony.go`), and the generated allow-list (driven by `commandOrder`).

## internal (domain packages)

The deterministic engines the CLI calls: `spec` (frontmatter parse/render, ID allocation via `NextID`, criteria parse/`SetCriterion`, `WriteFileAtomic`, lifecycle constants), `check` (validation rules = `preen`; receives the spec set, locked IDs, and evidence dir), `graph` (`New`, `Cycle`, `Waves`, `Ready` = `vee`), `lock` (`Acquire`/`Release`/`List`, `Record{Task,Owner,PID,Created,Branch}` = `brood`), `scan` (module inventory), `nest` (concern/scout `Doc` rendering, `ClearNest`, `ConcernDocs`, `IndexBody`), `repo` (root `Find`, directory paths incl. `EvidenceDir`, git HEAD/version).

Look here for: validation rules (`check`), plumage↔feather link and evidence semantics (`check` + `repo.EvidenceDir`), dependency/orphan logic (`graph`, `colony`), and nest doc rendering (`nest`).

## internal/bootstrap

The embed + scaffolding + adapter system. Edit this to change what `fledge init` emits — never the scaffolded copies in the repo.

Key files: `bootstrap.go` (`//go:embed core adapters`), `primitives.go` (the 6 primitives, `PrimitiveOrder`, `DeriveTier`), `registry.go` (`Manifest`, `ManifestFile` write policies, `LoadAdapters`, `WriteCore`, `WriteAdapter`, `DetectAdapters`, `CheckDuplicateSkills`, `writeIfChanged`), `registry_test.go` (manifest/coverage/neutrality/symlink tests), `core/skills/` (agent-neutral workflow source), `adapters/{claude,codex,pi}/` (per-harness manifests + files).

Look here for: anything about init/scaffolding, adapter detection, the primitive→mechanism map, agent/skill files an agent auto-loads, and the agent-neutral workflow prose.

## pluma

This repo's own spec corpus (dogfooding history). `pluma/plumage/` holds PLM-001..003 (all fledged: `fledge colony`, `fledge unfledged`, `fledge nest`). `pluma/feathers/` holds FTHR-001..007 (all fledged).

Look here for: worked examples of plumage/feather spec shape, tracer-bullet decomposition, and oversight usage.

## docs

Design prose, not shipped code. `docs/generalization-plan.md` (~333 lines): 23 locked Q&A decisions, the core+adapters model, tier derivation, milestones M0–M5, and open verification questions (Claude `skills` array support, Codex/Cursor/opencode config locations).

Look here for: the rationale behind the adapter architecture and the open harness-detection questions.
