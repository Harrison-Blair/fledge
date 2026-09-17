# herdr API: pane methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `pane.*` namespace controls individual terminal panes: the leaf terminals inside a
workspace/tab layout tree. Methods here inspect pane topology and geometry (`list`,
`get`, `current`, `layout`, `edges`, `neighbor`, `process_info`), mutate the split tree
(`split`, `swap`, `move`, `resize`, `zoom`, `close`, `focus`, `focus_direction`), drive a
pane's terminal (`read`, `wait_for_output`, `send_text`, `send_keys`, `send_input`,
`rename`, `input.set`), render inline images (`graphics.set`, `graphics.clear`,
`graphics.info`), and let an integration report/withdraw agent-lifecycle and display
metadata for a pane (`report_agent`, `report_agent_session`, `report_metadata`,
`release_agent`, `clear_agent_authority`). A pane exists whether or not it hosts a
recognized agent; use `agent.*` methods when herdr must validate agent identity or
interpret lifecycle states. Panes are addressed by workspace-qualified IDs such as
`w1:p1` (see [addressing.md](../addressing.md)); most read methods accept a null/omitted
`pane_id` to target the UI-focused pane, but callers should pass their own
`$HERDR_PANE_ID` to avoid targeting another client's focus.

Domain entities (`PaneInfo`, `AgentInfo`, `PaneLayoutSnapshot`, `WorkspaceInfo`,
`TabInfo`, …) are defined once in [data-model.md](../data-model.md); result tables below
name every top-level field and link there rather than re-expanding embedded entities.

30 methods:

