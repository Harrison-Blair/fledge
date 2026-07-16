---
generated: 2026-07-16T21:27:15Z
commit: a1ed62a38540df7ab1cbdc4c486176a64a762018
agent: fledge-forager
fledge_version: 0.5.8
---

# Modules

Repo map: every top-level module `fledge scan` reports, one entry each (large modules further split by the scout assignments actually used).

## `<root>`
Purpose: top-level repo docs and metadata; project overview, agent instructions, migration/release history, module manifest.
Key files: `README.md`, `CLAUDE.md`, `MIGRATION.md`, `RELEASING.md`, `VERSION`, `go.mod`, `LICENSE`, `.gitignore`.
Look here for: bird-terminology decoder, build/test/run commands, version-bump discipline, upgrade history across 0.1.0–0.4.0.

## `cmd`
Purpose: CLI entry point; thin wrapper delegating to `internal/cli.Run`.
Key files: `cmd/fledge/main.go` (11 lines), `cmd/fledge/main_test.go` (testscript harness setup), `cmd/fledge/testdata/*.txtar` (25 acceptance-test fixtures).
Look here for: the full list of CLI commands exercised end-to-end, exit-code/JSON-output contract, and acceptance-test fixtures to update after any scaffold or CLI change.

## `docs`
Purpose: design/research documents, not live specs — a locked generalization-plan design doc and two AI-infrastructure research artifacts of unclear current relevance.
Key files: `docs/generalization-plan.md` (23 locked decisions for the 0.1.0→0.2.0 refactor), `docs/google_ai_mode_response.md`, `docs/research_prompt.md`.
Look here for: historical rationale for the primitive/tier/manifest design; do not treat as current-state truth without cross-checking `internal/bootstrap`.

## `internal/bootstrap` (adapters)
Purpose: per-harness adapter definitions (claude, codex, pi) mapping the 6 primitives to harness-native mechanisms; each adapter is fully described by its `manifest.yaml`.
Key files: `internal/bootstrap/adapters/claude/{manifest.yaml,agents/*.md,team-loop.md,settings*.json}`, `internal/bootstrap/adapters/codex/manifest.yaml`, `internal/bootstrap/adapters/pi/{manifest.yaml,settings.json,prompts/*.md}`.
Look here for: what mechanism realizes a given primitive on a given harness, and what files `fledge init` writes into `.claude/`, `.codex/`, `.pi/`.

## `internal/bootstrap` (core skills)
Purpose: single agent-neutral source of the fledge-orchestrate and fledge-interrogate skills — the actual workflow prose scaffolded into every harness's `.fledge/skills/`.
Key files: `internal/bootstrap/core/skills/fledge-orchestrate/{SKILL,planning,implementation,foraging,incubator,brooder,skua,worker-protocols}.md`, `internal/bootstrap/core/skills/fledge-orchestrate/templates/*.md`, `internal/bootstrap/core/skills/fledge-interrogate/SKILL.md`.
Look here for: the source of truth for planning/implementation/foraging protocol changes — never edit the scaffolded copies under this repo's own `.fledge/skills/` directly.

## `internal/bootstrap` (Go package)
Purpose: implements the scaffold system itself — embedding, manifest loading, file-write policies, drift detection, and the `.fledge/scaffold.json` stamp.
Key files: `internal/bootstrap/bootstrap.go` (go:embed), `registry.go` (Manifest struct, loader, 6 write policies), `primitives.go` (primitive/tier definitions), `drift.go` (DriftReport), `stamp.go` (Stamp/.fledge/scaffold.json).
Look here for: `fledge init`/`init --refresh` mechanics, why a scaffolded file was/wasn't rewritten, and the ~15 embedded-prose guard tests (e.g. `brooder_test.go`, `skua_test.go`, `incubator_test.go`) that pin specific sentences in the core skill docs.

## `internal/cli`
Purpose: command dispatch layer; registry pattern with one file per subcommand, shared helpers (`loadSet`, `emitJSON`), exit codes.
Key files: `internal/cli/cli.go` (registry + `Run`), `specload.go`, and one file per command (`init.go`, `new.go`, `status.go`, `set.go`, `criteria.go`, `brood.go`, `nest.go`, `scan.go`, `vee.go`, `preen.go`, `ready.go`, `colony.go`, `unfledged.go`, `roster.go`, `agents.go`, `version.go`, `update.go`).
Look here for: exact CLI flag/JSON-output behavior for any `fledge <cmd>`, and `internal/cli/fledgeignore.default` (the embedded default `.fledgeignore`).

## `internal/check`, `internal/ciconfig`, `internal/graph`, `internal/lock`
Purpose: validation, dependency-graph, and claim-locking domain packages, plus CI-workflow-YAML structure tests.
Key files: `internal/check/check.go` (`fledge preen` — 14 validation rules), `internal/graph/graph.go` (`fledge vee` — cycle/topo-sort/ready-set), `internal/lock/lock.go` (`fledge brood` — atomic `.fledge/broods/*` claim files), `internal/ciconfig/*_test.go` (asserts `.github/workflows/*.yml` structure; no production code).
Look here for: preen's exact validation-rule list, brood-claim JSON schema (`Record{Task,Owner,PID,Created,Branch,Worktree}`), and cycle-detection/topological-wave algorithms.

## `internal/doctest`, `internal/hooktest`, `internal/repo`, `internal/roster`, `internal/scan`
Purpose: small utility packages — doc-content assertions, pre-commit-hook end-to-end tests, git-repo-root/dir resolution, worker-species allocation, and the module-scan foraging depends on.
Key files: `internal/repo/repo.go` (`Find()`, `FledgeDir()`/`ContextDir()`/etc.), `internal/roster/roster.go` (18-species list, flock-guarded `roster.json`), `internal/scan/scan.go` (`Run()` — the exact command this forager ran first).
Look here for: the canonical species list for worker naming, how `.fledge/` subdirectories are resolved, and how `fledge scan`'s module/file-list output is produced.

## `internal/nest`
Purpose: implements `fledge nest scaffold|scout|status|stamp` — the schemas, frontmatter, and stub-detection this very forager pipeline runs on.
Key files: `internal/nest/nest.go` (`Doc`, `Status`, `ClearNest`, `RefreshDoc`), `internal/nest/docs.go` (`ConcernDocs` 9-member closed set), `internal/nest/templates/*.md` (stub templates).
Look here for: exactly how `fledge nest status` computes `complete: true` (stub byte-comparison + index-commit-matches-HEAD check) — the same check gating this forager's final message.

## `internal/spec`
Purpose: plumage/feather spec file parsing, frontmatter serialization, race-free ID allocation, acceptance/functional-criteria extraction, spec templates.
Key files: `internal/spec/types.go` (Requirement/Task/Criterion/Set), `frontmatter.go` (`SplitFrontmatter`, byte-preserving), `ids.go` (`NextID`, `AllocateAndCreate` with flock), `criteria.go` (`ParseCriteria`/`SetCriterion`), `templates.go` + `templates/{plumage,feather}.md`.
Look here for: the exact frontmatter key order/schema for PLM/FTHR files, and why spec bodies are never re-serialized (byte-preservation guarantee).

## `scripts`, `.github`
Purpose: local dev tooling (pre-commit lint hook, install script) and CI/CD automation (PR gate, cross-platform release build).
Key files: `scripts/hooks/pre-commit`, `scripts/install.sh`, `.github/workflows/pr-check.yml`, `.github/workflows/release.yml`.
Look here for: what the pre-commit hook and CI actually check (must match — `internal/hooktest` and `internal/ciconfig` tests pin this), and the 4-platform release build matrix.
