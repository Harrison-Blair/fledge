# Integration Contracts

> Distilled from `docs/reference/integration-surfaces.md` (with in-window
> version data from `docs/reference/ai-sdlc-scan.md`), research snapshot
> 2026-07-17; re-verify version-specific claims at build time.

Each section pins the version Stage 0 was written against and carries a
`Last verified:` line. **The Stage 1 session must re-verify each surface
against the live binaries and update those lines** — every claim below is a
research-snapshot claim until then. Discrepancies go to `docs/DECISIONS.md`.

---

## Herdr — pinned v0.7.4, socket protocol v15

**Last verified:** NOT yet verified against a live binary (2026-07-18 — no
Herdr install available in the Stage 0 environment; see ADR-015). Update
this line after running `scripts/gen-herdr-types.sh` on a machine with
Herdr v0.7.4.

### Surface Fledge uses

- **Transport:** newline-delimited JSON, one request per line, over a Unix
  domain socket (named pipe on Windows). Socket resolution order: explicit
  `--session <name>` › `HERDR_SOCKET_PATH` › `HERDR_SESSION=<name>`
  (`sessions/<name>/herdr.sock`) › default session socket. Herdr injects
  `HERDR_SOCKET_PATH`, `HERDR_ENV=1`, `HERDR_WORKSPACE_ID`, `HERDR_TAB_ID`,
  `HERDR_PANE_ID` into managed panes; Herdr-managed vars beat caller env.
- **Bootstrap:** `session.snapshot` once (version/protocol metadata, focus,
  workspace/tab/pane/agent records), then `events.subscribe` and update a
  local cache from events; re-snapshot on reconnect.
- **Lifecycle:** `agent.start` (named, waitable agent pane), `pane.split`,
  `pane.read --source visible|recent|recent-unwrapped|detection`,
  `pane.send_input` (text + encoded keypresses in one request — the
  "text + real Enter" path), `pane.send_text`, `pane.send_keys`,
  `pane.wait_for_output`, `pane.close`, `worktree.create`.
- **Authority:** `pane.report_agent` (source `custom:*` seizes lifecycle
  authority and suppresses screen-manifest detection for that pane),
  `pane.report_metadata` (display-only, never seizes authority),
  `pane.clear_agent_authority` (fallback detection resumes),
  `pane.release_agent` (clean-exit: drop agent identity immediately),
  `agent.explain` (reports `screen_detection_skip_reason` when screen rules
  are non-authoritative).
- **Sequencing:** `pane.report_agent` takes an optional `seq`; for the same
  source, reports with seq ≤ the last accepted are accepted by the API but
  ignored by pane state — keep a monotonic counter per source.
- **Events:** `pane.created/updated/closed/exited/agent_detected/`
  `agent_status_changed/output_matched`, `worktree.*`, `layout.updated`;
  `events.wait` / `herdr wait agent-status <pane> --status done|blocked` as
  conveniences.

### Invocation examples

```sh
herdr agent start impl-1 --cwd ~/proj --split right -- pi
herdr pane read w1:p2 --source recent-unwrapped --lines 120
herdr wait agent-status w1:p1 --status blocked
herdr pane report-agent w1:p1 --source custom:relay --agent impl-1 --state working --seq 42
herdr pane report-metadata w1:p1 --source user:title --token summary=refactor
herdr agent explain w1:p1 --json      # check screen_detection_skip_reason
herdr api schema --json               # self-describing schema → generated types
```

### Version / stability caveats (soft spots — explicit)

- **Pre-1.0, solo-maintained, protocol-versioned** (AGPL-3.0). Breaking
  changes plausible between minor releases. Check server protocol via `ping`
  / `herdr status`; handle unknown fields gracefully.
- **Protocol bumps require a server restart** (or live handoff) before old
  clients can reattach. Upgrade runbook: upgrade Herdr → restart/handoff the
  server → run `scripts/gen-herdr-types.sh` → commit the regenerated schema
  dump and any type updates.
- **`pane.clear_agent_authority` / `pane.release_agent` semantics are sourced
  from Herdr's bundled `SOCKET_API.md`** (via a fork mirror), not the live
  docs. `gen-herdr-types.sh` checks whether the v0.7.4 schema dump documents
  them; record the finding in `docs/DECISIONS.md` (ADR-015).
- **The 32-distinct-source lifetime cap** is documented for sequenced
  token/metadata reports; whether it applies to `report_agent` state reports
  is **undocumented**. Keep source identifiers few and stable.
- Arbitration when a custom source *and* a native session hook both report on
  one Claude pane is not spelled out beyond "one authority per pane."
- `agent.explain` availability depends on server age; Windows support is
  preview (no `--remote`).

---

## Pi — pinned v0.80.x

**Last verified:** NOT yet verified against a live binary (2026-07-18 —
research snapshot only). Update after driving `pi --mode rpc` live.

### Surface Fledge uses

- **RPC mode** (`pi --mode rpc`): strict JSONL over stdin/stdout. Commands:
  `prompt` (with `streamingBehavior: "steer"|"followUp"`), `steer`,
  `follow_up`, `abort`, `new_session`, `switch_session`, `fork`, `clone`,
  `get_state`, `get_messages`, `get_entries` (append-only cursor via `since`
  — durable across restarts), `set_model`, `set_thinking_level`, `compact`,
  `set_auto_compaction`, `set_auto_retry`, `bash`. Events: `agent_start`,
  `agent_end` (with `willRetry`), `agent_settled` (fully settled — the
  transition Fledge keys on), `turn_start/end`, `message_*`,
  `tool_execution_*`, `queue_update`, `compaction_*`, `auto_retry_*`,
  `extension_error`.
