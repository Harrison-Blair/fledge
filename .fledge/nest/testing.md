---
generated: 2026-07-16T02:20:48Z
commit: 407b91e70b53764944447dae5829d2076fb852c5
agent: fledge-forager
fledge_version: 0.5.5
---

# Testing

Frameworks, how to run tests, and what each layer of the test suite actually covers.

## Frameworks

- **Standard `testing` package** for all unit tests, colocated beside their package (`internal/spec/*_test.go`, `internal/check/check_test.go`, `internal/graph/graph_test.go`, `internal/lock/lock_test.go`, `internal/bootstrap/*_test.go`, `internal/cli/*_test.go`, etc.). Use `t.TempDir()` for filesystem isolation; no external mocking library beyond stdlib `net/http/httptest` (only `internal/cli/update_test.go` needs it, for the GitHub-release fetch).
- **`testscript`** (`github.com/rogpeppe/go-internal/testscript`) for CLI acceptance tests: `cmd/fledge/main_test.go:TestMain`/`TestScripts` run every `cmd/fledge/testdata/*.txtar` file. Each txtar is a self-contained scenario: file markers (`-- path --`), `exec`/`! exec` command assertions, `stdout`/`stderr` regex checks, `exists`/`! exists`/`grep`/`! grep` file predicates. Every test gets a fresh `git init -q .` with deterministic identity (`GIT_AUTHOR_NAME=test`, `test@example.invalid`) for hermeticity.

## How to run

```sh
go test ./...                                  # everything
go test ./cmd/fledge -run TestScripts          # all 23 txtar acceptance tests
go test ./cmd/fledge -run TestScripts/init     # one script (init.txtar), add -v for script trace
go test ./internal/spec -run TestAllocateID    # one unit test
go vet ./...
gofmt -l .                                     # formatting check (CI + optional pre-commit gate)
```

## What's covered, by layer

**Acceptance (`cmd/fledge/testdata/*.txtar`, 23 files, ~2300 lines)** — one file per command or cross-command scenario: `init` (idempotency, scaffolding), `new` (ID allocation/templates), `status` (lifecycle transitions), `check`/`preen_scaffold` (validation + drift classification), `lock` (brood/abandon/broods, stale-PID detection), `nest`/`nest_status` (concern-doc scaffolding + completeness gate — directly exercises the pipeline this forager runs), `graph` (vee waves/cycles/dot output), `criteria` (checkbox mutation), `ready` (pipping computation), `set` (frontmatter mutation + cycle detection), `scan` (module grouping/`.fledgeignore`), `report` (colony counts), `agents` (adapter inventory/tiers), `e2e` (full init→fledged lifecycle chain), `forager_contract` (prose-leak guard: planning.md/worker-protocols.md must not leak internal pipeline vocabulary), `init_agents` (multi-agent flags), `unfledged`, `stamp_warning` (version-mismatch warning fires from subdirectories, not on init/version), `refresh_scaffold` (prune/confirm/force/recreate).

**Unit — spec (`internal/spec`)**: frontmatter round-trip byte-identity, CRLF/LF tolerance, unknown-key detection, concurrent ID allocation (20 goroutines × 5 rounds racing on the same `.alloc.lock`), Unicode-aware kebab slugging, checkbox parse/mutate idempotency.

**Unit — check (`internal/check`)**: ~20 rules individually tested (schema, duplicate-id, dangling-ref ×3 shapes, cycle, criteria-incomplete, criteria-evidence, lock-consistency, stale-pipping-hint, etc.).

**Unit — graph (`internal/graph`)**: 3-wave topological sort, cycle detection (5 scenarios including dangling-dep-is-not-a-cycle), ready-set computation.

**Unit — lock (`internal/lock`)**: acquire/release/get, held-conflict error, 16-way concurrent-acquire contention (exactly one winner), atomic-write verification (500 iterations, no partial file ever observed by a racing reader), corrupt-brood-file skip-and-continue.

**Unit — nest (`internal/nest`)**: fixed frontmatter key order for both doc kinds, body-preservation round-trip, stub detection (`IsStub`), and `Status()` (the exact logic behind `fledge nest status`'s complete/incomplete verdict this forager run is gated on).

**Unit — bootstrap (`internal/bootstrap`)**: every adapter manifest parses and its file sources exist; primitive-coverage matches expected tier per adapter (claude=C, codex=A, pi=A); core prose is harness-neutral (`TestCoreNeutral` — no `.claude/`/`.pi/`/`.codex/`/`.cursor/` mentions) and references every primitive (no dead contract); write-policy classification (fresh/refresh/no-refresh/edited) for both `WriteCore` and `WriteAdapter`; drift classification across all 5 states; skill-symlink idempotency and duplicate-skill guard; prose-invariant tests pinning exact wording in `team-loop.md`/`implementation.md`/skua protocol text.

**Unit — cli (`internal/cli`)**: `commandOrder` ↔ registered-commands parity (prevents silent usage/allow-list omission), binary-version-matches-VERSION-file pin, lock-rollback-on-write-failure, GitHub-release update flow (mocked HTTP, tar.gz build/extract, checksum match, atomic swap).

**"Docs/CI-as-tests"** (`internal/ciconfig`, `internal/doctest`, `internal/hooktest`) — these packages hold no runtime code, only tests that parse `.github/workflows/*.yml` and assert on job/trigger/matrix shape, assert README/RELEASING mention specific commands, and run the actual `scripts/hooks/pre-commit` script end-to-end against real temp git repos (blocks unformatted/vet-failing commits, no-ops when `core.hooksPath` isn't configured).

## Determinism guarantees

Git environment isolation in every txtar test; byte-identical marshaling for `.fledge/scaffold.json` (sorted JSON keys); `writeIfChanged()` byte-comparison before any scaffold write — together these make repeated `fledge init --refresh` runs and the acceptance suite reproducible across machines.
