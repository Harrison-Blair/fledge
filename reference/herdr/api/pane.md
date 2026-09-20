# herdr API: pane methods

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `pane.*` namespace controls individual terminal panes: the leaf terminals inside a
workspace/tab layout tree. Methods here inspect pane topology and geometry (`list`,
`get`, `current`, `layout`, `edges`, `neighbor`, `process_info`), mutate the split tree
(`split`, `swap`, `move`, `resize`, `zoom`, `close`, `focus`, `focus_direction`), drive a
pane's terminal (`read`, `wait_for_output`, `send_text`, `send_keys`, `send_input`,
`rename`, `input.set`), render inline images (`graphics.set`, `graphics.clear`,
`graphics.info`), scroll and copy a pane's content and resolve/activate links in it
(`scroll`, `edit_scrollback`, `copy_motion`, `copy_search`, `selection.read`,
`link.resolve`, `link.activate`), and let an integration report/withdraw agent-lifecycle and display
metadata for a pane (`report_agent`, `report_agent_session`, `report_metadata`,
`release_agent`, `clear_agent_authority`). A pane exists whether or not it hosts a
recognized agent; use `agent.*` methods when herdr must validate agent identity or
interpret lifecycle states. Panes are addressed by workspace-qualified IDs such as
`w1:p1` (see [addressing.md](../addressing.md)); nine methods accept a null/omitted
`pane_id` (`target_pane_id` for `split`) to target the UI-focused pane — four
topology/read methods (`layout`, `edges`, `neighbor`, `process_info`) and five
pane-arrangement methods (`focus_direction`, `resize`, `split`, `swap`, `zoom`), all
exposed on the CLI as `[--current | --pane <ID>]` — while the remaining methods that
take a pane require an explicit `pane_id` string, and `pane.current` resolves the
caller's pane from `caller_pane_id` instead — but callers should pass their own
`$HERDR_PANE_ID` to avoid targeting another client's focus.

Domain entities (`PaneInfo`, `AgentInfo`, `PaneLayoutSnapshot`, `WorkspaceInfo`,
`TabInfo`, …) are defined once in [data-model.md](../data-model.md); result tables below
name each field and link there rather than re-expanding embedded entities. Nine methods
(`edges`, `focus_direction`, `move`, `neighbor`, `process_info`, `read`, `resize`,
`swap`, `zoom`) nest those fields one level down, inside a result object named after the
method (`edges`, `focus`, `move_result`, `neighbor`, `process_info`, `read`, `resize`,
`swap`, `zoom`) rather than at the top level; each such section's table is written
against that nested object, matching its example.

Across this namespace `pane_not_found`'s message text is not stable (e.g. `"pane w9:p9
not found"` on some methods, a bare `"pane not found"` or `"source pane not found"` on
others) — match on `code`, never on `message`. `params` is mandatory on every request
even when every field inside it is optional; omitting it fails with `invalid_request`
(`"missing field \`params\`"`). A result field documented as nullable is generally
*omitted* rather than sent as `null` when unset; treat missing and `null` alike unless a
section says otherwise.

37 documented methods (the installed 0.9.1 binary also accepts an undocumented
`pane.graphics.stream`, named in the server's own "unknown variant" error text but absent
from `raw/schema.json`; no section below covers it):

