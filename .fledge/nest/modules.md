---
generated: 2026-07-15T18:14:39Z
commit: 5728c29953a7c218c923ce20333dbffebb00623f
agent: fledge-forager
fledge_version: 0.5.4
---

# Modules

Repo map: each top-level module (per `fledge scan`), its purpose, key files, and where to look for what.

## `<root>`

Repository configuration, contributor docs, release automation.

Key files: `VERSION` (source-of-truth version), `go.mod` (module `github.com/Harrison-Blair/fledge`, Go 1.26.4+), `README.md` (quickstart, CLI reference, architecture overview), `CLAUDE.md` (contributor/agent guide — routes to `.fledge/skills/fledge-orchestrate/SKILL.md`), `RELEASING.md` (version-bump checklist), `MIGRATION.md` (0.1→0.4 upgrade paths), `LICENSE` (AGPLv3).

Look here for: how to build/test/install fledge, release process, version-sync requirements across files, upgrade-path history.

## `.github` (merged into `root` scout assignment)

CI/CD workflow definitions.

Key files: `.github/workflows/pr-check.yml` (PR trigger: gofmt, vet, build, test), `.github/workflows/release.yml` (push-to-main trigger: version-diff detection, conditional 4-platform build, `gh release create`).

Look here for: what CI checks on every PR, how releases are triggered and built.

## `scripts` (merged into `root` scout assignment)

Optional local developer tooling.

Key files: `scripts/install.sh` (build+install+verify, `--refresh` option), `scripts/hooks/pre-commit` (opt-in local gate: gofmt -l, go vet; activated via `git config core.hooksPath scripts/hooks`).

Look here for: how contributors install fledge locally and enable local lint gating (not installed by default).

## `cmd`

CLI entry point and full acceptance test suite.

Key files: `cmd/fledge/main.go` (12-line entry point, calls `internal/cli.Run`), `cmd/fledge/main_test.go` (testscript harness, isolates git identity/config for determinism), `cmd/fledge/testdata/*.txtar` (20 testscript files, one per command/workflow area: init, init_agents, agents, new, status, set, check, criteria, preen_scaffold, refresh_scaffold, graph, lock, ready, nest, scan, report, e2e, unfledged, plan_delegation, stamp_warning).

Look here for: exact CLI behavior contracts (every observable command output/exit-code is pinned by a txtar assertion) — the authoritative spec for "what does this command actually do." Any change to `internal/bootstrap/core/` or `adapters/` content must update `init.txtar`, `init_agents.txtar`, `agents.txtar`.

## `docs`

Standalone design/research artifacts, not part of the runtime.

Key files: `docs/generalization-plan.md` (superseded 7-primitive design doc for the bootstrap/adapter architecture — current code has 6 primitives, see architecture.md), `docs/google_ai_mode_response.md` and `docs/research_prompt.md` (unrelated multi-tier-AI-routing research brief and its generator template — no integration with fledge's CLI or orchestration workflow).

Look here for: historical design rationale for the bootstrap/adapter split (with caution — cross-check against current `internal/bootstrap/` code, since this doc predates the current primitive set).

## `internal/bootstrap` — core skills (`internal-bootstrap-core` scout assignment)

The agent-neutral orchestration workflow prose plus the bootstrap package's Go logic (embedding, manifest/registry, primitives, stamping, drift).

Key files: `internal/bootstrap/bootstrap.go` (`//go:embed core adapters`), `primitives.go` (6-primitive contract, `DeriveTier`), `registry.go` (`Manifest`, `ManifestFile` write policies, `WriteCore`/`WriteAdapter`, `PruneObsolete`), `stamp.go` (`Stamp`/`StampEntry`, `ExpectedFiles`), `drift.go` (`DriftStatus` enum, `DriftReport`, `EditedOnRefresh`), and `core/skills/fledge-orchestrate/{SKILL.md, planning.md, implementation.md, foraging.md, worker-protocols.md, templates/}`, `core/skills/fledge-interrogate/SKILL.md`.

Look here for: the actual planning/implementation workflow text agents follow (planning.md, implementation.md, foraging.md, worker-protocols.md — including the Skua review checklist and verdict rules), the scaffold write-policy and drift-detection mechanics, and the plumage/feather/scout-report body templates.

## `internal/bootstrap` — adapters (`internal-bootstrap-adapters` scout assignment)

Per-harness (Claude, Codex, Pi) manifest-driven scaffolding: agent definitions, primitive-to-mechanism mappings, harness-specific runtime docs.

Key files: `adapters/claude/manifest.yaml` + `agents/fledge-{brooder,skua,forager,context-scout,incubator}.md` + `team-loop.md` (Claude-only piping file: tmux display, orchestrator naming, spawning, shutdown, recovery) + `settings.json`/`settings.local.json`; `adapters/codex/manifest.yaml` + `fledge-adapter.md` (Tier A); `adapters/pi/manifest.yaml` + `fledge-adapter.md` + `prompts/fledge-{plan,implement}.md` + `settings.json` (Tier A).

Look here for: exactly how each harness realizes the 6 primitives (Claude=all 6/Tier C, Codex/Pi=4/Tier A), the Claude skua/brooder/forager/incubator/context-scout system prompts, and the tmux precondition/fallback behavior in `team-loop.md` § Teammate display (tmux).

## `internal/cli`

Command dispatch, argument parsing, output formatting, exit codes — the deterministic CLI surface.

Key files: `cli.go` (dispatch, `commandOrder`, `Run()`), `specload.go` (`loadSet()` shared load pattern), one file per verb (`init.go`, `new.go`, `status.go`, `criteria.go`, `preen.go`, `vee.go`, `brood.go`, `colony.go`, `ready.go`, `unfledged.go`, `set.go`, `scan.go`, `agents.go`, `nest.go`, `version.go`, `update.go`), `fledgeignore.default`.

Look here for: how a CLI flag/subcommand maps to domain-package calls, JSON output shapes, exit-code semantics, status-transition legality tables.

## `internal/domain` (check, graph, lock, nest, repo, scan, spec, ciconfig, doctest, hooktest — `internal-domain` scout assignment)

The deterministic, agent-agnostic packages behind every CLI command.

Key files: `internal/spec/{types,frontmatter,ids,load,templates,criteria}.go` (spec parsing/rendering/ID-allocation/checkbox-editing), `internal/check/check.go` (`Run`/`Finding`, all preen validation rules), `internal/graph/graph.go` (cycle detection, waves, ready-set), `internal/lock/lock.go` (`Record`, atomic-link-based brood acquire/release), `internal/nest/{nest,docs}.go` (concern-doc/scout-report `Doc` type, frontmatter key order, stub bodies), `internal/repo/repo.go` (git-root discovery, `.fledge/*` path accessors), `internal/scan/scan.go` (module grouping for `fledge scan`), plus `internal/ciconfig`, `internal/doctest`, `internal/hooktest` (test-only packages asserting on root CI/doc/hook files).

Look here for: spec frontmatter schema and YAML rendering rules, ID allocation locking, acceptance-criteria checkbox byte-level editing, dependency-cycle/wave/ready-set algorithms, brood lock file format and race handling, `.fledge/nest/` document schema.

## Open Questions

None outstanding beyond what's carried into other concern docs.
