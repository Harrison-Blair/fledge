# herdr API: agent methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `agent` namespace inspects and controls the coding agent recognized inside a
pane. An agent is not a separate process from the pane; it is Herdr's
classification of the terminal occupant, exposing lifecycle states (`idle`,
`working`, `blocked`, `done`, `unknown`) and a stable, follow-the-occupant name.
Every method except `agent.list`, `agent.view.set`, and `agent.view.clear` takes
a `target` that resolves to a live agent by **either** a unique live agent name
(pattern `[a-z][a-z0-9_-]{0,31}`) **or** the pane ID currently hosting that agent
(e.g. `w2:p1`); terminal IDs and bare agent-kind labels are not accepted. A name
follows the current pane occupant and is cleared when that agent exits, is
released, or is replaced. `agent.start` requires an already-existing shell pane
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

Returned (as `agent`) by `agent.get`, `agent.focus` (inferred), `agent.rename`
(inferred), `agent.start`, and `agent.prompt`, and as array elements by
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
| `agent` | string \| null | no | — | Detected agent kind (e.g. `claude`, `codex`); null if unclassified. |
| `display_agent` | string \| null | no | — | Human-facing agent label (inferred). |
| `name` | string \| null | no | — | Assigned unique agent name, or null if unnamed. |
| `title` | string \| null | no | — | Agent-reported title (inferred). |
| `cwd` | string \| null | no | — | Working directory of the pane's shell. |
| `foreground_cwd` | string \| null | no | — | Working directory of the foreground process. |
| `agent_session` | AgentSessionInfo \| null | no | — | Session reference for the agent; see below. |
| `interactive_ready` | boolean | no | — | Whether the agent is ready for interactive input (inferred). |
| `launch_pending` | boolean | no | — | Whether an `agent.start` launch is still in progress (inferred). |
| `screen_detection_skipped` | boolean | no | — | Whether screen-based detection was skipped (inferred). |
| `state_change_seq` | uint64 | no | `0` | Monotonic counter bumped on each lifecycle state change. |
| `state_labels` | object<string,string> | no | — | Detection-provided state labels (map of label key to value). |
| `terminal_title` | string \| null | no | — | Raw terminal title (OSC). |
| `terminal_title_stripped` | string \| null | no | — | Terminal title with control/markup stripped. |
| `tokens` | object<string,string> | no | — | Custom tokens (≤32 keys, key pattern `^[A-Za-z0-9_-]{1,32}$`) usable as `agent.view` fields. |

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

**Errors**: `agent_pane_not_found` (invalid/ambiguous target); other codes possible.

**CLI**: `herdr agent explain [TARGET] [--file <PATH>] [--agent <LABEL>] [--json] [--format text|json] [-v]`

**Example** (the CLI renders `explain` as text):

```text
agent: claude
state: working
manifest: remote:/home/penguin/.local/state/herdr/agent-detection/remote/claude.toml 2026.08.13.1
rule: osc_title_working (region=osc_title priority=1100)
evidence: "◐ herdr-api-documentation"
```

Validated 2026-08-19 against herdr 0.8.2.

## agent.focus

Focuses the pane hosting the target agent and marks it seen. Marking seen
transitions an unseen `done` agent's UI attention state; per skill.md, focusing
the tab or targeting the pane/agent with a focus command marks it seen, whereas
plain CLI reads do not.

