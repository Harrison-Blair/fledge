---
generated: 2026-07-15T18:14:39Z
commit: 5728c29953a7c218c923ce20333dbffebb00623f
agent: fledge-forager
fledge_version: 0.5.4
---

# Dependencies

External Go modules, tooling, and harness-side capabilities fledge relies on, deduplicated across all modules with usage notes.

## Go module dependencies (`go.mod`)

- `github.com/goccy/go-yaml` v1.19.2 — YAML parsing/marshalling for adapter `manifest.yaml` files (`internal/bootstrap/registry.go` `loadManifest`) and spec frontmatter (`internal/spec/frontmatter.go`), used for its deterministic, canonical-key-order output.
- `github.com/rogpeppe/go-internal` v1.15.0 — `testscript` package; drives all `cmd/fledge/testdata/*.txtar` CLI acceptance tests via `TestScripts` in `cmd/fledge/main_test.go`.
- `golang.org/x/sys` v0.26.0 (indirect), `golang.org/x/tools` v0.26.0 (indirect) — transitive.
- Go **1.26.4** minimum (per `go.mod`); repo CLAUDE.md states Go 1.26.

## Standard library usage (notable, not exhaustive)

- `embed` — `internal/bootstrap/bootstrap.go`'s `//go:embed core adapters` bakes the entire skills/adapters tree into the binary; this is why `fledge init` needs no network or filesystem source beyond the binary itself.
- `syscall`/`os` file locking — `internal/spec/ids.go` uses exclusive `flock` on a per-directory `.alloc.lock` to serialize concurrent ID allocation.
- `os/exec` — `internal/repo/repo.go` shells out to `git rev-parse --show-toplevel`/HEAD lookups; `internal/scan/scan.go` shells out to `git ls-files` and `git check-ignore` (`.fledgeignore` filtering).
- `crypto/sha256` — content hashing for scaffold stamp entries (`internal/bootstrap/stamp.go`) and drift classification.
- `text/template` — renders `generate`/`primitive_map` policy files (each adapter's `fledge-adapter.md`, Claude's `settings.local.json` allow-list).
- `net/http`, `archive/tar`, `crypto/sha256` — `internal/cli/update.go` self-update: fetches GitHub releases, verifies checksum, swaps the binary.

## External tools (invoked as subprocesses, not linked)

- `git` — required for repo discovery, scanning, `.fledgeignore` filtering; fledge assumes it runs inside a git worktree (`ExitEnv` if not).
- `gofmt`, `go vet` — used by CI (`pr-check.yml`, `release.yml`) and the optional local pre-commit hook (`scripts/hooks/pre-commit`); shipped with Go, not a separate install.
- `gh` (GitHub CLI) — used by `release.yml`'s release job (`gh release create`); assumed present on the `ubuntu-latest` runner.

## GitHub Actions (CI/CD)

- `actions/checkout@v4`, `actions/setup-go@v5`, `actions/upload-artifact@v4`, `actions/download-artifact@v4` — all jobs run on `ubuntu-latest`; Go version resolved from `go.mod`.
- 4-platform release build matrix: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 (fledge is explicitly Unix-only per user memory — no Windows target).

## Per-harness runtime dependencies (adapters)

These are not Go dependencies — they're what each adapter's `manifest.yaml`/`fledge-adapter.md` assumes the *harness* provides to realize fledge's 6 primitives:

- **Claude Code** (Tier C, all 6 primitives): `AskUserQuestion` (confirm-gate), Bash tool (read-only-shell, run-fledge), Write tool (write-file), `SendMessage` (message-peer), `Task`/`TaskStop` tools (spawn-worker — spawn and *actual* termination are separate calls), **tmux** for the split-pane teammate display (precondition `test -n "$TMUX"`, with an in-process fallback gated by a confirm-gate per `implementation.md` §1 — see architecture.md and team-loop.md § Teammate display (tmux)), team-roster introspection (to confirm a shutdown actually completed: roster absence + pane close).
- **Codex CLI** (Tier A, 4/6 primitives): chat UI (confirm-gate), shell read-only (read-only-shell), `apply_patch`/edit (write-file), shell `fledge` (run-fledge); auto-loads `AGENTS.md` at repo root (append-policy file).
- **Pi** (Tier A, 4/6 primitives): bash tool (read-only-shell, run-fledge), write tool (write-file), chat (confirm-gate default) or the M4 extension `fledge_gate` tool (confirm-gate alternative); skills pointer `settings.json: ["../.fledge/skills"]`.

## Open Questions

None beyond what's tracked in architecture.md re: manifest validation completeness.
