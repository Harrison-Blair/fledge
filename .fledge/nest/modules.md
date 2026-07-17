---
generated: 2026-07-17T02:54:09Z
commit: e7a6d4969f861ed3f03af7833b750a7cd703a7a8
agent: fledge-forager
fledge_version: 0.5.8
---

# Modules

Repo map: each module, its purpose, key files, and where to look for what.

## root (repo root files, `.github/`, `docs/`, `scripts/`)

Purpose: repo-level docs, CI workflows, release scripts, and design-history material; not Go source.
Key files: `CLAUDE.md` (agent guidance), `README.md` (public overview, terminology decoder), `RELEASING.md` (release process), `MIGRATION.md` (upgrade paths 0.1.0→0.4.0), `VERSION`, `go.mod` (Go 1.26, deps: `goccy/go-yaml`, `rogpeppe/go-internal`), `.github/workflows/pr-check.yml` and `release.yml`, `scripts/hooks/pre-commit`, `scripts/install.sh`, `docs/generalization-plan.md` (locked multi-agent design decisions).
Look here for: build/release process, terminology definitions, CI gate behavior, historical design rationale (docs/).

## cmd (`cmd/fledge/`)

Purpose: the CLI binary's sole entry point plus its acceptance test suite.
Key files: `cmd/fledge/main.go` (11 lines — `main()` → `cli.Run(os.Args[1:])`), `cmd/fledge/main_test.go` (testscript harness registration), `cmd/fledge/testdata/*.txtar` (27 acceptance test scripts, one per command/scenario).
Look here for: how the binary starts, the full acceptance-test suite (testscript/txtar) covering every CLI command end-to-end.

## internal/cli (`internal/cli/`)

Purpose: command dispatch layer — argument parsing, subcommand routing, exit codes, `--json` output. 19 commands, each a thin file delegating to a domain package.
Key files: `cli.go` (`Run`, `command` registration, `commandOrder`, exit codes), `init.go` (538 lines — `fledge init`, scaffold/dev-mode/refresh), `nest.go` (364 lines — `fledge nest` family), `update.go` (341 lines — self-update via GitHub releases), `brood.go`, `preen.go`, `vee.go`, `colony.go`, `criteria.go`, `set.go`, `new.go`, `status.go`, `roster.go`, `heartbeat.go`, `scan.go`, `agents.go`, `ready.go`, `unfledged.go`, `version.go`, `specload.go` (shared helpers), `fledgeignore.default`.
Look here for: the full CLI command surface, exit code conventions, flag conventions (`--json`, `--force`, `--strict`), how commands compose domain packages.

## internal/spec (`internal/spec/`)

