---
generated: 2026-07-15T23:53:12Z
commit: a4d02e8187c64ef9f3f1231052990b282207420b
agent: fledge-forager
fledge_version: 0.5.5
---

# Testing

Test frameworks used, how to run each kind, and what coverage looks like per area.

## Frameworks

- **Go standard `testing` package** — used everywhere; no third-party assertion libraries observed anywhere in the repo.
- **`github.com/rogpeppe/go-internal/testscript`** — drives the CLI's black-box acceptance suite from `.txtar` (plaintext archive) fixtures in `cmd/fledge/testdata/`. `cmd/fledge/main_test.go` registers the `fledge` command via `TestMain` and configures deterministic git identity (fixed `GIT_AUTHOR_NAME`/`EMAIL`, global/system git config disabled) in `TestScripts` to prevent flakiness.
- **`t.TempDir()`** — used throughout unit tests for filesystem isolation; no manual fixture cleanup needed.
- **`httptest`** — mocks GitHub API/HTTP in `internal/cli/update_test.go`.

## How to run

```sh
go test ./...                                   # everything
go vet ./...

go test ./cmd/fledge -run TestScripts            # all 23 CLI acceptance tests
go test ./cmd/fledge -run TestScripts/init       # one script
go test ./cmd/fledge -run TestScripts/init -v    # verbose, shows script trace

go test ./internal/spec -run TestAllocateID      # one unit test in a package
```

## Unit test coverage by package

- **`internal/spec`** (`criteria_test.go`, `frontmatter_test.go`, `ids_test.go`, `load_test.go`): checkbox parsing/mutation (section detection, x/X, CRLF handling, byte-exact single-char toggles), frontmatter split/roundtrip (LF/CRLF, unknown-key detection, title quoting), concurrent ID allocation (20 goroutines racing, flock serialization, width persistence), `Load()` aggregation and per-file error/unknown-field tracking.
- **`internal/check`** (`check_test.go`, ~335 lines): every validation rule — parse errors, duplicate/mismatched IDs, dangling refs, unhatched-plumage references, dependency cycles, required sections, acceptance-criteria completeness + evidence cross-check, brood/lock consistency, legacy-format warnings.
- **`internal/graph`** (`graph_test.go`): topological wave layering, cycle detection (acyclic/self-loop/2-cycle/complex/dangling-not-a-cycle), ready-set filtering by status.
- **`internal/lock`** (`lock_test.go`, ~197 lines): acquire/release/get happy path, `*HeldError` on contention, 16-goroutine concurrent-acquire race (exactly one winner), corrupt-file tolerance in `List()`, 500-cycle atomic-write stress test (no partial/zero-length files, no leftover temp files).
- **`internal/repo`** (`repo_test.go`): minimal path-accessor agreement tests.
- **`internal/scan`** (`scan_test.go`): module grouping, byte counting, `.fledgeignore` filtering, empty-repo edge case.
- **`internal/nest`** (`nest_test.go`): frontmatter key order per doc kind, byte-preserved body rendering, stub detection (byte-equality, doc-name-aware), `RefreshDoc` (drops unknown keys, preserves body, applies agent override), and `Status()` across 4 scenarios — fresh scaffold (all stub → incomplete), all filled + index fresh (complete), filled but stale index (incomplete), missing doc (incomplete + named in `MissingDocs`).
- **`internal/bootstrap`** (`registry_test.go`, `stamp_test.go`, `drift_test.go`, `tmux_autodefault_test.go`, `worker_protocols_test.go`): manifest parsing/primitive-coverage/tier-derivation correctness, core-prose neutrality (`TestCoreNeutral`), write-classification (created/skip/refresh-updates), symlink wiring for Claude skills, stamp round-trip + determinism, 9-scenario drift classification (up-to-date/stale/modified/missing/obsolete × content/symlink/append), plus prose-invariant tests asserting specific wording survives edits to `team-loop.md`, `implementation.md`, `worker-protocols.md` (e.g. `TestSkuaEvidenceGuiltyUntilProven`, `TestSkuaRedTeamPass`).
- **`internal/cli`**: `init_test.go` (yes/no prompt parsing), `version_test.go` (binary/VERSION-file sync), `update_test.go` + `update_swap_test.go` (update flow, archive/checksum handling, binary swap), `lock_test.go` (`TestLockRollsBackOnStatusWriteFailure` — atomicity), `command_parity_test.go` (`commandOrder` list stays in sync with the registered-commands map).

## Structural "keep docs/CI honest" tests

Three packages exist purely to assert that non-Go artifacts stay consistent with what the code actually does — no production logic:
- **`internal/ciconfig`**: parses `.github/workflows/release.yml` and `pr-check.yml` as YAML and asserts on triggers, lint/build/test steps, the 4-platform build matrix, and job dependency ordering.
- **`internal/doctest`**: reads `README.md`/`RELEASING.md` and asserts they mention `fledge update`, `fledge init --refresh`, and `scaffold.json`.
- **`internal/hooktest`**: end-to-end tests of `scripts/hooks/pre-commit` in temporary git repos — blocks unformatted code, blocks vet violations, allows clean commits, no-ops without `core.hooksPath` configured, and asserts the hook's lint commands match CI's.

## CLI acceptance tests (`cmd/fledge/testdata/*.txtar`, 23 total)

Each file exercises one command or behavior area end to end (init scaffolding/idempotence, multi-agent detection/refresh, criteria manipulation, full e2e lifecycle, forager-contract prose hardening, dependency-graph rendering/cycles, feather locking + brood/abandon, nest scaffold/scout/stamp/status, plumage/feather creation + ID allocation, planning-delegation prose markers, scaffold-drift detection, colony status report, `fledge scan`, frontmatter field mutation via `fledge set`, version-mismatch warning behavior, status transitions, unfledged listing). See `entry-points.md`'s command table for the file↔command mapping, and `raw/cmd.md` for the full one-line-per-file breakdown.

## Local pre-commit hook (opt-in)

`scripts/hooks/pre-commit` mirrors the CI lint gate (`gofmt -l .`, `go vet ./...`) before a commit is created. Not installed automatically — one-time setup per clone: `git config core.hooksPath scripts/hooks`. Verified in sync with CI by `internal/hooktest`.

## Open Questions

None observed — testing conventions were consistent and unambiguous across every scout report.
