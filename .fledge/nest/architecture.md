---
generated: 2026-07-08T05:28:12Z
commit: e46c481a047d45ef10bcd79a3326d47932b32868
agent: fledge-forager
fledge_version: 0.2.1
---

# Architecture

fledge is a Go CLI that keeps spec-driven development artifacts on disk and scaffolds one agent-neutral orchestration workflow into any agent harness. The system is two deliberately separated layers plus the spec corpus they both operate on; understand how a change ripples across these before touching `internal/bootstrap` or the CLI surface.

## The two layers

**1. The deterministic CLI (`internal/cli` + domain packages).** Agent-agnostic spec operations with meaningful, shared exit codes (`ExitOK/Fail/Usage/Env` = 0/1/2/3) and a uniform `--json` on every command (`internal/cli/cli.go`). Command dispatch is registration-based: each command file's `init()` calls `register(name, run, usage)`, and `commandOrder` (`cli.go`) controls both usage output and the generated allow-lists. Domain logic lives in focused packages: `spec` (frontmatter, ID allocation, templates, load, atomic writes), `check` (validation = `preen`), `graph` (dependency graph = `vee`; cycles, waves, readiness), `lock` (feather claims = `brood`), `scan`, `nest` (concern/scout doc rendering), `repo` (root discovery, directory paths, git HEAD/version).

**2. The bootstrap/adapter system (`internal/bootstrap`).** What `fledge init` scaffolds. `bootstrap.go` embeds two trees via `//go:embed core adapters`:
- **`core/`** is the single agent-neutral source — the `fledge-orchestrate` and `fledge-interrogate` skills, written to a repo's `.fledge/skills/` by `WriteCore` (`registry.go:304`). The actual workflow prose (planning.md, implementation.md, foraging.md, worker-protocols.md, templates/) lives here.
- **`adapters/<harness>/`** is a thin format-only mapping per harness, driven entirely by its `manifest.yaml` (`registry.go` → `Manifest`): a detector, the `tier_primitives` map, and a file list with per-file write policies. Adding/changing a harness is editing a manifest — zero Go code.

## The primitive/tier contract

The workflow is written to **6 orchestration primitives** (`primitives.go`, `PrimitiveOrder`): `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`. An adapter declares which mechanism realizes each; its **tier** (A=4 / B=5 / C=6 primitives) is *derived* from that coverage via `DeriveTier` (`primitives.go:46`), never declared. Claude Code is Tier C. Core prose branches on capability ("If you provide `spawn-worker`…"), so one skill adapts to any tier without duplication (`docs/generalization-plan.md` §5.1).

## How the layers interact (the dogfooding loop)

`fledge init` scaffolds `.fledge/skills/` + the harness adapter → the agent auto-loads the skill and adapter → the agent drives the CLI for every deterministic spec op (`fledge new/status/set/criteria/brood/preen/nest`) and writes only spec *bodies* by hand. This repo is itself fledge-managed: it dogfoods its own binary, so the installed `fledge` must match the source (`scripts/install.sh`).

## Ripple map (what to change together)

- Change embedded `core/` or `adapters/` content → regenerate this repo's scaffold (`fledge init --refresh`) **and** update the `cmd/fledge/testdata/` txtar fixtures that assert on scaffolded output (`init.txtar`, `init_agents.txtar`, `agents.txtar`).
- Add a CLI command → `register()` in a new command file + append to `commandOrder` (feeds usage and the generated `Bash(fledge …)` allow-list) + a `<cmd>.txtar` acceptance test.
- Change the primitive set → update `primitives.go`, every adapter manifest's `tier_primitives`, and `registry_test.go` coverage tests (which fail if core prose references a primitive no adapter declares, or vice versa).

## Open Questions

- Whether Claude Code `settings.json` supports a `skills` array or only recursive `.claude/skills/` scanning determines if the adapter must symlink `.claude/skills/fledge-orchestrate` → `.fledge/skills/fledge-orchestrate` (`docs/generalization-plan.md` §12 — the one fragile piece of the core-relocation design). Directly relevant to agent-detection work.
