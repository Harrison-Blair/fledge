---
generated: 2026-07-16T21:27:15Z
commit: a1ed62a38540df7ab1cbdc4c486176a64a762018
agent: fledge-forager
fledge_version: 0.5.8
---

# Conventions

Naming, structural, and process idioms observed consistently across the CLI, bootstrap, and orchestration-prose layers.

## Go code conventions

- **Minimal entry-point pattern**: `cmd/fledge/main.go` is 11 lines and delegates everything to `internal/cli.Run` (`cmd/fledge/main.go`).
- **Command registry pattern**: each `internal/cli/*.go` file registers itself via `init()` calling `register(name, run, usage)`; `commandOrder` in `cli.go` controls usage output and generated allow-lists; `command_parity_test.go:TestCommandOrderMatchesRegistrations` pins the two in sync.
- **Exit codes are meaningful and shared**: `ExitOK=0`, `ExitFail=1`, `ExitUsage=2`, `ExitEnv=3` (`internal/cli/cli.go`); errors print to stderr prefixed `"fledge: "`; no panics, no silent failures.
- **`--json` on every command**: marshaled via `emitJSON()` (`internal/cli/cli.go`), 2-space indent, `omitempty` struct tags to avoid null clutter.
- **Shared setup helper**: most commands call `loadSet()` (`internal/cli/specload.go`) first — loads repo root, spec set, and locked feather IDs, returning a `ok bool` handled-exit-code pattern.
- **Byte preservation**: spec bodies after the frontmatter `---` are never re-serialized, only round-tripped (`internal/spec/frontmatter.go:SplitFrontmatter`); `SetCriterion` flips exactly one byte and is idempotent (`internal/spec/criteria.go`); CRLF line endings preserved through parsing/mutation (verified by tests with `\r\n`).
- **Atomic I/O everywhere state matters**: temp-file+rename pattern (`internal/spec/frontmatter.go:WriteFileAtomic`); `os.Link`-based atomic acquire for brood claims (`internal/lock/lock.go`); exclusive `flock` on `.alloc.lock` for ID allocation (`internal/spec/ids.go`) and on `roster.json` for species allocation (`internal/roster/roster.go`) — the same flock pattern reused in both places.
- **Error aggregation over fail-fast**: `spec.Load()` collects per-file parse errors into `Set.Errors` without stopping iteration (`internal/spec/load.go`); errors always returned to caller, never panicked or logged silently (`internal/repo`, `internal/roster`, `internal/scan`).
- **Regex-based ID/criterion parsing**: `^<PREFIX>-(\d+)[-.]` for filenames, `^- \[([ xX])\] (AC-(\d+)):` for checkboxes (no indentation tolerated; lowercase `x` written on check) — `internal/spec/ids.go`, `internal/spec/criteria.go`.
- **Doc/hook assertions by substring, not exact text**: `internal/doctest` and `internal/hooktest` check for section/command presence via substring match so prose edits don't spuriously break tests.

## Scaffold/bootstrap conventions

- **Manifest-driven adapters, zero per-adapter Go code**: all harness-specific behavior is data in `manifest.yaml` (`internal/bootstrap/adapters/*/manifest.yaml`); `registry.go` is the only code that reads them.
- **Tier is derived, never declared**: `Manifest.Tier()` computes A/B/C from which primitives the manifest's `tier_primitives` map covers (`internal/bootstrap/primitives.go`).
- **Write-policy vocabulary is fixed and named** in both code and prose: default (copy, skip-if-exists), `overwrite`, `generate`/`primitive_map`, `append_if_missing`, `symlink` (`internal/bootstrap/registry.go`, restated in root `CLAUDE.md`).
- **Determinism in generated state**: `.fledge/scaffold.json` marshaled with 2-space indent and sorted keys (`internal/bootstrap/stamp.go`); repo-relative paths use forward slashes internally, converted to host OS only at write time.
- **Core prose stays harness-neutral**: `internal/bootstrap/registry_test.go:TestCoreNeutral` asserts core skill prose never references harness-native paths (`.claude/`, `.pi/`, `.codex/`); harness specifics live only in adapters.
- **Refresh is a reset, not a merge**: `fledge init --refresh` overwrites every fledge-owned file to the shipped version and prunes obsolete ones; user edits are protected by a confirm-gate on an interactive terminal (skippable with `--force`), recoverable only via git (root `CLAUDE.md`, `RELEASING.md`).

## Spec-lifecycle conventions (plumage/feather)

- **Lifecycle**: plumage `egg → hatched → fledged`; feather `egg → pipping → hatching → fledged` (consistent across `internal/spec/types.go`, `internal/check/check.go`, root `CLAUDE.md`, and all orchestration prose).
- **Frontmatter is CLI-owned**: `id`, `authored`, `agent`, `fledge_version` are allocated/written only by the CLI (`fledge new`); agents never hand-edit frontmatter or invent IDs (root `CLAUDE.md`, `internal/spec/ids.go`).
- **Acceptance criteria only change via `fledge criteria check|uncheck`**: checkbox mutation is byte-preserving and idempotent (`internal/spec/criteria.go:SetCriterion`); hand-editing the checkbox text is out of convention.
- **AC-1 is always test-first evidence** on feathers (`internal/bootstrap/core/skills/fledge-orchestrate/templates/feather.md`, `brooder.md`): tests written and run against unchanged code first, failure captured verbatim in `.fledge/molt/FTHR-###.md`, then implemented until passing — never weakened to pass.
- **Evidence file format**: `.fledge/molt/<TaskID>.md`, one `## AC-N` heading per criterion (bare heading, not further labeled) with commands + verbatim output (`internal/check/check.go` criteria-evidence rule; `worker-protocols.md`).
- **No `Co-Authored-By` trailers** on any commit made by brooders/skuas (`brooder.md`, `skua.md`, and the user's own global instruction).
- **Brood consistency is bidirectional**: `hatching` status must have a held lock, and a held lock implies `hatching` status — checked both ways by `preen` (`internal/check/check.go`).

## Worker/orchestration prose conventions

- **Worker naming**: `<role>-<species>` (e.g. `fledge-brooder-adelie`); species drawn from the canonical 18-item penguin list in `internal/roster/roster.go`, allocated via `fledge roster assign`, numeric-suffixed once the base set is exhausted. Scouts are the one exception — unnamed, self-terminating.
- **Spawn prompt is the entire context**: workers inherit no conversation history; every spawn prompt must be fully self-contained (stated identically in `foraging.md`, `worker-protocols.md`, `implementation.md`).
- **One-shot lifecycle, explicit final message**: workers are done only at their named final message, never at bare idle; the commissioning party requests shutdown by name and force-terminates if it doesn't exit promptly.
- **Fixed communication topology**: brooder ↔ skua ↔ orchestrator; incubator ↔ orchestrator; forager → commissioner only. No worker nesting (workers don't spawn further workers) except forager→scout and orchestrator→everything.
- **Density over prose**: scout/concern-doc conventions and worker protocols alike mandate bullets, file references (`path/to/file.go:Symbol`), and identifiers — "no prose padding" is stated verbatim in `foraging.md` and `templates/context-doc.md`.

## Open Questions

- Exact traceability threshold for skua's "changes not traceable to spec" scope-creep check is undefined (line-by-line vs logical correspondence) — `skua.md` (via `internal-bootstrap-core` scout).
- Commit-message *content* convention beyond "no trailers" is unspecified — `brooder.md`/`skua.md` (via `internal-bootstrap-core` scout).
