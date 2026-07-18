# Experiments

> Distilled from `docs/reference/integration-surfaces.md` (Stage 0
> recommendations), snapshot 2026-07-17; re-verify version-specific claims at
> build time.

Three experiments de-risk the architecture's two hard unknowns (EXP1, EXP2)
and its scaling ceiling (EXP3). Flip thresholds mirror `docs/DECISIONS.md`
(ADR-012/013/014); a completed experiment resolves its ADR.

## Shared environment

All experiments run against the **named throwaway Herdr session
`fledge-exp`** — never the operator's working session. Experiments
deliberately churn pane authority (`report_agent`, `clear_agent_authority`,
`release_agent`) and the 32-source-cap question is unresolved; that churn
must not touch real work.

```sh
scripts/exp-session-up.sh          # start the throwaway session
export HERDR_SESSION=fledge-exp    # harnesses refuse any other session
scripts/exp-session-down.sh        # tear it down afterwards
```

Each harness is a small Go binary over `internal/herdrclient`. With
`--report`, a harness appends its structured results into the matching
**Results** section below (between the HTML markers — do not remove them).
Humans may also fill Results in by hand (EXP3 will be). Harnesses issue
socket commands, read pane output, and record observations only — they make
no LLM calls.

**Supervision rule:** EXP1 and EXP2 run only with the operator watching and
only after explicit in-session approval — they touch real user config
(`~/.claude/` hooks, Herdr integration state) in the sense that the Claude
pane under test uses the operator's real Claude login and hook setup. EXP3
is **never run by an agent** (see §EXP3).

---

## EXP1 — Authority override (supervised)

**Purpose.** Validate the pivotal Herdr v0.7.4 behavior: does
`pane.report_agent --source custom:*` seize lifecycle authority on a Claude
pane and suppress screen-manifest detection, and does
`pane.clear_agent_authority` restore it?

**Preconditions.**
- `fledge-exp` up; `HERDR_SESSION=fledge-exp` exported.
- Operator watching the Herdr UI; in-session approval given.
- A Claude Code pane (the harness can spawn one with `--spawn`), logged in
  with the operator's real account.

**Procedure.** (`go run ./cmd/exp1-authority --spawn --report`)
1. Spawn or attach to a Claude pane in `fledge-exp`.
2. Operator gives Claude a task that triggers a permission prompt; wait for
   the dialog. Harness records `agent.explain` — expect `blocked` via screen
   manifest.
3. Harness calls `pane.report_agent {source: custom:test, state: working,
   seq: 1}` and re-records `agent.explain`, checking
   `screen_detection_skip_reason`; operator reports what the sidebar shows.
4. Harness calls `pane.clear_agent_authority {source: custom:test}` and
   re-records `agent.explain`; operator confirms screen detection resumed.

**Expected observations.** After step 3 the custom source is the pane's
lifecycle authority, `screen_detection_skip_reason` is set, and the sidebar
shows `working` even though a permission dialog is visible. After step 4,
fallback detection resumes and the dialog shows as `blocked` again.

**Flip threshold (ADR-012).** If the override does **not** suppress screen
detection, dual-reporting is safe and the metadata-only rule (ADR-004) can
be relaxed. If it does (expected), metadata-only stands.

### Results

Status: **pending supervised operator run** — could not be executed in the
Stage 0 session (no Herdr in the remote environment; supervision requirement
unmeetable there). Fields per run: date, versions (Herdr / Claude Code /
Pi), raw observations, verdict.

<!-- BEGIN RESULTS EXP1 -->
<!-- END RESULTS EXP1 -->

---

## EXP2 — Interactive Claude input (supervised)

