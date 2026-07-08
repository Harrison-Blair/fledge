# fledge generalization plan — any-agent orchestration

Status: **locked design** — 23 decisions resolved through interrogation. Ready to become a plumage.
Target: fledge 0.1.0 → 0.2.0.
Author: planning session with pi (interrogation transcript preserved inline as Q1–Q23 below).

## 0. Thesis

fledge is two layers:

1. **The `fledge` CLI** (`cmd/fledge`, `internal/*` except `internal/bootstrap/`) — already fully agent-agnostic. It manages specs on disk (`.fledge/`, `pluma/`), allocates IDs, validates, locks, renders JSON. Zero agent references outside `init.go`'s hardcoded `.claude` destination.
2. **The orchestration layer** (`internal/bootstrap/`, scaffolded by `fledge init`) — currently Claude-Code-only. It tells an agent *how to drive* the CLI.

Generalizing means making layer 2 portable across agent harnesses **without forking the workflow logic per harness**. The linchpin: the **Agent Skills standard** (`SKILL.md` + sibling files), which Claude Code, Codex, and pi all load natively; fledge's existing skill already conforms to it.

**Decision (Q1): Core + adapters.** One agent-neutral workflow lives in a single `core/` skill that conforms to the standard; every harness loads *that same* skill; each harness gets only a thin adapter for the parts the standard doesn't cover. One source of workflow truth, no drift. This is the *only* option where "general piping, move to another target" (the user's stated priority) is literally true — adding/moving to a harness is editing a manifest, not Go code, not workflow prose.

The CLI stays untouched in behavior; only `init`, `agents` (new), and `internal/bootstrap` change.

## 1. Locked design decisions (traceability)

| # | Decision | Choice |
|---|---|---|
| Q1 | Generalization model | Core + adapters (one neutral `core/` skill, thin per-harness adapters) |
| Q2 | Where core lives in target repo | `.fledge/skills/` (fledge-owned, committed, scan-ignored); every adapter points at it |
| Q3 | Which tiers core owns | All three (A/B/C), branched in `implementation.md`; adapter files for harness piping specifics |
| Q4 | Primitive set | Includes `spawn-pool` as first-class (later folded to 7 via Q16) |
| Q5 | How tier is determined | Primitive coverage is the contract; tier is *derived*, never declared |
| Q6 | Adapter build mechanism | Manifest-driven; entry files generated from manifest at build time; zero Go per new adapter |
| Q7 | `fledge init` trigger | Auto-detect via marker existence; nothing detected → `claude` default + hint; `--agent` overrides/adds |
| Q8 | Re-init upgrade policy | Core skip-if-exists + `--upgrade-core` (trust-git + loud message); adapters always overwrite |
| Q9 | Migration of old repos | No migration tooling; `MIGRATION.md` documents the moves; default init stays purely additive |
| Q10 | Duplicate-skill guard | `fledge init` refuses to create a duplicate (same-name skill in sibling location) |
| Q11 | `confirm-gate` semantics | Two modes (review, decision); refusal pauses cleanly, spec state untouched; **fixes authoring-ordering wart** (gate-before-commit) |
| Q12 | `spawn-worker` semantics | Fresh, context-free, self-contained prompt; named, addressable, killable, may idle; returns one final message |
| Q13 | `spawn-pool` semantics | Core owns pool logic fully (sizing, round-robin, naming, reuse); adapter supplies only "keep N workers alive + route messages" |
| Q14 | `message-peer` semantics | Asynchronous, by-name; orchestrator-mediated topology (topology is an *instructed rule*, not the primitive) |
| Q15 | Constraint model | Primitives are *capability contracts about what the worker may attempt*, not enforced sandboxes; role constraints are instructed rules |
| Q16 | `edit-spec` primitive | Folded into `run-fledge`; contract is **7 primitives**; "never hand-edit" is a prominent instructed rule |
| Q17 | Feather lifecycle ownership | CLI owns 4 durable states (`egg\|pipping\|hatching\|fledged`); core owns runtime sub-states as orchestrator bookkeeping, reconstructed on resume |
| Q18 | `interrogate` dependency | Ship in `core/` as `fledge-interrogate` (namespaced to avoid collision); scaffolded by every init |
| Q19 | Team-loop prose boundary | core=logic, adapter map=mechanism, adapter piping file=harness runtime behavior |
| Q20 | Tier C degradation | Runs if 3 primitives truthfully provided; piping (tmux, `/resume`, permissions) degraded-optional with stated fallbacks |
| Q21 | CLI agent-awareness | CLI stays purely file-managing; `init`/`agents` only scaffold; no "current agent" concept |
| Q22 | First release scope | 0.2.0 = Claude + pi + Codex at Tier A; Cursor + opencode in 0.3.0; M4 (pi Tier C) deferred; no M4 stub |
| Q22b | M0 exit criterion | Behavioral identity (files present, exit codes, JSON shape + `agents` field), *not* byte-identity |
| Q23 | Claude permission allow-list | Generated from `cli.commandOrder` at build time into the manifest template |

