---
generated: 2026-07-17T17:48:26Z
commit: 1c9011d6e6a06f72f96bc98e3b2bd99c408ab79e
agent: fledge-forager
fledge_version: 0.6.10
---

# Dependencies

External dependencies for the `fledge` Go module (go.mod), deduplicated with usage notes.

## Go modules (direct)

- **`github.com/goccy/go-yaml v1.19.2`** — YAML parsing/unmarshaling. Used for: spec frontmatter parsing (`internal/spec/frontmatter.go`), adapter `manifest.yaml` parsing (`internal/bootstrap/registry.go`), and YAML validation in `internal/nest`/`internal/ciconfig`/`internal/doctest`.
- **`github.com/rogpeppe/go-internal v1.15.0`** — provides `testscript`, the txtar-based acceptance-test framework. Used exclusively by `cmd/fledge/main_test.go` to drive all 36 `.txtar` fixtures under `cmd/fledge/testdata/`.

## Go modules (indirect)

- **`golang.org/x/sys v0.26.0`** — system-level calls (transitively required, e.g. by flock usage patterns).
- **`golang.org/x/tools v0.26.0`** — Go tooling support (transitively required).

## Go toolchain

- **Go 1.26.4** (go.mod). No Makefile anywhere in the repo — `go build`/`go test`/`go vet` invoked directly.

## Standard library (heavy usage, notable packages)

- `flag` — every CLI command's argument parsing (`flag.FlagSet` with `ContinueOnError`, custom `parseMixed()` helper for mixed positional/flag args in brood.go).
- `syscall` (flock) — concurrency serialization in two independent packages: `internal/spec/ids.go` (ID allocation) and `internal/roster/roster.go` (species assignment) — same pattern, not shared code.
- `crypto/sha256` — content hashing for scaffold drift detection (`internal/bootstrap/stamp.go`) and binary checksum verification (`internal/cli/update.go`).
- `archive/tar`, `archive/zip`, `compress/gzip` — binary release extraction in `internal/cli/update.go`.
- `net/http` — GitHub API client for `fledge update`'s latest-release check.
- `text/template` — renders adapter templates (`fledge-adapter.md` primitive-map docs, `settings.local.json` permission allow-lists) in `internal/bootstrap/registry.go`.
- `encoding/json` — ledger records, lock/brood records, roster state, scaffold stamp — all persisted as JSON.
- `os/exec` — shells out to `git` (branch detection in `brood.go`, `init.go`).

## External services

- **GitHub Actions** — CI/CD, `.github/workflows/pr-check.yml` (every PR: gofmt, vet, build, test) and `.github/workflows/release.yml` (every push to main; safety-net, then 4-platform build+publish if VERSION changed: linux/{amd64,arm64}, darwin/{amd64,arm64}).
- **GitHub Releases API** (`https://api.github.com`) — polled by `fledge update` to check for a newer release; `release.yml` publishes built binaries and checksums there.

## Embedded/no-dependency modules

`internal/bootstrap/core/skills/` (the orchestration workflow prose) has **no external dependencies at all** — pure markdown, embedded via `//go:embed` and shipped inside the binary. It is read/tested only via Go's `testing` package asserting on embedded-file content (17 invariant test files in `internal/bootstrap/*_test.go`).

## Notably absent

- No web framework, no database driver, no ORM — the entire persistence model is flat files (markdown + JSON) under `.fledge/`.
- No third-party CLI-argument or logging library — `flag` and `fmt`/`os.Stderr` throughout.
- No third-party test framework beyond `testscript` (acceptance only); all unit tests use bare `testing.T`.
