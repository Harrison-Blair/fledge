---
generated: 2026-07-08T01:03:26Z
commit: e44524d1f089dcfe1c1f313f819ec18d9a42eceb
agent: fledge-forager
fledge_version: 0.2.1
---

# Architecture

fledge is a Go CLI that brings spec-driven development to agent-assisted repos. It is built as two deliberately separated layers plus a spec corpus that both layers operate on: a deterministic CLI/domain layer, an embedded bootstrap/adapter layer that scaffolds one agent-neutral orchestration workflow into any harness, and the `pluma/` spec tree the CLI manipulates. This repo dogfoods itself — it has its own `.fledge/` state and `pluma/` specs.

## Layer 1: deterministic CLI + domain packages (`internal/cli`, `internal/spec`, `internal/check`, `internal/graph`, `internal/lock`, `internal/repo`, `internal/scan`)

`cmd/fledge/main.go:9` is the sole binary entry point; it calls `cli.Run(os.Args[1:])` and exits with the returned code. `internal/cli/cli.go` dispatches to 16 commands registered via `register(name, runFunc, usage)` in each command file's `init()`; `commandOrder` (a global slice) drives both `usage` output and the generated Claude allow-list. Exit codes are shared and meaningful across every command: `ExitOK=0`, `ExitFail=1` (domain failure — check findings, lock held, illegal transition, cycle), `ExitUsage=2`, `ExitEnv=3` (not a git repo, no `.fledge/`). Every command supports `--json` via `emitJSON()`.

Domain packages beneath `internal/cli`:
- `internal/spec` — frontmatter parse/render with byte-exact body preservation (`internal/spec/frontmatter.go:SplitFrontmatter`), ID allocation (`internal/spec/ids.go:NextID`), criteria checkbox parsing/toggling by byte offset (`internal/spec/criteria.go`), embedded spec body templates (`internal/spec/templates.go`).
- `internal/check` — validation engine (`internal/check/check.go:Run`) producing `[]Finding` (error/warning severity) across ~15 rules (parse, duplicate-id, dangling-ref, cycle, brood-consistency, criteria-format, criteria-evidence, etc.).
- `internal/graph` — dependency graph over feathers (`internal/graph/graph.go:Graph`): `Cycle()`, `Waves()` (topological layers), `Ready()`.
- `internal/lock` — brood (claim) file management (`internal/lock/lock.go`): atomic `Acquire()` via `O_EXCL`, `Release()`, `Get()`, `List()`; one winner under contention.
- `internal/repo` — repo path resolution (`internal/repo/repo.go:Repo`): `FledgeDir()`, `LocksDir()`, `ContextDir()` (`.fledge/nest`), `EvidenceDir()` (`.fledge/molt`), `RequirementsDir()` (`pluma/plumage`), `TasksDir()` (`pluma/feathers`).
- `internal/scan` — file inventory (`internal/scan/scan.go:Run`) via `git ls-files` + `.fledgeignore` filtering (`git check-ignore`), grouped by top-level module.

This layer is entirely agent-agnostic — it has no notion of harnesses, workers, or orchestration.

## Layer 2: bootstrap/adapter system (`internal/bootstrap`)

What `fledge init` scaffolds. `internal/bootstrap/bootstrap.go` embeds two trees via `//go:embed core adapters`:

- **`core/`** — the single agent-neutral source of workflow prose: the `fledge-orchestrate` and `fledge-interrogate` skills (`internal/bootstrap/core/skills/`). Written verbatim to a target repo's `.fledge/skills/` by `WriteCore()` (`internal/bootstrap/registry.go`), shared unmodified by every harness. Core prose never references harness-specific paths (`.claude/`, `.pi/`, `.codex/`) — enforced by `TestCoreNeutral` in `internal/bootstrap/registry_test.go:140-159`.
- **`adapters/<harness>/`** — a thin, format-only mapping per harness (`claude`, `pi`, `codex`), driven entirely by that adapter's `manifest.yaml`. A `Manifest` (`internal/bootstrap/registry.go`) declares: `Detector` (marker path for auto-detection), `TierPrimitives` (primitive → mechanism map), `Files` (write-policy list), `PipingFile`. **Adding or changing a harness is editing a manifest, zero Go code** (`internal/bootstrap/registry.go:17-20`).

### The 7-primitive contract (`internal/bootstrap/primitives.go`)

`PrimitiveOrder`: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`. An adapter's **tier** (A/B/C) is *derived*, never declared, from which primitives it provides (`DeriveTier()`): Tier A = the first 4 (solo planning + implementation), Tier B = adds `spawn-worker` (fan-out foraging), Tier C = adds `message-peer` (team loop). Per `registry_test.go:70`, the Claude adapter derives Tier C, codex and pi derive Tier A.

### File write policies (`ManifestFile`, `internal/bootstrap/registry.go:37-44`)

- `generate`/`primitive_map` — render a `text/template`, feeding in provided/not-provided primitive rows.
- `overwrite` — always rewrite (generated files, never user-edited).
- `append_if_missing` — additive line (e.g. `AGENTS.md` pointer for codex).
- `symlink` — e.g. `.claude/skills/fledge-orchestrate` → `.fledge/skills/fledge-orchestrate` (`registry.go:364-388`).
- default — copy, **skip-if-exists** so user edits to skill prose survive; `fledge init --refresh` re-syncs to shipped bytes.

`writeIfChanged` makes writes byte-idempotent (compares on-disk bytes before rewriting), which the `cmd/fledge/testdata/*.txtar` acceptance tests depend on for idempotency assertions.

`CheckDuplicateSkills()` refuses to scaffold if a real (non-symlink) `SKILL.md` already exists at a harness's native skill path, preventing divergent copies.

## Layer 3: the spec corpus (`pluma/`)

`pluma/plumage/PLM-###.md` (feature intents) and `pluma/feathers/FTHR-###.md` (implementable tasks), YAML-fronted markdown, IDs and frontmatter allocated only by the CLI (`fledge new`, `status`, `set`, `criteria`, `brood`) — never hand-edited. Plumage lifecycle: `egg → hatched → fledged`. Feather lifecycle: `egg → pipping → hatching → fledged`, guarded by `internal/cli/status.go` transition tables; `--force` bypasses legality checks but not enum validity. Feathers hold `depends_on` (acyclic, feather-ID list) and optional `oversight` (`merge`/`during`) gates.

## How the layers interact

1. A user or agent runs `fledge init` (layer 2) to scaffold `.fledge/skills/` + a harness adapter into a target repo.
2. The scaffolded skill (`fledge-orchestrate`) instructs the agent to drive spec operations exclusively through the `fledge` CLI (layer 1) — `fledge new`, `fledge set`, `fledge criteria check`, `fledge brood` — never by hand-editing `pluma/` frontmatter.
3. `internal/bootstrap` itself depends on `internal/cli`'s `commandOrder` to generate the Claude `settings.local.json` Bash allow-list (`TestClaudeAllowListGenerated`, `registry_test.go:396-415`) — the one place the two layers are coupled.
4. The orchestration workflow (planning → feather authoring → implementation) is described in `core/skills/fledge-orchestrate/{planning,implementation,worker-protocols,foraging}.md` and executed by agents in the target repo using the primitives their harness's adapter provides.

## Open Questions
- How does bootstrap adapter detection integrate with command registration and primitive-coverage tiers beyond the allow-list generation described above? (internal-domain scout, unresolved from assigned files)
- How are `commandOrder` and each adapter's generated allow-list kept in sync as commands are added? (internal-domain scout)