Full interrogation transcript (one question at a time, recommended answers) is preserved in §13.

## 2. Architecture

```
internal/bootstrap/
  bootstrap.go                      # //go:embed core adapters
  core/                             # agent-neutral — single workflow source
    skills/
      fledge-orchestrate/
        SKILL.md                     # routing + ground rules, neutralized
        planning.md                  # neutral; capability-conditional prose
        implementation.md            # Tier A default + Tier B/C capability-conditional branches
        templates/
          context-doc.md
          feather.md
          plumage.md
          scout-report.md
      fledge-interrogate/
        SKILL.md                     # namespaced interrogation skill
  adapters/                         # per-harness; format-only, no workflow logic
    claude/
      manifest.yaml                  # detector, files map, 7-row primitive coverage, entry template
      agents/                        # fledge-brooder/forager/context-scout/skua.md (slimmed prose)
      settings.json                  # teammateMode + env + skills pointer
      team-loop.md                   # Claude piping: tmux, /resume, permission inheritance, task list
      README.md                      # primitive map (the 7 rows → Claude mechanism)
    pi/
      manifest.yaml
      settings.json                  # skills pointer
      prompts/{fledge-plan,fledge-implement}.md
      README.md
    codex/
      manifest.yaml
      AGENTS.md                      # pointer + skills-dir
      README.md
    cursor/                          # 0.3.0
      manifest.yaml
      rules/fledge.mdc
      README.md
    opencode/                        # 0.3.0
      manifest.yaml
      opencode.agent.json            # verified layout TBD
      README.md
  registry.go                       # generic manifest reader/walker (no per-adapter Go)
```

### 2.1 The 7-primitive contract (Q4–Q16)

| Primitive | Capability (what the worker may attempt) | Claude mechanism | pi mechanism | Tier required for |
|---|---|---|---|---|
| `confirm-gate` | present material, get a structured Accept/Make-changes or option choice | `AskUserQuestion` | `fledge_gate` tool / chat | A |
| `read-only-shell` | run read-only shell commands | `Bash` (ro) | `bash` tool | A |
| `write-file` | write a file | `Write` | write tool | A |
| `run-fledge` | run any `fledge` CLI subcommand (incl. all spec mutation) | `Bash(fledge …)` | `bash fledge …` | A |
| `spawn-worker` | spawn a fresh, context-free, named, addressable sub-session | teammate spawn | `fledge_spawn` (SDK) / sequential | B (foraging), C (brooders) |
| `spawn-pool` | keep N named workers alive and addressable across requests | persistent teammates | persistent SDK sub-sessions | C (skua pool) |
| `message-peer` | send an async by-name message; sender may idle, woken on reply | `SendMessage` | orchestrator relay | C (fix loop) |

**Instructed rules (not primitives, stated in `core/` prose at point of use):**
- "Never hand-edit spec frontmatter the CLI can write" — formerly `edit-spec`, folded into `run-fledge` (Q16).
- Role-specific shell constraints: scouts read-only; foragers write-confined to `.fledge/nest/`; brooders work only in their worktree. Defense-in-depth via instruction; real safety backstop lives in CLI + git + locks (Q15).
- Communication topology (brooder↔skua↔orchestrator only) is an instructed rule, not the `message-peer` primitive (Q13/Q14).
- Team task list / roster is orchestrator bookkeeping, not a primitive (Q4).

