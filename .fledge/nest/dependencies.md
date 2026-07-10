---
generated: 2026-07-10T14:50:00Z
commit: 7678344ab9a18730530b9f6edf507ad0c449d352
agent: fledge-forager
fledge_version: 0.2.1
---

# Dependencies

External libraries, tools, and services fledge relies on, deduplicated across modules with usage notes.

## Go module dependencies (`go.mod`)

- **`github.com/goccy/go-yaml` v1.19.2** — YAML parsing/unmarshalling. Used for spec frontmatter (`internal/spec/frontmatter.go`) and adapter `manifest.yaml` parsing (`internal/bootstrap/registry.go`).
- **`github.com/rogpeppe/go-internal` v1.15.0** (specifically its `testscript` package) — the txtar/testscript acceptance-test framework driving all `cmd/fledge/testdata/*.txtar` fixtures (`cmd/fledge/main_test.go`).
- **`golang.org/x/sys` v0.26.0** (indirect) — OS-level syscalls; backs `syscall.Kill` PID-liveness checks in `internal/cli` (brood).
- **`golang.org/x/tools` v0.26.0** (indirect) — Go tooling support.
- Go **1.26.4** toolchain (module declares Go 1.26; `CLAUDE.md` confirms Go 1.26 requirement).

## Standard library (heavy use, no third-party equivalent)

- `flag` — every CLI command's argument parsing (`internal/cli/*.go`).
- `encoding/json` — all `--json` output (`emitJSON`) and brood record persistence (`internal/lock/lock.go`).
- `os/exec` — shells out to `git` (rev-parse for repo root/HEAD, `ls-files`/`check-ignore` for scan, current-branch for brood records).
- `text/template` — renders all `Generate`/`PrimitiveMap`-policy scaffolded files (`internal/bootstrap/registry.go`).
- `io/fs` — reads the two `//go:embed` trees (`core/`, `adapters/`) in `internal/bootstrap/bootstrap.go`.
- `path/filepath`, `regexp`, `sort`, `slices`, `bytes`, `strings` — used pervasively across `internal/spec`, `internal/check`, `internal/scan`, `internal/cli`.

## External tools/services (not Go libraries)

- **git** — required at runtime (not just dev-time): `internal/repo.Find()` shells to `git rev-parse` to locate the repo root; `internal/scan` shells to `git ls-files`/`git check-ignore` to enumerate tracked/untracked files respecting `.fledgeignore`; `internal/cli/brood.go` records the current branch via `git` for brood entries. No caching strategy is documented for these repeated git invocations (see Open Questions).
- **Agent harnesses** (Claude Code, pi, Codex — not code dependencies but integration targets): each has a native skills/config mechanism fledge's adapters target — Claude Code's `.claude/skills/` (native Agent Skills support), pi's `.pi/` skills pointer, Codex's `AGENTS.md` auto-load convention. Cursor and opencode are planned (`docs/generalization-plan.md`) but not yet implemented.

## Test-only dependencies

- **`testscript`** (from `go-internal`) drives `cmd/fledge/testdata/*.txtar` — sets a deterministic git environment (`GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`, etc., disabling system/global git config) in `cmd/fledge/main_test.go`.

## Non-dependencies worth noting

- `pluma/` (plumage/feather specs) has **no** external dependency — pure markdown+YAML read by the CLI itself.
- No HTTP client, no database, no network calls anywhere in the codebase — fledge is a purely local, filesystem/git-based CLI tool.
- `docs/google_ai_mode_response.md` mentions unrelated external APIs (OpenCode Go/Zen, various LLM providers) — this is example output from a research-prompt exercise, not a real fledge dependency; do not confuse with the actual dependency list above.

## Open Questions

- No caching strategy observed for repeated `git` subprocess invocations (`rev-parse`, `ls-files`, `check-ignore`) across `scan.Run()` and `repo.Find()` — potential overhead on large repos, not yet measured or addressed.
