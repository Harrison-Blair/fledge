---
generated: 2026-07-06T23:33:05Z
commit: b701cf5a12a99b5adf9538e83f51178d4dead0c2
agent: fledge-context-gatherer
fledge_version: 0.1.0
---

# Dependencies

External and internal dependencies of the fledge codebase. Module path is `github.com/Harrison-Blair/fledge`, declared `go 1.26.4` (`go.mod`).

## Third-party (direct)
- **`github.com/goccy/go-yaml` v1.19.2** — YAML unmarshalling of spec frontmatter (with `time.Time` awareness). Used only in `internal/spec` for parsing; rendering is done by fledge's own canonical writer, not this library.
- **`github.com/rogpeppe/go-internal` v1.15.0** — provides `testscript`, the txtar-based end-to-end test framework driving `cmd/fledge/testdata/*.txtar` via `cmd/fledge/main_test.go`. Test-only usage.

## Third-party (indirect)
- **`golang.org/x/sys` v0.26.0** — transitive (OS/syscall support).
- **`golang.org/x/tools` v0.26.0** — transitive (tooling support pulled in by go-internal).

## External tools invoked at runtime
- **git** — shelled out via `os/exec` for repo discovery, commit/HEAD resolution, and listing tracked+untracked files. Used by `internal/repo` (`Find`, HEAD/VERSION resolution) and `internal/scan` (`Run` file listing and commit stamping). fledge assumes it runs inside a git repository; absence yields an environment error (exit code 3).

## Standard library (notable usage)
- `internal/spec`: `os`, `path/filepath`, `regexp`, `strings`, `bytes`, `strconv`, `time`, `unicode`, `embed` (templates and default .fledgeignore are embedded).
- `internal/lock`: `encoding/json` (lock records), `os` (`O_EXCL` atomic create), `syscall` (PID liveness via `Kill(pid, 0)` in the CLI layer, informational only).
- `internal/check`: `regexp` (ID validation), `slices`, `sort`, `time`.
- `internal/graph`: `fmt`, `strings` only.
- `internal/repo`, `internal/scan`: `os/exec` (git), plus `os`/`path/filepath`/`sort`/`bytes`.

## Internal dependency edges
- `cmd/fledge` → `internal/cli`.
- `internal/cli` → all core packages (`check`, `graph`, `lock`, `repo`, `scan`, `spec`).
- `internal/check` → `internal/graph`, `internal/spec`.
- `internal/graph` → `internal/spec`.
- `internal/spec`, `internal/lock`, `internal/repo`, `internal/scan` → standard library (spec also uses go-yaml). No import cycles.

## Licensing
The project is licensed under the **GNU Affero General Public License v3** (`LICENSE`).
