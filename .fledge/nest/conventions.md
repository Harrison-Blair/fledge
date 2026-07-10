---
generated: 2026-07-10T20:53:53Z
commit: f28efebd76d6aa135adb0956a3337a40a8d98351
agent: fledge-forager
fledge_version: 0.3.0
---

# Conventions

Coding, naming, and process conventions observed across the repo, reconciled across all seven scout reports.

## Spec/CLI conventions (repo-wide)

- **Deterministic spec mutation only**: IDs (`PLM-###`, `FTHR-###`) and frontmatter are always CLI-allocated (`fledge new`, `status`, `set`, `criteria`, `brood`) — never hand-edited. `internal/cli/status.go`, `criteria.go`, `brood.go` all route through `internal/spec` and `internal/lock`, never touch YAML directly.
- **Acceptance criteria**: checkbox lists only (`- [x] AC-N: ...`), mutated one byte at a time (space↔`x`) via `internal/spec/criteria.go:SetCriterion` — idempotent, byte-preserving, never re-serializes the rest of the body.
- **Body preservation**: markdown body after the closing `---` frontmatter delimiter is read as-is and re-emitted unchanged; round-trips are byte-idempotent (`internal/spec/frontmatter.go`).
- **Filenames**: `<ID>-<kebab-case-title>.md`, e.g. `PLM-004-agent-detection.md`; slugs generated via `internal/spec/ids.go:Kebab`.
- **Status lifecycles**: feathers `egg → pipping → hatching → fledged`; plumages `egg → hatched → fledged` (no pipping/hatching — that's feather-only). Legal transitions enforced by `taskTransitions`/`reqTransitions` maps in `internal/cli/status.go`, overridable with `--force`.
- **Frontmatter key order is fixed per type**: Requirement (`id, title, status, priority, authored, agent, fledge_version`); Task adds `plumage, depends_on` before `authored`, with `oversight` omitted when empty. Canonical YAML quoting via `spec.YAMLScalar` (empty/`""`/boolean-keyword/numeric/unsafe-char values get quoted).

## Bird terminology (match in all new code/prose)

plumage (requirement), feather (task), nest (repo knowledge dir), brood (feather claim), preen (validate), molt (evidence dir), vee (dependency graph viz), colony (status summary), forager (context-gathering orchestrator), scout (context reader), brooder (implementor), skua (reviewer). See `README.md` for the full glossary; `domain.md` in this doc set for definitions.

## Go/CLI code conventions (`internal/cli`, `internal/bootstrap`, `internal/*`)

- **Command registration**: each command file has its own `init()` calling `register(name, runFunc, "usage: ...")` exactly once; `commandOrder []string` in `cli.go` controls usage-listing order and feeds bootstrap's allow-list generation.
- **Error helpers**: `fail(format, ...)` → stderr `"fledge: "`-prefixed + `ExitFail`; `usageErr(...)` → `ExitUsage`; `envErr(...)` → `ExitEnv`. All CLI error strings are `"fledge: "`-prefixed for consistency.
- **Flag parsing**: each command owns a `flag.FlagSet("name", flag.ContinueOnError)`; several commands use a shared `parseMixed(fs, args)` helper to allow positional args before flags.
- **Shared state loading**: `internal/cli/specload.go:loadSet()` centralizes repo discovery + spec loading + lock listing; called once per command that needs specs.
- **Output**: text to stdout with no prefix; `--json` via `emitJSON()` (indented `encoding/json`, `ExitOK` on success else `ExitFail`); paths displayed relative via `relPath(root, p)`.
- **Atomic writes**: `spec.WriteFileAtomic` (temp file + rename in same dir, no partial reads); `internal/bootstrap`'s `writeIfChanged()` skips writes when bytes are unchanged, keeping scaffolding byte-idempotent.
- **Error wrapping**: contextual `fmt.Errorf("read %s: %w", ...)` style throughout `internal/bootstrap`.
- **Testing style**: table-driven tests, `t.TempDir()` for isolation, no explicit cleanup (OS handles temp dirs), small local helpers (`makeHash`, `writeTestFile`, `write`, `initRepo`) for DRY setup. No third-party assertion library (`testify` not used) — plain `t.Errorf`/`t.Fatal`.
- **Alphabetical ordering**: unknown-field lists, brood lists, module names sort alphabetically for deterministic output.

## Bootstrap/adapter conventions

- **File write policy semantics** (`internal/bootstrap/registry.go`): `overwrite` = fledge-managed, always repaired; default (skip-if-exists) = user may customize, preserved on plain `init`; `symlink` = never copied, always managed, must point at declared target; `append_if_missing` = additive only, never destructive.
- **Hash-based preserve/prune**: on `--refresh`, disk hash vs. old stamp hash distinguishes user edits (preserved unless `--force`) from provably-unedited files (silently rewritten).
- **Embed path convention**: `core/` content maps to `.fledge/<rel>` in the target repo; `adapters/<harness>/` content maps to harness-native paths (e.g. `.claude/...`).
- **Scout report structure** (`internal/bootstrap/core/skills/fledge-orchestrate/templates/scout-report.md`): fixed section order — Purpose, Structure & Key Files, Entry Points & Public Interfaces, Data Types, External Dependencies, Conventions Observed, Tests, Domain Terms, Open Questions. Sections with nothing to report say `"None observed."` — never omitted.
- **Frontmatter stamping**: only the CLI (`fledge nest scaffold`, `fledge nest stamp`) writes nest/scout frontmatter; agent prompts never hand-edit it.

## Test fixture conventions (`cmd/fledge/testdata/*.txtar`)

- testscript/txtar format (`github.com/rogpeppe/go-internal/testscript`); one script per command/feature area.
- Git config isolation: `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_SYSTEM=/dev/null`, fixed author/committer identity for determinism.
- Both human-readable and `--json` output shapes asserted per fixture; error cases use `! exec fledge <cmd>` plus stderr grep assertions.
- Fixture repo state (`.fledge/.gitkeep`, `pluma/` dirs) simulated inline via txtar `-- <path> --` blocks.
- **Must-update rule**: whenever `internal/bootstrap/core/` or `internal/bootstrap/adapters/` content changes, `init.txtar`, `init_agents.txtar`, and `agents.txtar` need matching updates — they assert on exact scaffolded output.

## Open Questions

- `internal/cli/init.go` and `internal/cli/preen.go` both define a local `baseScaffoldEntries()`-style helper — duplicated rather than shared from `internal/bootstrap` or `internal/spec`; unclear if intentional. (internal-cli scout)
- `tierLabel()` in `init.go` returns a Unicode en-dash "—" for empty tier — styling choice or portability concern not documented. (internal-cli scout)
</content>
