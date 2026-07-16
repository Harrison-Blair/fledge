---
generated: 2026-07-15T23:53:12Z
commit: a4d02e8187c64ef9f3f1231052990b282207420b
agent: fledge-forager
fledge_version: 0.5.5
---

# Conventions

Naming, layering, error-handling, and process conventions observed across the repo, reconciled across modules.

## Spec lifecycle & IDs

- Plumage (`PLM-###`) lifecycle: `egg` (authored, unapproved) → `hatched` (user sign-off) → `fledged` (all linked feathers fledged + all acceptance-criteria boxes checked). Constants in `internal/spec/types.go` (`ReqEgg`, `ReqHatched`, `ReqFledged`).
- Feather (`FTHR-###`) lifecycle: `egg` (created, awaiting dependencies) → `pipping` (dependencies fledged, ready to dispatch) → `hatching` (claimed under active implementation — set by `fledge brood`, `internal/cli/brood.go:85`) → `fledged` (merged, all AC boxes checked). Constants: `TaskEgg`, `TaskPipping`, `TaskHatching`, `TaskFledged`.
- IDs (`PLM-###`, `FTHR-###`) are CLI-allocated only, never hand-invented: `internal/spec/ids.go` `NextID`/`AllocateAndCreate`, zero-padded to 3 digits (widens if existing max is wider), serialized via a `.alloc.lock` flock file to survive concurrent `fledge new` calls.
- File naming: `PLM-###-<kebab-title>.md`, `FTHR-###-<kebab-title>.md` under `.fledge/pluma/plumage/` and `.fledge/pluma/feathers/`. Kebab conversion via `spec.Kebab()`.
- Frontmatter is CLI-written only — never hand-edited. Spec *bodies* (prose) are user-authored; only frontmatter and acceptance-criteria checkbox state are machine-owned.
- Acceptance criteria: `- [ ] AC-N: …` checkbox lists in a `## Acceptance Criteria` section, authored unchecked, checked only via `fledge criteria check FTHR-### <n>` (mutates a single byte, per `internal/spec/criteria.go`) — never hand-edited.

## Layering

- Two deliberately separated layers (see `architecture.md`): the deterministic CLI (`internal/cli` + domain packages) vs. the manifest-driven bootstrap/adapter system (`internal/bootstrap`). The CLI depends on bootstrap; bootstrap never depends on CLI command logic.
- Manifest-driven adapters: adding/changing a harness is editing `manifest.yaml`, not writing Go code (`internal/bootstrap/registry.go`).
- Core skill content (`internal/bootstrap/core/`) is harness-neutral by construction and enforced by test (`TestCoreNeutral` in `internal/bootstrap/registry_test.go` — no `.claude/`/`.pi/`/`.codex/` path strings allowed in core prose).

## Error handling & exit codes

- Shared exit code scheme across every CLI command: `ExitOK(0)/Fail(1)/Usage(2)/Env(3)` (`internal/cli/cli.go`). Error/usage/env helpers prefix stderr output with `"fledge: "`.
- Every command supports `--json` for machine-readable output (`emitJSON()` in `internal/cli/cli.go`).
- `internal/check` collects `Finding`s (error/warning severity) rather than panicking; validation failures are data, not control flow.
- `internal/lock` uses a custom `*HeldError` to report lock contention distinctly from other failures; `List()` tolerates corrupt `.brood` files by skipping and reporting them rather than failing.
- Atomic file writes throughout: `spec.WriteFileAtomic()` (temp file + rename, 0o644), `lock.Acquire()` via `os.Link` (O_EXCL semantics), `internal/cli/brood.go` rolls back the lock file if the subsequent status write fails.

## Naming

- Bird-themed vocabulary throughout: nest, plumage, feather, brood, preen, molt, forager, scout, brooder, skua, incubator, hatch/hatching, pipping, fledged, egg, tracer bullet — see `domain.md` for the full glossary. `README.md` decodes it; match it in new code and prose.
- Worker naming scheme: `<role>-<species>` (e.g. `fledge-brooder-adelie`), drawn from an 18-item penguin species pool; brooder+skua pairs share one species; species free for reuse only after both confirmed shut down.
- Go identifiers: `snake_case` in YAML/JSON (frontmatter, manifest fields), `CamelCase` for exported Go types/functions, unexported helpers lowercase.

## Process / workflow conventions

- Spec mutation only via CLI (`fledge new`, `status`, `set`, `criteria`) — never by hand-editing frontmatter; spec bodies are the one thing agents write directly.
- Interrogation protocol: one question at a time, recommended answer first, facts looked up from codebase/nest, decisions put to the user (`fledge-interrogate` skill, `planning.md`).
- Gating protocol: review gates show material as a file path + diff (never full body pasted into chat); decision gates present concrete options; "Make changes" loops until Accept or explicit Pause/Discard.
- Test-first discipline for implementers: write tests from the feather's Tests section, run against unchanged code, capture *failing* output verbatim as evidence, then implement until green — never weaken/skip/delete a test to make it pass (`worker-protocols.md` §Brooder).
- No attribution trailers on commits (this repo's `CLAUDE.md` overrides fledge's default; e.g. no `Co-Authored-By`).
- Communication topology during implementation: brooder ↔ skua ↔ orchestrator only, no peer-to-peer brooder/brooder or skua/skua channels; boundary questions always route through the orchestrator.

## Testing conventions

- Unit tests live beside their package (`internal/<pkg>/*_test.go`), standard library `testing` only (no third-party assertion libs observed).
- CLI acceptance tests are `testscript`/`.txtar` files under `cmd/fledge/testdata/`, run via `TestScripts`.
- `t.TempDir()` used throughout for isolation; no manual fixture cleanup needed.
- Structural "keep docs/CI honest" tests exist as their own packages (`internal/ciconfig`, `internal/doctest`, `internal/hooktest`) rather than being folded into the packages they check — see `testing.md`.

## Versioning & release

- `VERSION` (repo root) and `internal/cli/version.go`'s `binaryVersion` (injected via `-ldflags` at build/release time) must match; `internal/cli/version_test.go` asserts this.
- Release triggers only when `VERSION` changes on `main` (`.github/workflows/release.yml` `detect-version` job); the 4-platform build matrix is Unix-only (linux/darwin × amd64/arm64) — no Windows.
- `.fledge/scaffold.json` (written by `fledge init --refresh`) is the deterministic stamp of which files fledge owns and at what content hash; keys are sorted, output is byte-idempotent (`writeIfChanged`).

## Open Questions

- Whether `internal/bootstrap`'s `append_if_missing` write policy matches on exact line content or substring presence (raw `bootstrap-adapters.md` scout could not confirm from the manifest alone).
