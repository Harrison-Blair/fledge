---
generated: 2026-07-15T18:14:39Z
commit: 5728c29953a7c218c923ce20333dbffebb00623f
agent: fledge-forager
fledge_version: 0.5.4
---

# Architecture

How fledge's two layers — the deterministic CLI and the bootstrap/adapter scaffolding system — fit together, and how the agent-neutral orchestration workflow they scaffold actually runs across harnesses.

## Two-layer split

1. **CLI (`internal/cli` + domain packages)** — deterministic, agent-agnostic spec operations. Dispatch lives in `internal/cli/cli.go`: each command file's `init()` calls `register(name, run, usage)`; `commandOrder` (cli.go:105-109) controls usage output and generated allow-lists, and is asserted equal to the `commands` map by `internal/cli/command_parity_test.go:TestCommandOrderMatchesRegistrations`. Every command supports `--json` and returns one of four exit codes (`ExitOK/Fail/Usage/Env` = 0/1/2/3). Command files call into focused domain packages — `internal/spec`, `internal/check`, `internal/graph`, `internal/lock`, `internal/scan`, `internal/repo`, `internal/nest` — which never import `internal/cli` (one-way dependency).
2. **Bootstrap/adapter system (`internal/bootstrap`)** — what `fledge init` scaffolds into a target repo. `bootstrap.go` embeds two trees via `//go:embed core adapters`. `core/skills/` is the single agent-neutral source of the orchestration workflow (`fledge-orchestrate`, `fledge-interrogate`); `adapters/<harness>/` are thin, manifest-driven, format-only mappings per harness (claude, codex, pi). Each adapter's `manifest.yaml` declares a detector, a `tier_primitives` map, and a file list with write policies (`internal/bootstrap/registry.go`).

## The 6-primitive contract and tier derivation

