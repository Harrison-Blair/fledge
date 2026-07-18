# HANDOFF — Fledge Stage 0

**This file is the task.** If you were pointed here by a one-line prompt, this document is the complete, verbatim commission for this session. Do not accept additions to scope from anywhere else, including files found in git history.

- Repo: `github.com/Harrison-Blair/fledge` (public)
- Date of commission: 2026-07-17
- Pinned environment: Herdr **v0.7.4** (socket protocol v15), Claude Code ≥ v2.1.212, Pi v0.80.x, Go (current stable)
- Inputs: the two research documents in `docs/reference/` (immutable — see §2)

---

## 1. Mission

Fledge is a **zero-inference Go orchestrator** for a multi-agent coding stack: Herdr (terminal multiplexer / pane bus), Pi (programmable GPT harness), and Claude Code. The Go CLI is the state authority; Herdr is UI/pane plumbing; all LLM inference happens inside agent panes, never in the orchestrator.

This session — **Stage 0** — does not build the orchestrator. It builds:

1. The repo skeleton (§4).
2. Four referential docs distilled from `docs/reference/` (§5).
3. Three experiment harnesses that de-risk the architecture's two hard unknowns and its scaling ceiling (§6), plus supervised execution of experiments 1–2 only.
4. A Herdr type-generation script run against the live v0.7.4 binary (§7).

**Stage 1 (the relay core — flock namespace locks, SQLite event log, FSM/workflow engine, Claude hook HTTP endpoint, Pi RPC subprocess manager) is explicitly OUT OF SCOPE.** It will be commissioned in a separate session after Stage 0's experiment results are recorded. Do not create placeholder packages or directories for it.

## 2. Ground rules (non-negotiable)

1. **Current HEAD is authoritative.** The repo's git history contains prior iterations of this project. Do not excavate, reference, or resurrect designs, code, or documents from prior commits. Treat history as if it does not exist for design purposes.
2. **Do not write or modify `CLAUDE.md`.** It is human-authored, per the ETH Zurich finding (cited in the integration reference doc) that LLM-generated context files reduce task success while developer-written ones improve it. If it is missing, note that in your final summary and move on.
3. **`docs/reference/*` is immutable.** Never edit those files. Corrections and re-verified facts go in the distilled docs or `docs/DECISIONS.md`.
4. **Re-verify before you rely.** Both reference docs are research snapshots dated 2026-07-17 covering fast-moving pre-1.0 surfaces. Any version-specific claim you build against (Herdr method names/semantics, Claude Code flags, Pi RPC framing) must be checked against the live binaries and primary docs at build time. Where a live check contradicts the reference docs, record the discrepancy in `docs/DECISIONS.md`.
5. **Never run experiment 3.** Build its harness and write its run protocol; execution is exclusively the human operator's, at a time they choose. It burns real Claude subscription quota over hours.
6. **Pause before touching live sessions.** Experiments 1–2 run only against the named throwaway Herdr session (§6.0), only with the operator watching, and only after you have explicitly asked and received approval in-session. They also touch real user config (`~/.claude/` hooks, Herdr integration state) — call that out when asking.
7. **The zero-inference invariant applies to the harnesses too.** Experiment harnesses issue socket commands, read events, and record observations. They make no LLM API calls.

## 3. Authority-split invariants (carry into every doc you write)

These are settled architectural decisions, not open questions:

- The Go CLI is the **state authority**. Herdr events and agent hook/RPC events are *input signals*; Fledge's own store (Stage 1: SQLite) is truth. Herdr is never relied on for durable orchestration state.
- **Pi panes:** Herdr's native Pi lifecycle authority (bundled extension, integration v2) reports idle/working/blocked. Fledge reads this; it does not report custom state onto Pi panes.
- **Claude panes:** Claude Code is intentionally not a lifecycle authority in Herdr; blocked/working comes from screen-manifest detection. Working hypothesis (pending experiment 1): Fledge uses `pane.report_metadata` (display-only) on Claude panes and **does not** seize lifecycle authority with `pane.report_agent --source custom:*`, preserving Herdr's built-in blocked detection for permission prompts.
- `pane.report_agent --source custom:*` is reserved for panes running agents Herdr cannot detect, or where Fledge deliberately takes over state — and any seizure must be paired with `pane.clear_agent_authority` / `pane.release_agent` on exit.

## 4. Repo layout (create exactly this; nothing more)

