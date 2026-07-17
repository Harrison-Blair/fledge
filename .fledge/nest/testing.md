---
generated: 2026-07-17T07:00:54Z
commit: ee49464adb830bef7189f94a1d3253927d33fb5f
agent: fledge-forager
fledge_version: 0.6.7
---

# Testing

Test frameworks, how to run them, and coverage patterns across the codebase.

## Two test layers

1. **CLI acceptance tests** — testscript/txtar fixtures under `cmd/fledge/testdata/`, driven by `cmd/fledge/main_test.go`'s `TestScripts` (uses `github.com/rogpeppe/go-internal`'s testscript engine). Run: `go test ./cmd/fledge -run TestScripts` (all) or `go test ./cmd/fledge -run TestScripts/<name>` (one, e.g. `init`); add `-v` for the script trace. Git identity/config is isolated per test via testscript's `Setup` callback (`GIT_AUTHOR_*`, `GIT_CONFIG_*`) to avoid cross-test contamination.
2. **Go unit tests** — beside their packages (e.g. `internal/spec/criteria_test.go`), run via `go test ./...` or scoped, e.g. `go test ./internal/spec -run TestAllocateID`.

Both layers plus `go vet ./...` are what CI (`pr-check.yml`) and the optional local pre-commit hook (`scripts/hooks/pre-commit`, opt-in via `git config core.hooksPath scripts/hooks`) run before a commit/PR is accepted.

## CLI acceptance-test count and shape

**37 txtar fixtures** (`ls cmd/fledge/testdata/*.txtar | wc -l` = 37). Each fixture: an opening comment block naming the feather/plumage it traces to, `exec fledge <cmd> <args>` interleaved with `stdout`/`stderr`/`grep`/`exists`/`!` assertions, and fixture files in `-- <path> --` sections. Exit codes are asserted via `!` (expect-nonzero) or `sh -c 'cmd; test $? -eq N'` for a precise code.

Fixtures touching areas new since the last context regeneration:
- `await.txtar` (7 assertions) — `--exists` existence-wait, `--timeout`, mandatory `--kind`, `ExitTimeout` (4) on elapse.
- `verdict.txtar` (6 assertions) — `--result pass/fail`, `--note`, `--json`, validation.
- `escalate.txtar` (5 assertions) — `--message`, `--json`.
- `ledger-read.txtar` (9 assertions) — round-trip across all 3 kinds, `--kind` enum validation, path-traversal guard.
- `dev_preen.txtar` (5 assertions) — `fledge preen` on a dev-linked repo produces no false drift.
- `dev_rails.txtar` (8 assertions) — `fledge init --dev` bare form, non-fledge repos, tracked-path guard, version skew.
- `dev_refresh.txtar` (7 assertions) — `fledge init --refresh` preserves dev links, re-points, is idempotent, handles a vanished source.
- `dev_status.txtar` (5 assertions) — `fledge dev status` reports linked/unlinked/broken-link.

Other notable fixtures: `init.txtar` (22 assertions, the largest — idempotent scaffolding, agent scaffolding, team-loop generalization); `init_agents.txtar` (17); `lock.txtar` (18 — brood/abandon/broods lifecycle, stale-PID detection, `--force`); `roster.txtar` (15); `unfledged.txtar` (13); `report.txtar` (12); `status.txtar` (12); `set.txtar` (11); `criteria.txtar` (10); `nest.txtar` (14 — nest new/scout/stamp/scaffold, stub/stale detection); `preen_scaffold.txtar` (10); `forager_contract.txtar` (10 — planning.md/incubator.md forbid pipeline-stage leakage, enforce two-input framing).

## Unit test coverage by package

- **`internal/spec`**: `criteria_test.go` (9 tests — parsing, CRLF handling, single-byte flip correctness, idempotency); `frontmatter_test.go` (8 tests — split/parse/round-trip/atomic-write); `ids_test.go` (5+ tests, including `TestConcurrentAllocationYieldsDistinctIDs`: 20 goroutines × 5 rounds, all distinct via flock); `load_test.go` (2 tests, resilient parse-error handling).
- **`internal/check`**: `check_test.go` — **18 tests** covering every validation rule (duplicate-id, dangling-ref, unhatched-plumage, cycle, tests-section, stale-pipping-hint, brood-consistency, criteria-evidence, etc.).
- **`internal/graph`**: `graph_test.go` — 3 tests (`TestWaves`, `TestWavesCycle`, `TestCycle` with 5 sub-scenarios, `TestReady`).
- **`internal/scan`**: `scan_test.go` — 3 tests (complete scan with byte-count assertions, no-ignore-file, empty repo).
- **`internal/repo`**: `repo_test.go` — 1 test (path construction).
- **`internal/ledger`**: `ledger_test.go` — 13 tests (round-trip, overwrite-latest, invalid-subject rejection incl. path traversal, concurrent 16-way writes with atomicity check, liveness classification at TTL boundaries).
- **`internal/lock`**: `lock_test.go` — 8 tests (acquire/release/get, contention: 16 goroutines exactly one winner, corrupt-file skip+report, 500-cycle atomicity stress test).
- **`internal/nest`**: `nest_test.go` — 7 tests (frontmatter key order, body preservation, stub detection, `RefreshDoc` unknown-key dropping).
- **`internal/roster`**: `roster_test.go` — 5 tests (18-entry species list order, overflow to `-2`, pair release tracking, concurrent 18-way assign via flock).
- **`internal/cli`**: 14 test files — `await_test.go` (fake-clock-driven, no real sleeps; both wait modes × appearance/change/timeout); `verdict_test.go`/`escalate_test.go` (`setupLedgerRepo()` helper); `ledgerread_test.go` (path-traversal defense); `command_parity_test.go` (commandOrder ↔ commands map parity guard); `lock_test.go` (brood rollback on status-write failure); `update_test.go`/`update_swap_test.go` (`httptest`-mocked GitHub API, checksum/atomicity).
- **`internal/bootstrap`**: 15 test files, ~1966 lines (`wc`) — `registry_test.go` (manifest loading/coverage), `drift_test.go` (5-status classification incl. dev-link), `stamp_test.go` (expected-files derivation core/adapter/dev), plus higher-level integration tests (`brooder_test.go`, `incubator_test.go`, `planning_digest_test.go`, `core_docs_repoint_test.go` — checks core skill prose stays in sync).
- **`internal/ciconfig`**: 7 tests total pinning `.github/workflows/{release,pr-check}.yml` shape (triggers, matrix, job dependencies).
- **`internal/doctest`**: 3 tests pinning specific doc sections (`README.md` documents `fledge update`; `RELEASING.md` covers scaffold refresh; `CLAUDE.md` references role files).
- **`internal/hooktest`**: `precommit_test.go` — 5 tests exercising `scripts/hooks/pre-commit` end-to-end in temp repos (blocks unformatted/unvetted commits, allows clean ones, no-ops without `core.hooksPath` set, matches CI commands exactly).

## Test-writing idioms

- Table-driven and property-based patterns throughout; `*testing.T` always the first helper param; lowercase helper names (`req`, `task`, `newSet`, `initRepo`).
- Concurrency correctness is tested directly with goroutine fan-out + assertion of exactly-one-winner or all-distinct outcomes, not just single-threaded paths (ID allocation, lock contention, ledger writes, roster assignment).
- Fake/injectable clocks (`awaitClock`) used to keep polling-based logic (`fledge await`) fast and deterministic in tests.

## Open Questions

None observed — testing conventions and coverage were consistent and thorough across every scouted module; no contradictions needed resolving.
