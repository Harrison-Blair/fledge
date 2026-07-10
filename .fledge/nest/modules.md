---
generated: 2026-07-10T14:50:00Z
commit: 7678344ab9a18730530b9f6edf507ad0c449d352
agent: fledge-forager
fledge_version: 0.2.1
---

# Modules

Repo map: each top-level module (per `fledge scan`), its purpose, key files, and where to look for what.

## root (+ scripts)

**Purpose**: Repository-level metadata, versioning, licensing, and install scaffolding. The tiny `scripts/` module (1 file) is folded in here.

**Key files**: `README.md` (overview, quick start, architecture, CLI reference), `CLAUDE.md` (agent guidance: build/test/run, conventions), `VERSION` (semver source of truth, injected via ldflags), `go.mod` (Go 1.26.4; deps `github.com/goccy/go-yaml`, `github.com/rogpeppe/go-internal`), `LICENSE` (AGPLv3), `.gitignore`, `scripts/install.sh` (build+install+verify, optional `--refresh`).

**Look here for**: how to build/install the binary, versioning scheme, license terms, top-level project framing.

## cmd

**Purpose**: The CLI's thin entry point plus the entire testscript-based acceptance test suite.

**Key files**: `cmd/fledge/main.go` (one line: `cli.Run(os.Args[1:])`), `cmd/fledge/main_test.go` (TestMain/TestScripts harness, deterministic git env), `cmd/fledge/testdata/*.txtar` (18 files — one per command/feature area: init, init_agents, agents, new, status, criteria, set, check, scan, graph, ready, lock, report, unfledged, nest, e2e).

**Look here for**: expected CLI behavior/output for every command (human + `--json`), exit-code conventions in practice, end-to-end lifecycle flow (`e2e.txtar`).

## docs

**Purpose**: Design documentation and research artifacts, primarily the generalization roadmap for making fledge portable across agent harnesses.

**Key files**: `docs/generalization-plan.md` (locked 23-decision design spec: primitive contract, manifest format, adapter testing strategy, milestones M0–M5), `docs/google_ai_mode_response.md` (unrelated example AI-infra-proposal artifact), `docs/research_prompt.md` (prompt template used to produce that example).

**Look here for**: the rationale behind the primitive/tier/adapter design (before touching `internal/bootstrap`), planned-but-not-yet-implemented harness support (Cursor, opencode), open TBDs for Codex/pi Tier C.

## internal (bootstrap)

**Purpose**: Embeds the agent-neutral orchestration workflow (`core/`) and per-harness adapter manifests (`adapters/`) into the binary; implements `fledge init`/`fledge agents`.

**Key files**: `internal/bootstrap/bootstrap.go` (`//go:embed core adapters`), `internal/bootstrap/registry.go` (`Manifest`, `LoadAdapters`, `WriteCore`, `WriteAdapter`, `DeriveTier`), `internal/bootstrap/primitives.go` (6-primitive contract, tier derivation), `internal/bootstrap/core/skills/fledge-orchestrate/*` (SKILL.md, planning.md, implementation.md, foraging.md, worker-protocols.md, templates/), `internal/bootstrap/adapters/{claude,pi,codex}/manifest.yaml` + per-harness files.

**Look here for**: adding/changing a harness adapter, changing the orchestration workflow prose, how tiers/primitives are derived, scaffolding write policies.

## internal (cli)

**Purpose**: Command dispatch and every CLI subcommand implementation; the user-facing surface that calls into domain packages.

**Key files**: `internal/cli/cli.go` (dispatcher, `commandOrder`, exit codes), `internal/cli/specload.go` (shared `loadSet`/`lockedTaskIDs` helpers), one file per command — `agents.go`, `brood.go`, `colony.go`, `criteria.go`, `init.go`, `nest.go`, `new.go`, `preen.go`, `ready.go`, `scan.go`, `set.go`, `status.go`, `unfledged.go`, `vee.go`, `version.go` — plus `fledgeignore.default` (embedded default ignore rules) and `lock_test.go`/`version_test.go`.

**Look here for**: exact flag/argument shape of any `fledge <command>`, how a command's `--json` output is structured, where a new subcommand would be registered.

## internal (domain: check, graph, lock, nest, repo, scan, spec)

**Purpose**: Focused, dependency-free domain logic that the CLI commands delegate to — spec parsing/rendering, validation, dependency graph analysis, feather locking, context-doc scaffolding, repo/path resolution, file enumeration.

**Key files**: `internal/spec/{types,frontmatter,ids,load,criteria,templates}.go` + `templates/{plumage,feather}.md` (spec model, ID allocation, criteria checkbox parsing), `internal/check/check.go` (preen validation rules, `Finding`/`Severity`), `internal/graph/graph.go` (vee: cycle detection, topological waves, ready-set), `internal/lock/lock.go` (brood: O_EXCL advisory locks, `Record`, `HeldError`), `internal/nest/{nest,docs}.go` + `templates/{concern-doc,index,scout-report}.md` (nest doc frontmatter/body rendering, `ConcernDocs` closed set), `internal/repo/repo.go` (git-root discovery, `.fledge/` path resolution), `internal/scan/scan.go` (`fledge scan`'s module grouping).

**Look here for**: the spec YAML/markdown schema, validation rule list (preen), dependency-graph algorithms, how feather claims/locks work, how `.fledge/nest/` scaffolding templates are structured internally.

## pluma

**Purpose**: This repo's own spec-driven-development directory — plumage (feature intent) and feather (implementable task) markdown specs tracking fledge's own development. Direct evidence of dogfooding.

**Key files**: `pluma/plumage/PLM-001`…`PLM-008` (feature-intent specs; PLM-001–003 fledged, PLM-004–008 hatched/not-yet-dispatched), `pluma/feathers/FTHR-001`…`FTHR-008` (implementable tasks; FTHR-001–007 fledged, FTHR-008 pipping).

**Look here for**: what fledge features are planned vs. shipped, acceptance-criteria detail behind any shipped command, the spec frontmatter/prose schema in worked examples.
