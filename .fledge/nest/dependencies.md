---
generated: 2026-07-16T04:02:08Z
commit: 154510fc963e7071b2f09297ecfeba2b6710e85e
agent: fledge-forager
fledge_version: 0.5.8
---

# Dependencies

External libraries, tools, and services used across the repo, deduplicated with usage notes. From `go.mod` (module `github.com/Harrison-Blair/fledge`, Go 1.26.4): 2 direct, 2 indirect.

## Go module dependencies

| Dependency | Kind | Used by | Purpose |
|---|---|---|---|
| `github.com/goccy/go-yaml v1.19.2` | direct | `internal/spec` (frontmatter parsing), `internal/bootstrap` (manifest.yaml loading), `internal/ciconfig` (test-only workflow YAML parsing) | YAML unmarshal/marshal — frontmatter and adapter manifests. |
| `github.com/rogpeppe/go-internal v1.15.0` | direct | `cmd/fledge` (main_test.go), all 25 `.txtar` acceptance fixtures | `testscript` — interprets shell-like `.txtar` scripts as acceptance tests, running the CLI in-process. |
| `golang.org/x/sys v0.26.0` | indirect | (transitive) | Low-level syscall support. |
| `golang.org/x/tools v0.26.0` | indirect | (transitive) | Tooling support (likely pulled in by go-yaml or testscript). |

## Standard library (notable, non-obvious usage)

- `syscall` — `internal/roster` (flock for roster.json), `internal/spec/ids.go` (flock on `.alloc.lock` to serialize ID allocation).
- `os.Link` — `internal/lock` — atomic, exclusivity-guaranteeing brood-claim creation (EEXIST = conflict).
- `archive/tar`, `archive/zip`, `compress/gzip`, `crypto/sha256`, `net/http` — `internal/cli/update.go` — self-update: fetch GitHub release, verify checksum, unpack, swap binary.
- `text/template` — `internal/bootstrap/registry.go` — renders `generate`/`primitive_map` scaffold files (adapter.md, settings.local.json, etc.).
- `embed` — `internal/bootstrap/bootstrap.go` (`core/` + `adapters/` trees), `internal/spec/templates.go` (plumage/feather skeletons), `internal/nest` (concern-doc/index/scout-report templates).

## External tools / services

- **Go toolchain** (`gofmt`, `go vet`, `go build`, `go test`) — invoked identically by CI (`pr-check.yml`, `release.yml`) and the optional local `scripts/hooks/pre-commit`.
- **GitHub Actions** — `actions/checkout@v4`, `actions/setup-go@v5` (reads `go-version` from `go.mod`), `actions/upload-artifact@v4`, `actions/download-artifact@v4`.
- **`gh` CLI** — `gh release create "v$VERSION"` in `release.yml`, the only place a release is actually published.
- **git** — used extensively at runtime (not just dev tooling): `internal/repo` shells out to `git rev-parse --show-toplevel`/`HEAD`; `internal/scan` uses `git ls-files` + `git check-ignore` for module discovery and `.fledgeignore` filtering.

## No third-party services in the product itself

fledge the CLI has no runtime network dependency except `fledge update` (GitHub Releases API + asset download) — everything else is local filesystem + git + spawned agent harness. The `docs/google_ai_mode_response.md` document (multi-tier AI routing, vLLM/OpenCode/Claude/DeepSeek references) is an orthogonal personal-infrastructure exploration, not a dependency of the fledge product.

## Open Questions

- Why `golang.org/x/tools` is an indirect dependency — not confirmed which direct dependency pulls it in.
