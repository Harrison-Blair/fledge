---
generated: 2026-07-16T04:02:08Z
commit: 154510fc963e7071b2f09297ecfeba2b6710e85e
agent: fledge-forager
fledge_version: 0.5.8
---

# Testing

Frameworks, how to run tests, and coverage patterns across the repo.

## Two test layers

1. **Acceptance tests** — `cmd/fledge/testdata/*.txtar`, run via `go test ./cmd/fledge -run TestScripts`. Uses `github.com/rogpeppe/go-internal/testscript`: each `.txtar` file is a text archive encoding an initial filesystem snapshot, `exec`/`!exec` shell-like commands, and `stdout`/`stderr`/`exists` assertions. `cmd/fledge/main_test.go`'s `TestMain` registers the CLI function itself (not a compiled binary) so fixtures run in-process. **Exact fixture count: `ls cmd/fledge/testdata/*.txtar | wc -l` = 25.**
2. **Unit tests** — standard `go test`, live beside their package (`internal/<pkg>/*_test.go`). No external assertion library; table-driven style with `t.Run` subtests is the norm. Concurrency-sensitive packages (`lock`, `roster`, `spec/ids.go`) use goroutine-barrier patterns (e.g. 16–20 goroutines × several rounds) to test allocation/claim races.

## Acceptance fixture map (25 total)

| Fixture | Covers |
|---|---|
| agents.txtar | `fledge agents` — adapter inventory, tier derivation, scaffold status |
| broods_stale.txtar | `fledge broods` — worktree_exists field, `--stale` filter |
| check.txtar | `fledge preen` — dangling deps, missing sections, unchecked criteria |
| criteria.txtar | `fledge criteria` — list/check/uncheck, idempotence, bare-number + AC-label syntax |
| e2e.txtar | Full lifecycle: init→new→status→preen→vee→ready→brood→broods→abandon |
| forager_contract.txtar | Scaffolded planning.md/worker-protocols.md contract text (grep-based) |
| freshness_gate.txtar | planning.md's `fledge nest status --json` freshness gate wiring |
| graph.txtar | `fledge vee` — wave order, `--format dot`, cycle detection |
| init.txtar | `fledge init` scaffolding, idempotence (2nd run byte-identical) |
| init_agents.txtar | `--list-agents`, `--agent`, auto-detect via harness marker dirs |
| lock.txtar | `fledge brood`/`abandon`/`broods` — claim creation, holder name, PID-alive |
| nest.txtar | `fledge nest new` — concern-doc creation, unknown-doc rejection, `--force` |
| nest_status.txtar | `fledge nest scaffold` + `nest status` — completeness gate (all 9 docs past stub, index stamped to HEAD) |
| new.txtar | `fledge new plumage\|feather` — ID allocation, template instantiation, field validation |
| plan_delegation.txtar | Delegation-to-incubator branch text in planning.md/foraging.md |
| preen_scaffold.txtar | Scaffold drift detection — modified/stale/missing/obsolete, `--strict` exit code |
| ready.txtar | `fledge ready` — dependency unlock, brood exclusion |
| refresh_scaffold.txtar | `fledge init --refresh` — reset, prune, TTY confirm, `--force` bypass |
| report.txtar | `fledge colony` — status counts, per-plumage breakdown, orphans |
| roster.txtar | `fledge roster assign/release` — 18-species list, `-2` overflow |
| scan.txtar | `fledge scan` — module grouping, `.fledgeignore` filtering |
| set.txtar | `fledge set` — frontmatter mutation, enum/ref/acyclicity validation |
| stamp_warning.txtar | Version-mismatch warning (scaffold.json version < binary version) |
| status.txtar | `fledge status` — legal lifecycle transitions, `--force` bypass |
| unfledged.txtar | `fledge unfledged` — non-fledged listing, sort order |

## Notable unit-test suites

- `internal/check/check_test.go` — 19 tests covering all 13 preen validation rules.
- `internal/spec/{frontmatter,ids,criteria,load}_test.go` — round-trip parsing/rendering, byte-preservation of spec bodies, concurrent ID allocation (20 goroutines × 5 rounds), CRLF handling.
- `internal/lock/lock_test.go` — 8 tests incl. 16-goroutine contention test, atomic-write-no-partial-files test.
- `internal/roster/roster_test.go` — 5 tests incl. exact 18-species order assertion, concurrent assignment (18 goroutines × 5 rounds).
- `internal/bootstrap/{registry,drift,stamp}_test.go` — scaffold write/refresh/drift classification across all 5 `DriftStatus` states; `TestPrimitiveCoverage` pins derived tiers (`claude:C, codex:A, pi:A`).
- `internal/cli/version_test.go` — `TestBinaryVersionMatchesVersionFile` and `TestStampWarningTxtarVersionMatchesBinary`: the two automated guards on the release-version-consistency convention (see conventions.md).
- `internal/ciconfig/*_test.go`, `internal/doctest/docs_test.go`, `internal/hooktest/precommit_test.go` — meta-tests: assert CI workflow YAML structure, README/RELEASING.md content, and pre-commit hook behavior in real temp git repos, respectively.

## Running tests

```sh
go test ./...                                 # everything
go test ./cmd/fledge -run TestScripts         # all 25 acceptance fixtures
go test ./cmd/fledge -run TestScripts/init    # one fixture, add -v for script trace
go test ./internal/spec -run TestAllocateID   # one unit test
```

## CI enforcement

Both `.github/workflows/pr-check.yml` (every PR to main) and `release.yml`'s safety-net job run `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`. The optional local `scripts/hooks/pre-commit` (opt-in via `git config core.hooksPath scripts/hooks`) mirrors the gofmt+vet gate before a commit is even created.

## Open Questions

None observed beyond conventions.md's note on `ciconfig`'s two-file split.
