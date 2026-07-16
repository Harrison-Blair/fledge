---
generated: 2026-07-16T02:20:48Z
commit: 407b91e70b53764944447dae5829d2076fb852c5
agent: fledge-forager
fledge_version: 0.5.5
---

# Dependencies

External libraries, tools, and services fledge relies on, deduplicated across modules with usage notes.

## Go module dependencies (`go.mod`)

- **`github.com/goccy/go-yaml` v1.19.2** — the only third-party runtime dependency. Used for: spec frontmatter YAML unmarshaling (`internal/spec/frontmatter.go`), adapter `manifest.yaml` parsing (`internal/bootstrap/registry.go`), and nest doc frontmatter (`internal/nest`).
- **`github.com/rogpeppe/go-internal` v1.15.0** — test-only. Provides `testscript`, the txtar-based acceptance-test runner used by `cmd/fledge/main_test.go` for all 23 `cmd/fledge/testdata/*.txtar` files.
- **`golang.org/x/sys`, `golang.org/x/tools`** — indirect; system calls and build tooling.

## Standard library (heavily relied on, no third-party substitutes)

- `crypto/sha256` — scaffold-file hashing for drift classification (`internal/bootstrap/stamp.go`).
- `syscall` (flock) — exclusive-lock serialization for spec ID allocation (`internal/spec/ids.go:AllocateAndCreate`), Unix-only.
- `os.Link` — atomicity primitive for brood-file acquisition (`internal/lock/lock.go`).
- `text/template` — renders scaffolded generated/overwrite-policy files (primitive maps, adapter entry files).
- `archive/tar`, `archive/zip`, `compress/gzip`, `net/http` — `fledge update`'s GitHub-release download/extract/verify pipeline (`internal/cli/update.go`).
- `io/fs`, `embed` — the `internal/bootstrap` embedded `core/`+`adapters/` trees.

## External services

- **GitHub Releases API** — `fledge update` (`internal/cli/update.go`) fetches `latest`/named releases; platform-aware asset selection (linux/darwin × amd64/arm64), SHA-256 checksum verification, atomic binary swap via temp-file rename.
- **GitHub Actions** — `.github/workflows/{pr-check,release}.yml` use `actions/checkout@v4`, `actions/setup-go@v5`, `actions/upload-artifact@v4`, `actions/download-artifact@v4`, and the `gh` CLI (implicit in the release step, `gh release create --generate-notes`).
- **git** (subprocess, via `os/exec`) — repo root discovery (`internal/repo`), HEAD SHA lookup, tracked/untracked file enumeration for `fledge scan` (`internal/scan`), and the hermetic per-test `git init` in the txtar acceptance harness.

## Build/CI tooling

- `go build`, `go test`, `go vet`, `gofmt` — the whole lint/test gate, identical between CI (`pr-check.yml`) and the optional local pre-commit hook (`scripts/hooks/pre-commit`), asserted textually identical by `internal/hooktest`.
- `tar`, `sha256sum`, `bash`/`sh` — release packaging and install scripting (`.github/workflows/release.yml`, `scripts/install.sh`).

## Notably absent

- No web framework, no database, no ORM — fledge is a filesystem-native CLI; all "state" is markdown files with YAML frontmatter plus small JSON side-files (`.fledge/scaffold.json`, `.fledge/broods/*.brood`).
- No mocking library — tests use `httptest` (stdlib) for the one network-touching command (`update`), and `t.TempDir()` + real subprocess `git` elsewhere.
