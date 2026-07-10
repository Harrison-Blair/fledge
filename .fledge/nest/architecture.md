---
generated: 2026-07-10T20:53:53Z
commit: f28efebd76d6aa135adb0956a3337a40a8d98351
agent: fledge-forager
fledge_version: 0.3.0
---

# Architecture

How fledge's two layers — the deterministic CLI and the bootstrap/adapter scaffolding system — fit together, plus the repo's own dogfooded spec/context loop.

## Two layers, deliberately separated

**1. CLI (`internal/cli` + domain packages)** — deterministic, agent-agnostic spec operations. `internal/cli/cli.go` is the dispatcher: each command file (`internal/cli/*.go`) has an `init()` that calls `register(name, run, usage)`; `commandOrder` controls usage-listing order and is threaded down into bootstrap for adapter allow-list generation. `Run(args []string) int` is the sole public entry point, called by `cmd/fledge/main.go:main()` (11 lines — pure delegation). Exit codes are meaningful and shared across every command: `ExitOK`(0)/`ExitFail`(1)/`ExitUsage`(2)/`ExitEnv`(3). Every command supports `--json`, emitted via `emitJSON()`.

Domain logic underneath `internal/cli` is split into focused packages, each owning one concern:
- `internal/spec` — frontmatter parsing/rendering (`frontmatter.go`), ID allocation (`ids.go:NextID`), criteria checkbox mutation (`criteria.go:SetCriterion`), spec loading (`load.go:Load`), embedded templates (`templates.go`).
- `internal/check` — `check.go:Run()` implements `preen` validation (parse, schema, dangling-ref, cycle, brood-consistency, criteria findings).
- `internal/graph` — `graph.go` implements `vee`: `Cycle()` (DFS), `Waves()` (topological layers), `Ready()`.
- `internal/lock` — `lock.go` implements `brood`: `Acquire`/`Release`/`Get`/`List` over `.fledge/broods/<ID>.brood` files.
- `internal/nest` — `nest.go`/`docs.go` implement the `fledge nest` subcommands (`new`, `scaffold`, `scout`, `stamp`) that this very foraging process depends on: `Doc` (Concern|Scout kind), `ClearNest()`, `RefreshDoc()`, `ConcernDocs` (the 8 known doc names).
- `internal/repo` — `repo.go:Find()` locates the enclosing git repo and exposes path helpers (`.fledge/broods`, `.fledge/nest`, `.fledge/molt`, `pluma/...`).
- `internal/scan` — `scan.go:Run()` powers `fledge scan`, the authoritative work list for foraging (module → files/count/bytes, `.fledgeignore`-filtered via `git check-ignore`).

**2. Bootstrap/adapter system (`internal/bootstrap`)** — what `fledge init` scaffolds into a target repo. `bootstrap.go` embeds two trees via `//go:embed core adapters`:
- `core/` is the single agent-neutral source of the `fledge-orchestrate` and `fledge-interrogate` skills (routing rules, `planning.md`, `implementation.md`, `worker-protocols.md`, `foraging.md`, `templates/`). Written to a target repo's `.fledge/skills/` by `WriteCore()`.
- `adapters/<harness>/` is a thin, format-only mapping per harness (`claude/`, `codex/`, `pi/`), each driven entirely by its `manifest.yaml` (parsed into a `Manifest` struct by `registry.go`): a `detector` (auto-sense marker file), a `tier_primitives` map (primitive → harness mechanism string), and a `files[]` list with per-file write policies. **Adding or changing a harness is editing a manifest, zero Go code.**

