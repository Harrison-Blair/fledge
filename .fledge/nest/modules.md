---
generated: 2026-07-16T02:20:48Z
commit: 407b91e70b53764944447dae5829d2076fb852c5
agent: fledge-forager
fledge_version: 0.5.5
---

# Modules

Repo map: every top-level module (as `fledge scan` groups it), its purpose, its key files, and where to look for what.

## `.github` + `scripts` (CI/CD & dev tooling)

**Purpose**: GitHub Actions CI/release automation, a local pre-commit hook mirroring CI lint, and the install/dogfood script.

- `.github/workflows/pr-check.yml` — lints (`gofmt -l .`, `go vet ./...`) and tests every PR to main
- `.github/workflows/release.yml` — detects VERSION bumps (HEAD vs HEAD^), builds 4 Unix platforms (linux/darwin × amd64/arm64) with `-ldflags` version injection, creates a signed-checksum GitHub Release
- `scripts/hooks/pre-commit` — optional local git hook (`git config core.hooksPath scripts/hooks`), same gofmt/vet checks as CI
- `scripts/install.sh` — builds, `go install`s, verifies installed binary version against `VERSION`, optional `--refresh`

**Look here for**: changing CI lint/test/release behavior, understanding the 4-platform release matrix, the pre-commit opt-in hook.

## `<root>` (repo-level docs & config)

**Purpose**: top-level developer-facing docs, license, module definition, and the single version source of truth.

- `CLAUDE.md` — this file: architecture summary, build/test commands, workflow routing rules
- `README.md` — feature overview, quick start, command reference, multi-harness/6-primitive architecture
- `MIGRATION.md` — upgrade paths across skill-location, scaffold-stamp, and `pluma/` → `.fledge/pluma/` moves
- `RELEASING.md` — version-bump locations (`VERSION` + `internal/cli/version.go`), release workflow, dogfood refresh step
- `VERSION` — single source of truth for the binary version (currently 0.5.5)
- `go.mod` — module `github.com/Harrison-Blair/fledge`, Go 1.26.4; direct deps `goccy/go-yaml`, `rogpeppe/go-internal`
- `LICENSE` — AGPL v3

**Look here for**: build/test commands, release process, terminology decoder (README), version bump checklist.

## `cmd` (CLI binary entry point + acceptance tests)

**Purpose**: the actual `fledge` executable's `main()`, plus the full testscript/txtar acceptance-test suite that exercises every command end-to-end.

- `cmd/fledge/main.go` — 11-line bridge: `cli.Run(os.Args[1:])`
- `cmd/fledge/main_test.go` — `TestMain`/`TestScripts` wiring for `testscript`, with hermetic git env isolation
- `cmd/fledge/testdata/*.txtar` — 23 acceptance-test files, one per command or cross-command scenario (init, new, status, preen/check, brood/lock, nest, vee/graph, criteria, ready, set, scan, colony/report, agents, e2e, forager_contract, init_agents, unfledged, preen_scaffold, stamp_warning, nest_status, refresh_scaffold, plan_delegation)

**Look here for**: how any CLI command is expected to behave end-to-end (each txtar is executable spec), adding acceptance coverage for a new command or flag, understanding the git-isolated test harness pattern.

## `docs` (design reference / research notes)

**Purpose**: locked multi-harness generalization design decisions, and a separate, apparently-unconnected AI-infrastructure cost-routing research note.

