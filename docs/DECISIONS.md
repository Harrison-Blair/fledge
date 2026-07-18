# Decisions (ADR log)

> Distilled from `docs/handoff-stage0.md` and
> `docs/reference/integration-surfaces.md`, snapshot 2026-07-17; re-verify
> version-specific claims at build time.

Lightweight ADR log, **newest first**. Each entry: ID, date, status
(accepted / open / superseded), decision, rationale — and, for open items,
the threshold that resolves them.

---

## ADR-017 — herdrclient reconciled against live Herdr v0.7.4 / protocol 16

- **Date:** 2026-07-17 · **Status:** accepted (supersedes ADR-015, ADR-016)
- **Context:** the Stage 0 experiment harnesses failed at startup on the
  operator's machine. Root-caused against the live binary (`herdr api schema
  --json`, committed as `internal/herdrclient/herdr-schema.json`, and direct
  socket probes on session `fledge-exp`). The hand-authored client targeted
  protocol **v15**; the live server is protocol **16**, and several shapes
  differ from the reference snapshot.
- **Findings (all re-verified live, per ground rule 4):**
  1. **Protocol is 16, not 15.** `session.snapshot` reports
     `"protocol":16`. The gen-script pin is bumped to 16.
  2. **`params` is mandatory on every method.** Omitting it (the client used
     `json:"params,omitempty"`) yields `invalid_request: missing field
     params`, and the error echoes `id:""` (uncorrelatable) as the server
     drops the connection. Fix: always emit `params` (`{}` when empty).
  3. **Transport is one-request-per-connection.** The server writes one
     response and closes; there is no multiplexing or keep-alive on the RPC
     path. The client's single persistent connection + readLoop + pending-map
     model was wrong (call #1 worked, call #2 hit a dead socket — which is
     why `agent.start` reported `herdrclient: closed`). Fix: `Call` dials a
     fresh connection per request; only `events.subscribe` holds one open.
  4. **Results are wrapped by kind:** `session.snapshot → {snapshot:{…}}`,
     `agent.start`/`agent.get → {agent:{…}}`, `pane.read → {read:{…}}`. The
     flat-decode assumption returned zero values.
  5. **Field/enum drift:** `agent.start` takes `argv` (not `command`) and
     returns the pane at `result.agent.pane_id`; `agent.explain`/`agent.get`
     key the pane as `target` (not `pane_id`); snapshot uses `protocol` (not
     `protocol_version`); `pane.read` source is `recent_unwrapped` (underscore,
     not the reference doc's `recent-unwrapped`).
  6. **Screen-detection signal changed.** There is no
     `screen_detection_skip_reason`; the typed signal is the boolean
     `screen_detection_skipped` on the agent record (`agent.get`).
     `agent.explain` returns an open, server-defined `explain` payload
     (untyped in the schema) — captured raw. EXP1's harness now records
     `screen_detection_skipped` as the pivotal ADR-012 measurement plus the
     raw explain blob.
- **Resolution status:** `client.go` transport rewritten, `types.go`
  reconciled, and the three harnesses updated (`argv`, `recent_unwrapped`,
  EXP1 measurement). Verified end-to-end against `fledge-exp`:
  `session.snapshot`, `agent.start`, `pane.read`, `agent.get`, `pane.close`
  and multi-call reuse all pass. **Not yet verified:** `agent.explain`'s full
  success payload (needs a Herdr-detected Claude pane — the supervised EXP1
  run) and the `events.subscribe` streaming path (unused by Stage 0
  experiments).

## ADR-016 — Wire-envelope and socket-path shapes are documented assumptions

- **Date:** 2026-07-18 · **Status:** superseded by ADR-017 (envelope confirmed
  against the live binary: `{"id","method","params"}` with `params` mandatory;
  RPC is one-shot per connection, not id-multiplexed)
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

- **Date:** 2026-07-18 · **Status:** superseded by ADR-017 — resolved
  2026-07-17. `scripts/gen-herdr-types.sh` ran clean against live Herdr
  v0.7.4; the schema dump is committed and the types reconciled (ADR-017).
  **Handoff §7 sub-question answered:** the v0.7.4 schema dump **documents
  both `pane.clear_agent_authority` and `pane.release_agent` as first-class
  methods** — their semantics are no longer SOCKET_API.md-only.
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

- **Date:** 2026-07-17 · **Resolved:** 2026-07-18 · **Status:** accepted
- **Decision:** **no fixed concurrent-pane cap.** Rate limits are handled
  *reactively* — the `StopFailure`/`rate_limit` hook is the authoritative
  throttle signal — rather than by pre-limiting fan-out. fledge may spawn as
  many concurrent Claude agent panes as the work needs.
- **Evidence:** EXP3 found no throttling at n=2 (burst) and none at n=3 under
  sustained `--sustain` re-fed load (`docs/EXPERIMENTS.md §EXP3`). The operator
  aborted the ceiling hunt early, satisfied that pooled subscription bandwidth
  comfortably exceeds fledge's practical concurrency needs. The absolute
  account-wide ceiling was not driven to failure; it sits above what fledge
  requires, so no cap is warranted.
- **Revisit if:** sustained throttling is observed in real use (the hook fires
  routinely) — then reintroduce a reactive backoff/cap informed by where it
  bites. The `cmd/exp3-ratelimit --sustain` harness remains available to
  re-probe at higher n.

## ADR-013 — EXP2 (interactive input): drive Claude interactively or headless

- **Date:** 2026-07-17 · **Status:** RESOLVED 2026-07-17 → interactive panes.
- **Decision:** `pane.send_input {text, keys:["enter"]}` reliably submits to
  an interactive Claude pane, so **Claude workers run in visible panes** (no
  fallback to `-p`/stream-json needed).
- **Evidence:** supervised EXP2 run against `fledge-exp` (Herdr 0.7.4 /
  protocol 16, Claude Code 2.1.214) — submit reliability **3/3** gated sends;
  rounds 2–3 show the prompt echoed and a `● pong` response. Recorded in
  `docs/EXPERIMENTS.md` §EXP2. (The Ink `\r`-not-submit limitation is real;
  the explicit `keys:["enter"]` is what makes it submit.)

## ADR-012 — EXP1 (authority override): is metadata-only forced?

- **Date:** 2026-07-17 · **Status:** RESOLVED 2026-07-17 → override does NOT
  suppress screen detection on Claude panes (native detection wins).
- **Decision:** a `pane.report_agent --source custom:*` report on a
  natively-detected Claude pane does **not** suppress Herdr's screen-manifest
  detection and does **not** change the pane's state. Native Claude detection
  takes precedence. Therefore custom reports on Claude panes are **safe**
  (they cannot break Herdr's `blocked` detection), so **metadata-only
  (ADR-004) holds as safe and may be relaxed** — with the caveat that pushing
  custom lifecycle *state* onto a Claude pane is harmless-but-ineffective,
  since native detection overrides it.
- **Evidence:** supervised EXP1 run (`docs/EXPERIMENTS.md` §EXP1) — across
  baseline / after `report_agent {custom:test, working, seq 1}` / after
  `clear_agent_authority`, the pane held `screen_detection_skipped=false`,
  `agent_status="blocked"`, `agent="claude"`, `matched_rule=
  legacy_no_prompt_blocker` (byte-identical explain each phase); operator
  confirmed the sidebar stayed `blocked` throughout. **Control** (unopposed):
  the same `report_agent` on a plain shell pane Herdr does not detect flipped
  it `agent=null→"probe"`, `agent_status=unknown→"working"` — proving the call
  seizes authority when nothing native competes, so the Claude-pane no-op is
  precedence, not a broken call.
- **Note (Stage 1):** `pane.clear_agent_authority {source:custom:test}` did
  **not** revert the shell pane's reported `working` back to `unknown` — clear
  vs. `release_agent` semantics need a live check (the schema documents both;
  ADR-017). The metadata-only path (ADR-004) does not depend on this.
- **Note:** the flip threshold originally keyed on
  `screen_detection_skip_reason`; protocol 16 renamed the signal to the
  boolean `screen_detection_skipped` (ADR-017), which is what was measured.

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

- **Date:** 2026-07-17 · **Status:** accepted (EXP1/ADR-012 resolved
  2026-07-17 — confirmed safe; see below)
- **Decision:** Fledge uses `pane.report_metadata` (display-only) on Claude
  panes and does not seize lifecycle authority with
  `pane.report_agent --source custom:*`. Seizure is reserved for panes Herdr
  cannot detect or where Fledge deliberately takes over — always paired with
  `pane.clear_agent_authority` / `pane.release_agent` on exit.
- **Rationale:** Claude Code is intentionally not a lifecycle authority in
  Herdr; screen-manifest detection is what surfaces permission prompts as
  `blocked`, which Fledge wants to keep for human-escalation routing.
- **EXP1 outcome (ADR-012):** confirmed that native Claude detection *outranks*
  a custom `report_agent`, so this rule is safe rather than forced — even a
  custom report cannot break `blocked` detection. Metadata-only stays the
  design; the rule *may* be relaxed to dual-report if ever useful, though
  custom state on Claude panes is overridden by native detection anyway.

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
