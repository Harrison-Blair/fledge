---
generated: 2026-07-17T17:48:26Z
commit: 1c9011d6e6a06f72f96bc98e3b2bd99c408ab79e
agent: fledge-forager
fledge_version: 0.6.10
---

# Architecture

Two deliberately separated layers — a deterministic spec CLI, and a bootstrap/adapter system that scaffolds one agent-neutral orchestration workflow into any harness — plus the workflow prose itself, which this repo also consumes on itself (dogfooding).

## Layer 1: the CLI (`internal/cli` + domain packages)

`cmd/fledge/main.go` is an 11-line wrapper that calls `cli.Run(os.Args[1:])` (cmd/fledge/main.go). All dispatch lives in `internal/cli/cli.go`: a `commandOrder` array (26 names) and a `commands` map populated by each command file's `init()` calling `register(name, runFunc, usage)` (internal/cli/cli.go). `internal/cli/command_parity_test.go` guards `commandOrder` and `commands` staying in sync.

Every command returns one of four meaningful exit codes: `ExitOK=0`, `ExitFail=1` (domain error — check findings, lock held, illegal transition, cycle), `ExitUsage=2`, `ExitEnv=3` (no git repo / no `.fledge/`), plus `ExitTimeout=4` for `fledge await` (internal/cli/cli.go, internal/cli/await.go). Every command supports `--json` via `emitJSON()` (internal/cli/*.go).

`internal/cli/specload.go`'s `loadSet()` is the shared entry point for commands needing repo + specs: it enforces `.fledge/` presence, reads locked feather IDs from `.brood` files, and returns `(repo, spec.Set, lockedIDs, exitCode, ok)`.

Domain logic lives in focused packages, each with a single clear responsibility (see modules.md for the full map): `spec` (frontmatter, ID allocation, templates, load — the PLM/FTHR file format), `check` (validation = `preen`), `graph` (dependency graph = `vee`), `lock` (feather claims = `brood`, files under `.fledge/broods/`), `ledger` (agent handoff records — status/verdict/escalation — under `.fledge/ledger/`), `nest` (context-doc scaffolding, the system that produced this very document set), `roster` (worker species-name allocation), `scan` (repo file enumeration = `fledge scan`), `repo` (git-root + `.fledge/` subdirectory path resolution).

## Layer 2: bootstrap/adapter system (`internal/bootstrap`)

What `fledge init` scaffolds. `internal/bootstrap/bootstrap.go` embeds two trees via `//go:embed core adapters` (internal/bootstrap/bootstrap.go).

- **`core/skills/`** is the single agent-neutral source of the orchestration workflow: `fledge-orchestrate` (SKILL.md routing + planning.md + implementation.md + foraging.md + incubator.md + brooder.md + skua.md + worker-protocols.md + templates/) and `fledge-interrogate` (SKILL.md). `WriteCore()` writes this tree to a consumer repo's `.fledge/skills/` (internal/bootstrap/registry.go).
- **`adapters/<harness>/`** (claude, codex, pi) are thin format-only mappings, each driven entirely by a `manifest.yaml`: detector, `tier_primitives` map, and a file list with per-file write policies (internal/bootstrap/registry.go — `LoadAdapters`, `FindAdapter`, `DetectAdapters`, `WriteAdapter`).

**The 6 primitives** (`internal/bootstrap/primitives.go` — `PrimitiveOrder`): `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`. An adapter declares which harness mechanism realizes each; its tier (A/B/C) is *derived* from that coverage via `DeriveTier()`, never declared. Tier A = all 4 base primitives (Codex, pi); Tier B = Tier A + `spawn-worker`; Tier C = all 6, including `message-peer` (Claude Code — full team-loop support).

**File write policies** (`ManifestFile`, internal/bootstrap/registry.go:38): `generate`/`primitive_map` (render a `text/template`), `overwrite` (copy verbatim, always rewritten), `append_if_missing` (additive line, e.g. CLAUDE.md/AGENTS.md pointers), `symlink` (e.g. `.claude/skills/...` → `.fledge/skills/`), and the default (copy, skip-if-exists — user edits survive; `init --refresh` re-syncs). `writeIfChanged()` makes writes byte-idempotent.

**Drift & stamp** (internal/bootstrap/drift.go, internal/bootstrap/stamp.go): `fledge init --refresh` writes `.fledge/scaffold.json` (the `Stamp` — fledge version + per-file `StampEntry` recording policy and content hash/symlink-target/append-lines). `DriftReport()` classifies each file as up-to-date/stale/modified/missing/obsolete by comparing on-disk state against the stamp and the expected manifest; `fledge preen` validates the stamp's presence and consistency. `EditedOnRefresh()` identifies user edits that a refresh would clobber, so `init --refresh` confirms before overwriting on an interactive terminal (refuses otherwise unless `--force`).

**Dev mode** (PLM-031): `fledge init --dev <path>` symlinks scaffold files into a source checkout instead of copying, so shipped prose can be edited live while dogfooding (`ExpectedFilesDev`, `ValidateDevSource`, ADR in stamp.go/registry.go). This repo, hearth, and stenographer are all dev-linked to `~/source/fledge`.

## Layer 3: the orchestration workflow itself (`core/skills/fledge-orchestrate/`)

Pure markdown protocol, no code — but architecturally central, since it defines every role and every state transition the CLI and adapters exist to serve:

- **Planning phase** (planning.md, §0–4): delegation decision → freshness gate (`.fledge/nest/index.md` stamp vs. HEAD) → context gathering (foraging.md) → plumage interrogation → feather interrogation, closing with a digest.
- **Foraging** (foraging.md): a **Commissioner** spawns a **Forager**, which fans out **Scout** workers (one per module, parallel) and synthesizes their raw reports into this eight-document `.fledge/nest/` set plus `index.md`. This document was itself produced by that pipeline.
- **Implementation phase** (implementation.md, §1–6): scope resolution → solo implementation (Tier A/B) or team-loop dispatch (Tier C, brooder+skua pairs) → escalation triage → end-of-run digest → resume recovery from `fledge broods --stale`.
- **Worker protocols** (worker-protocols.md, incubator.md, brooder.md, skua.md): shared discipline — ledger-driven handoffs (never inferred from message arrival), heartbeat-before-and-during, `fledge await` change-wait vs. exists-wait, exit-4 recovery via `fledge pulse`.

## Cross-module relationships

- `cli` commands are the only sanctioned way to mutate spec frontmatter/status/criteria — the workflow prose (bootstrap-core) repeatedly forbids hand-editing and routes every mutation through `fledge new/status/set/criteria/brood`.
- `cli`'s `nest.go` and `internal/nest` implement the exact `fledge nest scaffold/scout/stamp/status` commands the Forager/Scout protocol (bootstrap-core/foraging.md) depends on — this module's own pipeline is dogfooded by the CLI it documents.
- `internal/bootstrap`'s adapters reference `internal/cli`'s `commandOrder` when rendering `settings.local.json`'s permission allow-list (`{{.CommandOrder}}` placeholder, internal/bootstrap/adapters/claude/settings.local.json).
- `internal/ledger` underlies both the CLI's `heartbeat`/`await`/`verdict`/`escalate` commands and the workflow's entire worker-coordination model (status/verdict/escalation records, never message-body state).
- `internal/spec` types (`Requirement`, `Task`) are the runtime representation of the plumage/feather markdown format that `core/skills/fledge-orchestrate/templates/{plumage,feather}.md` document in prose.

## Open Questions

- The relationship between `.agents/skills/fledge-{orchestrate,interrogate}/` and `.fledge/skills/` is described as symlinks in CLAUDE.md's `.gitignore` section (dev-mode) but not confirmed for non-dev `fledge init`. (root-misc scout)
- `docs/google_ai_mode_response.md` and `docs/research_prompt.md` (multi-tier AI infrastructure: OpenCode, DeepSeek, local models) appear orthogonal to fledge's core spec-driven orchestration — exploratory research, or a planned feature? (root-misc scout)
