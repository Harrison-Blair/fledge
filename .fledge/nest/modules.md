---
generated: 2026-07-11T01:58:32Z
commit: 96a3ac38bc843217824d6d6886c49906053bf686
agent: fledge-forager
fledge_version: 0.3.4
---

# Modules

Repo map: one entry per top-level module (as reported by `fledge scan`), its purpose, its key files, and where to look for what.

## `<root>`
Project metadata and entry-point guidance: `README.md` (quick-start, terminology decoder, feature overview), `CLAUDE.md` (agent build/architecture/convention guidance), `VERSION` (semver, read by `scripts/install.sh` and build ldflags), `go.mod` (module `github.com/Harrison-Blair/fledge`, Go 1.26.4), `LICENSE` (AGPLv3), `.gitignore`, `MIGRATION.md`, and `scripts/install.sh` (build/install/verify script).
Look here for: how to install/build fledge, top-level conventions, licensing, version source of truth.

## `cmd`
The binary's entry point and its acceptance-test harness: `cmd/fledge/main.go` (`main()` → `cli.Run()`), `cmd/fledge/main_test.go` (`TestScripts`/`TestMain`, deterministic git env for tests), and 21 testscript/txtar files under `cmd/fledge/testdata/` — one or more per CLI command (init, new, status, preen, vee, brood, ready, criteria, set, colony, unfledged, scan, agents, nest, version, etc.).
Look here for: how to run/read the CLI acceptance test suite; the authoritative behavioral spec for every command (each `.txtar` pins exact stdout/exit-code assertions).

## `docs`
Design and research artifacts, no executable code: `docs/generalization-plan.md` (locked 0.2.0 design for porting fledge across harnesses — core/adapter split, 7-primitive contract, milestones M0–M5, 23 resolved decisions), `docs/google_ai_mode_response.md` (hybrid AI infrastructure tiering proposal, likely exploratory/unadopted), `docs/research_prompt.md` (a system-prompt template for infra research briefs, not tied to fledge's own code).
Look here for: rationale behind the multi-harness adapter architecture and open design questions about Cursor/opencode/Codex support; do not treat as current-state documentation — it's forward-looking/planning material.

## `internal`
The Go implementation, split into focused packages (grouped below to match how this nest was foraged):
- **`internal/cli`** (21 files) — command dispatch, formatting, exit codes; one file per subcommand.
- **`internal/bootstrap`** (35 files) — scaffolding/adapter engine: embed, manifest, primitives, stamp, drift, plus the embedded `core/` skills and `adapters/<harness>/` trees.
- **`internal/spec`** (12 files) — PLM/FTHR frontmatter parsing, ID allocation, templates, atomic writes.
- **`internal/check`** — spec validation (`preen`'s engine).
- **`internal/graph`** — dependency graph, cycle detection, waves (`vee`'s engine).
- **`internal/lock`** — feather claim files (`brood`'s engine).
- **`internal/nest`** — context-doc/scout-report rendering (`nest`'s engine).
- **`internal/repo`** — git-root + `.fledge/` path derivation, shared by everything.
- **`internal/scan`** — file inventory grouped by module (`scan`'s engine).
Look here for: all actual business logic. `internal/cli` is a thin shell; the domain packages hold the rules.

## `pluma`
This repo's own spec set (dogfooding): `.fledge/pluma/plumage/PLM-001`…`PLM-010` (feature specs) and `.fledge/pluma/feathers/FTHR-001`…`FTHR-016` (task specs), each YAML-frontmatter + markdown, validated and mutated only via the CLI.
Look here for: the history and status of every feature/task fledge has planned or shipped against itself; a worked example of the plumage/feather spec format.

## `scripts`
`scripts/install.sh` — bash build/install script; reads `VERSION`, sets ldflags, installs to `$(go env GOPATH)/bin`, verifies the installed binary's version matches, optionally runs `fledge init --refresh`.
Look here for: the exact install/upgrade procedure end users and CI are expected to follow.
