---
generated: 2026-07-16T02:20:48Z
commit: 407b91e70b53764944447dae5829d2076fb852c5
agent: fledge-forager
fledge_version: 0.5.5
---

# Architecture

How fledge's two layers — the deterministic CLI and the embedded bootstrap/adapter scaffolding system — fit together, plus the repo's own dogfooding loop (specs, nest, broods).

## Two layers, deliberately separated

**1. CLI dispatch + domain packages** (`internal/cli`, `cmd/fledge`). `cmd/fledge/main.go` is an 11-line bridge calling `internal/cli.Run(os.Args[1:])`. `internal/cli/cli.go` holds the command registry (`commands` map, `Run()` entry point, `commandOrder` list) — each command file (`init.go`, `new.go`, `status.go`, `brood.go`, `preen.go`, `nest.go`, `set.go`, `criteria.go`, `scan.go`, `vee.go`, `ready.go`, `unfledged.go`, `update.go`, `agents.go`, `colony.go`) calls `register(name, run, usage)` in its own `init()`. Exit codes are semantic and shared: `ExitOK=0`, `ExitFail=1` (domain error), `ExitUsage=2` (CLI misuse), `ExitEnv=3` (not a git repo / missing `.fledge/`). Every command supports `--json`. Domain logic underneath the CLI layer lives in focused packages: `internal/spec` (frontmatter, IDs, templates, load), `internal/check` (validation = preen), `internal/graph` (dependency waves = vee), `internal/lock` (feather claims = brood), `internal/nest` (context-doc scaffolding — the system this very document is a product of), `internal/scan`, `internal/repo`.

**2. Bootstrap/adapter system** (`internal/bootstrap`). What `fledge init` scaffolds into a target repo. `bootstrap.go` embeds two trees via `//go:embed core adapters`. `core/` is the single agent-neutral source — the `fledge-orchestrate` and `fledge-interrogate` skills (`internal/bootstrap/core/skills/fledge-orchestrate/{SKILL.md,planning.md,implementation.md,foraging.md,worker-protocols.md,templates/}`) — written to a repo's `.fledge/skills/` by `WriteCore()`. `adapters/<harness>/` (claude, codex, pi) is a thin format-only mapping, each driven entirely by its `manifest.yaml` (`registry.go` → `Manifest` struct): the detector marker (e.g. `.claude/`), the `tier_primitives` map, and a file list with per-file write policies. Adding a harness is a manifest edit, not a Go code change (`internal/bootstrap/registry_test.go:TestAdapterManifests`).

**The 6 primitives** (`internal/bootstrap/primitives.go`, `PrimitiveOrder`): `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`. An adapter declares which primitives it realizes; its tier (A/B/C) is *derived* from that coverage via `DeriveTier()`, never declared directly — Tier A requires `{confirm-gate, read-only-shell, write-file, run-fledge}`, B adds `spawn-worker`, C adds `message-peer`. Claude's adapter is tier C (6/6), codex and pi are tier A (4/6) (`internal/bootstrap/registry_test.go:TestPrimitiveCoverage`).

**Write policies** (`ManifestFile`, `internal/bootstrap/registry.go`): `core` (embedded verbatim, byte-compared, never user-edited), `default` (copy verbatim, skip-if-exists so user edits survive; reset by `--refresh`), `generate`/`primitive_map` (render a `text/template`), `overwrite` (always template-rendered and repaired, e.g. the fledge-adapter.md entry file), `symlink` (e.g. `.claude/skills/...` → `.fledge/skills/...`, lets Claude Code discover core skills natively), `append` (additive line into an existing file, e.g. `.gitignore`). `writeIfChanged()` makes writes byte-idempotent — this is what the txtar acceptance tests depend on for determinism.

**Drift and the stamp.** `fledge init --refresh` writes `.fledge/scaffold.json` (`internal/bootstrap/stamp.go`, `Stamp` struct: version, agents, file map of policy+hash/target/lines) — the record of what fledge owns and at what content hash. `internal/bootstrap/drift.go` classifies every scaffolded file on disk into one of 5 states relative to that stamp: up-to-date, stale (binary moved, file unedited — refresh-safe), modified (user-edited — requires confirmation before refresh), missing, or obsolete (binary no longer ships it). `fledge preen` surfaces this via `internal/cli/preen.go`.

## The repo's own dogfooding loop

This repo consumes its own binary: `.fledge/pluma/{plumage,feathers}/` hold this repo's own PLM-###/FTHR-### specs (owned by `internal/spec`), `.fledge/broods/` hold feather claims (`internal/lock`), and `.fledge/nest/` (this directory) holds the synthesized repository knowledge that the fledge-orchestrate skill's planning phase reads before touching specs. The forager/scout roles that produce `.fledge/nest/` are themselves implemented in `internal/nest` (scaffold/scout/stamp/status logic) and specified in `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md`; the Claude-specific worker specs live at `internal/bootstrap/adapters/claude/agents/fledge-{forager,context-scout,incubator,brooder,skua}.md` (symlinked into `.claude/agents/` in this repo).

## Cross-module relationships

- `cmd/fledge` → `internal/cli.Run()` → dispatches to command handlers, which call `internal/spec`, `internal/check`, `internal/graph`, `internal/lock`, `internal/nest`, `internal/scan`, `internal/repo`, and (for `init`/`preen`/`agents`) `internal/bootstrap`.
- `internal/bootstrap` never calls back into `internal/cli`; `internal/cli/init.go` passes `commandOrder` into `bootstrap.WriteAdapter`/`ExpectedFiles` so scaffolded allow-lists and primitive-map entries stay in sync with the live command registry (`internal/cli/command_parity_test.go:TestCommandOrderMatchesRegistrations`).
- `internal/nest` (concern-doc/scout templates and `Status()`/`IsStub()` logic) is consumed both by `internal/cli/nest.go` (the `fledge nest` subcommands) and conceptually by this very forager pipeline.
- CI (`.github/workflows/pr-check.yml`, `release.yml`) and the optional local pre-commit hook (`scripts/hooks/pre-commit`) both gate on `gofmt -l .` and `go vet ./...`, matching `internal/ciconfig` tests that assert on the workflow YAML shape itself.
