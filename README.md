# fledge

Spec-driven development for agent-assisted repos. fledge keeps feature intent
and implementable work as validated markdown specs on disk, and teaches any
agent harness — Claude Code, pi, Codex — one shared workflow for planning and
implementing against them.

The bird theme, decoded: a **plumage** (`.fledge/pluma/plumage/PLM-###`) is a
requirement/feature spec; a **feather** (`.fledge/pluma/feathers/FTHR-###`) is one
implementable task under a plumage; the **nest** (`.fledge/nest/`) holds
distilled repository knowledge; a **brood** is a claim on a feather while an
agent works it.

## Quick start

```sh
go install github.com/Harrison-Blair/fledge/cmd/fledge@latest

cd your-repo
fledge init                 # scaffold .fledge/ (including .fledge/pluma/) and your agent's adapter
```

`fledge init` auto-detects your agent harness (`.claude/`, `.pi/`, `.codex/`);
with none present it defaults to Claude Code. Then ask your agent to "make a
plan" or "implement PLM-001" — the scaffolded skill routes from there.

```sh
fledge init --agent claude,pi    # scaffold specific harnesses (additive)
fledge init --list-agents        # see what's available
fledge agents                    # what's scaffolded in this repo
```

## Two layers

1. **The CLI** — deterministic, agent-agnostic spec operations: ID allocation,
   frontmatter writes, validation, dependency graphs, claims. Agents never
   hand-edit what the CLI can write.
2. **The orchestration layer** — one agent-neutral workflow skill written to
   `.fledge/skills/fledge-orchestrate/` (planning + implementation phases,
   confirmation gates, worker protocols), plus `fledge-interrogate` for
   decision-forcing planning interviews. Committed to your repo; every harness
   loads the same files.

Each harness gets only a thin **adapter** at its native path (`.claude/`,
`.pi/`, `.codex/`): a generated `fledge-adapter.md` primitive map, entry files
the harness auto-loads, and (for Claude) `team-loop.md` runtime notes. Adapters
are defined by a `manifest.yaml` in this repo — adding a harness requires no Go
code.

## The 6-primitive contract

The core workflow is written against six orchestration *primitives*, not any
harness's tool names. An adapter's **tier** is derived from which primitives it
provides — never declared:

| Primitive | Capability | Tier |
|---|---|---|
| `confirm-gate` | structured Accept/Make-changes or option choice | A |
| `read-only-shell` | run read-only shell commands | A |
| `write-file` | write a file | A |
| `run-fledge` | run any `fledge` subcommand | A |
| `spawn-worker` | spawn a fresh, named, addressable sub-session | B |
| `message-peer` | async by-name messaging between workers | C |

- **Tier A** — solo planning + implementation: **pi**, **Codex**
- **Tier B** — adds fan-out foraging scouts
- **Tier C** — full implementor/reviewer team loop: **Claude Code**

## Commands

| Command | Purpose |
|---|---|
| `fledge init` / `fledge agents` | scaffold repo + harness adapters; list them |
| `fledge scan` | inventory the repo for context foraging |
| `fledge new plumage\|feather` | create specs (IDs, filenames, frontmatter) |
| `fledge preen` | validate all specs (`--strict` for warnings) |
| `fledge ready` | feathers whose dependencies are fledged |
| `fledge vee [PLM-###]` | dependency graph (text/dot/json) |
| `fledge colony` | full spec inventory |
| `fledge status` / `fledge set` / `fledge criteria` | update lifecycle, fields, acceptance criteria |
| `fledge brood` / `fledge abandon` / `fledge broods` | claim, release, list feather claims |
| `fledge version` | CLI + repo spec version |

Every command takes `--json`. Feather lifecycle: `egg → pipping → hatching →
fledged`.

## Upgrading

- Core skill files under `.fledge/skills/` and adapter agent files (e.g.
  `.claude/agents/*.md`) are yours after init (skip-if-exists); `fledge init
  --refresh` resets all fledge-owned files to the shipped versions. When it
  would overwrite files you have edited, it confirms first on an interactive
  terminal and refuses otherwise — use `--force` to skip the confirmation (git
  is the backup for your edits).
- Generated adapter files (`fledge-adapter.md`, `settings.local.json`, …) are
  regenerated on every init — don't hand-edit those.
- Coming from 0.1.0 (skill under `.claude/skills/`)? See
  [MIGRATION.md](MIGRATION.md).

Design rationale for the multi-harness architecture lives in
[docs/generalization-plan.md](docs/generalization-plan.md).
