---
generated: 2026-07-10T20:53:53Z
commit: f28efebd76d6aa135adb0956a3337a40a8d98351
agent: fledge-forager
fledge_version: 0.3.0
---

# Modules

Repo map: one entry per top-level module (matching `fledge scan` output), what it's for, its key files, and where to look for what.

## `<root>`
**Purpose:** Project metadata, license, top-level docs, and the one build/install script. Includes `scripts/install.sh` (merged in — 1 file, 1735 bytes, too small for its own scout assignment).

**Key files:** `CLAUDE.md` (architecture + conventions for agents), `README.md` (overview, quick start, command reference), `MIGRATION.md` (0.1.0→0.2.0→0.3.0 upgrade paths), `VERSION` (semver, currently 0.3.0), `go.mod` (Go 1.26 module), `LICENSE` (AGPL v3), `scripts/install.sh` (build/install/verify the `fledge` binary, `--refresh` option).

**Look here for:** the canonical build/install/verify sequence, version history and migration notes, license terms, top-of-repo agent guidance.

## `cmd`
**Purpose:** The compiled binary's entry point and the entire CLI acceptance-test suite.

**Key files:** `cmd/fledge/main.go` (11-line `main()`, delegates to `internal/cli.Run`), `cmd/fledge/main_test.go` (testscript harness: `TestMain` registers the `fledge` command, `TestScripts` runs every `.txtar` fixture with git env isolation), `cmd/fledge/testdata/*.txtar` (20 fixture files, one per command/feature area — `init.txtar`, `init_agents.txtar`, `agents.txtar`, `nest.txtar`, `preen_scaffold.txtar`, `refresh_scaffold.txtar`, `stamp_warning.txtar`, `e2e.txtar`, etc.).

**Look here for:** end-to-end proof of CLI behavior (human + `--json` output, exit codes, error messages); the fixtures that MUST be updated whenever `internal/bootstrap/core` or `internal/bootstrap/adapters` content changes (`init.txtar`, `init_agents.txtar`, `agents.txtar` especially — they assert on scaffolded output byte-for-byte).

## `docs`
**Purpose:** Reference/design prose, not executable. Mixed relevance to the current codebase.

**Key files:** `docs/generalization-plan.md` (334-line locked design doc for the 0.2.0→0.3.0 core+adapters generalization — largely realized in current `internal/bootstrap`; 23 resolved design decisions, the origin of the 6/7-primitive contract and manifest-driven init), `docs/google_ai_mode_response.md` (unrelated: 3-tier AI-infrastructure cost-routing brief with a Python `MultiTierHardwareHarness` sketch), `docs/research_prompt.md` (unrelated: a generic research-prompt template).

**Look here for:** the historical rationale behind the bootstrap/adapter architecture (`generalization-plan.md` only). The other two docs are not connected to fledge's design — do not treat them as authoritative for this codebase.

## `pluma`
**Purpose:** fledge's own specs — the plumage/feather set that drove (and continues to drive) this repo's own development. Dogfoods the spec-driven workflow end-to-end.

**Key files:** `pluma/plumage/PLM-001..009-*.md` (9 requirement specs, mostly fledged — colony report, unfledged listing, `fledge nest` authoring, agent detection, terminology comprehension, orphan triage, single-step authoring, brooder-skua pairing, scaffold version stamp), `pluma/feathers/FTHR-001..013-*.md` (13 implementation tasks, all fledged, each decomposing one plumage into Description/Affected Modules/Approach/Tests/Acceptance Criteria).

**Look here for:** worked examples of well-formed plumage/feather specs (frontmatter shape, section structure, AC phrasing); precedent for how feathers cite `.fledge/nest/` concern docs as context; the traceability pattern from functional criteria (FC-N) to acceptance criteria (AC-N) to txtar fixture assertions.

## `internal` (split into 3 scout assignments — 81 files / 306KB, over the split threshold)

### `internal/bootstrap`
**Purpose:** Embeds and scaffolds the agent-neutral orchestration core plus per-harness adapter manifests. The system `fledge init` runs.

**Key files:** `bootstrap.go` (`//go:embed core adapters`), `registry.go` (`Manifest`, `ManifestFile` policies, `LoadAdapters`/`FindAdapter`/`WriteCore`/`WriteAdapter`), `primitives.go` (6 primitives, `DeriveTier`), `stamp.go` (`Stamp`/`StampEntry`, `ExpectedFiles`), `drift.go` (`DriftStatus`, `DriftReport`), `adapters/claude/manifest.yaml` + `adapters/claude/agents/*.md` (Tier C: brooder/forager/scout/skua spawn prompts), `adapters/codex/manifest.yaml`, `adapters/pi/manifest.yaml` (both Tier A), `core/skills/fledge-orchestrate/*.md` (SKILL.md, planning.md, implementation.md, foraging.md, worker-protocols.md, templates/).

**Look here for:** how a harness adapter is structured (manifest + file policies + primitive map) — the pattern any new Claude adapter subagent (e.g. a new `.claude/agents/*.md` role) must follow; how tiers are derived; how scaffold drift/preserve/prune works; the actual prose of the orchestration skills that this forager and its scouts are following right now.

### `internal/cli`
**Purpose:** Command dispatch, argument parsing, output formatting, and exit codes for every `fledge` subcommand.

**Key files:** `cli.go` (`register`, `commandOrder`, `Run`, exit constants), `specload.go` (shared `loadSet()` helper), one file per command — `init.go`, `new.go`, `nest.go`, `scan.go`, `brood.go`, `status.go`, `set.go`, `criteria.go`, `preen.go`, `ready.go`, `colony.go`, `unfledged.go`, `vee.go`, `agents.go`, `version.go` — plus `fledgeignore.default` (default `scan` ignore patterns).

**Look here for:** exactly what a command validates/outputs before touching domain packages; the status-transition legality matrices (`status.go`); JSON output shapes per command (each file defines its own `*JSON`/`*Info` struct).

### `internal` core packages (check, graph, lock, nest, repo, scan, spec)
**Purpose:** Domain logic packages: spec parsing/frontmatter (`spec`), validation rules (`check`), dependency graph (`graph`), feather claim locks (`lock`), context-doc rendering (`nest`), repo discovery (`repo`), file/module scanning (`scan`).

**Key files:** `internal/spec/{types,frontmatter,ids,load,criteria,templates}.go`, `internal/check/check.go` (`Finding`, `Run`), `internal/graph/graph.go` (`Cycle`, `Waves`, `Ready`), `internal/lock/lock.go` (`Record`, `Acquire`/`Release`/`List`), `internal/nest/{nest,docs}.go` (`Doc`, `ConcernDocs`, `ClearNest`, `RefreshDoc`), `internal/repo/repo.go` (`Repo`, `Find`), `internal/scan/scan.go` (`Module`, `Result`, `Run`).

**Look here for:** the canonical frontmatter schema and key ordering (`spec/frontmatter.go`), how acceptance-criteria checkboxes are mutated byte-for-byte (`spec/criteria.go:SetCriterion`), what `preen` actually checks (`check/check.go`), how `.fledge/broods/*.brood` claim files are structured (`lock/lock.go`), and the `nest.Doc`/`ConcernDocs` types this very document set is an instance of.

## Open Questions

None survive synthesis — module boundaries and scope were unambiguous across all scout reports.
</content>
