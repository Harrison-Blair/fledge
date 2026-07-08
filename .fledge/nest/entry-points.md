---
generated: 2026-07-08T05:28:12Z
commit: e46c481a047d45ef10bcd79a3326d47932b32868
agent: fledge-forager
fledge_version: 0.2.1
---

# Entry Points

Where execution and agents enter the system: the CLI command surface, the process entry, and the files an agent harness auto-loads.

## Process entry

`cmd/fledge/main.go:main()` → `cli.Run(os.Args[1:])` (returns the exit code). `Run` dispatches `args[0]` to a registered command, prints usage on unknown/empty.

## CLI command surface (`commandOrder`)

`init`, `agents`, `scan`, `new`, `nest`, `preen`, `ready`, `vee`, `colony`, `unfledged`, `status`, `set`, `criteria`, `brood`, `abandon`, `broods`, `version`. All support `--json`. Grouped by role:

- **Scaffolding:** `init [--agent <name>] [--refresh] [--list-agents]` (detects harness, writes core skills + adapter, ensures `.gitignore`/`.fledgeignore`), `agents` (lists adapters with derived tier + scaffolded status).
- **Spec creation/mutation:** `new plumage|feather --title … [--plumage] [--depends-on] [--priority] [--oversight]`, `status <ID> [<new>] [--force]`, `set <ID> <field> <value>` (priority/oversight/depends_on/title; cycle-checked), `criteria <ID> [check|uncheck] <AC-N>`.
- **Validation & triage:** `preen`/`check [--strict]` (validation errors/warnings — bad frontmatter, dangling refs, unchecked fledged criteria, missing evidence), `colony` (repo-wide report: counts by status, per-plumage completion, **orphan feathers** with unresolved plumage, **dangling refs**, blocked feathers, active broods, degraded-data issues), `unfledged [--plumage|--feathers]` (all non-fledged items, priority-then-ID).
- **Graph & readiness:** `vee [--format text|dot|json] [PLM-###]` (dependency waves, cycle detection, subgraph filter), `ready`/`pipping` (dispatchable-now: pipping, deps fledged, no brood held).
- **Claims:** `brood <FTHR> --owner <name> [--branch]` (acquire lock + set `hatching`), `abandon <FTHR> [--fledged] [--force]`, `broods` (held locks + PID liveness).
- **Knowledge:** `nest {scaffold|new <doc>|scout --module <m>|stamp <file>}` (deterministic nest-doc creation/frontmatter stamping), `scan` (module inventory), `version`.

### Triage-relevant detail (for orphan/evidence work)

Integrity surfacing today is split: `preen` returns hard validation errors; `colony` reports orphans/dangling-refs/degraded-data but **exits 0** (it is an observer, leaving enforcement to `preen`). Neither currently detects an **orphaned evidence file** in `.fledge/molt/` whose feather was renamed/removed, nor evidence *missing* for a fledged feather except via the checked-criteria rule in `check`. `check.Run()` already receives `repo.EvidenceDir()`, so evidence-orphan detection is a natural extension of the `check`/`colony` surface.

## Agent-loaded entry (harness discovery)

An agent working in a fledge repo is meant to enter through scaffolded files:
- `.claude/` adapter — `fledge-adapter.md` (primitive map), `settings.local.json` (generated `Bash(fledge …)` allow-list), `settings.json` (`skills` pointer), `team-loop.md` (runtime notes), `agents/*.md` (fledge-brooder/skua/forager/context-scout entry files).
- `.claude/skills/fledge-orchestrate` → symlink into `.fledge/skills/fledge-orchestrate/` (`SKILL.md` routing → `planning.md` / `implementation.md`).
- Root `CLAUDE.md` (this repo) tells the agent to route feature/spec/impl requests through the orchestration skill.

How reliably an agent *notices* these on entry — vs improvising — is exactly the agent-detection/legibility concern (feedback items 1–2).
