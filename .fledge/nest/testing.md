---
generated: 2026-07-16T21:27:15Z
commit: a1ed62a38540df7ab1cbdc4c486176a64a762018
agent: fledge-forager
fledge_version: 0.5.8
---

# Testing

Frameworks, how to run tests at every scope, and what each test layer actually covers.

## Frameworks

- **Go standard `testing`** — all unit tests, across every `internal/*` package.
- **`testscript`** (`github.com/rogpeppe/go-internal/testscript`) — drives `.txtar` acceptance-test fixtures; configured in `cmd/fledge/main_test.go` with a deterministic git environment (fixed `GIT_*` env vars and author identity) so fixture output is reproducible.
- No mocking framework or third-party assertion library anywhere in the repo — plain `t.Errorf`/`t.Fatalf`, `t.TempDir()`, `t.Cleanup()`, `sync.WaitGroup` for concurrency tests.

## How to run

```sh
go test ./...                                        # everything
go vet ./...

go test ./cmd/fledge -run TestScripts                 # all txtar acceptance tests
go test ./cmd/fledge -run TestScripts/init             # one fixture (init.txtar)
go test ./cmd/fledge -run TestScripts/init -v          # verbose, shows script trace

go test ./internal/spec -run TestAllocateID            # one unit test
go test ./internal/bootstrap -run TestAdapterManifests
```
Unit tests live beside their package (`internal/spec/ids_test.go` next to `ids.go`, etc.) — this is the universal placement convention, no separate `_test` module.

## Acceptance tests (`cmd/fledge/testdata/*.txtar`, 25 fixtures)

Each `.txtar` is an executable spec: shell-like commands plus embedded file fixtures, asserting end-to-end CLI behavior. Grouped by concern:
- **Scaffold/init**: `init.txtar`, `init_agents.txtar`, `refresh_scaffold.txtar`, `preen_scaffold.txtar`, `agents.txtar`
- **Spec lifecycle**: `new.txtar`, `status.txtar`, `unfledged.txtar`
- **Mutation**: `set.txtar`, `criteria.txtar`
- **Validation**: `check.txtar`
- **Claiming/locking**: `lock.txtar`, `broods_stale.txtar`
- **Dependency graph**: `graph.txtar`, `ready.txtar`
- **Reporting**: `colony.txtar`, `report.txtar`, `scan.txtar`
- **Context/foraging**: `nest.txtar`, `nest_status.txtar`, `roster.txtar`
- **Planning/workflow gates**: `freshness_gate.txtar`, `forager_contract.txtar`, `plan_delegation.txtar`, `e2e.txtar`
- **Edge cases**: `stamp_warning.txtar` (version-mismatch warning; content pinned in sync with `VERSION` — `version_test.go:TestStampWarningTxtarVersionMatchesBinary`)

**Important**: `internal/bootstrap/core/` or `adapters/` prose/content changes must update these fixtures (especially `init.txtar`, `init_agents.txtar`, `agents.txtar`) alongside — they assert on the literal scaffolded output.

## Unit test coverage by package

- **`internal/spec`** (22 tests across `criteria_test.go`, `frontmatter_test.go`, `ids_test.go`, `load_test.go`): checkbox parsing/toggling (CRLF-safe, idempotent), frontmatter round-trip (parse→render→reparse identical), atomic write leaves no temp files, concurrent ID allocation (20 goroutines × 5 rounds, no dupes via flock), `Kebab()` unicode handling.
- **`internal/check`** (19 tests): all 14 validation rules — parse errors, schema, duplicate IDs, dangling refs, unhatched-plumage, cycles, missing sections, stale-pipping hints, brood consistency, criteria-incomplete/evidence, clean baseline.
- **`internal/lock`** (8 tests): acquire/release/get/list lifecycle, worktree field round-trip + pre-Worktree backward compat, held-conflict detection, **16-way contention test asserting exactly 1 winner**, corrupt-file skipping, atomicity under churn.
- **`internal/graph`** (4 test groups): topological waves, cycle detection (5 subtests: acyclic, self-loop, two-cycle, buried cycle, dangling deps ignored), ready-set filtering.
- **`internal/cli`** (17 tests across 6 files): command-registry/order parity, init confirmation-prompt edge cases, brood-rollback-on-status-write-failure, `fledge update` flow (5 tests with `httptest` GitHub API mocking), archive-build/checksum/atomic-swap (6+ tests), version-consistency checks.
- **`internal/bootstrap`** (41 test functions across 15 files, per `grep -c "^func Test" internal/bootstrap/*_test.go`): manifest parsing/primitive-coverage/tier-matching, core-prose neutrality guard (`TestCoreNeutral`), write-classification (created/updated/skipped, refresh idempotence, edit preservation), symlink resolution + duplicate guard, drift detection table test (all 5 statuses), stamp round-trip/determinism, plus ~15 embedded-prose "guard" tests that pin specific sentences/sections in the scaffolded skill docs (e.g. `TestBrooderFixLoopInvariant`, `TestSkuaRedTeamPass`, `TestIncubatorDocDescribesScratchpadBatching`) — these fail if `core/skills/*.md` prose drifts from what the tests expect.
- **`internal/nest`** (in `nest_test.go`): frontmatter key order (concern vs scout), body byte-preservation, stub detection (`IsStub`), `fledge nest status` verdict logic across fresh/complete/stale-index/missing-doc states.
- **`internal/roster`** (5 tests): full 18-species order, sequential allocation + numeric-suffix overflow, pair-release-frees-only-when-both-released, list omits fully-released, 18-goroutine concurrent allocation under flock.
- **`internal/repo`, `internal/scan`** (1 and 3 tests): dir-path construction; module grouping/byte-counts/`.fledgeignore` filtering/empty-repo case.
- **`internal/doctest`** (2 tests): substring assertions that README documents the update command and RELEASING.md covers scaffold refresh, and that root `CLAUDE.md` lists the three role-protocol files.
- **`internal/hooktest`** (5 tests): drives `scripts/hooks/pre-commit` against real temp git repos — blocks unformatted/vet-failing commits, allows clean ones, no-ops without `core.hooksPath` set, and (`TestPreCommitHook_MatchesCICommands`) asserts the hook's commands match `pr-check.yml` exactly.
- **`internal/ciconfig`** (7 tests, no production code — test-only package): asserts `.github/workflows/release.yml` structure (push→main trigger, 4-platform build matrix restricted to linux/darwin × amd64/arm64, release job needs `detect-version`) and `pr-check.yml` structure (pull_request→main, lint/build/test commands present).

## Test-first discipline (process convention, not a test-runner detail)

Every feather's AC-1 is defined by protocol (`internal/bootstrap/core/skills/fledge-orchestrate/brooder.md`, `skua.md`, `templates/feather.md`) to be test-first evidence: write the test, run it against unimplemented code, capture verbatim FAILING output in `.fledge/molt/FTHR-###.md`, then implement until passing. Skua review re-runs tests in the brooder's worktree and audits that the pre-impl failure evidence is genuine (not a setup error or weakened test).

## Local lint safety net (optional, opt-in)

`git config core.hooksPath scripts/hooks` activates `scripts/hooks/pre-commit`, which runs `gofmt -l .` then `go vet ./...` before allowing a commit — mirrors the CI `pr-check.yml` gate exactly.

## Open Questions

None observed.