- **Herdr lifecycle integration (v2):** `herdr integration install pi` writes
  `~/.pi/agent/extensions/herdr-agent-state.ts` (honors
  `PI_CODING_AGENT_DIR`). When installed, Pi authoritatively reports
  idle/working/blocked *and* a native session reference — Fledge trusts this
  and never reports custom state onto Pi panes.
- **Sessions:** `pi -c`, `pi --session <id>`, `pi --fork`, `--name`,
  `--no-session`; RPC `switch_session`/`fork`/`clone`.

### Invocation examples

```sh
herdr agent start impl-gpt --cwd ~/proj --split right -- pi --mode rpc --provider openai --model gpt-5.6
```

```json
{"id":"1","type":"prompt","message":"implement X"}
{"type":"steer","message":"also update tests"}
```

Wait for `{"type":"agent_settled"}` before advancing the FSM.

### Version / stability caveats (soft spots — explicit)

- **LF-only framing.** The RPC stream is strict JSONL split on `\n` only —
  do not use line readers that also split on U+2028/U+2029 (Node `readline`
  pitfall). Go's `bufio.Scanner` with default `ScanLines` is safe.
- **No built-in permission sandbox** — Pi runs with the launching user's
  permissions; extensions run arbitrary in-process code. Containerize if
  boundaries are needed; Fledge's role constraints are orchestrator-level.
- Herdr↔Pi lifecycle reanchoring had recent fixes (working-pane-stuck-idle
  after session replacement; startup-race retries) — pin and verify versions.
- v0.80.7 removed `openai-responses` `compat.sendSessionIdHeader` (breaking);
  session affinity now via `compat.sessionAffinityFormat`.
- Open-core under RFC 0015 (MIT core, Fair Source layers) — watch the
  licensing of any layer Fledge depends on.

---

## Claude Code — pinned ≥ v2.1.212

**Last verified:** NOT yet verified against a live binary (2026-07-18 —
research snapshot only). Update after EXP1/EXP2 run.

### Surface Fledge uses

- **Hooks** (settings.json; 30+ events): `Stop`/`SubagentStop` (completion;
  can force continuation), `Notification` (permission_prompt / idle_prompt —
  the "needs input" signal), `PreToolUse`/`PermissionRequest`/
  `PermissionDenied` (gating), `SessionStart`/`SessionEnd`,
  `UserPromptSubmit`, `PreCompact`, and a `StopFailure` matcher on
  `rate_limit` (the throttle-detection path). Command hooks receive event
  JSON on stdin (`session_id`, `cwd`, `transcript_path`); HTTP hooks receive
  it as POST body; `async: true` runs non-blocking. Stage 1's relay endpoint
  receives these POSTs — mirroring how Herdr's own Claude hook works
  (session identity only, via `pane.report_agent_session`).
- **Headless:** `claude -p "<prompt>"` with
  `--output-format text|json|stream-json` (stream-json requires
  `--verbose`); `--input-format stream-json` for bidirectional streaming;
  `--forward-subagent-text` (v2.1.211+) to capture subagent text in
  stream-json. Guardrails: `--max-turns`, `--allowedTools`,
  `--permission-mode default|acceptEdits|plan|dontAsk|bypassPermissions`,
  `--bare`.
- **Sessions:** `--session-id <uuid>` on first run, `--resume <id>` after;
  `--continue`/`-c` resumes the most recent session *in the current cwd*.
  Sessions live under `~/.claude/projects/<cwd-hash>/` — **resume must run
  from the same cwd**; in `-p` mode `--continue` may fork a new session, so
  always resume by explicit id.
- **Interactive panes:** Herdr's integration reports session identity only;
  blocked/working comes from screen manifest. Input injection must be
  `pane.send_input {text, keys:["enter"]}` — a real Enter keypress.
- **Fan-out governors** (v2.1.212): `CLAUDE_CODE_MAX_SUBAGENTS_PER_SESSION`
  and `CLAUDE_CODE_MAX_WEB_SEARCHES_PER_SESSION` (default 200 each),
  `CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY` — set explicitly in the
  orchestrator's launch env.

### Invocation examples

```sh
# interactive reviewer pane
herdr agent start reviewer --cwd ~/proj -- claude --permission-mode plan
# drive it (socket): pane.send_input {"pane_id":"w1:p3","text":"review the diff","keys":["enter"]}

# headless side task with session threading
sid=$(claude -p "start review" --output-format json | jq -r .session_id)
claude -p --resume "$sid" "now write tests"
```

### Version / stability caveats (soft spots — explicit)

- **Ink Enter-submit limitation** (issue #15553): the interactive TUI does
  not treat programmatic `\r`/`\n` as submit — text must be followed by a
  real Enter keypress; trust/`--dangerously-skip-permissions` dialogs need
  `Down`+`Enter`. Reliability is EXP2's question.
- **Self-permission-change guard (v2.1.200+):** Claude Code blocks an agent
  sending keystrokes to its own tmux/pane to change its own permissions —
  aggressive input injection can trip it, which is why EXP2's sends are
  human-triggered.
- **cwd-bound resume**; `-p --continue` may fork a new session; no published
  exit-code table (branch on zero/non-zero).
- **Rate limits are pooled and parallel-hostile:** 5-hour rolling window +
  weekly cap, shared across all Claude Code sessions and Claude chat on the
  account; subagent fan-out is the documented premature-exhaustion cause.
  Anthropic publishes multipliers, not quotas; the billing boundary for
  non-interactive usage (separate monthly credit, June 15 2026) is in flux —
  never architect around a specific token quota. Max concurrent Claude panes
  is EXP3's question.
- Auto-mode defaults changed in-window (v2.1.200–201: interactions pause by
  default; v2.1.183: destructive git commands blocked in auto mode) —
  re-check assumed defaults on every upgrade.