| method | purpose |
|---|---|
| [pane.clear_agent_authority](#paneclear_agent_authority) | Withdraw a source's authority over a pane's agent lifecycle reporting |
| [pane.close](#paneclose) | Close a pane and its terminal |
| [pane.current](#panecurrent) | Return the pane the caller/UI is currently in |
| [pane.edges](#paneedges) | Report which of a pane's four edges border the tab boundary |
| [pane.focus](#panefocus) | Focus a specific pane by ID and return its agent info |
| [pane.focus_direction](#panefocus_direction) | Move focus to the neighboring pane in a direction |
| [pane.get](#paneget) | Fetch a single pane's `PaneInfo` |
| [pane.graphics.clear](#panegraphicsclear) | Clear graphics layer(s) from a pane |
| [pane.graphics.info](#panegraphicsinfo) | Report a pane's graphics capabilities |
| [pane.graphics.set](#panegraphicsset) | Draw/replace an image layer in a pane |
| [pane.input.set](#paneinputset) | Set a pane's right-click input routing |
| [pane.layout](#panelayout) | Return the layout snapshot of a pane's tab |
| [pane.list](#panelist) | List panes, optionally scoped to a workspace |
| [pane.move](#panemove) | Move a pane to another tab/new tab/new workspace |
| [pane.neighbor](#paneneighbor) | Resolve the neighboring pane ID in a direction |
| [pane.process_info](#paneprocess_info) | Report a pane's shell and foreground processes |
| [pane.read](#paneread) | Read a pane's terminal output snapshot |
| [pane.release_agent](#panerelease_agent) | Release a source's agent lifecycle authority for one agent |
| [pane.rename](#panerename) | Set or clear a pane's label |
| [pane.report_agent](#panereport_agent) | Report agent lifecycle state for a pane |
| [pane.report_agent_session](#panereport_agent_session) | Report agent session identity for a pane |
| [pane.report_metadata](#panereport_metadata) | Report display-only pane metadata (title, tokens, labels) |
| [pane.resize](#paneresize) | Resize the split enclosing a pane |
| [pane.send_input](#panesend_input) | Send text and/or logical keys to a pane in one call |
| [pane.send_keys](#panesend_keys) | Send logical key presses to a pane |
| [pane.send_text](#panesend_text) | Send literal text to a pane |
| [pane.split](#panesplit) | Split a pane, creating a new sibling pane |
| [pane.swap](#paneswap) | Swap two panes' positions in the layout |
| [pane.wait_for_output](#panewait_for_output) | Block until pane output matches a pattern |
| [pane.zoom](#panezoom) | Toggle/set zoom (maximize one pane in its tab) |

Enums used across this namespace:

- **PaneDirection**: `left`, `right`, `up`, `down`.
- **SplitDirection**: `right`, `down`.
- **ReadSource**: `visible`, `recent`, `recent_unwrapped`, `detection`.
- **ReadFormat**: `text`, `ansi`.
- **PaneRightClickTarget**: `herdr`, `pane`.
- **PaneZoomMode**: `toggle`, `on`, `off`.
- **PaneAgentState**: `idle`, `working`, `blocked`, `unknown`.
- **PaneGraphicsFormat**: `png`, `rgb`, `rgba`, `bgra`.

Read-source semantics (from skill.md): `visible` is the currently rendered viewport;
`recent` is recent rendered output including soft wraps; `recent_unwrapped` joins soft
wraps (preferred for logs/transcripts); `detection` is the plain-text bottom-buffer
snapshot herdr uses for agent detection. CLI reads do **not** mark an agent's tab as
seen; only focusing (via the UI or a focus command) does.

---

## pane.clear_agent_authority

Withdraw the authority a reporting `source` established over a pane's agent-lifecycle
reporting, without naming a specific agent. Use it when an integration stops managing a
pane entirely; to release a single named agent instead, use
[pane.release_agent](#panerelease_agent). Idempotent — returns `ok` even when the source
holds no authority.

**Params** (`PaneClearAgentAuthorityParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane ID. |
| `source` | string \| null | no | null | Reporting source whose authority is cleared. Null clears the ambient/default source (inferred). |
| `seq` | integer (uint64) \| null | no | null | Monotonic sequence number for ordering out-of-order reports from the same source (inferred). |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found` (unknown `pane_id`); other codes possible.

**CLI**: API-only (no CLI subcommand). The CLI exposes `release-agent` but not a bare
authority-clear.

**Example**

```json
{"id":"p6","method":"pane.clear_agent_authority","params":{"pane_id":"w1:p1","source":"docprobe"}}
{"id":"p6","result":{"type":"ok"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.close

Close a pane and terminate its terminal process. Do not close panes the caller did not
create unless the user explicitly asked (skill.md). Closed pane IDs are not reused.

**Params** (`PaneTarget`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to close. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane close <pane_id>`

**Example**

```json
{"id":"cli:pane:close","method":"pane.close","params":{"pane_id":"w1:p2"}}
{"id":"cli:pane:close","result":{"type":"ok"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.current

Return the pane the caller (or, absent caller context, the UI-focused surface) is
currently in. Prefer this with the caller's own `caller_pane_id` to resolve "my pane".

**Params** (`PaneCurrentParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `caller_pane_id` | string \| null | no | null | Pane ID to treat as the caller's own pane; when set, resolves that pane rather than the UI focus (inferred from CLI `--current`/`--pane`). |

**Result**: `type: "pane_current"`

| field | type | meaning |
|---|---|---|
| `pane` | [PaneInfo](../data-model.md) | The resolved current pane. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane current [--current | --pane <ID>]`

**Example**

```json
{"id":"cli:pane:current","method":"pane.current","params":{"caller_pane_id":"w2:p1"}}
{"id":"cli:pane:current","result":{"pane":{"agent":"claude","agent_session":{"agent":"claude","kind":"id","source":"herdr:claude","value":"ef3b9d04-…"},"agent_status":"working","cwd":"/home/penguin/source/fledge","focused":false,"foreground_cwd":"/home/penguin/source/fledge","pane_id":"w2:p1","revision":4,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":54},"tab_id":"w2:t1","terminal_id":"term_659708952f5514","terminal_title":"◐ herdr-api-documentation","terminal_title_stripped":"herdr-api-documentation","workspace_id":"w2"},"type":"pane_current"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.edges

Report, for a pane, whether each of its four edges lies on the tab's outer boundary
(`true`) versus adjoining another pane (`false`). Useful for deciding whether a
directional focus/move would leave the tab. Also returns the full tab layout snapshot.

**Params** (`PaneEdgesParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string \| null | no | null | Pane to inspect; null targets the UI-focused pane. |

**Result**: `type: "pane_edges"`

| field | type | meaning |
|---|---|---|
| `pane_id` | string | Pane inspected. |
| `left` | boolean | Left edge is on the tab boundary. |
| `right` | boolean | Right edge is on the tab boundary. |
| `up` | boolean | Top edge is on the tab boundary. |
| `down` | boolean | Bottom edge is on the tab boundary. |
| `layout` | [PaneLayoutSnapshot](../data-model.md) | The enclosing tab's layout. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane edges [--current | --pane <ID>]`

**Example**

```json
{"id":"cli:pane:edges","method":"pane.edges","params":{"pane_id":"w2:p1"}}
{"id":"cli:pane:edges","result":{"edges":{"down":true,"layout":{"area":{"height":54,"width":166,"x":26,"y":1},"focused_pane_id":"w2:p1","panes":[{"focused":true,"pane_id":"w2:p1","rect":{"height":54,"width":166,"x":26,"y":1}}],"splits":[],"tab_id":"w2:t1","workspace_id":"w2","zoomed":false},"left":true,"pane_id":"w2:p1","right":true,"up":true},"type":"pane_edges"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.focus

Focus a specific pane by ID and return the `AgentInfo` for its occupant. Focusing marks
the pane's agent (and its tab) as **seen**, which collapses a background `done` state
back to observed `idle` (skill.md). Unlike [pane.focus_direction](#panefocus_direction),
this targets an exact pane rather than a neighbor.

**Params** (`PaneTarget`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to focus. |

**Result**: `type: "agent_info"` (inferred: `agent_info` is the sole result type in this
slice not otherwise produced by a pane method, and skill.md describes focusing as
returning/refreshing the pane occupant's agent state)

| field | type | meaning |
|---|---|---|
| `agent` | [AgentInfo](../data-model.md) | Agent occupying the now-focused pane (fields present even when no agent is recognized). |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: API-only (no CLI subcommand); the CLI `herdr pane focus` maps to
[pane.focus_direction](#panefocus_direction).

**Example**

```json
{"id":"1","method":"pane.focus","params":{"pane_id":"w1:p1"}}
{"id":"1","result":{"type":"agent_info","agent":{"agent":"claude","agent_status":"idle","focused":true,"interactive_ready":true,"launch_pending":false,"pane_id":"w1:p1","revision":7,"screen_detection_skipped":false,"state_change_seq":3,"tab_id":"w1:t1","terminal_id":"term_…","workspace_id":"w1"}}}
```

Constructed from schema; not live-validated.

---

## pane.focus_direction

Move keyboard focus from a pane to its neighbor in a given direction. When there is no
neighbor in that direction, `changed` is `false` and `reason` is `no_neighbor`.

**Params** (`PaneFocusDirectionParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `direction` | PaneDirection (`left`,`right`,`up`,`down`) | yes | — | Direction to move focus. |
| `pane_id` | string \| null | no | null | Source pane; null uses the UI-focused pane. |

**Result**: `type: "pane_focus_direction"`

| field | type | meaning |
|---|---|---|
| `changed` | boolean | Whether focus actually moved. |
| `source_pane_id` | string | Pane focus started from. |
| `focused_pane_id` | string \| null | Pane now focused (null if unchanged / no neighbor). |
| `reason` | `no_neighbor` \| null | Why focus did not change, if applicable. |
| `layout` | [PaneLayoutSnapshot](../data-model.md) | Resulting tab layout. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane focus --direction <left|right|up|down> [--current | --pane <ID>]`

**Example**

```json
{"id":"1","method":"pane.focus_direction","params":{"pane_id":"w1:p1","direction":"right"}}
{"id":"1","result":{"type":"pane_focus_direction","focus":{"changed":true,"source_pane_id":"w1:p1","focused_pane_id":"w1:p3","reason":null,"layout":{"area":{"height":39,"width":94,"x":26,"y":1},"focused_pane_id":"w1:p3","panes":[{"focused":false,"pane_id":"w1:p1","rect":{"height":39,"width":47,"x":26,"y":1}},{"focused":true,"pane_id":"w1:p3","rect":{"height":39,"width":47,"x":73,"y":1}}],"splits":[{"direction":"right","id":"split_0_root","ratio":0.5,"rect":{"height":39,"width":94,"x":26,"y":1}}],"tab_id":"w1:t1","workspace_id":"w1","zoomed":false}}}}
```

Constructed from schema; not live-validated.

---

## pane.get

Fetch a single pane's `PaneInfo` by exact ID.

**Params** (`PaneTarget`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to fetch. |

**Result**: `type: "pane_info"`

| field | type | meaning |
|---|---|---|
| `pane` | [PaneInfo](../data-model.md) | The requested pane. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane get <pane_id>`

**Example**

```json
{"id":"cli:pane:get","method":"pane.get","params":{"pane_id":"w2:p1"}}
{"id":"cli:pane:get","result":{"pane":{"agent":"claude","agent_session":{"agent":"claude","kind":"id","source":"herdr:claude","value":"ef3b9d04-…"},"agent_status":"working","cwd":"/home/penguin/source/fledge","focused":false,"foreground_cwd":"/home/penguin/source/fledge","pane_id":"w2:p1","revision":4,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":54},"tab_id":"w2:t1","terminal_id":"term_659708952f5514","terminal_title":"◐ herdr-api-documentation","terminal_title_stripped":"herdr-api-documentation","workspace_id":"w2"},"type":"pane_info"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.graphics.clear

Clear a graphics image layer (or all layers) from a pane. Requires the
`experimental.kitty_graphics` feature to be enabled; otherwise returns `feature_disabled`.

**Params** (`PaneGraphicsClearParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane whose graphics to clear. |
| `layer_id` | string \| null | no | null | Specific layer to clear; null clears all layers on the pane (inferred). |

**Result**: `type: "ok"` — no other fields.

**Errors**: `feature_disabled` (`pane graphics require experimental.kitty_graphics`),
`pane_not_found`; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"1","method":"pane.graphics.clear","params":{"pane_id":"w1:p1","layer_id":"overlay-1"}}
{"id":"1","result":{"type":"ok"}}
```

Constructed from schema; not live-validated (feature disabled on the probed server).

---

## pane.graphics.info

Report a pane's graphics capabilities: cell pixel dimensions, layer limits, and file-frame
transport parameters. Requires `experimental.kitty_graphics`.

**Params** (`PaneTarget`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to query. |

**Result**: `type: "pane_graphics_info"`

| field | type | meaning |
|---|---|---|
| `cell_width_px` | integer (uint32) | Width of one terminal cell in pixels. |
| `cell_height_px` | integer (uint32) | Height of one terminal cell in pixels. |
| `pane_visible` | boolean | True only when the pane is on the currently rendered terminal surface. |
| `max_layers_per_pane` | integer (uint) | Maximum simultaneous graphics layers per pane (default 0). |
| `pixel_mouse` | boolean | Whether pixel-precision mouse reporting is active (default false). |
| `file_frame_damage` | boolean | Accepts damage metadata while still consuming a complete canonical file (default false). |
| `file_frame_transport` | string \| null | Name of the file-frame transport, if any. |
| `file_frame_directory` | string \| null | Directory where file-frame payloads are staged. |
| `file_frame_formats` | array&lt;string&gt; | Accepted file-frame image formats. |
| `file_frame_max_bytes` | integer (uint) \| null | Max bytes for a file-frame payload. |
| `file_frame_direct_max_bytes` | integer (uint) \| null | Max bytes for a directly-inlined file frame. |

**Errors**: `feature_disabled` (`pane graphics require experimental.kitty_graphics`),
`pane_not_found`; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"p3","method":"pane.graphics.info","params":{"pane_id":"w1:p1"}}
{"id":"p3","error":{"code":"feature_disabled","message":"pane graphics require experimental.kitty_graphics"}}
```

Validated 2026-08-19 against herdr 0.8.2 (error path; the feature was disabled).

---

## pane.graphics.set

Draw or replace an image layer in a pane. The image bytes are base64-encoded in
`data_base64`, described by `format`/`image_width`/`image_height`, and positioned via
`placement`. Requires `experimental.kitty_graphics`.

**Params** (`PaneGraphicsSetParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `format` | PaneGraphicsFormat (`png`,`rgb`,`rgba`,`bgra`) | yes | — | Pixel/encoding format of `data_base64`. |
| `image_width` | integer (uint32) | yes | — | Source image width in pixels. |
| `image_height` | integer (uint32) | yes | — | Source image height in pixels. |
| `data_base64` | string | no | `""` | Base64-encoded image payload. |
| `layer_id` | string \| null | no | null | Layer to create/replace; null assigns/uses the default layer (inferred). |
| `z_index` | integer (int32) | no | 0 | Stacking order among layers. |
| `placement` | PaneGraphicsPlacementParams | no | `{grid_cols:0,grid_rows:0,viewport_col:0,viewport_row:0}` | Where and how large to place the image (see below). |

`PaneGraphicsPlacementParams`:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `grid_cols` | integer (uint32) | no | 0 | Image width in terminal cells (0 = derive from pixels, inferred). |
| `grid_rows` | integer (uint32) | no | 0 | Image height in terminal cells (0 = derive from pixels, inferred). |
| `viewport_col` | integer (int32) | no | 0 | Column offset within the pane viewport. |
| `viewport_row` | integer (int32) | no | 0 | Row offset within the pane viewport. |

**Result**: `type: "pane_graphics_frame_ack"`

| field | type | meaning |
|---|---|---|
| `sequence` | integer (uint64) | Frame sequence number acknowledged. |
| `revision` | integer (uint64) | Pane graphics revision after applying the frame. |

**Errors**: `feature_disabled` (`pane graphics require experimental.kitty_graphics`),
`pane_not_found`; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"1","method":"pane.graphics.set","params":{"pane_id":"w1:p1","format":"png","image_width":64,"image_height":64,"data_base64":"iVBORw0KGgo…","placement":{"grid_cols":8,"grid_rows":4,"viewport_col":0,"viewport_row":0},"z_index":0}}
{"id":"1","result":{"type":"pane_graphics_frame_ack","sequence":1,"revision":1}}
```

Constructed from schema; not live-validated (feature disabled on the probed server).

---

## pane.input.set

Set a pane's right-click input routing: whether a right-click is handled by herdr's own
UI (context menu) or forwarded into the pane's terminal program.

**Params** (`PaneInputSetParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `right_click` | PaneRightClickTarget (`herdr`,`pane`) | yes | — | Route right-clicks to herdr's UI or into the pane program. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane input --right-click <herdr|pane> [--current | --pane <ID> | PANE_ID]`

**Example**

```json
{"id":"p2","method":"pane.input.set","params":{"pane_id":"w1:p1","right_click":"pane"}}
{"id":"p2","result":{"type":"ok"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.layout

Return the layout snapshot of the tab containing a pane: the tab's area, split tree, and
per-pane rectangles.

**Params** (`PaneLayoutParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string \| null | no | null | Pane whose tab layout to return; null uses the UI-focused pane. |

**Result**: `type: "pane_layout"`

| field | type | meaning |
|---|---|---|
| `layout` | [PaneLayoutSnapshot](../data-model.md) | Layout of the pane's tab (`workspace_id`, `tab_id`, `zoomed`, `area`, `focused_pane_id`, `panes[]`, `splits[]`). |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane layout [--current | --pane <ID>]`

**Example**

```json
{"id":"cli:pane:layout","method":"pane.layout","params":{"pane_id":"w2:p1"}}
{"id":"cli:pane:layout","result":{"layout":{"area":{"height":54,"width":166,"x":26,"y":1},"focused_pane_id":"w2:p1","panes":[{"focused":true,"pane_id":"w2:p1","rect":{"height":54,"width":166,"x":26,"y":1}}],"splits":[],"tab_id":"w2:t1","workspace_id":"w2","zoomed":false},"type":"pane_layout"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.list

List panes. With no `workspace_id`, lists all panes known to the server; with one, scopes
to that workspace.

**Params** (`PaneListParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `workspace_id` | string \| null | no | null | Restrict to this workspace; null lists all. |

**Result**: `type: "pane_list"`

| field | type | meaning |
|---|---|---|
| `panes` | array&lt;[PaneInfo](../data-model.md)&gt; | Matching panes. |

**Errors**: `workspace_not_found` (unknown `workspace_id`, inferred); other codes possible.

**CLI**: `herdr pane list [--workspace <WORKSPACE_ID>]`

**Example**

```json
{"id":"cli:pane:list","method":"pane.list","params":{"workspace_id":"w2"}}
{"id":"cli:pane:list","result":{"panes":[{"agent":"claude","agent_session":{"agent":"claude","kind":"id","source":"herdr:claude","value":"ef3b9d04-…"},"agent_status":"working","cwd":"/home/penguin/source/fledge","focused":false,"foreground_cwd":"/home/penguin/source/fledge","pane_id":"w2:p1","revision":4,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":54},"tab_id":"w2:t1","terminal_id":"term_659708952f5514","terminal_title":"◐ herdr-api-documentation","terminal_title_stripped":"herdr-api-documentation","workspace_id":"w2"}],"type":"pane_list"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.move

Move a pane to another location: into an existing tab (`tab`), into a freshly created tab
(`new_tab`), or into a freshly created workspace (`new_workspace`). After the move the
pane receives a new workspace-qualified ID; continue with
`result.move_result.pane.pane_id` (or a live agent name), not
`result.move_result.previous_pane_id` (skill.md).

**Params** (`PaneMoveParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to move. |
| `destination` | PaneMoveDestination | yes | — | Where to move it (tagged union, below). |
| `focus` | boolean | no | false | Whether to focus the pane at its destination. |

`destination` is a `oneOf` discriminated by `type`:

*Variant `tab`* — move into an existing tab:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `type` | const `"tab"` | yes | — | Selects this variant. |
| `tab_id` | string | yes | — | Destination tab. |
| `split` | SplitDirection (`right`,`down`) | yes | — | Split direction against the target within that tab. |
| `target_pane_id` | string \| null | no | null | Pane in the destination tab to split against; null uses the tab's default/focused pane (inferred). |
| `ratio` | number (float) \| null | no | null | Split ratio for the new placement. |

*Variant `new_tab`* — move into a new tab:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `type` | const `"new_tab"` | yes | — | Selects this variant. |
| `workspace_id` | string \| null | no | null | Workspace to create the tab in; null uses the pane's current workspace (inferred). |
| `label` | string \| null | no | null | Label for the new tab. |

*Variant `new_workspace`* — move into a new workspace:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `type` | const `"new_workspace"` | yes | — | Selects this variant. |
| `label` | string \| null | no | null | Label for the new workspace. |
| `tab_label` | string \| null | no | null | Label for the new workspace's initial tab. |

**Result**: `type: "pane_move"`

| field | type | meaning |
|---|---|---|
| `changed` | boolean | Whether the pane actually moved. |
| `pane` | [PaneInfo](../data-model.md) | The pane at its new location (new `pane_id`). |
| `previous_pane_id` | string | The pane's ID before the move (do not reuse as a target). |
| `previous_workspace_id` | string | Workspace before the move. |
| `previous_tab_id` | string | Tab before the move. |
| `focused_pane_id` | string | Pane focused after the move. |
| `target_layout` | [PaneLayoutSnapshot](../data-model.md) | Destination tab layout. |
| `source_layout` | [PaneLayoutSnapshot](../data-model.md) \| null | Origin tab layout after removal (null if the origin tab closed, inferred). |
| `created_tab` | [TabInfo](../data-model.md) \| null | Tab created by a `new_tab`/`new_workspace` move, if any. |
| `created_workspace` | [WorkspaceInfo](../data-model.md) \| null | Workspace created by a `new_workspace` move, if any. |
| `closed_tab_id` | string \| null | Tab closed because it became empty after the move. |
| `closed_workspace_id` | string \| null | Workspace closed because it became empty. |
| `reason` | `same_tab` \| `zoomed_tab` \| null | Why the move was a no-op/constrained, if applicable. |

**Errors**: `pane_not_found`; `tab_not_found` when a `tab` destination names an unknown
`tab_id` (inferred); other codes possible.

**CLI**: `herdr pane move <PANE_ID> [--tab <TAB_ID> --split <right|down> [--target-pane <ID>] [--ratio <FLOAT>] | --new-tab [--workspace <ID>] | --new-workspace] [--label <TEXT>] [--tab-label <TEXT>] [--focus | --no-focus]`

**Example**

```json
{"id":"cli:pane:move","method":"pane.move","params":{"pane_id":"w1:p3","destination":{"type":"new_tab","label":"moved-tab"},"focus":false}}
{"id":"cli:pane:move","result":{"move_result":{"changed":true,"created_tab":{"agent_status":"unknown","focused":false,"label":"moved-tab","number":3,"pane_count":1,"tab_id":"w1:t3","workspace_id":"w1"},"focused_pane_id":"w1:p3","pane":{"agent_status":"unknown","cwd":"/tmp/…/scratch-repo","focused":false,"foreground_cwd":"/tmp/…/scratch-repo","label":"--label docs-pane","pane_id":"w1:p3","revision":1,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":39},"tab_id":"w1:t3","terminal_id":"term_65970bc8a38ec4","terminal_title":"penguin@raft: /tmp/…/scratch-repo","terminal_title_stripped":"penguin@raft: /tmp/…/scratch-repo","workspace_id":"w1"},"previous_pane_id":"w1:p3","previous_tab_id":"w1:t1","previous_workspace_id":"w1","source_layout":{"area":{"height":39,"width":94,"x":26,"y":1},"focused_pane_id":"w1:p1","panes":[{"focused":true,"pane_id":"w1:p1","rect":{"height":39,"width":94,"x":26,"y":1}}],"splits":[],"tab_id":"w1:t1","workspace_id":"w1","zoomed":false},"target_layout":{"area":{"height":39,"width":94,"x":26,"y":1},"focused_pane_id":"w1:p3","panes":[{"focused":true,"pane_id":"w1:p3","rect":{"height":39,"width":94,"x":26,"y":1}}],"splits":[],"tab_id":"w1:t3","workspace_id":"w1","zoomed":false}},"type":"pane_move"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.neighbor

Resolve the pane ID that neighbors a given pane in a direction, without moving focus. A
null `neighbor_pane_id` means there is no neighbor that way.

**Params** (`PaneNeighborParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `direction` | PaneDirection (`left`,`right`,`up`,`down`) | yes | — | Direction to look. |
| `pane_id` | string \| null | no | null | Origin pane; null uses the UI-focused pane. |

**Result**: `type: "pane_neighbor"`

| field | type | meaning |
|---|---|---|
| `pane_id` | string | Origin pane. |
| `direction` | PaneDirection | Direction queried. |
| `neighbor_pane_id` | string \| null | The neighboring pane, or null if none. |
| `layout` | [PaneLayoutSnapshot](../data-model.md) | Enclosing tab layout. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane neighbor --direction <left|right|up|down> [--current | --pane <ID>]`

**Example**

```json
{"id":"cli:pane:neighbor","method":"pane.neighbor","params":{"pane_id":"w2:p1","direction":"right"}}
{"id":"cli:pane:neighbor","result":{"neighbor":{"direction":"right","layout":{"area":{"height":54,"width":166,"x":26,"y":1},"focused_pane_id":"w2:p1","panes":[{"focused":true,"pane_id":"w2:p1","rect":{"height":54,"width":166,"x":26,"y":1}}],"splits":[],"tab_id":"w2:t1","workspace_id":"w2","zoomed":false},"pane_id":"w2:p1"},"type":"pane_neighbor"}}
```

Validated 2026-08-19 against herdr 0.8.2. (Here the single-pane tab has no right neighbor,
so `neighbor_pane_id` is omitted/null.)

---

## pane.process_info

Report OS process information for a pane: its shell PID/TTY and the current foreground
process group and processes.

**Params** (`PaneProcessInfoParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string \| null | no | null | Pane to inspect; null uses the UI-focused pane. |

**Result**: `type: "pane_process_info"`

| field | type | meaning |
|---|---|---|
| `pane_id` | string | Pane inspected. |
| `shell_pid` | integer (uint32) \| null | PID of the pane's shell. |
| `tty` | string \| null | Controlling TTY path. |
| `foreground_process_group_id` | integer (uint32) \| null | Foreground process group ID. |
| `foreground_processes` | array&lt;PaneProcessInfoProcess&gt; | Processes in the foreground group (see below). |

`PaneProcessInfoProcess`:

| field | type | required | meaning |
|---|---|---|---|
| `pid` | integer (uint32) | yes | Process ID. |
| `name` | string | yes | Process name. |
| `argv0` | string \| null | no | `argv[0]` as executed. |
| `argv` | array&lt;string&gt; \| null | no | Full argument vector. |
| `cmdline` | string \| null | no | Raw command line. |
| `cwd` | string \| null | no | Process working directory. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane process-info [--current | --pane <ID>]`

**Example**

```json
{"id":"cli:pane:process_info","method":"pane.process_info","params":{"pane_id":"w2:p1"}}
{"id":"cli:pane:process_info","result":{"process_info":{"foreground_process_group_id":130012,"foreground_processes":[{"argv":["claude"],"cmdline":"claude","cwd":"/home/penguin/source/fledge","name":"claude","pid":130012}],"pane_id":"w2:p1","shell_pid":129736},"type":"pane_process_info"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.read

Read a snapshot of a pane's terminal output. Choose `source` per the task (see the
read-source notes at the top of this file). `lines` bounds how many rows to return;
`strip_ansi` (default true) removes escape sequences unless `format` is `ansi`. CLI reads
do not mark an agent as seen.

**Params** (`PaneReadParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to read. |
| `source` | ReadSource (`visible`,`recent`,`recent_unwrapped`,`detection`) | yes | — | Which snapshot to read. |
| `format` | ReadFormat (`text`,`ansi`) | no | `text` | `text` = plain, `ansi` = keep escape sequences. |
| `lines` | integer (uint32) \| null | no | null | Max rows to return from screen + scrollback; null = server default. |
| `strip_ansi` | boolean | no | true | Strip ANSI escapes from the returned text. |

**Result**: `type: "pane_read"`

| field | type | meaning |
|---|---|---|
| `pane_id` | string | Pane read. |
| `workspace_id` | string | Its workspace. |
| `tab_id` | string | Its tab. |
| `source` | ReadSource | Snapshot source actually used. |
| `format` | ReadFormat | Format of `text`. |
| `text` | string | The captured output. |
| `revision` | integer (uint64) | Pane content revision at capture time. |
| `truncated` | boolean | Whether output was cut off by `lines`. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane read <PANE_ID> [--source <visible|recent|recent-unwrapped|detection>] [--lines <N>] [--format <text|ansi> | --ansi] [--raw]`

**Example**

```json
{"id":"1","method":"pane.read","params":{"pane_id":"w1:p3","source":"recent_unwrapped","lines":120,"format":"text","strip_ansi":true}}
{"id":"1","result":{"type":"pane_read","read":{"pane_id":"w1:p3","workspace_id":"w1","tab_id":"w1:t1","source":"recent_unwrapped","format":"text","text":"echo docprobe-marker-42\n…","revision":0,"truncated":false}}}
```

Constructed from schema; not live-validated. (The `read` sub-object structure matches the
live `output_matched.read` field captured for [pane.wait_for_output](#panewait_for_output).)

---

## pane.release_agent

Release one named agent's lifecycle authority held by a `source` on a pane. Narrower than
[pane.clear_agent_authority](#paneclear_agent_authority), which drops the source's whole
claim on the pane. Idempotent.

**Params** (`PaneReleaseAgentParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `source` | string | yes | — | Reporting source releasing authority. |
| `agent` | string | yes | — | Agent label whose authority is released. |
| `seq` | integer (uint64) \| null | no | null | Monotonic sequence for ordering reports from this source. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane release-agent <PANE_ID> --source <ID> --agent <LABEL> [--seq <N>]`

**Example**

```json
{"id":"p7","method":"pane.release_agent","params":{"pane_id":"w1:p1","source":"docprobe","agent":"claude"}}
{"id":"p7","result":{"type":"ok"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.rename

Set or clear a pane's user-facing label. A null/omitted `label` clears it (CLI `--clear`).

**Params** (`PaneRenameParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to rename. |
| `label` | string \| null | no | null | New label; null clears the label. |

**Result**: `type: "pane_info"`

| field | type | meaning |
|---|---|---|
| `pane` | [PaneInfo](../data-model.md) | The updated pane. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane rename <PANE_ID> [LABEL]... [--clear]`

**Example**

```json
{"id":"cli:pane:rename","method":"pane.rename","params":{"pane_id":"w1:p3","label":"docs-pane"}}
{"id":"cli:pane:rename","result":{"pane":{"agent_status":"unknown","cwd":"/tmp/…/scratch-repo","focused":false,"foreground_cwd":"/tmp/…/scratch-repo","label":"--label docs-pane","pane_id":"w1:p3","revision":0,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":39},"tab_id":"w1:t1","terminal_id":"term_65970bc8a38ec4","workspace_id":"w1"},"type":"pane_info"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.report_agent

Report a coding agent's lifecycle state for a pane, from an integration acting as
`source`. This is how an agent integration tells herdr its `idle`/`working`/`blocked`/
`unknown` state and optional session identity. Establishes/refreshes the source's
authority over that pane's agent reporting until released.

**Params** (`PaneReportAgentParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane hosting the agent. |
| `source` | string | yes | — | Reporting integration/source ID. |
| `agent` | string | yes | — | Agent label. |
| `state` | PaneAgentState (`idle`,`working`,`blocked`,`unknown`) | yes | — | Reported lifecycle state. |
| `message` | string \| null | no | null | Human-readable status message. |
| `seq` | integer (uint64) \| null | no | null | Monotonic sequence for ordering reports from this source. |
| `agent_session_id` | string \| null | no | null | Agent session identifier (ID form). |
| `agent_session_path` | string \| null | no | null | Agent session identifier (path form). |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane report-agent <PANE_ID> --source <ID> --agent <LABEL> --state <idle|working|blocked|unknown> [--message <TEXT>] [--seq <N>] [--agent-session-id <ID>] [--agent-session-path <PATH>]`

**Example**

```json
{"id":"p4","method":"pane.report_agent","params":{"pane_id":"w1:p1","source":"docprobe","agent":"claude","state":"working"}}
{"id":"p4","result":{"type":"ok"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.report_agent_session

Report (or update) the agent **session identity** for a pane without changing lifecycle
state. Use it to attach a session ID/path and record where the session started.

**Params** (`PaneReportAgentSessionParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane hosting the agent. |
| `source` | string | yes | — | Reporting integration/source ID. |
| `agent` | string | yes | — | Agent label. |
| `agent_session_id` | string \| null | no | null | Session identifier (ID form). |
| `agent_session_path` | string \| null | no | null | Session identifier (path form). |
| `session_start_source` | string \| null | no | null | Where/how the session was started (inferred). |
| `seq` | integer (uint64) \| null | no | null | Monotonic sequence for ordering reports from this source. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane report-agent-session <PANE_ID> --source <ID> --agent <LABEL> [--seq <N>] [--agent-session-id <ID>] [--agent-session-path <PATH>] [--session-start-source <SOURCE>]`

**Example**

```json
{"id":"p5","method":"pane.report_agent_session","params":{"pane_id":"w1:p1","source":"docprobe","agent":"claude","agent_session_id":"abc123"}}
{"id":"p5","result":{"type":"ok"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.report_metadata

Report display-only pane metadata: title, per-status labels, arbitrary tokens, and a
display-agent name, optionally with a TTL. These affect only presentation, not agent
lifecycle. Boolean `clear_*` flags remove the corresponding metadata. `tokens` keys must
match `^[A-Za-z0-9_-]{1,32}$` (max 16 keys); `state_labels` map status → label.

**Params** (`PaneReportMetadataParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `source` | string | yes | — | Reporting source ID. |
| `agent` | string \| null | no | null | Agent label this metadata is about. |
| `applies_to_source` | string \| null | no | null | Restrict metadata to reports from this source (inferred). |
| `title` | string \| null | no | null | Display title to set. |
| `clear_title` | boolean | no | false | Clear the display title. |
| `display_agent` | string \| null | no | null | Display-agent name to show. |
| `clear_display_agent` | boolean | no | false | Clear the display-agent name. |
| `state_labels` | object&lt;string,string&gt; | no | — | Map of status → custom label text. |
| `clear_state_labels` | boolean | no | false | Clear all custom state labels. |
| `tokens` | object&lt;string,string\|null&gt; | no | — | Display tokens (≤16 keys, key pattern `^[A-Za-z0-9_-]{1,32}$`; null value clears one key, inferred). |
| `seq` | integer (uint64) \| null | no | null | Monotonic sequence for ordering reports from this source. |
| `ttl_ms` | integer (uint64) \| null | no | null | Expiry in ms (1..=86400000) after which metadata is dropped. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane report-metadata <PANE_ID> --source <ID> [--agent <LABEL>] [--applies-to-source <ID>] [--title <TEXT> | --clear-title] [--display-agent <TEXT> | --clear-display-agent] [--state-label <STATUS=TEXT> | --clear-state-labels] [--token <NAME=VALUE> | --clear-token <NAME>] [--seq <N>] [--ttl-ms <N>]`

**Example**

```json
{"id":"1","method":"pane.report_metadata","params":{"pane_id":"w1:p1","source":"docprobe","title":"build","tokens":{"branch":"main"},"ttl_ms":60000}}
{"id":"1","result":{"type":"ok"}}
```

Constructed from schema; not live-validated.

---

## pane.resize

Resize the split enclosing a pane by nudging it in a direction. `amount` (a float ratio
delta) is optional; omitting it uses the server's default step. When the resize has no
effect, `changed` is false and `reason` is `unchanged`.

**Params** (`PaneResizeParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `direction` | PaneDirection (`left`,`right`,`up`,`down`) | yes | — | Direction to grow/shrink. |
| `pane_id` | string \| null | no | null | Pane to resize; null uses the UI-focused pane. |
| `amount` | number (float) \| null | no | null | Resize magnitude (ratio delta); null = default step. |

**Result**: `type: "pane_resize"`

| field | type | meaning |
|---|---|---|
| `changed` | boolean | Whether geometry changed. |
| `pane_id` | string | Pane resized. |
| `focused_pane_id` | string | Focused pane after resize. |
| `layout` | [PaneLayoutSnapshot](../data-model.md) | Resulting tab layout. |
| `reason` | `unchanged` \| null | Why nothing changed, if applicable. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane resize --direction <left|right|up|down> [--amount <FLOAT>] [--current | --pane <ID>]`

**Example**

```json
{"id":"cli:pane:resize","method":"pane.resize","params":{"pane_id":"w1:p3","direction":"right","amount":0.1}}
{"id":"cli:pane:resize","result":{"resize":{"changed":true,"focused_pane_id":"w1:p1","layout":{"area":{"height":39,"width":94,"x":26,"y":1},"focused_pane_id":"w1:p1","panes":[{"focused":false,"pane_id":"w1:p3","rect":{"height":39,"width":56,"x":26,"y":1}},{"focused":true,"pane_id":"w1:p1","rect":{"height":39,"width":38,"x":82,"y":1}}],"splits":[{"direction":"right","id":"split_0_root","ratio":0.6,"rect":{"height":39,"width":94,"x":26,"y":1}}],"tab_id":"w1:t1","workspace_id":"w1","zoomed":false},"pane_id":"w1:p3"},"type":"pane_resize"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.send_input

Send text and/or a sequence of logical keys to a pane in one call — the general-purpose
input primitive underlying `send_text` and `send_keys`. At least a `pane_id` is required;
`text` and `keys` are both optional and, when both present, are delivered together.

**Params** (`PaneSendInputParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `text` | string | no | — | Literal text to type. |
| `keys` | array&lt;string&gt; | no | — | Logical key names to press (e.g. `enter`, `esc`, `ctrl+c`). |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; `invalid`/key-validation errors if a key name is unknown
(inferred — herdr validates all keys before writing any bytes); other codes possible.

**CLI**: API-only (no CLI subcommand); the CLI splits this into `send-text`, `send-keys`,
and `run`.

**Example**

```json
{"id":"p1","method":"pane.send_input","params":{"pane_id":"w1:p1","text":"true\r"}}
{"id":"p1","result":{"type":"ok"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.send_keys

Send a sequence of logical key presses to a pane. herdr validates every key name before
writing any bytes. Use `esc` as the canonical Escape name (`escape` is also accepted).

**Params** (`PaneSendKeysParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `keys` | array&lt;string&gt; | yes | — | Logical key names to press in order (e.g. `enter`, `esc`, `ctrl+c`). |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; key-validation error on an unknown key name (inferred);
other codes possible.

**CLI**: `herdr pane send-keys <PANE_ID> <KEY>...`

**Example**

```json
{"id":"1","method":"pane.send_keys","params":{"pane_id":"w1:p1","keys":["ctrl+c"]}}
{"id":"1","result":{"type":"ok"}}
```

Constructed from schema; not live-validated.

---

## pane.send_text

Send literal text to a pane's terminal (no implicit Enter). To send text plus Enter
atomically, use the CLI `run` verb (which drives `send_input`).

**Params** (`PaneSendTextParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `text` | string | yes | — | Literal text to type. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane send-text <PANE_ID> <TEXT>` (and `herdr pane run <PANE_ID> <COMMAND>...`
sends text followed by Enter in one call).

**Example**

```json
{"id":"1","method":"pane.send_text","params":{"pane_id":"w1:p1","text":"echo hello"}}
{"id":"1","result":{"type":"ok"}}
```

Constructed from schema; not live-validated.

---

## pane.split

Split a pane, creating a new sibling pane running a fresh shell (or, with `cwd`/`env`, a
shell in a chosen directory/environment). Returns the new pane as `result.pane`; read its
ID from `result.pane.pane_id`. Keep the caller's focus with `focus: false` unless the user
wants focus moved. `direction` is required and restricted to `right`/`down`.

**Params** (`PaneSplitParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `direction` | SplitDirection (`right`,`down`) | yes | — | Split the target to the right or downward. |
| `target_pane_id` | string \| null | no | null | Pane to split; null uses the UI-focused pane (inferred). |
| `workspace_id` | string \| null | no | null | Workspace context for the split (inferred). |
| `ratio` | number (float) \| null | no | null | Initial split ratio; null = default. |
| `cwd` | string \| null | no | null | Working directory for the new pane's shell. |
| `env` | object&lt;string,string&gt; | no | — | Extra environment variables for the launched process. |
| `right_click` | PaneRightClickTarget (`herdr`,`pane`) | no | `herdr` | Right-click routing for the new pane. |
| `focus` | boolean | no | false | Whether to focus the new pane. |

**Result**: `type: "pane_info"`

| field | type | meaning |
|---|---|---|
| `pane` | [PaneInfo](../data-model.md) | The newly created pane. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane split [--current | --pane <ID> | PANE_ID] --direction <right|down> [--ratio <FLOAT>] [--cwd <PATH>] [--env <KEY=VALUE>] [--right-click <herdr|pane>] [--focus | --no-focus]`

**Example**

```json
{"id":"cli:pane:split","method":"pane.split","params":{"target_pane_id":"w1:p1","direction":"right","cwd":"/tmp/…/scratch-repo","focus":false}}
{"id":"cli:pane:split","result":{"pane":{"agent_status":"unknown","cwd":"/tmp/…/scratch-repo","focused":false,"foreground_cwd":"/tmp/…/scratch-repo","pane_id":"w1:p3","revision":0,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":39},"tab_id":"w1:t1","terminal_id":"term_65970bc8a38ec4","workspace_id":"w1"},"type":"pane_info"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.swap

Swap two panes' positions in the layout. Targets may be given explicitly
(`source_pane_id`/`target_pane_id`) or a `direction` from `pane_id` selects the neighbor
to swap with. When the swap cannot happen, `changed` is false and `reason` explains why.

**Params** (`PaneSwapParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string \| null | no | null | Reference pane (used with `direction`); null uses the UI-focused pane. |
| `direction` | PaneDirection (`left`,`right`,`up`,`down`) \| null | no | null | Swap with the neighbor in this direction. |
| `source_pane_id` | string \| null | no | null | Explicit first pane to swap. |
| `target_pane_id` | string \| null | no | null | Explicit second pane to swap. |

**Result**: `type: "pane_swap"`

| field | type | meaning |
|---|---|---|
| `changed` | boolean | Whether panes were swapped. |
| `source_pane_id` | string | First pane involved. |
| `target_pane_id` | string \| null | Second pane involved (null if none resolved). |
| `focused_pane_id` | string | Focused pane after the swap. |
| `layout` | [PaneLayoutSnapshot](../data-model.md) | Resulting tab layout. |
| `reason` | `no_neighbor` \| `same_pane` \| `not_found` \| `cross_tab` \| null | Why the swap did not happen, if applicable. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane swap [--direction <left|right|up|down>] [--current | --pane <ID>] [--source-pane <ID>] [--target-pane <ID>]`

**Example**

```json
{"id":"cli:pane:swap","method":"pane.swap","params":{"source_pane_id":"w1:p1","target_pane_id":"w1:p3"}}
{"id":"cli:pane:swap","result":{"swap":{"changed":true,"focused_pane_id":"w1:p1","layout":{"area":{"height":39,"width":94,"x":26,"y":1},"focused_pane_id":"w1:p1","panes":[{"focused":false,"pane_id":"w1:p3","rect":{"height":39,"width":47,"x":26,"y":1}},{"focused":true,"pane_id":"w1:p1","rect":{"height":39,"width":47,"x":73,"y":1}}],"splits":[{"direction":"right","id":"split_0_root","ratio":0.5,"rect":{"height":39,"width":94,"x":26,"y":1}}],"tab_id":"w1:t1","workspace_id":"w1","zoomed":false},"source_pane_id":"w1:p1","target_pane_id":"w1:p3"},"type":"pane_swap"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.wait_for_output

Block until a pane's output matches a pattern. The selected snapshot is searched
immediately (existing output can match), then polled until match or `timeout_ms`.
Omitting `timeout_ms` waits indefinitely. Returns the matched line plus a `PaneReadResult`
snapshot. This is a long-running request; the connection stays open until it resolves.

**Params** (`PaneWaitForOutputParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to watch. |
| `source` | ReadSource (`visible`,`recent`,`recent_unwrapped`,`detection`) | yes | — | Snapshot source to search. |
| `match` | OutputMatch | yes | — | Match specification (tagged union, below). |
| `lines` | integer (uint32) \| null | no | null | Restrict the searched snapshot to N rows. |
| `strip_ansi` | boolean | no | true | Strip ANSI escapes before matching (CLI `--raw` sets false). |
| `timeout_ms` | integer (uint64) \| null | no | null | Fail with a timeout after this many ms; null = wait indefinitely. |

`match` is a `oneOf` discriminated by `type`:

| variant | fields | meaning |
|---|---|---|
| `substring` | `type` const `"substring"`, `value` string | Match a literal substring. |
| `regex` | `type` const `"regex"`, `value` string | Match a Rust regular expression. |

**Result**: `type: "output_matched"`

| field | type | meaning |
|---|---|---|
| `pane_id` | string | Pane that matched. |
| `revision` | integer (uint64) | Pane content revision at match time. |
| `matched_line` | string \| null | The line that matched (null if not line-scoped, inferred). |
| `read` | [PaneReadResult](../data-model.md) | Snapshot at match time (`pane_id`, `workspace_id`, `tab_id`, `source`, `format`, `text`, `revision`, `truncated`). |

**Errors**: `pane_not_found`; a timeout error when `timeout_ms` elapses without a match
(inferred); other codes possible.

**CLI**: `herdr pane wait-output <PANE_ID> <--match <TEXT> | --regex <PATTERN>> [--source <visible|recent|recent-unwrapped>] [--lines <N>] [--timeout <MS>] [--raw]`

**Example**

```json
{"id":"cli:pane:wait-output","method":"pane.wait_for_output","params":{"pane_id":"w1:p3","source":"recent_unwrapped","match":{"type":"substring","value":"docprobe-marker-42"}}}
{"id":"cli:pane:wait-output","result":{"matched_line":"echo docprobe-marker-42","pane_id":"w1:p3","read":{"format":"text","pane_id":"w1:p3","revision":0,"source":"recent_unwrapped","tab_id":"w1:t1","text":"echo docprobe-marker-42","truncated":false,"workspace_id":"w1"},"revision":0,"type":"output_matched"}}
```

Validated 2026-08-19 against herdr 0.8.2.

---

## pane.zoom

Toggle, enable, or disable **zoom** — temporarily maximizing one pane to fill its tab.
`mode` defaults to `toggle`. When the tab has a single pane or is already in the requested
state, `zoom_changed` is false and `reason` explains why.

**Params** (`PaneZoomParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `mode` | PaneZoomMode (`toggle`,`on`,`off`) | no | `toggle` | Whether to toggle, force on, or force off. |
| `pane_id` | string \| null | no | null | Pane to zoom; null uses the UI-focused pane. |

**Result**: `type: "pane_zoom"`

| field | type | meaning |
|---|---|---|
| `changed` | boolean | Whether anything changed (zoom or focus). |
| `zoom_changed` | boolean | Whether the zoom state changed. |
| `focus_changed` | boolean | Whether focus changed as a result. |
| `pane_id` | string | Pane targeted. |
| `focused_pane_id` | string | Focused pane after the call. |
| `zoomed` | boolean | Zoom state after the call. |
| `layout` | [PaneLayoutSnapshot](../data-model.md) | Resulting tab layout (its `zoomed` reflects the new state). |
| `reason` | `single_pane` \| `already_zoomed` \| `already_unzoomed` \| null | Why zoom did not change, if applicable. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane zoom [PANE_ID] [--current | --pane <ID>] [--toggle | --on | --off]`

**Example**

```json
{"id":"cli:pane:zoom","method":"pane.zoom","params":{"pane_id":"w1:p3","mode":"on"}}
{"id":"cli:pane:zoom","result":{"type":"pane_zoom","zoom":{"changed":true,"focus_changed":true,"focused_pane_id":"w1:p3","layout":{"area":{"height":39,"width":94,"x":26,"y":1},"focused_pane_id":"w1:p3","panes":[{"focused":false,"pane_id":"w1:p1","rect":{"height":39,"width":47,"x":26,"y":1}},{"focused":true,"pane_id":"w1:p3","rect":{"height":39,"width":47,"x":73,"y":1}}],"splits":[{"direction":"right","id":"split_0_root","ratio":0.5,"rect":{"height":39,"width":94,"x":26,"y":1}}],"tab_id":"w1:t1","workspace_id":"w1","zoomed":true},"pane_id":"w1:p3","zoom_changed":true,"zoomed":true}}}
```

Validated 2026-08-19 against herdr 0.8.2.
