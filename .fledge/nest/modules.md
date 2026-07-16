---
generated: 2026-07-15T23:53:12Z
commit: a4d02e8187c64ef9f3f1231052990b282207420b
agent: fledge-forager
fledge_version: 0.5.5
---

# Modules

Repo map: every top-level module (per `fledge scan`), its purpose, its key files, and where to look for what.

## `.github` (+ `scripts`)
**Purpose:** GitHub Actions CI/release workflows and the local dev tooling that mirrors them.
**Key files:** `.github/workflows/pr-check.yml` (PR gate: gofmt, go vet, go build, go test), `.github/workflows/release.yml` (VERSION-change-triggered 4-platform release: linux/darwin × amd64/arm64), `scripts/hooks/pre-commit` (local mirror of CI lint gates), `scripts/install.sh` (build/install/verify the `fledge` binary, optional `--refresh`).
**Look here for:** CI trigger conditions, release/versioning mechanics, local git hook setup (`git config core.hooksPath scripts/hooks`).

## `<root>`
**Purpose:** Project metadata, licensing, and top-level docs; the Go module root.
**Key files:** `go.mod` (Go 1.26, deps: `goccy/go-yaml`, `rogpeppe/go-internal`), `VERSION` (single source of truth for release + binary version), `CLAUDE.md` (architecture/build guide), `README.md` (quick start, 6-primitive contract), `RELEASING.md` (release checklist), `MIGRATION.md` (upgrade guides across breaking changes).
**Look here for:** build/test commands, release process steps, version-bump locations, license terms (AGPL v3).

## `cmd`
**Purpose:** The `fledge` binary's entry point and its full black-box acceptance test suite.
**Key files:** `cmd/fledge/main.go` (delegates to `cli.Run()`), `cmd/fledge/main_test.go` (testscript harness, deterministic git identity), `cmd/fledge/testdata/*.txtar` (23 acceptance tests, one per command/behavior area — see `testing.md`).
**Look here for:** what each CLI command is expected to do end to end; the authoritative behavior spec when `internal/cli` source is ambiguous.

## `docs`
**Purpose:** Historical design/research notes — not part of the shipped tool.
**Key files:** `docs/generalization-plan.md` (23-decision design doc behind the 0.1.0→0.2.0 adapter refactor), `docs/google_ai_mode_response.md` (unrelated multi-tier AI routing infrastructure prototype), `docs/research_prompt.md` (meta-template for AI-generated infra proposals).
**Look here for:** the *why* behind the adapter/primitive architecture (generalization-plan.md only); the other two files are unrelated exploratory content, not current design docs — don't treat them as live specs.

## `internal/bootstrap` (adapters)
**Purpose:** Per-harness scaffold manifests mapping fledge's 6 primitives to concrete mechanisms.
**Key files:** `internal/bootstrap/adapters/claude/manifest.yaml` (Tier C: 6/6 primitives, team-loop, 5 agent defs), `internal/bootstrap/adapters/codex/manifest.yaml` and `internal/bootstrap/adapters/pi/manifest.yaml` (Tier A: 4/6, solo execution).
**Look here for:** what a harness can/can't do (tier), what files `fledge init` writes for a given adapter, harness-specific runtime config (`team-loop.md`, `settings.json`).

## `internal/bootstrap` (core skills)
**Purpose:** The single agent-neutral source of the fledge-orchestrate/fledge-interrogate workflow prose — the actual planning/implementation/foraging protocol text, written verbatim into every scaffolded repo's `.fledge/skills/`.
**Key files:** `internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md` (router), `planning.md`, `implementation.md`, `foraging.md` (this protocol), `worker-protocols.md` (Incubator/Brooder/Skua specs), `templates/*.md` (context-doc, feather, plumage, scout-report conventions).
**Look here for:** the authoritative definition of every workflow phase, worker role, spec lifecycle state, and bird-themed term used throughout this repo and its scaffolded output.

