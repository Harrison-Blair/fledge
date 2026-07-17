---
generated: 2026-07-17T17:48:26Z
commit: 1c9011d6e6a06f72f96bc98e3b2bd99c408ab79e
agent: fledge-forager
fledge_version: 0.6.10
---

# Modules

Repo map: one entry per top-level module (as enumerated by `fledge scan`), purpose, key files, and where to look for what.

## `cmd` (cmd/fledge/)
**Purpose:** CLI entry point and the full acceptance-test suite.
**Key files:** `main.go` (11 lines, calls `cli.Run`), `main_test.go` (testscript driver), `testdata/*.txtar` — 36 fixtures, one per CLI command area (init, new, status, set, criteria, brood, nest, nest_status, graph, heartbeat, await, pulse, verdict, escalate, dev*, preen_scaffold, refresh_scaffold, e2e, forager_contract, freshness_gate, plan_delegation, ...).
**Look here for:** how any CLI command behaves end-to-end, exit codes, `--json` output shapes, dev-mode symlink assertions. Run via `go test ./cmd/fledge -run TestScripts` (or `/init` for a single script).

## `internal/cli`
**Purpose:** Deterministic command dispatch and per-command implementation — the CLI's dispatch layer.
**Key files:** `cli.go` (commandOrder + registration + Run()), `specload.go` (shared `loadSet()`), one file per command (new.go, status.go, set.go, criteria.go, preen.go, ready.go, vee.go, colony.go, unfledged.go, brood.go, ledger.go, verdict.go, escalate.go, heartbeat.go, pulse.go, await.go, roster.go, init.go, scan.go, nest.go, dev.go, agents.go, version.go, update.go).
**Look here for:** adding/changing a CLI command, exit-code conventions, JSON output structs, flag parsing patterns (`loadSet`, `parseMixed`).

## `internal/bootstrap`
**Purpose:** The scaffolding engine — embeds `core/` (agent-neutral workflow) and `adapters/` (per-harness mappings) via `go:embed`, and the Go logic that writes them into a consumer repo.
**Key files:** `bootstrap.go` (embed declaration), `primitives.go` (6-primitive contract + `DeriveTier`), `registry.go` (manifest loading, `WriteCore`/`WriteAdapter`, file write policies), `drift.go` (drift classification), `stamp.go` (`.fledge/scaffold.json` read/write), plus 17 invariant test files (brooder_test.go, incubator_test.go, skua_test.go, ledger_handoff_test.go, etc.) that assert on the embedded prose itself.
**Sub-trees:**
- `core/skills/fledge-orchestrate/` + `core/skills/fledge-interrogate/` — the single source of the orchestration workflow (planning.md, implementation.md, foraging.md, incubator.md, brooder.md, skua.md, worker-protocols.md, SKILL.md, templates/).
- `adapters/claude/`, `adapters/codex/`, `adapters/pi/` — one manifest.yaml + file set per harness.
**Look here for:** changing what `fledge init` scaffolds, adding a harness (edit a manifest, zero Go code), changing workflow prose (edit `core/`, never the scaffolded copies), drift/refresh semantics.

## `internal/spec`
**Purpose:** Parses, validates, and mutates PLM/FTHR markdown spec files.
**Key files:** `types.go` (Requirement/Task structs, lifecycle constants), `frontmatter.go` (parse/serialize, atomic writes), `ids.go` (flock-serialized ID allocation), `load.go` (batch loading), `criteria.go` (checkbox mutation), `templates.go` + `templates/{plumage,feather}.md` (new-spec bodies).
**Look here for:** the plumage/feather file format, frontmatter fields, ID allocation mechanics, acceptance-criteria checkbox semantics.

## `internal/check`, `internal/graph`, `internal/ledger`, `internal/lock`, `internal/nest`, `internal/repo`, `internal/roster`, `internal/scan`
**Purpose:** Small, focused support packages, each backing one CLI command area.
**Key files:** `check/check.go` (`preen` validation engine — schema, dangling refs, cycles, brood/criteria consistency), `graph/graph.go` (`vee` — cycles, waves, ready set), `ledger/ledger.go` (status/verdict/escalation records under `.fledge/ledger/`, `StaleAfter = 5m`), `lock/lock.go` (`brood`/`abandon` feather claims under `.fledge/broods/`), `nest/nest.go` + `nest/docs.go` (the `fledge nest scaffold/scout/stamp/status` machinery this forager pipeline itself runs on), `repo/repo.go` (git-root + `.fledge/` subdir resolution), `roster/roster.go` (18-penguin-species worker-name allocation), `scan/scan.go` (`fledge scan` — the module list this very document is built from).
**Also in this group (test-only, no production code):** `internal/ciconfig` (validates `.github/workflows/*.yml` structure), `internal/doctest` (validates README.md/CLAUDE.md cross-references), `internal/hooktest` (integration-tests `scripts/hooks/pre-commit`).
**Look here for:** `preen` rule definitions, dependency-cycle/wave logic, ledger record semantics (status vs. verdict vs. escalation), brood/lock file format, nest scaffolding internals, worker-species naming, `fledge scan`'s module-grouping algorithm.

## `<root>`
**Purpose:** Project docs, build config, dogfooding entry points.
**Key files:** `CLAUDE.md` (primary developer/agent guide), `README.md` (terminology + architecture overview), `AGENTS.md` (one-line routing pointer), `MIGRATION.md` (version-upgrade steps), `RELEASING.md` (version-bump + release procedure), `VERSION`, `go.mod`, `LICENSE` (AGPLv3).
**Look here for:** build/test commands, terminology glossary, release procedure, migration notes between fledge versions.

## `.agents`, `.codex`
**Purpose:** Scaffolded routing pointers for non-Claude harnesses consuming this repo (dogfooding artifacts, not source).
**Key files:** `.agents/skills/fledge-{orchestrate,interrogate}/SKILL.md` (frontmatter + pointer), `.codex/fledge-adapter.md` (Codex primitive map, Tier A).
**Look here for:** confirming what a non-Claude harness sees when it opens this repo.

## `.github`
**Purpose:** CI/CD.
**Key files:** `workflows/pr-check.yml` (gofmt/vet/build/test on every PR), `workflows/release.yml` (safety-net + version-detect + 4-platform build on VERSION change to main).
**Look here for:** what CI actually checks, release trigger conditions, platform matrix (linux/darwin × amd64/arm64).

## `docs`
**Purpose:** Design/planning documents; mixed relevance to current fledge scope.
**Key files:** `generalization-plan.md` (locked design decisions Q1-Q23, core+adapters thesis, spawn-pool amendment). `google_ai_mode_response.md` and `research_prompt.md` cover multi-tier AI infrastructure (OpenCode, DeepSeek, local models) — orthogonal to fledge's spec-driven orchestration; purpose unclear, see Open Questions.
**Look here for:** rationale behind the core+adapters architecture and primitive-tier design (generalization-plan.md only).

## `scripts`
**Purpose:** Local build/install/lint tooling.
**Key files:** `install.sh` (build, install, version-verify, optional `--refresh`), `hooks/pre-commit` (optional gofmt+vet gate, opt-in via `git config core.hooksPath scripts/hooks`).
**Look here for:** local dev setup, optional pre-commit gate installation.