**Params** (`AgentTarget`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |

**Result** — `type: "agent_info"` (inferred; no probe capture): the focused
[AgentInfo](#agentinfo) under `agent`, now reflecting `focused: true`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_info"` | yes | Result discriminator. |
| `agent` | [AgentInfo](#agentinfo) | yes | The focused agent. |

**Errors**: `agent_pane_not_found`; other codes possible.

**Events**: focusing the pane emits `pane_focused` (and, if it changes the active tab/workspace, `tab_focused`/`workspace_focused`) to subscribers.

**CLI**: `herdr agent focus <target>`

**Example**:

```json
{"id":"1","method":"agent.focus","params":{"target":"claude"}}
{"id":"1","result":{"type":"agent_info","agent":{"agent":"claude","agent_status":"working","pane_id":"w2:p1","workspace_id":"w2","tab_id":"w2:t1","focused":true,"revision":4,"terminal_id":"term_659708952f5514"}}}
```

Constructed from schema; not live-validated.

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

**Errors**: `agent_pane_not_found`; other codes possible.

**CLI**: `herdr agent get <target>`

**Example**:

```json
{"id":"cli:agent:get","method":"agent.get","params":{"target":"claude"}}
{"id":"cli:agent:get","result":{"agent":{"agent":"claude","agent_session":{"agent":"claude","kind":"id","source":"herdr:claude","value":"ef3b9d04-…"},"agent_status":"working","cwd":"/home/penguin/source/fledge","focused":true,"foreground_cwd":"/home/penguin/source/fledge","pane_id":"w2:p1","revision":4,"state_change_seq":54,"tab_id":"w2:t1","terminal_id":"term_659708952f5514","terminal_title":"◐ herdr-api-documentation","terminal_title_stripped":"herdr-api-documentation","workspace_id":"w2"},"type":"agent_info"}}
```

Validated 2026-08-19 against herdr 0.8.2.

## agent.list

Lists every live agent Herdr currently recognizes across all workspaces. Takes
no parameters. Read-only.

**Params** (`EmptyParams`): none — send `"params": {}`.

**Result** — `type: "agent_list"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_list"` | yes | Result discriminator. |
| `agents` | array<[AgentInfo](#agentinfo)> | yes | All live agents; empty array when none. |

**Errors**: none observed for well-formed requests; wire-level `invalid_request` if `params` is omitted.

**CLI**: `herdr agent list`

**Example**:

```json
{"id":"r8","method":"agent.list","params":{}}
{"id":"r8","result":{"type":"agent_list","agents":[]}}
```

Validated 2026-08-19 against herdr 0.8.2. (A populated live capture: two agents
`codex` at `w1:p1` and `claude` at `w2:p1`.)

## agent.prompt

Submits prompt text to the agent, honoring the pane's live bracketed-paste mode:
it sends the text followed by an encoded Enter after a short delay. If the agent
is already at an approval/question dialog, submission is rejected with
`agent_blocked` **before any input is sent**. When `wait` is supplied and the
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

**Errors**: `agent_blocked` (agent already at approval/question UI), `agent_prompt_stalled` (no lifecycle change within 5000 ms from a non-working start), `timeout` (wait exceeded a shorter `timeout_ms`), `agent_pane_not_found`; other codes possible.

**CLI**: `herdr agent prompt <TARGET> <TEXT> [--wait] [--until <STATUS>]... [--timeout <MS>]`

**Example**:

```json
{"id":"1","method":"agent.prompt","params":{"target":"reviewer","text":"Review the current diff.","wait":{"until":[],"timeout_ms":120000}}}
{"id":"1","result":{"type":"agent_prompted","agent":{"agent":"codex","agent_status":"idle","pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1","focused":false,"revision":588,"terminal_id":"term_6596fd32191491"}}}
```

Constructed from schema; not live-validated.

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
| `lines` | uint32 \| null | no | null | Number of rows to request; null uses the default extent. |
| `strip_ansi` | boolean | no | `true` | Strip ANSI escapes from the returned text. |

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

**Errors**: `agent_pane_not_found`; other codes possible.

**CLI**: `herdr agent read <TARGET> [--source visible|recent|recent-unwrapped|detection] [--lines <N>] [--format text|ansi] [--ansi]`

**Example** (CLI prints `read.text` directly):

```text
✽ Doing… (11m 49s · ↓ 35.1k tokens)
  ⎿  Tip: Run tasks in the cloud while you keep coding locally · clau.de/web
───────────────────────────────────────────────────── herdr-api-documentation ─
❯
```

Validated 2026-08-19 against herdr 0.8.2.

## agent.rename

Sets or clears an agent's unique name. Provide `name` (matching
`[a-z][a-z0-9_-]{0,31}`, unique among live agents) to set it, or `null` to clear
it. The name follows the pane occupant until the agent exits, is released, or is
replaced.

**Params** (`AgentRenameParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |
| `name` | string \| null | no | — | New name; `null` clears the current name. |

**Result** — `type: "agent_info"` (inferred; no probe capture): the updated
[AgentInfo](#agentinfo) under `agent`, reflecting the new `name`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_info"` | yes | Result discriminator. |
| `agent` | [AgentInfo](#agentinfo) | yes | The renamed agent. |

**Errors**: `agent_pane_not_found`; a duplicate-name error is likely when the requested name is already taken (inferred). Other codes possible.

**CLI**: `herdr agent rename <TARGET> <NAME>|--clear` (`--clear` sends `name: null`).

**Example**:

```json
{"id":"1","method":"agent.rename","params":{"target":"w1:p1","name":"reviewer"}}
{"id":"1","result":{"type":"agent_info","agent":{"agent":"codex","name":"reviewer","agent_status":"idle","pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1","focused":false,"revision":587,"terminal_id":"term_6596fd32191491"}}}
```

Constructed from schema; not live-validated.

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

**Result** — `type: "ok"` (inferred; no probe capture):

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"ok"` | yes | Acknowledges the keys were written. |

**Errors**: an invalid-key validation error rejects before writing (inferred); `agent_pane_not_found`. Other codes possible.

**CLI**: `herdr agent send-keys <TARGET> <KEY>...`

**Example**:

```json
{"id":"1","method":"agent.send_keys","params":{"target":"reviewer","keys":["esc"]}}
{"id":"1","result":{"type":"ok"}}
```

Constructed from schema; not live-validated.

## agent.start

Starts a supported interactive agent of `kind` in an existing pane identified by
`pane_id`, assigning it `name`. The pane must already be at its interactive shell
prompt with no foreground command; `agent.start` never creates, splits, or moves
layout. It returns only after Herdr detects the expected agent in that same pane
and considers it ready for interactive input. If the agent is blocked during
startup it returns `agent_not_ready` immediately, but the name stays available
for `agent.read`/`agent.send_keys`. Startup defaults to a 30-second timeout.

**Params** (`AgentStartParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `name` | string | yes | — | Unique name to assign (`[a-z][a-z0-9_-]{0,31}`). |
| `kind` | string | yes | — | Agent kind. CLI-supported values: `pi`, `claude`, `codex`, `gemini`, `cursor`, `devin`, `agy`, `cline`, `omp`, `mastracode`, `opencode`, `copilot`, `kimi`, `kiro`, `droid`, `amp`, `grok`, `hermes`, `kilo`, `qodercli`, `qwen`, `maki`. |
| `pane_id` | string | yes | — | Existing pane at an interactive shell prompt. |
| `args` | array<string> | no | `[]` | Native agent arguments (passed after `--` on the CLI). |
| `timeout_ms` | uint64 \| null | no | null (server default 30000) | Startup timeout in ms; must be greater than 3000 and at most 300000. |

**Result** — `type: "agent_started"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_started"` | yes | Result discriminator. |
| `agent` | [AgentInfo](#agentinfo) | yes | The started agent. |
| `argv` | array<string> | yes | The full argument vector Herdr launched. |

**Errors**: `agent_pane_not_found` (target pane does not exist — validated below), `agent_not_ready` (agent blocked during startup), `timeout` (startup timeout exceeded). Other codes possible.

**Events**: a successful start emits `pane_agent_detected` and `pane_agent_status_changed` to subscribers.

**CLI**: `herdr agent start <NAME> --kind <KIND> --pane <ID> [--timeout <MS>] [-- <AGENT_ARG>...]`

**Example** (error capture):

```json
{"id":"cli:agent:start","method":"agent.start","params":{"name":"reviewer","kind":"codex","pane_id":"w1:p99"}}
{"id":"cli:agent:start","error":{"code":"agent_pane_not_found","message":"agent target pane w1:p99 not found"}}
```

Validated 2026-08-19 against herdr 0.8.2.

## agent.view.clear

Deactivates the saved agent view registered for `source` (a client/view
identifier). After clearing, `active` is `false`.

**Params** (`AgentViewClearParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `source` | string \| null | no | — | The view/client identifier to clear; `null`/omitted targets the caller's default view (inferred). |

**Result** — `type: "agent_view"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"agent_view"` | yes | Result discriminator. |
| `active` | boolean | yes | Whether a view is now active (`false` after clear). |
| `source` | string \| null | no | Echoed source, when present. |
| `label` | string \| null | no | Echoed label, when present. |

**Errors**: none observed; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"a2","method":"agent.view.clear","params":{"source":"docprobe"}}
{"id":"a2","result":{"type":"agent_view","active":false}}
```

Validated 2026-08-19 against herdr 0.8.2.

## agent.view.set

Registers/activates a saved agent view for `source` (a client/view identifier):
an optional `filter` predicate and optional `sort` ordering over the live agent
list, with an optional display `label`. This drives how a client presents and
orders agents; it does not alter the agents themselves.

**Params** (`AgentViewSetParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `source` | string | yes | — | The view/client identifier this view belongs to. |
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

**Errors**: `invalid_request` for a malformed filter/sort (inferred); other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"a1","method":"agent.view.set","params":{"source":"docprobe","filter":null,"label":"probe-view"}}
{"id":"a1","result":{"type":"agent_view","active":true,"source":"docprobe","label":"probe-view"}}
```

Validated 2026-08-19 against herdr 0.8.2.

## agent.wait

Blocks until the target agent reaches one of the requested lifecycle states.
Without `until`, it matches the first settled `idle`, `done`, or `blocked` state
(the same default set as `agent.prompt`'s wait); pass `until` to match specific
states (use `unknown` explicitly when needed). Without `timeout_ms`, it waits
indefinitely. The result carries the matching event envelope.

**Params** (`AgentWaitParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | string | yes | — | Live agent name or hosting pane ID. |
| `until` | array<AgentStatus> | no | `[]` | States to match; empty means default settled set (`idle`, `done`, `blocked`). Values: `idle`, `working`, `blocked`, `done`, `unknown`. |
| `timeout_ms` | uint64 \| null | no | null | Fail after this many ms; null waits indefinitely. |

**Result** — `type: "wait_matched"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"wait_matched"` | yes | Result discriminator. |
| `event` | EventEnvelope | yes | The matched event: `{event: EventKind, data: EventData}`. For an agent state match, `event` is `pane_agent_status_changed` and `data` carries `pane_id`, `workspace_id`, and the new `agent_status`. See [../data-model.md](../data-model.md) and [../events.md](../events.md). |

**Errors**: `timeout` (no matching state within `timeout_ms`), `agent_pane_not_found`; other codes possible.

**CLI**: `herdr agent wait <TARGET> [--until <STATUS>]... [--timeout <MS>]`

**Example**:

```json
{"id":"1","method":"agent.wait","params":{"target":"reviewer","until":["blocked"],"timeout_ms":120000}}
{"id":"1","result":{"type":"wait_matched","event":{"event":"pane_agent_status_changed","data":{"type":"pane_agent_status_changed","pane_id":"w1:p1","workspace_id":"w1","agent_status":"blocked"}}}}
```

Constructed from schema; not live-validated.