## `internal/bootstrap` (Go implementation)
**Purpose:** The manifest-driven registry and file-write machinery `fledge init`/`--refresh`/`preen` run on.
**Key files:** `internal/bootstrap/bootstrap.go` (`//go:embed core adapters`), `registry.go` (`Manifest`, `WriteCore`, `WriteAdapter`, write-policy dispatch), `primitives.go` (6 primitives, `DeriveTier`), `stamp.go` (`Stamp`/`StampEntry`, `.fledge/scaffold.json`), `drift.go` (`DriftReport`, 5-state classification).
**Look here for:** how scaffolding, tier derivation, drift detection, and `--refresh` actually work; the struct definitions behind `.fledge/scaffold.json`.

## `internal/{check,ciconfig,doctest,graph,hooktest,lock,repo,scan}`
**Purpose:** Eight small single-purpose support packages: spec validation (`check`), dependency graph analysis (`graph`), feather claim locking (`lock`), repo-root/path helpers (`repo`), module scanning (`scan`), plus three test-only packages that structurally assert CI workflows (`ciconfig`), root docs (`doctest`), and the pre-commit hook (`hooktest`) stay in sync with what the code does.
**Key files:** `internal/check/check.go` (`Run` → `[]Finding`, backs `fledge preen`), `internal/graph/graph.go` (`Cycle`/`Waves`/`Ready`, backs `fledge vee`/readiness), `internal/lock/lock.go` (`Acquire`/`Release`/`Get`/`List`, O_EXCL atomic, backs `fledge brood`), `internal/repo/repo.go` (`Find`, `FledgeDir`, `ContextDir`, etc.), `internal/scan/scan.go` (`Run`, backs `fledge scan`).
**Look here for:** the exact validation rules `fledge preen` enforces, cycle/wave/readiness algorithms, brood-file locking semantics, and the CI/docs/hook consistency checks (useful before editing `.github/workflows/*`, README/RELEASING, or `scripts/hooks/pre-commit`, since these tests will catch drift).

## `internal/cli`
**Purpose:** Implements all 24 `fledge` subcommands: argument dispatch, output formatting (human + `--json`), shared exit codes.
**Key files:** `internal/cli/cli.go` (dispatch table, `Run()`, exit codes), one file per command family (`init.go`, `new.go`, `nest.go`, `preen.go`, `brood.go`, `vee.go`, `colony.go`, `status.go`, `set.go`, `criteria.go`, `ready.go`, `unfledged.go`, `scan.go`, `agents.go`, `update.go`, `version.go`), `specload.go` (shared `loadSet()` helper).
**Look here for:** exact CLI flags/behavior per command; see `entry-points.md` for the full command table.

## `internal/nest`
**Purpose:** Schema, rendering, and completeness-checking for `.fledge/nest/` documents — the Go implementation behind `fledge nest scaffold/scout/stamp/status`, i.e. the machinery that produced the very files you're reading.
**Key files:** `internal/nest/nest.go` (`Doc`, `Frontmatter()`, `Status()`, `RefreshDoc()`), `internal/nest/docs.go` (`ConcernDocs` ordered list, `IsKnownDoc`), `internal/nest/templates/*.md` (scaffold stub templates).
**Look here for:** what `fledge nest status` actually checks to decide "complete" (see `testing.md`); stub-detection semantics; frontmatter key ordering per doc kind.

## `internal/spec`
**Purpose:** Core spec domain model — frontmatter parsing/rendering, PLM-###/FTHR-### ID allocation, acceptance-criteria checkbox mutation, embedded plumage/feather markdown templates.
**Key files:** `internal/spec/types.go` (`Requirement`, `Task` structs), `frontmatter.go` (`SplitFrontmatter`, atomic writes), `ids.go` (`NextID`, `AllocateAndCreate`, flock-serialized), `criteria.go` (`ParseCriteria`, `SetCriterion`), `templates.go` + `templates/*.md` (skeleton bodies for `fledge new`).
**Look here for:** the authoritative shape of plumage/feather frontmatter and body sections; how IDs are allocated safely under concurrency; how acceptance-criteria checkboxes are parsed and toggled byte-exactly.
