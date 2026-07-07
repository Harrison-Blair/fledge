---
generated: 2026-07-06T23:33:05Z
commit: b701cf5a12a99b5adf9538e83f51178d4dead0c2
agent: fledge-context-gatherer
fledge_version: 0.1.0
---

# Testing

fledge has two complementary test layers, both run by `go test ./...`.

## End-to-end: testscript / txtar (`cmd/fledge`)
- **Framework:** `github.com/rogpeppe/go-internal/testscript`, wired in `cmd/fledge/main_test.go`. It registers the command name `fledge` to call `cli.Run`, so `.txtar` scripts invoke `fledge <subcommand>` as if it were the real binary.
- **Determinism:** the runner pins `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_SYSTEM` to `/dev/null` and locks `GIT_AUTHOR_*`/`GIT_COMMITTER_*` to `test` / `test@example.invalid`, so git-dependent output is reproducible.
- **Script format:** each `.txtar` in `cmd/fledge/testdata/` declares shell-style commands (`exec`, `!` for expected failure, `grep`, `exists`, `mkdir`, `cp`, `rm`), sets up inline files (`-- path --`), runs in an isolated dir, and asserts stdout/stderr with regex.
- **Coverage (one script per command plus an integration script):**
  - `init.txtar` — scaffold creation, idempotency.
  - `new.txtar` — ID allocation, dependency validation, field defaults, enum validation (e.g. `P9` rejected).
  - `check.txtar` — dangling refs, missing sections, `--strict`.
  - `graph.txtar` — wave layout, DOT export, cycle detection.
  - `ready.txtar` — dependency resolution, lock exclusion, JSON.
  - `lock.txtar` — acquisition/conflict, status auto-update, force unlock, done-task restriction.
  - `status.txtar` — legal transitions, `--force`, unknown ID, body preservation (sentinel body line survives frontmatter rewrite).
  - `set.txtar` — field updates, enum checks, acyclicity, immutable-field rejection.
  - `scan.txtar` — module grouping, .fledgeignore filtering, JSON.
  - `e2e.txtar` — full workflow: init → new → status change → lock/unlock → graph.

## Unit tests (`internal/*`)
Per-package `_test.go` files covering the core algorithms:
- **`internal/spec`** — `frontmatter_test.go` (LF/CRLF split, missing/unterminated delimiters, EOF handling, body-with-`---` survival, task round-trip integrity, optional-oversight rendering, title quoting, atomic write with no leftover temp files); `ids_test.go` (`NextID` sequencing, gaps use max not count, width preservation, missing dir; `Kebab` casing incl. unicode); `load_test.go` (valid load, per-file parse-error aggregation, ID lookup, missing dirs → empty set).
- **`internal/check`** — `check_test.go`, ~15 tests spanning every rule: clean set, parse errors, unknown fields, schema/ID/filename mismatch, duplicate IDs, dangling refs (missing requirement, missing dep, self-reference), unapproved-requirement links, cycles, required sections incl. Tests section, stale-ready hints, lock consistency, `HasErrors`.
- **`internal/graph`** — `graph_test.go`, ~5 tests: wave topological grouping, cycle detection (self-loop, multi-node, dangling-dep-still-acyclic), ready-set filtering (blocked/started excluded, dangling deps block).
- **`internal/lock`** — `lock_test.go`, ~4 tests: acquire/release/get lifecycle, held-error reporting, 16-way concurrent contention with exactly one winner, list on missing dir → empty & sorted.
- **`internal/scan`** — `scan_test.go`, 3 tests: module grouping with file/byte counts, no-.fledgeignore includes all, empty repo → no modules.
- **`internal/cli`** — `lock_test.go`: `TestLockRollsBackOnStatusWriteFailure` verifies the lock is rolled back when the atomic task-status rewrite fails.

## Patterns
- **Test-first discipline is a product convention:** the task template (`internal/spec/templates/task.md`) mandates AC-1 = "tests observed failing before implementation and passing after," so authored tasks carry the same failing-then-passing requirement the codebase itself follows.
- **Round-trip and atomicity are explicitly guarded** (parse→render→reparse equality; no leftover temp files), reflecting the byte-preservation and atomic-write design goals.

## Coverage gaps
- `internal/repo` has **no test file** — repo discovery and path resolution are exercised only indirectly through the txtar e2e scripts.

## Open Questions
- scan behavior on symlinks and non-UTF-8 file content is not directly tested.
- Whether `lock.Record.Branch` is consumed downstream or is purely informational is untested.
