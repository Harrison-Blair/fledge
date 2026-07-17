---
generated: 2026-07-17T02:54:09Z
commit: e7a6d4969f861ed3f03af7833b750a7cd703a7a8
agent: fledge-forager
fledge_version: 0.5.8
---

# Dependencies

External (third-party) dependencies used across the repo, deduplicated, with usage notes. fledge intentionally has a minimal dependency footprint.

## Go module dependencies (`go.mod`, Go 1.26.4)

- **`github.com/goccy/go-yaml`** (v1.19.2) — the only YAML library used, everywhere YAML is parsed or rendered:
  - `internal/spec/frontmatter.go` — spec frontmatter parse/render (`parseFrontmatterMap`)
  - `internal/nest/nest.go` — nest doc frontmatter
  - `internal/bootstrap/registry.go` — adapter `manifest.yaml` parsing
  - `internal/ciconfig/*_test.go` — parses `.github/workflows/*.yml` for meta-tests
- **`github.com/rogpeppe/go-internal`** (v1.15.0) — provides the `testscript` package, the framework behind every `cmd/fledge/testdata/*.txtar` acceptance test (`cmd/fledge/main_test.go`).

No other third-party Go packages are imported anywhere in the repo.

## Go standard library (notable, by area)

- **File/spec I/O**: `os`, `path/filepath`, `bytes`, `embed` (spec + nest templates, bootstrap core/adapters trees)
- **Concurrency/locking**: `syscall` (`Flock` — spec ID allocation, roster state file, brood-adjacent locking patterns), `sync` (WaitGroup/Mutex in concurrency tests)
- **Hashing**: `crypto/sha256`, `encoding/hex` — scaffold stamp content hashes, update-binary checksum verification
- **Templating**: `text/template` — bootstrap `generate`/`primitive_map` file rendering
- **Networking**: `net`, `net/http` — `fledge update`'s GitHub Releases API fetch
- **Archive extraction**: `archive/tar`, `archive/zip`, `compress/gzip` — `fledge update`'s release-asset unpacking
- **JSON**: `encoding/json` — all `--json` command output, ledger records, lock/roster/scaffold state files
- **Time**: RFC3339 timestamps used uniformly for `authored` frontmatter fields and ledger records

## Runtime / process dependencies (not Go packages)

- **`git`** — required at runtime, not vendored. Used by: `internal/repo` (root resolution via `git rev-parse --show-toplevel`, HEAD SHA), `internal/scan` (`git ls-files -z --cached --others --exclude-standard`, `git check-ignore --stdin -z` for `.fledgeignore` filtering), `cmd/fledge/testdata` acceptance tests (isolated via `GIT_CONFIG_GLOBAL=/dev/null` and fixed `GIT_AUTHOR_NAME`/etc. for determinism).
- **GitHub Releases API + GitHub Actions** — `fledge update` fetches release metadata/binaries from GitHub; `.github/workflows/release.yml` builds and publishes them (4 Unix platform binaries: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64).
- **Harness-specific tools** (only relevant to the scaffolded/embedded `core`+`adapters` prose, not fledge's own build): Claude Code's `tmux` (teammate panes), `Bash`/`Write`/`AskUserQuestion`/`SendMessage`/`Task` tools; Codex CLI's shell/apply_patch; pi's bash/write tools + optional M4 extension (`fledge_gate`, `fledge_spawn`, not yet implemented).

## Dependency direction (internal)

- `internal/cli` depends on every domain package (`spec`, `check`, `graph`, `lock`, `repo`, `scan`, `nest`, `roster`, `ledger`, `bootstrap`) — never the reverse.
- `internal/check` and `internal/graph` both depend on `internal/spec` (shared `Requirement`/`Task` types).
- `internal/nest` depends on `internal/spec` (reuses `spec.YAMLScalar` for safe frontmatter quoting, `SplitFrontmatter`).
- `internal/bootstrap` is self-contained apart from `goccy/go-yaml` and stdlib — it does not import other `internal/` packages.
- `cmd/fledge` depends only on `internal/cli`.

## Open Questions

None observed — the dependency footprint is small and consistent across all scouted modules.
