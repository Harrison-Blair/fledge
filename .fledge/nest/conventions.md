---
generated: 2026-07-17T07:00:54Z
commit: ee49464adb830bef7189f94a1d3253927d33fb5f
agent: fledge-forager
fledge_version: 0.6.7
---

# Conventions

Coding, naming, and process conventions observed across the codebase, reconciled across modules.

## CLI command pattern

Every one of the 25 registered commands (`internal/cli/cli.go`) follows the same shape: `flag.FlagSet(ContinueOnError)` parsing → `repo.Find()` + `r.RequireFledge()` → `loadSet()` (repo + specs + locked task IDs) → business logic → error classification (`envErr()` → `ExitEnv`, `fail()` → `ExitFail`, `usageErr()` → `ExitUsage`) → `emitJSON()` for `--json` or human text otherwise. Every command supports `--json`; JSON output omits zero-value fields via `omitempty`.

## Exit codes

Shared, meaningful, process-level: `ExitOK=0`, `ExitFail=1`, `ExitUsage=2`, `ExitEnv=3`, `ExitTimeout=4` (`internal/cli/cli.go`) — `ExitTimeout` is new (added for `fledge await`) and not yet reflected in every older description of the exit-code set; treat any doc that lists only 0–3 as stale.

## Spec (plumage/feather) conventions

- IDs (`PLM-###`, `FTHR-###`) and frontmatter are always CLI-allocated (`internal/spec/ids.go` `NextID`/`AllocateAndCreate`, flock-serialized via `.alloc.lock`) — never hand-written.
- Frontmatter has a fixed key order per type (`internal/spec/frontmatter.go`): id, title, [plumage for tasks], status, priority, depends_on [tasks only], oversight [optional, omitted when empty], authored, agent, fledge_version.
- Spec bodies are byte-preserved — never re-serialized (`internal/spec/frontmatter.go`, comment: "opaque markdown body that is preserved byte-for-byte"). Prose is the only part an agent hand-edits.
- Acceptance criteria are checkbox lists (`- [ ] AC-N: ...`) parsed by a start-of-line regex under a bare `## Acceptance Criteria` heading (`internal/spec/criteria.go`); checked only via `fledge criteria check`, which flips a single byte in place (`SetCriterion`).
- Lifecycle: Plumage = egg → hatched → fledged; Feather = egg → pipping → hatching → fledged (`internal/spec/types.go`). `status:` in frontmatter is an authoring hint only — actual dispatchability is recomputed by `fledge ready`/`fledge vee`, never read directly off frontmatter.
- `fledge set` replaces field values wholesale (e.g. `depends_on`) — it does not append; callers must always pass the full list.

## Bootstrap/scaffold conventions

- **Manifest-driven adapters**: every per-harness difference lives in that harness's `manifest.yaml` (detector, `tier_primitives`, file list + write policy) — adding or changing a harness is a manifest edit, never Go code (`internal/bootstrap/registry.go` `LoadAdapters`).
- **Tier is derived, never declared**: `DeriveTier` (`internal/bootstrap/primitives.go`) computes A/B/C from an adapter's declared primitive coverage.
- **File write policies** (`internal/bootstrap/registry.go` `ManifestFile`): `generate`/`primitive_map` (render a `text/template`), `overwrite` (copy verbatim, always managed), `append_if_missing` (additive line, never clobbers), `symlink` (managed always, created/repointed), and the default (copy, skip-if-exists — user edits survive until an explicit `--refresh`).
- **`writeIfChanged`** makes every scaffold write byte-idempotent — the txtar tests (`init.txtar`, `refresh_scaffold.txtar`) depend on this.
- **Drift has 5 statuses** (`internal/bootstrap/drift.go`): up-to-date, stale (binary moved), modified (user edited), missing, obsolete (no longer shipped) — dev-linked files compare symlink target to the stamp's recorded target, not content.
- **`fledge init --refresh`** resets every fledge-owned file to the shipped version and prunes obsolete ones; it is a reset, not a merge — confirms before clobbering user edits on an interactive terminal, refuses otherwise (`--force` skips the confirmation).

## Ledger conventions (PLM-030)

- Atomic writes only: temp-file-then-`os.Rename` (`internal/ledger/ledger.go`) — no partial files ever observed even under 16-way concurrent writes (`ledger_test.go`).
- Latest-value-only semantics: concurrent writes to the same (subject, kind) are idempotent — last writer wins, no history retained.
- Subject validation rejects path separators, `..`, `.`, and empty strings outright (`InvalidSubjectError`) — never sanitizes; same defense-in-depth pattern used in `internal/cli/ledger.go`'s `ledger read --kind` enum validation (path-traversal guard tested in `ledger-read.txtar`).
- Kind-dependent wait contract in `fledge await` (`internal/cli/await.go`): change-wait (default, for the repeatedly-written `status` kind) vs. existence-wait (`--exists`, for the write-once `verdict`/`escalation` kinds); `--timeout` is mandatory on both paths (`ExitUsage` if omitted); `ExitTimeout` on elapse.

## Concurrency-safety pattern

A recurring idiom across independently-developed packages: exclusive `flock`/`os.Link` for claim-style state (`internal/spec/ids.go` `.alloc.lock`, `internal/lock/lock.go` `.brood` files via `os.Link` O_EXCL, `internal/roster/roster.go` `.roster.lock`), vs. atomic rename for record-style state (`internal/ledger/ledger.go`, `internal/spec/frontmatter.go` `WriteFileAtomic`). Corruption is tolerated, never panicked on: `lock.List` skips unparseable `.brood` files and reports which were skipped; `nest.Status` treats malformed doc files as stubs.

## Naming and terminology

- **Bird terminology throughout**: plumage, feather, nest, brood, preen, molt, forager, skua, roster, colony (see `domain.md` for the full glossary) — match it in new code and prose.
- **Hyphenated validation rule names** in `internal/check/check.go` (e.g. `duplicate-id`, `dangling-ref`, `unhatched-plumage`, `stale-pipping-hint`, `brood-consistency`, `criteria-evidence`).
- **Test helper naming**: lowercase helpers (`req`, `task`, `newSet`, `initRepo`, `write`, `run`, `commit`) with `*testing.T` always first param — consistent across `internal/check`, `internal/graph`, `internal/scan` tests.

## Test-fixture conventions

CLI acceptance tests are testscript/txtar files under `cmd/fledge/testdata/`: an opening comment block states the test's purpose and the feather/plumage it traces to, then `exec fledge <cmd>` interleaved with `stdout`/`stderr`/`grep`/`exists`/`!` assertions, with fixture files defined in `-- <path> --` sections. Exit-code precision uses `sh -c 'cmd; test $? -eq N'` where a bare `!` (expect-nonzero) isn't specific enough (e.g. distinguishing `ExitUsage` from `ExitTimeout`).

## Documentation self-consistency

Test-only packages `internal/ciconfig`, `internal/doctest` pin the exact shape of non-Go artifacts against what the tests assert: `.github/workflows/*.yml` structure, and specific `README.md`/`RELEASING.md`/`CLAUDE.md` sections/mentions. When editing those docs or workflows, expect these tests to catch drift.

## Open Questions

None — conventions were consistent across all nine scout reports; no contradictions required re-reading source to resolve beyond the nest-templates question tracked in `architecture.md`.
