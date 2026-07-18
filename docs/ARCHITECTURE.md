# Fledge Architecture

> Distilled from `docs/reference/integration-surfaces.md` and
> `docs/reference/ai-sdlc-scan.md`, research snapshot 2026-07-17; re-verify
> version-specific claims at build time.

Fledge is a **zero-inference Go orchestrator** for a multi-agent coding stack:
Herdr (terminal multiplexer / pane bus), Pi (programmable GPT harness), and
Claude Code. The Go CLI is the state authority; Herdr is UI/pane plumbing; all
LLM inference happens inside agent panes, never in the orchestrator.

## Invariants

These are settled decisions (see `docs/DECISIONS.md`), not open questions:

1. **The Go CLI is the state authority.** Herdr events and agent hook/RPC
   events are *input signals*; Fledge's own store (Stage 1: SQLite) is truth.
   Herdr is never relied on for durable orchestration state — it does not
   restore token metadata across server restarts and only restores native
   session references.
2. **Pi panes: trust Herdr's native lifecycle authority.** Pi's bundled
   Herdr extension (integration v2) authoritatively reports idle/working/
   blocked plus a native session reference. Fledge *reads* this; it never
   reports custom state onto Pi panes.
3. **Claude panes: metadata only** *(accepted pending experiment EXP1)*.
   Claude Code is intentionally not a lifecycle authority in Herdr;
   blocked/working comes from screen-manifest detection — which is exactly
   what surfaces permission prompts as `blocked`. Fledge uses
   `pane.report_metadata` (display-only) on Claude panes and does **not**
   seize lifecycle authority with `pane.report_agent --source custom:*`,
   preserving Herdr's built-in blocked detection.
4. **Authority seizure is deliberate and paired.**
   `pane.report_agent --source custom:*` is reserved for panes running agents
   Herdr cannot detect, or where Fledge deliberately takes over state — and
   any seizure must be paired with `pane.clear_agent_authority` /
   `pane.release_agent` on exit so fallback detection resumes.

## The zero-inference rule

The Go CLI may only:

- (a) issue Herdr socket commands (spawn panes/agents, send input, read
  output, report display metadata, manage worktrees);
- (b) consume Herdr events and agent hook/RPC events as input signals;
- (c) advance a deterministic FSM / workflow engine;
- (d) write its own event log and acquire/release namespace locks.

The Go CLI must never:

- make an LLM API call, embed a model, or evaluate prompts — all inference
  runs inside Pi/Claude panes;
- treat Herdr (or any agent's self-report) as durable truth;
- hand-drive an agent invisibly: agents run in visible, interactable panes
  the operator can watch and take over.

The Stage 0 experiment harnesses obey the same rule: they issue socket
commands, read events, and record observations only.

## Data / event flow

Adapted from Figure 1 of the integration reference doc. Three numbered paths:

```
                  ┌────────────────────────────────────────────────┐
                  │             Go Orchestrator (CLI)              │
                  │  STATE AUTHORITY — deterministic routing only  │
                  │  ┌──────────┐  ┌───────────┐  ┌─────────────┐  │
                  │  │ FSM /    │  │ SQLite    │  │ flock       │  │
                  │  │ workflow │  │ event log │  │ namespace   │  │
                  │  │ engine   │  │ + tasks   │  │ locks       │  │
                  │  └────┬─────┘  └────┬──────┘  └──────┬──────┘  │
                  └───────┼─────────────┼────────────────┼─────────┘
       (1) commands       │             │ truth          │ (2) hook/RPC callbacks
       pane.split /       │             │                │  Claude hooks POST event
       agent.start /      │             │                │  JSON to the relay;
       send_input /       ▼             │                ▲  Pi RPC events read
       report_metadata  ┌──────────────────────────┐     │  directly by the relay
       ───────────────► │  Herdr server (socket)   │     │
       ◄─────────────── │  NDJSON, session.snapshot│     │
       (3) events.subscribe: pane.agent_status_changed,  │
           pane.output_matched, worktree.*, layout.*     │
                  ┌───────────┬───────────────┬──────────┴───────┐
                  ▼           ▼               ▼                  ▼
           ┌────────────┐ ┌────────────┐ ┌─────────────┐  ┌─────────────┐
           │ Pane: Pi   │ │ Pane: Pi   │ │ Pane: Claude│  │ Pane: Claude│
           │ planner    │ │ reviewer   │ │ implementer │  │ reviewer    │
           │ (RPC/ext)  │ │ (lifecycle │ │ (screen-    │  │ (plan mode) │
           │ lifecycle  │ │  authority)│ │  detected)  │  │             │
           │  authority │ │            │ │  worktree A │  │  worktree A │
           └────────────┘ └────────────┘ └─────────────┘  └─────────────┘

   (1) Go→Herdr: spawn/route/inject input; report_metadata (display only)
       or report_agent (deliberate authority seizure)
   (2) Agent→Go: Claude hooks POST event JSON to the relay's HTTP endpoint;
       Pi RPC events are read directly from the subprocess stream
   (3) Herdr→Go: subscriptions deliver semantic-state and output events
       as INPUT SIGNALS — never as truth
```

Bootstrap pattern per Herdr's docs: read `session.snapshot` once, subscribe
to resource events, keep a local mirror updated from events, re-snapshot on
reconnect — while the orchestrator's own store remains authoritative.

## Staged roadmap

| Stage | Scope | Status |
|---|---|---|
| **0** | Repo skeleton; distilled docs; Herdr type generation (`scripts/gen-herdr-types.sh`); three experiment harnesses (authority override, interactive input, rate limits); supervised execution of EXP1–EXP2 | **in progress (this stage)** |
| 1 | Relay core: flock namespace locks, SQLite event log, FSM/workflow engine, Claude hook HTTP endpoint, Pi RPC subprocess manager (LF-framed). Commissioned in a separate session after Stage 0 experiment results are recorded. | not started — deliberately no placeholder packages |
| 2 | Planner + adversarial reviewer, single task, no parallelism; human escalation via `blocked` rollup + notifications | not started |
| 3 | Parallel implementer/reviewer dyads: worktree per implementer, declared file ownership, one-branch-at-a-time merge, Claude concurrency capped per EXP3 | not started |
| 4 | Hardening: protocol-version pinning + type regeneration on Herdr upgrade, retry/backoff on socket and RPC, StopFailure-driven queue-for-reset, chaos-test Herdr restart to confirm the log reconstructs state | not started |

## Repo layout (Stage 0)

```
go.mod                      module github.com/Harrison-Blair/fledge
docs/                       this doc, INTEGRATION-CONTRACTS, DECISIONS,
                            EXPERIMENTS, handoff-stage0; reference/ (immutable)
internal/herdrclient/       NDJSON socket transport; session.snapshot +
                            events.subscribe; typed method surface — the one
                            component Stage 1 reuses as-is
cmd/exp1-authority/         EXP1 harness (authority override)
cmd/exp2-input/             EXP2 harness (interactive Claude input)
cmd/exp3-ratelimit/         EXP3 harness (rate limits — NEVER agent-run)
scripts/                    gen-herdr-types.sh, exp-session-up.sh,
                            exp-session-down.sh
```

`CLAUDE.md` is human-authored and out of bounds for agents (see
`docs/DECISIONS.md`).