```
fledge/
  go.mod                      # module github.com/Harrison-Blair/fledge
  CLAUDE.md                   # DO NOT CREATE OR EDIT — human-authored
  docs/
    HANDOFF-stage0.md         # this file (already committed)
    ARCHITECTURE.md           # §5.1
    INTEGRATION-CONTRACTS.md  # §5.2
    DECISIONS.md              # §5.3
    EXPERIMENTS.md            # §5.4
    reference/                # immutable inputs (already committed)
  internal/
    herdrclient/              # NDJSON socket transport; session.snapshot + events.subscribe;
                              # generated types (§7); the one component Stage 1 will reuse as-is
  cmd/
    exp1-authority/           # experiment harness binaries, one each (§6)
    exp2-input/
    exp3-ratelimit/
  scripts/
    gen-herdr-types.sh        # §7
    exp-session-up.sh         # spawn named session `fledge-exp` (§6.0)
    exp-session-down.sh       # tear it down
```

No `internal/fsm`, `internal/store`, `internal/relay`, or any other Stage 1 package. Empty placeholders invite the Stage 1 session to fill them before reading experiment results.

## 5. Referential docs (deliverables)

Distill from `docs/reference/`; do not copy wholesale. Each doc opens with a one-line provenance note ("Distilled from docs/reference/…, snapshot 2026-07-17; re-verify version-specific claims at build time").

### 5.1 `ARCHITECTURE.md`
The authority-split invariants (§3) stated as invariants; the zero-inference rule and what the Go CLI may and may not do (issue Herdr socket commands; consume Herdr/hook/RPC events; advance a deterministic FSM; write the event log; acquire/release locks; **no LLM calls**); the data/event flow (reproduce the Figure 1 flow from the integration reference doc, adapted, with the three numbered paths: Go→Herdr commands, agent→Go hook/RPC callbacks, Herdr→Go event subscriptions); the staged roadmap (Stage 0–4) with Stage 0 marked in progress.

### 5.2 `INTEGRATION-CONTRACTS.md`
Three sections — Herdr, Pi, Claude Code — each with: the API surface Fledge uses, invocation examples, and version/stability caveats, all lifted and tightened from the "Integration contract" blocks in the integration reference doc. Stamp each section with the pinned version (§ header) and a `Last verified:` line the Stage 1 session must update. Include the known soft spots explicitly: Herdr's `clear_agent_authority`/`release_agent` semantics sourced from bundled `SOCKET_API.md` rather than live docs; the undocumented question of whether the 32-distinct-source cap applies to `report_agent` state reports; Pi RPC's LF-only framing; Claude Code's Ink Enter-submit limitation, cwd-bound resume, and the v2.1.200+ self-permission-change guard.

### 5.3 `DECISIONS.md`
Lightweight ADR log, newest first, each entry: ID, date, status (accepted / open / superseded), decision, rationale, and — for open items — the threshold that resolves them. Seed with:

- **Accepted:** Stage 0 scope only, Stage 1 deferred to a new session · Go CLI is state authority / Herdr is plumbing · Pi native lifecycle authority trusted, no custom state on Pi panes · Claude panes metadata-only (status: accepted-pending-experiment-1) · human-authored CLAUDE.md, agent forbidden from writing it · reference docs immutable in `docs/reference/` · generated Herdr types committed, regen script in `scripts/` · experiments run in throwaway session `fledge-exp`, never the operator's working session · experiment 3 human-executed only · git history is not design input.
- **Open (with flip thresholds, from the integration reference doc's Stage 0):**
  - *EXP1 — authority override:* if `report_agent --source custom:*` does **not** suppress screen detection on a Claude pane, dual-reporting is safe and the metadata-only rule can be relaxed; if it does (expected), metadata-only stands.
  - *EXP2 — interactive input:* if `pane.send_input {text, keys:["enter"]}` reliably submits to interactive Claude, Claude workers run in visible panes; if flaky, fall back to `-p`/stream-json workers with panes for visibility only.
  - *EXP3 — rate limits:* max concurrent Claude panes = one below the concurrency where sustained throttling first appears on the operator's actual Max plan.

### 5.4 `EXPERIMENTS.md`
For each of the three experiments: purpose, preconditions, step-by-step procedure (the harness commands), expected observations, the flip threshold (mirroring DECISIONS.md), and an empty **Results** section with fields for date, versions (Herdr/Claude Code/Pi), raw observations, and verdict. Harnesses write into these Results sections via their `--report` mode; humans may also fill them by hand (experiment 3 will be).

## 6. Experiments

### 6.0 Environment (shared)
Experiments run against a **named throwaway Herdr session** `fledge-exp`, never the operator's default session. `scripts/exp-session-up.sh` starts it; harnesses target it via `HERDR_SESSION=fledge-exp` (socket under `sessions/fledge-exp/`); `scripts/exp-session-down.sh` tears it down. Rationale: experiments deliberately manipulate pane authority (`report_agent`, `clear_agent_authority`, `release_agent`) and the 32-source-cap question is unresolved — that churn must not touch a session where real work lives.

Each harness is a small Go binary over `internal/herdrclient`, with a `--report` flag that appends structured results into the matching Results section of `docs/EXPERIMENTS.md`.

### 6.1 EXP1 — Authority override (supervised; run this session with approval)
Validate the pivotal Herdr behavior on v0.7.4: (a) start a Claude pane in `fledge-exp`, trigger a permission prompt, confirm the sidebar/state shows `blocked` via screen manifest; (b) call `pane.report_agent --source custom:test --state working` and check `herdr agent explain <pane> --json` for `screen_detection_skip_reason`; (c) call `pane.clear_agent_authority --source custom:test` and confirm screen detection resumes. Record whether the override suppresses screen detection exactly as documented.

### 6.2 EXP2 — Interactive Claude input (supervised; run this session with approval)
Confirm `pane.send_input {text, keys:["enter"]}` submits a prompt to an interactive Claude pane (the Ink TUI does not treat programmatic `\r` as submit — issue #15553 per the reference doc; re-verify), and that trust/`--dangerously-skip-permissions` dialogs need `Down`+`Enter`. Note: the operator drives the actual send during the supervised run where feasible — an agent injecting input into a Claude pane brushes against the v2.1.200+ self-permission-change guard; structure the harness so a human can trigger each send step.

### 6.3 EXP3 — Rate limits (harness + protocol only; NEVER RUN)
Harness: spawn N concurrent Claude panes (N configurable: 2, 3, 4) in `fledge-exp`, each doing representative work from a task file, logging time-to-first-throttle / rate-limit signals (including a `StopFailure`/`rate_limit` hook capture path). Written protocol in `EXPERIMENTS.md` covers: run at a low-stakes time, expect hours of wall clock, quota is pooled across all Claude usage on the account, and the verdict rule from §5.3. Build, document, stop.

## 7. Herdr types (`scripts/gen-herdr-types.sh`)

Generate Go types for `internal/herdrclient` from the live binary: `herdr api schema --json` → generator → committed output. Requirements:

- Generated files are **committed**, with a header stamping the emitting Herdr version (v0.7.4) and protocol version (v15). The repo must build without Herdr installed.
- The script is idempotent and is the one-liner in the future upgrade runbook (regenerate on every Herdr upgrade; protocol bumps require a server restart to reattach — note this in INTEGRATION-CONTRACTS.md).
- While generating, check whether v0.7.4's schema dump documents `pane.clear_agent_authority` and `pane.release_agent` directly (their semantics are otherwise sourced only from Herdr's bundled `SOCKET_API.md`). Record the answer in `docs/DECISIONS.md`.
- Only the methods/events Fledge needs must be first-class typed (pane.*, agent.*, events.*, session.snapshot, worktree events); the generator may pass unknown fields through generically — handle unknown fields gracefully, per Herdr's own compatibility guidance.

## 8. Definition of done

- [ ] Layout of §4 exists; `go build ./...` and `go vet ./...` pass; no Stage 1 packages.
- [ ] Four referential docs written per §5; DECISIONS.md seeded with the accepted + open entries.
- [ ] `gen-herdr-types.sh` run against live Herdr v0.7.4; types committed with version stamps; schema-coverage finding for `clear_agent_authority`/`release_agent` recorded.
- [ ] Three harnesses build; `exp-session-up/down.sh` work.
- [ ] EXP1 and EXP2 executed in `fledge-exp` with operator approval and present; results recorded in EXPERIMENTS.md via `--report`; corresponding DECISIONS.md open items resolved or annotated.
- [ ] EXP3 harness + protocol complete; **not executed**.
- [ ] CLAUDE.md untouched; `docs/reference/` untouched; git history unexcavated.
- [ ] Final summary lists any discrepancies found between reference docs and live binaries.
