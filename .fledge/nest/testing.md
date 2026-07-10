---
generated: 2026-07-10T20:53:53Z
commit: f28efebd76d6aa135adb0956a3337a40a8d98351
agent: fledge-forager
fledge_version: 0.3.0
---

# Testing

Test frameworks, how to run them, and coverage patterns across the repo.

## Two test styles

**1. Acceptance tests** — testscript/txtar format under `cmd/fledge/testdata/*.txtar` (20 files), driven by `github.com/rogpeppe/go-internal/testscript`. `cmd/fledge/main_test.go`: `TestMain` registers the `fledge` binary as a testscript command; `TestScripts` runs every fixture with git environment isolation (`GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_SYSTEM=/dev/null`, fixed author/committer identity for determinism). Each fixture is a single file combining setup commands, fledge invocations, and assertions (`stdout`, `stderr`, `grep`, `! exec` for expected failures); fixture repo state (`.fledge/.gitkeep`, `pluma/` dirs) is inlined via txtar `-- <path> --` blocks.

Run: `go test ./cmd/fledge -run TestScripts` (all), `go test ./cmd/fledge -run TestScripts/init` (one), add `-v` for the script trace.

Fixture-by-fixture coverage: `agents.txtar` (adapter inventory/tier/scaffolded status), `check.txtar` (preen: dangling deps, unchecked criteria, missing evidence), `criteria.txtar` (AC listing/check/uncheck, idempotency, body preservation), `e2e.txtar` (full lifecycle init→new→status→preen→vee→brood→abandon→fledged), `graph.txtar` (vee: waves, dot format, cycle detection), `init.txtar` (idempotency, scaffold generation, version stamp), `init_agents.txtar` (--list-agents, auto-detect, --agent override, --refresh/--force, CLAUDE.md/AGENTS.md pointer lines), `lock.txtar` (brood claims, status auto-set, abandon --fledged, AC gate, ready exclusion), `nest.txtar` (nest new/scout/stamp/scaffold, frontmatter, raw/<module>.md schema), `new.txtar` (ID allocation, dependency validation, templates), `preen_scaffold.txtar` (drift detection: modified/missing/obsolete/stale, --strict), `ready.txtar` (pipping computation, JSON), `refresh_scaffold.txtar` (--refresh preserve/prune/force, hash-matching, user-edit retention), `report.txtar` (colony: feather summary, completion, orphans, broods, degraded data), `scan.txtar` (module scan, .fledgeignore filtering), `set.txtar` (field updates, immutable-field rejection), `stamp_warning.txtar` (version mismatch warning, silent on init/version), `status.txtar` (status transitions, illegality checks, AC gate), `unfledged.txtar` (listing, priority-then-ID order, scoping flags).

**Must-update rule**: `init.txtar`, `init_agents.txtar`, `agents.txtar` assert on scaffolded output byte-for-byte — update alongside any `internal/bootstrap/core` or `internal/bootstrap/adapters` change.

**2. Unit tests** — beside their packages, plain `go test` (no testify — `t.Errorf`/`t.Fatal`, table-driven, `t.TempDir()` for isolation):
- `internal/spec/{ids,frontmatter,load,criteria}_test.go` — `NextID`/`Kebab`, `SplitFrontmatter`/parse-render round-trips/quoting, `Load` with errors, `ParseCriteria`/`SetCriterion`/CRLF preservation.
- `internal/check/check_test.go` — `TestCleanSetHasNoFindings`, parse/schema/dangling-ref/cycle/brood-consistency findings.
- `internal/graph/graph_test.go` — `Waves` topological layers, `Cycle` (self-loop, two-cycle), `Ready` filtering.
- `internal/lock/lock_test.go` — `Acquire`/`Release`/`Get`, concurrent contention, `HeldError`, `List` sorting.
- `internal/nest/nest_test.go` — frontmatter key order (Concern/Scout), body preservation on `Render`.
- `internal/scan/scan_test.go` — module grouping, `.fledgeignore` filtering, byte counts.
- `internal/bootstrap/registry_test.go` — `TestAdapterManifests`, `TestPrimitiveCoverage`, `TestCorePrimitivesReferenced`, `TestCoreNeutral`, `TestSkillFrontmatter`, `TestWriteCoreClassification`, `TestClaudeSkillSymlinks`, `TestWriteAdapterRefresh`, `TestClaudeAllowListGenerated`, `TestPreserveDecision`, `TestPruneObsolete`.
- `internal/bootstrap/stamp_test.go` — `TestStampRoundTrip`, `TestStampAbsent`, `TestStampDeterministic`, `TestExpectedFilesCoversAllPolicies`, `TestRenderEntryMatchesWritePath`.
- `internal/bootstrap/drift_test.go` — `TestDriftReport` (9-case table over all `DriftStatus` values), `TestDriftReportNilStampNoObsolete`.
- `internal/cli/version_test.go::TestBinaryVersionMatchesVersionFile` — prevents a stale binary after a `VERSION` bump.
- `internal/cli/lock_test.go::TestLockRollsBackOnStatusWriteFailure` — brood lock rollback on status-write failure (uses `chmod 0o555` to force a write failure — portability to Windows is unverified, see Open Questions).

## Coverage patterns

- Happy path (populated repo, expected shape, `--json` shape, priority/ID ordering) + error paths (usage errors exit 2, environment errors exit 3) + edge cases (empty repo, degraded/unparseable specs, dangling refs, missing flags) + idempotence (second run produces identical bytes — validates `--refresh` determinism).
- Test-first discipline is a first-class repo convention: `pluma/feathers/*.md` explicitly document an observe-FAIL → implement → observe-PASS order per acceptance criterion, with evidence captured under `.fledge/molt/FTHR-###.md`.

## Open Questions

- `TestLockRollsBackOnStatusWriteFailure` forces a write failure via `chmod 0o555` — unclear if this test is skipped or fails on Windows. (internal-cli scout)
</content>
