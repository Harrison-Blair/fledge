---
generated: 2026-07-15T18:14:39Z
commit: 5728c29953a7c218c923ce20333dbffebb00623f
agent: fledge-forager
fledge_version: 0.5.4
---

# Testing

Frameworks, how to run tests, and coverage patterns across the repo.

## Frameworks

- **Go `testing`** — standard unit tests, co-located beside every package (`internal/*/*_test.go`). No mocking framework; table-driven tests are the norm, `t.TempDir()` for filesystem isolation.
- **`testscript`** (`github.com/rogpeppe/go-internal/testscript`) — CLI acceptance tests. `cmd/fledge/main_test.go` registers the `fledge` command and runs every `cmd/fledge/testdata/*.txtar` file via `TestScripts(t *testing.T)`. Git identity/config (`GIT_AUTHOR_*`, `GIT_COMMITTER_*`, global config) is isolated per test for determinism.

## How to run

```sh
go test ./...                                   # everything
go test ./cmd/fledge -run TestScripts           # all 20 txtar acceptance scripts
go test ./cmd/fledge -run TestScripts/init      # one script by name (no .txtar suffix)
go test ./cmd/fledge -run TestScripts/init -v   # verbose, shows the script trace
go test ./internal/spec -run TestAllocateID     # a single unit test
go vet ./...
```

## Acceptance test coverage (`cmd/fledge/testdata/*.txtar`, 20 files)

Each `.txtar` embeds a literal command sequence plus fixture files and asserts on stdout/stderr (regex/literal), file existence, exit codes, and JSON structure. One file per command/workflow area: `init`, `init_agents` (multi-agent, auto-detect, `--refresh` confirm/force), `agents` (tier derivation, scaffold status), `new` (ID allocation, templates), `status` (state machine), `set` (frontmatter mutation, immutability guards), `check` (validation rules), `criteria` (checkbox ops), `preen_scaffold` (drift detection, `--strict`), `refresh_scaffold` (reset/prune/confirm), `graph` (waves, DOT, cycles), `lock` (brood/abandon, PID liveness, force bypass), `ready`, `nest` (concern docs, scout reports, stamp), `scan`, `report` (colony), `e2e` (full lifecycle), `unfledged`, `plan_delegation` (planning.md delegation markers), `stamp_warning` (version-mismatch warnings).

**These fixtures are the authoritative behavioral spec** — any change to `internal/bootstrap/core/` or `adapters/` content must update `init.txtar`, `init_agents.txtar`, `agents.txtar` alongside it (CLAUDE.md, confirmed across every scout).

## Unit test coverage by package

- **`internal/bootstrap`**: `registry_test.go` (manifest parsing, primitive coverage, `TestCoreNeutral` — core prose never references harness-native paths, `TestClaudeSkillSymlinks`, `TestWriteAdapterRefresh`, `TestClaudeAllowListGenerated`, `TestEditedOnRefresh`, `TestPruneObsolete`); `stamp_test.go` (round-trip, determinism, `TestExpectedFilesCoversAllPolicies`); `drift_test.go` (table-driven over all 5 `DriftStatus` values, content + symlink + append entries).
- **`internal/spec`**: `frontmatter_test.go` (CRLF, unterminated frontmatter, unknown keys), `ids_test.go` (gap-aware `NextID`, **20-goroutine × 5-round concurrent allocation race test**), `criteria_test.go` (single-byte checkbox toggle, idempotence), `load_test.go`.
- **`internal/check`**: schema rules, dangling refs, duplicate IDs, criteria completeness, evidence-file presence, dependency cycles, brood-consistency, stale-pipping hints.
- **`internal/graph`**: cycle detection (acyclic/self-loop/two-cycle/complex-chain/dangling-not-a-cycle), waves, ready-set.
- **`internal/lock`**: acquire/release/get lifecycle, `HeldError` on contention, **N-goroutine concurrent contention (exactly one winner)**, corrupt-file-skipping `List()`.
- **`internal/nest`**: frontmatter key-order-by-kind, body preservation across `Render`, `RefreshDoc` idempotence.
- **`internal/cli`**: `command_parity_test.go` (`commandOrder` ≡ `commands` map — catches silently-dropped commands from usage), `lock_test.go` (brood rollback on status-write failure), `update_test.go`/`update_swap_test.go` (self-update against a mocked GitHub API test server, archive/checksum swap mechanics), `version_test.go` (binaryVersion pinned to `VERSION` file).
- **`internal/ciconfig`**: asserts CI workflow YAML structure (`.github/workflows/*.yml` triggers, job/step presence) — code-free, structural tests only.
- **`internal/doctest`**: asserts root docs (README.md Commands/Upgrading sections, RELEASING.md scaffold-refresh coverage) stay in sync with actual behavior.
- **`internal/hooktest`**: drives `scripts/hooks/pre-commit` against a real temp git repo, verifies gofmt/vet enforcement and exit codes.

## CI enforcement

- `pr-check.yml` (every PR to main): `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...` on `ubuntu-latest`.
- `release.yml` (push to main): same safety-net job, plus VERSION-diff detection and conditional 4-platform build/release.
- Local pre-commit hook (`scripts/hooks/pre-commit`) mirrors the lint gate; opt-in via `git config core.hooksPath scripts/hooks`, never auto-installed.

## Notable patterns worth reusing

- **Test-first is enforced in the workflow itself**, not just in fledge's own tests: `worker-protocols.md`'s Skua review checklist requires the brooder's AC-1 evidence to be a captured *failing* test run before implementation — this is a project-wide discipline pattern, not just a Go-testing convention.
- Graceful degradation tests: `colony`/`unfledged` commands are tested to exit 0 even on spec parse errors, surfacing problems under an "Issues"/degraded-data section rather than failing hard.

## Open Questions

None observed.