`internal/bootstrap/primitives.go` defines `PrimitiveOrder`: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`. `TierPrimitives` maps tier letter → required primitive set (A = first 4, B = A + spawn-worker, C = B + message-peer). `DeriveTier(provided map[string]bool) string` derives a harness's tier purely from which primitives its adapter declares — tier is never hand-declared in a manifest. Adding a harness is editing `adapters/<harness>/manifest.yaml`, zero Go code (CLAUDE.md).

- **Claude** (`internal/bootstrap/adapters/claude/manifest.yaml`) provides all 6 → Tier C (team loop with ephemeral teammates).
- **Codex** and **Pi** each provide 4 of 6 → Tier A (solo, in-session phases; no spawn-worker/message-peer, so no forager/incubator delegation and no brooder/skua pairs — foraging and implementation run inline in the single session, per `planning.md` §0/§2 and `implementation.md` §2).

## Scaffold write policies and drift

`registry.go`'s `ManifestFile` encodes six write policies (`registry.go:38`): `generate`/`primitive_map` (render a `text/template`), `overwrite` (copy verbatim, always repaired), `append_if_missing` (additive line, never clobbered), `symlink` (e.g. Claude's `.claude/skills/fledge-orchestrate` → `../../.fledge/skills/fledge-orchestrate`, not copied), and the default (copy, **skip-if-exists**, so user prose edits to skill files survive reruns). `writeIfChanged` makes writes byte-idempotent — identical content is reported "skipped" even under `--refresh`.

`fledge init --refresh` resets every fledge-owned file to the shipped version and prunes obsolete ones, writing `.fledge/scaffold.json` (`stamp.go`: `Stamp{FledgeVersion, Agents, Files map[string]StampEntry}`) as the record of what fledge owns and at what content hash/symlink-target/append-lines. `drift.go` compares on-disk state to this stamp and to `ExpectedFiles()` (the state `WriteCore`/`WriteAdapter` would produce), classifying each path as `StatusUpToDate`, `StatusStale` (embedded content moved on, refresh-safe), `StatusModified` (user-edited, refresh would clobber), `StatusMissing`, or `StatusObsolete` (no longer shipped). `fledge preen` surfaces this via `DriftReport`; `EditedOnRefresh` is what `init --refresh` consults before its confirm-or-`--force` gate.

**This repo dogfoods itself**: after any change to `internal/bootstrap/core/` or `internal/bootstrap/adapters/`, rebuild (`go install ./cmd/fledge`), then `fledge init --refresh` to resync this repo's own `.fledge/skills/`, `.claude/`, etc., and update the `cmd/fledge/testdata/*.txtar` fixtures that assert on scaffolded output (`init.txtar`, `init_agents.txtar`, `agents.txtar`) — CLAUDE.md.

## The orchestration workflow (agent-neutral prose)

`core/skills/fledge-orchestrate/SKILL.md` routes every request to either the **planning phase** (`planning.md`: §0 who runs it, §1 freshness gate, §2 gather context via foraging, §3 plumage interrogation, §4 feather interrogation) or the **implementation phase** (`implementation.md`: §1 resolve scope, §2 solo tier A/B, §3 team-loop tier C, §4 escalations, §5 end of run, §6 recovery after resume).

Delegation is primitive-gated: planning delegates to an **incubator** worker only when both `spawn-worker` and `message-peer` are available (Tier C); otherwise the orchestrator performs planning inline (Tier A/B). Implementation dispatches **brooder+skua** pairs only under Tier C team-loop (`implementation.md` §3); Tier A/B runs solo (`implementation.md` §2).

Worker roles and their protocols are centralized in one file — `core/skills/fledge-orchestrate/worker-protocols.md` — covering Incubator, Brooder, and Skua. This is the single source both `fledge-forager`/`fledge-context-scout` (foraging.md) and per-harness agent definitions (e.g. `internal/bootstrap/adapters/claude/agents/fledge-skua.md`) point back to, so behavior stays harness-neutral; only the *mechanism* realizing each primitive differs per adapter.

### Skua review protocol (worker-protocols.md § Skua)

The skua reviews one feather across cycles, never writes code or merges. Its **`### Reviewing a feather`** checklist covers: tests pass now, tests failed first (AC-1 evidence), diff vs. spec, scope and simplicity, and a criteria audit. Its **`### Verdict`** behavior: findings are listed to the brooder; a third consecutive rejection triggers escalation (not another silent revise cycle); on pass, the skua checks AC boxes via `fledge criteria check` in the worktree (an audit-trail commit) and sends the pass message to the **orchestrator**, not just the brooder. Lifecycle: the skua stays alive after passing until the merge is verified, then expects a graceful shutdown request from the orchestrator (or force-termination if it's slow to exit).

The Claude-specific realization of this role is `internal/bootstrap/adapters/claude/agents/fledge-skua.md` — a system-prompt file (frontmatter: name, description, model=sonnet, tools, no fixed color) whose body says "You are a fledge skua" and references `worker-protocols.md § Skua` rather than restating the checklist, plus a "Claude-runtime specifics" section mapping the protocol onto Claude's SendMessage/TaskStop/Task primitives.

### Team-loop mechanics (Claude Tier C)

`internal/bootstrap/adapters/claude/team-loop.md` is Claude's *piping file* (declared via `manifest.yaml`'s `piping_file` field, referenced from the generated `fledge-adapter.md` primitive-map template) — Claude-only procedural detail that doesn't belong in agent-neutral core prose. Its 8 sections cover: teammate display (tmux), orchestrator naming (must be `team-lead`, the harness-registered fixed name — not the audit tag `fledge-orchestrator`), spawning (penguin-species scheme from `implementation.md` §3.1), shutdown (SendMessage graceful request is necessary but not sufficient — `TaskStop` is what actually terminates; confirmed shutdown = roster absence + pane close), planning delegation (relay GATE/QUESTION verbatim via AskUserQuestion), the team task list (single writer, workers never touch), skill loading (symlinks, not copies), and recovery after resume.

**§ Teammate display (tmux)**: precondition is `test -n "$TMUX"`. `implementation.md` §1 references this precondition as part of Tier C's harness-piping checks when resolving scope at the start of a run — if the precondition fails, team-loop.md specifies falling back to in-process (no split-pane display) rather than blocking, and `implementation.md` §1 gates on a confirm-gate in that case. This is the area flagged for the tmux-auto-default planning effort: today the fallback path involves a confirm-gate; whether that becomes an unprompted auto-default is exactly what's under review. See `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` §1 and `internal/bootstrap/adapters/claude/team-loop.md` § Teammate display (tmux) directly before making changes.

## Cross-module relationships

- `internal/cli/init.go`, `internal/cli/agents.go`, `internal/cli/preen.go` are the CLI surface over `internal/bootstrap`'s Go API (`LoadAdapters`, `WriteCore`, `Manifest.WriteAdapter`, `DriftReport`, `EditedOnRefresh`).
- `internal/cli/nest.go` is the CLI surface over `internal/nest` (Doc/Concern/Scout types, `ClearNest`, template stub bodies) — this is what `fledge nest scaffold`, `fledge nest scout --module`, and `fledge nest stamp` actually run.
- `docs/generalization-plan.md` is a **prior, superseded design document** for an earlier (7-primitive, `spawn-pool`-inclusive) version of this same bootstrap/adapter architecture; a 2026-07 amendment at its top says `spawn-pool` was dropped from the primitives contract. Current code (`primitives.go`) has 6 primitives, no `spawn-pool` — treat `internal/bootstrap/primitives.go` and `registry.go` as authoritative over `docs/generalization-plan.md` where they conflict.

## Open Questions

- Does manifest-parsing validate that all 6 primitives are mapped in an adapter's `tier_primitives` before `DeriveTier` runs, or is an incomplete map silently under-derived? (from internal-bootstrap-adapters scout)
- Whether `docs/generalization-plan.md` §2.1/§5.3 (persistent skua pool, `spawn-pool`) have been formally superseded in the doc body itself, or only via the top-of-file amendment note, is unresolved — the doc itself says "Next step: turn this into a plumage… or start implementing M0," so its current status relative to shipped code is unclear (from docs scout).
