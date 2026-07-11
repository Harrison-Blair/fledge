# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

fledge is a Go CLI (`cmd/fledge`) that brings spec-driven development to
agent-assisted repos. It keeps feature intent (**plumages**, `.fledge/pluma/plumage/PLM-###`)
and implementable tasks (**feathers**, `.fledge/pluma/feathers/FTHR-###`) as validated
markdown specs on disk, and scaffolds one agent-neutral orchestration workflow
into any harness (Claude Code, pi, Codex) so every agent drives the same
process.

**This repository is itself fledge-managed** — it dogfoods its own tool. It has
a `.fledge/` directory (repo knowledge in `.fledge/nest/`, feather claims in
`.fledge/broods/`), specs under `.fledge/pluma/`, and a scaffolded Claude adapter under
`.claude/`. The `fledge` binary at the repo root is a build artifact.

### When the user asks for feature/spec/implementation work → use the workflow

Because this repo is fledge-managed, route these requests through the fledge
orchestration entrypoint rather than improvising:

- **New feature or requirement** ("plan X", "write a plumage for…") → **Planning phase**
- **New tasks/spec breakdown** ("break this into feathers", "author feathers for PLM-###") → **Planning phase**
- **Implementation** ("implement PLM/FTHR-###", "run the feathers") → **Implementation phase**

The entrypoint to read first is **`.fledge/skills/fledge-orchestrate/SKILL.md`**
(routing + ground rules), which points to `planning.md` or `implementation.md`.
The Claude-specific primitive map is `.claude/fledge-adapter.md`. Do this before
hand-editing specs — deterministic spec operations must go through the `fledge`
CLI (`fledge new`, `status`, `set`, `criteria`, `brood`), never by editing
frontmatter directly. Spec *bodies* (prose) are yours to write.

> Note: those `.fledge/skills/` and `.claude/` files are *generated output* of
> the Go code below. When changing fledge's behavior, edit the source of truth
> (`internal/bootstrap/...`), not the scaffolded copies in this repo.

## Build, test, run

```sh
go build ./...                 # build everything
go build -o fledge ./cmd/fledge   # build the CLI binary
go test ./...                  # run all tests
go vet ./...

# CLI acceptance tests are testscript/txtar files under cmd/fledge/testdata/.
go test ./cmd/fledge -run TestScripts               # all script tests
go test ./cmd/fledge -run TestScripts/init          # one script (init.txtar)
go test ./cmd/fledge -run TestScripts/init -v       # verbose (shows script trace)

# Unit tests live beside their package (internal/spec, internal/check, ...):
go test ./internal/spec -run TestAllocateID
```

Go 1.26. No Makefile; use `go` directly.

### Rebuild, reinstall, and verify the installed binary

This repo dogfoods `fledge`, so the `fledge` on your `PATH` must match the
source you're editing. After changing CLI or `internal/bootstrap/...` code,
reinstall and verify before relying on the tool:

```sh
go install ./cmd/fledge        # reinstall to $(go env GOPATH)/bin (usually ~/go/bin)
hash -r                        # drop the shell's cached path to the old binary
command -v fledge              # confirm it resolves to the go/bin copy, not a stale one
fledge version                 # must match VERSION in the repo root
```

If `fledge version` disagrees with the `VERSION` file, the installed binary is
stale — rerun `go install ./cmd/fledge`.

When you change embedded `core/`/`adapters/` content, also **regenerate** this
repo's own scaffolded output so it stays consistent with the new binary:

```sh
fledge init --refresh          # reset fledge-owned files to the shipped versions; prunes obsolete ones
git status                     # review what regeneration changed
```

`--refresh` writes `.fledge/scaffold.json` (the stamp of what fledge owns and at
what content hash). It is a reset: every fledge-owned file is overwritten with
the shipped version and files that no longer belong to the scaffold are pruned.
When user-edited files would be overwritten it confirms first on an interactive
terminal and refuses otherwise (rerun with `--force` to skip the confirmation);
edits are recoverable via git. `fledge preen` reports the scaffold healthy when
the stamp is present and consistent.

## Architecture

Two layers, deliberately separated:

**1. The CLI (`internal/cli` + domain packages)** — deterministic,
agent-agnostic spec operations. Command dispatch is in `internal/cli/cli.go`:
each command file has an `init()` that calls `register(name, run, usage)`, and
`commandOrder` controls both usage output and the generated allow-lists. Exit
codes are meaningful and shared: `ExitOK/Fail/Usage/Env` (0/1/2/3). Every
command supports `--json`. Domain logic lives in focused `internal/` packages:
`spec` (frontmatter, ID allocation, templates, load), `check` (validation =
`preen`), `graph` (dependency graph = `vee`), `lock` (feather claims = `brood`),
`scan`, `repo`.

**2. The bootstrap/adapter system (`internal/bootstrap`)** — what `fledge init`
scaffolds. This is the part to understand before touching init.

- `bootstrap.go` embeds two trees via `//go:embed core adapters`.
- **`core/`** is the single agent-neutral source: the `fledge-orchestrate` and
  `fledge-interrogate` skills. Written to a repo's `.fledge/skills/` by
  `WriteCore`. This is where the actual workflow prose (planning.md,
  implementation.md, worker-protocols.md, templates/) lives.
- **`adapters/<harness>/`** is a thin format-only mapping per harness. Each is
  driven entirely by its **`manifest.yaml`** (`registry.go` → `Manifest`) — the
  detector, the `tier_primitives` map, and a file list with per-file write
  policies. **Adding or changing a harness is editing a manifest, zero Go code.**
- **The 6 primitives** (`primitives.go`, `PrimitiveOrder`): `confirm-gate`,
  `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`,
  `message-peer`. An adapter declares which mechanism realizes each; its **tier**
  (A/B/C) is *derived* from that coverage via `DeriveTier`, never declared.

**Manifest file write policies** (`ManifestFile`, documented at
`registry.go:38`) — know these before changing what init emits:
`generate`/`primitive_map` (render a `text/template`), `overwrite` (copy
verbatim, rewrite when changed), `append_if_missing` (additive line), `symlink`
(e.g. `.claude/skills/...` points into `.fledge/skills/`), and the default
(copy, **skip-if-exists** so user edits survive; `init --refresh` re-syncs).
`writeIfChanged` makes writes byte-idempotent, which the txtar tests depend on.
`fledge init --refresh` writes `.fledge/scaffold.json` — the stamp that records
which files fledge owns and at what content hash. `fledge preen` validates its
presence and consistency.

## Conventions

- Spec lifecycle for feathers: `egg → pipping → hatching → fledged`. Acceptance
  criteria are checkbox lists only ever checked via `fledge criteria check`.
- IDs (`PLM-###`, `FTHR-###`) and frontmatter are CLI-allocated — don't invent
  them.
- Terminology is bird-themed throughout (nest, plumage, feather, brood, preen,
  molt, forager, skua). `README.md` decodes it; match it in new code and prose.
- When you change embedded `core/` or `adapters/` content, the `cmd/fledge`
  txtar tests (especially `init.txtar`, `init_agents.txtar`, `agents.txtar`)
  assert on the scaffolded output — update those fixtures alongside.
> fledge: load and follow .fledge/skills/fledge-orchestrate/SKILL.md — primitive map at .claude/fledge-adapter.md