The 6 primitives (`primitives.go`, `PrimitiveOrder`) are: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`. An adapter declares which mechanism realizes each primitive; its **tier** (A/B/C) is *derived* from that coverage via `DeriveTier()`, never hand-declared. Tier A = 4 primitives (solo planning/implementation — pi, Codex). Tier B = +`spawn-worker` (fan-out foraging scouts). Tier C = +`message-peer` (full brooder/skua team loop — Claude Code is the only Tier C adapter today, per `adapters/claude/manifest.yaml`).

## Manifest file write policies

`ManifestFile` (`internal/bootstrap/registry.go:38`) supports, in cascading precedence: `symlink` (e.g. `.claude/skills/fledge-orchestrate` → `../../.fledge/skills/fledge-orchestrate`, never copied) > `append_if_missing` (additive line, e.g. a CLAUDE.md/AGENTS.md pointer) > `generate`/`primitive_map`/`overwrite` (always (re)written — rendered via `text/template` for the first two, copied verbatim for the third) > default (copy, **skip-if-exists**, so user edits survive plain `fledge init` runs). `writeIfChanged()` makes writes byte-idempotent, which the `cmd/fledge/testdata/*.txtar` acceptance tests depend on for determinism.

`fledge init --refresh` writes `.fledge/scaffold.json` (a `Stamp`: `FledgeVersion`, `Agents[]`, `Files map[path]StampEntry`) — the record of which files fledge owns and at what SHA256 content hash. On subsequent `--refresh` runs, disk hash is compared against the old stamp's hash: unedited files are silently rewritten to the new embedded version; user-edited files are preserved unless `--force`. Obsolete files (present in the old stamp, absent from the new expected set) are pruned if unedited, reported if user-edited. `DriftReport()` (`internal/bootstrap/drift.go`) classifies every file into `StatusUpToDate|Stale|Modified|Missing|Obsolete`; `fledge preen` surfaces this without mutating anything.

## Cross-module relationships

- `internal/cli/init.go` orchestrates `internal/bootstrap` (`LoadAdapters`, `DetectAdapters`, `WriteCore`, `Manifest.WriteAdapter`) to scaffold `.fledge/skills/`, harness adapter files, and `.fledge/scaffold.json`.
- `internal/cli/nest.go` orchestrates `internal/nest` (`Doc`, `ClearNest`, `RefreshDoc`) — this is the CLI surface the forager/scout protocol (this document's own generation process) runs on top of.
- `internal/cli/preen.go` composes `internal/check.Run()` (spec validation) with `internal/bootstrap.DriftReport()` (scaffold validation) into one command.
- `internal/cli/status.go`/`criteria.go`/`brood.go` all call into `internal/spec` for frontmatter mutation and `internal/lock` for claim state — the CLI never hand-edits YAML frontmatter directly; it always goes through `spec.SetCriterion`, `spec.Task.Save()`, etc.
- `internal/cli/set.go` calls `graph.Cycle()` before accepting a `depends_on` edit, preventing dependency cycles at write time.
- `pluma/` (the repo's own plumage/feather specs) is the CLI's own dogfood data: every `PLM-###`/`FTHR-###` file under `pluma/plumage/` and `pluma/feathers/` is parsed by `internal/spec.Load()` and validated by `internal/check.Run()` exactly like any other fledge-managed repo's specs. `.claude/` (this repo's own scaffolded Claude adapter) is generated by the same `internal/bootstrap` code that this repo's `internal/bootstrap` package implements — a closed dogfooding loop.
- The **foraging protocol** itself (`internal/bootstrap/core/skills/fledge-orchestrate/foraging.md`, embedded and scaffolded to `.fledge/skills/fledge-orchestrate/foraging.md`) is what generated this document set: a forager (Tier B/C worker) fans out scouts per module, scouts call `fledge nest scout --module <name>` (implemented in `internal/cli/nest.go` + `internal/nest`), and the forager synthesizes the 8 concern docs you are reading now.

## Open Questions

- How do harnesses distinguish `spawn-worker` vs. `message-peer` capability at runtime, beyond the manifest's static declaration? (root scout)
- Relationship between `docs/generalization-plan.md` (locked 0.2.0→0.3.0 design, largely realized in current `internal/bootstrap`) and `docs/google_ai_mode_response.md`/`docs/research_prompt.md` (unrelated multi-tier AI-routing infra exploration) is not established anywhere in-repo — the latter two appear to be independent research artifacts, not fledge design input. (docs scout)
