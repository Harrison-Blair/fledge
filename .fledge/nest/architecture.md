---
generated: 2026-07-16T21:27:15Z
commit: a1ed62a38540df7ab1cbdc4c486176a64a762018
agent: fledge-forager
fledge_version: 0.5.8
---

# Architecture

How fledge's two layers — the deterministic CLI and the agent-neutral orchestration/scaffold system — fit together, and how the repo's own `.fledge/` directory dogfoods that system.

## Two layers, deliberately separated

1. **The CLI** (`internal/cli` + domain packages) — deterministic, agent-agnostic spec operations. `cli.go:Run` dispatches subcommands via a registry (`internal/cli/cli.go`); each command file (`new.go`, `set.go`, `status.go`, `criteria.go`, `brood.go`, `nest.go`, `preen.go`, `vee.go`, `scan.go`, `ready.go`, `colony.go`, `unfledged.go`, `roster.go`, `agents.go`, `version.go`, `update.go`, `init.go`) registers itself in `init()`. Exit codes are shared and meaningful: `ExitOK=0`, `ExitFail=1`, `ExitUsage=2`, `ExitEnv=3` (`internal/cli/cli.go`). Domain logic lives in focused packages the CLI composes: `internal/spec` (frontmatter/ID/template/load), `internal/check` (validation = `preen`), `internal/graph` (dependency graph = `vee`), `internal/lock` (feather claims = `brood`), `internal/scan` (module discovery), `internal/repo` (repo-root/dir resolution), `internal/roster` (worker species allocation), `internal/nest` (context-doc scaffold/scout/status).
2. **The bootstrap/adapter system** (`internal/bootstrap`) — what `fledge init` scaffolds into a target repo. `bootstrap.go` embeds two trees via `//go:embed core adapters` into a single `FS`. `core/skills/{fledge-orchestrate,fledge-interrogate}/*` is the single agent-neutral source of the orchestration workflow (`internal/bootstrap/core/skills/fledge-orchestrate/{SKILL,planning,implementation,foraging,incubator,brooder,skua,worker-protocols}.md` + `templates/`), written verbatim into a target repo's `.fledge/skills/` by `WriteCore` (`internal/bootstrap/registry.go`). `adapters/<harness>/` (claude, codex, pi) is a thin format-only mapping, each driven entirely by its `manifest.yaml` (`internal/bootstrap/registry.go:Manifest`) — detector, `tier_primitives` map, and a `files` list with per-file write policies. Adding a harness is editing a manifest, not writing Go.

`cmd/fledge/main.go` is an 11-line wrapper: `main()` calls `cli.Run(os.Args[1:])` and exits with its status code. All logic lives below `internal/`.

## The 6 primitives and tier derivation

Six fixed orchestration primitives (`internal/bootstrap/primitives.go:PrimitiveOrder`): `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`. Each adapter's `manifest.yaml` declares, per primitive, the harness-native mechanism that realizes it (e.g. Claude: `confirm-gate`→AskUserQuestion, `spawn-worker`→teammate-spawn, `message-peer`→SendMessage; Codex/pi: only the first 4, no team primitives). An adapter's **tier** (A/B/C) is *derived* from that coverage by `Manifest.Tier()` (`internal/bootstrap/primitives.go` DeriveTier logic), never declared directly — Tier A = 4 primitives (solo), Tier B = +`spawn-worker` (foraging with scouts), Tier C = +`message-peer` (full team loop with brooder/skua pairs). Claude Code is Tier C; Codex and pi are Tier A (`internal/bootstrap/adapters/{codex,pi}/manifest.yaml`).

## Scaffold write policies and drift

`registry.go` implements 6 file write policies per `ManifestFile` entry: default (copy, skip-if-exists — user edits survive), `overwrite` (always rewrite, e.g. `team-loop.md`, `settings.json`), `generate`/`primitive_map` (render via `text/template`, e.g. `fledge-adapter.md`, `settings.local.json`), `append_if_missing` (additive line, e.g. `AGENTS.md`/`CLAUDE.md` pointer), `symlink` (e.g. `.claude/skills/fledge-orchestrate` → `.fledge/skills/fledge-orchestrate`). `drift.go` compares on-disk state to the expected manifest (SHA256 content hash, symlink targets, append-lines) and classifies each file as up-to-date/stale/modified/missing/obsolete. `stamp.go` persists this expected-file manifest as `.fledge/scaffold.json` (`Stamp{FledgeVersion, Agents, Files map[string]StampEntry}`), written by `fledge init --refresh`; `fledge preen` validates the stamp is present and consistent (`internal/cli/preen.go`, calling into `drift.go`).

