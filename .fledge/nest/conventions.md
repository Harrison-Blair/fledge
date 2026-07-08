---
generated: 2026-07-08T05:28:12Z
commit: e46c481a047d45ef10bcd79a3326d47932b32868
agent: fledge-forager
fledge_version: 0.2.1
---

# Conventions

Cross-cutting patterns actually observed in the code — follow these so new work matches existing idiom.

## CLI structure

- **Command registration:** every command file has an `init()` calling `register(name, run, usage)`; `commandOrder` (`cli.go`) is the single ordering used for both usage output and the generated `Bash(fledge …)` allow-list. Add a command → also add it to `commandOrder`.
- **Exit codes are meaningful and shared:** `ExitOK`=0, `ExitFail`=1 (domain errors: validation, lock held, illegal transition, cycle), `ExitUsage`=2 (bad args), `ExitEnv`=3 (not a fledge repo / env). Error printers `fail`/`usageErr`/`envErr` prepend `fledge: `, print to stderr, return the code.
- **Uniform `--json`:** every command supports it via `emitJSON()`; JSON is indented. Human text is default; both come from one computed struct.
- **Shared loader:** most commands use `loadSet()` (`specload.go`) → repo + spec set + brood-held IDs + exit code. `relPath()` renders repo-relative paths for display.

## CLI owns frontmatter (never hand-edit)

- IDs (`PLM-###`, `FTHR-###`) and all frontmatter are CLI-allocated via `fledge new` / `set` / `status` / `criteria`. `id`, `plumage`, `authored`, `agent`, `fledge_version` are immutable through `set`. Spec *bodies* (prose) are hand-authored.
- Acceptance criteria are checkbox lists checked **only** via `fledge criteria check` — never by editing a box. `status … fledged` and `abandon --fledged` refuse while boxes are unchecked; `preen` errors on fledged specs with unchecked boxes.
- Status transitions are governed by maps (`taskTransitions`, `reqTransitions`); `--force` bypasses legality but not enum validity.

## Writes & idempotence

- Spec creation uses `O_CREATE|O_EXCL` (concurrent ID-allocation safety); teardown uses `WriteFileAtomic` (temp + rename).
- Scaffolding writes are **byte-idempotent** via `writeIfChanged()` (`registry.go:482`) — identical content = no write; this is what the txtar tests depend on.
- Lock acquisition rolls back if the status write fails after the lock file is created (`brood.go`; `lock_test.go:TestLockRollsBackOnStatusWriteFailure`).

## Manifest-driven scaffolding

- Adding/changing a harness is editing a `manifest.yaml`, zero Go code. Write policies (`ManifestFile`, documented `registry.go:38`): `generate`/`primitive_map` (render a template), `overwrite` (verbatim, rewrite on diff), `append_if_missing` (additive line), `symlink`, and default (copy, **skip-if-exists** so user edits survive; `init --refresh` re-syncs).
- **Agent-neutral core:** prose in `core/` must not reference harness-native paths (`.claude/`, `.pi/`) — enforced by `TestCoreNeutral`. Prose branches on *primitives* ("If you provide `spawn-worker`…"), never tier labels.

## Bird-themed naming

Terminology is bird-themed throughout and load-bearing — match it (see `domain.md`; `README.md` decodes it). nest, plumage, feather, brood, preen (validate), vee (graph), molt (evidence), colony (report), forager/scout/brooder/skua (agent roles). Worker names are `<role>-<species>` from an 18-penguin-species pool; **one species per brooder+skua pair** (`fledge-brooder-adelie` + `fledge-skua-adelie`), freed only after both confirm shutdown.

## Dogfooding discipline (this repo)

The installed `fledge` on `PATH` must match the source. After changing CLI or `internal/bootstrap/...`: `scripts/install.sh` (build → `go install` with version ldflags → `hash -r` → verify `fledge version` == `VERSION`), then `fledge init --refresh` to re-sync `.fledge/skills/` and the `.claude/` adapter, then update the affected `cmd/fledge/testdata/*.txtar` fixtures.
