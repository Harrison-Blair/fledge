---
generated: 2026-07-17T17:48:26Z
commit: 1c9011d6e6a06f72f96bc98e3b2bd99c408ab79e
agent: fledge-forager
fledge_version: 0.6.10
---

# Testing

Frameworks, how to run, and coverage patterns across the repo. Every test uses Go's standard `testing` package or `testscript`; no other framework appears anywhere.

## Frameworks

- **`testing`** (stdlib) — all unit tests, everywhere.
- **`github.com/rogpeppe/go-internal/testscript`** — the only external test dependency; drives txtar acceptance fixtures exclusively via `cmd/fledge/main_test.go` (`TestMain` registers `fledge` as a runnable command calling `cli.Run`; `TestScripts` runs every `.txtar` file).

## Running tests

```sh
go test ./...                                  # everything
go vet ./...
go test ./cmd/fledge -run TestScripts          # all 36 acceptance scripts
go test ./cmd/fledge -run TestScripts/init     # one script (init.txtar)
go test ./cmd/fledge -run TestScripts/init -v  # verbose, shows script trace
go test ./internal/spec -run TestAllocateID    # one unit test
```

## Acceptance tests (`cmd/fledge/testdata/*.txtar`, 36 files, ~3441 lines)

One file per CLI command area: `agents`, `await`, `broods_stale`, `check`, `criteria`, `dev`, `dev_preen`, `dev_rails`, `dev_refresh`, `dev_status`, `e2e`, `escalate`, `forager_contract`, `freshness_gate`, `graph`, `heartbeat`, `init`, `init_agents`, `ledger-read`, `lock`, `nest`, `nest_status`, `new`, `plan_delegation`, `preen_scaffold`, `pulse`, `ready`, `refresh_scaffold`, `report`, `roster`, `scan`, `set`, `stamp_warning`, `status`, `unfledged`, `verdict`.

Notable coverage: `init.txtar`/`init_agents.txtar` (multi-harness scaffolding + auto-detect), `freshness_gate.txtar` (nest-staleness gate), `plan_delegation.txtar` (planning.md gates), `forager_contract.txtar` (vocabulary-leakage guard between raw scout output and synthesized docs), `preen_scaffold.txtar`/`refresh_scaffold.txtar` (drift detection, prune, confirm-before-overwrite), `stamp_warning.txtar` (version-mismatch warning, pinned old/new version pair), `dev*.txtar` (dev-mode symlink behavior: linking, no false drift, preserve-on-refresh, bare `--dev`, link status), `e2e.txtar` (full plumage→feather lifecycle).

Deterministic test environment: `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_SYSTEM` set to `/dev/null`, `GIT_AUTHOR_*`/`GIT_COMMITTER_*` fixed to `test`/`test@example.invalid`.

## Unit tests by package (representative, not exhaustive — counts are exact per-scout `grep -c`/file counts at authoring time)

- **`internal/spec`**: 4 test files, 22 `func Test*` (frontmatter split incl. CRLF, unknown-field handling, round-trip render/reparse, YAML quoting, atomic writes, `NextID` sequencing incl. gaps/widening, 22-goroutine concurrent-allocation race, kebab-casing incl. Unicode, criteria parsing incl. checkbox variants/indentation, single-byte mutation idempotence).
- **`internal/cli`**: 11 test files in scouted set (17 total in repo) — `await_test.go` (fake-clock polling, existence- vs. change-wait, baseline-race/identical-payload-rewrite guards), `command_parity_test.go` (commandOrder/commands sync), `escalate_test.go`, `init_test.go` (promptYesNo reader injection), `ledgerread_test.go`, `lock_test.go`, `pulse_test.go`, `update_swap_test.go`, `update_test.go`, `verdict_test.go`, `version_test.go`.
- **`internal/check`**: 13 test functions — schema errors, duplicate IDs, dangling refs, unhatched plumage, cycles, missing sections, stale pipping hint, lock consistency, incomplete/legacy criteria, evidence integrity.
- **`internal/graph`**: 3 test functions — Waves, WavesCycle, Cycle, Ready.
- **`internal/ledger`**: 7 test functions incl. `ConcurrentWrites` (16 goroutines), `ClassifyLiveness`, `StaleAfterIsFiveMinutes`.
- **`internal/lock`**: 7 test functions incl. `AcquireContention` (16 goroutines), corrupt-brood-file skip.
- **`internal/nest`**: 6 test functions — frontmatter key order (concern vs. scout), body-preservation on render, stamp drops unknown keys, stub detection, `Status()`, YAML scalar quoting.
- **`internal/repo`**: 1 test function.
- **`internal/roster`**: 5 test functions incl. `ConcurrentAssignYieldsDistinctSpecies` (18 goroutines × 5 rounds), pair-release-only-when-both-released.
- **`internal/scan`**: 3 test functions (Run, RunNoScanIgnore, RunEmptyRepo).
- **`internal/bootstrap`** (top-level `.go` + 17 invariant test files, ~1771 lines): `registry_test.go` (678 lines — manifest parsing, embed-FS source validation, primitive-coverage/tier-derivation correctness for all three shipped adapters), `drift_test.go` (433 lines — table test over all 5 `DriftStatus` values), `stamp_test.go` (215 lines — round-trip, absent-stamp, deterministic marshaling). Remaining 14 files assert on the **embedded prose itself** — section presence, exact wording/markers (e.g. `brooder_test.go` checks the fix-loop sentence, `ledger_handoff_test.go` validates `await --kind/--exists/--timeout` correctness across all 7 state-bearing files, `pid_liveness_test.go` asserts *absence* of stale "pid-alive" prose). Pattern: read embedded file bytes, assert presence/absence of markers — zero mocks or fixtures.

## Repo-consistency test packages (no production code, all in support-pkgs)

- **`internal/ciconfig`**: validates `.github/workflows/{release,pr-check}.yml` structure (triggers, safety-net commands, 4-platform matrix, job wiring) — catches CI-config drift the same way bootstrap tests catch prose drift.
- **`internal/doctest`**: validates README.md/RELEASING.md/CLAUDE.md cross-references (command mentions, scaffold-refresh documentation, role-file pointers).
- **`internal/hooktest`**: end-to-end integration test of `scripts/hooks/pre-commit` against a real temporary git repo (gofmt-block, vet-block, clean-allow, no-op-when-unconfigured, command-parity-with-CI).

## Coverage philosophy (observed pattern across the whole repo)

Three layers, deliberately non-overlapping: (1) unit tests beside each package prove function-level correctness with table-driven cases and explicit concurrency/race tests; (2) txtar acceptance tests in `cmd/fledge/testdata/` prove end-to-end CLI behavior (exit codes, stdout/stderr, JSON shapes) via `exec`/`stdout`/`! stdout` assertions; (3) "invariant" tests (`internal/bootstrap/*_test.go`, `ciconfig`, `doctest`, `hooktest`) prove that generated/prose/config artifacts stay consistent with each other and with the source that's supposed to produce them — a documentation/config drift guard unique to this repo's dogfooding structure.
