# herdr API: session methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `session` namespace exposes the two read-only, whole-session introspection methods. `ping` is the liveness and version handshake: it returns the server version, protocol number, and the server's advertised capabilities. `session.snapshot` returns the complete, point-in-time model of the running session — every workspace, tab, pane, pane layout, and detected agent. Both methods are non-mutating and take empty params. Neither method emits events. As with all herdr methods, the server closes the connection after a single response (see [protocol.md](../protocol.md)).

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

**Result** — `result.type` = `"pong"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"pong"` | yes | — | Result discriminator. |
| `version` | string | yes | — | Server semantic version (e.g. `"0.8.2"`). |
| `protocol` | integer (uint32) | yes | — | Wire-protocol number the server speaks (e.g. `20`). |
| `capabilities` | `ServerCapabilities` \| null | no | `null` | Optional server feature flags; `null` when the server does not report capabilities. See below. |

`ServerCapabilities`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `live_handoff` | boolean | yes | — | Server supports live handoff of an attached client between processes. |
| `detached_server_daemon` | boolean | no | `false` | Server runs as a detached background daemon. |

**Errors**: No error codes observed for `ping`; malformed envelopes fail at the protocol layer (see [protocol.md](../protocol.md)). Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — `Validated 2026-08-19 against herdr 0.8.2.`

```json
{"id":"m1","method":"ping","params":{}}
{"id":"m1","result":{"type":"pong","version":"0.8.2","protocol":20,"capabilities":{"live_handoff":true,"detached_server_daemon":false}}}
```

## session.snapshot