### 2.2 Tier derivation (Q5)

The tier is **not declared**; it falls out of which primitives the adapter provides:

- **Tier A (solo):** `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge` (4)
- **+Tier B (fan-out foraging):** adds `spawn-worker` (5)
- **+Tier C (team loop):** adds `spawn-pool`, `message-peer` (7)

An adapter declaring `spawn-worker` but not `spawn-pool`/`message-peer` is automatically "fan-out foraging + solo implementation" — exactly Codex's profile, no awkward labeling. The §8 coverage test checks every primitive the `core/` prose uses appears in the adapter's map.

## 3. Adapter format — the manifest (Q6)

Each adapter directory carries a `manifest.yaml` — the single source of truth, build-time-only (stays in the binary, never clutters target repo):

```yaml
name: claude
detector:                           # how init auto-senses this harness
  exists: .claude/
tier_primitives:                     # the 7-row coverage → derives tier
  confirm-gate: AskUserQuestion
  read-only-shell: Bash
  write-file: Write
  run-fledge: Bash(fledge ...)
  spawn-worker: teammate-spawn
  spawn-pool: persistent-teammate
  message-peer: SendMessage
files:                               # source (embed) → target (repo) path map
  - src: agents/fledge-brooder.md
    dst: .claude/agents/fledge-brooder.md
  - src: settings.json
    dst: .claude/settings.json
    generate: true                    # template-fed from manifest at build
entry:                              # the native file the harness auto-loads
  - dst: .claude/settings.json
    inline_map: true                  # primitive coverage inlined here
piping_file: team-loop.md            # harness runtime behavior (Q19), optional
```

**Generation rules:**
- `generate: true` files are produced from a template fed by the manifest (e.g., Claude `settings.local.json` allow-list generated from `cli.commandOrder` — Q23).
- The `entry` file's primitive map is generated/inlined from `tier_primitives` so the agent reads its map from the file its harness *already loads* — no second lookup. The manifest is the single source; the inlined copy is derived; the §8 test verifies they match.
- **Zero Go code per new adapter** — `registry.go` reads manifests generically. Adding/moving to a new harness = adding/editing a `manifest.yaml`.

## 4. `fledge init` behavior (Q7–Q10, Q21–Q23)

```
fledge init [--agent <name>]... [--upgrade-core] [--json]    # repeatable
fledge init --agent claude,pi                                 # comma form
fledge init --list-agents                                      # enumerate + exit 0
fledge agents [--json]                                         # adapters + which scaffolded in this repo
```

### 4.1 Detection & default (Q7)
- **No `--agent`:** scan repo root for each adapter's `detector` (`.claude/`, `.pi/`, `.cursor/`, `AGENTS.md`, `opencode.json`); scaffold exactly the harnesses found. If none detected, default to `claude` and print a hint.
- `--agent <name>` overrides/adds; re-running `fledge init --agent cursor` on an initialized repo *adds* the cursor adapter without touching existing files (additive invariant).
- **Q21:** `init`/`agents` are the *only* agent-aware commands; they only scaffold, never depend on a harness at runtime. Every other command is identically agent-agnostic.

### 4.2 Upgrade policy (Q8)
- **Core skill** (`.fledge/skills/fledge-orchestrate/`, `fledge-interrogate/`): **skip-if-exists** by default; `--upgrade-core` overwrites with a loud "overwrote N core files; `git diff` to review — your edits are recoverable" message (trust-git, no backup machinery; `.fledge/skills/` is committed so git is the backup).
- **Adapter entry files** (`.claude/settings.json`, `.pi/settings.json`, etc.): **always overwrite** — they're generated from the manifest; a new primitive must regenerate the inlined map. (Forced by Q6.)

