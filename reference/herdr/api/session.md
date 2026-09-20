# herdr API: session methods

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `session` namespace exposes the two read-only, whole-session introspection methods. `ping` is the liveness and version handshake: it returns the server version, protocol number, and the server's advertised capabilities. `session.snapshot` returns the complete, point-in-time model of the running session — every workspace, tab, pane, pane layout, and detected agent. Both methods are non-mutating and take empty params. Neither method emits events. As with all herdr methods, the server closes the connection after a single response (see [protocol.md](../protocol.md)).

Throughout this page a field typed `X | null` is quoting the JSON schema. On the wire, herdr 0.9.1 never emits `null` for any of them: an unset optional field is omitted from the JSON object entirely. No `null` value appeared anywhere in roughly fifty snapshots taken across an empty session, a populated one, and a session with agents mid-launch. Read every optional field with a default rather than expecting a `null`.

2 methods:

| method | purpose |
| --- | --- |
| [`ping`](#ping) | Liveness/version handshake; returns server version, protocol, and capabilities. |
| [`session.snapshot`](#sessionsnapshot) | Return the full live session model (workspaces, tabs, panes, layouts, agents). |

## ping

Liveness and version-negotiation handshake. Returns the running server's semantic version, its wire-protocol number, and (when the server supports the capabilities field) a `ServerCapabilities` object describing optional server features. A client should call `ping` first to confirm the socket is a live herdr server and that its `protocol` matches the client's expectation before issuing other methods. Non-mutating; emits no events.

**Params** — `PingParams` (empty object; send `{}`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| _(none)_ | — | — | — | No parameters. Send `"params": {}`. |

The `params` key itself is mandatory: omitting it fails with `invalid_request` / ``invalid request: missing field `params` at line 1 column 27``. Beyond that the server is tolerant — unknown keys inside `params` are ignored, unknown keys in the request envelope are ignored, and even `"params": []` is accepted and answered with a normal pong — but `"params": null` and `"params": "x"` are rejected with `invalid_request` / `invalid type: …, expected struct PingParams`.

**Result** — `result.type` = `"pong"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"pong"` | yes | — | Result discriminator. |
| `version` | string | yes | — | Server semantic version (e.g. `"0.9.1"`). |
| `protocol` | integer (uint32) | yes | — | Wire-protocol number the server speaks (e.g. `22`). |
| `capabilities` | `ServerCapabilities` \| null | no | `null` | Optional server feature flags. See below. herdr 0.9.1 always reports the object, so the schema's `null` case was not reachable. |

`ServerCapabilities`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `live_handoff` | boolean | yes | — | Server supports live handoff of an attached client between processes. |
| `detached_server_daemon` | boolean | no | `false` | Server runs as a detached background daemon. |
| `endpoint_protocol_generation` | integer (uint32) \| null | no | — | Stable client-owned endpoint generation supported by this server. |
| `health_check` | boolean | no | `false` | Whether this server supports endpoint health probes. |
| `surface_interest` | boolean | no | `false` | Whether this server supports explicit client-shell surface interest. |

herdr 0.9.1 emits all five fields on every `ping`, so none of the documented defaults is exercised in practice. `live_handoff`, `endpoint_protocol_generation`, `health_check`, and `surface_interest` were `true`/`1` on every server probed. `detached_server_daemon` is the one flag observed both ways on the same binary: `false` for a foreground `herdr --session <name> server`, `true` for a daemonized session — a client can use it to tell an ad-hoc scratch server from a persistent one.

**Errors**: No error codes observed for `ping`; malformed envelopes fail at the protocol layer (see [protocol.md](../protocol.md)). Other codes possible. Every envelope-level error answers with `"id":""` rather than echoing the request id, because the envelope fails to deserialize before the id is read — a client cannot correlate a malformed request to its error response by id.

**CLI**: API-only (no CLI subcommand). `herdr api ping` exits 2 with a usage message; `herdr api` offers only `snapshot` and `schema`.

**Example** — `Validated 2026-09-19 against herdr 0.9.1. (The schema's capabilities:null case was not reachable on this build.)`

```json
{"id":"m1","method":"ping","params":{}}
{"id":"m1","result":{"type":"pong","version":"0.9.1","protocol":22,"capabilities":{"live_handoff":true,"detached_server_daemon":false,"endpoint_protocol_generation":1,"surface_interest":true,"health_check":true}}}
```

## session.snapshot

Return the complete, point-in-time model of the running session: its focus pointers, and the full lists of workspaces, tabs, panes, pane layouts, and detected agents. This is the primary read source for a client that needs to enumerate or reconcile session state. Non-mutating; emits no events. The returned `snapshot` also carries its own `version` and `protocol` fields (identical to `ping`'s), so a single `session.snapshot` call doubles as a version check. The call is cheap and flat — 0.144–0.200 ms over ten calls on a 2-workspace / 3-tab / 5-pane / 3-agent session, against 0.023–0.045 ms for `ping` — so polling it is affordable; there is no cheaper change-detector, because `PaneInfo.revision` does not track most changes (see below).

**Params** — `EmptyParams` (empty object; send `{}`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| _(none)_ | — | — | — | No parameters. Send `"params": {}`. |

The `params` key is mandatory here too, same as `ping`.

**Result** — `result.type` = `"session_snapshot"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"session_snapshot"` | yes | — | Result discriminator. |
| `snapshot` | `SessionSnapshot` | yes | — | The full session model. See below. |

### `SessionSnapshot`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `version` | string | yes | — | Server semantic version. |
| `protocol` | integer (uint32) | yes | — | Wire-protocol number. |
| `workspaces` | `WorkspaceInfo[]` | yes | — | All workspaces in the session. |
| `tabs` | `TabInfo[]` | yes | — | All tabs across all workspaces. |
| `panes` | `PaneInfo[]` | yes | — | All panes across all tabs. |
| `layouts` | `PaneLayoutSnapshot[]` | yes | — | Per-tab pane geometry/layout. |
| `agents` | `AgentInfo[]` | yes | — | Detected agents (subset of panes that host an agent); empty array when none. |
| `focused_workspace_id` | string \| null | no | — | ID of the focused workspace. Present only when something is focused; the key is absent otherwise, never emitted as `null`. |
| `focused_tab_id` | string \| null | no | — | ID of the focused tab. Present only when something is focused; the key is absent otherwise. |
| `focused_pane_id` | string \| null | no | — | ID of the focused pane. Present only when something is focused; the key is absent otherwise. |

A headless `herdr server` with no attached client starts empty: `workspaces`, `tabs`, `panes`, `layouts`, and `agents` are all `[]` and all three focus pointers are absent, leaving `version` and `protocol` as the only populated fields. Closing every workspace returns the snapshot to exactly that shape. A client that assumes at least one workspace exists, or that indexes `snapshot["focused_pane_id"]` directly, breaks against a freshly started server.

### `WorkspaceInfo`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | Workspace ID (e.g. `"w1"`). |
| `number` | integer (uint) | yes | — | 1-based workspace ordinal. |
| `label` | string | yes | — | Display label. |
| `focused` | boolean | yes | — | Whether this workspace is focused. |
| `pane_count` | integer (uint) | yes | — | Number of panes in the workspace. |
| `tab_count` | integer (uint) | yes | — | Number of tabs in the workspace. |
| `active_tab_id` | string | yes | — | ID of the workspace's active tab. |
| `agent_status` | `AgentStatus` | yes | — | Aggregate agent status for the workspace. |
| `tokens` | map<string,string> | no | — | Arbitrary key/value tokens; ≤32 entries accumulated across sources, keys match `^[A-Za-z0-9_-]{1,32}$` (e.g. `{"branch":"main"}`). Absent until metadata is reported. A single `workspace.report_metadata` call may set at most 16. |
| `worktree` | `WorkspaceWorktreeInfo` \| null | no | — | Git worktree binding for the workspace. Absent until an explicit `worktree.open` or `worktree.create`; creating a workspace with a `cwd` inside a git repo does not populate it. |

### `WorkspaceWorktreeInfo`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `repo_key` | string | yes | — | Stable key identifying the repository (typically its `.git` path). |
| `repo_name` | string | yes | — | Repository name. |
| `repo_root` | string | yes | — | Absolute path to the repository root. |
| `checkout_path` | string | yes | — | Absolute path of this workspace's checkout. |
| `is_linked_worktree` | boolean | yes | — | True when `checkout_path` is a linked git worktree rather than the main checkout. |

### `TabInfo`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `tab_id` | string | yes | — | Tab ID (e.g. `"w1:t1"`). |
| `workspace_id` | string | yes | — | Owning workspace ID. |
| `number` | integer (uint) | yes | — | 1-based tab ordinal within the workspace. |
| `label` | string | yes | — | Display label. |
| `focused` | boolean | yes | — | Whether this tab is focused. Focus is session-global: at most one tab in the whole session reports `true`. For a workspace's own active tab, read that workspace's `active_tab_id` instead. |
| `pane_count` | integer (uint) | yes | — | Number of panes in the tab. |
| `agent_status` | `AgentStatus` | yes | — | Aggregate agent status for the tab. |

### `PaneInfo`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `pane_id` | string | yes | — | Pane ID (e.g. `"w1:p1"`). |
| `terminal_id` | string | yes | — | Underlying terminal ID (e.g. `"term_65970bc8958f71"`). |
| `workspace_id` | string | yes | — | Owning workspace ID. |
| `tab_id` | string | yes | — | Owning tab ID. |
| `focused` | boolean | yes | — | Whether this pane is focused. |
| `agent_status` | `AgentStatus` | yes | — | Agent status for the pane. |
| `revision` | integer (uint64) | yes | — | Monotonic revision counter. It is not a general change counter: the only bumps observed (1 → 2) were on the two panes that acquired an agent session identity. See the note below. |
| `agent` | string \| null | no | — | Detected agent label (e.g. `"claude"`); absent when unset. |
| `display_agent` | string \| null | no | — | Human-facing agent label; absent when unset. |
| `agent_session` | `AgentSessionInfo` \| null | no | — | Reported agent session identity; absent when unset. |
| `cwd` | string \| null | no | — | Pane shell working directory; absent when unset. |
| `foreground_cwd` | string \| null | no | — | Working directory of the foreground process; absent when unset. |
| `label` | string \| null | no | — | User-assigned pane label (set by `pane.rename`); absent when unset. |
| `title` | string \| null | no | — | Pane title; absent when unset. |
| `terminal_title` | string \| null | no | — | Raw terminal title; absent when unset. |
| `terminal_title_stripped` | string \| null | no | — | Terminal title with control/decoration stripped; absent when unset. |
| `scroll` | `PaneScrollInfo` \| null | no | — | Scrollback/viewport position; present on every pane observed. |
| `state_labels` | map<string,string> | no | — | State labels the server recognizes. Keys come from a configured allowlist, not free-form: the write path rejects anything else with `invalid_state_label` / `unknown state label: <key>`. |
| `tokens` | map<string,string> | no | — | Arbitrary key/value tokens; ≤32 entries, keys match `^[A-Za-z0-9_-]{1,32}$`. Absent until metadata is reported; a single report may set at most 16. |

> Note: `revision` does not track pane activity. It stayed at 1 across 500 lines of terminal output, a scroll to offset 37 and a clamp to 462, `pane.rename`, `pane.report_metadata` (title), `pane.report_agent`, `pane.report_agent_session` with `agent_session_path`, `pane.zoom` on and off, and a split-ratio change. Do not use it to detect that a pane changed; re-read the snapshot instead.

> Note: `state_labels` was absent from every `PaneInfo` observed. A default config defines no valid keys — thirteen plausible ones (`plan`, `mode`, `status`, `model`, `state`, `context`, `cost`, `tokens`, `branch`, `queue`, `permission`, `thinking`, `output_style`) were all rejected — so the field can only be populated on a config that declares the keys. The allowlist is not discoverable from the API.

### `PaneScrollInfo`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `offset_from_bottom` | integer (uint64) | yes | — | Current scroll offset from the bottom (0 = pinned to newest output). |
| `max_offset_from_bottom` | integer (uint64) | yes | — | Maximum scrollable offset from the bottom. |
| `viewport_rows` | integer (uint64) | yes | — | Number of rows in the visible viewport. |

### `AgentInfo`

One entry per pane that hosts a detected agent; panes without an agent are omitted from the `agents` array (three of five panes in the probe session). The shape is close to `PaneInfo` but not identical: `AgentInfo` adds `name`, `interactive_ready`, `launch_pending`, `screen_detection_skipped`, and `state_change_seq`, and has no `label` or `scroll`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `terminal_id` | string | yes | — | Underlying terminal ID. |
| `agent_status` | `AgentStatus` | yes | — | Agent status. |
| `workspace_id` | string | yes | — | Owning workspace ID. |
| `tab_id` | string | yes | — | Owning tab ID. |
| `pane_id` | string | yes | — | Owning pane ID. |
| `focused` | boolean | yes | — | Whether the hosting pane is focused. |
| `revision` | integer (uint64) | yes | — | Monotonic revision counter. |
| `agent` | string \| null | no | — | Detected agent label (e.g. `"claude"`); absent until detection has run — see the launch-race note below. |
| `display_agent` | string \| null | no | — | Human-facing agent label; absent when unset. |
| `agent_session` | `AgentSessionInfo` \| null | no | — | Reported agent session identity; absent when unset. |
| `cwd` | string \| null | no | — | Shell working directory; absent when unset. |
| `foreground_cwd` | string \| null | no | — | Foreground process working directory; absent when unset. |
| `name` | string \| null | no | — | Agent name. Set only for herdr-launched agents (`agent.start --name`); absent for agents herdr learned about through `pane.report_agent`, so it is not a universal agent key. |
| `title` | string \| null | no | — | Pane/agent title; absent when unset. |
| `terminal_title` | string \| null | no | — | Raw terminal title; absent when unset. |
| `terminal_title_stripped` | string \| null | no | — | Terminal title with decoration stripped; absent when unset. |
| `interactive_ready` | boolean | no | `false` | True when the agent is ready to receive interactive input. Emitted only when true; the key is absent otherwise. |
| `launch_pending` | boolean | no | `false` | True while an agent launch is still in progress. Emitted only when true; the key disappears once the launch settles, it does not become `false`. |
| `screen_detection_skipped` | boolean | no | `false` | True when screen-based agent detection was skipped for this pane. Emitted only when true; never observed on 0.9.1. |
| `state_change_seq` | integer (uint64) | no | `0` | Sequence number incremented on each agent state change. Always emitted, including as an explicit `0` on a freshly started agent. |
| `state_labels` | map<string,string> | no | — | State labels the server recognizes; same allowlisted keys as `PaneInfo.state_labels`. |
| `tokens` | map<string,string> | no | — | Arbitrary key/value tokens; ≤32 entries, keys match `^[A-Za-z0-9_-]{1,32}$`. |

> Note: `terminal_id`, `agent_status`, `workspace_id`, `tab_id`, `pane_id`, `focused`, and `revision` are the schema's required fields and were present on every agent entry observed. `interactive_ready`, `launch_pending`, and `screen_detection_skipped` are non-nullable booleans in the schema and are not in its `required` set; herdr 0.9.1 emits each of them only when it is true. A settled agent carries `interactive_ready: true` with no `launch_pending` key at all, agents reported through `pane.report_agent` carry none of the three, and `screen_detection_skipped` was never emitted in roughly fifty snapshots. Read all three with a `false` default; indexing them directly raises on most agents.

> Note: there is a detection race after `agent.start`. The call itself returns in well under a millisecond, and the `agents[]` entry appears immediately with `name` and `launch_pending: true` but no `agent` label and `agent_status: "unknown"`. The `agent` label appeared by about one second; `agent_status` reached `blocked` while claude sat on its first-run trust prompt; `interactive_ready`, `agent_session`, and `agent_status: "idle"` arrived only about two seconds after that prompt was answered. `launch_pending` can therefore stay true well past the usual settle time. A client polling `session.snapshot` for a just-started agent must key on `pane_id` or `name`, never on `agent`.

A settled herdr-launched agent, captured live (paths shortened):

```json
{"terminal_id":"term_65bdd4e2702433","name":"rvagent","agent":"claude","terminal_title":"✳ Claude Code","terminal_title_stripped":"Claude Code","agent_status":"idle","agent_session":{"source":"herdr:claude","agent":"claude","kind":"id","value":"dfc9756d-87bd-4dd5-bf6a-8dadbb121ec5"},"workspace_id":"w1","tab_id":"w1:t2","pane_id":"w1:p2","focused":false,"interactive_ready":true,"state_change_seq":3,"cwd":"…/scratch-repo","foreground_cwd":"…/scratch-repo","revision":2}
```

### `AgentSessionInfo`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `source` | string | yes | — | Origin of the session identity (e.g. `"herdr:claude"`). |
| `agent` | string | yes | — | Agent label the session belongs to. |
| `kind` | `AgentSessionRefKind` | yes | — | Reference kind: `id` or `path`. |
| `value` | string | yes | — | The session ID or path value, per `kind`. |

`AgentSessionRefKind` enum: `id`, `path`. Only `id` was observed in a snapshot, both for a herdr-detected claude session (`{"source":"herdr:claude","agent":"claude","kind":"id","value":"dfc9756d-87bd-4dd5-bf6a-8dadbb121ec5"}`) and for one reported through `pane.report_agent_session`'s `agent_session_id`. The `path` variant is schema-derived; it is reachable through that method's `agent_session_path`, but no capture isolated it.

### `PaneLayoutSnapshot`

Per-tab pane geometry. One entry per tab.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | Owning workspace ID. |
| `tab_id` | string | yes | — | Tab this layout describes. |
| `zoomed` | boolean | yes | — | True when a pane is zoomed to fill the tab. |
| `area` | `PaneLayoutRect` | yes | — | The tab's total layout area. |
| `focused_pane_id` | string | yes | — | ID of the focused pane in this tab. |
| `panes` | `PaneLayoutPane[]` | yes | — | Placed panes with their rectangles. |
| `splits` | `PaneLayoutSplit[]` | yes | — | Split nodes dividing the area; empty for a single-pane tab. |

> Note: when `zoomed` is true the pane rectangles do **not** change — every pane in the tab keeps its un-zoomed `rect`, and `area` is unchanged too. A client rendering from `layouts[].panes[].rect` draws the un-zoomed tiling unless it checks `zoomed` and special-cases `focused_pane_id` to fill `area` itself. Also note `focused_pane_id` here is per-tab (the tab's own focused pane), unlike `SessionSnapshot.focused_pane_id`, which is session-global.

### `PaneLayoutPane`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `pane_id` | string | yes | — | Pane ID. |
| `focused` | boolean | yes | — | Whether this pane is focused. |
| `rect` | `PaneLayoutRect` | yes | — | Pane rectangle within the layout area. |

### `PaneLayoutSplit`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `id` | string | yes | — | Split node ID. |
| `direction` | `SplitDirection` | yes | — | Split orientation: `right` or `down`. |
| `ratio` | number (float) | yes | — | Split ratio between the two children. |
| `rect` | `PaneLayoutRect` | yes | — | Rectangle covered by this split node. |

`SplitDirection` enum: `right`, `down`.

### `PaneLayoutRect`

All fields are integers (uint16, range 0–65535), measured in terminal cells.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `x` | integer (uint16) | yes | — | Left column. |
| `y` | integer (uint16) | yes | — | Top row. |
| `width` | integer (uint16) | yes | — | Width in cells. |
| `height` | integer (uint16) | yes | — | Height in cells. |

### `AgentStatus`

Enum used by `WorkspaceInfo`, `TabInfo`, `PaneInfo`, and `AgentInfo`: `idle`, `working`, `blocked`, `done`, `unknown`. `done` is read-only: the write-side `PaneAgentState` enum accepted by `pane.report_agent` has only four values (`idle`, `working`, `blocked`, `unknown`) and rejects `"done"` with `unknown variant \`done\``, so an external reporter can never set it.

**Errors**: No error codes observed for `session.snapshot`. Other codes possible.

**CLI**: `herdr api snapshot` — prints the *full response envelope* `{"id":...,"result":{"type":"session_snapshot","snapshot":{...}}}` as one line of JSON, not the bare `snapshot` object. The snapshot itself is at `.result.snapshot`; piping straight to `jq '.workspaces'` returns `null`.

**Example** — `Validated 2026-09-19 against herdr 0.9.1.` (arrays truncated with `…`; structure intact; focus pointers are shown present, which holds only while something is focused — see the note above)

```json
{"id":"r2","method":"session.snapshot","params":{}}
{"id":"r2","result":{"type":"session_snapshot","snapshot":{"version":"0.9.1","protocol":22,"focused_workspace_id":"w1","focused_tab_id":"w1:t1","focused_pane_id":"w1:p1","workspaces":[{"workspace_id":"w1","number":1,"label":"--label docs-ws-renamed","focused":true,"pane_count":3,"tab_count":3,"active_tab_id":"w1:t1","agent_status":"unknown","tokens":{"branch":"main"},"worktree":{"repo_key":"…/scratch-repo/.git","repo_name":"scratch-repo","repo_root":"…/scratch-repo","checkout_path":"…/scratch-repo","is_linked_worktree":false}},…],"tabs":[{"tab_id":"w1:t1","workspace_id":"w1","number":1,"label":"1","focused":true,"pane_count":1,"agent_status":"unknown"},…],"panes":[{"pane_id":"w1:p1","terminal_id":"term_65970bc8958f71","workspace_id":"w1","tab_id":"w1:t1","focused":true,"cwd":"…/scratch-repo","foreground_cwd":"…/scratch-repo","terminal_title":"penguin@raft: …","terminal_title_stripped":"penguin@raft: …","agent_status":"unknown","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":0,"viewport_rows":39},"revision":1},…],"layouts":[{"workspace_id":"w1","tab_id":"w1:t1","zoomed":false,"area":{"x":26,"y":1,"width":94,"height":39},"focused_pane_id":"w1:p1","panes":[{"pane_id":"w1:p1","focused":true,"rect":{"x":26,"y":1,"width":94,"height":39}}],"splits":[]},…],"agents":[]}}}
```
