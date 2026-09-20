# herdr API: agent methods

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `agent` namespace inspects and controls the coding agent recognized inside a
pane. An agent is not a separate process from the pane; it is Herdr's
classification of the terminal occupant, exposing lifecycle states (`idle`,
`working`, `blocked`, `done`, `unknown`) and a stable, follow-the-occupant name.
Every method except `agent.list`, `agent.start`, `agent.view.set`, and
`agent.view.clear` takes a `target` that resolves to a live agent by **either**
a unique live agent name (pattern `[a-z][a-z0-9_-]{0,31}`) **or** the pane ID
currently hosting that agent (e.g. `w2:p1`); terminal IDs and bare agent-kind
labels are not accepted. `agent.start` instead identifies the pane by
`pane_id` and assigns the new agent's `name` directly; it has no `target`
field. A name follows the current pane occupant and is cleared when that
agent exits or is released (replacement by a different harness in the same
pane was not exercised). `agent.start` requires an already-existing shell pane
at its interactive prompt and never creates, splits, or moves layout. The
`agent.view.*` methods configure a per-client saved filter/sort over the agent
list and are unrelated to targeting a single agent.

12 methods:

| method | purpose |
| --- | --- |
| [agent.explain](#agentexplain) | Explain how Herdr detected/classified an agent |
| [agent.focus](#agentfocus) | Focus an agent's pane and mark it seen |
| [agent.get](#agentget) | Fetch one agent's full info |
| [agent.list](#agentlist) | List all live agents |
| [agent.prompt](#agentprompt) | Submit prompt text to an agent, optionally waiting |
| [agent.read](#agentread) | Read the agent pane's terminal output |
| [agent.rename](#agentrename) | Set or clear an agent's name |
| [agent.send_keys](#agentsend_keys) | Send logical key presses to an agent |
| [agent.start](#agentstart) | Start a supported interactive agent in a pane |
| [agent.view.clear](#agentviewclear) | Deactivate a client's saved agent view |
| [agent.view.set](#agentviewset) | Set/activate a client's saved agent view |
| [agent.wait](#agentwait) | Block until an agent reaches a requested state |

## Shared types

### AgentInfo

Returned (as `agent`) by `agent.get`, `agent.focus`, `agent.rename`,
`agent.start`, `agent.wait`, and `agent.prompt`, and as array elements by
`agent.list`. Also a domain entity in [../data-model.md](../data-model.md).

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `terminal_id` | string | yes | — | Stable internal terminal handle hosting the agent. |
| `agent_status` | enum | yes | — | Lifecycle state: `idle`, `working`, `blocked`, `done`, `unknown`. |
| `workspace_id` | string | yes | — | Workspace of the hosting pane (e.g. `w2`). |
| `tab_id` | string | yes | — | Tab of the hosting pane (e.g. `w2:t1`). |
| `pane_id` | string | yes | — | Public ID of the hosting pane (e.g. `w2:p1`). |
| `focused` | boolean | yes | — | Whether the hosting pane is UI-focused. |
| `revision` | uint64 | yes | — | Monotonic revision of the pane's output/state; increments on change. |
| `agent` | string \| null | no | — | Detected agent kind (e.g. `claude`, `codex`); omitted until classification completes. |
| `display_agent` | string \| null | no | — | Human-facing agent label (inferred). |
| `name` | string \| null | no | — | Assigned unique agent name, or null if unnamed. |
| `title` | string \| null | no | — | Agent-reported title (inferred). |
| `cwd` | string \| null | no | — | Working directory of the pane's shell. |
| `foreground_cwd` | string \| null | no | — | Working directory of the foreground process. |
| `agent_session` | AgentSessionInfo \| null | no | — | Session reference for the agent; see below. |
| `interactive_ready` | boolean | no | — | Whether the agent is ready for interactive input (inferred). |
| `launch_pending` | boolean | no | — | Whether an `agent.start` launch is still in progress (inferred). |
| `screen_detection_skipped` | boolean | no | — | Whether screen-based detection was skipped (inferred). |
| `state_change_seq` | uint64 | no | `0` | Monotonic counter bumped on a lifecycle state change; see note below. |
| `state_labels` | object<string,string> | no | — | Detection-provided state labels (map of label key to value). |
| `terminal_title` | string \| null | no | — | Raw terminal title (OSC). |
| `terminal_title_stripped` | string \| null | no | — | Terminal title with control/markup stripped. |
| `tokens` | object<string,string> | no | — | Custom tokens (≤32 keys total, key pattern `^[A-Za-z0-9_-]{1,32}$`) usable as `agent.view` fields. |

Only some of the optional fields above are schema-typed nullable
(`X \| null`, or `anyOf` a null variant for `agent_session`): `agent`,
`display_agent`, `name`, `cwd`, `foreground_cwd`, `agent_session`,
`terminal_title`, `terminal_title_stripped`, and `title`. The rest are
schema-typed as plain, non-nullable `boolean` (`interactive_ready`,
`launch_pending`, `screen_detection_skipped`) or plain `object`
(`state_labels`, `tokens`), and `state_change_seq` is a plain `integer`
(`uint64`, default `0`). Constructed from schema; not live-validated
(2026-09-19, herdr 0.9.1).

Regardless of type, herdr 0.9.1 never emits `null` for any of these fields:
an unset field is omitted from the JSON object entirely (e.g. `agent` and
`interactive_ready` are absent, not `null`, while an `agent.start` launch is
still pending). Read every optional field with a default rather than
expecting a literal `null`. Validated 2026-09-19 against herdr 0.9.1.

`state_change_seq` is a **session-global** sequence, not a per-agent counter:
it is shared by every agent in the session, so two agents never report the
same value, but a single agent's value can jump by several between reads
purely because *other* agents changed state in between.

`tokens` accumulates across sources up to the 32-key cap on `AgentInfo`
(`metadata_token_limit`), but a single `pane.report_metadata` call may set at
most 16 keys per call (`invalid_metadata_token: "a metadata report may update
at most 16 tokens"`) — send more than 16 new tokens as separate calls.
`state_labels` only accepts a closed, currently undiscoverable vocabulary:
every key tried (`phase`, `lane`, `status`, `detail`) was rejected with
`invalid_state_label`.

`screen_detection_skipped` was never observed set on any captured
`AgentInfo` on 0.9.1 — every probe that looked for a state populating it
came back without the field — even though the same key does appear,
populated, as a top-level field of `agent.explain`'s `explain` object (see
below). Its meaning and trigger condition on `AgentInfo` specifically remain
schema-derived and unconfirmed.

### AgentSessionInfo

| field | type | required | meaning |
| --- | --- | --- | --- |
| `source` | string | yes | Origin of the session reference (e.g. `herdr:claude`). |
| `agent` | string | yes | Agent kind the session belongs to. |
| `kind` | enum | yes | `id` or `path` — how `value` should be interpreted. |
| `value` | string | yes | The session identifier or path. |

## agent.explain

Explains how Herdr classified (or failed to classify) the agent in the target
pane: the detected agent kind, current state, the manifest and rule that fired,
and the matched evidence. Read-only; does not mark the pane seen.

**Params** (`AgentTarget`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |

**Result** — `type: "agent_explain"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_explain"` | yes | Result discriminator. |
| `explain` | any | yes | Free-form detection explanation (agent kind, state, manifest, rule, evidence). Schema allows any JSON value. |

On herdr 0.9.1, `explain` is consistently an 18-key object: `agent`, `state`,
`matched_rule`, `evaluated_rules` (each with its own `evidence`),
`manifest_source`, `manifest_version`, `cached_remote_version`,
`local_override_shadowing_remote`, `remote_update_status`,
`remote_update_error`, `fallback_reason`, `screen_detection_skipped`,
`skip_state_update`, `skipped_update_reason`, `visible_idle`,
`visible_blocker`, `visible_working`, `warning`.

**Errors**: `agent_not_found` (invalid/ambiguous target); other codes possible.

**CLI**: `herdr agent explain [TARGET] [--file <PATH>] [--agent <LABEL>] [--json] [--format text|json] [-v]`

**Example** (the CLI renders `explain` as text):

```text
agent: claude
state: working
manifest: remote:/home/penguin/.local/state/herdr/agent-detection/remote/claude.toml 2026.08.13.1
rule: osc_title_working (region=osc_title priority=1100)
evidence: "◐ herdr-api-documentation"
```

`-v` appends `visible:`, `cached_remote_version:`, `local_override_shadowing_remote:`,
`remote_update_status:`, and `evaluated_rules:` lines. The `--file`/`--agent`
form (no live pane) prints `rule: none` plus `fallback_reason`.

Validated 2026-09-19 against herdr 0.9.1.

## agent.focus

Focuses the pane hosting the target agent and marks it seen. Marking seen
transitions an unseen `done` agent's UI attention state; per skill.md, focusing
the tab or targeting the pane/agent with a focus command marks it seen, whereas
plain CLI reads do not.

**Params** (`AgentTarget`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |

**Result** — `type: "agent_info"`: the focused [AgentInfo](#agentinfo) under
`agent`, now reflecting `focused: true`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_info"` | yes | Result discriminator. |
| `agent` | [AgentInfo](#agentinfo) | yes | The focused agent. |

**Errors**: `agent_not_found`; other codes possible.

**Events**: focusing the pane emits `pane_focused`, `tab_focused`, and
`workspace_focused` together on every call, even when the target pane's tab
and workspace were already the active ones — `tab_focused`/`workspace_focused`
are not conditional on a change.

**CLI**: `herdr agent focus <target>`

**Example**:

```json
{"id":"1","method":"agent.focus","params":{"target":"claude"}}
{"id":"1","result":{"type":"agent_info","agent":{"agent":"claude","agent_status":"working","pane_id":"w2:p1","workspace_id":"w2","tab_id":"w2:t1","focused":true,"revision":4,"terminal_id":"term_659708952f5514"}}}
```

A real response also carries `name`, `agent_session`, `interactive_ready`,
`terminal_title`, `cwd`, `foreground_cwd`, and `state_change_seq` when they
apply, as in the [AgentInfo](#agentinfo) table above.

Validated 2026-09-19 against herdr 0.9.1.

## agent.get

Returns full [AgentInfo](#agentinfo) for one agent. Read-only; does not mark the
pane seen.

**Params** (`AgentTarget`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |

**Result** — `type: "agent_info"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_info"` | yes | Result discriminator. |
| `agent` | [AgentInfo](#agentinfo) | yes | The requested agent. |

**Errors**: `agent_not_found`; other codes possible.

**CLI**: `herdr agent get <target>`

**Example**:

```json
{"id":"cli:agent:get","method":"agent.get","params":{"target":"claude"}}
{"id":"cli:agent:get","result":{"agent":{"agent":"claude","agent_session":{"agent":"claude","kind":"id","source":"herdr:claude","value":"ef3b9d04-…"},"agent_status":"working","cwd":"/home/penguin/source/fledge","focused":true,"foreground_cwd":"/home/penguin/source/fledge","pane_id":"w2:p1","revision":4,"state_change_seq":54,"tab_id":"w2:t1","terminal_id":"term_659708952f5514","terminal_title":"◐ herdr-api-documentation","terminal_title_stripped":"herdr-api-documentation","workspace_id":"w2"},"type":"agent_info"}}
```

Validated 2026-09-19 against herdr 0.9.1.

## agent.list

Lists every live agent Herdr currently recognizes across all workspaces. Takes
no parameters. Read-only.

**Params** (`EmptyParams`): none — send `"params": {}`.

**Result** — `type: "agent_list"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_list"` | yes | Result discriminator. |
| `agents` | array<[AgentInfo](#agentinfo)> | yes | All live agents; empty array when none. |

**Errors**: none observed for well-formed requests; wire-level `invalid_request`
if `params` is omitted ("missing field `params`") or sent as `null` ("invalid
type: null, expected struct EmptyParams").

**CLI**: `herdr agent list`

**Example**:

```json
{"id":"r8","method":"agent.list","params":{}}
{"id":"r8","result":{"type":"agent_list","agents":[]}}
```

Validated 2026-09-19 against herdr 0.9.1. (A populated live capture: two agents
`codex` at `w1:p1` and `claude` at `w2:p1`.)

## agent.prompt

Submits prompt text to the agent, honoring the pane's live bracketed-paste mode:
it sends the text followed by an encoded Enter after a short delay. If the agent
is already at an approval/question dialog, submission is rejected with
`agent_blocked` **before any input is sent** — but only for a dialog the
detection manifest recognizes as blocking; a modal the manifest does not
recognize (e.g. Claude Code's Rewind menu) is still reported `idle`, so the
prompt is accepted, typed into the modal, and only fails 5.3 s later with
`agent_prompt_stalled`. When `wait` is supplied and the
agent starts from a non-`working` state, Herdr first requires an observed
lifecycle change within 5000 ms or returns `agent_prompt_stalled` (a shorter
`timeout_ms` returns `timeout` instead); it then matches the first settled
`idle`/`done`/`blocked` state, or any exact state listed in `until`. The wait
tracks lifecycle state, not a single turn: if the agent is already `working`,
completion of the active turn may satisfy it. Without `timeout_ms`, the settled
wait is indefinite.

**Params** (`AgentPromptParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |
| `text` | string | yes | — | Prompt text to submit. |
| `wait` | AgentPromptWaitOptions \| null | no | null | Wait behavior after submission; null/omitted submits without waiting. |

`AgentPromptWaitOptions`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `until` | array<AgentStatus> | no | `[]` | States to match after submission; empty means default settled set (`idle`, `done`, `blocked`). Values: `idle`, `working`, `blocked`, `done`, `unknown`. |
| `timeout_ms` | uint64 \| null | no | null | Fail after this many ms; null waits indefinitely. |

**Result** — `type: "agent_prompted"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_prompted"` | yes | Result discriminator. |
| `agent` | [AgentInfo](#agentinfo) | yes | The agent after submission (and after the wait, if requested). |

**Errors**: `empty_agent_prompt` (`text` is `""`), `agent_blocked` (agent already at approval/question UI), `agent_prompt_stalled` (no lifecycle change within 5000 ms from a non-working start), `agent_not_ready` (the target cannot currently take a prompt; measured causes include a target's `agent.start` launch still being pending and a `pane.report_agent`-only synthetic agent that is no longer the pane's foreground process, message `"agent X is no longer the pane foreground process"`), `timeout` (wait exceeded a shorter `timeout_ms`), `agent_not_found`; other codes possible.

**CLI**: `herdr agent prompt <TARGET> <TEXT> [--wait] [--until <STATUS>]... [--timeout <MS>]`

**Example**:

```json
{"id":"1","method":"agent.prompt","params":{"target":"reviewer","text":"Review the current diff.","wait":{"until":[],"timeout_ms":120000}}}
{"id":"1","result":{"type":"agent_prompted","agent":{"agent":"codex","agent_status":"idle","pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1","focused":false,"revision":588,"terminal_id":"term_6596fd32191491"}}}
```

Validated 2026-09-19 against herdr 0.9.1 (the `timeout`-vs-`agent_prompt_stalled`
race under a shorter `timeout_ms`, and an indefinite wait with `timeout_ms`
omitted, were not exercised).

## agent.read

Reads a snapshot of the agent pane's terminal output. Returns the shared
`pane_read` result (`PaneReadResult`; see [pane.md](pane.md) /
[../data-model.md](../data-model.md)). CLI reads do **not** mark the pane seen.
`lines` requests additional rows from the pane's screen and host scrollback;
rows that have left an alternate screen cannot be recovered by a larger count.

**Params** (`AgentReadParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |
| `source` | enum | yes | — | Snapshot source: `visible` (rendered viewport), `recent` (recent output incl. soft wraps), `recent_unwrapped` (soft wraps joined; best for logs/transcripts), `detection` (plain-text bottom-buffer snapshot used for agent detection). CLI defaults this to `recent`. |
| `format` | enum | no | `text` | `text` or `ansi`. Use `ansi` when colors/styling are evidence. |
| `lines` | uint32 \| null | no | null | Number of rows to request; null uses the default extent. A value smaller than the default truncates *downward* to the bottom N rows (`truncated: true`); `lines: 0` returns an empty string with `truncated: true` rather than an error. |
| `strip_ansi` | boolean | no | `true` | Documented to strip ANSI escapes from the returned text. On herdr 0.9.1 it has no observable effect in either direction: only `format` (`text` vs `ansi`) governs whether escapes appear. |

**Result** — `type: "pane_read"`, with `read` (`PaneReadResult`):

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"pane_read"` | yes | Result discriminator. |
| `read.pane_id` | string | yes | Pane the text came from. |
| `read.workspace_id` | string | yes | Workspace of the pane. |
| `read.tab_id` | string | yes | Tab of the pane. |
| `read.source` | enum | yes | Echoed source (`visible`/`recent`/`recent_unwrapped`/`detection`). |
| `read.format` | enum | yes | Echoed format (`text`/`ansi`). |
| `read.text` | string | yes | The captured terminal text. |
| `read.revision` | uint64 | yes | Pane output revision at capture time. |
| `read.truncated` | boolean | yes | Whether the snapshot was truncated. |

**Errors**: `agent_not_found`; other codes possible.

**CLI**: `herdr agent read <TARGET> [--source visible|recent|recent-unwrapped|detection] [--lines <N>] [--format text|ansi] [--ansi]`
The CLI accepts the hyphenated `recent-unwrapped` and translates it to the
API's `recent_unwrapped`; sending `recent-unwrapped` directly over the wire is
rejected with `invalid_request`. `--ansi` and `--format ansi` produce the same
escape-laden output.

**Example** (CLI prints `read.text` directly):

```text
✽ Doing… (11m 49s · ↓ 35.1k tokens)
  ⎿  Tip: Run tasks in the cloud while you keep coding locally · clau.de/web
───────────────────────────────────────────────────── herdr-api-documentation ─
❯
```

`format: "text"` reads of `recent`/`recent_unwrapped` with no (or a large)
`lines` consistently take roughly 374 ms to return; `format: "ansi"`,
`source: "detection"`, `source: "visible"`, and small `lines` values return in
under 1 ms. A caller polling `agent.read` in a loop pays that cost per call.

Validated 2026-09-19 against herdr 0.9.1.

## agent.rename

Sets or clears an agent's unique name. Provide `name` (matching
`[a-z][a-z0-9_-]{0,31}`, unique among live agents) to set it, or `null` to clear
it. The name follows the pane occupant until the agent exits or is released.

**Params** (`AgentRenameParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |
| `name` | string \| null | no | — | New name; `null` **or omitting the key** clears the current name — there is no "leave the name alone" form of this call. |

**Result** — `type: "agent_info"`: the updated [AgentInfo](#agentinfo) under
`agent`, reflecting the new `name`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_info"` | yes | Result discriminator. |
| `agent` | [AgentInfo](#agentinfo) | yes | The renamed agent. |

**Errors**: `agent_not_found`; `agent_name_taken` when the requested name is
already used by another live agent (message includes candidate
`terminal_id`/`pane_id`/`workspace_id`/`tab_id`/`cwd`/`status` for the
holder) — renaming an agent to its own current name is not a conflict and
succeeds; `agent_launch_pending` ("agent name cannot change while startup is
pending") while the target's `agent.start` launch is still pending —
independently reproduced with `name` supplied and with `name` omitted, both
denied identically. Other codes possible.

**CLI**: `herdr agent rename <TARGET> <NAME>|--clear` (`--clear` sends `name: null`).

**Example**:

```json
{"id":"1","method":"agent.rename","params":{"target":"w1:p1","name":"reviewer"}}
{"id":"1","result":{"type":"agent_info","agent":{"agent":"codex","name":"reviewer","agent_status":"idle","pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1","focused":false,"revision":587,"terminal_id":"term_6596fd32191491"}}}
```

**Undocumented destructive default**: `agent.rename {"target":"prober"}` with
no `name` key at all returns a normal `agent_info` result with the name
removed, exactly as if `name: null` had been sent — a subsequent `agent.get`
by the old name returns `agent_not_found`. Always send `name` explicitly —
though an explicit name does not avoid `agent_launch_pending`; wait for the
launch to settle first (`agent.wait`).

Validated 2026-09-19 against herdr 0.9.1.

## agent.send_keys

Sends logical key presses to the agent's terminal. All keys are validated before
any bytes are written; a single invalid key rejects the whole request. Use `esc`
as the canonical Escape name (`escape` is also accepted); chords like `ctrl+c`
are supported.

**Params** (`AgentSendKeysParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |
| `keys` | array<string> | yes | — | Ordered logical key names to send (e.g. `["esc"]`, `["ctrl+c"]`). |

**Result** — `type: "ok"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"ok"` | yes | Acknowledges the keys were written. |

**Errors**: `invalid_key` (an unsupported key name rejects the whole request before any bytes are written, e.g. `"unsupported key bogus"`); `agent_not_ready` (target is not an active named agent; measured on a `pane.report_agent`-only synthetic agent, message `"agent X is not an active named agent"`); `agent_not_found`. Other codes possible.

**CLI**: `herdr agent send-keys <TARGET> <KEY>...`

**Example**:

```json
{"id":"1","method":"agent.send_keys","params":{"target":"reviewer","keys":["esc"]}}
{"id":"1","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1.

## agent.start

Starts a supported interactive agent of `kind` in an existing pane identified by
`pane_id`, assigning it `name`. The pane must already be at its interactive shell
prompt with no foreground command; `agent.start` never creates, splits, or moves
layout. On herdr 0.9.1 it returns almost immediately after the launch begins
(three real launches measured at 0.2, 0.3, and 0.3 ms — an order of magnitude
faster than earlier trials had suggested), not once the agent is ready for
interactive input: the returned [AgentInfo](#agentinfo) reports
`agent_status: "unknown"` and `launch_pending: true`, with no `agent` key and
no `interactive_ready` key at all (detection fires roughly 300–450 ms later).
The launch settles several seconds later (~3.6 s measured) — the target either
reaches `idle` with `interactive_ready: true` and `launch_pending` cleared, or,
if it blocks on its own startup UI (e.g. a workspace-trust dialog), reaches
`agent_status: "blocked"` with `launch_pending` still `true`; `agent.start`
itself was never observed returning `agent_not_ready` in either case, and the
name stays available for `agent.read`/`agent.send_keys` throughout. Callers
that need to wait for readiness before calling `agent.prompt` must settle the
launch themselves: either call `agent.wait` (measured on 0.9.1: called
immediately after `agent.start` it blocks and returns at ~3.6 s with
`agent_status: "idle"`, after which `agent.prompt` is accepted on the first
try) or poll `agent.get` until `agent_status` leaves `unknown`. `agent.wait`
matches `agent_status`, not `launch_pending`, so callers must check the
returned status: `blocked` means the agent is at a startup dialog,
`launch_pending` is still `true`, and `agent.prompt` will return
`agent_blocked` — measured directly on a real `claude` launch that hit its
workspace-trust dialog. Startup defaults to a 30-second timeout (not
independently observable on the wire: `agent.start` always returns before the
deadline can matter, and the only observable effect of too short a deadline is
the silent name drop documented under Errors below).

**Undocumented silent data loss**: a `timeout_ms` shorter than the real launch
takes (measured: 3001 ms against a ~3.6 s `claude` startup) makes `agent.start`
return a normal `agent_started` result carrying the requested `name`, and then,
when the deadline passes, herdr silently drops the name — no error is ever
delivered. The harness process keeps running and stays reachable by pane ID
(`agent.get`/`pane.process_info` on the pane ID still work), but `agent.get`,
`agent.prompt`, and `agent.send_keys` by the now-dropped name all return
`agent_not_found` forever. The documented `agent_not_ready`/`timeout` errors
for `agent.start` (see Errors) were not observed in any trial, including this
one.

**Params** (`AgentStartParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `name` | string | yes | — | Unique name to assign (`[a-z][a-z0-9_-]{0,31}`). |
| `kind` | string | yes | — | Agent kind. CLI-supported values: `pi`, `claude`, `codex`, `gemini`, `cursor`, `devin`, `agy`, `cline`, `omp`, `mastracode`, `opencode`, `copilot`, `kimi`, `kiro`, `droid`, `amp`, `grok`, `hermes`, `kilo`, `qodercli`, `qwen`, `letta`, `maki`, `muse`. |
| `pane_id` | string | yes | — | Existing pane at an interactive shell prompt. |
| `args` | array<string> | no | `[]` | Native agent arguments (passed after `--` on the CLI). |
| `timeout_ms` | uint64 \| null | no | null (server default 30000) | Startup timeout in ms; must be greater than 3000 and at most 300000. |

**Result** — `type: "agent_started"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_started"` | yes | Result discriminator. |
| `agent` | [AgentInfo](#agentinfo) | yes | The started agent. |
| `argv` | array<string> | yes | The full argument vector Herdr launched. |

**Errors**: `unsupported_agent_kind` (`kind` not in the supported list), `invalid_agent_name` (see [AgentInfo](#agentinfo)'s name pattern), `agent_pane_not_found` (target pane does not exist — validated below), `agent_pane_busy` (`"agent target pane X is not an available shell"` — covers both a foreground command already running and the pane already hosting an agent), `invalid_agent_timeout` (`timeout_ms` not in `(3000, 300000]`, e.g. `"agent start timeout must be greater than 3000ms and at most 300000ms"`). Validation runs in that order: name → kind → pane exists → pane available → timeout — when both `kind` and `name` are invalid the server returns `invalid_agent_name`, never `unsupported_agent_kind` (independently reproduced 3 times; kind-alone and name-alone each still trigger their own distinct error in isolation). `agent_not_ready` (agent blocked during startup) and `timeout` (startup timeout exceeded) were never observed on 0.9.1 in any trial — see the silent-drop bug above and the blocked-launch behavior in the prose. Other codes possible.

**Events**: a successful start emits `pane_agent_detected` (underscored) and, on each subsequent lifecycle change, `pane.agent_status_changed` — **dotted**; subscribers must match the dotted spelling, not the underscored `pane_agent_status_changed` form.

**CLI**: `herdr agent start <NAME> --kind <KIND> --pane <ID> [--timeout <MS>] [-- <AGENT_ARG>...]`

**Example** (error capture):

```json
{"id":"cli:agent:start","method":"agent.start","params":{"name":"reviewer","kind":"codex","pane_id":"w1:p99"}}
{"id":"cli:agent:start","error":{"code":"agent_pane_not_found","message":"agent target pane w1:p99 not found"}}
```

**Example** (the silent name-drop bug):

```json
{"id":"p7","method":"agent.start","params":{"name":"shorty","kind":"claude","pane_id":"w1:p2","timeout_ms":3001,"args":["--model","claude-haiku-4-5-20251001"]}}
{"id":"p7","result":{"type":"agent_started","agent":{"terminal_id":"term_65bdd4f64788e2","name":"shorty","agent_status":"unknown","workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p2","focused":false,"launch_pending":true,"revision":2},"argv":["claude","--model","claude-haiku-4-5-20251001"]}}
```

3.6 seconds later, with no further request sent, `agent.get` for `"shorty"`
returns `agent_not_found`, while `agent.get` for `"w1:p2"` still returns the
running `claude` agent with no `name` field at all.

Validated 2026-09-19 against herdr 0.9.1 (the 30-second default `timeout_ms`
is not independently observable, and kinds other than `claude` were not
launched — see the CLI-listed kinds and the `unsupported_agent_kind` error).

## agent.view.clear

Nominally deactivates the saved agent view registered for `source` (a
client/view identifier). On herdr 0.9.1 this is **not what actually
happens**: there is a single live agent-view slot server-wide, not one
slot per `source`. `agent.view.clear` only takes effect when the `source`
it is given names that one currently-active view; called with any other
`source` — including one that was never set, or one that was validly set
earlier but is no longer the most-recently-set one — it silently no-ops
and echoes back the *other*, still-active view's `source`/`label` with
`active: true`, not the requested `source` and not `active: false` as the
method's name implies. Only clearing the single most-recently-`set` source
actually deactivates it. See the bug callout below and the corresponding
correction on [agent.view.set](#agentviewset)'s page.

**Params** (`AgentViewClearParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `source` | string \| null | no | — | The view/client identifier to clear; `null`/omitted targets the caller's default view (inferred). Validated the same as `agent.view.set`'s `source`, but see the bug below: a `source` that does not name the single currently-active view is ignored rather than cleared. |

**Result** — `type: "agent_view"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_view"` | yes | Result discriminator. |
| `active` | boolean | yes | `false` only when the request's `source` named the view that was actually active; otherwise `true`, describing the still-active *other* view (see bug below). |
| `source` | string \| null | no | Echoes the still-active view's `source` when the clear no-oped — not necessarily the requested `source`. |
| `label` | string \| null | no | Echoes the still-active view's `label` under the same condition. |

**Errors**: `invalid_agent_view` for a `source` that fails the same validation
`agent.view.set` applies (see below), e.g. a 121-character source: `"agent
view source must be non-empty, at most 120 characters, and contain only
ASCII letters, digits, colon, dot, underscore, or hyphen"`. A `source` that
was never set, or one that is no longer the active view, does not raise an
error — it is indistinguishable on the wire from a successful clear of a
different view (see bug below). Other codes possible.

**Undocumented cross-source bug**: `agent.view.clear` does not clear by
`source`; it clears a single server-wide slot regardless of which `source`
is named, and only reports `active: false` when the named `source` happens
to be the one occupying that slot. Reproduced 3 times: `agent.view.set`
`source: bugA`, then `agent.view.set` `source: bugB`, then
`agent.view.clear` `source: bugA` returns
`{"type":"agent_view","active":true,"source":"bugB","label":"B"}` — the
unrelated, still-active `bugB` view, not `active: false` for `bugA`. A
following `agent.view.clear` `source: bugB` (the view actually holding the
slot) then correctly returns `{"type":"agent_view","active":false}`. The
same pattern reproduced with `viewX`/`viewY` and with a `source` that was
never set at all (`onlyOne`/`differentNeverSet`): clearing anything other
than the single currently-active `source` always echoes that active view
back with `active: true` instead of acting on, or reporting on, the
requested `source`.

**CLI**: API-only (no CLI subcommand).

**Example** (clearing the active view):

```json
{"id":"a2","method":"agent.view.clear","params":{"source":"docprobe"}}
{"id":"a2","result":{"type":"agent_view","active":false}}
```

**Example** (the cross-source bug: `bugA` is set, then `bugB` is set, then
clearing `bugA` echoes back the still-active `bugB` instead of
deactivating, or reporting on, `bugA`):

```json
{"id":"bug3","method":"agent.view.clear","params":{"source":"bugA"}}
{"id":"bug3","result":{"type":"agent_view","active":true,"source":"bugB","label":"B"}}
```

Validated 2026-09-19 against herdr 0.9.1.

## agent.view.set

Registers/activates a saved agent view for `source` (a client/view identifier):
an optional `filter` predicate and optional `sort` ordering over the live agent
list, with an optional display `label`. This drives how a client presents and
orders agents; it does not alter the agents themselves. A view set on one
connection is visible to a `set`/`clear` from any other connection to the
same server, so views are not keyed by the connection that sent them. On
herdr 0.9.1, though, `source` does not key independent, coexisting per-source
storage either: `agent.view.clear`'s behavior (see its bug callout) shows a
single live view slot server-wide, and the last `set` call — for any
`source` — is the one that wins, replacing whatever view previously occupied
that slot. `agent.view.set`'s own result only ever echoes back the `source`
and `label` you just sent, which is not by itself evidence that per-source
views coexist independently. No `agent.list` or other agent result reflects
which view is currently active, so a caller cannot read back the live view's
filter/sort/label/source short of a `clear` call that happens to target it.

**Params** (`AgentViewSetParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `source` | string | yes | — | The view/client identifier this view belongs to. Must be non-empty, at most 120 characters, and contain only ASCII letters, digits, `:`, `.`, `_`, or `-` (`invalid_agent_view` otherwise). |
| `filter` | AgentViewFilter \| null | no | null | Predicate selecting which agents appear; null shows all. |
| `sort` | array<AgentViewSort> | no | `[]` | Ordered sort keys applied to the filtered agents. |
| `label` | string \| null | no | null | Human-facing label for the view. |

**`AgentViewFilter`** — one of (discriminated by `op`):

| `op` | shape | meaning |
| --- | --- | --- |
| `all` | `{op, filters: AgentViewFilter[]}` | Logical AND of subfilters. |
| `any` | `{op, filters: AgentViewFilter[]}` | Logical OR of subfilters. |
| `not` | `{op, filter: AgentViewFilter}` | Negation of a subfilter. |
| `eq` | `{op, field: AgentViewField, value: AgentViewValue}` | Field equals value. |
| `in` | `{op, field: AgentViewField, values: AgentViewValue[]}` | Field is one of values. |
| `exists` | `{op, field: AgentViewField}` | Field is present. |

**`AgentViewField`** — either a builtin field name (enum: `status`,
`workspace_id`, `tab_id`, `pane_id`, `agent`, `seen`, `state_change_seq`) or a
custom-token reference object `{"token": "<name>"}` matching an
[AgentInfo](#agentinfo) `tokens` key.

**`AgentViewValue`** — one of: `string`, `boolean`, `uint64` integer, or a
context object `{"context": "current_workspace_id" | "current_tab_id"}` that
resolves to the caller's current workspace/tab at evaluation time.

**`AgentViewSort`**:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `field` | AgentViewSortField | yes | — | Sort key. |
| `order` | enum | no | `asc` | `asc` or `desc`. |

**`AgentViewSortField`** — either a builtin sort field (enum: `workspace_order`,
`tab_order`, `pane_order`, `attention`, `status`, `agent`, `seen`,
`state_change_seq`) or a custom-token reference object `{"token": "<name>"}`.

**Result** — `type: "agent_view"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_view"` | yes | Result discriminator. |
| `active` | boolean | yes | Whether the view is now active (`true` after set). |
| `source` | string \| null | no | Echoed source. |
| `label` | string \| null | no | Echoed label. |

**Errors**: `invalid_request` for a malformed filter/sort (e.g. an unknown `op`, an unknown `AgentViewField`, or a bad sort `order`); `invalid_agent_view` for a `source` that fails the validation above. Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"a1","method":"agent.view.set","params":{"source":"docprobe","filter":null,"label":"probe-view"}}
{"id":"a1","result":{"type":"agent_view","active":true,"source":"docprobe","label":"probe-view"}}
```

Validated 2026-09-19 against herdr 0.9.1.

## agent.wait

Blocks until the target agent reaches one of the requested lifecycle states.
Without `until`, it matches the first settled `idle`, `done`, or `blocked` state
(the same default set as `agent.prompt`'s wait); pass `until` to match specific
states (use `unknown` explicitly when needed). Without `timeout_ms`, it waits
indefinitely. The wait is level-triggered: if the agent is already in a
matching state when the call is made, it returns immediately (0.2 ms measured
on an already-idle agent). On herdr 0.9.1 the result carries a full
[AgentInfo](#agentinfo) snapshot, the same shape as `agent.get`'s result, not
an event envelope — the schema names the result type `wait_matched` with an
`event` field, but the measured 0.9.1 response is `type: "agent_info"` with an
`agent` field.

**Params** (`AgentWaitParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |
| `until` | array<AgentStatus> | no | `[]` | States to match; empty means default settled set (`idle`, `done`, `blocked`). Values: `idle`, `working`, `blocked`, `done`, `unknown`. |
| `timeout_ms` | uint64 \| null | no | null | Fail after this many ms; null waits indefinitely. |

**Result** — `type: "agent_info"` (measured; the schema names this result
`wait_matched` with an `event: EventEnvelope` field instead — see note above):

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_info"` | yes | Result discriminator. |
| `agent` | [AgentInfo](#agentinfo) | yes | The agent in its matched state. |

**Errors**: `timeout` (no matching state within `timeout_ms`), `agent_not_found`, `agent_not_running` (`"agent is no longer running in the target pane"` — the agent process exits while the wait is blocked; measured 2502 ms into a 30 s wait). Other codes possible.

**CLI**: `herdr agent wait <TARGET> [--until <STATUS>]... [--timeout <MS>]`

**Example**:

```json
{"id":"1","method":"agent.wait","params":{"target":"reviewer","timeout_ms":15000}}
{"id":"1","result":{"agent":{"agent":"claude","agent_session":{"agent":"claude","kind":"id","source":"herdr:claude","value":"0187e81f-…"},"agent_status":"idle","cwd":"/home/penguin/source/fledge","focused":false,"foreground_cwd":"/home/penguin/source/fledge","interactive_ready":true,"name":"reviewer","pane_id":"w1:p1","revision":2,"state_change_seq":169,"tab_id":"w1:t1","terminal_id":"term_65bcdef99050e20","terminal_title":"✳ Claude Code","terminal_title_stripped":"Claude Code","workspace_id":"w1"},"type":"agent_info"}}
```

Validated 2026-09-19 against herdr 0.9.1 (the `timeout` and `blocked` paths are
now exercised, in addition to the success path shown here; a wait with
`timeout_ms` omitted was not exercised, since it would block indefinitely).