| method | purpose |
|---|---|
| [pane.clear_agent_authority](#paneclear_agent_authority) | Withdraw a source's authority over a pane's agent lifecycle reporting |
| [pane.close](#paneclose) | Close a pane and its terminal |
| [pane.copy_motion](#panecopy_motion) | Move a copy-mode cursor by a text motion |
| [pane.copy_search](#panecopy_search) | Search pane text for copy-mode match navigation |
| [pane.current](#panecurrent) | Return the pane the caller/UI is currently in |
| [pane.edges](#paneedges) | Report which of a pane's four edges border the tab boundary |
| [pane.edit_scrollback](#paneedit_scrollback) | Open a pane's scrollback in an external editor |
| [pane.focus](#panefocus) | Focus a specific pane by ID and return its pane info |
| [pane.focus_direction](#panefocus_direction) | Move focus to the neighboring pane in a direction |
| [pane.get](#paneget) | Fetch a single pane's `PaneInfo` |
| [pane.graphics.clear](#panegraphicsclear) | Clear graphics layer(s) from a pane |
| [pane.graphics.info](#panegraphicsinfo) | Report a pane's graphics capabilities |
| [pane.graphics.set](#panegraphicsset) | Draw/replace an image layer in a pane |
| [pane.input.set](#paneinputset) | Set a pane's right-click input routing |
| [pane.layout](#panelayout) | Return the layout snapshot of a pane's tab |
| [pane.link.activate](#panelinkactivate) | Activate a detected link at a pane viewport position |
| [pane.link.resolve](#panelinkresolve) | Resolve link region(s) at a pane viewport position |
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
| [pane.scroll](#panescroll) | Set a pane's scrollback offset from the bottom |
| [pane.selection.read](#paneselectionread) | Read the text spanned by a selection range in a pane |
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
- **PaneAgentState**: `idle`, `working`, `blocked`, `unknown` — the states a
  `pane.report_agent` call may report. `PaneInfo.agent_status` is a separate `AgentStatus`
  with a fifth, observation-only value, `done`, that no report can set directly: reporting
  `idle` after `working`/`blocked` surfaces as `done` until the pane is focused/seen (see
  [pane.report_agent](#panereport_agent)).
- **PaneGraphicsFormat**: `png`, `rgb`, `rgba`, `bgra`.
- **PaneCopyMotion**: `line_end`, `first_non_blank`, `next_word_start`, `previous_word_start`,
  `next_word_end`, `next_big_word_start`, `previous_big_word_start`, `next_big_word_end`,
  `previous_paragraph`, `next_paragraph`.
- **PaneCopySearchDirection**: `forward`, `backward`.

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
holds no authority. Emits `pane_agent_detected` (with `agent` absent) and
`pane.agent_status_changed` (`agent_status` `"unknown"`).

**Params** (`PaneClearAgentAuthorityParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane ID. |
| `source` | string \| null | no | null | Reporting source whose authority is cleared. A null/omitted `source` acts as a wildcard, clearing whichever source currently holds authority — it is not a distinct "ambient/default" source. |
| `seq` | integer (uint64) \| null | no | null | Monotonic sequence number for ordering out-of-order reports from the same source (inferred). A clear is applied only when its `seq` is strictly greater than the last report's `seq` from that source; an equal, lower, null, or omitted `seq` after a seq'd report returns `ok` but silently changes nothing. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found` (unknown `pane_id`); other codes possible.

**CLI**: API-only (no CLI subcommand). The CLI exposes `release-agent` but not a bare
authority-clear.

**Example**

```json
{"id":"p6","method":"pane.clear_agent_authority","params":{"pane_id":"w1:p1","source":"docprobe"}}
{"id":"p6","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1 (seq-gate and event behaviour from evidence;
example response shape unchanged since 0.8.2).

---

## pane.close

Close a pane and terminate its terminal process. Do not close panes the caller did not
create unless the user explicitly asked (skill.md). Closed pane IDs are not reused.
Closing a workspace's last pane closes the workspace too. Emits `pane_closed`.

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

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.copy_motion

Move a copy-mode cursor from a given position by a text motion (word/line/paragraph
stepping, vi-style) and return where it lands. Does not itself read or select text; pair
with [pane.selection.read](#paneselectionread) to fetch a span.

**Params** (`PaneCopyMotionParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `cursor` | PaneTextPoint (below) | yes | — | Copy-mode cursor position to move from. |
| `motion` | PaneCopyMotion (`line_end`,`first_non_blank`,`next_word_start`,`previous_word_start`,`next_word_end`,`next_big_word_start`,`previous_big_word_start`,`next_big_word_end`,`previous_paragraph`,`next_paragraph`) | yes | — | Motion to apply. |
| `content_revision` | integer (uint64) \| null | no | null | Content revision the cursor position is relative to; null skips the check (inferred). |

`PaneTextPoint` (a zero-based row/column position in a pane's text content, `row` indexed
from the top of the pane's scrollback rather than the visible viewport; also used by
[pane.copy_search](#panecopy_search) and [pane.selection.read](#paneselectionread) —
[pane.link.resolve](#panelinkresolve)/[pane.link.activate](#panelinkactivate) use a
separate, viewport-relative row instead):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `row` | integer (uint32) | yes | — | Zero-based row, from the top of scrollback. |
| `col` | integer (uint16) | yes | — | Zero-based column. |

An out-of-range `cursor` is not an error: the position is echoed back unchanged, with no
signal that the motion had nothing to act on.

**Result**: `type: "pane_copy_motion"`

| field | type | meaning |
|---|---|---|
| `pane_id` | string | Pane targeted. |
| `cursor` | PaneTextPoint | Cursor position after applying the motion. |
| `content_revision` | integer (uint64) | Content revision the returned cursor is relative to. This is a private copy-mode counter — it is not `pane.read`'s `revision` (constant 0) or `pane.get`'s `PaneInfo.revision` (a third counter). The only documented way to obtain a valid value for `content_revision` elsewhere in this namespace is from a prior `copy_motion`/`copy_search` result. |

**Errors**: `pane_not_found`; a `content_revision` that does not match the pane's current
copy-mode counter (older, newer, or arbitrary) fails with the undocumented
`stale_content` (`"pane content changed"`); other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"cm1","method":"pane.copy_motion","params":{"pane_id":"w1:p1","cursor":{"row":0,"col":0},"motion":"next_word_start"}}
{"id":"cm1","result":{"type":"pane_copy_motion","pane_id":"w1:p1","cursor":{"row":0,"col":1},"content_revision":32}}
```

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.copy_search

Search a pane's text content for copy-mode match navigation (find-next/previous),
starting from a cursor position and, for repeat searches, continuing past a `previous`
match range. The search covers the whole scrollback, not just the visible viewport, is
case-sensitive, and an empty `query` is accepted and returns zero matches rather than an
error.

**Params** (`PaneCopySearchParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `query` | string | yes | — | Text to search for. |
| `direction` | PaneCopySearchDirection (`forward`,`backward`) | yes | — | Search direction. |
| `cursor` | PaneTextPoint (above) | yes | — | Position to search from. |
| `content_revision` | integer (uint64) | yes | — | Content revision the search is relative to. |
| `previous` | PaneTextRange \| null | no | null | Previously matched range, to continue searching past it (inferred). |

`PaneTextRange` (also used in this method's result):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `start` | PaneTextPoint (above) | yes | — | Range start point. |
| `end` | PaneTextPoint (above) | yes | — | Range end point. |

**Result**: `type: "pane_copy_search"`

| field | type | meaning |
|---|---|---|
| `pane_id` | string | Pane searched. |
| `content_revision` | integer (uint64) | Content revision the results are relative to. |
| `matches` | array&lt;PaneTextRange&gt; | All matching ranges found. |
| `total` | integer (uint64) | Count of `matches`. |
| `current` | integer (uint32) \| null | Index into `matches` of the current match; **omitted**, not `null`, when there is no current match (e.g. `total: 0`). |
| `current_global` | integer (uint64) \| null | Global match index across the search history (inferred); omitted under the same condition as `current`, and equal to it in every single-search probe. |

**Errors**: `pane_not_found`; a `content_revision` that does not match the pane's current
copy-mode counter fails with the undocumented `stale_content` (`"pane content changed"`)
— since `content_revision` is required here and obtainable only from a prior
`copy_motion`/`copy_search` result (see [pane.copy_motion](#panecopy_motion)),
`stale_content` is the default outcome for a first call; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"cs1","method":"pane.copy_search","params":{"pane_id":"w1:p1","query":"docprobe","direction":"forward","cursor":{"row":0,"col":0},"content_revision":32}}
{"id":"cs1","result":{"type":"pane_copy_search","pane_id":"w1:p1","content_revision":32,"matches":[{"start":{"row":0,"col":26},"end":{"row":0,"col":33}},{"start":{"row":1,"col":0},"end":{"row":1,"col":7}},{"start":{"row":2,"col":26},"end":{"row":2,"col":33}},{"start":{"row":3,"col":0},"end":{"row":3,"col":7}}],"total":4,"current":0,"current_global":0}}
```

Validated 2026-09-19 against herdr 0.9.1.

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

Validated 2026-09-19 against herdr 0.9.1 (the example's `agent`/`agent_session` values
come from a real agent session and were not re-captured; the shape and the
`caller_pane_id` behaviour were re-confirmed).

---

## pane.edges

Report, for a pane, whether each of its four edges lies on the tab's outer boundary
(`true`) versus adjoining another pane (`false`). Useful for deciding whether a
directional focus/move would leave the tab. Also returns the full tab layout snapshot.

**Params** (`PaneEdgesParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string \| null | no | null | Pane to inspect; null targets the UI-focused pane. |

**Result**: `type: "pane_edges"`, with the fields below nested one level down under `edges`:

| field | type | meaning |
|---|---|---|
| `edges` | PaneEdgesResult (below) | Edge and layout data. |

`PaneEdgesResult`:

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

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.edit_scrollback

Open a pane's scrollback buffer in an external editor for browsing/copying. The target
must be the currently focused pane. Each call also allocates a real (if short-lived)
pane, consuming a pane-id slot from the workspace counter even though it never appears
in `pane.list` once the editor process exits — combined with "closed pane IDs are not
reused", visible pane numbers can skip as a result.

**Params** (`PaneTarget`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane whose scrollback to edit. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; a `pane_id` that is not the currently focused pane fails
with the undocumented `stale_pane_target` (`"pane is no longer focused"`) — focus the
pane first; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"e1","method":"pane.edit_scrollback","params":{"pane_id":"w1:p1"}}
{"id":"e1","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1 (focus precondition, `stale_pane_target`, and
pane-id consumption confirmed; the editor actually opening was not probed — the scratch
server resolves no `$EDITOR`/opener, so the spawned pane exits immediately).

---

## pane.focus

Focus a specific pane by ID and return its `PaneInfo`. Focusing marks
the pane's agent (and its tab) as **seen**, which collapses a background `done` state
back to observed `idle` (skill.md). Unlike [pane.focus_direction](#panefocus_direction),
this targets an exact pane rather than a neighbor. Emits `pane_focused`. `tab.focus` on
the pane's tab has the same seen/`done`→`idle` effect regardless of which pane inside it
is focused (see [tab.md](tab.md)).

**Params** (`PaneTarget`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to focus. |

**Result**: `type: "pane_info"` (live-validated).

| field | type | meaning |
|---|---|---|
| `pane` | [PaneInfo](../data-model.md) | The now-focused pane, including its IDs, cwd, focus state, and agent status. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: API-only (no CLI subcommand); the CLI `herdr pane focus` maps to
[pane.focus_direction](#panefocus_direction).

**Example**

```json
{"id":"fledge-focus-probe","method":"pane.focus","params":{"pane_id":"wQ:p6"}}
{"id":"fledge-focus-probe","result":{"type":"pane_info","pane":{"pane_id":"wQ:p6","terminal_id":"term_65bb7c91616a65","workspace_id":"wQ","tab_id":"wQ:t5","focused":true,"cwd":"/home/penguin/source/fledge","foreground_cwd":"/home/penguin/source/fledge","label":"Claude smoke test","terminal_title":"penguin@iceberg:~/source/fledge","terminal_title_stripped":"penguin@iceberg:~/source/fledge","agent_status":"unknown","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":0,"viewport_rows":58},"revision":1}}}
```

Validated 2026-09-19 against herdr 0.9.1 (this example was originally captured 2026-09-18
by a direct socket probe against the live session's `wQ:p6`, replacing an earlier
inferred `agent_info` response, and preserves that captured response including
session-specific IDs and paths; a repeated focus call returns the same `pane_info`/`pane`
shape; the `done`→`idle` collapse and `pane_focused` event were confirmed separately
against a scratch server).

---

## pane.focus_direction

Move keyboard focus from a pane to its neighbor in a given direction. When there is no
neighbor in that direction, `changed` is `false` and `reason` is `no_neighbor`.

**Params** (`PaneFocusDirectionParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `direction` | PaneDirection (`left`,`right`,`up`,`down`) | yes | — | Direction to move focus. |
| `pane_id` | string \| null | no | null | Source pane; null uses the UI-focused pane. |

**Result**: `type: "pane_focus_direction"`, with the fields below nested one level down
under `focus`:

| field | type | meaning |
|---|---|---|
| `focus` | PaneFocusDirectionResult (below) | Focus-move outcome. |

`PaneFocusDirectionResult`:

| field | type | meaning |
|---|---|---|
| `changed` | boolean | Whether focus actually moved. |
| `source_pane_id` | string | Pane focus started from. |
| `focused_pane_id` | string | Pane focused after the call. Never null: when `changed` is false it holds whichever pane is currently focused, which may be neither `source_pane_id` nor a neighbor of it. |
| `reason` | `no_neighbor` | Why focus did not change; **omitted** (not `null`) whenever `changed` is `true`. |
| `layout` | [PaneLayoutSnapshot](../data-model.md) | Resulting tab layout. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane focus --direction <left|right|up|down> [--current | --pane <ID>]`

**Example**

```json
{"id":"1","method":"pane.focus_direction","params":{"pane_id":"w1:p1","direction":"right"}}
{"id":"1","result":{"type":"pane_focus_direction","focus":{"changed":true,"source_pane_id":"w1:p1","focused_pane_id":"w1:p2","layout":{"workspace_id":"w1","tab_id":"w1:t1","zoomed":false,"area":{"x":0,"y":0,"width":120,"height":40},"focused_pane_id":"w1:p2","panes":[{"pane_id":"w1:p1","focused":false,"rect":{"x":0,"y":0,"width":60,"height":40}},{"pane_id":"w1:p2","focused":true,"rect":{"x":60,"y":0,"width":60,"height":40}}],"splits":[{"id":"split_0_root","direction":"right","ratio":0.5,"rect":{"x":0,"y":0,"width":120,"height":40}}]}}}}
```

Validated 2026-09-19 against herdr 0.9.1.

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

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.graphics.clear

Clear a graphics image layer from a pane by name. Requires the `terminal.kitty_graphics`
feature, which is **enabled by default** (`default-config.toml`'s `[terminal]` section
documents the default as a commented-out `# kitty_graphics = true`, not an active
key=value pair — confirmed live, since a fresh scratch server with no custom config
returns `cell_size_unavailable` rather than `feature_disabled` from
[pane.graphics.info](#panegraphicsinfo)); otherwise returns `feature_disabled`. A
null/omitted `layer_id` is a **silent no-op**, not a clear-all: it clears nothing, so a
pane stays pinned at `max_layers_per_pane` and every later
[pane.graphics.set](#panegraphicsset) fails with `layer_limit` once the limit is
reached. Clear each layer you created by its own `layer_id`.

**Params** (`PaneGraphicsClearParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane whose graphics to clear. |
| `layer_id` | string \| null | no | null | Layer to clear, by name. Null/omitted clears **nothing** — confirmed by filling a pane to its 16-layer limit, calling `graphics.clear` with `layer_id` null (then omitted) and getting `ok` both times while the limit stayed in effect; only clearing each of the 16 layers individually by name freed them. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `feature_disabled` (`"pane graphics are disabled by terminal.kitty_graphics"`
— the gating config key is `terminal.kitty_graphics`, under `[terminal]`, not the
schema's `experimental.kitty_graphics`), `invalid_layer_id` (`"layer_id must contain
between 1 and 64 characters"` for an empty or over-64-character name, `"layer_id contains
unsupported characters"` for one with characters like `/`), `pane_not_found`;
`feature_disabled` takes precedence over both when the feature is off; other codes
possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"1","method":"pane.graphics.clear","params":{"pane_id":"w1:p1","layer_id":"overlay-1"}}
{"id":"1","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1 (`feature_disabled`'s message was reproduced
with `terminal.kitty_graphics` explicitly set `false`; the default config leaves the
feature on).

---

## pane.graphics.info

Report a pane's graphics capabilities: cell pixel dimensions, layer limits, and file-frame
transport parameters. Requires `terminal.kitty_graphics` (enabled by default, see
[pane.graphics.clear](#panegraphicsclear)). Also requires a client actually rendering the
session — a headless server has no host cell size to report — so this method is unusable
from a server with no attached client even when the feature is on.

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

**Errors**: `feature_disabled` (`"pane graphics are disabled by terminal.kitty_graphics"`),
`pane_not_found`, `cell_size_unavailable` (`"host cell size is unavailable"`, on a
headless server / one with no rendering client attached); precedence measured as
`feature_disabled` > `pane_not_found` > `cell_size_unavailable`; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"p3","method":"pane.graphics.info","params":{"pane_id":"w1:p1"}}
{"id":"p3","result":{"type":"pane_graphics_info","cell_width_px":10,"cell_height_px":22,"pane_visible":false,"file_frame_directory":"/run/user/1000/herdr-pane-graphics-1000/server-11441-1789856182335799369/source","file_frame_formats":["rgba","bgra"],"file_frame_max_bytes":16777216,"file_frame_direct_max_bytes":419430400,"file_frame_damage":true,"max_layers_per_pane":16,"pixel_mouse":true,"file_frame_transport":"direct-kitty"}}
```

Validated 2026-09-19 against herdr 0.9.1 (success path captured read-only against the
live session's rendered pane; the `cell_size_unavailable` and `feature_disabled` error
paths were confirmed separately on headless/feature-off scratch servers).

---

## pane.graphics.set

Draw or replace an image layer in a pane. The image bytes are base64-encoded in
`data_base64`, described by `format`/`image_width`/`image_height`, and positioned via
`placement`. Requires `terminal.kitty_graphics` (enabled by default, see
[pane.graphics.clear](#panegraphicsclear)). Limited to 16 layers per pane — re-setting an
existing `layer_id` does not consume a new slot, but reaching the limit fails every
further layer until one is freed with [pane.graphics.clear](#panegraphicsclear). The
decoded image payload is capped at 512 KiB, and the request line itself at 1 MiB
(base64's 4/3 inflation makes a roughly-786 KB image unsendable even before the 512 KiB
image cap applies — see [protocol.md](../protocol.md)).

**Params** (`PaneGraphicsSetParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `format` | PaneGraphicsFormat (`png`,`rgb`,`rgba`,`bgra`) | yes | — | Pixel/encoding format of `data_base64`. |
| `image_width` | integer (uint32) | yes | — | Source image width in pixels. |
| `image_height` | integer (uint32) | yes | — | Source image height in pixels. |
| `data_base64` | string | no | `""` | Base64-encoded image payload. The `""` default is unusable — an empty payload is rejected with `invalid_image`, making this field effectively required. |
| `layer_id` | string \| null | no | null | Layer to create/replace, by name; null assigns/uses the default layer (inferred). A non-null value must be 1-64 characters from a restricted set — an empty or over-64-character name, or one containing characters like `/`, fails with `invalid_layer_id` (see [pane.graphics.clear](#panegraphicsclear)). |
| `z_index` | integer (int32) | no | 0 | Stacking order among layers. |
| `placement` | PaneGraphicsPlacementParams | no | `{grid_cols:0,grid_rows:0,viewport_col:0,viewport_row:0}` | Where and how large to place the image (see below). |

`PaneGraphicsPlacementParams`:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `grid_cols` | integer (uint32) | no | 0 | Image width in terminal cells (0 = derive from pixels, inferred). |
| `grid_rows` | integer (uint32) | no | 0 | Image height in terminal cells (0 = derive from pixels, inferred). |
| `viewport_col` | integer (int32) | no | 0 | Column offset within the pane viewport. |
| `viewport_row` | integer (int32) | no | 0 | Row offset within the pane viewport. |

**Result**: `type: "ok"` — no other fields. The schema's `ResponseResult` union also
declares a `pane_graphics_frame_ack` variant (`sequence`, `revision`), but every
successful call observed on 0.9.1 returned plain `ok`; treat `pane_graphics_frame_ack` as
unreachable from this method until contradicted by evidence.

**Errors**: `feature_disabled` (`"pane graphics are disabled by terminal.kitty_graphics"`),
`pane_not_found`, `invalid_image` (`"image data must not be empty"`, `"image_width and
image_height must be greater than zero"`, `"data_base64 is not valid base64"`, or `"image
data does not match the frame contract"` for a byte length inconsistent with the declared
dimensions — that last check applies to `rgb`/`rgba`/`bgra` only; a `png` payload's bytes
are not validated against `format` or the declared dimensions at all), `invalid_layer_id`
(above), `layer_limit` (`"pane graphics layer limit reached"`, at 16 layers per pane),
`image_too_large` (`"image data is too large"`, decoded payload over 512 KiB); other
codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"1","method":"pane.graphics.set","params":{"pane_id":"w1:p1","format":"png","image_width":64,"image_height":64,"data_base64":"iVBORw0KGgo…","placement":{"grid_cols":8,"grid_rows":4,"viewport_col":0,"viewport_row":0},"z_index":0}}
{"id":"1","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1 (the default config leaves the feature on, so
this and the layer/size limits above were live-probed; `feature_disabled`'s message was
confirmed from the installed binary's string table rather than provoked, to avoid editing
the user's shared global config).

---

## pane.input.set

Set a pane's right-click input routing: whether a right-click is handled by herdr's own
UI (context menu) or forwarded into the pane's terminal program. Nothing on the wire
reports the setting back — `PaneInfo` has no `right_click` field, a subsequent
`pane.get` is unchanged, and no event is emitted — so a caller cannot read back what it
set.

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

Validated 2026-09-19 against herdr 0.9.1 (wire behavior — params, errors, `ok` result,
and the absence of any read-back or event — confirmed; the actual right-click routing
effect needs a human at a real terminal with a mouse and was not exercised).

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

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.link.activate

Resolve, and — if a link handler is available — open, a link detected at a viewport
position in a pane. On this build, activating never actually opened anything: with a
real URL under the position, `handled` still came back `false` while `url` was
populated, and no opener process ran (`xdg-open`, `gio`, `gnome-open`, `kde-open`,
`x-www-browser`, `www-browser`, `sensible-browser`, `wslview`, `firefox`, and `chromium`
were all shimmed onto `PATH` — and `$BROWSER` pointed at the shim — with none invoked,
including with a real client attached over a pty). `url` presence, not `handled`, is
what distinguishes "a link is here" from "no link here"; whatever makes `handled` become
`true` looks link-handler/plugin dependent and was not reachable without linking a
plugin (out of scope for this probe). Use [pane.link.resolve](#panelinkresolve) first to
find link regions without going through this uncertainty.

**Params** (`PaneLinkActivateParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `viewport_row` | integer (uint16) | yes | — | Zero-based viewport row. |
| `col` | integer (uint16) | yes | — | Zero-based viewport column. |
| `content_revision` | integer (uint64) \| null | no | null | Content revision the position is relative to (inferred) — the same private copy-mode counter as [pane.copy_motion](#panecopy_motion), obtainable only from a `copy_motion`/`copy_search` result. A mismatch fails with `stale_content` (`"pane content changed before link resolution"`); null/omitted skips the check. |
| `offset_from_bottom` | integer (uint64) \| null | no | null | Scroll offset the viewport row is relative to (inferred). This is a **strict equality check against the pane's live scroll offset**, not a value that repositions the read: anything other than the pane's current offset fails with `stale_content` (`"pane viewport changed before link resolution"`); null/omitted matches the pane's current position. |

**Result**: `type: "pane_link_activated"`

| field | type | meaning |
|---|---|---|
| `handled` | boolean | Whether a link handler actually activated the link. Can be `false` even when a link **was** found at that position — see the prose above. |
| `url` | string \| null | The URL found at that position, when any; its presence (not `handled`) is what signals a link was there. Omitted, not null, when there is none. |

**Errors**: `pane_not_found`; `stale_content` (above, two message variants);
`stale_target` (`"pane is no longer visible"`) when `pane_id` is not in the currently
focused tab — this and [pane.link.resolve](#panelinkresolve) are the only pane read
methods in this namespace with that requirement; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"la1","method":"pane.link.activate","params":{"pane_id":"w1:p1","viewport_row":0,"col":0}}
{"id":"la1","result":{"type":"pane_link_activated","handled":false}}
```

Validated 2026-09-19 against herdr 0.9.1. (This example is the no-link case: `handled`
false, `url` omitted. A link-present case was also captured — `handled` still false,
`url` populated with the link's text, no opener process run — see the prose above.)

---

## pane.link.resolve

Resolve the link region(s) present at a viewport position in a pane, without activating
them. Takes the same params shape as [pane.link.activate](#panelinkactivate).

**Params** (`PaneLinkActivateParams`): same fields as
[pane.link.activate](#panelinkactivate) above.

**Result**: `type: "pane_link_resolved"`

| field | type | meaning |
|---|---|---|
| `regions` | array&lt;PaneLinkRegion&gt; | Link regions found at the position (empty if none). |

`PaneLinkRegion` (inclusive display-cell columns on the pane's current viewport):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `row` | integer (uint16) | yes | — | Viewport row of the region. |
| `start_col` | integer (uint16) | yes | — | First column of the region (inclusive). |
| `end_col` | integer (uint16) | yes | — | Last column of the region (inclusive). |

**Errors**: `pane_not_found`; the same `stale_content` and `stale_target` conditions as
[pane.link.activate](#panelinkactivate) (the two methods share request handling); an
out-of-range `viewport_row`/`col` is not an error and simply returns `regions: []`; other
codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"lr2","method":"pane.link.resolve","params":{"pane_id":"w1:p1","viewport_row":5,"col":5}}
{"id":"lr2","result":{"type":"pane_link_resolved","regions":[{"row":5,"start_col":0,"end_col":27}]}}
```

Validated 2026-09-19 against herdr 0.9.1.

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

**Errors**: `workspace_not_found` (unknown `workspace_id`, including the empty string);
other codes possible.

**CLI**: `herdr pane list [--workspace <WORKSPACE_ID>]`

**Example**

```json
{"id":"cli:pane:list","method":"pane.list","params":{"workspace_id":"w2"}}
{"id":"cli:pane:list","result":{"panes":[{"agent":"claude","agent_session":{"agent":"claude","kind":"id","source":"herdr:claude","value":"ef3b9d04-…"},"agent_status":"working","cwd":"/home/penguin/source/fledge","focused":false,"foreground_cwd":"/home/penguin/source/fledge","pane_id":"w2:p1","revision":4,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":54},"tab_id":"w2:t1","terminal_id":"term_659708952f5514","terminal_title":"◐ herdr-api-documentation","terminal_title_stripped":"herdr-api-documentation","workspace_id":"w2"}],"type":"pane_list"}}
```

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.move

Move a pane to another location: into an existing tab (`tab`), into a freshly created tab
(`new_tab`), or into a freshly created workspace (`new_workspace`). Continue with
`result.move_result.pane.pane_id` (or a live agent name), not
`result.move_result.previous_pane_id` (skill.md) — but the pane's ID only actually
*changes* when the move crosses into a different workspace; moving within the same
workspace (even into a different tab) keeps the same `pane_id`.

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
| `ratio` | number (float) \| null | no | null | Split ratio for the new placement, clamped to `[0.1, 0.9]` rather than validated (`5.0` becomes `0.9`, `-1.0` becomes `0.1`, both with `changed: true` and no warning). |

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

**Result**: `type: "pane_move"`, with the fields below nested one level down under
`move_result`:

| field | type | meaning |
|---|---|---|
| `move_result` | PaneMoveResult (below) | Move outcome. |

`PaneMoveResult`:

| field | type | meaning |
|---|---|---|
| `changed` | boolean | Whether the pane actually moved. |
| `pane` | [PaneInfo](../data-model.md) | The pane at its new location. Its `pane_id` only changes when the move crossed into a different workspace (see above); a same-workspace move keeps the same `pane_id`, equal to `previous_pane_id`. |
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
| `reason` | `same_tab` \| `zoomed_tab` \| null | Why the move was a no-op/constrained, if applicable. A destination tab equal to the pane's current tab short-circuits to `reason: "same_tab"` **before** `target_pane_id`/`ratio` are validated — an unknown `target_pane_id` or an out-of-range `ratio` that would otherwise error is silently accepted in that case. |

**Errors**: `pane_not_found`; `target_pane_not_found` (`"target pane <ID> not found"`, or
`"target pane <ID> is not in tab <TAB_ID>"` when it names a real pane outside the
destination tab); `workspace_not_found` when a `new_tab` destination names an unknown
`workspace_id`; other codes possible.

**CLI**: `herdr pane move <PANE_ID> [--tab <TAB_ID> --split <right|down> [--target-pane <ID>] [--ratio <FLOAT>] | --new-tab [--workspace <ID>] | --new-workspace] [--label <TEXT>] [--tab-label <TEXT>] [--focus | --no-focus]`

A successful move emits `pane_moved` (mirroring `move_result`: `pane`,
`previous_pane_id`/`previous_tab_id`/`previous_workspace_id`, `created_tab`,
`closed_tab_id`, `closed_workspace_id`) plus `layout_updated` for both tabs;
`new_tab`/`new_workspace` additionally emit `tab_created`/`workspace_created`/
`workspace_focused`/`tab_focused`; emptying a tab or workspace emits `tab_closed`/
`workspace_closed`; `focus: true` adds `pane_focused`. A no-op move (`reason: "same_tab"`
or `"zoomed_tab"`) emits nothing.

**Example**

```json
{"id":"cli:pane:move","method":"pane.move","params":{"pane_id":"w1:p2","destination":{"type":"new_tab","label":"moved-tab"},"focus":false}}
{"id":"cli:pane:move","result":{"type":"pane_move","move_result":{"changed":true,"previous_pane_id":"w1:p2","previous_workspace_id":"w1","previous_tab_id":"w1:t1","pane":{"pane_id":"w1:p2","terminal_id":"term_65bdd52099c732","workspace_id":"w1","tab_id":"w1:t3","focused":false,"cwd":"/tmp","foreground_cwd":"/tmp","terminal_title":"penguin@iceberg:/tmp","terminal_title_stripped":"penguin@iceberg:/tmp","agent_status":"unknown","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":0,"viewport_rows":40},"revision":1},"source_layout":{"workspace_id":"w1","tab_id":"w1:t1","zoomed":false,"area":{"x":0,"y":0,"width":120,"height":40},"focused_pane_id":"w1:p3","panes":[{"pane_id":"w1:p1","focused":false,"rect":{"x":0,"y":0,"width":120,"height":10}},{"pane_id":"w1:p3","focused":true,"rect":{"x":0,"y":10,"width":120,"height":30}}],"splits":[{"id":"split_0_root","direction":"down","ratio":0.25,"rect":{"x":0,"y":0,"width":120,"height":40}}]},"target_layout":{"workspace_id":"w1","tab_id":"w1:t3","zoomed":false,"area":{"x":0,"y":0,"width":120,"height":40},"focused_pane_id":"w1:p2","panes":[{"pane_id":"w1:p2","focused":true,"rect":{"x":0,"y":0,"width":120,"height":40}}],"splits":[]},"created_tab":{"tab_id":"w1:t3","workspace_id":"w1","number":3,"label":"moved-tab","focused":false,"pane_count":1,"agent_status":"unknown"},"focused_pane_id":"w1:p2"}}}
```

Note `pane.pane_id` ("w1:p2") equals `previous_pane_id` — the move only crossed tabs
within workspace `w1`, so the ID did not change.

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.neighbor

Resolve the pane ID that neighbors a given pane in a direction, without moving focus. A
null `neighbor_pane_id` means there is no neighbor that way.

**Params** (`PaneNeighborParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `direction` | PaneDirection (`left`,`right`,`up`,`down`) | yes | — | Direction to look. |
| `pane_id` | string \| null | no | null | Origin pane; null uses the UI-focused pane. |

**Result**: `type: "pane_neighbor"`, with the fields below nested one level down under
`neighbor`:

| field | type | meaning |
|---|---|---|
| `neighbor` | PaneNeighborResult (below) | Neighbor lookup outcome. |

`PaneNeighborResult`:

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

Validated 2026-09-19 against herdr 0.9.1. (Here the single-pane tab has no right
neighbor, so `neighbor_pane_id` is omitted/null.)

---

## pane.process_info

Report OS process information for a pane: its shell PID/TTY and the current foreground
process group and processes.

**Params** (`PaneProcessInfoParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string \| null | no | null | Pane to inspect; null uses the UI-focused pane. |

**Result**: `type: "pane_process_info"`, with the fields below nested one level down
under `process_info`:

| field | type | meaning |
|---|---|---|
| `process_info` | PaneProcessInfoResult (below) | Process data. |

`PaneProcessInfoResult`:

| field | type | meaning |
|---|---|---|
| `pane_id` | string | Pane inspected. |
| `shell_pid` | integer (uint32) \| null | PID of the pane's shell. |
| `tty` | string \| null | Controlling TTY path. Not observed on this Linux 0.9.1 build across an idle shell, a busy pipeline, a visible pane, and a hidden pane, both via the API and the CLI — never populated on this platform. |
| `foreground_process_group_id` | integer (uint32) \| null | Foreground process group ID. |
| `foreground_processes` | array&lt;PaneProcessInfoProcess&gt; | Processes in the foreground group (see below). |

`PaneProcessInfoProcess`:

| field | type | required | meaning |
|---|---|---|---|
| `pid` | integer (uint32) | yes | Process ID. |
| `name` | string | yes | Process name. |
| `argv0` | string \| null | no | `argv[0]` as executed. Also never observed populated on this platform (absent from every process entry seen; `argv`, `cmdline`, `cwd`, `name`, `pid` were always present). |
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

Validated 2026-09-19 against herdr 0.9.1 (`tty` and `argv0` are documented from the
schema only — see the notes above — since neither was ever observed populated on this
platform).

---

## pane.read

Read a snapshot of a pane's terminal output. Choose `source` per the task (see the
read-source notes at the top of this file). `lines` bounds how many rows to return, up to
an undocumented hard ceiling of 1000 regardless of the requested value (999 → 999 rows;
1000, 1001, 2000, and 5000 all → 1000 rows, each with `truncated: true`); rows are taken
from the most recent end, and `lines: 0` returns empty text with `truncated: true`.
Omitting `lines` does not mean "everything": for `recent`/`recent_unwrapped` the server
default is exactly 80 rows (`truncated: true`), while `visible`/`detection` return the
whole viewport (`truncated: false`). `strip_ansi` (default true) is documented to remove
escape sequences unless `format` is `ansi`, but had no observed effect on 0.9.1 in either
direction — `format: "text"` with `strip_ansi: false` still returned fully stripped text,
and `format: "ansi"` with `strip_ansi: true` still returned SGR sequences; `format` alone
decides. Whether CLI reads mark an agent as seen was not probed — no API surface exposes
a "seen" flag, and confirming it would need a live agent plus a UI client.

**Params** (`PaneReadParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to read. |
| `source` | ReadSource (`visible`,`recent`,`recent_unwrapped`,`detection`) | yes | — | Which snapshot to read. |
| `format` | ReadFormat (`text`,`ansi`) | no | `text` | `text` = plain, `ansi` = keep escape sequences. This alone decides stripping; see `strip_ansi` below. |
| `lines` | integer (uint32) \| null | no | null | Max rows to return from screen + scrollback, capped at 1000; null uses the server default described above (80 rows for `recent`/`recent_unwrapped`, the full viewport for `visible`/`detection`) rather than returning everything. |
| `strip_ansi` | boolean | no | true | Documented to strip ANSI escapes from the returned text; had no observed effect on 0.9.1 in either direction (see above). |

**Result**: `type: "pane_read"`, with the fields below nested one level down under
`read`:

| field | type | meaning |
|---|---|---|
| `read` | PaneReadResult (below) | The captured snapshot. |

`PaneReadResult` (also used by [pane.wait_for_output](#panewait_for_output)'s
`output_matched.read`):

| field | type | meaning |
|---|---|---|
| `pane_id` | string | Pane read. |
| `workspace_id` | string | Its workspace. |
| `tab_id` | string | Its tab. |
| `source` | ReadSource | Snapshot source actually used. |
| `format` | ReadFormat | Format of `text`. |
| `text` | string | The captured output. |
| `revision` | integer (uint64) | Documented as the pane content revision at capture time, but observed as `0` on every single read (a fresh pane, after 12 lines, after 90 lines, and after 3000 lines of output) — it tracks neither `PaneInfo.revision` nor the copy-mode `content_revision` (see [pane.copy_motion](#panecopy_motion)). Using this value as a `content_revision` elsewhere in this namespace fails every time with `stale_content`. |
| `truncated` | boolean | Whether output was cut off by `lines` or the server default. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane read <PANE_ID> [--source <visible|recent|recent-unwrapped|detection>] [--lines <N>] [--format <text|ansi> | --ansi] [--raw]`

**Example**

```json
{"id":"1","method":"pane.read","params":{"pane_id":"w1:p3","source":"recent_unwrapped","lines":120,"format":"text","strip_ansi":true}}
{"id":"1","result":{"type":"pane_read","read":{"pane_id":"w1:p3","workspace_id":"w1","tab_id":"w1:t1","source":"recent_unwrapped","format":"text","text":"echo docprobe-marker-42\n…","revision":0,"truncated":false}}}
```

Validated 2026-09-19 against herdr 0.9.1.

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
| `seq` | integer (uint64) \| null | no | null | Monotonic sequence for ordering reports from this source. A release whose `seq` is lower than the last report's `seq` from that source is silently dropped — it still returns `{"type":"ok"}`, but the pane's agent is unchanged; callers cannot distinguish "released" from "dropped as stale" by the result alone. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane release-agent <PANE_ID> --source <ID> --agent <LABEL> [--seq <N>]`

**Example**

```json
{"id":"p7","method":"pane.release_agent","params":{"pane_id":"w1:p1","source":"docprobe","agent":"claude"}}
{"id":"p7","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.rename

Set or clear a pane's user-facing label. A null/omitted `label` clears it (CLI
`--clear`); an empty string clears it the same way. There is no documented or observed
maximum length (a 500-character label round-tripped intact). Neither this method nor
[pane.input.set](#paneinputset) emits any event — no `pane_updated`/`layout_updated` —
so a client cannot learn about a label change from the event stream.

**Params** (`PaneRenameParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane to rename. |
| `label` | string \| null | no | null | New label; null **or empty string** clears the label. No enforced length limit observed. |

**Result**: `type: "pane_info"`

| field | type | meaning |
|---|---|---|
| `pane` | [PaneInfo](../data-model.md) | The updated pane. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane rename <PANE_ID> [LABEL]... [--clear]`

**Example**

```json
{"id":"cli:pane:rename","method":"pane.rename","params":{"pane_id":"w1:p3","label":"docs-pane"}}
{"id":"cli:pane:rename","result":{"pane":{"agent_status":"unknown","cwd":"/tmp/…/scratch-repo","focused":false,"foreground_cwd":"/tmp/…/scratch-repo","label":"docs-pane","pane_id":"w1:p3","revision":0,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":39},"tab_id":"w1:t1","terminal_id":"term_65970bc8a38ec4","workspace_id":"w1"},"type":"pane_info"}}
```

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.report_agent

Report a coding agent's lifecycle state for a pane, from an integration acting as
`source`. This is how an agent integration tells herdr its `idle`/`working`/`blocked`/
`unknown` state and optional session identity. Authority is **not exclusive and does not
last "until released"**: a report from any other source immediately takes over the pane
— both the agent label and status change, with no error — so the newest reporting source
holds authority, and [pane.release_agent](#panerelease_agent) only actually works for
the *current* holder (called by a superseded source it still returns `ok` but is a
silent no-op). Reporting `idle` after `working`/`blocked` does not necessarily surface as
`idle`: it becomes the observation-only `done` (see the intro's `PaneAgentState` note)
until the pane/tab is focused/seen. Emits `pane_agent_detected` and
`pane.agent_status_changed` on every report. A `source` beginning with the reserved
prefix `herdr:` (e.g. `herdr:claude`) is silently ignored — the call returns `ok` but the
pane's agent is left unset; the same reserved-prefix rule presumably applies to `source`
in [pane.release_agent](#panerelease_agent) and
[pane.clear_agent_authority](#paneclear_agent_authority) (inferred — only
`pane.report_agent` itself was probed for this).

**Params** (`PaneReportAgentParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Pane hosting the agent. |
| `source` | string | yes | — | Reporting integration/source ID. An empty string is accepted. Once a source has sent a report carrying `seq`, a later report from that same source *without* `seq` is silently dropped (treated as `seq: 0`) rather than applied — keep supplying `seq` once you start. |
| `agent` | string | yes | — | Agent label. An empty string is rejected with the undocumented `invalid_agent` (`"agent label must not be empty"`). |
| `state` | PaneAgentState (`idle`,`working`,`blocked`,`unknown`) | yes | — | Reported lifecycle state. |
| `message` | string \| null | no | null | Human-readable status message. |
| `seq` | integer (uint64) \| null | no | null | Monotonic sequence for ordering reports from this source. |
| `agent_session_id` | string \| null | no | null | Agent session identifier (ID form). Accepted but not observable through any read method afterward — `PaneInfo.agent_session` stays absent, and `agent.get`/`agent.list`/`session.snapshot` carry no session fields. |
| `agent_session_path` | string \| null | no | null | Agent session identifier (path form). Same unobservability caveat as `agent_session_id`. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; `invalid_agent` (empty `agent`); other codes possible.

**CLI**: `herdr pane report-agent <PANE_ID> --source <ID> --agent <LABEL> --state <idle|working|blocked|unknown> [--message <TEXT>] [--seq <N>] [--agent-session-id <ID>] [--agent-session-path <PATH>]`

**Example**

```json
{"id":"p4","method":"pane.report_agent","params":{"pane_id":"w1:p1","source":"docprobe","agent":"claude","state":"working"}}
{"id":"p4","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.report_agent_session

Report (or update) the agent **session identity** for a pane without changing lifecycle
state. Use it to attach a session ID/path and record where the session started. None of
the reported identity is observable afterward through any read method — `PaneInfo.
agent_session` stays absent, and `agent.get`/`agent.list`/`session.snapshot` carry no
session fields — and, unlike [pane.report_agent](#panereport_agent) (which emits
`pane_agent_detected`/`pane.agent_status_changed`), this method emits no event at all.

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

Validated 2026-09-19 against herdr 0.9.1 (the request/response envelope and the absence
of an emitted event were confirmed; the stored session identity's effect could not be
exercised, since no read method exposes `agent_session_id`/`agent_session_path`/
`session_start_source`).

---

## pane.report_metadata

Report display-only pane metadata: title, per-status labels, arbitrary tokens, and a
display-agent name, optionally with a TTL. These affect only presentation, not agent
lifecycle. Boolean `clear_*` flags remove the corresponding metadata. `tokens` keys must
match `^[A-Za-z0-9_-]{1,32}$` (max 16 keys per report — see the accumulation note below);
`state_labels` map status → label. A report **replaces** the pane's visible metadata
record rather than merging field-by-field with earlier reports: a later report carrying
only `title` makes a previously set `display_agent`/`state_labels` disappear too, unless
it repeats them. Emits a `pane.updated` event carrying the full `PaneInfo` and bumps
`PaneInfo.revision` (`pane.report_agent` does not emit `pane.updated`).

**Params** (`PaneReportMetadataParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `source` | string | yes | — | Reporting source ID. |
| `agent` | string \| null | no | null | **A display gate, not just a label**: metadata carrying `agent` is stored but withheld from `PaneInfo` until the pane's currently reported agent equals that value — and replaces whatever metadata was previously visible once it does. |
| `applies_to_source` | string \| null | no | null | Restrict metadata visibility to reports from this source (inferred) — gated the same way as `agent`: visible only while it matches the pane's current reporting source, mismatching hides it. |
| `title` | string \| null | no | null | Display title to set. |
| `clear_title` | boolean | no | false | Clear the display title. |
| `display_agent` | string \| null | no | null | Display-agent name to show. |
| `clear_display_agent` | boolean | no | false | Clear the display-agent name. |
| `state_labels` | object&lt;string,string&gt; | no | — | Map of status → custom label text. |
| `clear_state_labels` | boolean | no | false | Clear all custom state labels. |
| `tokens` | object&lt;string,string\|null&gt; | no | — | Display tokens (≤16 keys per report, key pattern `^[A-Za-z0-9_-]{1,32}$`; null value clears one key, inferred). Tokens **accumulate across reports** up to 32 total per pane (matching `PaneInfo.tokens`'s `maxProperties: 32`); exceeding that fails the whole report with `metadata_token_limit`. |
| `seq` | integer (uint64) \| null | no | null | Monotonic sequence for ordering reports from this source. |
| `ttl_ms` | integer (uint64) \| null | no | null | Expiry in ms (1..=86400000) after which metadata is dropped. |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; `invalid_metadata_request` (`"cannot set and clear the same
metadata field"` when e.g. both `title` and `clear_title` are sent, or `"missing metadata
field to set or clear"` for a report with only `pane_id`+`source`); `invalid_metadata_ttl`;
`invalid_metadata_token`; `invalid_state_label`; `metadata_token_limit` (`"pane metadata
may contain at most 32 tokens"`, above); other codes possible.

**CLI**: `herdr pane report-metadata <PANE_ID> --source <ID> [--agent <LABEL>] [--applies-to-source <ID>] [--title <TEXT> | --clear-title] [--display-agent <TEXT> | --clear-display-agent] [--state-label <STATUS=TEXT> | --clear-state-labels] [--token <NAME=VALUE> | --clear-token <NAME>] [--seq <N>] [--ttl-ms <N>]`

**Example**

```json
{"id":"1","method":"pane.report_metadata","params":{"pane_id":"w1:p1","source":"docprobe","title":"build","tokens":{"branch":"main"},"ttl_ms":60000}}
{"id":"1","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.resize

Resize the split enclosing a pane by nudging it in a direction. `amount` (a float ratio
delta) is optional; omitting it uses the server's default step (~0.05 measured). The
sign of `amount` is ignored — only its magnitude matters, with `direction` supplying the
sign, so `direction: "right", amount: -0.2` still grows the pane rightward. Split ratios
are clamped to `[0.1, 0.9]`; when the resize has no effect (including a further nudge
past either clamp bound), `changed` is false and `reason` is `unchanged`.

**Params** (`PaneResizeParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `direction` | PaneDirection (`left`,`right`,`up`,`down`) | yes | — | Direction to grow/shrink; also determines the effective sign of `amount`. |
| `pane_id` | string \| null | no | null | Pane to resize; null uses the UI-focused pane. |
| `amount` | number (float) \| null | no | null | Resize magnitude; only its absolute value is used. Null = default step. |

**Result**: `type: "pane_resize"`, with the fields below nested one level down under
`resize`:

| field | type | meaning |
|---|---|---|
| `resize` | PaneResizeResult (below) | Resize outcome. |

`PaneResizeResult`:

| field | type | meaning |
|---|---|---|
| `changed` | boolean | Whether geometry changed. |
| `pane_id` | string | Pane resized. |
| `focused_pane_id` | string | Focused pane in the resized pane's tab after the call — not necessarily the globally focused pane if that tab was not itself focused. |
| `layout` | [PaneLayoutSnapshot](../data-model.md) | Resulting tab layout. |
| `reason` | `unchanged` | Why nothing changed; **omitted**, not `null`, when `changed` is `true`. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane resize --direction <left|right|up|down> [--amount <FLOAT>] [--current | --pane <ID>]`

**Example**

```json
{"id":"cli:pane:resize","method":"pane.resize","params":{"pane_id":"w1:p3","direction":"right","amount":0.1}}
{"id":"cli:pane:resize","result":{"resize":{"changed":true,"focused_pane_id":"w1:p1","layout":{"area":{"height":39,"width":94,"x":26,"y":1},"focused_pane_id":"w1:p1","panes":[{"focused":false,"pane_id":"w1:p3","rect":{"height":39,"width":56,"x":26,"y":1}},{"focused":true,"pane_id":"w1:p1","rect":{"height":39,"width":38,"x":82,"y":1}}],"splits":[{"direction":"right","id":"split_0_root","ratio":0.6,"rect":{"height":39,"width":94,"x":26,"y":1}}],"tab_id":"w1:t1","workspace_id":"w1","zoomed":false},"pane_id":"w1:p3"},"type":"pane_resize"}}
```

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.scroll

Set a pane's scrollback offset from the bottom (0 returns to the live bottom of output).
An out-of-range offset clamps to `max_offset_from_bottom` rather than erroring. Emits
`pane.scroll_changed` with the new `PaneScrollInfo`. While a pane is scrolled back,
[pane.read](#paneread) with `source: "visible"` returns the scrolled viewport, not the
live tail.

**Params** (`PaneScrollParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `offset_from_bottom` | integer (uint64) | yes | — | Rows to scroll back from the bottom; 0 returns to the live bottom. |

**Result**: `type: "pane_info"`

| field | type | meaning |
|---|---|---|
| `pane` | [PaneInfo](../data-model.md) | The pane after scrolling (its `scroll` field reflects the resulting offset). |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"s1","method":"pane.scroll","params":{"pane_id":"w1:p1","offset_from_bottom":1}}
{"id":"s1","result":{"type":"pane_info","pane":{"pane_id":"w1:p1","terminal_id":"term_65bb4da209a581","workspace_id":"w1","tab_id":"w1:t1","focused":true,"cwd":"/home/penguin","foreground_cwd":"/home/penguin","terminal_title":"penguin@iceberg:~","terminal_title_stripped":"penguin@iceberg:~","agent_status":"unknown","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":0,"viewport_rows":40},"revision":1}}}
```

Validated 2026-09-19 against herdr 0.9.1. (Requested offset 1 was clamped to 0 because
the pane's scrollback did not extend beyond the viewport; on a pane with more
scrollback, an offset beyond it clamps to `max_offset_from_bottom` instead.)

---

## pane.selection.read

Read the plain text spanned by a selection range in a pane, given an anchor and cursor
point (the two ends of a copy-mode selection). Both endpoints are inclusive (`col` 0..10
spans 11 characters), and `anchor`/`cursor` order does not matter — reversing them
returns the same text. Remember `row` is an absolute scrollback row, not a viewport row
(see [pane.copy_motion](#panecopy_motion)'s `PaneTextPoint`).

**Params** (`PaneSelectionReadParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `anchor` | PaneTextPoint ([above](#panecopy_motion)) | yes | — | One end of the selection. |
| `cursor` | PaneTextPoint ([above](#panecopy_motion)) | yes | — | Other end of the selection. |
| `content_revision` | integer (uint64) \| null | no | null | Content revision the points are relative to (inferred) — the same private copy-mode counter as [pane.copy_motion](#panecopy_motion)/[pane.copy_search](#panecopy_search), the only documented source for a valid value (neither `PaneInfo.revision` nor `pane.read`'s `revision` works). A mismatch fails with `stale_content` (`"pane content changed"`); null/omitted skips the check. |

**Result**: `type: "pane_selection"`

| field | type | meaning |
|---|---|---|
| `pane_id` | string | Pane read. |
| `text` | string | Text spanned by the selection. |

**Errors**: `pane_not_found`; `stale_content` (`content_revision` mismatch, above);
`selection_unavailable` (`"selection text is unavailable"`) for an out-of-range point;
other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**

```json
{"id":"sel1","method":"pane.selection.read","params":{"pane_id":"w1:p1","anchor":{"row":0,"col":0},"cursor":{"row":0,"col":10}}}
{"id":"sel1","result":{"type":"pane_selection","pane_id":"w1:p1","text":"[penguin@ic"}}
```

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.send_input

Send text and/or a sequence of logical keys to a pane in one call — the general-purpose
input primitive underlying `send_text` and `send_keys`. At least a `pane_id` is required;
`text` and `keys` are both optional and, when both present, are delivered together.
`text` is delivered as a paste, so a trailing `\r`/`\n` inside it does **not** submit the
line — it lands in the shell's line editor as a literal newline; only an explicit
`enter` key (via `keys`) submits. This is exactly what `herdr pane run` does.

**Params** (`PaneSendInputParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string | yes | — | Target pane. |
| `text` | string | no | — | Literal text to type. |
| `keys` | array&lt;string&gt; | no | — | Logical key names to press (e.g. `enter`, `esc`, `ctrl+c`). |

**Result**: `type: "ok"` — no other fields.

**Errors**: `pane_not_found`; `invalid_key` (`"unsupported key <name>"`) if a key name is
unknown — herdr validates all keys before writing any bytes; an empty `keys` array is
accepted and returns `ok` rather than erroring; other codes possible.

**CLI**: API-only (no CLI subcommand); the CLI splits this into `send-text`, `send-keys`,
and `run`.

**Example**

```json
{"id":"p1","method":"pane.send_input","params":{"pane_id":"w1:p1","text":"true","keys":["enter"]}}
{"id":"p1","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1.

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

**Errors**: `pane_not_found`; `invalid_key` (`"unsupported key <name>"`) on an unknown key
name; other codes possible. An empty `keys` array is accepted and returns `ok`.

**CLI**: `herdr pane send-keys <PANE_ID> <KEY>...`

**Example**

```json
{"id":"1","method":"pane.send_keys","params":{"pane_id":"w1:p1","keys":["ctrl+c"]}}
{"id":"1","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1.

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

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.split

Split a pane, creating a new sibling pane running a fresh shell (or, with `cwd`/`env`, a
shell in a chosen directory/environment). Returns the new pane as `result.pane`; read its
ID from `result.pane.pane_id`. Keep the caller's focus with `focus: false` unless the user
wants focus moved. `direction` is required and restricted to `right`/`down`. A
nonexistent `cwd` is silently ignored — the new pane starts in the home directory with no
error or warning. Emits `pane_created` then `layout_updated`, plus `pane_focused` in
between when `focus: true`.

**Params** (`PaneSplitParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `direction` | SplitDirection (`right`,`down`) | yes | — | Split the target to the right or downward. |
| `target_pane_id` | string \| null | no | null | Pane to split; null uses the UI-focused pane (inferred). |
| `workspace_id` | string \| null | no | null | Workspace context for the split (inferred). |
| `ratio` | number (float) \| null | no | null | Initial split ratio, clamped to `[0.1, 0.9]` rather than validated (`5.0` becomes `0.9`, `-1.0` becomes `0.1`, no error); null = default. |
| `cwd` | string \| null | no | null | Working directory for the new pane's shell. A nonexistent path is silently ignored; the shell starts in the home directory instead. |
| `env` | object&lt;string,string&gt; | no | — | Extra environment variables for the launched process. |
| `right_click` | PaneRightClickTarget (`herdr`,`pane`) | no | `herdr` | Right-click routing for the new pane. Accepted by the server and CLI, but unverifiable — no read method exposes a pane's right-click routing (see [pane.input.set](#paneinputset)). |
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

Validated 2026-09-19 against herdr 0.9.1 (`right_click`'s effect could not be exercised —
no read-back path exists for it).

---

## pane.swap

Swap two panes' positions in the layout. Targets may be given explicitly
(`source_pane_id`/`target_pane_id`) or a `direction` from `pane_id` selects the neighbor
to swap with. Although every field is individually optional in the schema, the server
requires **exactly one** of the two forms: the empty object, a lone `pane_id`, and
supplying both forms at once all fail with `invalid_pane_swap` (`"provide either
direction with optional pane_id, or source_pane_id and target_pane_id"`). When the swap
cannot happen, `changed` is false and `reason` explains why.

**Params** (`PaneSwapParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `pane_id` | string \| null | no | null | Reference pane (used with `direction`); null uses the UI-focused pane. |
| `direction` | PaneDirection (`left`,`right`,`up`,`down`) \| null | no | null | Swap with the neighbor in this direction. |
| `source_pane_id` | string \| null | no | null | Explicit first pane to swap. |
| `target_pane_id` | string \| null | no | null | Explicit second pane to swap. |

**Result**: `type: "pane_swap"`, with the fields below nested one level down under
`swap`:

| field | type | meaning |
|---|---|---|
| `swap` | PaneSwapResult (below) | Swap outcome. |

`PaneSwapResult`:

| field | type | meaning |
|---|---|---|
| `changed` | boolean | Whether panes were swapped. |
| `source_pane_id` | string | First pane involved. |
| `target_pane_id` | string \| null | Second pane involved; **omitted**, not `null`, when none resolved. |
| `focused_pane_id` | string | Focused pane in the swap's tab after the call — not necessarily the globally focused pane. |
| `layout` | [PaneLayoutSnapshot](../data-model.md) | Resulting tab layout. |
| `reason` | `no_neighbor` \| `same_pane` \| `not_found` \| `cross_tab` | Why the swap did not happen; **omitted**, not `null`, when `changed` is `true`. |

**Errors**: `invalid_pane_swap` (above); `pane_not_found`, reachable only through the
`direction` form (`"source pane not found"`) — in the explicit `source_pane_id`/
`target_pane_id` form an unknown pane is **not** an error, it returns `changed: false,
reason: "not_found"`; other codes possible.

**CLI**: `herdr pane swap [--direction <left|right|up|down>] [--current | --pane <ID>] [--source-pane <ID>] [--target-pane <ID>]`

**Example**

```json
{"id":"cli:pane:swap","method":"pane.swap","params":{"source_pane_id":"w1:p1","target_pane_id":"w1:p3"}}
{"id":"cli:pane:swap","result":{"swap":{"changed":true,"focused_pane_id":"w1:p1","layout":{"area":{"height":39,"width":94,"x":26,"y":1},"focused_pane_id":"w1:p1","panes":[{"focused":false,"pane_id":"w1:p3","rect":{"height":39,"width":47,"x":26,"y":1}},{"focused":true,"pane_id":"w1:p1","rect":{"height":39,"width":47,"x":73,"y":1}}],"splits":[{"direction":"right","id":"split_0_root","ratio":0.5,"rect":{"height":39,"width":94,"x":26,"y":1}}],"tab_id":"w1:t1","workspace_id":"w1","zoomed":false},"source_pane_id":"w1:p1","target_pane_id":"w1:p3"},"type":"pane_swap"}}
```

Validated 2026-09-19 against herdr 0.9.1.

---

## pane.wait_for_output

Block until a pane's output matches a pattern. The selected snapshot is searched
immediately (existing output can match), then polled until match or `timeout_ms`.
Omitting `timeout_ms` waits indefinitely. Returns the matched line plus a `PaneReadResult`
snapshot. This is a long-running request; the connection stays open until it resolves.
`timeout_ms: 0` means "do not wait": it fails with the timeout error almost immediately
if the snapshot does not already match, rather than waiting forever. `lines: 0`
similarly searches an empty snapshot and never matches.

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
| `revision` | integer (uint64) | Documented as the pane content revision at match time, but observed constant `0` in every probe (before/after new output, after `report_agent`, after `report_metadata`) — it tracks neither `PaneInfo.revision` nor the copy-mode `content_revision`, so its documented meaning could not be confirmed on 0.9.1. |
| `matched_line` | string \| null | The line that matched (null if not line-scoped, inferred). Every match observed across 12 probes — substring, regex, the `detection` source, and `strip_ansi: false` — returned a non-null value; no input was found that produces `null`. |
| `read` | [PaneReadResult](../data-model.md) | Snapshot at match time (`pane_id`, `workspace_id`, `tab_id`, `source`, `format`, `text`, `revision`, `truncated`); `read.revision` is likewise constant `0`. |

**Errors**: `pane_not_found`; a timeout error when `timeout_ms` elapses without a match
(inferred); `invalid_regex` for an unparseable `regex` pattern, with the full Rust
regex-parser diagnostic passed through (e.g. `"regex parse error:\n    ([unclosed\n
^\nerror: unclosed character class"`); other codes possible.

**CLI**: `herdr pane wait-output <PANE_ID> <--match <TEXT> | --regex <PATTERN>> [--source <visible|recent|recent-unwrapped>] [--lines <N>] [--timeout <MS>] [--raw]`

**Example**

```json
{"id":"cli:pane:wait-output","method":"pane.wait_for_output","params":{"pane_id":"w1:p3","source":"recent_unwrapped","match":{"type":"substring","value":"docprobe-marker-42"}}}
{"id":"cli:pane:wait-output","result":{"matched_line":"echo docprobe-marker-42","pane_id":"w1:p3","read":{"format":"text","pane_id":"w1:p3","revision":0,"source":"recent_unwrapped","tab_id":"w1:t1","text":"echo docprobe-marker-42","truncated":false,"workspace_id":"w1"},"revision":0,"type":"output_matched"}}
```

Validated 2026-09-19 against herdr 0.9.1 (`matched_line`'s null case and `revision`'s
documented meaning could not be confirmed — see above).

---

## pane.zoom

Toggle, enable, or disable **zoom** — temporarily maximizing one pane to fill its tab.
`mode` defaults to `toggle`. Zooming also **focuses** the pane: `mode: "on"` on an
unfocused pane reports `focus_changed: true` and moves focus, and on a single-pane tab
`mode: "on"` returns `changed: true` / `zoom_changed: false` purely because focus moved.
When the tab has a single pane or is already in the requested state, `zoom_changed` is
false and `reason` explains why.

**Params** (`PaneZoomParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `mode` | PaneZoomMode (`toggle`,`on`,`off`) | no | `toggle` | Whether to toggle, force on, or force off. |
| `pane_id` | string \| null | no | null | Pane to zoom; null uses the UI-focused pane. |

**Result**: `type: "pane_zoom"`, with the fields below nested one level down under
`zoom`:

| field | type | meaning |
|---|---|---|
| `zoom` | PaneZoomResult (below) | Zoom outcome. |

`PaneZoomResult`:

| field | type | meaning |
|---|---|---|
| `changed` | boolean | Whether anything changed (zoom or focus) — see the focus note above. |
| `zoom_changed` | boolean | Whether the zoom state changed. |
| `focus_changed` | boolean | Whether focus changed as a result. |
| `pane_id` | string | Pane targeted. |
| `focused_pane_id` | string | Focused pane in the zoomed pane's tab after the call — not necessarily the globally focused pane. |
| `zoomed` | boolean | Zoom state after the call. |
| `layout` | [PaneLayoutSnapshot](../data-model.md) | Resulting tab layout (its `zoomed` reflects the new state). |
| `reason` | `single_pane` \| `already_zoomed` \| `already_unzoomed` | Why zoom did not change; **omitted**, not `null`, when it does not apply. |

**Errors**: `pane_not_found`; other codes possible.

**CLI**: `herdr pane zoom [PANE_ID] [--current | --pane <ID>] [--toggle | --on | --off]`

**Example**

```json
{"id":"cli:pane:zoom","method":"pane.zoom","params":{"pane_id":"w1:p3","mode":"on"}}
{"id":"cli:pane:zoom","result":{"type":"pane_zoom","zoom":{"changed":true,"focus_changed":true,"focused_pane_id":"w1:p3","layout":{"area":{"height":39,"width":94,"x":26,"y":1},"focused_pane_id":"w1:p3","panes":[{"focused":false,"pane_id":"w1:p1","rect":{"height":39,"width":47,"x":26,"y":1}},{"focused":true,"pane_id":"w1:p3","rect":{"height":39,"width":47,"x":73,"y":1}}],"splits":[{"direction":"right","id":"split_0_root","ratio":0.5,"rect":{"height":39,"width":94,"x":26,"y":1}}],"tab_id":"w1:t1","workspace_id":"w1","zoomed":true},"pane_id":"w1:p3","zoom_changed":true,"zoomed":true}}}
```

Validated 2026-09-19 against herdr 0.9.1.
