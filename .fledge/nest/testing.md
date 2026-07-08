---
generated: 2026-07-08T01:03:26Z
commit: e44524d1f089dcfe1c1f313f819ec18d9a42eceb
agent: fledge-forager
fledge_version: 0.2.1
---

# Testing

Two layers of tests: CLI acceptance tests (testscript/txtar, black-box through the built binary) and Go unit tests (white-box, beside each package). No Makefile — everything runs via `go test`.

## Running tests

```sh
go test ./...                                        # everything
go test ./cmd/fledge -run TestScripts                 # all CLI acceptance tests
go test ./cmd/fledge -run TestScripts/init             # one script (init.txtar)
go test ./cmd/fledge -run TestScripts/init -v           # verbose, shows script trace
go test ./internal/spec -run TestAllocateID              # a single unit test
go test ./internal/bootstrap -run TestAdapterManifests    # a single bootstrap test
go vet ./...
```

## CLI acceptance tests (`cmd/fledge/testdata/*.txtar`, testscript format)

Driven by `cmd/fledge/main_test.go`, which registers `fledge` as a testscript command and isolates git config (author, committer) for reproducibility. 17 `.txtar` files, each covering one command or workflow:

| File | Covers |
|---|---|
| `init.txtar` | repo scaffolding, `.fledge/` structure, idempotency, harness auto-detection |
| `init_agents.txtar` | multi-agent scaffolding: `--list-agents`, `--agent` override, `--refresh` idempotency, duplicate prevention |
| `agents.txtar` | adapter inventory, tier derivation, scaffolding status |
| `new.txtar` | plumage/feather creation, ID allocation, title/priority/dependency validation, template instantiation |
| `status.txtar` | lifecycle transitions, legal/illegal paths (e.g. `egg→fledged` requires intermediates), gate enforcement |
| `set.txtar` | frontmatter updates, validation, immutable-field rejection |
| `criteria.txtar` | acceptance-criteria listing, check/uncheck by number or `AC-N` label, idempotency, body preservation |
| `check.txtar` | `preen` validation, error/warning reporting, strict mode, degraded-data handling |
| `lock.txtar` | brood/abandon lifecycle, criteria gates, `--force` override |
| `graph.txtar` | `vee` dependency graph, wave detection, dot format, cycle detection with path reporting |
| `scan.txtar` | module grouping, `.fledgeignore` filtering, byte-accurate summaries |
| `ready.txtar` | pipping-feather detection, brood exclusion, JSON output |
| `report.txtar` | `colony` status summaries, orphan detection, per-plumage fledge counts, active brood listing, degraded-repo behavior |
| `unfledged.txtar` | non-fledged item listing, `--plumage`/`--feathers` scoping, priority-then-ID ordering, JSON shape |
| `e2e.txtar` | full workflow: init → new plumage/feathers → preen → vee → brood → criteria check → abandon `--fledged` → dependent unlock |

Convention: idempotent, human-readable output — second runs of `init` report "exists", not "created". Testscript files mix shell directives (`exec`, `! exec`, `stdout`, `grep`, `mkdir`) with inline test data (`-- path --` blocks).

**Update discipline:** when embedded `core/`/`adapters/` content changes, these fixtures (especially `init.txtar`, `init_agents.txtar`, `agents.txtar`) must be updated alongside, since they assert on the scaffolded output byte-for-byte in places.

## Go unit tests (beside each package, standard `testing.T`)

- `internal/spec/frontmatter_test.go` — `SplitFrontmatter` (leading/closing delimiter, CRLF, body preservation).
- `internal/spec/ids_test.go` — `NextID` (sequential, gap-handling, wide IDs), `Kebab` (lowercasing, Unicode, hyphens).
- `internal/spec/load_test.go` — `Load` with parse errors, unknown-field tracking.
- `internal/spec/criteria_test.go` — `ParseCriteria` (format, checkbox states, section scoping), `SetCriterion`.
- `internal/check/check_test.go` — clean set, parse errors, rule coverage (duplicate-id, dangling-ref, cycle, brood-consistency, criteria).
- `internal/graph/graph_test.go` — `Waves` (topological order), `Cycle` (self-loop, two-cycle), `Ready`.
- `internal/lock/lock_test.go` — `Acquire`/`Release`/`Get`, `HeldError`, contention (16 concurrent acquirers → exactly 1 winner), `LockRollsBackOnStatusWriteFailure` (integration-style).
- `internal/scan/scan_test.go` — module grouping, `.fledgeignore` filtering, byte counts.
- `internal/cli/lock_test.go` — brood + status transactionality (read-only dir forces rollback).
- `internal/bootstrap/registry_test.go` (9 tests, the most extensive suite) — `TestAdapterManifests` (manifest validity, file sources exist in embed FS), `TestPrimitiveCoverage` (tier derivation correctness), `TestCorePrimitivesReferenced` (no dead primitive contracts), `TestCoreNeutral` (core prose never references harness-specific paths), `TestSkillFrontmatter` (Agent-Skills-format validity), `TestWriteCoreClassification` (created/updated/skipped correctness), `TestClaudeSkillSymlinks` (symlink resolution + idempotency + duplicate guard), `TestWriteAdapterRefresh` (default vs overwrite policy behavior on refresh), `TestClaudeAllowListGenerated` (generated allow-list matches `commandOrder`).

Patterns: table-driven tests, `t.TempDir()` for filesystem isolation, `exec.Command("git", "init", ...)` to set up a real git repo where scan/lock tests need one.

## Test-first discipline (spec-level convention, not a test framework)

Every feather's Acceptance Criteria mandate an AC-1-style test-first pattern: write the test, observe it fail against unfixed/unimplemented code, then implement until it passes. Evidence of this is expected in `.fledge/molt/<FTHR-ID>.md` (see `data-model.md` Open Questions — exact generation mechanism for these evidence files is unresolved from source alone).

## Open Questions
- What exactly triggers `preen --strict` to fail — `check.HasErrors` alone, or warnings escalated under `--strict` too?
- Is there a test asserting `commandOrder` and generated per-adapter allow-lists never drift out of sync when a command is added?