## Orchestration workflow: planning → foraging → implementation

The `core/skills/fledge-orchestrate` prose (mirrored live in this repo at `.fledge/skills/fledge-orchestrate/`) defines three phases an agent routes into based on user request (`SKILL.md`):
- **Planning** (`planning.md`) — plumage interrogation, foraging trigger, feather decomposition into tracer bullets, spec creation via `fledge new`, gated review, close-out digest. Can run inline (orchestrator does it) or delegated to a spawned `fledge-incubator-<species>` worker.
- **Foraging** (`foraging.md`) — the protocol this forager itself is executing: scan → plan scout split → `fledge nest scaffold` → fan out `fledge-context-scout` workers per module → synthesize 8 concern docs + `index.md` → verify via `fledge nest status`.
- **Implementation** (`implementation.md`) — dispatch logic branches on harness tier: Tier A solo, Tier B foraging+solo, Tier C team loop with `fledge-brooder-<species>`/`fledge-skua-<species>` pairs spawned per feather, test-first evidence recorded to `.fledge/molt/FTHR-###.md`, review gate, green teardown (merge, suite run, `fledge abandon --fledged`, worktree/lock cleanup).

Worker roles (`incubator.md`, `brooder.md`, `skua.md`, `worker-protocols.md`) are one-shot: spawned with a fully self-contained prompt (no inherited conversation history), communicate only with their commissioning orchestrator/peer, and end on an explicit final message — never bare idle.

## Cross-module relationships

- `cmd/fledge/main.go` → `internal/cli.Run` → per-command files → domain packages (`spec`, `check`, `graph`, `lock`, `scan`, `repo`, `roster`, `nest`, `bootstrap`).
- `internal/cli/init.go` → `internal/bootstrap.{LoadAdapters, WriteCore, Manifest.WriteAdapter}` → embedded `core/`/`adapters/` trees.
- `internal/cli/nest.go` → `internal/nest.{ClearNest, ScoutBody, ConcernBody, IndexBody, Status, RefreshDoc}` — implements exactly the `fledge nest scaffold|scout|status|stamp` commands this forager pipeline depends on.
- `internal/cli/preen.go` → `internal/check.Run` (spec validation) + `internal/bootstrap` drift detection (scaffold health).
- `internal/cli/brood.go` → `internal/lock.{Acquire,Release,List}` (feather claim files under `.fledge/broods/`).
- `internal/cli/set.go`/`status.go` → `internal/graph` (cycle detection before accepting `depends_on` mutations).
- `internal/cli/scan.go` → `internal/scan.Run` — produces exactly the module/file-list JSON this forager consumed as its authoritative work list.
- Scaffolded `core/` prose (`internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` etc.) is the literal source this forager is following; the copy at this repo's own `.fledge/skills/fledge-orchestrate/foraging.md` is scaffolded output of that source — changes to fledge's own behavior are made in `internal/bootstrap/core/...`, never by hand-editing the scaffolded copy (CLAUDE.md).

## Open Questions

- How does `text/template` rendering in `fledge-adapter.md`/`settings.local.json` precisely work — what struct is `.` in the template context, and which fields are populated? (`internal/bootstrap/adapters/*` scout)
- The `piping_file` manifest field appears only on the Claude adapter — are Codex/pi team loops unsupported, or handled differently? (`internal/bootstrap/adapters/*` scout)
- Current implementation status of `docs/generalization-plan.md` milestones M0–M5 (authored ~2026-01, "locked design" status; progress since unclear) — and whether the `spawn-pool` primitive removal (amendment 2026-07) is fully reconciled with that doc, which still references a "persistent skua pool" sizing formula.
