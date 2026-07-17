---
generated: 2026-07-17T07:00:54Z
commit: ee49464adb830bef7189f94a1d3253927d33fb5f
agent: fledge-forager
fledge_version: 0.6.7
---

# Modules

Repo map: every top-level module (per `fledge scan`), its purpose, key files, and where to look for what.

## `<root>` (8 files, 56050 bytes)

Repo-level scaffolding and docs. `VERSION` (0.6.7, source of truth for the release ceremony); `go.mod` (module `github.com/Harrison-Blair/fledge`, Go 1.26.4); `CLAUDE.md` (primary dev guide); `README.md` (feature overview, 6-primitive contract, CLI command table); `MIGRATION.md` (3 sequential upgrade paths: 0.1.0→0.2.0 skill relocation, 0.2.x→0.3.0 scaffold stamp, 0.3.x→0.4.0 `pluma/`→`.fledge/pluma/`); `RELEASING.md` (version-bump ceremony: 3 files sync — `VERSION`, `internal/cli/version.go`, `cmd/fledge/testdata/stamp_warning.txtar`); `LICENSE` (AGPLv3); `.gitignore` (ignores build artifacts, per-run intermediates, dev-link symlinks, `.claude/settings.local.json`).

Look here for: version/release ceremony, licensing, module path, top-level upgrade history, bird-terminology glossary (README.md).

## `cmd` (37 files, 105255 bytes)

CLI entrypoint + acceptance-test suite. `cmd/fledge/main.go` (single-function entrypoint calling `internal/cli.Run`); `cmd/fledge/main_test.go` (testscript harness, git-identity isolation); `cmd/fledge/testdata/*.txtar` — 37 fixtures (`ls cmd/fledge/testdata/*.txtar | wc -l` = 37), one per command/feature path, including recent additions `await.txtar`, `verdict.txtar`, `escalate.txtar`, `ledger-read.txtar`, `dev_preen.txtar`, `dev_rails.txtar`, `dev_refresh.txtar`, `dev_status.txtar`.

Look here for: how any specific CLI command is expected to behave (flags, exit codes, JSON shape) — each `.txtar` is a runnable spec; run via `go test ./cmd/fledge -run TestScripts` (all) or `.../TestScripts/<name>` (one).

## `docs` (3 files, 37084 bytes)

Standalone planning/research prose, not wired into the build. `docs/generalization-plan.md` — locked 0.2.0 multi-agent generalization design (23 resolved decisions Q1–Q23, milestones M0–M5); status: "ready to become a plumage," not yet converted. `docs/google_ai_mode_response.md` and `docs/research_prompt.md` — unrelated multi-tier AI-routing infrastructure proposal and its research-prompt template; not referenced anywhere in fledge code or specs.

Look here for: historical design rationale behind the multi-harness generalization (adapters, primitives, tiers) predating their current implementation; do not treat as current-state documentation without cross-checking `internal/bootstrap`.

## `.github` + `scripts` (merged small modules: 2+2 files, 3900+2282 bytes)

CI/CD and local dev tooling. `.github/workflows/pr-check.yml` (lint/build/test gate on PRs to main); `.github/workflows/release.yml` (VERSION-change-gated cross-platform release: linux/darwin × amd64/arm64, 4 targets, no Windows, tar.gz + sha256, `gh release create`); `scripts/hooks/pre-commit` (optional local hook mirroring CI lint — gofmt, go vet — opt-in via `git config core.hooksPath scripts/hooks`); `scripts/install.sh` (build/install/verify fledge binary, optional `--refresh` re-syncs scaffold).

Look here for: release process mechanics, CI gate contents, the pre-commit hook's exact commands.

## `internal/bootstrap` (50 files, 218346 bytes)

