# Decisions (ADR log)

> Distilled from `docs/handoff-stage0.md` and
> `docs/reference/integration-surfaces.md`, snapshot 2026-07-17; re-verify
> version-specific claims at build time.

Lightweight ADR log, **newest first**. Each entry: ID, date, status
(accepted / open / superseded), decision, rationale — and, for open items,
the threshold that resolves them.

---

## ADR-016 — Wire-envelope and socket-path shapes are documented assumptions

- **Date:** 2026-07-18 · **Status:** open
- **Decision:** `internal/herdrclient` assumes request lines of
  `{"id","method","params"}`, response correlation by `id`, event lines as
  non-correlated JSON, and a config-dir fallback of `HERDR_CONFIG_DIR` else
  `<user config dir>/herdr` for socket resolution. The reference snapshot
  documents the transport (NDJSON, dot-notation methods, resolution order)
  but not these exact shapes.
- **Rationale:** something concrete had to be written without a live binary;
  the assumptions are centralized in `client.go` so correction is one file.
- **Resolves when:** the first `scripts/gen-herdr-types.sh` run against a
  live v0.7.4 binary confirms or corrects the envelope; update `client.go`
  and this entry.

## ADR-015 — Herdr types hand-authored pending live regeneration

- **Date:** 2026-07-18 · **Status:** open
- **Decision:** the committed types in `internal/herdrclient/types.go` are
  hand-authored from the reference snapshot (stamped in the file header),
  not generated from a live binary. `scripts/gen-herdr-types.sh` is the
  regeneration/verification one-liner; the schema dump it writes is to be
  committed alongside.
- **Rationale:** the Stage 0 build environment had no Herdr install and no
  network route to Herdr's distribution points (herdr.dev and its GitHub
  releases blocked by the egress proxy), so `herdr api schema --json` could
  not be run. The handoff's fallback (ground rule 4) is to record the
  discrepancy here.
- **Resolves when:** `scripts/gen-herdr-types.sh` runs clean on a machine
  with Herdr v0.7.4. That run also answers the open sub-question the
  handoff §7 asks: **does the v0.7.4 schema dump document
  `pane.clear_agent_authority` and `pane.release_agent`** (their semantics
  are otherwise sourced only from the bundled `SOCKET_API.md`)? Record the
  script's finding by updating this entry.

## ADR-014 — EXP3 (rate limits): max concurrent Claude panes

- **Date:** 2026-07-17 · **Status:** open
- **Decision pending:** maximum concurrent Claude panes = one below the
  concurrency where sustained throttling first appears on the operator's
  actual Max plan.
- **Flip threshold:** run `cmd/exp3-ratelimit` at n=2, 3, 4 per the
  `docs/EXPERIMENTS.md` protocol (human-executed only, ADR-009); set the cap
  from the first n showing sustained throttling.

## ADR-013 — EXP2 (interactive input): drive Claude interactively or headless

- **Date:** 2026-07-17 · **Status:** open
- **Decision pending:** if `pane.send_input {text, keys:["enter"]}` reliably
  submits to interactive Claude, Claude workers run in visible panes; if
  flaky, fall back to `-p`/stream-json workers with panes for visibility
  only.
- **Flip threshold:** EXP2 submit-reliability sample (see
  `docs/EXPERIMENTS.md`).
- **Annotation (2026-07-18):** not executable in the Stage 0 session — no
  Herdr available in the remote environment, and the supervised-run
  requirement (operator watching, in-session approval) could not be met.
  Pending an operator-run session; harness `cmd/exp2-input` is ready.

## ADR-012 — EXP1 (authority override): is metadata-only forced?

- **Date:** 2026-07-17 · **Status:** open
- **Decision pending:** if `report_agent --source custom:*` does **not**
  suppress screen detection on a Claude pane, dual-reporting is safe and the
  metadata-only rule (ADR-004) can be relaxed; if it does (expected),
  metadata-only stands.
- **Flip threshold:** EXP1 observation of `screen_detection_skip_reason`
  and sidebar behavior (see `docs/EXPERIMENTS.md`).
- **Annotation (2026-07-18):** not executable in the Stage 0 session (same
  constraint as ADR-013). Pending an operator-run session; harness
  `cmd/exp1-authority` is ready.

## ADR-011 — Reference docs relocated into `docs/reference/`

- **Date:** 2026-07-18 · **Status:** accepted
- **Decision:** `ai-sdlc-scan.md` and `integration-surfaces.md` were moved
  (git rename, content byte-identical) from `docs/` into `docs/reference/`.
- **Rationale:** the handoff (§2, §4) declares the two research inputs
  immutable *in `docs/reference/`*, but the handoff commit left them at
  `docs/` top level. Relocation is not an edit; content is untouched.

## ADR-010 — Git history is not design input