- `docs/generalization-plan.md` — locked Q1–Q23 design decisions, bootstrap/core/adapters architecture, the (now-6, historically-7) primitive contract, manifest schema, target-adapter list (Claude/pi/Codex/Cursor/opencode), milestones M0–M5
- `docs/google_ai_mode_response.md` — multi-tier AI routing cost-optimization proposal (unrelated to fledge's own architecture; likely archived/speculative)
- `docs/research_prompt.md` — prompt template used to generate the above research note

**Look here for**: historical context on *why* the bootstrap/adapter design looks the way it does, and the primitive-contract's original rationale. Not authoritative for current behavior — cross-check against `internal/bootstrap` source, since the doc predates the primitive count dropping from 7 to 6 (no `spawn-pool`).

## `internal/bootstrap` (embedded scaffold/adapter system)

**Purpose**: embeds and writes the agent-neutral core skills and per-harness adapter configs that `fledge init` scaffolds into a target repo.

- `bootstrap.go` — `embed.FS` for `core/` + `adapters/`
- `primitives.go` — the 6-primitive contract, tier A/B/C definitions, `DeriveTier`
- `registry.go` — manifest loading, adapter detection, the 6 write policies, `WriteCore`/`WriteAdapter`
- `stamp.go` — `Stamp`/`.fledge/scaffold.json`, `ExpectedFiles`
- `drift.go` — 5-status drift classification (up-to-date/stale/modified/missing/obsolete)
- `adapters/{claude,codex,pi}/manifest.yaml` — one manifest per harness; claude is tier C, codex/pi are tier A
- `core/skills/fledge-orchestrate/{SKILL.md,planning.md,implementation.md,foraging.md,worker-protocols.md,templates/}` — the actual agent-neutral workflow prose
- `core/skills/fledge-interrogate/SKILL.md` — plumage-interrogation skill

**Look here for**: adding/changing a harness adapter (edit a manifest, no Go code), changing the orchestration workflow prose itself (edit `core/`, then run `fledge init --refresh` in this repo to regenerate scaffolded output), tier/primitive semantics, scaffold drift/refresh logic.

## `internal/cli` (command dispatch + all 18 command implementations)

**Purpose**: routes CLI args to subcommand handlers, formats `--json`/human output, emits exit codes; implements all domain commands.

- `cli.go` — registry (`commands` map, `Run()`, `commandOrder`), exit codes, `.fledge` root search, stamp-mismatch warning
- One file per command: `init.go`, `new.go`, `status.go`, `brood.go` (brood/abandon/broods), `preen.go`, `nest.go` (5 subcommands), `set.go`, `criteria.go`, `scan.go`, `vee.go`, `ready.go`, `unfledged.go`, `update.go`, `agents.go`, `colony.go`
- `specload.go` — shared `loadSet()` used by every spec-touching command
- `fledgeignore.default` — default scan exclusions embedded into new repos

**Look here for**: any CLI command's exact flags/behavior/exit codes, adding a new command (register + commandOrder + usage), the legal-transition state machines (`taskTransitions`/`reqTransitions` in `status.go`).

## `internal/spec` (spec frontmatter, IDs, templates)

**Purpose**: parses/writes `PLM-###`/`FTHR-###` markdown spec files — frontmatter, ID allocation, body-preserving mutation, and the skeleton templates used by `fledge new`.

- `types.go` — `Requirement`, `Task` structs; status constants
- `frontmatter.go` — `SplitFrontmatter`, parse/render, atomic writes
- `ids.go` — `NextID`, `AllocateAndCreate` (flock-serialized on `.alloc.lock`), `Kebab`
- `load.go` — `Load()` → `Set{Reqs, Tasks, Errors, UnknownFields}`
- `criteria.go` — checkbox parsing/mutation (`- [ ] AC-N: text`)
- `templates.go` + `templates/{plumage,feather}.md` — skeleton bodies for `fledge new`

**Look here for**: spec file format/frontmatter key order, ID allocation and concurrency-safety mechanics, acceptance-criteria checkbox format, adding a field to plumage/feather frontmatter.

## `internal-misc`: `internal/check`, `internal/ciconfig`, `internal/doctest`, `internal/graph`, `internal/hooktest`, `internal/lock`, `internal/nest`, `internal/repo`, `internal/scan`

**Purpose**: nine small, independent utility packages (merged into one scout assignment for context-gathering efficiency; each is its own real Go package).

- `internal/check` — `check.Run()`: ~20 validation rules (preen) — parse, schema, duplicate-id, dangling-ref, cycle, brood-consistency, criteria-complete/evidence
- `internal/graph` — `graph.New()`: cycle detection, topological waves, ready-set computation (vee)
- `internal/lock` — `lock.Acquire/Release/Get/List`: atomic `.brood` file operations via `os.Link`
- `internal/nest` — `Doc`/`Kind`/`Status()`/`IsStub()`/`RefreshDoc()`: the concern-doc and scout-report scaffolding this very document is part of
- `internal/repo` — `repo.Find()`, directory accessors (`FledgeDir`, `TasksDir`, `ContextDir`, etc.)
- `internal/scan` — `scan.Run()`: file enumeration + `.fledgeignore` filtering + module grouping (what `fledge scan` reports, and what the forager pipeline's step 1 relies on)
- `internal/ciconfig` — tests asserting on `.github/workflows/*.yml` shape (no runtime code)
- `internal/doctest` — tests asserting README/RELEASING mention certain commands (no runtime code)
- `internal/hooktest` — end-to-end tests of `scripts/hooks/pre-commit` against real temp git repos

**Look here for**: preen rule definitions (`internal/check`), brood/lock atomicity mechanics, the nest scaffold/status logic itself, `.fledgeignore` filtering behavior, dependency-wave computation for `fledge vee`/`fledge ready`.
