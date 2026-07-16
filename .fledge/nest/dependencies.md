---
generated: 2026-07-16T21:27:15Z
commit: a1ed62a38540df7ab1cbdc4c486176a64a762018
agent: fledge-forager
fledge_version: 0.5.8
---

# Dependencies

External libraries, tools, and services the repo build, test, and release pipeline relies on. Go standard library usage is noted only where load-bearing (crypto, embed, flock); routine stdlib (fmt, strings, sort, ...) is omitted.

## Go module dependencies (`go.mod`)

Module: `github.com/Harrison-Blair/fledge`. Go **1.26.4** minimum.

- **`github.com/goccy/go-yaml` v1.19.2** (direct) — YAML parsing throughout: adapter `manifest.yaml` loading (`internal/bootstrap/registry.go`), spec frontmatter unmarshaling (`internal/spec/frontmatter.go`), nest frontmatter (`internal/nest/nest.go`), and `internal/ciconfig` test assertions on GitHub Actions workflow YAML (decoded generically as `map[string]any`, no struct unmarshal).
- **`github.com/rogpeppe/go-internal` v1.15.0** (direct) — `testscript` package; drives all 25 `.txtar` acceptance-test fixtures under `cmd/fledge/testdata/` (`cmd/fledge/main_test.go`).
- **`golang.org/x/sys` v0.26.0** (indirect, via rogpeppe) — syscall support.
- **`golang.org/x/tools` v0.26.0** (indirect, via rogpeppe) — Go tooling support.

## Load-bearing standard library usage

- **`os/exec`** — shells out to `git` throughout: `internal/repo` (`git rev-parse --show-toplevel`), `internal/scan` (`git ls-files -z --cached --others --exclude-standard`, `git check-ignore --stdin`), `internal/cli/brood.go` (current-branch detection).
- **`syscall`** (Flock) — exclusive-lock pattern reused in two places: `internal/spec/ids.go` (`.alloc.lock`, race-free ID allocation) and `internal/roster/roster.go` (`roster.json`, concurrent species assignment).
- **`crypto/sha256`** — content hashing for scaffold drift detection and the `.fledge/scaffold.json` stamp (`internal/bootstrap/{drift,stamp}.go`); also binary checksum verification in `fledge update` (`internal/cli/update.go`).
- **`embed`** (`//go:embed`) — embeds `core/` and `adapters/` trees into the fledge binary (`internal/bootstrap/bootstrap.go`), and spec templates (`internal/spec/templates.go`), and nest templates (`internal/nest` `templates/`).
- **`text/template`** — renders `generate`-policy scaffold files: `fledge-adapter.md`, `settings.local.json` (`internal/bootstrap/registry.go`).
- **`archive/tar`, `archive/zip`, `compress/gzip`** — release-archive extraction in `fledge update` (`internal/cli/update.go`).
- **`net/http`** — GitHub Releases API polling in `fledge update` (`internal/cli/update.go`).

## External services / tools

- **GitHub Actions** — CI/CD: `.github/workflows/pr-check.yml` (lint/build/test gate on PRs to main), `.github/workflows/release.yml` (detects VERSION change on push to main, cross-compiles 4 platform binaries, publishes a GitHub Release). Actions used: `actions/checkout@v4`, `actions/setup-go@v5`, `actions/upload-artifact@v4`, `actions/download-artifact@v4`.
- **`gh` (GitHub CLI)** — release creation (`gh release create`) in the release workflow.
- **Git** — version control and the mechanism `internal/repo`/`internal/scan` shell out to; also required at runtime by CLI commands that read repo state.
- **`git` pre-commit hook** (`scripts/hooks/pre-commit`, opt-in via `git config core.hooksPath scripts/hooks`) — runs `gofmt -l .` and `go vet ./...` locally, mirroring the CI lint gate exactly (asserted by `internal/hooktest`).

## Agent-Skills standard (design-level dependency, not code)

Per `docs/generalization-plan.md` and the bootstrap adapter design: fledge's `core/skills/` conforms to the cross-harness "Agent Skills" format (frontmatter with `name`/`description`) so pi, Claude Code, and Codex can all load it natively without translation.

## Open Questions

- `docs/google_ai_mode_response.md` references external inference infrastructure (OpenCode Go/Zen APIs, local vLLM/SGLang/Ollama servers, Google Gemini) — unclear whether this is a live fledge integration target or standalone research unrelated to the current codebase (flagged by the `docs` scout).
