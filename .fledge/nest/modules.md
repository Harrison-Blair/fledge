---
generated: 2026-07-16T04:02:08Z
commit: 154510fc963e7071b2f09297ecfeba2b6710e85e
agent: fledge-forager
fledge_version: 0.5.8
---

# Modules

Repo map — one entry per top-level module (as `fledge scan` groups them), its purpose, its key files, and where to look for what.

## root (8 files)
Repo-root metadata: `.gitignore`, `CLAUDE.md` (this file — agent guidance), `LICENSE` (AGPLv3), `MIGRATION.md` (upgrade notes across 0.1→0.4), `README.md` (quick start, terminology, command reference), `RELEASING.md` (release process), `VERSION` (single-line current version, currently `0.5.8`), `go.mod` (module `github.com/Harrison-Blair/fledge`, Go 1.26.4, 2 direct deps).
**Look here for:** release process facts, licensing, module identity, project-level conventions.

## cmd (27 files: `cmd/fledge/main.go`, `main_test.go`, 25 `testdata/*.txtar`)
The binary entry point. `main.go` is a 1-line dispatcher to `internal/cli.Run()`. `main_test.go` wires the CLI into `testscript` so `.txtar` fixtures can run `fledge` as a shell command. `cmd/fledge/testdata/` holds all 25 acceptance-test fixtures (`ls cmd/fledge/testdata/*.txtar | wc -l` = 25), one per command or cross-cutting behavior (init, e2e, criteria, lock, roster, stamp_warning, forager_contract, freshness_gate, plan_delegation, nest_status, etc.).
**Look here for:** what CLI-level behavior is contractually guaranteed (each `.txtar` is an acceptance contract); how to add/verify a new command end-to-end.

## docs (3 files)
Planning/research prose, not shipped documentation: `generalization-plan.md` (locked design doc for generalizing fledge beyond Claude Code — 23 resolved decisions, M0–M5 milestones), `google_ai_mode_response.md` + `research_prompt.md` (an orthogonal personal-infrastructure cost-optimization exploration, not part of fledge's product surface).
**Look here for:** design rationale behind multi-harness generalization; historical decisions (Q1–Q23) if revisiting adapter/tier design. Largely not relevant to day-to-day fledge development.

## .github + scripts (4 files, merged scout assignment)
`.github/workflows/pr-check.yml` (gofmt/vet/build/test gate on every PR) and `release.yml` (version-triggered 4-platform build + GitHub Release); `scripts/hooks/pre-commit` (opt-in local mirror of the CI lint gate) and `scripts/install.sh` (build+install+verify+optional-refresh).
**Look here for:** CI gate contents, release trigger mechanics, local dev setup.

## internal/cli (26 files)
All 19 CLI commands and their handlers. `cli.go` is the dispatcher/registry; one file per command (`init.go`, `scan.go`, `new.go`, `nest.go`, `preen.go`, `ready.go`, `vee.go`, `colony.go`, `unfledged.go`, `status.go`, `set.go`, `criteria.go`, `brood.go`, `roster.go`, `update.go`, `version.go`, `agents.go`); `specload.go` holds the shared `loadSet()` loader.
**Look here for:** what any `fledge <command>` actually does; exit-code conventions; `--json` output shapes; where `binaryVersion` lives (`version.go`) for release-version consistency.

## internal/bootstrap (36 files, split into two scout assignments)
- **internal-bootstrap-core** (20 files): `bootstrap.go` (embed), `primitives.go` (6-primitive contract, tier derivation), `registry.go` (Manifest/ManifestFile, Write*, LoadAdapters), `drift.go` (5-state drift classification), `stamp.go` (scaffold.json I/O), plus `core/skills/fledge-orchestrate/*` and `fledge-interrogate/SKILL.md` — the actual agent-neutral workflow prose.
- **internal-bootstrap-adapters** (17 files): `adapters/claude/`, `adapters/codex/`, `adapters/pi/` — per-harness manifest.yaml + generated adapter.md + agent definitions (Claude only: brooder, forager, context-scout, incubator, skua) + settings files.
**Look here for:** what `fledge init` scaffolds and why; the source of truth for `.fledge/skills/` and `.claude/` content (never edit the scaffolded copies directly — edit here, then `fledge init --refresh`).

## internal/spec (12 files)
Frontmatter parsing/rendering (`frontmatter.go`), ID allocation (`ids.go`, flock-serialized `NextID`/`AllocateAndCreate`), acceptance-criteria checkbox parsing/mutation (`criteria.go`), bulk spec loading (`load.go`), type definitions (`types.go`: `Requirement`, `Task`), and embedded plumage/feather markdown templates.
**Look here for:** exact frontmatter field lists and rendering order for PLM/FTHR files; how AC-N checkboxes are parsed/flipped; ID allocation/collision-safety mechanics.

## internal/nest (6 files)
Implements `fledge nest` subcommands (`new`, `scaffold`, `scout --module`, `stamp`, `status`) — the machinery behind this very document set. `nest.go`/`docs.go` hold the logic; `templates/concern-doc.md`, `templates/index.md`, `templates/scout-report.md` are the embedded templates (note: these are the same template family surfaced to agents at `.fledge/skills/fledge-orchestrate/templates/`, scaffolded from `internal/bootstrap/core`).
**Look here for:** exact mechanics of `fledge nest status`'s completeness check (stub-detection, index-commit-matches-HEAD gate); what `fledge nest scaffold` clears/recreates.

## internal-misc (16 files, merges 9 small packages: check, ciconfig, doctest, graph, hooktest, lock, repo, roster, scan)
- `check` (`check.go`, 328 lines): preen's 13 validation rules → `[]Finding`.
- `ciconfig`/`doctest`: tests-only packages that assert on `.github/workflows/*.yml` structure and on README/RELEASING.md content — no production code.
- `graph` (`graph.go`, 134 lines): dependency DAG — `Cycle()`, `Waves()`, `Ready()`; backs `vee`/`ready`.
- `hooktest`: end-to-end tests of `scripts/hooks/pre-commit` in real temp git repos.
- `lock` (`lock.go`, 124 lines): brood claim files (`.fledge/broods/FTHR-###.brood`), atomic `os.Link`-based `Acquire()`.
- `repo` (`repo.go`, 66 lines): git-root discovery + `.fledge/` path helpers.
- `roster` (`roster.go`, 208 lines): 18-species bird-themed worker-name allocator with pair/overflow semantics.
- `scan` (`scan.go`, 123 lines): backs `fledge scan` — git-tracked+untracked file listing, `.fledgeignore` filtering, per-directory grouping.
**Look here for:** preen's exact rule set; brood-lock atomicity guarantees; roster species-naming mechanics; what `fledge scan`'s module list actually includes/excludes.

## Open Questions

- Whether `ciconfig`'s two test files (`release_workflow_test.go`, `workflow_test.go`) should be merged — flagged by the internal-misc scout as a possible simplification, unconfirmed.