Purpose: the on-disk spec file format — YAML frontmatter + byte-preserved markdown body — for plumages (PLM-###) and feathers (FTHR-###). ID allocation, lifecycle status constants, acceptance-criteria checkbox parsing, and the embedded spec templates all live here. **Critical for data-model.md and conventions.md accuracy.**
Key files: `types.go` (Requirement/Task structs, status constants), `frontmatter.go` (250 lines — parse/render/atomic write), `ids.go` (125 lines — `NextID`, `AllocateAndCreate` with flock), `criteria.go` (89 lines — AC checkbox parse/toggle), `load.go` (batch loading into `Set`), `templates.go` + `templates/feather.md`, `templates/plumage.md`.
Look here for: exact frontmatter field lists, valid status values, ID allocation scheme, acceptance-criteria format rules.

## internal/bootstrap (core skills) (`internal/bootstrap/core/skills/`)

Purpose: the single agent-neutral source of the orchestration workflow prose — what gets embedded and scaffolded to `.fledge/skills/` in every target repo. Defines the fledge-orchestrate and fledge-interrogate skills, planning/implementation/foraging phases, and worker-role protocols (incubator, brooder, skua, forager/scout).
Key files: `skills/fledge-orchestrate/SKILL.md` (router), `planning.md`, `implementation.md`, `foraging.md`, `incubator.md`, `brooder.md`, `skua.md`, `worker-protocols.md` (now a stub pointing to the three per-role files), `templates/{plumage,feather,context-doc,scout-report}.md`, `skills/fledge-interrogate/SKILL.md`.
Look here for: the actual planning/implementation/foraging process steps, worker spawn-prompt contracts, gate/message envelope semantics, digest file conventions.

## internal/bootstrap (adapters) (`internal/bootstrap/adapters/`)

Purpose: thin, format-only per-harness mappings (claude, codex, pi) of the 6 primitives to concrete harness mechanisms. No workflow logic lives here — only manifests and format glue.
Key files: `claude/manifest.yaml` + `claude/agents/*.md` (5 agent definitions) + `claude/settings.json`/`settings.local.json` + `claude/team-loop.md` (Tier C, full team support); `codex/manifest.yaml` (Tier A, solo); `pi/manifest.yaml` + `pi/prompts/{fledge-plan,fledge-implement}.md` + `pi/settings.json` (Tier A, solo).
Look here for: how a given harness realizes `confirm-gate`/`spawn-worker`/etc., what files `fledge init` writes into `.claude/`, `.codex/`, `.pi/`, and their write policies.

## internal/bootstrap (Go implementation) (`internal/bootstrap/*.go`)

Purpose: the Go code that embeds core/adapters (`//go:embed`), loads manifests, derives tiers, writes/refreshes scaffolded files, detects drift, and (new, PLM-031) stamps `.fledge/scaffold.json` including dev-install symlink mode.
Key files: `bootstrap.go` (embed FS), `primitives.go` (`PrimitiveOrder`, `DeriveTier`), `registry.go` (`Manifest`, `WriteCore`, `WriteAdapter`, file write policies, `WriteOpts`), `drift.go` (`DriftStatus`, `DriftReport`, `EditedOnRefresh`), `stamp.go` (new — `Stamp`, `StampEntry`, `LoadStamp`, `Write`, `ExpectedFiles`/`ExpectedFilesDev`, `ValidateDevSource`, `writeDevLink`), plus 15 test files covering contract/expected-files/drift/refresh/prose invariants.
Look here for: exactly how scaffolding, drift detection, and dev-mode (`fledge init --dev`) are implemented; the source of truth for `.fledge/scaffold.json`'s shape.

## internal/check, internal/graph, internal/ciconfig, internal/doctest, internal/hooktest (small internal packages, merged scout)

Purpose:
- `internal/check` — spec validation, backs `fledge preen` (`check.go:Run`, `Finding`, `Severity`; 15 named rules e.g. `dangling-ref`, `cycle`, `criteria-incomplete`)
- `internal/graph` — dependency graph/cycle detection/topological waves, backs `fledge vee` (`graph.go:New`, `Cycle`, `Waves`, `Ready`)
- `internal/ciconfig` — test-only; asserts CI workflow YAML structure (release.yml, pr-check.yml) matches expectations
- `internal/doctest` — test-only; asserts README.md/RELEASING.md/CLAUDE.md cross-references stay accurate
- `internal/hooktest` — test-only; end-to-end tests of `scripts/hooks/pre-commit` against real git repos
Look here for: validation rule definitions (`check`), dependency-cycle/readiness logic (`graph`), and the meta-tests that keep CI config and docs honest (`ciconfig`/`doctest`/`hooktest`).

## internal/ledger, internal/nest, internal/repo, internal/roster, internal/scan (small internal packages, merged scout)

Purpose:
- `internal/ledger` (new) — deterministic agent-handoff ledger under `.fledge/ledger/`; atomic (subject, kind)-addressed records (status/verdict/escalation), backs `fledge heartbeat`
- `internal/nest` — schemas, embedded templates, and status-checking for `.fledge/nest/` context docs (concern docs + `raw/` scout reports); backs `fledge nest`
- `internal/repo` — git-root resolution and standardized `.fledge/` subdirectory path accessors
- `internal/roster` — worker species-name allocation (18 canonical names) with pair/overflow semantics, backs `fledge roster`
- `internal/scan` — repo file/module enumeration via `git ls-files` + `.fledgeignore`, backs `fledge scan`
Look here for: the ledger record schema (`ledger.go`), the nest doc/status schema this very document set conforms to (`nest.go`, `docs.go`), `.fledge/` path conventions (`repo.go`), species-name allocation rules (`roster.go`), and module/file-listing rules (`scan.go`).

## internal/lock (`internal/lock/`)

Purpose: advisory feather-claim (brood) files under `.fledge/broods/`; atomic exclusive-claim mechanism via `os.Link`, backs `fledge brood`/`abandon`/`broods`.
Key files: `lock.go` (`Record`, `Acquire`, `Release`, `Get`, `List`, `HeldError`), `lock_test.go` (8 tests incl. concurrency/corruption resilience).
Look here for: brood file format (`<FTHR-ID>.brood`), atomicity guarantees, how a claim maps to feather ownership.
