# herdr API: tab methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `tab` namespace manages tabs within a workspace. A tab is a container of one or
more panes inside a workspace; it carries an ordinal `number`, a mutable `label`, a
`focused` flag, a `pane_count`, and an aggregate `agent_status`. Every tab belongs to
exactly one workspace. Tab public IDs are opaque, stable, workspace-qualified handles of
the form `w1:t1` and are never reused after a tab closes. Creating a tab also creates its
`root_pane`; closing a tab closes the panes it contains. Most methods operate on a single
tab identified by its `tab_id`; `tab.list` enumerates tabs and `tab.move` reorders them
within a workspace.

7 methods:

| method | purpose |
| --- | --- |
| [tab.close](#tabclose) | Close a tab and its panes. |
| [tab.create](#tabcreate) | Create a new tab (and its root pane) in a workspace. |
| [tab.focus](#tabfocus) | Focus a tab in the UI and mark it seen. |
| [tab.get](#tabget) | Retrieve one tab's metadata by ID. |
| [tab.list](#tablist) | List tabs, optionally scoped to one workspace. |
| [tab.move](#tabmove) | Reorder a tab within its workspace. |
| [tab.rename](#tabrename) | Change a tab's label. |

## Shared types

### TabInfo

Returned in every tab result except `tab.close` (which returns `ok`). Also linkable in
[../data-model.md](../data-model.md).

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `tab_id` | string | yes | — | Opaque workspace-qualified tab ID (e.g. `w1:t1`). |
| `workspace_id` | string | yes | — | ID of the workspace that owns the tab. |
| `number` | integer (uint, ≥0) | yes | — | 1-based ordinal position of the tab within its workspace. |
| `label` | string | yes | — | Human-visible tab label. |
| `focused` | boolean | yes | — | Whether this tab is currently focused in the Herdr UI. |
| `pane_count` | integer (uint, ≥0) | yes | — | Number of panes contained in the tab. |
| `agent_status` | enum | yes | — | Aggregate agent status across the tab's panes. One of `idle`, `working`, `blocked`, `done`, `unknown`. |

`agent_status` values (from skill.md): `idle` = agent ready for input and its tab has been
seen in the focused UI; `working` = agent is running; `blocked` = Herdr recognized an
approval or question UI; `done` = same underlying idle state after unseen background work
finished; `unknown` = an agent is present but cannot be classified confidently (does not
prove completion).

### PaneInfo (embedded in `tab.create` result)

The `root_pane` of a newly created tab. Full domain entity — see
[../data-model.md](../data-model.md). Top-level fields:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `pane_id` | string | yes | — | Opaque workspace-qualified pane ID (e.g. `w1:p2`). |
| `terminal_id` | string | yes | — | Underlying terminal handle (e.g. `term_65970bc89ef7d3`). |
| `workspace_id` | string | yes | — | Owning workspace ID. |
| `tab_id` | string | yes | — | Owning tab ID. |
| `focused` | boolean | yes | — | Whether the pane is focused. |
| `agent_status` | enum | yes | — | One of `idle`, `working`, `blocked`, `done`, `unknown`. |
| `revision` | integer (uint64, ≥0) | yes | — | Monotonic revision counter for the pane. |
| `agent` | string \| null | no | — | Detected agent name, or null. |
| `agent_session` | AgentSessionInfo \| null | no | — | Agent session reference, or null. |
| `cwd` | string \| null | no | — | Pane working directory, or null. |
| `foreground_cwd` | string \| null | no | — | Working directory of the foreground process, or null. |
| `display_agent` | string \| null | no | — | Display name for the agent, or null. |
| `label` | string \| null | no | — | Pane label, or null. |
| `title` | string \| null | no | — | Pane title, or null. |
| `terminal_title` | string \| null | no | — | Raw terminal title, or null. |
| `terminal_title_stripped` | string \| null | no | — | Terminal title with control sequences stripped, or null. |
| `scroll` | PaneScrollInfo \| null | no | — | Scrollback position info, or null. |
| `state_labels` | object (string→string) | no | — | Arbitrary state label map. |
| `tokens` | object (string→string) | no | — | Token map; ≤32 entries, keys match `^[A-Za-z0-9_-]{1,32}$`. |

`PaneScrollInfo`: `{ offset_from_bottom, max_offset_from_bottom, viewport_rows }`, each an
integer (uint64, ≥0), all required. `AgentSessionInfo`:
`{ source, agent, kind, value }` (all required strings; `kind` is enum `id` | `path`).

## tab.close

Close a tab, closing the panes it contains. The tab ID is not reused afterward. Do not
close tabs you did not create unless explicitly asked (skill.md safety guidance).

**Params** — `TabTarget`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `tab_id` | string | yes | — | ID of the tab to close. |

**Result** — `type: "ok"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | const `"ok"` | yes | — | Success sentinel; no payload. |

**Errors**: `tab_not_found` (unknown `tab_id`); other codes possible.

**Events**: emits a `tab_closed` event to subscribers (subscription type `tab.closed`).

**CLI**: `herdr tab close <tab_id>`

**Example** — `Validated 2026-08-19 against herdr 0.8.2.`

```json
{"id":"cli:tab:close","method":"tab.close","params":{"tab_id":"w1:t2"}}
{"id":"cli:tab:close","result":{"type":"ok"}}
```

## tab.create

Create a new tab in a workspace, along with its root pane. Returns both the new `tab` and
its `root_pane`; use `root_pane.pane_id` and `tab.tab_id` for subsequent operations
(skill.md). When `workspace_id` is null the current/target workspace is used. When `focus`
is true the new tab is focused on creation.

**Params** — `TabCreateParams`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string \| null | no | null | Workspace to create the tab in; null uses the default/current workspace. |
| `cwd` | string \| null | no | null | Working directory for the root pane's launched process; null uses the default. |
| `label` | string \| null | no | null | Initial tab label; null auto-assigns (e.g. the tab number). |
| `env` | object (string→string) | no | — | Environment variables to set for the launched process. |
| `focus` | boolean | no | `false` | Whether to focus the new tab on creation. |

**Result** — `type: "tab_created"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | const `"tab_created"` | yes | — | Result discriminant. |
| `tab` | TabInfo | yes | — | The newly created tab. See [TabInfo](#tabinfo). |
| `root_pane` | PaneInfo | yes | — | The tab's root pane. See [PaneInfo](#paneinfo-embedded-in-tabcreate-result). |

**Errors**: `workspace_not_found` (unknown `workspace_id`); other codes possible.

**Events**: emits a `tab_created` event to subscribers (subscription type `tab.created`);
the event `data` mirrors the `tab` field.

**CLI**: `herdr tab create [--workspace <workspace_id>] [--cwd PATH] [--label TEXT] [--env KEY=VALUE]... [--focus] [--no-focus]`

**Example** — `Validated 2026-08-19 against herdr 0.8.2.`

```json
{"id":"cli:tab:create","method":"tab.create","params":{"label":"second-tab","cwd":"…/scratch-repo"}}
{"id":"cli:tab:create","result":{"type":"tab_created","tab":{"tab_id":"w1:t2","workspace_id":"w1","number":2,"label":"second-tab","focused":false,"pane_count":1,"agent_status":"unknown"},"root_pane":{"pane_id":"w1:p2","terminal_id":"term_65970bc89ef7d3","workspace_id":"w1","tab_id":"w1:t2","focused":false,"agent_status":"unknown","revision":0,"cwd":"…/scratch-repo","foreground_cwd":"…/scratch-repo","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":0,"viewport_rows":39}}}}
```

## tab.focus

Focus a tab in the Herdr UI and mark it seen. Focusing marks the tab's agent state seen,
which transitions an unseen `done`/background-idle tab to `idle` (skill.md); CLI reads do
not mark seen. Returns the updated tab metadata.

**Params** — `TabTarget`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `tab_id` | string | yes | — | ID of the tab to focus. |

**Result** — `type: "tab_info"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | const `"tab_info"` | yes | — | Result discriminant. |
| `tab` | TabInfo | yes | — | The focused tab (with `focused: true`). See [TabInfo](#tabinfo). |

**Errors**: `tab_not_found` (unknown `tab_id`); other codes possible.

**CLI**: `herdr tab focus <tab_id>`

**Example** — `Validated 2026-08-19 against herdr 0.8.2.`

```json
{"id":"cli:tab:focus","method":"tab.focus","params":{"tab_id":"w1:t2"}}
{"id":"cli:tab:focus","result":{"type":"tab_info","tab":{"tab_id":"w1:t2","workspace_id":"w1","number":2,"label":"--label renamed-tab","focused":true,"pane_count":1,"agent_status":"unknown"}}}
```

## tab.get

Retrieve one tab's metadata by ID. This is a read; it does not mark the tab seen.

**Params** — `TabTarget`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `tab_id` | string | yes | — | ID of the tab to fetch. |

**Result** — `type: "tab_info"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | const `"tab_info"` | yes | — | Result discriminant. |
| `tab` | TabInfo | yes | — | The requested tab. See [TabInfo](#tabinfo). |

**Errors**: `tab_not_found` (unknown `tab_id`); other codes possible.

**CLI**: `herdr tab get <tab_id>`

**Example** — `Validated 2026-08-19 against herdr 0.8.2.`

```json
{"id":"cli:tab:get","method":"tab.get","params":{"tab_id":"w2:t1"}}
{"id":"cli:tab:get","result":{"type":"tab_info","tab":{"tab_id":"w2:t1","workspace_id":"w2","number":1,"label":"1","focused":false,"pane_count":1,"agent_status":"working"}}}
```

## tab.list

List tabs. With `workspace_id` null all tabs are listed; with a workspace ID the result is
scoped to that workspace. This is a read; it does not mark tabs seen.

**Params** — `TabListParams`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string \| null | no | null | Restrict the listing to this workspace; null lists across workspaces. |

**Result** — `type: "tab_list"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | const `"tab_list"` | yes | — | Result discriminant. |
| `tabs` | array of TabInfo | yes | — | Tabs in workspace order. See [TabInfo](#tabinfo). |

**Errors**: `workspace_not_found` (unknown `workspace_id`); other codes possible.

**CLI**: `herdr tab list [--workspace <workspace_id>]`

**Example** — `Validated 2026-08-19 against herdr 0.8.2.`

```json
{"id":"cli:tab:list","method":"tab.list","params":{"workspace_id":"w2"}}
{"id":"cli:tab:list","result":{"type":"tab_list","tabs":[{"tab_id":"w2:t1","workspace_id":"w2","number":1,"label":"1","focused":false,"pane_count":1,"agent_status":"working"}]}}
```

## tab.move

Reorder a tab within its workspace by inserting it at a target index. Returns the full,
reordered tab list for the affected workspace (result type `tab_list`, not `ok`).

`insert_index` is a 0-based position in the workspace's tab ordering. In the validated
capture, moving `w1:t1` to `insert_index: 1` produced a list ordered `w1:t1, w1:t2, w1:t3,
…` — the response reflects the post-move order and `number` fields are the reassigned
ordinals.

**Params** — `TabMoveParams`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `tab_id` | string | yes | — | ID of the tab to move. |
| `insert_index` | integer (uint, ≥0) | yes | — | 0-based target position within the workspace's tab ordering. |

**Result** — `type: "tab_list"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | const `"tab_list"` | yes | — | Result discriminant. |
| `tabs` | array of TabInfo | yes | — | The workspace's tabs in their new order. See [TabInfo](#tabinfo). |

**Errors**: `tab_not_found` (unknown `tab_id`); other codes possible.

**CLI**: API-only (no CLI subcommand). The `herdr tab` command exposes only `list`,
`create`, `get`, `focus`, `rename`, and `close`.

**Example** — `Validated 2026-08-19 against herdr 0.8.2.`

```json
{"id":"t1","method":"tab.move","params":{"tab_id":"w1:t1","insert_index":1}}
{"id":"t1","result":{"type":"tab_list","tabs":[{"tab_id":"w1:t1","workspace_id":"w1","number":1,"label":"1","focused":true,"pane_count":1,"agent_status":"unknown"},{"tab_id":"w1:t2","workspace_id":"w1","number":2,"label":"--label renamed-tab","focused":false,"pane_count":1,"agent_status":"unknown"},{"tab_id":"w1:t3","workspace_id":"w1","number":3,"label":"moved-tab","focused":false,"pane_count":1,"agent_status":"unknown"},…]}}
```

## tab.rename

Change a tab's label. Returns the updated tab metadata.

**Params** — `TabRenameParams`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `tab_id` | string | yes | — | ID of the tab to rename. |
| `label` | string | yes | — | New label for the tab. |

**Result** — `type: "tab_info"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | const `"tab_info"` | yes | — | Result discriminant. |
| `tab` | TabInfo | yes | — | The renamed tab, with its updated `label`. See [TabInfo](#tabinfo). |

**Errors**: `tab_not_found` (unknown `tab_id`); other codes possible.

**CLI**: `herdr tab rename <tab_id> <label>...` (the CLI joins multiple `LABEL` words into
the label string).

**Example** — `Validated 2026-08-19 against herdr 0.8.2.`

```json
{"id":"cli:tab:rename","method":"tab.rename","params":{"tab_id":"w1:t2","label":"--label renamed-tab"}}
{"id":"cli:tab:rename","result":{"type":"tab_info","tab":{"tab_id":"w1:t2","workspace_id":"w1","number":2,"label":"--label renamed-tab","focused":false,"pane_count":1,"agent_status":"unknown"}}}
```