### 4.3 Duplicate guard (Q10)
- Before writing `.fledge/skills/<skill>/`, check all adapter native paths for a same-name skill (e.g., existing `.claude/skills/fledge-orchestrate/`). If found, **refuse** with "remove the old copy at X first; see MIGRATION.md." A few lines in `writeBootstrapFiles`'s successor; no scope change to `preen`.
- Stance (Q9 + Q10): fledge won't move your files, but it won't let you create a broken state.

### 4.4 Migration (Q9)
- **No migration tooling.** `MIGRATION.md` documents the manual `git mv` + pointer edits for 0.1.0 → 0.2.0 repos.
- Default `init` stays **purely additive**: on any repo, new or old, bare `fledge init` only adds missing files / overwrites generated adapters — never destroys or moves existing user files.

### 4.5 JSON output
- `--json` gains an `agents` array (which adapters were scaffolded) alongside `created`/`skipped`.

### 4.6 Claude allow-list generation (Q23)
- The Claude adapter's `settings.local.json` `Bash(fledge <cmd> *)` allow-list is **generated from `cli.commandOrder`** at build time. Source of truth: `cli.go` (`init, scan, new, preen, ready, vee, colony, status, set, criteria, brood, abandon, broods, version`). Eliminates the drift already present in today's hand-maintained list (which lists retired `check/graph/lock/unlock/locks` and omits current `preen/vee/colony/criteria/brood/abandon/broods`).

## 5. Orchestration docs — `core/` authoring rules

### 5.1 Capability-conditional prose (Q5)
`core/` prose is written to the 7 primitives and branches on **declared capability**, not tier labels:
- "Run a `confirm-gate`" — adapter map says how.
- "If you provide `spawn-worker`, you may fan out foraging scouts…"
- "If you also provide `spawn-pool` and `message-peer`, you may run the team loop; follow `implementation.md` §team."

### 5.2 Three-way prose split for the team loop (Q19)
- **Logic** (brooder/skua roles, fix loop, merge gating, pool sizing, recovery *steps*) → `core/implementation.md` — one shared copy, no drift.
- **Mechanism** (per-primitive: "spawn-worker = Claude teammate spawn") → adapter's primitive map (README).
- **Piping** (tmux display, `/resume` recovery, permission inheritance, team task list) → adapter's `team-loop.md`. Small, stable; changes with harness, not workflow. Core's team section: "for each primitive's mechanism, see your adapter's map; for harness runtime behavior, see your adapter's team-loop file."

### 5.3 Pool logic in core (Q13)
`core/implementation.md` specifies pool logic fully: size = `ceil(active brooders / 3)`, min 1; round-robin assignment; reuse-on-shutdown; the penguin-species naming scheme (18 species, suffix on overflow). Adapter supplies only "keep N named workers alive and addressable; route a message to worker X."

### 5.4 `confirm-gate` semantics (Q11)
- Two modes: **review** (Accept / Make changes, loop until Accept) and **decision** (choose between concrete options).
- **Refusal pauses the phase cleanly** — spec state untouched, orchestrator reports "paused at `<gate>`, awaiting your direction." Refusal is a decision, not progress.
- **Authoring-ordering wart fix:** today `planning.md` creates the spec file *then* gates, so refusal leaves an un-gated file on disk. `core/` fixes this: author to a draft/buffer, gate, commit only on Accept. Refusal = no spec mutation occurred.

