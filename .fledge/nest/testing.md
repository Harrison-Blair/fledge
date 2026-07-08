---
generated: 2026-07-08T05:28:12Z
commit: e46c481a047d45ef10bcd79a3326d47932b32868
agent: fledge-forager
fledge_version: 0.2.1
---

# Testing

Two test styles, both run with plain `go test`. Know which to touch for a given change.

## CLI acceptance tests — testscript/txtar

- Framework: `github.com/rogpeppe/go-internal/testscript`; files in `cmd/fledge/testdata/*.txtar` (16 files, one per command/area). Harness in `cmd/fledge/main_test.go` registers the `fledge` command and pins a deterministic git env (`GIT_CONFIG_GLOBAL=/dev/null`, fixed author/committer).
- Run: `go test ./cmd/fledge -run TestScripts` (all), `…/TestScripts/init` (one), add `-v` for the script trace.
- Each file sets up a repo (git init, `.fledge/`, `pluma/` dirs, seed specs) then asserts on stdout (regex), exit codes (`!` prefix = expect failure), and file state (`exists`/`grep`/`cmp`).
- Coverage highlights: `init`/`init_agents`/`agents` (scaffolding, adapter auto-detect, `--refresh`, allow-list, duplicate guard), `new`/`status`/`set`/`criteria` (lifecycle + gates), `check` (validation), `graph` (waves/cycles/dot), `ready`, `lock` (brood/abandon/broods), `report` (colony: counts, orphans, blocked, locks, degraded data), `unfledged`, `scan`, `nest` (scaffold/new/scout/stamp), `e2e` (full init→abandon lifecycle).

## Unit tests — colocated

- Beside their package: `internal/spec`, `internal/check`, `internal/graph`, `internal/lock`, `internal/scan`, `internal/nest`, `internal/bootstrap`. Example: `go test ./internal/spec -run TestAllocateID`.
- Notable: `internal/nest/nest_test.go` (frontmatter key order, body byte preservation, YAML scalar quoting); `internal/cli/lock_test.go` (`TestLockRollsBackOnStatusWriteFailure`); `internal/cli/version_test.go` (`TestBinaryVersionMatchesVersionFile` — pins `binaryVersion` to `VERSION`); `internal/bootstrap/registry_test.go` (manifest parse, primitive coverage vs core prose, `TestCoreNeutral` = no harness paths in core, symlink idempotence, generated allow-list).

## What breaks when you change embedded content

Changing `core/` or `adapters/` content shifts scaffolded bytes → `init.txtar`, `init_agents.txtar`, `agents.txtar` (and `registry_test.go` neutrality/coverage tests) will fail until updated. Treat fixture updates as part of the change, not follow-up.

## Test-first discipline (project rule)

Feathers are test-driven: tests are written and observed **failing** against unchanged code (for the expected reason) before implementation, then the code is corrected until they pass — evidence captured per criterion in `.fledge/molt/FTHR-###.md`. A test that has only ever been seen passing does not count.

## Standard checks

`go build ./...`, `go vet ./...`, `go test ./...`. Go 1.26; no Makefile.
