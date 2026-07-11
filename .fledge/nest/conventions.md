---
generated: 2026-07-11T01:58:32Z
commit: 96a3ac38bc843217824d6d6886c49906053bf686
agent: fledge-forager
fledge_version: 0.3.4
---

# Conventions

Coding, naming, and process conventions observed across the codebase and its specs, reconciled from all scouted modules.

## Naming & terminology

- Bird-themed vocabulary throughout, documented in `README.md`: plumage, feather, nest, brood, preen, molt, forager, scout, skua, colony, ready, vee. See `domain.md` for the full glossary — match it in new code and prose (`CLAUDE.md`).
- Exported Go functions are action verbs (`Acquire`, `Release`, `Run`, `Find`, `DeriveTier`); unexported helpers use a `check*` prefix in `internal/check` (`checkRequired`, `checkEnum`, `checkAuthored`).
- IDs are CLI-allocated, never hand-invented: `PLM-###` (`internal/spec/ids.go:NextID`), `FTHR-###`. Filenames use kebab-case titles (`internal/spec/ids.go:Kebab`).

## CLI command pattern

- Every command file self-registers: `init()` calls `register(name, run, usage)` (`internal/cli/cli.go`); `commandOrder` is the single list controlling usage text, dispatch validation, and generated allow-lists — there is no central switch statement.
- Shared exit codes: `ExitOK`/`Fail`/`Usage`/`Env` = 0/1/2/3 (`internal/cli/cli.go`).
- Flag convention: `--json` for machine-readable output (not `--output=json`); `--force` to override confirmation gates.
- Positional + flag parsing goes through `parseMixed()` so `--flag` can appear before or after positional args (`internal/cli/cli.go`).
- Shared error helpers `fail()`, `usageErr()`, `envErr()` prefix messages with `"fledge: "` and return the matching exit code.
- Sorting is always deterministic: by ID (with priority tie-break in `ready`/`unfledged`), byte-sorted file lists in `scan`, feather-ID order in `lock.List`.

## File I/O & data safety

- **Atomic writes everywhere**: `spec.WriteFileAtomic` (temp file + `os.Rename`), `lock.Acquire` (`os.O_CREATE|os.O_EXCL` to prevent race conditions), `internal/bootstrap`'s `writeIfChanged()` (byte comparison before write, enabling idempotent `init` re-runs and deterministic test fixtures).
- **Byte preservation**: spec markdown bodies are read/written exactly as-is (`internal/spec/frontmatter.go`); `internal/nest`'s `RefreshDoc()` preserves body while refreshing only frontmatter fields.
- **YAML safety**: `spec.YAMLScalar()` quotes strings that are empty, numeric-like, boolean-keyword-like, or contain unsafe characters — reused by `internal/nest` frontmatter rendering.
- **Silent, collected errors**: `spec.Load()` collects per-file parse errors into `Set.Errors`/`UnknownFields` rather than failing the whole load, so one bad spec file doesn't block every command.
- Path normalization: manifests use slash-separated paths; `filepath.FromSlash()`/`ToSlash()` convert at the OS boundary (`internal/bootstrap`).

## Spec lifecycle & governance

- Feather lifecycle: `egg → pipping → hatching → fledged`. Plumage lifecycle: `egg → hatched → fledged`. Acceptance criteria are checkbox lists (`AC-N`) checked *only* via `fledge criteria check` — never hand-edited.
- Frontmatter is CLI-owned; hand-editing is discouraged by doctrine (`CLAUDE.md`) and structurally enforced by the fact that `fledge new`/`set`/`status`/`criteria` are the only supported mutation paths.
- Spec section order is fixed: feathers use Description → Affected Modules → Approach → Tests → Acceptance Criteria; plumage uses Context → User Stories → Functional Criteria → Acceptance Criteria → Out of Scope → Open Questions.
- Test-first discipline is a written convention in feather specs themselves, not just an aspiration: specs describe writing a failing test first, then implementing.

## Scaffolding conventions (`internal/bootstrap`)

- **Manifest-as-source**: every adapter's behavior is declared in YAML (`manifest.yaml`); adding a harness requires zero Go code changes.
- **Write policies are mutually exclusive** and classified in priority order: `primitive_map` > `generate` > `overwrite` > `symlink` > `append` > default (copy, skip-if-exists).
- **Stamp-then-drift**: `.fledge/scaffold.json` (written by `Stamp.Write`) records exactly what fledge wrote; `DriftReport()` compares on-disk vs. expected (freshly rendered) vs. stamped (for stale detection) — there is no drift classification without a prior stamp.
- Error wrapping: `fmt.Errorf("context: %w", err)` throughout `internal/bootstrap`.

## Build & versioning

- Go 1.26, no Makefile — use `go` directly (`go build ./...`, `go test ./...`, `go vet ./...`).
- Binary version is ldflag-injected from `VERSION`; `version_test.go` (`internal/cli`) pins the binary's version constant to the repo's `VERSION` file at test time to catch drift.
- `warnStampMismatch()` (`internal/cli`) emits a stderr advisory when a scaffolded repo's stamped `fledge_version` diverges from the running binary's version — skipped only for `init` and `version` themselves.

## Open Questions

- Whether Claude Code supports a `skills` array in `settings.json` (like pi) or requires the `.claude/skills/` symlink fallback currently used — flagged as critical in `docs/generalization-plan.md` but unresolved as of this scout pass.
- Exact Codex/Cursor/opencode skill-discovery layout (`AGENTS.md` auto-load, `.cursor/rules/*.mdc`, `opencode.json` vs `.opencode/`) — all deferred per `docs/generalization-plan.md` §12.
