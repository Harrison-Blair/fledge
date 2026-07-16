---
generated: 2026-07-16T02:20:48Z
commit: 407b91e70b53764944447dae5829d2076fb852c5
agent: fledge-forager
fledge_version: 0.5.5
---

# Conventions

Naming, error handling, and structural idioms observed across the codebase, reconciled where modules disagree in emphasis.

## Terminology (bird-themed, propagated everywhere)

plumage, feather, nest, brood, preen, molt, forager, skua, scout, brooder, incubator — see `domain.md` for definitions. Match this vocabulary in new code, tests, and prose (`CLAUDE.md`).

## Go code conventions

- **Exit codes are semantic, not just success/fail**: `ExitOK=0`, `ExitFail=1` (domain/validation error), `ExitUsage=2` (CLI misuse), `ExitEnv=3` (not a git repo / missing `.fledge/`) — defined once in `internal/cli/cli.go`, used consistently across every command file.
- **Error messages prefixed `"fledge: "`** via shared helpers `fail()`, `usageErr()`, `envErr()` (`internal/cli/cli.go`); domain errors go to stderr, never stdout.
- **`--json` on every command**: all 18 `internal/cli` commands accept it; `emitJSON()` marshals structured output; JSON field names are snake_case.
- **Command registration pattern**: each command file's `init()` calls `register(name, run, usage)`; `commandOrder` (a single array in `cli.go`) drives both `usage` text and generated adapter allow-lists — kept in sync by `internal/cli/command_parity_test.go:TestCommandOrderMatchesRegistrations`.
- **State machines as maps**: legal status transitions (`taskTransitions`, `reqTransitions` in `internal/cli/status.go`) are hardcoded maps; `--force` bypasses legality checks but never bypasses enum validation.
- **Atomic file writes everywhere spec state changes**: `spec.WriteFileAtomic()` (temp file + rename) for spec files; `lock.Acquire()` uses `os.CreateTemp` + `os.Link` for brood files. No code path partially writes a file that another process could observe mid-write.
- **Byte-preserving mutation**: spec bodies are never re-serialized from a parsed structure — `SetCriterion()` flips exactly one checkbox-state byte; `RefreshDoc()` (nest) preserves the body and only rewrites frontmatter. This is a repo-wide idiom, not local to one package.
- **Fixed frontmatter key order**, validated by tests, differs by artifact kind: requirement (id, title, status, priority, authored, agent, fledge_version), task (id, title, plumage, status, priority, depends_on, [oversight], authored, agent, fledge_version), nest concern doc (generated, commit, agent, fledge_version), nest scout report (module, authored, agent, fledge_version). Optional keys omitted when empty rather than written blank.
- **Concurrency safety via flock/os.Link, not mutexes-in-memory**: ID allocation (`internal/spec/ids.go:AllocateAndCreate`) uses an exclusive `flock` on `.alloc.lock`; brood acquisition uses `os.Link`'s atomicity. Both are exercised by dedicated concurrency tests (20 goroutines / 16 racers).
- **Manifest-driven extension, not code branches**: adding a new agent harness is a new `adapters/<name>/manifest.yaml`, never new Go code (`internal/bootstrap/registry.go` loads all adapters generically from the embedded FS).
- **Idempotent writes**: `writeIfChanged()` (bootstrap) compares bytes before writing and reports `wrote=false` on no-op — this is what makes scaffold refresh and the txtar tests deterministic.

## Testing conventions

- **Unit tests beside their package** (`internal/spec`, `internal/check`, `internal/graph`, `internal/lock`, `internal/scan`, `internal/repo`, `internal/bootstrap`), using `t.TempDir()` for isolation — no external mocks beyond stdlib and `httptest` (for `update.go`'s GitHub-release fetch).
- **Acceptance tests as executable specs**: `cmd/fledge/testdata/*.txtar`, run via `testscript` (`go-internal`), each file a hermetic git-isolated scenario asserting on stdout/stderr/exit code/file existence.
- **"Assert on the docs themselves"** pattern: `internal/ciconfig` parses `.github/workflows/*.yml` as data and asserts on job/step shape; `internal/doctest` asserts README/RELEASING mention specific commands; `internal/bootstrap/worker_protocols_test.go` / `tmux_autodefault_test.go` assert on exact wording survives in the scaffolded skill prose. This repo treats its own documentation and CI config as testable artifacts, not just prose.
- Lint gate (`gofmt -l .`, `go vet ./...`) is identical in CI (`pr-check.yml`) and the optional local pre-commit hook (`scripts/hooks/pre-commit`), and `hooktest` asserts the two stay textually identical.

## Documentation/spec conventions

- IDs (`PLM-###`, `FTHR-###`) and all frontmatter are CLI-allocated — never hand-edited; enforced by workflow routing in `CLAUDE.md` and `SKILL.md`.
- Acceptance criteria are checkbox lists (`- [ ] AC-N: text`), only ever flipped via `fledge criteria check`/`uncheck`.
- Every concern doc in `.fledge/nest/` (this directory) must reference source as `path/to/file.go:Symbol` per `templates/context-doc.md` — carried through in this synthesis.

## Open Questions

- `docs/generalization-plan.md` describes a 7-primitive contract including `spawn-pool`; current shipped code (`internal/bootstrap/primitives.go`) has only 6. Is `spawn-pool` a dropped/deferred primitive, or was it folded into `spawn-worker`? (raised independently by the `docs` and `internal-bootstrap` scouts)
- Whether user-authored adapter manifests (outside the three shipped ones) are a supported/future extension point, or whether `adapters/` is intentionally closed to the three shipped harnesses.
