---
generated: 2026-07-10T14:50:00Z
commit: 7678344ab9a18730530b9f6edf507ad0c449d352
agent: fledge-forager
fledge_version: 0.2.1
---

# Conventions

Naming, error-handling, and idiom conventions observed across the CLI, domain packages, bootstrap system, and specs, reconciled across all scouted modules.

## Bird-themed terminology (repo-wide)

Every layer uses the same vocabulary — see `domain.md` for full glossary. Match it in new code/prose (`README.md`, `CLAUDE.md`).

## Command structure (`internal/cli`)

- One file per command; each file's `init()` calls `register(name, run, usage)` (`internal/cli/cli.go`); `commandOrder` centrally controls both `--help` ordering and generated Claude allow-lists (Q23, `docs/generalization-plan.md`).
- Command signature: `func(args []string) int`. Uses `flag.NewFlagSet` with `flag.ContinueOnError`; several commands use a custom `parseMixed()` to allow positionals before flags (`brood`, `criteria`, `set`, `nest`).
- Shared helpers in `internal/cli/specload.go`: `loadSet()`, `lockedTaskIDs()`, `relPath()`, `fileExists()`, reused across nearly every command.
- Output: human text to stdout by default; `--json` via shared `emitJSON()` (indented, snake_case field names e.g. `depends_on`, `fledge_version`). Errors to stderr via `fail()` (domain), `usageErr()` (usage), `envErr()` (environment) — exit codes `ExitOK/Fail/Usage/Env` = 0/1/2/3.
- Sorted output everywhere: adapters by name, tasks by priority-then-ID, findings/orphans/issues alphabetically.

## Spec/file conventions (`internal/spec`, `internal/nest`, `internal/bootstrap`)

- **Byte preservation**: spec bodies are never re-serialized — frontmatter is canonical and rewritten, body returned exactly as found on disk (`internal/spec/frontmatter.go`).
- **Atomic writes**: `spec.WriteFileAtomic()` (temp file + rename) used for all file creation (`new`, `init`, `nest`); locking uses `os.OpenFile(...O_CREATE|O_EXCL...)` for single-writer guarantees (`internal/lock/lock.go`).
- **Byte idempotence**: `internal/bootstrap`'s `writeIfChanged` skips writes when content is identical — depended on by the `cmd/fledge/testdata/*.txtar` fixtures; a second `fledge init --refresh` over unchanged files reports all-skipped.
- **YAML scalar quoting**: `spec.YAMLScalar` quotes strings containing special characters, numeric-like values, or ambiguous keywords, to keep frontmatter round-trippable.
- **Frontmatter key order** is fixed per doc kind (`internal/nest/nest.go`): concern docs use `generated, commit, agent, fledge_version`; scout docs use `module, authored, agent, fledge_version`.
- **ID allocation**: `spec.NextID` scans existing filenames per-prefix (never hand-invented), preserving digit width if any existing ID is wider than 3 digits. `spec.Kebab` lowercases titles, collapses non-alphanumeric runs to single hyphens, strips leading/trailing hyphens, preserves Unicode letters.
- **Acceptance criteria**: strict regex format `^- \[([ xX])\] (AC-(\d+)):[ \t]?(.*)$`, unindented only, mutated only via `fledge criteria check/uncheck` — never hand-edited.

## Bootstrap/manifest conventions (`internal/bootstrap`)

- Core skills are a single agent-neutral source under `core/skills/` (no harness-specific naming); adapters live under `adapters/<harness>/`, each with exactly one `manifest.yaml`.
- Adapter directories prefixed `_` are skipped (shared assets, not surfaced as harnesses).
- Adapter/primitive names are lowercase, kebab-case single words (`claude`, `pi`, `codex`; `confirm-gate`, `read-only-shell`).
- Generated files (`Generate`/`PrimitiveMap` policy) always use `text/template` with a shared `renderContext`; default-policy files skip-if-exists so user edits survive; `overwrite`-policy files are always repaired.
- Registry errors wrap context via `fmt.Errorf` (adapter name, operation, file path).

## Testing conventions

See `testing.md` for full detail. Summary: unit tests live beside their package; CLI-level behavior is asserted via testscript/txtar acceptance fixtures in `cmd/fledge/testdata/`, which the `cmd/fledge/testdata/*.txtar` fixtures depend on byte-idempotent writes to pass reliably across repeated runs.

## Documentation/build conventions

- No Makefile; direct `go build`/`go test`/`go vet` commands (`CLAUDE.md`).
- `scripts/install.sh` uses bash strict mode (`set -euo pipefail`); verifies installed binary version matches the `VERSION` file post-install.
- Version is single-sourced from the root `VERSION` file, injected into the binary via `-ldflags "-X .../internal/cli.binaryVersion=$want"`; `internal/cli/version_test.go` pins this at test time.
