---
generated: 2026-07-17T17:48:26Z
commit: 1c9011d6e6a06f72f96bc98e3b2bd99c408ab79e
agent: fledge-forager
fledge_version: 0.6.10
---

# Conventions

Coding, process, and prose conventions reconciled across the Go source (`internal/`, `cmd/`) and the orchestration workflow (`internal/bootstrap/core/skills/`).

## Go source conventions

- **Command registration**: each `internal/cli/*.go` command file has a `func init()` calling `register(name, runFunc, usage)`; `commandOrder` in cli.go must list the same names — enforced by `internal/cli/command_parity_test.go` (FC-3).
- **Exit codes, always returned**: `ExitOK=0`, `ExitFail=1` (domain error: check findings, lock held, illegal transition, cycle), `ExitUsage=2`, `ExitEnv=3` (no git repo / no `.fledge/`), `ExitTimeout=4` (`fledge await` only). `ExitUsage`/`Fail`/`Env` print to stderr prefixed `"fledge: "` (internal/cli/cli.go).
- **`--json` on every command**: `emitJSON(map[string]any)` emits indented JSON to stdout (all internal/cli/*.go).
- **Shared spec loading**: `internal/cli/specload.go`'s `loadSet()` is the single entry point for commands needing repo+specs; enforces `.fledge/` presence and returns locked feather IDs alongside the set.
- **Error wrapping**: `fmt.Errorf(...: %w)` throughout; typed error returns for caller discrimination (`ledger.NotFoundError`/`CorruptError`/`InvalidSubjectError`, `lock.HeldError`).
- **Atomicity**: `spec.WriteFileAtomic` (temp file + chmod + rename); `ledger` writes via atomic `os.Rename`; `lock.Acquire` uses `os.Link` for exclusive creation; both tolerate partial/zero-length observation from concurrent readers.
- **Flock-serialized allocation**: `internal/spec/ids.go`'s `AllocateAndCreate` and `internal/roster/roster.go` both use `syscall.Flock(LOCK_EX)` on a dotfile lock to serialize concurrent state mutation — the same pattern in two independent packages.
- **Byte-preservation**: spec `Body` is stored/rendered as raw `[]byte`, never re-serialized; `SetCriterion` mutates a single checkbox byte, preserving everything else (internal/spec/criteria.go, internal/spec/frontmatter.go).
- **Deterministic JSON**: `.fledge/scaffold.json` stamp uses `json.MarshalIndent` with 2-space indent, trailing newline, alphabetically sorted map keys, so refresh output is byte-reproducible (internal/bootstrap/stamp.go).
- **Testing seams**: injectable clocks/readers for determinism — `awaitClock` (internal/cli/await.go), `updateExecutablePath` (update.go), `promptYesNo` reader injection (init.go).
- **Test frameworks**: standard library `testing` only, no external frameworks, anywhere in the repo. Acceptance tests use `github.com/rogpeppe/go-internal/testscript` (txtar format) exclusively in `cmd/fledge`.

## Spec lifecycle & CLI-only mutation

- Plumages: `egg` (authoring) → `hatched` (user-accepted) → `fledged` (all feathers fledged + all AC boxes checked).
- Feathers: `egg` (authoring) → `pipping` (ready hint once depends_on fledged) → `hatching` (claimed via `fledge brood`) → `fledged` (merged + verified).
- **All status transitions go through `fledge status`/`fledge set`; acceptance-criteria boxes only via `fledge criteria check` — never hand-edited.** Repeated across CLAUDE.md, bootstrap-core prose, and enforced by `internal/cli/status.go`'s transition maps + `internal/spec/criteria.go`'s single-byte mutation.
- `status:egg`/`pipping` in frontmatter is an authoring-time hint only; only `fledge ready`/`fledge vee` recompute actual dispatchability from `depends_on` + lock state — never judge blocked-ness from frontmatter alone.
- `fledge set X depends_on Y` **replaces** the field, it does not append — always pass the full list when updating dependencies.

## ID and naming conventions

- `PLM-###` / `FTHR-###`: sequential, zero-padded, allocated per-directory (`.fledge/pluma/plumage/`, `.fledge/pluma/feathers/`), never hand-invented — always via `fledge new`.
- Files: `PLM-###-<kebab-name>.md`, `FTHR-###-<kebab-name>.md`. `Kebab()` preserves Unicode letters.
- Worker instances: `<role>-<species>` (e.g. `fledge-brooder-adelie`), species allocated from a canonical 18-penguin-species list (`internal/roster/roster.go`) via `fledge roster assign`, never invented ad hoc. Overflow issues numeric suffixes (`adelie-2`).
- Scouts are unnamed (no species) — spawned by a forager, self-terminating.
- Harness adapter/agent files: `fledge-<role>.md` (Claude agents), one manifest.yaml per adapter directory named after the harness (`claude`, `codex`, `pi`).

## Bootstrap/scaffold conventions

- **Core vs. scaffolded copies**: `internal/bootstrap/core/` and `internal/bootstrap/adapters/` are the source of truth; `.fledge/skills/`, `.claude/`, `.codex/`, `.pi/` in a consumer repo (including this one) are *generated output*. Edit the source, never the copy, then `fledge init --refresh` to resync.
- **Tier is derived, never declared**: `DeriveTier()` computes A/B/C from an adapter's declared `tier_primitives` coverage (internal/bootstrap/primitives.go).
- **File write policies** (internal/bootstrap/registry.go `ManifestFile`): `generate`/`primitive_map` (template), `overwrite` (always rewritten), `append_if_missing` (additive), `symlink`, default (copy, skip-if-exists — preserves user edits).
- **Adding a harness = editing a manifest.yaml, zero Go code.**
- Dev mode (`fledge init --dev`) symlinks scaffold files into a source checkout instead of copying — lets shipped prose be edited live while dogfooding; hand-made scaffold symlinks predating `--dev` let plain `--refresh` silently overwrite `internal/bootstrap` source, so use `--dev` for that setup.

## Worker-coordination conventions (orchestration workflow)

- **Ledger over messages**: state-bearing handoffs (verdict, escalation, liveness/done signal) are written to and read from CLI ledger records (`fledge heartbeat`/`verdict`/`escalate`/`ledger read`), never inferred from message arrival or lifecycle notifications alone.
- **Heartbeat discipline**: `fledge heartbeat <name> [--note]` called before *and* periodically during long operations — never before-only, to avoid a long step being misread as a stall.
- **`fledge await`**: change-wait (repeatedly-written `status` records, never `--exists`) vs. exists-wait (write-once `verdict`/`escalation`). Exit 4 (timeout) is recovered with `fledge pulse`, never hand-rolled polling or timestamp comparison.
- **Communication topology is fixed per phase**: planning — incubator↔orchestrator (relay), incubator↔forager (commission/shutdown); implementation — brooder↔skua (handoff/findings/fix), brooder↔orchestrator (escalation), skua↔orchestrator (verdict/escalation). No other peer channels.
- **Role boundaries are hard prohibitions**: brooders/skuas never spawn workers or write the team task list; brooders never edit spec files on main; incubators never spawn implementation workers.
- **Scope discipline**: only changes traceable to the spec; no speculative features/abstractions/configurability; don't "improve" adjacent code; match existing style; remove only orphans your own change created.
- **Test-first, no exceptions**: AC-1 is always "tests observed FAILING before implementation, PASS after"; tests never weakened/skipped/deleted to pass — escalate instead. Evidence captured verbatim into `.fledge/molt/FTHR-###.md` incrementally, not from memory.
- **Commits never carry attribution trailers** (no `Co-Authored-By`) — stated repeatedly across worker protocols and the user's own global CLAUDE.md.
- **Interrogation style**: one question at a time with a recommended answer first; every decision put to the user explicitly (never assumed from silence); independent low-stakes questions may batch into a scratchpad file, but load-bearing gates (breakdowns, spec-draft review) are always individual.

## Documentation & terminology conventions

- Bird-themed vocabulary is used consistently everywhere — code comments, CLI usage strings, prose, commit conventions (see domain.md for the full glossary).
- `CLAUDE.md` is the canonical developer/agent reference; `AGENTS.md` is a one-line pointer auto-loaded by non-Claude harnesses.
- When embedded `core/`/`adapters/` content changes, the `cmd/fledge` txtar fixtures (`init.txtar`, `init_agents.txtar`, `agents.txtar`) must be updated alongside — they assert on exact scaffolded output.

## CI & release conventions

- `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...` run on every PR (`.github/workflows/pr-check.yml`) and again as a release safety-net (`release.yml`).
- Optional local pre-commit hook mirrors the same two lint checks (`scripts/hooks/pre-commit`, opt-in via `git config core.hooksPath scripts/hooks`).
- Release triggers on VERSION-file change on push to main; builds 4 platforms (linux/darwin × amd64/arm64). VERSION bump touches 3 files; a failed release burns the version number (cannot be reused).
