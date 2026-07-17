---
generated: 2026-07-17T02:54:09Z
commit: e7a6d4969f861ed3f03af7833b750a7cd703a7a8
agent: fledge-forager
fledge_version: 0.5.8
---

# Conventions

Coding, spec-authoring, and workflow conventions observed across the repo, reconciled across all scouted modules.

## Spec lifecycle & IDs

- Spec files are markdown with a `---`-delimited YAML frontmatter block + byte-preserved body. Frontmatter is **never hand-edited** — always mutated via the CLI (`fledge new`, `fledge set`, `fledge status`, `fledge criteria check`) (`internal/spec/frontmatter.go`).
- IDs (`PLM-###`, `FTHR-###`) are zero-padded (3+ digits, width grows if wider IDs exist), CLI-allocated only, never hand-invented. Allocation scans the target dir for the max existing ID with the prefix, then serializes concurrent allocation via `flock` on a per-directory `.alloc.lock` file (`internal/spec/ids.go:NextID`, `AllocateAndCreate`).
- Filenames follow `<ID>-<kebab-title>.md`, where the title is lowercased and non-alphanumeric runs collapse to single hyphens (`internal/spec/ids.go:Kebab`).
- Status lifecycle: plumages `egg → hatched → fledged`; feathers `egg → pipping → hatching → fledged`. Only these CLI-recognized states exist in frontmatter — see `data-model.md` for the exact constants. Runtime sub-states (e.g. "claimed", "in-review") are orchestrator bookkeeping and are never persisted to frontmatter.
- Acceptance criteria are checkbox lists (`- [ ] AC-N: text` / `- [x] AC-N: text`) under a case-sensitive `## Acceptance Criteria` heading, at column 0. Only ever toggled via `fledge criteria check|uncheck`, never hand-ticked; write always normalizes to lowercase `x` (`internal/spec/criteria.go`).
- `fledge status <ID> fledged` and `fledge abandon --fledged` both refuse to complete if any acceptance-criteria box is unchecked, unless `--force`.

## CLI conventions (`internal/cli`)

- Exit codes are meaningful and shared: `ExitOK`=0, `ExitFail`=1 (domain error), `ExitUsage`=2 (bad flags/args), `ExitEnv`=3 (not a git repo / `.fledge/` missing) (`internal/cli/cli.go`).
- Every command supports `--json`, emitting indented JSON to stdout (empty slices render as `[]`, never `null`, via a `nonEmpty()` helper); `--json` is mutually exclusive with human-readable text output, not supplementary.
- Command registration: each command file calls `register(name, run, usage)` in its own `init()`; `commandOrder` (in `cli.go`) controls both help-text ordering and generated allow-lists (e.g. Claude `settings.local.json`). A test (`command_parity_test.go`) enforces `commandOrder` and the registration map stay in sync.
- `--force` bypasses validation gates (status-transition legality, unchecked-AC refusal, dependency-cycle checks).
- Errors are never masked: `fail()`/`usageErr()`/`envErr()` always print `"fledge: <message>"` to stderr before returning the corresponding exit code.

## Go/build conventions

- No Makefile — `go build ./...`, `go test ./...`, `go vet ./...` run directly.
- `gofmt -l .` and `go vet ./...` are mandatory CI gates (`.github/workflows/pr-check.yml`, `release.yml` safety-net); an optional local pre-commit hook (`scripts/hooks/pre-commit`, opt-in via `git config core.hooksPath scripts/hooks`) mirrors them.
- Buildable only on Unix; the release matrix covers linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 (no Windows).
- `VERSION` (repo-root file) is the single source of truth for the binary version; `internal/cli/version.go`'s `binaryVersion` var is a build-time-injected fallback (`-ldflags` in `release.yml`). A pinned test (`version_test.go`) checks the binary version matches `VERSION`.

## Bootstrap/scaffold conventions (`internal/bootstrap`)

- `core/` (agent-neutral skill prose) and `adapters/<harness>/` (per-harness manifest + files) are the only two embedded trees; adding a new harness means adding a `manifest.yaml` — zero new Go code.
- File write policies, applied per `ManifestFile` entry: `generate`/`primitive_map` (render a `text/template`, always rewritten), `overwrite` (always rewritten verbatim), `append_if_missing` (additive line, never deleted by pruning), `symlink` (always-managed repoint target, e.g. `.claude/skills/fledge-orchestrate` → `.fledge/skills/fledge-orchestrate`), and default/none (copy, **skip-if-exists** so user edits survive; only `fledge init --refresh` re-syncs).
- Tier (A/B/C) is *derived*, never declared: A = `confirm-gate`+`read-only-shell`+`write-file`+`run-fledge`; B = A + `spawn-worker`; C = B + `message-peer` (`internal/bootstrap/primitives.go:DeriveTier`).
- `.fledge/scaffold.json` is the stamp of what fledge owns and at what content hash/symlink-target/append-lines (`internal/bootstrap/stamp.go`); `fledge preen` validates its presence/consistency via `DriftReport`; `fledge init --refresh` is the only mechanism that resyncs fledge-owned files (interactive confirm on edited files unless `--force`).
- Dev install mode (`fledge init --dev=<path>`, PLM-031): core skill docs and default-policy adapter files are written as symlinks into the given fledge source checkout instead of copied; `generate`/`primitive_map`/`overwrite` files remain regular rendered files. `ValidateDevSource` requires an absolute path with a `go.mod` declaring the fledge module.

## Test conventions

- Unit tests live beside their package (`internal/spec/frontmatter_test.go`, `internal/lock/lock_test.go`, etc.); no mocking framework — table-driven, `t.TempDir()`/`t.Chdir()` for isolation.
- Acceptance/CLI tests use testscript/txtar files under `cmd/fledge/testdata/*.txtar`, run via `go test ./cmd/fledge -run TestScripts`; `TestMain` sets deterministic `GIT_*` env vars for reproducible git behavior in sandboxed test repos.
- Concurrency-sensitive packages (`spec` ID allocation, `lock` brood acquisition, `roster` species allocation, `ledger` writes) are tested with concurrent goroutines asserting exclusivity/no-partial-writes.
- Meta-tests (`internal/ciconfig`, `internal/doctest`, `internal/hooktest`) assert CI workflow YAML shape, cross-doc references, and the pre-commit hook's real git behavior stay consistent with source — no production code, tests only.

## Code style & scope discipline (from orchestration prose, applies to agent-driven changes)

- Feathers touch only files their spec's "Affected Modules" section names; no speculative abstraction or unrequested configurability.
- Match existing code style in edits; don't "improve" adjacent code or remove pre-existing dead code unless asked.
- Commit messages are logical units; no `Co-Authored-By` attribution trailers (repo-level `CLAUDE.md` rule, mirrored in orchestration prose).

## Terminology

- Bird-themed throughout: nest, plumage, feather, brood, preen, molt, forager, skua, roster, ledger. See `domain.md` for full definitions; `README.md` is the canonical decoder.

## Open Questions

- `foraging.md` step 5 directs synthesis to "resolve contradictions between reports by re-reading the source file," but no reconciliation algorithm or definition of "contradiction" is specified — left to synthesizer judgment (noted by the bootstrap-core scout).