**Purpose.** Confirm `pane.send_input {text, keys:["enter"]}` reliably
submits a prompt to an *interactive* Claude pane — the Ink TUI does not
treat programmatic `\r` as submit (issue #15553; re-verify) — and that
trust/`--dangerously-skip-permissions` dialogs need `Down`+`Enter`.

**Preconditions.** Same as EXP1. Note: every send is behind an operator
gate — a human triggers each injection step, keeping clear of Claude Code's
v2.1.200+ self-permission-change guard.

**Procedure.** (`go run ./cmd/exp2-input --spawn --rounds 3 --report`,
optionally `--trust-dialog`)
1. Spawn or attach to a Claude pane; wait for it to be idle at the prompt.
2. For each round: operator triggers the gated
   `pane.send_input {text, keys:["enter"]}`; harness waits, reads the pane
   tail, and asks the operator whether the prompt submitted.
3. Optional trust-dialog phase: operator sets up a trust or
   `--dangerously-skip-permissions` dialog; gated sends of `down` then
   `enter`; operator confirms whether the dialog was accepted.

**Expected observations.** Text plus a real Enter keypress submits on every
round; bare text without the keypress would sit unsubmitted; `Down`+`Enter`
accepts the dialog.

**Flip threshold (ADR-013).** Reliable → Claude workers run in visible
panes. Flaky → fall back to `-p`/stream-json workers, panes for visibility
only.

### Results

Status: **pending supervised operator run** — could not be executed in the
Stage 0 session (same constraint as EXP1). Fields per run: date, versions,
raw observations (incl. submit tally), verdict.

<!-- BEGIN RESULTS EXP2 -->
<!-- END RESULTS EXP2 -->

---

## EXP3 — Rate limits (**NEVER RUN BY AN AGENT — human-executed only**)

**Purpose.** Find the Claude-pane concurrency ceiling on the operator's
actual Max plan: how many concurrent Claude panes doing representative work
run before sustained subscription throttling appears?

**Why agents never run this.** It burns real Claude subscription quota for
hours, and quota is **pooled across all Claude Code sessions and Claude
chat on the account** — a bad run degrades everything else the operator is
doing. The harness refuses to start without `--i-am-the-operator`.

**Preconditions.**
- A deliberately chosen low-stakes time (expect **hours of wall clock**;
  the 5-hour rolling window plus the weekly cap both apply).
- `fledge-exp` up; operator present for the spawn gates.
- A task file (one representative, meaty prompt per line — real work, not
  toy prompts; at least `n` lines).
- Optional but recommended — the hook capture path: add a `StopFailure`
  hook (matcher `rate_limit`) to the Claude settings used by the spawned
  panes that appends the event JSON to a capture file, e.g.:

  ```json
  {"hooks": {"StopFailure": [{"matcher": "rate_limit",
    "hooks": [{"type": "command", "command": "cat >> /tmp/fledge-exp3-hooks.jsonl"}]}]}}
  ```

  Pass the same path via `--hook-capture` so the harness tails it.

**Procedure.** (run per n, low n first)
```sh
go run ./cmd/exp3-ratelimit --i-am-the-operator --n 2 --tasks exp3-tasks.txt \
    --hook-capture /tmp/fledge-exp3-hooks.jsonl --report
# then, on a later window: --n 3; then --n 4
```
1. Harness spawns n Claude panes (each spawn operator-gated), each running
   `claude "<task>"`.
2. Harness polls each pane's output for throttle markers (rate limit /
   usage limit / 429 / …) and tails the hook capture file, logging
   time-to-first-throttle per pane.
3. Stop at sustained throttling (or the watch duration); record the run
   with `--report`, or fill Results in by hand.

**Expected observations.** Some n between 2 and 4 shows sustained
throttling well before the watch window ends; the hook capture corroborates
pane-output detection.

**Flip threshold (ADR-014).** Max concurrent Claude panes = one below the
concurrency where sustained throttling first appears.

### Results

Status: **not executed** (by design — Stage 0 ships harness + protocol
only). Fields per run: date, versions, n, raw observations (per-pane
time-to-first-throttle, hook captures), verdict.

<!-- BEGIN RESULTS EXP3 -->
<!-- END RESULTS EXP3 -->