Return the complete, point-in-time model of the running session: its focus pointers, and the full lists of workspaces, tabs, panes, pane layouts, and detected agents. This is the primary read source for a client that needs to enumerate or reconcile session state. Non-mutating; emits no events. The returned `snapshot` also carries its own `version` and `protocol` fields (identical to `ping`'s), so a single `session.snapshot` call doubles as a version check.

**Params** — `EmptyParams` (empty object; send `{}`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| _(none)_ | — | — | — | No parameters. Send `"params": {}`. |

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
| `focused_workspace_id` | string \| null | no | — | ID of the focused workspace, or null. |
| `focused_tab_id` | string \| null | no | — | ID of the focused tab, or null. |
| `focused_pane_id` | string \| null | no | — | ID of the focused pane, or null. |

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
| `tokens` | map<string,string> | no | — | Arbitrary key/value tokens; ≤32 entries, keys match `^[A-Za-z0-9_-]{1,32}$` (e.g. `{"branch":"main"}`). |
| `worktree` | `WorkspaceWorktreeInfo` \| null | no | — | Git worktree binding for the workspace, or null. |

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
| `focused` | boolean | yes | — | Whether this tab is focused. |
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
| `revision` | integer (uint64) | yes | — | Monotonic revision counter for the pane's state. |
| `agent` | string \| null | no | — | Detected agent label (e.g. `"claude"`), or null. |
| `display_agent` | string \| null | no | — | Human-facing agent label, or null. |
| `agent_session` | `AgentSessionInfo` \| null | no | — | Reported agent session identity, or null. |
| `cwd` | string \| null | no | — | Pane shell working directory, or null. |
| `foreground_cwd` | string \| null | no | — | Working directory of the foreground process, or null. |
| `label` | string \| null | no | — | User-assigned pane label, or null. |
| `title` | string \| null | no | — | Pane title, or null. |
| `terminal_title` | string \| null | no | — | Raw terminal title, or null. |
| `terminal_title_stripped` | string \| null | no | — | Terminal title with control/decoration stripped, or null. |
| `scroll` | `PaneScrollInfo` \| null | no | — | Scrollback/viewport position, or null. |
| `state_labels` | map<string,string> | no | — | Free-form state labels for the pane. |
| `tokens` | map<string,string> | no | — | Arbitrary key/value tokens; ≤32 entries, keys match `^[A-Za-z0-9_-]{1,32}$`. |

### `PaneScrollInfo`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `offset_from_bottom` | integer (uint64) | yes | — | Current scroll offset from the bottom (0 = pinned to newest output). |
| `max_offset_from_bottom` | integer (uint64) | yes | — | Maximum scrollable offset from the bottom. |
| `viewport_rows` | integer (uint64) | yes | — | Number of rows in the visible viewport. |

### `AgentInfo`

Same shape as a pane that hosts a detected agent. Panes without an agent are omitted from the `agents` array.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `terminal_id` | string | yes | — | Underlying terminal ID. |
| `agent_status` | `AgentStatus` | yes | — | Agent status. |
| `workspace_id` | string | yes | — | Owning workspace ID. |
| `tab_id` | string | yes | — | Owning tab ID. |
| `pane_id` | string | yes | — | Owning pane ID. |
| `focused` | boolean | yes | — | Whether the hosting pane is focused. |
| `revision` | integer (uint64) | yes | — | Monotonic revision counter. |
| `agent` | string \| null | no | — | Detected agent label (e.g. `"codex"`), or null. |
| `display_agent` | string \| null | no | — | Human-facing agent label, or null. |
| `agent_session` | `AgentSessionInfo` \| null | no | — | Reported agent session identity, or null. |
| `cwd` | string \| null | no | — | Shell working directory, or null. |
| `foreground_cwd` | string \| null | no | — | Foreground process working directory, or null. |
| `name` | string \| null | no | — | Agent name, or null. |
| `title` | string \| null | no | — | Pane/agent title, or null. |
| `terminal_title` | string \| null | no | — | Raw terminal title, or null. |
| `terminal_title_stripped` | string \| null | no | — | Terminal title with decoration stripped, or null. |
| `interactive_ready` | boolean | yes | — | True when the agent is ready to receive interactive input. |
| `launch_pending` | boolean | yes | — | True while an agent launch is still in progress. |
| `screen_detection_skipped` | boolean | yes | — | True when screen-based agent detection was skipped for this pane. |
| `state_change_seq` | integer (uint64) | no | `0` | Sequence number incremented on each agent state change. |
| `state_labels` | map<string,string> | no | — | Free-form state labels. |
| `tokens` | map<string,string> | no | — | Arbitrary key/value tokens; ≤32 entries, keys match `^[A-Za-z0-9_-]{1,32}$`. |

> Note: `agent_status`, `workspace_id`, `tab_id`, `pane_id`, `focused`, `revision`, and `terminal_id` are required on `AgentInfo`; `interactive_ready`, `launch_pending`, and `screen_detection_skipped` are non-nullable booleans but are not listed in the schema `required` set, so treat them as always present per the schema type.

### `AgentSessionInfo`

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `source` | string | yes | — | Origin of the session identity (e.g. `"herdr:claude"`). |
| `agent` | string | yes | — | Agent label the session belongs to. |
| `kind` | `AgentSessionRefKind` | yes | — | Reference kind: `id` or `path`. |
| `value` | string | yes | — | The session ID or path value, per `kind`. |

`AgentSessionRefKind` enum: `id`, `path`.

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

Enum used by `WorkspaceInfo`, `TabInfo`, `PaneInfo`, and `AgentInfo`: `idle`, `working`, `blocked`, `done`, `unknown`.

**Errors**: No error codes observed for `session.snapshot`. Other codes possible.

**CLI**: `herdr api snapshot` — prints the live session snapshot (the `snapshot` object) as JSON.

**Example** — `Validated 2026-08-19 against herdr 0.8.2.` (arrays truncated with `…`; structure intact)

```json
{"id":"r2","method":"session.snapshot","params":{}}
{"id":"r2","result":{"type":"session_snapshot","snapshot":{"version":"0.8.2","protocol":20,"focused_workspace_id":"w1","focused_tab_id":"w1:t1","focused_pane_id":"w1:p1","workspaces":[{"workspace_id":"w1","number":1,"label":"--label docs-ws-renamed","focused":true,"pane_count":3,"tab_count":3,"active_tab_id":"w1:t1","agent_status":"unknown","tokens":{"branch":"main"},"worktree":{"repo_key":"…/scratch-repo/.git","repo_name":"scratch-repo","repo_root":"…/scratch-repo","checkout_path":"…/scratch-repo","is_linked_worktree":false}},…],"tabs":[{"tab_id":"w1:t1","workspace_id":"w1","number":1,"label":"1","focused":true,"pane_count":1,"agent_status":"unknown"},…],"panes":[{"pane_id":"w1:p1","terminal_id":"term_65970bc8958f71","workspace_id":"w1","tab_id":"w1:t1","focused":true,"cwd":"…/scratch-repo","foreground_cwd":"…/scratch-repo","terminal_title":"penguin@raft: …","terminal_title_stripped":"penguin@raft: …","agent_status":"unknown","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":0,"viewport_rows":39},"revision":1},…],"layouts":[{"workspace_id":"w1","tab_id":"w1:t1","zoomed":false,"area":{"x":26,"y":1,"width":94,"height":39},"focused_pane_id":"w1:p1","panes":[{"pane_id":"w1:p1","focused":true,"rect":{"x":26,"y":1,"width":94,"height":39}}],"splits":[]},…],"agents":[]}}}
```
