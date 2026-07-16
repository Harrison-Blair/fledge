---
generated: 2026-07-16T04:02:08Z
commit: 154510fc963e7071b2f09297ecfeba2b6710e85e
agent: fledge-forager
fledge_version: 0.5.8
---

# Conventions

Naming, error-handling, layering, and process conventions observed across the repo, reconciled across all scouted modules.

## Terminology & naming

- **Bird-themed vocabulary** throughout: plumage, feather, nest, brood, preen, molt, forager, scout, brooder, skua, incubator, roster, colony, vee. Match this in new prose (README.md decodes it).
- **IDs**: `PLM-###` (plumage/requirement), `FTHR-###` (feather/task) — 3-digit zero-padded, widened only if existing IDs are wider (`internal/spec/ids.go`). Never hand-invented; always CLI-allocated (`fledge new`).
- **Kebab-case** for derived slugs (`internal/spec.Kebab()`): lowercase, non-alphanumeric runs collapse to one hyphen, Unicode letters/digits preserved.
- Worker names follow `fledge-<role>-<species>` (e.g. `fledge-brooder-adelie`), drawn from an 18-species bird list with `-2`/`-3` overflow suffixes (`internal/roster`).

## CLI architecture conventions

- Command dispatch: each command file registers via `register(name, run, usage)` in its own `init()`; `internal/cli/cli.go`'s `commandOrder` slice (19 entries, exact order verified via `awk '/commandOrder = /,/^}/' internal/cli/cli.go`) drives both help output and generated allow-lists.
- Exit codes are a shared, meaningful enum: `ExitOK=0`, `ExitFail=1` (domain error), `ExitUsage=2` (bad args), `ExitEnv=3` (not a git repo / missing `.fledge/`).
- Every command supports `--json` (indented, deterministic key order via struct tags).
- Each command uses its own `flag.FlagSet` with `flag.ContinueOnError` (never exits on parse failure directly); `parseMixed()` collects positionals before flags for commands with flexible arg order.
- Common `loadSet()` (`internal/cli/specload.go`) loads repo + all specs before most command bodies run.

## Spec-file conventions

- Frontmatter is YAML between `---` delimiters, snake_case keys, fixed rendering order (Requirement: `id, title, status, priority, authored, agent, fledge_version`; Task adds `plumage, depends_on, oversight`). Body markdown is preserved byte-for-byte — never reparsed/reformatted by CLI mutations.
- Acceptance criteria: `- [x] AC-N: text` lines under a bare `## Acceptance Criteria` heading; only ever mutated via `fledge criteria check` (never hand-edited); one byte flipped per mutation, preserving all other bytes.
- Lifecycle states — **Plumage**: `egg → hatched → fledged`. **Feather**: `egg → pipping → hatching → fledged` (verified directly against `internal/spec/types.go` status constants and `internal/cli/status.go` transition rules: task transitions `egg→hatching`, `pipping→hatching`, `hatching→{fledged,pipping}`; requirement transitions `egg→hatched`, `hatched→{fledged,egg}`; `--force` bypasses legality but not enum validity).
- Deterministic spec operations always go through the CLI (`fledge new`, `status`, `set`, `criteria`, `brood`) — frontmatter is never hand-edited; spec *body* prose is agent-writable.

## File write / scaffolding conventions

- `fledge init` write policies (`internal/bootstrap/registry.go`): `generate`/`primitive_map` (template-rendered, always rewritten), `overwrite` (verbatim, always rewritten), `append_if_missing` (additive one-liner, never deletes), `symlink` (OS-level link, e.g. `.claude/skills/*` → `.fledge/skills/*`), and the default (copy, **skip-if-exists** so user edits survive; `--refresh` re-syncs).
- `.fledge/scaffold.json` is the deterministic stamp (`json.MarshalIndent`, alphabetic key sort, trailing newline) of what fledge owns and at what content hash; `fledge preen` and `fledge init --refresh` both consult it for drift.
- `writeIfChanged` makes writes byte-idempotent — a second identical run reports files as skipped, not rewritten. The `cmd/fledge` txtar tests (especially `init.txtar`, `init_agents.txtar`, `agents.txtar`) assert on this; update those fixtures whenever embedded `core/`/`adapters/` content changes.

## Error handling

- `check` package surfaces validation problems as `[]Finding{File, Rule, Severity, Message}`, never panics; warnings and errors coexist in one report.
- `lock` package: atomic claim creation via temp-file + `os.Link` (EEXIST → conflict); corrupt `.brood` files are skipped and reported, not fatal.
- `spec` package: atomic file writes via temp file (`O_CREATE|O_EXCL`) + rename, `.fledge-tmp-*` pattern, `0o644` permissions.

## Versioning & release

Three files must move together on every release — this is the load-bearing convention this regeneration exists to pin down precisely:

1. **`VERSION`** (repo root) — single-line plain-text source of truth; currently `0.5.8`.
2. **`internal/cli/version.go`** — the `binaryVersion` constant, injected at build time via ldflags (`-X github.com/Harrison-Blair/fledge/internal/cli.binaryVersion=$VERSION` in both `scripts/install.sh` and `.github/workflows/release.yml`); must match `VERSION` verbatim. `internal/cli/version_test.go`'s `TestBinaryVersionMatchesVersionFile` enforces this.
3. **`cmd/fledge/testdata/stamp_warning.txtar`** — the acceptance fixture pinning an old/new version pair to test the scaffold-mismatch warning (every command except `init`/`version` warns when `.fledge/scaffold.json`'s recorded version is older than the running binary's version; silent when matched or absent). `TestStampWarningTxtarVersionMatchesBinary` enforces this fixture stays in sync with `binaryVersion`.

Release process (`RELEASING.md`, `.github/workflows/release.yml`): bump all three files → push to `main` → CI's `detect-version` job compares current vs. prior-commit `VERSION` and, if changed, runs the 4-platform build (linux/darwin × amd64/arm64) behind an always-run safety-net (gofmt/vet/build/test) → `gh release create` with per-platform `tar.gz` + `sha256` artifacts → after release, `fledge init --refresh` re-stamps this repo's own scaffold to dogfood the new version. A failed release burns the version number (no automatic retry of the same VERSION value).

## Testing conventions

- Acceptance tests: `testscript`/`.txtar` format (`github.com/rogpeppe/go-internal/testscript`), 25 fixtures under `cmd/fledge/testdata/` (`ls cmd/fledge/testdata/*.txtar | wc -l` = 25); `cmd/fledge/main_test.go` registers the CLI function itself (not a built binary) so tests run in-process.
- Unit tests live beside their package; table-driven style is common (`t.Run` subtests), with concurrency tests using goroutine-barrier patterns for lock/roster/ID-allocation contention.
- CI and the optional local `scripts/hooks/pre-commit` hook both gate on `gofmt -l .` (check, not `-w` rewrite) + `go vet ./...`; CI additionally runs `go test ./...`.

## Open Questions

- Full go/template rendering context for `generate`/`primitive_map` scaffold files not fully traced (see architecture.md).
- Whether pi/codex adapter prose is fully reconciled with their derived Tier A label everywhere it's mentioned.
