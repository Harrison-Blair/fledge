# herdr API: workspace methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `workspace` namespace manages herdr's top-level containers. A workspace holds one or
more tabs, each tab holds one or more panes, and every pane runs a terminal that herdr may
recognize as a coding agent. These methods create, inspect, focus, rename, reorder, close,
and attach display-only metadata to workspaces. Workspace IDs are short opaque strings of
the form `w1`, `w2`, … and are never reused after a workspace is closed. Reads (`get`,
`list`) do not mark an agent's tab as seen; only focusing does. Most mutations emit a
push event to connections subscribed via `events.subscribe`.

9 methods:

| method | purpose |
| --- | --- |
| [workspace.close](#workspaceclose) | Close a workspace and its tabs, panes, and terminals. |
| [workspace.create](#workspacecreate) | Create a new workspace with a root tab and pane. |
| [workspace.focus](#workspacefocus) | Make a workspace the focused one in the UI. |
| [workspace.get](#workspaceget) | Fetch one workspace's current info. |
| [workspace.list](#workspacelist) | List all workspaces in display order. |
| [workspace.move](#workspacemove) | Move one workspace to an absolute index in the order. |
| [workspace.move_block](#workspacemove_block) | Move a contiguous block of workspaces before a target. |
| [workspace.rename](#workspacerename) | Change a workspace's label. |
| [workspace.report_metadata](#workspacereport_metadata) | Attach display-only tokens/metadata to a workspace. |

Shared domain entities — `WorkspaceInfo`, `TabInfo`, `PaneInfo` — are defined in
[../data-model.md](../data-model.md) and only their top-level fields are named here. The
`agent_status` enum used throughout is one of `idle`, `working`, `blocked`, `done`,
`unknown` (see skill.md for the state semantics).

## workspace.close

Close a workspace, destroying all of its tabs, panes, and their terminals. Do not close a
workspace you did not create unless the user explicitly asked. Closed workspace, tab, and
pane IDs are never reused.

**Params** (`WorkspaceTarget`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | ID of the workspace to close (e.g. `w2`). |

**Result** — `type: "ok"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"ok"` | Acknowledgment; no payload. |

**Errors**

| code | when |
| --- | --- |
| `workspace_not_found` | `workspace_id` does not match a live workspace. |

Other codes possible.

**Events**: emits `workspace.closed` to subscribers (event name in schema; inferred from name).

**CLI**: `herdr workspace close <workspace_id>`

**Example** — Validated 2026-08-19 against herdr 0.8.2.

```json
{"id":"cli:workspace:close","method":"workspace.close","params":{"workspace_id":"w2"}}
{"id":"cli:workspace:close","result":{"type":"ok"}}
```

## workspace.create

Create a new workspace containing a single root tab and a single root pane. The response
exposes the IDs to use next: `workspace`, `tab`, and `root_pane`. Per skill.md, do not
create a new workspace unless the user explicitly requested that topology. The new pane's
process starts in `cwd` (or herdr's default when null) with any supplied `env` overlaid.

**Params** (`WorkspaceCreateParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `cwd` | string \| null | no | null | Working directory for the root pane's process; null uses herdr's default. |
| `env` | object (string→string) | no | `{}` | Environment variables set for the launched process. |
| `focus` | boolean | no | `false` | If true, focus the new workspace in the UI; false creates it in the background. |
| `label` | string \| null | no | null | Display label; null lets herdr auto-assign one. |

**Result** — `type: "workspace_created"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_created"` | Result discriminant. |
| `workspace` | WorkspaceInfo | The created workspace ([../data-model.md](../data-model.md)). |
| `tab` | TabInfo | The workspace's root tab ([../data-model.md](../data-model.md)). |
| `root_pane` | PaneInfo | The root pane of the root tab ([../data-model.md](../data-model.md)). |

**Errors**: `invalid_params` (malformed input) and other codes possible.

**Events**: emits `workspace.created` to subscribers (event name in schema; inferred from name).

**CLI**: `herdr workspace create [--cwd PATH] [--label TEXT] [--env KEY=VALUE] [--focus] [--no-focus]`

**Example** — Validated 2026-08-19 against herdr 0.8.2.

```json
{"id":"cli:workspace:create","method":"workspace.create","params":{"label":"docs-ws","focus":true}}
{"id":"cli:workspace:create","result":{"type":"workspace_created","workspace":{"active_tab_id":"w1:t1","agent_status":"unknown","focused":true,"label":"docs-ws","number":1,"pane_count":1,"tab_count":1,"workspace_id":"w1"},"tab":{"agent_status":"unknown","focused":true,"label":"1","number":1,"pane_count":1,"tab_id":"w1:t1","workspace_id":"w1"},"root_pane":{"agent_status":"unknown","cwd":"/…/scratch-repo","focused":true,"foreground_cwd":"/…/scratch-repo","pane_id":"w1:p1","revision":0,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":40},"tab_id":"w1:t1","terminal_id":"term_65970bc8958f71","workspace_id":"w1"}}}
```

## workspace.focus

Make the target workspace the focused one in the herdr UI. Focusing a workspace (or its
tab/pane) marks the agent's tab as seen, which converts an `idle`/`done` status to seen
state; CLI reads do not. Use background creation and avoid focusing unless the user asked
to switch context.

**Params** (`WorkspaceTarget`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | ID of the workspace to focus. |

**Result** — `type: "workspace_info"` (inferred; the slice's only single-workspace result
type, matching `workspace.rename`; the alternative in the slice is `ok`):

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_info"` | Result discriminant. |
| `workspace` | WorkspaceInfo | The now-focused workspace ([../data-model.md](../data-model.md)). |

**Errors**

| code | when |
| --- | --- |
| `workspace_not_found` | `workspace_id` does not match a live workspace. |

Other codes possible.

**Events**: emits `workspace.focused` to subscribers (event name in schema; inferred from name).

**CLI**: `herdr workspace focus <workspace_id>`

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"workspace.focus","params":{"workspace_id":"w2"}}
{"id":"1","result":{"type":"workspace_info","workspace":{"active_tab_id":"w2:t1","agent_status":"working","focused":true,"label":"fledge","number":2,"pane_count":1,"tab_count":1,"workspace_id":"w2"}}}
```

## workspace.get

Fetch the current info for a single workspace. This is a read: it does not mark the
workspace's tab as seen.

**Params** (`WorkspaceTarget`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | ID of the workspace to fetch. |

**Result** — `type: "workspace_info"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_info"` | Result discriminant. |
| `workspace` | WorkspaceInfo | The requested workspace ([../data-model.md](../data-model.md)). |

**Errors**

| code | when |
| --- | --- |
| `workspace_not_found` | `workspace_id` does not match a live workspace. |

Other codes possible.

**CLI**: `herdr workspace get <workspace_id>`

**Example** — Validated 2026-08-19 against herdr 0.8.2.

```json
{"id":"cli:workspace:get","method":"workspace.get","params":{"workspace_id":"w2"}}
{"id":"cli:workspace:get","result":{"type":"workspace_info","workspace":{"active_tab_id":"w2:t1","agent_status":"working","focused":false,"label":"fledge","number":2,"pane_count":1,"tab_count":1,"workspace_id":"w2"}}}
```

The `workspace_not_found` error is confirmed by probe:

```json
{"id":"cli:workspace:get","method":"workspace.get","params":{"workspace_id":"w99"}}
{"id":"cli:workspace:get","error":{"code":"workspace_not_found","message":"workspace w99 not found"}}
```

## workspace.list

List every workspace in display order. Read-only; does not mark any tab as seen.

**Params** (`EmptyParams`): none. Send `params: {}`.

**Result** — `type: "workspace_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_list"` | Result discriminant. |
| `workspaces` | array of WorkspaceInfo | All workspaces, in display order ([../data-model.md](../data-model.md)). |

**Errors**: other codes possible.

**CLI**: `herdr workspace list`

**Example** — Validated 2026-08-19 against herdr 0.8.2.

```json
{"id":"cli:workspace:list","method":"workspace.list","params":{}}
{"id":"cli:workspace:list","result":{"type":"workspace_list","workspaces":[{"active_tab_id":"w1:t1","agent_status":"idle","focused":true,"label":"fledge","number":1,"pane_count":1,"tab_count":1,"workspace_id":"w1"},{"active_tab_id":"w2:t1","agent_status":"working","focused":false,"label":"fledge","number":2,"pane_count":1,"tab_count":1,"workspace_id":"w2"}]}}
```

## workspace.move

Move one workspace to an absolute position in the display order. `insert_index` is a
zero-based slot in the reordered list. The response returns the full reordered list, whose
`number` fields reflect the new positions.

**Params** (`WorkspaceMoveParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | ID of the workspace to move. |
| `insert_index` | integer (uint, ≥ 0) | yes | — | Zero-based target index in the display order. |

**Result** — `type: "workspace_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_list"` | Result discriminant. |
| `workspaces` | array of WorkspaceInfo | The full list after reordering ([../data-model.md](../data-model.md)). |

**Errors**: `workspace_not_found` (unknown `workspace_id`); other codes possible.

**Events**: emits `workspace.moved` / `workspace.reordered` to subscribers (event names in schema; inferred from name).

**CLI**: API-only (no CLI subcommand). The `herdr workspace` command group lists only
list/create/get/focus/rename/report-metadata/close.

**Example** — Validated 2026-08-19 against herdr 0.8.2.

```json
{"id":"wm1","method":"workspace.move","params":{"workspace_id":"w2","insert_index":0}}
{"id":"wm1","result":{"type":"workspace_list","workspaces":[{"workspace_id":"w2","number":1,"label":"second-ws","focused":false,"pane_count":1,"tab_count":1,"active_tab_id":"w2:t1","agent_status":"unknown"},{"workspace_id":"w1","number":2,"label":"--label docs-ws-renamed","focused":true,"pane_count":6,"tab_count":6,"active_tab_id":"w1:t1","agent_status":"unknown","tokens":{"branch":"main"},"worktree":{"repo_key":"/…/scratch-repo/.git","repo_name":"scratch-repo","repo_root":"/…/scratch-repo","checkout_path":"/…/scratch-repo","is_linked_worktree":false}},{"workspace_id":"w3","number":3,"label":"docs-probe","focused":false,"pane_count":1,"tab_count":1,"active_tab_id":"w3:t1","agent_status":"unknown","worktree":{"repo_key":"/…/scratch-repo/.git","repo_name":"scratch-repo","repo_root":"/…/scratch-repo","checkout_path":"/…/worktrees/scratch-repo/docs-probe","is_linked_worktree":true}}]}}
```

## workspace.move_block

Move a contiguous block of workspaces so that they sit immediately before a target
workspace, preserving their relative order. When `before_workspace_id` is null the block is
moved to the end of the display order. The response returns the full reordered list.

**Params** (`WorkspaceMoveBlockParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_ids` | array of string | yes | — | The workspace IDs to move, as a block (kept in the given relative order). |
| `before_workspace_id` | string \| null | no | null | Insert the block immediately before this workspace; null appends to the end. |

**Result** — `type: "workspace_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_list"` | Result discriminant. |
| `workspaces` | array of WorkspaceInfo | The full list after reordering ([../data-model.md](../data-model.md)). |

**Errors**: `workspace_not_found` (any unknown ID); other codes possible.

**Events**: emits `workspace.reordered` / `workspace.moved` to subscribers (event names in schema; inferred from name).

**CLI**: API-only (no CLI subcommand).

**Example** — Validated 2026-08-19 against herdr 0.8.2.

```json
{"id":"wm2","method":"workspace.move_block","params":{"workspace_ids":["w2"],"before_workspace_id":null}}
{"id":"wm2","result":{"type":"workspace_list","workspaces":[{"workspace_id":"w1","number":1,"label":"--label docs-ws-renamed","focused":true,"pane_count":6,"tab_count":6,"active_tab_id":"w1:t1","agent_status":"unknown","tokens":{"branch":"main"},"worktree":{"repo_key":"/…/scratch-repo/.git","repo_name":"scratch-repo","repo_root":"/…/scratch-repo","checkout_path":"/…/scratch-repo","is_linked_worktree":false}},{"workspace_id":"w3","number":2,"label":"docs-probe","focused":false,"pane_count":1,"tab_count":1,"active_tab_id":"w3:t1","agent_status":"unknown","worktree":{"repo_key":"/…/scratch-repo/.git","repo_name":"scratch-repo","repo_root":"/…/scratch-repo","checkout_path":"/…/worktrees/scratch-repo/docs-probe","is_linked_worktree":true}},{"workspace_id":"w2","number":3,"label":"second-ws","focused":false,"pane_count":1,"tab_count":1,"active_tab_id":"w2:t1","agent_status":"unknown"}]}}
```

## workspace.rename

Change a workspace's display label. Returns the updated workspace info.

**Params** (`WorkspaceRenameParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | ID of the workspace to rename. |
| `label` | string | yes | — | The new display label. |

**Result** — `type: "workspace_info"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_info"` | Result discriminant. |
| `workspace` | WorkspaceInfo | The renamed workspace ([../data-model.md](../data-model.md)). |

**Errors**: `workspace_not_found` (unknown `workspace_id`); other codes possible.

**Events**: emits `workspace.renamed` to subscribers (event name in schema; inferred from name).

**CLI**: `herdr workspace rename <workspace_id> <label>` (the CLI accepts the label as one
or more trailing arguments: `rename <WORKSPACE_ID> <LABEL>...`).

**Example** — Validated 2026-08-19 against herdr 0.8.2.

```json
{"id":"cli:workspace:rename","method":"workspace.rename","params":{"workspace_id":"w1","label":"--label docs-ws-renamed"}}
{"id":"cli:workspace:rename","result":{"type":"workspace_info","workspace":{"active_tab_id":"w1:t1","agent_status":"unknown","focused":true,"label":"--label docs-ws-renamed","number":1,"pane_count":1,"tab_count":1,"workspace_id":"w1"}}}
```

## workspace.report_metadata

Attach display-only metadata to a workspace: a bag of short string tokens keyed by name,
scoped to a `source`. Tokens are advisory display state (they surface in `WorkspaceInfo.tokens`,
e.g. `{"branch":"main"}`); they do not change topology or agent status. A token whose value
is null clears that token. `seq` provides ordering so stale reports can be discarded, and
`ttl_ms` bounds how long a token remains before it expires.

**Params** (`WorkspaceReportMetadataParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | ID of the workspace to annotate. |
| `source` | string | yes | — | Namespace/identity of the reporter; scopes the tokens it owns. |
| `tokens` | object (string→(string \| null)) | yes | — | Token map. Keys match `^[A-Za-z0-9_-]{1,32}$`; at most 16 entries; a null value clears that token. |
| `seq` | integer (uint64, ≥ 0) \| null | no | null | Monotonic sequence number for this source; lets the server drop out-of-order reports. |
| `ttl_ms` | integer (uint64) \| null | no | null | Token lifetime in milliseconds, 1 … 86400000 (24 h); null means no explicit expiry. |

**Result** — `type: "ok"` (inferred; the sibling `pane.report_metadata` returns `type:"ok"`,
and `ok` is the matching variant in this slice):

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"ok"` | Acknowledgment; no payload. |

**Errors**: `workspace_not_found` (unknown `workspace_id`), `invalid_params` (token key
pattern, count, or `ttl_ms` range violation); other codes possible.

**Events**: emits `workspace.metadata_updated` to subscribers (event name in schema; inferred from name).

**CLI**: `herdr workspace report-metadata <workspace_id> --source ID [--token NAME=VALUE] [--clear-token NAME] [--seq N] [--ttl-ms N]`
(`--token` sets a token; `--clear-token` sends a null value for that token).

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"workspace.report_metadata","params":{"workspace_id":"w1","source":"my-tool","tokens":{"branch":"main"},"ttl_ms":60000}}
{"id":"1","result":{"type":"ok"}}
```
