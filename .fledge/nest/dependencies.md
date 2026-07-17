---
generated: 2026-07-17T07:00:54Z
commit: ee49464adb830bef7189f94a1d3253927d33fb5f
agent: fledge-forager
fledge_version: 0.6.7
---

# Dependencies

External dependencies across the codebase, deduplicated, with usage notes.

## Go module dependencies (`go.mod`)

- **`github.com/goccy/go-yaml`** v1.19.2 — YAML parsing/unmarshaling. Used in two independent places: `internal/spec/frontmatter.go` (spec frontmatter parse/render, `YAMLScalar` safe-quoting) and `internal/bootstrap/registry.go` (adapter `manifest.yaml` parsing). Also used by `internal/nest` and `internal/ledger` (via `internal/spec`'s exported helpers) for their own frontmatter-shaped YAML.
- **`github.com/rogpeppe/go-internal`** v1.15.0 — provides `testscript`, the txtar-based CLI acceptance-test engine. Drives all 37 fixtures in `cmd/fledge/testdata/*.txtar` via `cmd/fledge/main_test.go`'s `TestScripts`.
- **`golang.org/x/sys`** v0.26.0 — indirect; syscalls (backs `flock`-based file locking in `internal/spec/ids.go`, `internal/roster/roster.go`).
- **`golang.org/x/tools`** v0.26.0 — indirect; Go tooling support.

No direct runtime network dependencies — `fledge update` (`internal/cli/update.go`) is the one command that talks to the network (GitHub Releases API) and only at the user's explicit request.

## Standard library usage patterns

- **`embed`** — `internal/bootstrap/bootstrap.go` embeds the entire `core/` and `adapters/` trees (`//go:embed core adapters`) into the binary; `internal/nest` similarly embeds its own placeholder templates.
- **`syscall.Flock`** — exclusive file locking for concurrent-safe state: `internal/spec/ids.go` (`.alloc.lock`, ID allocation), `internal/roster/roster.go` (`.roster.lock`, species allocation).
- **`os.Link`** — exclusivity primitive for feather claims: `internal/lock/lock.go` uses `O_EXCL`-style link semantics (link fails with `EEXIST` if a claim already exists) rather than flock.
- **`os.Rename`** — atomic replace for record-style state: `internal/ledger/ledger.go` (ledger records), `internal/spec/frontmatter.go` `WriteFileAtomic` (spec file writes) — both use temp-file-then-rename.
- **`crypto/sha256`** — content hashing for scaffold drift detection (`internal/bootstrap/stamp.go`, `drift.go`).
- **`text/template`** — rendering generated scaffold files (e.g. `fledge-adapter.md`, `settings.local.json`) from `renderContext`/`primitiveRow` data (`internal/bootstrap/registry.go`).
- **`net/http`, `archive/tar`, `compress/gzip`** — self-update mechanics in `internal/cli/update.go` (download, verify checksum, extract, atomic binary swap).
- **`exec`** — git subprocess calls in `internal/repo/repo.go` (`git rev-parse --show-toplevel`, `Head()`), and shelling out in test helpers (`internal/scan`, `internal/hooktest` invoking `scripts/hooks/pre-commit` end-to-end).

## Third-party GitHub Actions (`.github/workflows/*.yml`)

- `actions/checkout@v4`, `actions/setup-go@v5`, `actions/upload-artifact@v4`, `actions/download-artifact@v4` — standard CI plumbing for both `pr-check.yml` (lint/build/test gate) and `release.yml` (cross-platform build + release).
- **GitHub CLI** (`gh release create`) — used by `release.yml` to publish the release artifact.

## Test-only dependencies

- **`net/http/httptest`** — mocks the GitHub API in `internal/cli/update_test.go`/`update_swap_test.go` for self-update testing without real network calls.
- **`testscript`** (see above) is exercised only from `cmd/fledge/main_test.go`; no other package imports it.

## No dependency on

- No web framework, no database/ORM, no third-party CLI-flag library (uses stdlib `flag` directly), no logging framework (plain `fmt`/stdout/stderr).

## Open Questions

None observed — dependency set is small, stable, and consistently used across all nine scouted modules; `go.mod` fully enumerates direct + indirect dependencies with no undocumented third-party services.