The scaffolding/orchestration engine. `bootstrap.go` (embed.FS of `core/` + `adapters/`); `primitives.go` (6-primitive model, tier derivation); `registry.go` (manifest loading, `WriteCore`/`WriteAdapter`, dev-link write path); `stamp.go` (`Stamp`/`StampEntry`, `ExpectedFiles`/`ExpectedFilesDev`); `drift.go` (5-status drift classification, dev-link-aware). `core/skills/fledge-orchestrate/` and `core/skills/fledge-interrogate/` hold the actual agent-neutral workflow prose (this is the source of truth — the repo's own `.fledge/skills/` is dev-linked output). `adapters/{claude,pi,codex}/` hold per-harness `manifest.yaml` + prompts + settings.

Look here for: how `fledge init`/`fledge agents`/`fledge dev` scaffold a repo, the 6-primitive/tier model, per-harness differences, drift/refresh semantics, and the canonical prose behind `.fledge/skills/fledge-orchestrate/*.md` (edit here, not the scaffolded copy).

## `internal/cli` (36 files, 143315 bytes)

Command-dispatch layer: 25 registered subcommands. `cli.go` (registry, exit codes `ExitOK/Fail/Usage/Env/Timeout` = 0/1/2/3/4); `await.go`/`verdict.go`/`escalate.go`/`ledger.go`/`heartbeat.go` (PLM-030 ledger commands); `dev.go` (PLM-031 dev-link status); `init.go` (scaffold/refresh/prune); `new.go`/`status.go`/`set.go`/`criteria.go` (spec mutation); `brood.go` (feather claims); `colony.go`/`unfledged.go`/`ready.go`/`vee.go`/`scan.go`/`preen.go` (reporting/validation); `nest.go` (concern-doc scaffolding); `update.go` (self-update); `roster.go`/`agents.go` (agent assignment).

Look here for: exact CLI flag/behavior/exit-code contracts for any command, especially `await`'s change-wait-vs-existence-wait semantics (`await.go`).

## `internal/spec` (12 files, 35658 bytes)

Spec (plumage/feather) parsing, ID allocation, criteria. `types.go` (`Requirement`, `Task`, lifecycle constants); `frontmatter.go` (YAML parse/render, byte-preserved body, `WriteFileAtomic`); `criteria.go` (AC-N checkbox regex parsing, single-byte checked-state flip); `ids.go` (`NextID`, flock-serialized `AllocateAndCreate`, `Kebab`); `load.go` (bulk `Load`, resilient to per-file parse errors); `templates.go` + `templates/{plumage,feather}.md` (new-spec skeletons).

Look here for: frontmatter schema/key order, ID-allocation algorithm, how acceptance-criteria checkboxes are parsed/flipped, spec template skeletons.

## `internal/{check,ciconfig,doctest,graph,hooktest,repo,scan}` (merged small modules: 13 files, 54301 bytes)

Foundational single-purpose packages. `check/check.go` (`fledge preen` validation, `Finding` type, hyphenated rule names). `graph/graph.go` (`fledge vee`: `Cycle`, `Waves`, `Ready`). `repo/repo.go` (git-root discovery, `.fledge` path accessors, `RequireFledge`). `scan/scan.go` (`fledge scan`: module grouping, `.fledgeignore` filtering). `ciconfig/*_test.go`, `doctest/*_test.go`, `hooktest/precommit_test.go` — test-only packages that pin the shape of `.github/workflows/*.yml`, `README.md`/`CLAUDE.md`/`RELEASING.md` sections, and `scripts/hooks/pre-commit` respectively, so those non-Go artifacts stay consistent with what's documented.

Look here for: validation rule names/behavior (`check`), dependency-graph/readiness semantics (`graph`), `.fledge` path layout (`repo`), scan/module-grouping logic (`scan`); and — for ciconfig/doctest/hooktest — the tests that will fail if CI workflows, key docs, or the pre-commit hook drift from what's asserted.

## `internal/{ledger,lock,nest,roster}` (merged small modules: 12 files, 57563 bytes)

State-backing packages beneath the CLI. `ledger/ledger.go` (new PLM-030 package: `Record`/`StatusRecord`/`VerdictRecord`/`EscalationRecord`, atomic temp-then-rename writes, subject-path-traversal rejection, `ClassifyLiveness` against a 5-minute `StaleAfter` TTL). `lock/lock.go` (`fledge brood` backing: exclusive `os.Link`-based claim acquisition, corruption-tolerant `List`). `nest/nest.go` + `docs.go` + `templates/` (backs `fledge nest scaffold/scout/stamp/status` — the very machinery that produced this document set; embeds its own placeholder templates, distinct in purpose from `.fledge/skills/fledge-orchestrate/templates/`, see Open Questions in `architecture.md`). `roster/roster.go` (`fledge roster`: 18-species token table, flock-guarded, overflow to numeric suffixes).

Look here for: ledger record shapes/liveness rules, brood-lock exclusivity mechanics, how nest concern-doc/scout-report scaffolding and stub/completion detection work, roster species-allocation algorithm.

## Open Questions

None carried forward at the module-map level; see `architecture.md` and `data-model.md` for unresolved scout questions on specific subsystems.
