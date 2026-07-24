# Repository Guidelines

## Single user
I am the only user, "breaking" changes are acceptable, I am the only migration point

## Project Structure & Module Organization

Fledge is a Go CLI and per-flock daemon. The command entrypoint and CLI parsing live in `cmd/fledge/`. Domain packages are under `internal/` (for example, `daemon`, `protocol`, `workspace`, and `agentcfg`), with tests colocated as `*_test.go`. Build and installation helpers live in `scripts/`. Treat `docs/` as completed Stage 0 history unless current code or `CLAUDE.md` explicitly points there; `README.md` is the current user-facing command reference.

## Build, Test, and Development Commands

- `./scripts/build.sh` builds `./cmd/fledge` into `bin/fledge`.
- `./scripts/install.sh` installs the built binary to `GOBIN`, `GOPATH/bin`, or `BINDIR`.
- `go test ./...` runs the complete test suite.
- `go test -run TestSpawn ./internal/daemon/` runs a focused test.
- `gofmt -l .` lists files needing formatting; `go vet ./...` performs static checks.

Go 1.26+ and Unix are required. YAML frontmatter is parsed with the maintained
`github.com/goccy/go-yaml` dependency; otherwise keep dependencies lean.

## Coding Style & CLI Conventions

Use `gofmt`; write idiomatic Go with tabs, short lowercase package names, exported identifiers in `PascalCase`, and clear package comments. Keep synchronization and external calls separate: never hold daemon locks across Herdr calls or runner shutdown.

CLI parsing is deliberately hand-written in `cmd/fledge/main.go`; reuse `takeFlag`, `takeBoolFlag`, and `rejectFlags`. Flags use `--whole-flag` plus a globally unique uppercase short form. Check the flag table in `README.md` before assigning one. For new commands, consider whether `--json` output is useful to agents, and keep contextual help plus the README command surface synchronized.

## Testing Guidelines

Use Go's `testing` package and name tests `TestXxx` around observable behavior. Add regression tests with fixes and cover journal ordering, restart behavior, validation boundaries, and failure cleanup where relevant. Run focused package tests while developing, then `go test ./...`, `gofmt -l .`, and `go vet ./...` before submission. No numeric coverage threshold is enforced.

## Commits & Pull Requests

Use concise, action-oriented commit subjects consistent with history, such as `Add codex integration and discovered model catalog`. Pull requests should explain the behavior change, list tests run, link relevant issues, and include terminal output or screenshots for user-visible CLI changes.

## Architecture & Configuration

Preserve two invariants: Fledge's append-only journal is authoritative state, and the orchestrator performs no LLM inference. Do not commit generated `.fledge/flocks/`, locks, sockets, catalogs, binaries, or local environment files.

Portable agents live at `.fledge/agents/user/<name>/<name>.agent.md`; their
Markdown is authoritative and generated indexes must stay deterministic.
Fledge reserves the `fledge-*` namespace and the managed sibling directory.