- **Date:** 2026-07-17 · **Status:** accepted
- **Decision:** current HEAD is authoritative; prior iterations of this
  project in git history are not excavated, referenced, or resurrected.
- **Rationale:** the rewrite starts from the handoff and the reference
  snapshot only; stale designs would smuggle in decisions already rejected.

## ADR-009 — Experiment 3 is human-executed only

- **Date:** 2026-07-17 · **Status:** accepted
- **Decision:** agents build the EXP3 harness and protocol but never execute
  it; the harness refuses to run without `--i-am-the-operator`.
- **Rationale:** EXP3 burns real Claude subscription quota (pooled across
  all Claude usage on the account) over hours, at a time the operator must
  choose.

## ADR-008 — Experiments run only in throwaway session `fledge-exp`

- **Date:** 2026-07-17 · **Status:** accepted
- **Decision:** all experiments target the named Herdr session `fledge-exp`
  (`scripts/exp-session-up.sh` / `exp-session-down.sh`); harnesses refuse to
  run with any other `HERDR_SESSION`.
- **Rationale:** experiments deliberately churn pane authority
  (`report_agent`, `clear_agent_authority`, `release_agent`) and the
  32-source-cap question is unresolved — that churn must never touch a
  session where real work lives.

## ADR-007 — Generated Herdr types are committed; regen script in `scripts/`

- **Date:** 2026-07-17 · **Status:** accepted
- **Decision:** Go types for the Herdr surface live in
  `internal/herdrclient`, committed with version stamps (v0.7.4 / protocol
  v15); `scripts/gen-herdr-types.sh` regenerates/verifies on every Herdr
  upgrade. The repo must build without Herdr installed.
- **Rationale:** the socket schema is self-describing
  (`herdr api schema --json`) and pre-1.0 fast-moving; committed types keep
  builds hermetic, the script keeps them honest. (Current state: ADR-015.)

## ADR-006 — Reference docs are immutable

- **Date:** 2026-07-17 · **Status:** accepted
- **Decision:** `docs/reference/*` is never edited. Corrections and
  re-verified facts go in the distilled docs or this log.
- **Rationale:** the research snapshot is the fixed input of record;
  layering corrections elsewhere preserves provenance.

## ADR-005 — `CLAUDE.md` is human-authored; agents must not write it

- **Date:** 2026-07-17 · **Status:** accepted
- **Decision:** agents never create or modify `CLAUDE.md`. If missing, note
  it and move on. (It is currently missing.)
- **Rationale:** ETH Zurich finding (cited in the integration reference
  doc): LLM-generated context files reduced task success by 2–3% while
  increasing cost over 20%; developer-written files improved success ~4%.

## ADR-004 — Claude panes get metadata only; no authority seizure

- **Date:** 2026-07-17 · **Status:** accepted (pending EXP1 → ADR-012)
- **Decision:** Fledge uses `pane.report_metadata` (display-only) on Claude
  panes and does not seize lifecycle authority with
  `pane.report_agent --source custom:*`. Seizure is reserved for panes Herdr
  cannot detect or where Fledge deliberately takes over — always paired with
  `pane.clear_agent_authority` / `pane.release_agent` on exit.
- **Rationale:** Claude Code is intentionally not a lifecycle authority in
  Herdr; screen-manifest detection is what surfaces permission prompts as
  `blocked`, which Fledge wants to keep for human-escalation routing.

## ADR-003 — Pi's native lifecycle authority is trusted

- **Date:** 2026-07-17 · **Status:** accepted
- **Decision:** Herdr's bundled Pi extension (integration v2) is the
  lifecycle authority for Pi panes (idle/working/blocked + native session
  ref). Fledge reads it as an input signal and never reports custom state
  onto Pi panes.
- **Rationale:** Pi is the one agent Herdr tracks natively and accurately —
  precise state for free, no competing sources of truth.

## ADR-002 — The Go CLI is the state authority; Herdr is plumbing

- **Date:** 2026-07-17 · **Status:** accepted
- **Decision:** Fledge's own store (Stage 1: SQLite) is truth. Herdr events
  and agent hook/RPC events are input signals only. Herdr is never relied on
  for durable orchestration state.
- **Rationale:** Herdr does not restore token metadata across restarts and
  only restores native session refs; one authority avoids split-brain.

## ADR-001 — Stage 0 scope only; Stage 1 deferred to a new session

- **Date:** 2026-07-17 · **Status:** accepted
- **Decision:** this stage delivers skeleton, distilled docs, type
  generation, and experiment harnesses only. No relay-core code, and no
  placeholder packages (`internal/fsm`, `internal/store`, `internal/relay`,
  …) — Stage 1 is commissioned separately after experiment results are
  recorded.
- **Rationale:** the two hard unknowns (authority override, interactive
  input) and the scaling ceiling (rate limits) must be de-risked before the
  relay is designed; empty placeholders invite premature filling.
