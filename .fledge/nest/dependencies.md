---
generated: 2026-07-08T01:03:26Z
commit: e44524d1f089dcfe1c1f313f819ec18d9a42eceb
agent: fledge-forager
fledge_version: 0.2.1
---

# Dependencies

External dependencies used by the fledge codebase, deduplicated across modules with usage notes.

## Go module dependencies (`go.mod`, Go 1.26.4 required)

- **`github.com/goccy/go-yaml`** (v1.19.2, direct) — YAML parsing/unmarshaling for two things: spec frontmatter (`internal/spec/frontmatter.go`) and adapter `manifest.yaml` files (`internal/bootstrap/registry.go:114`).
- **`github.com/rogpeppe/go-internal`** (v1.15.0, direct) — supplies the `testscript` package used to run the `cmd/fledge/testdata/*.txtar` acceptance tests (`cmd/fledge/main_test.go`).
- **`golang.org/x/sys`** (v0.26.0, indirect) — system-level calls (e.g. PID liveness checks in lock handling).
- **`golang.org/x/tools`** (v0.26.0, indirect) — Go tooling support, indirect transitive dependency.

## Standard library (heavily used, not third-party but notable)

- `embed` — `internal/bootstrap/bootstrap.go` embeds the entire `core/` and `adapters/` trees into the binary via `//go:embed core adapters`.
- `text/template` — renders scaffolded adapter files (`generate`/`primitive_map` write policies) in `internal/bootstrap/registry.go`.
- `os/exec` — shells out to `git` for repo introspection: `git rev-parse` (commit sha), `git ls-files` (file inventory for `scan`), `git check-ignore` (`.fledgeignore` filtering).
- `flag`, `encoding/json`, `regexp`, `sort`/`slices`, `path/filepath` — CLI argument parsing, JSON output, ID/kebab pattern matching, sorting, and path handling across `internal/cli` and `internal/spec`.

## Runtime / tooling dependencies (not Go packages)

- **Git** — required at runtime for `fledge scan` (`git ls-files`, `git check-ignore`) and for commit-sha stamping in scaffolded/generated file frontmatter. The project takes a "trust-git" stance per `docs/generalization-plan.md`: no backup machinery for scaffolded files — `.fledge/skills/` is expected to be committed and recoverable via git.
- **Agent Skills standard** — all currently-targeted harnesses (Claude Code, pi, Codex) and future ones (Cursor, opencode per the generalization plan) load skills natively; fledge's `core/skills/` already conforms to that format.

## Per-harness adapter dependencies (mechanism, not code dependency)

These aren't Go dependencies but are external mechanisms each adapter maps fledge's 6 primitives onto (`docs/generalization-plan.md` §2, verified/unverified per adapter):
- **Claude Code** — teammate spawn (`spawn-worker`), `AskUserQuestion` (confirm-gate), tmux for team-loop piping (`internal/bootstrap/adapters/claude/team-loop.md`).
- **pi** — `fledge_gate` tool + SDK sessions.
- **Codex** — skills config + `AGENTS.md` auto-load (unverified exact layout, per `docs/generalization-plan.md` open verification V2).
- **Cursor** (0.3.0, not yet built) — `.cursor/rules/*.mdc` format, unverified.
- **opencode** (0.3.0, not yet built) — config layout (`opencode.json` / `.opencode/`), unverified.

## Notes

No web framework, database, or network-service dependency anywhere in the codebase — fledge is a purely local, filesystem/git-backed CLI. No Makefile; `go build`/`go test`/`go vet` are used directly (see `README.md`/`CLAUDE.md` for exact invocations).

## Open Questions
None observed.
