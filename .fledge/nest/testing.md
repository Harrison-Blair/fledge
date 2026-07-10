---
generated: 2026-07-10T14:50:00Z
commit: 7678344ab9a18730530b9f6edf507ad0c449d352
agent: fledge-forager
fledge_version: 0.2.1
---

# Testing

Test frameworks, how to run them, and what each layer's suite covers — unit tests beside their packages, plus a comprehensive CLI-level acceptance suite.

## Frameworks

- **Standard `testing` package** for all unit tests (`internal/*/**_test.go`).
- **`testscript`** (from `github.com/rogpeppe/go-internal`) for CLI acceptance tests — drives `.txtar` fixture files that combine setup commands, `fledge` CLI invocations, and assertions in one file (`cmd/fledge/main_test.go` wires `TestMain`/`TestScripts`).

## Running tests

```sh
go test ./...                                  # everything
go test ./cmd/fledge -run TestScripts           # all txtar acceptance tests
go test ./cmd/fledge -run TestScripts/init -v   # one script, verbose trace
go test ./internal/spec -run TestAllocateID     # one unit test
```

## Unit test coverage by package (`internal/*/**_test.go`)

- **`internal/spec`**: `frontmatter_test.go` (CRLF handling, round-trip, atomic-write cleanup), `ids_test.go` (`NextID` gaps/wide-IDs, `Kebab` unicode), `load_test.go` (parse-error accumulation, missing dirs), `criteria_test.go` (checkbox parsing incl. CRLF/indentation rejection, mutation idempotence).
- **`internal/check`**: `check_test.go` — every validation rule (parse, unknown-field, duplicate-id, dangling-ref, cycle, unhatched, schema, id-filename, tests-section, stale-pipping-hint).
- **`internal/graph`**: `graph_test.go` — `Waves` (topo order + cycle), `Cycle` (acyclic/self-loop/two-cycle/dangling), `Ready` (satisfied-deps computation).
- **`internal/lock`**: `lock_test.go` — acquire/release/get, `HeldError`, sorted `List`, concurrent-contention (exactly one winner).
- **`internal/nest`**: `nest_test.go` — frontmatter key order per doc kind, body preservation on refresh, `RefreshDoc` dropping unknown keys.
- **`internal/scan`**: `scan_test.go` — module grouping, `.fledgeignore` filtering, empty-repo/no-ignore fallback.
- **`internal/bootstrap`**: `registry_test.go` — 9 tests: manifest parsing, primitive-coverage validation, core-prose neutrality (no harness-native paths), skill frontmatter validity, `WriteCore` idempotence/refresh classification, Claude symlink behavior, adapter refresh (default vs. overwrite policy), Claude allow-list generation from `commandOrder` (Q23).
- **`internal/cli`**: `lock_test.go` (`TestLockRollsBackOnStatusWriteFailure` — brood rollback on status-write failure), `version_test.go` (`TestBinaryVersionMatchesVersionFile` — pins `binaryVersion` to the `VERSION` file).

## CLI acceptance suite (`cmd/fledge/testdata/*.txtar`, 18 files)

Deterministic git env (author/committer name+email, disabled global/system config) set up per-test in `main_test.go`. Assertion primitives: `exec`/`! exec`, `stdout`/`stderr` (regex), `grep`/`! grep`, `exists`/`! exists`, `mkdir`/`cp`.

- **Scaffolding**: `init.txtar` (idempotent setup, default adapter, core skills), `init_agents.txtar` (multi-agent detect/select/refresh), `agents.txtar` (adapter inventory + derived tier).
- **Spec lifecycle**: `new.txtar` (ID allocation, templates), `status.txtar` (legal transitions, `--force`, AC gating), `criteria.txtar` (list/check/uncheck on both spec types), `set.txtar` (frontmatter field updates, immutability).
- **Validation & discovery**: `check.txtar` (preen: dangling deps, missing sections, unchecked criteria, evidence), `scan.txtar` (module grouping/filtering/counts).
- **Dependency graph & readiness**: `graph.txtar` (vee: waves, `--format dot`, cycles), `ready.txtar` (pipping recompute, brood exclusion).
- **Claiming & completion**: `lock.txtar` (brood/abandon/broods: claim, auto-status, criteria gating, `--force`, release).
- **Reporting**: `report.txtar` (colony: counts, orphans, brood list, degraded data), `unfledged.txtar` (scoping flags, priority-then-ID order).
- **Nest operations**: `nest.txtar` (new/scaffold/scout/stamp: doc creation, frontmatter refresh/preservation) — this is the same command surface this forager/scout run just exercised.
- **End-to-end**: `e2e.txtar` (full lifecycle: init → new → status → vee → ready → brood → abandon → criteria check → evidence → fledged).

## Testing strategy for the bootstrap layer specifically

Per `docs/generalization-plan.md` §8 (design intent — cross-check `internal/bootstrap/registry_test.go` for what's actually implemented): assert core skill written on every `init`; assert each adapter scaffolds only to its native path; assert M0 behavioral identity (files present, exit codes, JSON shape); adapter smoke checks (no network) verifying `tier_primitives` keys cover every primitive the core prose references; skill-frontmatter validity (`name` ≤64 chars lowercase-hyphens, `description` ≤1024 chars); regression guard that pre-existing command txtars pass unchanged when bootstrap content changes.

## Byte-idempotence — why it matters for these tests

`writeIfChanged` (bootstrap) and `spec.WriteFileAtomic` make writes byte-idempotent; `init.txtar`/`init_agents.txtar`/`agents.txtar`/`nest.txtar` all depend on re-running the same operation producing zero diffs (no spurious "updated" entries) to pass reliably.
