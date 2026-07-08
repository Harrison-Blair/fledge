---
generated: 2026-07-08T05:28:12Z
commit: e46c481a047d45ef10bcd79a3326d47932b32868
agent: fledge-forager
fledge_version: 0.2.1
---

# Dependencies

External libraries, the internal package dependency direction, and the agent-harness systems fledge targets.

## Third-party (go.mod, Go 1.26.4)

- `github.com/goccy/go-yaml v1.19.2` — YAML frontmatter and manifest parsing.
- `github.com/rogpeppe/go-internal v1.15.0` — `testscript`/txtar engine for CLI acceptance tests.
- `golang.org/x/sys`, `golang.org/x/tools` — indirect.

Deliberately lean: no cobra/urfave (hand-rolled dispatch in `cli.go`), no external logging. Standard library does the rest (`flag`, `text/template`, `embed`, `os`, `path/filepath`, `io/fs`, `encoding/json`, `syscall` for PID checks).

## Internal package dependency direction

`cmd/fledge` → `internal/cli` → domain packages. `internal/cli` depends on `repo`, `spec`, `check`, `graph`, `lock`, `scan`, `nest`, `bootstrap`. The domain packages do not depend on `cli` (one-way). Notable couplings:
- `cli` → `bootstrap` (`LoadAdapters`, `DetectAdapters`, `WriteCore`, `WriteAdapter`) for `init`/`agents`.
- `cli` → `check` (`Run`) for `preen`; `check` receives the spec set, locked IDs, and `repo.EvidenceDir()`.
- `cli` → `graph` (`New`, `Cycle`, `Waves`, `Ready`) for `vee`/`ready`/cycle validation in `set`/`new`.
- `cli` → `nest` (`Doc`, `ClearNest`, `ConcernDocs`, renderers) for `nest`.
- `bootstrap` → `goccy/go-yaml` for manifests; embeds `core/` + `adapters/` via `//go:embed`.

## Target agent harnesses (adapters)

Realized as manifests under `internal/bootstrap/adapters/`: **claude** (Tier C — all 6 primitives), **codex** (Tier A), **pi** (Tier A). Detection markers: `.claude/`, `.codex/`, `.pi/`. Planned for 0.3.0 (`docs/generalization-plan.md`): Cursor, opencode. Each harness relies on an external "Agent Skills" convention (a `SKILL.md` + siblings the harness auto-discovers); skill names must be ≤64 chars, lowercase-hyphen.

## Open Questions

- Whether Claude Code auto-discovers skills via a `settings.json` `skills` array or only by scanning `.claude/skills/` recursively is unverified (`docs/generalization-plan.md` §12) and governs the symlink-vs-array detection strategy — a direct input to agent-detection work.
- Exact skill/config discovery for Codex (`.codex/skills/` vs `AGENTS.md`), Cursor, and opencode is unverified pending those releases.
