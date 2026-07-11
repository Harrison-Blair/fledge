---
generated: 2026-07-11T01:58:32Z
commit: 96a3ac38bc843217824d6d6886c49906053bf686
agent: fledge-forager
fledge_version: 0.3.4
---

# Testing

Test frameworks, how to run them, and what each layer covers.

## Frameworks

- **Go's standard `testing` package**, table-driven throughout — no third-party assertion library anywhere in the repo.
- **`github.com/rogpeppe/go-internal/testscript`** — drives CLI acceptance tests from `.txtar` fixture files (script + inline file fixtures), invoked via `cmd/fledge/main_test.go:TestScripts`/`TestMain`.

## How to run

```sh
go test ./...                                  # everything
go test ./cmd/fledge -run TestScripts          # all 21 CLI acceptance tests
go test ./cmd/fledge -run TestScripts/init -v  # one, verbose (shows script trace)
go test ./internal/spec -run TestAllocateID    # one unit test
```

## CLI acceptance tests (`cmd/fledge/testdata/*.txtar`, 21 files)

Each `.txtar` is a self-contained, hermetic test: a bash-like script section (`exec fledge <cmd>`, `stdout`/`stderr` substring assertions, `! exec` for expected failures) followed by inline file fixtures at paths relative to a temp dir. `TestMain` hardcodes git determinism (`GIT_CONFIG_GLOBAL/SYSTEM=/dev/null`, fixed `GIT_AUTHOR_*`/`GIT_COMMITTER_*`) so commit-touching tests are reproducible. Coverage spans every subcommand: `init` (idempotent scaffolding, multi-agent detection), `new`/`set`/`status`/`criteria` (spec mutation + legality), `preen`/`preen_scaffold` (validation + drift), `ready`/`vee`/`graph` (dependency computation, cycles), `brood`/`lock`/`abandon`/`broods` (claim lifecycle), `nest` (doc new/scaffold/scout/stamp), `colony`/`report`/`unfledged` (status reporting), `scan` (module grouping), `agents` (adapter/tier inventory), `e2e` (full init→new→status→preen→vee→brood→abandon→ready lifecycle), `stamp_warning`/`refresh_scaffold` (version drift, `--force`).

## Unit tests by package

- **`internal/spec`**: `criteria_test.go` (8 cases — checkbox parsing, CRLF, byte-level `SetCriterion` idempotency), `frontmatter_test.go` (6 cases — round-trip parse→render→reparse byte-equality, unknown-key detection, CRLF, quoting edge cases), `ids_test.go` (2 cases — `NextID` gap/width handling, `Kebab` unicode/punctuation), `load_test.go` (2 cases — directory loading, missing-dir handling).
- **`internal/bootstrap`**: `drift_test.go` (`TestDriftReport`, 9 table cases across all 5 drift statuses), `registry_test.go` (`TestAdapterManifests`, `TestPrimitiveCoverage`, `TestCoreNeutral`, `TestSkillFrontmatter`, `TestWriteCoreClassification`, `TestClaudeSkillSymlinks`, `TestWriteAdapterRefresh`, `TestClaudeAllowListGenerated`), `stamp_test.go` (`TestEditedOnRefresh`, `TestPruneObsolete`, `TestExpectedFilesCoversAllPolicies`, `TestRenderEntryMatchesWritePath`, `TestStampRoundTrip`, `TestStampDeterministic`).
- **`internal/check`**: 13 cases — parse errors, schema/enum validation, ID-filename agreement, duplicate IDs, dangling refs, cycles, section/criteria completeness, brood consistency, legacy criteria format.
- **`internal/graph`**: 4 cases — `Waves()` multi-layer topological sort, `Cycle()` (acyclic/self-loop/transitive/dangling-as-non-cycle), `Ready()` filtering.
- **`internal/lock`**: 4 cases — acquire/release/get lifecycle, `HeldError` on contention, a 16-goroutine concurrent-acquire race test, sorted `List()`.
- **`internal/nest`**: 5 cases — concern vs. scout frontmatter key order, body preservation through `Render`, stale-key drop + body preservation in `RefreshDoc`, agent override, YAML scalar-quoting safety.
- **`internal/scan`**: 3 cases — normal scan (`.fledgeignore` filtering, module grouping, byte aggregation), missing `.fledgeignore`, empty repo.
- **`internal/cli`**: `init_test.go` (`TestPromptYesNo` — y/Y/yes parsing, default false), `lock_test.go` (`TestLockRollsBackOnStatusWriteFailure` — atomic rollback invariant), `version_test.go` (`TestBinaryVersionMatchesVersionFile` — pins binary version const to repo `VERSION` file).

## Conventions

- Tests are hermetic: `os.TempDir()`/testscript temp dirs, no shared global state, no external service dependencies.
- Test-first is a written discipline for feathers themselves (`pluma/feathers/*.md` describe writing a failing test before implementing) — mirrored in this repo's own `CLAUDE.md` "Test Verification" rules (a new test must be shown to fail against unfixed code before it's trusted).

## Open Questions

None observed.
