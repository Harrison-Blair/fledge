---
generated: 2026-07-17T02:54:09Z
commit: e7a6d4969f861ed3f03af7833b750a7cd703a7a8
agent: fledge-forager
fledge_version: 0.5.8
---

# Testing

Test frameworks, how to run them, and coverage patterns across the repo. Go's built-in `testing` package is used everywhere — no third-party test framework except `testscript` for CLI acceptance tests.

## Running tests

```sh
go test ./...                                        # everything
go vet ./...                                          # static checks (CI gate)
gofmt -l .                                            # formatting check (CI gate)

# CLI acceptance tests (testscript/txtar), all or one:
go test ./cmd/fledge -run TestScripts
go test ./cmd/fledge -run TestScripts/init
go test ./cmd/fledge -run TestScripts/init -v         # verbose, shows script trace

# Unit tests, package-scoped:
go test ./internal/spec -run TestAllocateID
go test ./internal/check
```

No `-cover` flag is used in CI or documented workflows — coverage is not explicitly measured.

## Acceptance tests (`cmd/fledge/testdata/*.txtar`, 27 files)

Framework: `github.com/rogpeppe/go-internal/testscript`. `cmd/fledge/main_test.go` registers the `fledge` binary as `os.Exit(cli.Run(os.Args[1:]))` so scripts invoke it directly; `TestMain` sets deterministic `GIT_*` env vars (`GIT_CONFIG_GLOBAL=/dev/null`, fixed `GIT_AUTHOR_NAME`, etc.) so git behavior inside test repos is reproducible. Each `.txtar` file has a shell-script section (`exec`, `stdout`/`stderr` assertions) followed by embedded file fixtures.

Coverage by area (file → command under test):
- **Scaffold/init**: `init.txtar`, `init_agents.txtar`, `dev.txtar` (dev install mode), `refresh_scaffold.txtar`, `preen_scaffold.txtar`, `freshness_gate.txtar`
- **Spec CRUD**: `new.txtar`, `status.txtar`, `set.txtar`, `criteria.txtar`
- **Spec queries**: `check.txtar`, `preen.txtar`, `scan.txtar`, `graph.txtar`, `ready.txtar`, `unfledged.txtar`
- **Feather locking/lifecycle**: `e2e.txtar`, `lock.txtar`, `broods_stale.txtar`
- **Nest/context**: `nest.txtar`, `nest_status.txtar`
- **Runtime/process**: `heartbeat.txtar`, `roster.txtar`
- **Contract/quality**: `forager_contract.txtar`, `plan_delegation.txtar`, `stamp_warning.txtar`, `agents.txtar`
- **Reporting**: `report.txtar` (`fledge colony`)

## Unit tests (beside their package)

- **internal/spec**: `frontmatter_test.go` (7 tests — split/parse/round-trip/quoting/atomic write), `ids_test.go` (4 tests — sequential allocation, gap/width handling, 20-goroutine × 5-round concurrent allocation via flock, Kebab), `load_test.go` (2 tests), `criteria_test.go` (9 tests — checkbox extraction/toggle/CRLF/idempotence).
- **internal/check**: `check_test.go` — 20 tests covering all 15 named validation rules (`parse`, `unknown-field`, `duplicate-id`, `schema`, `id-filename`, `dangling-ref`, `unhatched-plumage`, `tests-section`, `required-sections`, `cycle`, `brood-consistency`, `stale-pipping-hint`, `criteria-incomplete`, `criteria-format`, `criteria-evidence`).
- **internal/graph**: `graph_test.go` — 3 tests (waves, cycle detection incl. self-loop/dangling-dep-is-not-a-cycle, readiness).
- **internal/lock**: `lock_test.go` — 8 tests, including `TestAcquireContention` (16 concurrent goroutines, exactly 1 wins) and `TestAcquireWritesAtomically` (500 acquire/release cycles with a concurrent reader, no partial/zero-length files observed).
- **internal/roster**: `roster_test.go` — 5 tests, including `TestConcurrentAssignYieldsDistinctSpecies` (18 concurrent assigns × 5 rounds).
- **internal/ledger**: `ledger_test.go` — 11 tests, including `TestConcurrentWrites` (16 concurrent writers, one value persists) and 9 invalid-subject rejection cases.
- **internal/nest**: `nest_test.go` — 7 tests (frontmatter key order per kind, body preservation through refresh, stub detection, `Status` completeness logic).
- **internal/repo**, **internal/scan**: `repo_test.go` (1 test), `scan_test.go` (3 tests — filtering, no-ignore, empty repo).
- **internal/bootstrap**: 15 test files, 105+ test functions — `stamp_test.go` (write/load round-trip, marshal determinism, all-policy `ExpectedFiles` coverage), `drift_test.go` (18 table cases across 5 statuses × 3 content types), `registry_test.go` (47 assertions — manifest parsing, tier coverage per harness, `WriteCore` classification, refresh policies, duplicate-skill guard), plus structural/prose-invariant tests for each per-role skill doc (`brooder_test.go`, `skua_test.go`, `incubator_test.go`) and digest/batching invariants (`planning_digest_test.go`, `implementation_digest_test.go`, `foraging_digest_test.go`, `planning_batching_test.go`, `interrogate_batching_test.go`, `tmux_autodefault_test.go`, `core_docs_repoint_test.go`, `worker_protocols_stub_test.go`, `compact_advisory_test.go`).
- **internal/cli**: `command_parity_test.go` (registration/order drift guard), `init_test.go`, `lock_test.go` (status-write-failure rollback), `update_test.go` + `update_swap_test.go` (mocked HTTP server, checksum verification, atomic binary swap), `version_test.go` (pins `binaryVersion` to `VERSION` file and a txtar fixture).

## Meta-tests (test-only packages, no production code)

- **internal/ciconfig**: `release_workflow_test.go`, `workflow_test.go` — assert `.github/workflows/*.yml` structure (triggers, safety-net steps, 4-platform build matrix) matches expectations by parsing the YAML with `goccy/go-yaml` and type-asserting.
- **internal/doctest**: `docs_test.go`, `claude_md_test.go` — assert README/RELEASING/CLAUDE.md cross-references stay accurate (e.g. CLAUDE.md references `incubator.md`/`brooder.md`/`skua.md`) via substring section extraction (tolerant of prose edits, no snapshots).
- **internal/hooktest**: `precommit_test.go` — 5 end-to-end tests running the real `scripts/hooks/pre-commit` script against temp git repos (blocks unformatted/vet-bad commits, allows clean ones, no-ops if `core.hooksPath` unset, matches CI's exact lint commands).

## Test-first discipline (implementation workflow, not a Go testing mechanism)

Per the orchestration prose (`internal/bootstrap/core/skills/fledge-orchestrate/`), every feather names its tests before implementation; the brooder runs them against unchanged code, captures the FAIL output verbatim into an evidence file (`.fledge/molt/FTHR-###.md`, `## AC-1` section), then implements to make them pass. The paired skua re-runs tests, audits that AC-1's evidence shows a real FAIL→PASS transition (not a test that only ever passed), and rejects weak tests.