### 5.5 `spawn-worker` semantics (Q12)
Fresh, context-free sub-session with a self-contained prompt (a spawn prompt is the worker's entire context). Named, addressable, killable, may go idle, returns one final message. Models both brooder (alive across fix loop) and scout (named, re-spawnable, short-lived) under one primitive. Distinct from `spawn-pool`'s persistent case.

### 5.6 Tier C degradation (Q20)
Tier C runs if `spawn-worker` + `spawn-pool` + `message-peer` are truthfully provided. Piping (tmux display, `/resume`, permission inheritance) is **degraded-optional**, stated by the adapter, with fallbacks the current prose already specifies:
- Display: in-process text (today's prose already offers "proceed degraded with in-process teammates").
- Recovery: manual `fledge vee` + `fledge broods` + `git worktree list` reconstruction — which `implementation.md` §6 already describes as the resume method (`/resume` and `/rewind` do not restore teammates).
- The primitive contract is the real gate; piping is orthogonal to whether the loop works.

### 5.7 Template paths
`core/` references templates self-relatively ("the template at `templates/scout-report.md` in this skill's directory"), never `.claude/...`.

### 5.8 `fledge-interrogate` (Q18)
- Ships in `core/skills/fledge-interrogate/`, scaffolded to `.fledge/skills/fledge-interrogate/` by every `init`. Self-contained planning on every harness.
- **Namespaced** as `fledge-interrogate` (not `interrogate`) to avoid collision with any pre-existing repo `interrogate` skill (pi warns and keeps first-found on collision). `planning.md` references `fledge-interrogate` by skill name; the Q10 duplicate guard covers it.

## 6. Feather lifecycle (Q17)

- **CLI owns 4 durable states:** `egg | pipping | hatching | fledged` (per `internal/spec/types.go`). The CLI stays orchestration-agnostic — no new status values.
- **`core/` owns runtime sub-states** (claimed/dispatched/in-review/green-on-main) as orchestrator bookkeeping, **never persisted** to spec frontmatter. `hatching` is the single CLI state covering all of "claimed/dispatched/in-review/green-on-main."
- **Resume reconstruction** is from `fledge broods` + `git worktree list` + `fledge vee` — exactly what `implementation.md` §6 already does. No CLI change; the neutral spine stays clean.

## 7. Target adapters

### 7.1 Claude Code (existing → refactored) — ships 0.2.0
- **Keep** `.claude/agents/{fledge-brooder,fledge-forager,fledge-context-scout,fledge-skua}.md` (subagent defs; prose slimmed to Claude-runtime specifics, referencing `core/` for shared protocol).
- **Keep** `.claude/settings.json` (`teammateMode: tmux`, `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`); add `skills` pointer to `../.fledge/skills` (or symlink fallback — see §12 verification).
- **Regenerate** `settings.local.json` allow-list from `commandOrder` (Q23).
- **Move** the skill to `.fledge/skills/fledge-orchestrate/` (core); `.claude/` adapter points at it.
- **Piping file** `team-loop.md`: tmux, `/resume` recovery, permission-mode inheritance, team task list.

### 7.2 pi (new) — ships 0.2.0 at Tier A
- **Skill:** none to write — pi loads the shared `core/` skill directly via `.pi/settings.json` `skills: ["../.fledge/skills"]`. Confirmed: pi discovers `SKILL.md` recursively and explicitly supports Claude/Codex skill dirs (`docs/skills.md`).
- **Prompt templates** (`.pi/prompts/`): `/fledge-plan`, `/fledge-implement` — verb-shaped discoverable entry points that expand to "load and follow `.fledge/skills/fledge-orchestrate/SKILL.md`, routing to the `<phase>`." (pi skills also register as `/skill:name`, but the verb-shaped prompts are friendlier.)
- **Tier A** in 0.2.0. **Tier C deferred to M4** (optional, no stub shipped).
- **M4 (deferred):** `.pi/extensions/fledge.ts` TypeScript extension exposing `fledge_gate` (wraps `ctx.ui.select`/`confirm`) and `fledge_spawn` (uses the pi SDK `createAgentSession` to spawn brooder/skua sub-sessions). The pi-native path to the full team loop.

### 7.3 Codex CLI (new) — ships 0.2.0 at Tier A
- **Skill:** Codex supports the Agent Skills standard (per pi's docs). Point Codex at `.fledge/skills/` via its skills config; `AGENTS.md` at repo root adds a one-line pointer.
- **Tier A** (planning + solo implementation); Codex subagents may do Tier-B foraging if available.
- **Verify before M3:** exact Codex skills location and `AGENTS.md` auto-load behavior against a current Codex CLI release.

### 7.4 Cursor CLI (new) — 0.3.0
- `.cursor/rules/fledge.mdc` (MDC frontmatter: `description`, `globs`, `alwaysApply: false`) triggering on plan/implement phrases. Tier A.
- **Verify before 0.3.0:** `.cursor/rules/*.mdc` vs newer agents format.

### 7.5 opencode (new) — 0.3.0
- `opencode.json` / `.opencode/` agent or command entry pointing at the core skill; `AGENTS.md` fallback. Tier A, upgradeable if opencode exposes subagent/teammate primitives.
- **Verify before 0.3.0:** exact opencode config layout.

## 8. Testing strategy

- **testscript/txtar** (`cmd/fledge/testdata/`):
  - `init.txtar`: extend for `--agent`, `--list-agents`, auto-detect (seed `.claude/` / `.pi/` and assert correct adapters scaffold), re-run additivity, `--json` `agents` field, `--upgrade-core`, duplicate guard refusal.
  - `agents.txtar` (new): `fledge agents` text + JSON shapes.
  - Assert core skill written to `.fledge/skills/fledge-orchestrate/SKILL.md` for every `init`.
  - Assert each adapter scaffolds to its native path and only there.
  - Assert M0 behavioral identity (Q22b): same files present modulo new core location, same exit codes, `--json` shape + `agents` field.
- **Adapter smoke checks** (Go test, no network): for each adapter, assert all referenced `core/` paths exist and the adapter's primitive-map keys cover every primitive the `core/` prose uses (parse `core/` for the 7 primitive names, diff against the adapter manifest's `tier_primitives`). Catches "added a primitive, forgot an adapter."
- **Primitive coverage ↔ prose consistency:** assert that the primitives `core/`'s capability-conditional branches *require for a given subset* are exactly the ones in the adapter's declared subset (Q5 derivation).
- **Skill validity:** assert every `SKILL.md` under `core/` has valid Agent-Skills frontmatter (`name` ≤64 chars, lowercase-hyphens, matches parent dir except where namespaced; `description` ≤1024 chars, present) — mirrors pi's validation so a pi install won't warn.
- **Adapter map ↔ inlined entry consistency:** assert the inlined primitive map in each generated entry file matches the manifest's `tier_primitives` (Q6 single-source guarantee).
- **Regression guard:** existing `new/preen/ready/vee/colony/status/set/criteria/brood/abandon/broods/version` txtar scripts pass without modification (CLI stayed neutral).

## 9. Release scope (Q22)

- **0.2.0:** Claude + pi + Codex at Tier A. Three harnesses sharing one `core/` skill is the load-bearing breadth proof. M0–M3 (Codex). No M4 stub.
- **0.3.0:** Cursor + opencode (after verifying layouts). M4 (pi Tier C extension) optional, whenever ready.
- **VERSION:** bump to `0.2.0`; `fledge_version` frontmatter stamp picks it up automatically.

## 10. File reorganization (M0)

1. `internal/bootstrap/claude/skills/fledge-orchestrate/{SKILL.md,planning.md,implementation.md,templates/*}` → `internal/bootstrap/core/skills/fledge-orchestrate/…`, neutralized per §5.
2. `internal/bootstrap/claude/agents/*.md` → `internal/bootstrap/adapters/claude/agents/*.md`, prose slimmed to Claude-runtime specifics + "see core skill for protocol."
3. Repo's `.claude/skills/interrogate/SKILL.md` → `internal/bootstrap/core/skills/fledge-interrogate/SKILL.md` (namespaced).
4. `.claude/settings.json` / `settings.local.json` → `internal/bootstrap/adapters/claude/` (generated permissions, skills pointer).
5. `internal/cli/init.go`: replace `writeBootstrapFiles` (walks `claude/`, writes `.claude/`) with a registry-driven walker writing `.fledge/skills/` + each selected adapter's native paths.
6. `internal/bootstrap/bootstrap.go`: `//go:embed core adapters`.
7. Add `internal/cli/agents.go`: `fledge agents` command.
8. Re-forge `.fledge/nest/` context docs after reorg (the modules map changes).

## 11. Milestones

- **M0 — Refactor bootstrap (behavioral identity).** Move Claude content into `core/` + `adapters/claude/`; neutralize prose; move `interrogate` → `fledge-interrogate`. `fledge init` (no flags) produces the new layout and the resulting repo still drives Claude. *Exit: behavioral identity per Q22b; all existing tests green.*
- **M1 — Multi-agent `init`.** Add `--agent`, `--list-agents`, `fledge agents`, auto-detect, `--upgrade-core`, duplicate guard, JSON `agents` field. Core skill writes to `.fledge/skills/`; Claude adapter points at it. New txtar tests. *Exit: `fledge init --agent claude,pi` works; one core skill, two adapters.*
- **M2 — pi adapter (Tier A).** `.pi/settings.json` pointer + prompt templates + README. Verify the shared skill loads in a real pi session and routing works end-to-end. *Exit: pi drives fledge planning with zero new workflow prose.*
- **M3 — Codex adapter (Tier A).** Confirm Codex skills/`AGENTS.md` layout, write pointer artifacts + README, smoke-test. *Exit: three harnesses Tier A.*
- **M4 — pi Tier C extension (deferred, optional).** Implement `fledge_gate` + `fledge_spawn` in `.pi/extensions/fledge.ts` via the SDK; wire the Tier C branch; validate the team loop on a 2-feather plumage. *Exit: pi runs the full brooder/skua loop.*
- **M5 — Distribution & docs.** README rewrite, per-adapter docs, publish `core/` as a pi package + Claude/Codex skills repo. Bump 0.2.0.

M0 and M1 are the load-bearing refactor; M2–M3 are mostly pointer work thanks to the Agent Skills standard; M4 is the ambitious optional piece.

## 12. Risks & verification tasks

- **R: Workflow drift across adapters** — mitigated by single `core/` source + the primitive-coverage test (§8). Adapters own format only.
- **R: Tier C prose complexity** — keep Tier A as the linear default; isolate Tier C in clearly-marked capability-conditional blocks; if it grows, split `implementation.md` into `implementation.md` (A) + `implementation-teams.md` (C).
- **R: Harness layout churn** — keep adapters minimal/pointer-shaped; record verified-against version in each adapter README.
- **R: Breaking existing Claude repos** — M0's behavioral-identity exit criterion (Q22b) + the additive `init` invariant (Q9) + manual `MIGRATION.md` (Q9) contain this.
- **R: Skill-name collisions** — `fledge-interrogate` namespaced (Q18); Q10 duplicate guard covers all native+core locations.
- **V (M0-critical): Claude `settings.json` `skills` array** — could not verify in this environment whether Claude Code supports a `skills` array the way pi does, or only scans `.claude/skills/`. If unsupported, the Claude adapter falls back to a symlink (`.claude/skills/fledge-orchestrate` → `.fledge/skills/fledge-orchestrate`). Works but is the one fragile piece. M0 task; does not block any locked decision.
- **V (M3): Codex** skills location + `AGENTS.md` auto-load — confirm against a current Codex CLI release.
- **V (0.3.0): Cursor** `.cursor/rules/*.mdc` vs agents format; **opencode** config layout.

## 13. Interrogation transcript (Q1–Q23)

Each question asked one at a time with a recommended answer; the user's choices are recorded in §1. The full reasoning (facts looked up, why each option was/wasn't recommended) is in the session transcript. Key facts confirmed against this repo:

- The `fledge` CLI is already fully agent-agnostic (`grep` of `internal/cli` finds no agent refs except `init.go`'s hardcoded `.claude`).
- The existing `SKILL.md` already conforms to the Agent Skills standard.
- pi loads Claude/Codex skill dirs natively (`docs/skills.md`); pi warns on skill-name collisions.
- `.fledge/` is in `.fledgeignore` (scan-ignored) but not `.gitignore` except per-run intermediates — so `.fledge/skills/` is committed and shared.
- Today's `settings.local.json` allow-list is stale (lists retired commands, omits current ones); `commandOrder` is the source of truth.
- Feather status values are `egg|pipping|hatching|fledged` (`internal/spec/types.go`); `implementation.md` §6 already reconstructs runtime state from `vee`+`broods`+worktrees on resume (no `/resume` dependence).

---

*Next step: turn this into a plumage (`fledge new plumage --title "Generalize fledge to any-agent orchestration" --priority P0`), with feathers per milestone (M0–M3 for 0.2.0; M4/M5 deferred), or start implementing M0.*
