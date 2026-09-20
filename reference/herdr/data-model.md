# herdr data model

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](README.md). Wire format: [protocol.md](protocol.md).

This file catalogs the domain entities that herdr's socket API embeds inside method
results and pushed events. Every entity here comes from the `$defs` of the
`success_response` schema in [raw/schema.json](raw/schema.json); the per-method result
wrappers in `api/*.md` link back to these definitions instead of re-expanding them.
Field tables use the schema's exact JSON names. `T | null` marks a schema
`type: ["T","null"]` (optional/nullable); in practice herdr 0.9.1 omits such a field from
the response entirely when its value is absent, rather than sending an explicit `null`, so
callers should treat "key missing" and "value null" as equivalent. Unless a row says
"(inferred)", the meaning is either stated by the schema or unambiguous from the field name
and corroborating probe captures under `scratchpad/probes/`. All probe paths below are
relative to `scratchpad/probes/`.

Two ID conventions appear throughout and are opaque stable handles: workspace `w1`, tab
`w1:t1`, pane `w1:p1`. Closed tab/pane IDs are never reused; the counter that mints new
tab/pane numbers continues past `9` into single hex-style letters (`w1:p9` → `w1:pA` →
`w1:pB`) rather than wrapping to two digits, so treat the whole ID as opaque rather than
parsing its trailing number. A pane moved to another workspace gets a new
workspace-qualified pane ID. A workspace's `number` (the display ordinal) is not subject to
this never-reused rule, but it is not lazily recycled onto the next create either: closing
any workspace immediately renumbers every other still-open workspace so numbers stay
contiguous from 1, whether or not those workspaces were otherwise touched, and a
subsequently created workspace is appended as (open workspace count + 1) rather than
backfilled into the vacated ordinal. Callers should treat a held `number` as unstable across
any workspace close elsewhere in the session, unlike `workspace_id`, which is never reused.
Validated 2026-09-19 against herdr 0.9.1 (created three workspaces A/B/C numbered 1/2/3;
closing A alone, with no other call in between, immediately renumbered B to 1 and C to 2;
a workspace D created afterward got number 3, not the vacated 1).

## Contents

| entity | role |
| --- | --- |
| [WorkspaceInfo](#workspaceinfo) | a workspace (top-level container) |
| [TabInfo](#tabinfo) | a tab within a workspace |
| [PaneInfo](#paneinfo) | a pane (terminal cell), possibly hosting an agent |
| [AgentInfo](#agentinfo) | a recognized coding agent occupying a pane |
| [AgentStatus](#agentstatus) | agent lifecycle enum + semantics |
| [AgentSessionInfo](#agentsessioninfo) / [AgentSessionRefKind](#agentsessionrefkind) | native session handle for an agent |
| [SessionSnapshot](#sessionsnapshot) | full-session dump |
| [LayoutDescription](#layoutdescription) | portable, exportable layout tree |
| [LayoutNode](#layoutnode) | recursive layout tree node (pane \| split) |
| [SplitDirection](#splitdirection) / [PaneDirection](#panedirection) | direction enums |
| [PaneLayoutSnapshot](#panelayoutsnapshot) | rendered geometry snapshot |
| [PaneLayoutPane](#panelayoutpane) / [PaneLayoutSplit](#panelayoutsplit) / [PaneLayoutRect](#panelayoutrect) | geometry parts |
| [PaneScrollInfo](#panescrollinfo) | scrollback viewport position |
| [PaneProcessInfo](#paneprocessinfo) / [PaneProcessInfoProcess](#paneprocessinfoprocess) | process introspection |
| [PaneReadResult](#panereadresult) / [ReadSource](#readsource) / [ReadFormat](#readformat) | terminal read output + selectors |
| [PaneTextPoint](#panetextpoint) / [PaneTextRange](#panetextrange) | row/col coordinate and span, used by copy-motion and copy-search results |
| [PaneLinkRegion](#panelinkregion) | resolved link hit region on a pane's viewport |
| [Pane operation result shapes](#pane-operation-result-shapes) | swap/move/zoom/resize/edges/neighbor/focus results |
| [WorktreeInfo](#worktreeinfo) / [WorkspaceWorktreeInfo](#workspaceworktreeinfo) / [WorktreeSourceInfo](#worktreesourceinfo) | git worktree entities |
| [ServerCapabilities](#servercapabilities) | server feature flags (in `pong`) |
| [AgentManifestInfo](#agentmanifestinfo) | installed agent-detection manifest |
| [IntegrationInfo](#integrationinfo) / [IntegrationState](#integrationstate) | an agent integration's install status |
| [IntegrationTarget](#integrationtarget) | agent-integration install targets |
| [ConfigReloadStatus](#configreloadstatus) | config-reload outcome enum |
| [PopupSize](#popupsize) | popup dimension value |
| [Plugin entities](#plugin-entities) | installed plugins, actions, panes, logs, sources |
| [Event entities](#event-entities) | EventKind / EventEnvelope / EventData |
| [Reason & auxiliary enums](#reason--auxiliary-enums) | operation-outcome reason enums |
| [Appendix: on-disk persistence (session.json, version 3)](#appendix-on-disk-persistence-sessionjson-version-3) | persisted session file |

---

## WorkspaceInfo

A workspace: the top-level container that holds tabs (each tab holds a pane tree). Emitted
by `workspace.get`/`workspace.list`/`workspace.create`, embedded in `SessionSnapshot`, and
carried by `workspace_*` events. Corroborated by `workspace-get.json`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `workspace_id` | string | yes | opaque workspace handle, e.g. `w2` |
| `number` | integer (uint) | yes | 1-based display ordinal in the sidebar |
| `label` | string | yes | display name (custom name, else derived) |
| `focused` | boolean | yes | this workspace holds the UI focus |
| `pane_count` | integer (uint) | yes | total panes across all its tabs |
| `tab_count` | integer (uint) | yes | number of tabs |
| `active_tab_id` | string | yes | tab ID currently active in this workspace |
| `agent_status` | [AgentStatus](#agentstatus) | yes | rolled-up worst/most-significant agent state across the workspace |
| `tokens` | object&lt;string,string&gt; | no | caller-set metadata key→value map; keys match `^[A-Za-z0-9_-]{1,32}$`, ≤32 entries |
| `worktree` | [WorkspaceWorktreeInfo](#workspaceworktreeinfo) \| null | no | git worktree this workspace is checked out to, if any |

`tokens` is absent until the caller sets metadata, then appears as an object. Validated
2026-09-19 against herdr 0.9.1 (`worktree` was not opened from this section's own probing
chunk, but is confirmed populated — with exactly the `WorkspaceWorktreeInfo` shape below —
on a worktree-backed workspace in the companion worktree-probing chunk).

## TabInfo

A tab inside a workspace; a tab owns one pane tree. Emitted by `tab.get`/`tab.list`/
`tab.create` and carried by `tab_*` events. Corroborated by `tab-get.json`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `tab_id` | string | yes | opaque tab handle, e.g. `w2:t1` |
| `workspace_id` | string | yes | owning workspace |
| `number` | integer (uint) | yes | 1-based ordinal within the workspace |
| `label` | string | yes | display name (custom, else the number) |
| `focused` | boolean | yes | this tab is the workspace's active/focused tab |
| `pane_count` | integer (uint) | yes | panes in this tab |
| `agent_status` | [AgentStatus](#agentstatus) | yes | rolled-up agent state across the tab's panes |

Validated 2026-09-19 against herdr 0.9.1.

## PaneInfo

A pane: one terminal cell in a tab's layout. A pane exists with or without an agent; when
an agent is recognized, the agent fields are populated. Emitted by `pane.get`/`pane.list`/
`pane.current` and by `pane_created`/`pane_updated` events. Corroborated by
`pane-current.json`, `pane-get.json`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `pane_id` | string | yes | opaque pane handle, e.g. `w2:p1` |
| `terminal_id` | string | yes | stable underlying terminal ID, e.g. `term_659708952f5514` |
| `workspace_id` | string | yes | owning workspace |
| `tab_id` | string | yes | owning tab |
| `focused` | boolean | yes | this pane holds focus within its tab |
| `agent_status` | [AgentStatus](#agentstatus) | yes | status of the agent in this pane (`unknown` when none/unclassified) |
| `revision` | integer (uint64) | yes | monotonic output revision; bumps when pane output changes |
| `agent` | string \| null | no | canonical agent kind detected (e.g. `claude`), or null |
| `display_agent` | string \| null | no | human-facing agent label (inferred: presentation form of `agent`) |
| `agent_session` | [AgentSessionInfo](#agentsessioninfo) \| null | no | native session handle for the agent, if known |
| `name` | — | — | not present on PaneInfo (see [AgentInfo](#agentinfo)) |
| `label` | string \| null | no | caller-set pane label/title override |
| `title` | string \| null | no | resolved pane title |
| `terminal_title` | string \| null | no | raw terminal title (may include status glyphs, e.g. `◐ …`) |
| `terminal_title_stripped` | string \| null | no | `terminal_title` with status decoration removed |
| `cwd` | string \| null | no | pane shell working directory |
| `foreground_cwd` | string \| null | no | working directory of the current foreground process |
| `scroll` | [PaneScrollInfo](#panescrollinfo) \| null | no | scrollback viewport position |
| `state_labels` | object&lt;string,string&gt; | no | detector-produced state annotations (label→value) |
| `tokens` | object&lt;string,string&gt; | no | caller metadata map; keys `^[A-Za-z0-9_-]{1,32}$`, ≤32 entries |

The 32-entry cap above is on the `tokens` map itself, not on a single write: setting a
pane's tokens enforces a separate, lower per-call cap of 16 tokens — updating 17+ tokens in
one `pane.report_metadata` call fails with `invalid_metadata_token: "a metadata report may
update at most 16 tokens"` even though the resulting map is still under the 32-entry limit.
Reach the 17-32 range with more than one call. Validated 2026-09-19 against herdr 0.9.1.

## AgentInfo

The recognized coding agent currently occupying a pane. Superset of the agent-relevant
`PaneInfo` fields plus lifecycle bookkeeping. Emitted by `agent.get`/`agent.list`/
`agent.start`/`agent.prompt` and embedded in `SessionSnapshot.agents`. Corroborated by
`agent-get.json`, `agent-list.json`. Agent targets are a unique live agent `name` or the
hosting `pane_id`; names match `[a-z][a-z0-9_-]{0,31}` and follow the pane's occupant.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `terminal_id` | string | yes | underlying terminal ID |
| `agent_status` | [AgentStatus](#agentstatus) | yes | current lifecycle state |
| `workspace_id` | string | yes | owning workspace |
| `tab_id` | string | yes | owning tab |
| `pane_id` | string | yes | hosting pane |
| `focused` | boolean | yes | pane holds focus |
| `revision` | integer (uint64) | yes | output revision |
| `agent` | string \| null | no | canonical agent kind (e.g. `claude`) |
| `display_agent` | string \| null | no | human-facing agent label |
| `agent_session` | [AgentSessionInfo](#agentsessioninfo) \| null | no | native session handle |
| `name` | string \| null | no | live agent name assigned by the caller (target handle) |
| `cwd` | string \| null | no | shell working directory |
| `foreground_cwd` | string \| null | no | foreground-process working directory |
| `interactive_ready` | boolean | no | agent has reached interactive-input readiness |
| `launch_pending` | boolean | no | a launch is in progress and not yet detected |
| `screen_detection_skipped` | boolean | no | screen-based detection bypassed (e.g. integration supplies state) |
| `state_change_seq` | integer (uint64) | no (default 0) | monotonic counter bumped on each lifecycle change; use to detect state transitions |
| `state_labels` | object&lt;string,string&gt; | no | detector state annotations |
| `title` | string \| null | no | resolved title |
| `terminal_title` | string \| null | no | raw terminal title |
| `terminal_title_stripped` | string \| null | no | title with decoration removed |
| `tokens` | object&lt;string,string&gt; | no | caller metadata map (same constraints as PaneInfo) |

Note: `PaneInfo` carries `scroll` and `label`; `AgentInfo` instead carries `name`,
`interactive_ready`, `launch_pending`, `screen_detection_skipped`, and `state_change_seq`.

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: this reverification
pass kept `agent.start` calls to a minimum, so no agent occupied a pane in this chunk and
`AgentInfo`'s lifecycle fields went unexercised here; see the agent-lifecycle-focused
reverification for live `agent.start`/`agent.prompt`/`agent.wait` behavior).

## AgentStatus

Enum. The lifecycle state herdr assigns to an agent in a pane.

| value | meaning |
| --- | --- |
| `idle` | Agent is ready for input **and** its tab has been seen in the focused Herdr UI. |
| `working` | Agent is actively processing a turn. |
| `blocked` | Herdr recognized an approval or question UI; the agent is waiting on the human. |
| `done` | The same underlying idle state as `idle`, reported after **unseen** background work finished. |
| `unknown` | An agent is present but Herdr cannot classify it confidently; it does **not** prove completion. When no agent occupies a pane, status reads `unknown`. |

Lifecycle semantics (from [raw/skill.md](raw/skill.md)):

- The only difference between `idle` and `done` is whether the tab has been *seen*.
  Focusing the tab, or targeting the pane/agent with a focus command, marks it seen and
  collapses `done` → `idle`. **CLI reads do not mark a tab seen**, so reading an agent's
  output does not clear `done`.
- `blocked` means an approval/question dialog was detected — inspect it (`agent get`,
  `agent read`) and ask the human before answering.
- `unknown` is not a completion signal.
- `agent.explain` reports which detection rule produced the current state and the evidence
  string (probe `agent-explain.json`: `state: working`, `rule: osc_title_working`,
  `evidence: "◐ herdr-api-documentation"`).
- Roll-ups: `TabInfo.agent_status` and `WorkspaceInfo.agent_status` aggregate the most
  significant state across their panes.

Validated 2026-09-19 against herdr 0.9.1 (only the `unknown` value, on panes with no
recognized agent, was exercised live in this chunk — confirmed on every plain-shell pane
probed; `idle`/`working`/`blocked`/`done` and the seen/unseen distinction are drawn from
`raw/skill.md` and were not independently reproduced against a live agent here).

## AgentSessionInfo

The native session handle for a recognized agent — how herdr correlates a pane's agent to
that agent's own on-disk/native session for restore and integration. Corroborated by
`pane-current.json` (`{"agent":"claude","kind":"id","source":"herdr:claude","value":"ef3b9d04-…"}`).

| field | type | required | meaning |
| --- | --- | --- | --- |
| `source` | string | yes | provider/namespace of the reference, e.g. `herdr:claude` |
| `agent` | string | yes | agent kind, e.g. `claude` |
| `kind` | [AgentSessionRefKind](#agentsessionrefkind) | yes | whether `value` is a session id or a filesystem path |
| `value` | string | yes | the session identifier or path itself |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: no agent was attached
to a pane in this reverification chunk, so `agent_session` was never populated; the shape
above was captured 2026-09-17, see `pane-current.json`).

## AgentSessionRefKind

Enum, used by `AgentSessionInfo.kind`.

| value | meaning |
| --- | --- |
| `id` | `value` is a native session identifier |
| `path` | `value` is a filesystem path to the session |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: tied to
`AgentSessionInfo`, above, which was not exercised in this chunk).

## SessionSnapshot

A full dump of the live session: every workspace, tab, pane, layout, and agent plus the
current focus. Returned by the `session.snapshot` method (`result.type = session_snapshot`).
Corroborated by `snapshot.json` (2 workspaces / 2 tabs / 2 panes / 2 layouts / 2 agents).

| field | type | required | meaning |
| --- | --- | --- | --- |
| `version` | string | yes | herdr version that produced the snapshot, e.g. `0.9.1` |
| `protocol` | integer (uint32) | yes | protocol number, e.g. `22` |
| `workspaces` | array&lt;[WorkspaceInfo](#workspaceinfo)&gt; | yes | all workspaces |
| `tabs` | array&lt;[TabInfo](#tabinfo)&gt; | yes | all tabs across all workspaces |
| `panes` | array&lt;[PaneInfo](#paneinfo)&gt; | yes | all panes |
| `layouts` | array&lt;[PaneLayoutSnapshot](#panelayoutsnapshot)&gt; | yes | one rendered layout per tab |
| `agents` | array&lt;[AgentInfo](#agentinfo)&gt; | yes | all recognized agents |
| `focused_workspace_id` | string \| null | no | currently focused workspace |
| `focused_tab_id` | string \| null | no | currently focused tab |
| `focused_pane_id` | string \| null | no | currently focused pane |

`workspaces`/`tabs`/`panes`/`layouts`/`agents` are present as `[]` (not omitted) when
empty; the three `focused_*` fields are omitted entirely until something is focused, then
all three appear together. Validated 2026-09-19 against herdr 0.9.1.

## LayoutDescription

A portable description of a tab's layout as a recursive split tree — the shape used by
`layout.export`/`layout.apply`/`layout.set_split_ratio` (`result.type` of `layout_export`,
`layout_apply`, `layout_split_ratio_set`). Unlike [PaneLayoutSnapshot](#panelayoutsnapshot)
(pixel/cell geometry), this is the logical tree with ratios and pane specs and is meant to
be re-applied. Corroborated by `raw/layout-export.json`, `raw/layout-apply.json`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `workspace_id` | string | yes | owning workspace |
| `tab_id` | string | yes | described tab |
| `zoomed` | boolean | yes | a pane is currently zoomed |
| `focused_pane_id` | string | yes | focused pane in the tree |
| `root` | [LayoutNode](#layoutnode) | yes | root of the split tree |

Validated 2026-09-19 against herdr 0.9.1.

## LayoutNode

Recursive layout-tree node — a tagged union (`type` discriminator) with two variants: a
leaf `pane` or an interior `split`.

Variant `pane` (a leaf terminal):

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"pane"` | yes | discriminator |
| `pane_id` | string \| null | no | existing pane to bind here (null when describing a to-be-created pane) |
| `label` | string \| null | no | pane label |
| `cwd` | string \| null | no | working directory to launch in |
| `command` | array&lt;string&gt; \| null | no | command + argv to run in the pane |
| `env` | object&lt;string,string&gt; | no | environment overrides |

Variant `split` (an interior division):

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"split"` | yes | discriminator |
| `direction` | [SplitDirection](#splitdirection) | yes | `right` (side-by-side) or `down` (stacked) |
| `ratio` | number (float) | yes | fraction of space given to `first` (0..1) |
| `first` | [LayoutNode](#layoutnode) | yes | first child subtree |
| `second` | [LayoutNode](#layoutnode) | yes | second child subtree |

Validated 2026-09-19 against herdr 0.9.1 (both variants round-tripped through
`layout.export`/`layout.apply`, including a nested split).

## SplitDirection

Enum. Direction a split divides space, used by `LayoutNode`, `PaneLayoutSplit`, and pane
splitting.

| value | meaning |
| --- | --- |
| `right` | children placed side by side; `first` on the left |
| `down` | children stacked; `first` on top |

Validated 2026-09-19 against herdr 0.9.1.

## PaneDirection

Enum. A four-way spatial direction, used by neighbor/edge/focus queries.

| value | meaning |
| --- | --- |
| `left` | toward the left neighbor |
| `right` | toward the right neighbor |
| `up` | toward the pane above |
| `down` | toward the pane below |

Validated 2026-09-19 against herdr 0.9.1.

## PaneLayoutSnapshot

The rendered geometry of a tab: absolute cell rectangles for every pane and split. Returned
by `pane.layout` and embedded in most pane-operation results and the `layout_updated` event.
Corroborated by `pane-layout.json`, `pane-edges.json`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `workspace_id` | string | yes | owning workspace |
| `tab_id` | string | yes | described tab |
| `zoomed` | boolean | yes | a pane is zoomed |
| `area` | [PaneLayoutRect](#panelayoutrect) | yes | the tab's total drawable area (in cells) |
| `focused_pane_id` | string | yes | focused pane |
| `panes` | array&lt;[PaneLayoutPane](#panelayoutpane)&gt; | yes | placed panes with rectangles |
| `splits` | array&lt;[PaneLayoutSplit](#panelayoutsplit)&gt; | yes | split dividers with rectangles |

Validated 2026-09-19 against herdr 0.9.1.

## PaneLayoutPane

A pane's placement within a [PaneLayoutSnapshot](#panelayoutsnapshot).

| field | type | required | meaning |
| --- | --- | --- | --- |
| `pane_id` | string | yes | the pane |
| `focused` | boolean | yes | this pane is focused |
| `rect` | [PaneLayoutRect](#panelayoutrect) | yes | its cell rectangle |

Validated 2026-09-19 against herdr 0.9.1.

## PaneLayoutSplit

A split divider's placement within a [PaneLayoutSnapshot](#panelayoutsnapshot).

| field | type | required | meaning |
| --- | --- | --- | --- |
| `id` | string | yes | split identifier |
| `direction` | [SplitDirection](#splitdirection) | yes | orientation |
| `ratio` | number (float) | yes | first-child fraction |
| `rect` | [PaneLayoutRect](#panelayoutrect) | yes | the divider's cell rectangle |

This `id` is a rendered-geometry identifier only. `layout.set_split_ratio` does not take it:
the request instead takes a `path` array of booleans descending the
[LayoutNode](#layoutnode) tree to the target split, with `path: []` addressing the root
split — passing `split_id` instead fails with `invalid_request: missing field 'path'`.
Validated 2026-09-19 against herdr 0.9.1.

## PaneLayoutRect

A rectangle in terminal cells. All fields are `uint16` (0..65535).

| field | type | required | meaning |
| --- | --- | --- | --- |
| `x` | integer (uint16) | yes | left column (cells) |
| `y` | integer (uint16) | yes | top row (cells) |
| `width` | integer (uint16) | yes | width in cells |
| `height` | integer (uint16) | yes | height in cells |

Validated 2026-09-19 against herdr 0.9.1.

## PaneScrollInfo

The scrollback viewport position for a pane (`PaneInfo.scroll`). All fields `uint64`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `offset_from_bottom` | integer (uint64) | yes | rows scrolled up from the live bottom (0 = pinned to bottom) |
| `max_offset_from_bottom` | integer (uint64) | yes | maximum scrollable offset (history depth) |
| `viewport_rows` | integer (uint64) | yes | visible rows in the viewport |

Validated 2026-09-19 against herdr 0.9.1.

## PaneProcessInfo

Process introspection for a pane, returned by `pane.process_info`
(`result.type = pane_process_info`). Corroborated by `pane-process-info.json`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `pane_id` | string | yes | the pane |
| `shell_pid` | integer (uint32) \| null | no | PID of the pane's shell |
| `tty` | string \| null | no | controlling tty path |
| `foreground_process_group_id` | integer (uint32) \| null | no | foreground process group ID |
| `foreground_processes` | array&lt;[PaneProcessInfoProcess](#paneprocessinfoprocess)&gt; | no | processes in the foreground group |

Validated 2026-09-19 against herdr 0.9.1.

## PaneProcessInfoProcess

One process in a pane's foreground group.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `pid` | integer (uint32) | yes | process ID |
| `name` | string | yes | process name (e.g. `claude`) |
| `argv0` | string \| null | no | argv[0] as launched |
| `argv` | array&lt;string&gt; \| null | no | full argument vector |
| `cmdline` | string \| null | no | joined command line |
| `cwd` | string \| null | no | process working directory |

`pid`, `name`, `cmdline`, and `cwd` were confirmed live for a plain login shell. `argv0`
and `tty` are unverifiable in this pass: the single foreground process captured never
exercised a case that would distinguish a null value from an omitted one for either field
(would need a process launched with a distinct `argv[0]`, or a platform with different tty
resolution). Validated 2026-09-19 against herdr 0.9.1 (`argv0`/`tty` not exercised, see
above).

## PaneReadResult

The output of reading a pane's screen/scrollback, returned by `pane.read` and embedded in
`output_matched` (from `pane.wait_for_output`; there is no `pane.wait_output` method — the
schema's per-method docs in `api/pane.md` name it correctly, but this cross-reference used
the wrong name). Corroborated by `pane-read.json`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `pane_id` | string | yes | the pane read |
| `workspace_id` | string | yes | owning workspace |
| `tab_id` | string | yes | owning tab |
| `source` | [ReadSource](#readsource) | yes | which snapshot was read |
| `format` | [ReadFormat](#readformat) | yes | `text` or `ansi` |
| `text` | string | yes | the captured content |
| `revision` | integer (uint64) | yes | pane output revision at read time |
| `truncated` | boolean | yes | output exceeded the requested/allowed size and was cut |

`truncated` can read `true` even when all of the pane's actual (non-blank) content was
returned: it is computed against the pane's full rendered-height line buffer, blank padding
included, so a small explicit `lines` cap (e.g. `0`, `1`, `5`) on a pane with only a few
non-blank lines still comes back `truncated:true` with text identical to an untruncated,
much larger `lines` request — don't treat `truncated:true` alone as proof that real content
was cut. Validated 2026-09-19 against herdr 0.9.1.

## ReadSource

Enum. Selects which snapshot `pane.read`/`pane.wait_for_output` reads. Semantics from
[raw/skill.md](raw/skill.md).

| value | meaning |
| --- | --- |
| `visible` | the currently rendered viewport |
| `recent` | recent rendered output, including soft wraps |
| `recent_unwrapped` | recent output with soft wraps joined; preferred for logs/transcripts |
| `detection` | the plain-text bottom-buffer snapshot used for agent detection |

Note: the CLI spells this option `recent-unwrapped` (hyphen); the wire enum is
`recent_unwrapped` (underscore). Validated 2026-09-19 against herdr 0.9.1.

## ReadFormat

Enum. Output encoding for a read.

| value | meaning |
| --- | --- |
| `text` | plain text, styling stripped |
| `ansi` | includes ANSI color/style escapes (use when styling is evidence) |

Validated 2026-09-19 against herdr 0.9.1.

## PaneTextPoint

A single row/column coordinate into a pane's text content. Used by the `pane.copy_motion`
and `pane.copy_search` results (see `api/pane.md`). Corroborated by `scratch/pane-copy-motion.json`
(`{"row":0,"col":1}`).

| field | type | required | meaning |
| --- | --- | --- | --- |
| `row` | integer (uint32) | yes | row index |
| `col` | integer (uint16) | yes | column index |

Validated 2026-09-19 against herdr 0.9.1.

## PaneTextRange

A span between two [PaneTextPoint](#panetextpoint)s. Used by the `pane.copy_search` result's
`matches` (see `api/pane.md`). Corroborated by `scratch/pane-copy-search.json`
(`{"start":{"row":0,"col":1},"end":{"row":0,"col":7}}`).

| field | type | required | meaning |
| --- | --- | --- | --- |
| `start` | [PaneTextPoint](#panetextpoint) | yes | start of the range |
| `end` | [PaneTextPoint](#panetextpoint) | yes | end of the range |

The `pane.copy_motion`/`pane.copy_search` result wrappers that carry these points also
carry a `content_revision` counter, unrelated to and far finer-grained than
[`PaneInfo.revision`](#paneinfo) (observed values in the thousands versus single digits for
`revision` on the same pane). `pane.copy_search` requires the caller's `content_revision` to
exactly match the pane's current one, or the call fails with `stale_content: "pane content
changed"` — always fetch a fresh `content_revision` immediately before searching. Validated
2026-09-19 against herdr 0.9.1.

## PaneLinkRegion

Inclusive display-cell columns on a pane's current viewport where a resolved link hit
occurs. Returned by `pane.link.resolve` (see `api/pane.md`). Corroborated by
`scratch/pane-link-resolve.json` (a pane with no links returned `regions: []`).

| field | type | required | meaning |
| --- | --- | --- | --- |
| `row` | integer (uint16) | yes | viewport row |
| `start_col` | integer (uint16) | yes | first column of the hit, inclusive |
| `end_col` | integer (uint16) | yes | last column of the hit, inclusive |

The result's own `row` is named as above, but the request parameter for the same value is
not `row` — `pane.link.resolve` takes `viewport_row`, and calling it with `row` fails with
`invalid_request: missing field 'viewport_row'`. Validated 2026-09-19 against herdr 0.9.1.

## Pane operation result shapes

These result wrappers are returned by individual `pane.*`/`layout.*` methods; each embeds a
`PaneLayoutSnapshot` (named `layout`, `source_layout`/`target_layout`, etc.) so a caller can
re-render immediately. They are collected here because they share the same layout/reason
building blocks; see `api/pane.md` for which method returns which.

### PaneEdgesResult

Whether each side of a pane touches the tab's outer boundary (`pane.edges`). Corroborated by
`pane-edges.json`. Required: `pane_id`, `left`, `right`, `up`, `down`, `layout`.

| field | type | meaning |
| --- | --- | --- |
| `pane_id` | string | the pane |
| `left` / `right` / `up` / `down` | boolean | this side touches the tab's outer edge — i.e. there is **no** neighbor beyond it |
| `layout` | [PaneLayoutSnapshot](#panelayoutsnapshot) | current geometry |

The schema's own field-level description reads "a neighbor exists in that direction," but
the live meaning is the opposite: a lone pane filling the whole tab (zero neighbors on every
side) returns `left:true,right:true,up:true,down:true`, and a pane with a real neighbor on
one side but sitting flush against the screen's other three edges reports `true` on exactly
those three boundary sides and `false` on the side with the neighbor. Use
[PaneNeighborResult](#paneneighborresult) below to ask whether a neighbor exists in a given
direction. Validated 2026-09-19 against herdr 0.9.1.

### PaneNeighborResult

The neighbor in a given direction (`pane.neighbor`). Corroborated by `pane-neighbor.json`.
Required: `pane_id`, `direction`, `layout`.

| field | type | meaning |
| --- | --- | --- |
| `pane_id` | string | the source pane |
| `direction` | [PaneDirection](#panedirection) | queried direction |
| `neighbor_pane_id` | string \| null | neighbor pane, or null if none |
| `layout` | [PaneLayoutSnapshot](#panelayoutsnapshot) | current geometry |

When there is no neighbor, `neighbor_pane_id` is not sent as `null` — the key is absent
from the object entirely. Validated 2026-09-19 against herdr 0.9.1.

### PaneFocusDirectionResult

Result of a directional focus move (`pane.focus_direction`). Required: `changed`,
`source_pane_id`, `layout`.

| field | type | meaning |
| --- | --- | --- |
| `changed` | boolean | focus actually moved |
| `source_pane_id` | string | pane focus started from |
| `focused_pane_id` | string \| null | pane now focused |
| `reason` | [PaneFocusDirectionReason](#reason--auxiliary-enums) \| null | why no move happened (`no_neighbor`) |
| `layout` | [PaneLayoutSnapshot](#panelayoutsnapshot) | current geometry |

As with the other nullable `reason`/`*_id` fields on this page, `reason` is omitted (not
sent as `null`) on a successful move. Validated 2026-09-19 against herdr 0.9.1.

### PaneResizeResult

Result of resizing (`pane.resize`). Required: `changed`, `pane_id`, `focused_pane_id`,
`layout`.

| field | type | meaning |
| --- | --- | --- |
| `changed` | boolean | geometry changed |
| `pane_id` | string | resized pane |
| `focused_pane_id` | string | focused pane |
| `reason` | [PaneResizeReason](#reason--auxiliary-enums) \| null | why nothing changed (`unchanged`) |
| `layout` | [PaneLayoutSnapshot](#panelayoutsnapshot) | current geometry |

Validated 2026-09-19 against herdr 0.9.1.

### PaneSwapResult

Result of swapping two panes (`pane.swap`). Required: `changed`, `source_pane_id`,
`focused_pane_id`, `layout`.

| field | type | meaning |
| --- | --- | --- |
| `changed` | boolean | a swap occurred |
| `source_pane_id` | string | first pane |
| `target_pane_id` | string \| null | second pane, if resolved |
| `reason` | [PaneSwapReason](#reason--auxiliary-enums) \| null | why no swap (`no_neighbor`/`same_pane`/`not_found`/`cross_tab`) |
| `focused_pane_id` | string | focused pane |
| `layout` | [PaneLayoutSnapshot](#panelayoutsnapshot) | current geometry |

Validated 2026-09-19 against herdr 0.9.1.

### PaneZoomResult

Result of zoom toggling (`pane.zoom`). Required: `changed`, `zoom_changed`,
`focus_changed`, `pane_id`, `focused_pane_id`, `zoomed`, `layout`.

| field | type | meaning |
| --- | --- | --- |
| `changed` | boolean | any state changed |
| `zoom_changed` | boolean | zoom state toggled |
| `focus_changed` | boolean | focus moved as a side effect |
| `pane_id` | string | target pane |
| `focused_pane_id` | string | focused pane |
| `zoomed` | boolean | resulting zoom state |
| `reason` | [PaneZoomReason](#reason--auxiliary-enums) \| null | why no change (`single_pane`/`already_zoomed`/`already_unzoomed`) |
| `layout` | [PaneLayoutSnapshot](#panelayoutsnapshot) | current geometry |

Validated 2026-09-19 against herdr 0.9.1.

### PaneMoveResult

Result of moving a pane to another tab/workspace (`pane.move`). Required: `changed`,
`previous_pane_id`, `previous_workspace_id`, `previous_tab_id`, `pane`, `target_layout`,
`focused_pane_id`. A move can create or close a tab/workspace. Corroborated by
`raw/workspace-move.json`, `scratch/pane-move.json`.

| field | type | meaning |
| --- | --- | --- |
| `changed` | boolean | the pane actually moved |
| `pane` | [PaneInfo](#paneinfo) | the moved pane, with its **new** `pane_id` |
| `previous_pane_id` | string | the pane's ID before the move (only the moved process's inherited context resolves it) |
| `previous_workspace_id` | string | source workspace |
| `previous_tab_id` | string | source tab |
| `focused_pane_id` | string | focused pane after the move |
| `reason` | [PaneMoveReason](#reason--auxiliary-enums) \| null | `same_tab` / `zoomed_tab` |
| `created_tab` | [TabInfo](#tabinfo) \| null | tab created to host the pane, if any |
| `created_workspace` | [WorkspaceInfo](#workspaceinfo) \| null | workspace created to host the pane, if any |
| `closed_tab_id` | string \| null | tab emptied and closed by the move |
| `closed_workspace_id` | string \| null | workspace emptied and closed by the move |
| `source_layout` | [PaneLayoutSnapshot](#panelayoutsnapshot) \| null | geometry of the source tab after the move |
| `target_layout` | [PaneLayoutSnapshot](#panelayoutsnapshot) | geometry of the destination tab |

`source_layout` is omitted (not sent as `null`) when the move empties and closes the source
workspace, since no source tab remains to describe. Validated 2026-09-19 against herdr 0.9.1
(every `reason` value, a plain cross-tab move, a move that populates `created_tab`, one that
populates `created_workspace`, and one that closes the source tab/workspace were all
exercised live).

## WorktreeInfo

A git worktree known to herdr, listed by `worktree.list` and returned by
`worktree.create`/`worktree.open`/`worktree.remove` events. Corroborated by
`worktree-list.json`, `scratch/worktree-create.json`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `path` | string | yes | absolute checkout path |
| `label` | string | yes | display label (repo/worktree name) |
| `is_bare` | boolean | yes | the repo is bare |
| `is_detached` | boolean | yes | HEAD is detached |
| `is_prunable` | boolean | yes | git considers the worktree prunable |
| `is_linked_worktree` | boolean | yes | a linked worktree (not the primary checkout) |
| `branch` | string \| null | no | checked-out branch, or null if detached/bare |
| `open_workspace_id` | string \| null | no | workspace currently holding this worktree open, if any |

Validated 2026-09-19 against herdr 0.9.1 (live-exercised across a normal worktree, a
detached-HEAD worktree with `branch:null`, a bare source repo, and a worktree made
`is_prunable:true` by removing its directory out from under it; `open_workspace_id` was
observed to populate and clear as the worktree's workspace opened and closed).

## WorkspaceWorktreeInfo

The worktree a workspace is checked out to (`WorkspaceInfo.worktree`,
`PluginInvocationContext.worktree`). A workspace-centric view of the checkout.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `repo_key` | string | yes | canonical repo identity key (typically the `.git` path) |
| `repo_name` | string | yes | repository name |
| `repo_root` | string | yes | repository root path |
| `checkout_path` | string | yes | this workspace's checkout path |
| `is_linked_worktree` | boolean | yes | the checkout is a linked worktree |

Validated 2026-09-19 against herdr 0.9.1 (confirmed via `workspace.get` on a
worktree-backed workspace, which returns exactly this shape as `worktree`).

## WorktreeSourceInfo

Describes the repository/source that a `worktree.list` result was taken from
(`worktree_list.source`). Corroborated by `worktree-list.json`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `repo_key` | string | yes | canonical repo identity key |
| `repo_name` | string | yes | repository name |
| `repo_root` | string | yes | repository root path |
| `source_checkout_path` | string | yes | checkout the listing was resolved from |
| `source_workspace_id` | string \| null | no | workspace that provided the source context |

Validated 2026-09-19 against herdr 0.9.1. `worktree.list` from a location outside any Git
work tree fails with `not_git_worktree`; the message text differs slightly depending on
whether the problem is the caller's own workspace ("...require a workspace inside a Git
work tree") or an explicit `path`/`cwd` param ("...require a path inside a Git work tree").

## ServerCapabilities

Feature flags reported by the server in the `pong` result (`ping`). Corroborated by
`scratch/ping.json` (server running, version 0.9.1, protocol 22:
`{"live_handoff":true,"detached_server_daemon":false,"endpoint_protocol_generation":1,"surface_interest":true,"health_check":true}`).

| field | type | required | meaning |
| --- | --- | --- | --- |
| `live_handoff` | boolean | yes | server supports live handoff of the session to another client |
| `detached_server_daemon` | boolean | no (default false) | server can run as a detached daemon |
| `endpoint_protocol_generation` | integer (uint32) \| null | no | stable client-owned endpoint generation this server supports |
| `health_check` | boolean | no (default false) | server supports endpoint health probes |
| `surface_interest` | boolean | no (default false) | server supports explicit client-shell surface interest (`client_shell.surface.set`) |

Validated 2026-09-19 against herdr 0.9.1.

## AgentManifestInfo

An installed agent-detection manifest and its remote-update state, returned by
`server.agent_manifests`/`server.reload_agent_manifests` (results
`agent_manifest_status` / `agent_manifest_reload`). Corroborated by
`raw/server-agent-manifests.json`.

| field | type | required | meaning |
| --- | --- | --- | --- |
| `agent` | string | yes | agent kind, e.g. `claude`, `codex`, `pi` |
| `source` | string | yes | manifest source path (e.g. `remote:/…/claude.toml`) |
| `source_kind` | string | yes | source kind, e.g. `remote`, `local` |
| `local_override_shadowing_remote` | boolean | yes | a local manifest is shadowing the remote one |
| `active_version` | string \| null | no | version currently in effect (e.g. `2026.08.13.1`) |
| `cached_remote_version` | string \| null | no | last-fetched remote version |
| `remote_update_result` | string \| null | no | outcome of the last remote check (e.g. `current`) |
| `remote_update_error` | string \| null | no | error message from the last failed check |
| `remote_last_checked_unix` | integer (uint64) \| null | no | unix time of the last remote check |
| `warning` | string \| null | no | non-fatal load warning |

Validated 2026-09-19 against herdr 0.9.1 (23 live manifests, each with the required fields
populated; `remote_update_error`/`warning` were absent — not `null` — in this healthy
state, not independently forced).

## IntegrationInfo

One agent's editor/CLI integration status, as listed by `integration.list` (result
`integration_list`, see `api/integration.md`). Corroborated by `scratch/integration-list.json`
(e.g. `{"target":"claude","label":"claude","command":"claude","available":true,"state":"outdated"}`).

| field | type | required | meaning |
| --- | --- | --- | --- |
| `target` | [IntegrationTarget](#integrationtarget) | yes | the agent this entry describes |
| `label` | string | yes | display label |
| `command` | string | yes | the command herdr invokes for this integration |
| `available` | boolean | yes | the agent's command is available on this machine |
| `state` | [IntegrationState](#integrationstate) | yes | current install state of the integration |

`command` is not always derived from `target`/`label`: e.g. `cursor`'s command is
`cursor-agent`, and `antigravity_cli`'s label is `antigravity-cli` while its command is
`agy`. Validated 2026-09-19 against herdr 0.9.1 (all 17 `IntegrationTarget` entries
returned live with these 5 fields).

## IntegrationState

Enum. `IntegrationInfo.state`.

| value | meaning |
| --- | --- |
| `not_installed` | the integration has not been installed |
| `current` | installed and up to date |
| `outdated` | installed but older than what herdr would install now |

Validated 2026-09-19 against herdr 0.9.1 (`not_installed`/`current` observed live across the
17 targets; `outdated` requires an installed integration older than what herdr would
install now, not present on the probed machine, and is schema-confirmed only).

## IntegrationTarget

Enum. The agent whose editor/CLI integration `integration.install`/`integration.uninstall`
targets. Full value set:

`pi`, `omp`, `claude`, `codex`, `copilot`, `devin`, `droid`, `kimi`, `opencode`, `kilo`,
`hermes`, `qodercli`, `qwen`, `cursor`, `mastracode`, `antigravity_cli`, `grok`.

Validated 2026-09-19 against herdr 0.9.1 (`integration.list` returned exactly these 17
values in exactly this order).

## ConfigReloadStatus

Enum. Outcome of `server.reload_config` (`config_reload` result). Corroborated by
`scratch/server-reload-config.json` (`status: applied`, empty `diagnostics`).

| value | meaning |
| --- | --- |
| `applied` | new config fully applied |
| `partial` | applied with some settings rejected (see `diagnostics`) |
| `failed` | reload failed; previous config retained |

Validated 2026-09-19 against herdr 0.9.1 (`applied` observed live on a healthy config,
matching the worked example verbatim; `partial`/`failed` were not provoked, since
`server.reload_config` reads the shared, non-session-scoped config file and crafting an
invalid one risked affecting state outside the scratch session — both are schema-confirmed
only).

## PopupSize

A popup/pane dimension value (`PluginManifestPane.width`/`height`). Union of two forms:

| form | schema | meaning |
| --- | --- | --- |
| integer | `uint16` (0..65535) | outer size in terminal cells, including the border |
| percentage string | matches `^(100\|[1-9][0-9]?)%$` | outer size as a percent of the terminal area, e.g. `"80%"` |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: no plugin was
installed on the probed machine, and `plugin.link` was excluded from live probing because
it can write a managed install path outside the scratch session directory, so no
`PluginManifestPane` with a real `width`/`height` was ever produced to read back).

## Plugin entities

Herdr plugins contribute actions, panes, event hooks, and link handlers. On the probed
session no plugins were installed (`raw/plugin-list.json` → `plugins: []`,
`raw/plugin-action-list.json` → `actions: []`), so shapes below are from the schema.

The empty-list state above (`plugin.list`, `plugin.action.list`, and `plugin.log.list` all
returning `[]`) and the not-found error paths (`plugin.action.invoke`/`plugin.pane.open`
with an unknown `plugin_id` both fail `plugin_not_found`) are Validated 2026-09-19 against
herdr 0.9.1. No plugin was installed on the probed machine, and `plugin.link`/`enable`/
`disable` were excluded from live probing because they can write a managed plugin path
outside the scratch session directory, so each entity below (`InstalledPluginInfo` through
the [Plugin enums](#plugin-enums)) carries its own not-live-validated stamp instead of one
shared for the group.

### InstalledPluginInfo

A registered plugin (`plugin.list`, `plugin.link`, `plugin.enable`/`disable`). Required:
`plugin_id`, `name`, `version`, `manifest_path`, `plugin_root`, `enabled`.

| field | type | meaning |
| --- | --- | --- |
| `plugin_id` | string | stable plugin identifier |
| `name` | string | plugin name |
| `version` | string | plugin version |
| `manifest_path` | string | path to the plugin manifest file |
| `plugin_root` | string | plugin root directory |
| `enabled` | boolean | plugin is enabled |
| `description` | string \| null | human description |
| `min_herdr_version` | string (default "") | minimum required herdr version |
| `source` | [PluginSourceInfo](#pluginsourceinfo) (default `{kind:"local"}`) | where the plugin came from |
| `platforms` | array&lt;[PluginPlatform](#pluginplatform)&gt; \| null | supported platforms |
| `actions` | array&lt;[PluginManifestAction](#pluginmanifestaction)&gt; | contributed actions |
| `panes` | array&lt;PluginManifestPane&gt; | contributed panes |
| `startup` | array&lt;PluginManifestStartup&gt; | startup commands |
| `build` | array&lt;PluginManifestBuild&gt; | build commands |
| `events` | array&lt;PluginManifestEventHook&gt; | event hooks |
| `link_handlers` | array&lt;PluginManifestLinkHandler&gt; | URL/link handlers |
| `warnings` | array&lt;string&gt; | non-fatal link/load warnings (kept and surfaced) |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: no plugin was
installed on the probed machine, so this shape was cross-checked byte-for-byte against
`raw/schema.json` instead of read back from a live `plugin.list`).

### PluginManifestAction

A declared action inside a manifest. Required: `id`, `title`, `command`.

| field | type | meaning |
| --- | --- | --- |
| `id` | string | action id |
| `title` | string | display title |
| `command` | array&lt;string&gt; | command + argv to run |
| `contexts` | array&lt;[PluginActionContext](#pluginactioncontext)&gt; | contexts the action is offered in |
| `description` | string \| null | description |
| `platforms` | array&lt;[PluginPlatform](#pluginplatform)&gt; \| null | supported platforms |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

### PluginActionInfo

A resolved, invokable action (`plugin.action.list`, `plugin_action_invoked`). Required:
`plugin_id`, `action_id`, `title`, `command`.

| field | type | meaning |
| --- | --- | --- |
| `plugin_id` | string | owning plugin |
| `action_id` | string | action id |
| `title` | string | display title |
| `command` | array&lt;string&gt; | command + argv |
| `contexts` | array&lt;[PluginActionContext](#pluginactioncontext)&gt; | offered contexts |
| `description` | string \| null | description |
| `platforms` | array&lt;[PluginPlatform](#pluginplatform)&gt; \| null | supported platforms |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

### PluginManifestBuild / PluginManifestStartup

Each has `command` (array&lt;string&gt;, required) and optional `platforms`
(array&lt;[PluginPlatform](#pluginplatform)&gt; \| null). Build commands run at link/build
time; startup commands run when the plugin starts.

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

### PluginManifestEventHook

Runs a command on a herdr event. Required: `on`, `command`.

| field | type | meaning |
| --- | --- | --- |
| `on` | string | event name to hook (validated against known [EventKind](#eventkind) names; unknown names produce a warning) |
| `command` | array&lt;string&gt; | command + argv |
| `platforms` | array&lt;[PluginPlatform](#pluginplatform)&gt; \| null | supported platforms |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

### PluginManifestLinkHandler

Handles matching URLs/links. Required: `id`, `title`, `pattern`, `action`.

| field | type | meaning |
| --- | --- | --- |
| `id` | string | handler id |
| `title` | string | display title |
| `pattern` | string | match pattern |
| `action` | string | action id to invoke on match |
| `platforms` | array&lt;[PluginPlatform](#pluginplatform)&gt; \| null | supported platforms |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

### PluginManifestPane

Declares a plugin-provided pane. Required: `id`, `title`, `command`.

| field | type | meaning |
| --- | --- | --- |
| `id` | string | pane id |
| `title` | string | display title |
| `command` | array&lt;string&gt; | command + argv |
| `description` | string \| null | description |
| `placement` | [PluginPanePlacement](#pluginpaneplacement) (default `overlay`) | how the pane is placed |
| `width` | [PopupSize](#popupsize) \| null | pane width |
| `height` | [PopupSize](#popupsize) \| null | pane height |
| `platforms` | array&lt;[PluginPlatform](#pluginplatform)&gt; \| null | supported platforms |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

### PluginPaneInfo

A live plugin pane (`plugin_pane_opened`/`plugin_pane_focused`). Required: `plugin_id`,
`entrypoint`, `pane`.

| field | type | meaning |
| --- | --- | --- |
| `plugin_id` | string | owning plugin |
| `entrypoint` | string | the pane manifest entrypoint id |
| `pane` | [PaneInfo](#paneinfo) | the underlying pane |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

### PluginCommandLogInfo

A record of a plugin command execution (`plugin.log.list`, `plugin_action_invoked.log`).
Required: `log_id`, `plugin_id`, `command`, `status`, `started_unix_ms`.

| field | type | meaning |
| --- | --- | --- |
| `log_id` | string | log entry id |
| `plugin_id` | string | owning plugin |
| `command` | array&lt;string&gt; | command + argv executed |
| `status` | [PluginCommandStatus](#plugincommandstatus) | running/succeeded/failed |
| `started_unix_ms` | integer (uint64) | start time (unix ms) |
| `finished_unix_ms` | integer (uint64) \| null | finish time (unix ms) |
| `action_id` | string \| null | originating action, if any |
| `event` | string \| null | originating event, if any |
| `exit_code` | integer (int32) \| null | process exit code |
| `error` | string \| null | error message |
| `stdout` | string \| null | captured stdout |
| `stderr` | string \| null | captured stderr |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

### PluginInvocationContext

The context passed to a plugin action invocation (`plugin_action_invoked.context`). All
fields optional/nullable — the object records whatever focus/selection state existed at
invocation.

| field | type | meaning |
| --- | --- | --- |
| `invocation_source` | string \| null | what triggered the invocation |
| `correlation_id` | string \| null | correlation id |
| `link_handler_id` | string \| null | link handler that fired |
| `clicked_url` | string \| null | clicked URL |
| `selected_text` | string \| null | selected text |
| `workspace_id` / `workspace_label` / `workspace_cwd` | string \| null | focused workspace context |
| `tab_id` / `tab_label` | string \| null | focused tab context |
| `focused_pane_id` / `focused_pane_cwd` / `focused_pane_agent` | string \| null | focused pane context |
| `focused_pane_status` | [AgentStatus](#agentstatus) \| null | focused pane's agent status |
| `worktree` | [WorkspaceWorktreeInfo](#workspaceworktreeinfo) \| null | focused workspace's worktree |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

### PluginSourceInfo

Provenance of an installed plugin (`InstalledPluginInfo.source`). No required fields.

| field | type | meaning |
| --- | --- | --- |
| `kind` | [PluginSourceKind](#pluginsourcekind) (default `local`) | `local` or `github` |
| `owner` / `repo` / `subdir` | string \| null | GitHub coordinates |
| `requested_ref` | string \| null | requested git ref |
| `resolved_commit` | string \| null | resolved commit |
| `managed_path` | string \| null | managed install path |
| `installed_unix_ms` | integer (uint64) \| null | install time (unix ms) |

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

### Plugin enums

- **PluginActionContext** — `global`, `workspace`, `tab`, `pane`, `selection`.
- **PluginPanePlacement** — `overlay`, `popup`, `split`, `tab`, `zoomed`.
- **PluginPlatform** — `linux`, `macos`, `windows`.
- **PluginCommandStatus** — `running`, `succeeded`, `failed`.
- **PluginSourceKind** — `local`, `github`.

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: same as
InstalledPluginInfo, above).

## Event entities

Pushed to a connection after `events.subscribe` as `{"event": …, "data": …}` lines (see
[protocol.md](protocol.md)); `EventEnvelope` is also embedded in the `wait_matched`
result from `events.wait`. Corroborated by `raw/events-subscribe-capture.json`.

### EventEnvelope

| field | type | required | meaning |
| --- | --- | --- | --- |
| `event` | [EventKind](#eventkind) | yes | the event name |
| `data` | EventData | yes | the event payload (a tagged union whose `type` equals `event`) |

The pushed-event half of this shape is Validated 2026-09-19 against herdr 0.9.1: every line
on a held-open `events.subscribe` connection had exactly this `{event, data}` envelope. The
`wait_matched`-from-`events.wait` half is Constructed from schema; not live-validated
(2026-09-19, herdr 0.9.1: `events.wait` requires a `match_event` field, not documented on
this page, even to wait for the next event of any kind with only a `timeout_ms` — a request
supplying just `timeout_ms` fails `invalid_request: missing field 'match_event'`; see
`api/events.md` for the params shape).

### EventKind

Enum of all event names:

`workspace_created`, `workspace_updated`, `workspace_metadata_updated`, `workspace_closed`,
`workspace_renamed`, `workspace_moved`, `workspace_reordered`, `workspace_focused`,
`worktree_created`, `worktree_opened`, `worktree_removed`, `tab_created`, `tab_closed`,
`tab_renamed`, `tab_moved`, `tab_focused`, `pane_created`, `pane_closed`, `pane_updated`,
`pane_focused`, `pane_moved`, `pane_output_changed`, `pane_exited`, `pane_agent_detected`,
`pane_agent_status_changed`, `layout_updated`.

Validated 2026-09-19 against herdr 0.9.1 (16 of the 26 kinds were live-triggered and
observed on the wire in this pass — `workspace_created`, `workspace_renamed`,
`workspace_closed`, `workspace_focused`, `worktree_created`, `worktree_removed`,
`tab_created`, `tab_closed`, `tab_renamed`, `tab_focused`, `pane_created`, `pane_updated`,
`pane_focused`, `pane_moved`, `pane_exited`, `layout_updated`; the remaining 10 —
`workspace_updated`, `workspace_metadata_updated`, `workspace_moved`,
`workspace_reordered`, `worktree_opened`, `tab_moved`, `pane_closed`,
`pane_output_changed`, `pane_agent_detected`, `pane_agent_status_changed` — were not
triggered by this probe sequence and remain schema-confirmed only).

### EventData

A tagged union (`type` discriminator matching the [EventKind](#eventkind)) carrying the
payload for each event. Payloads embed the domain entities above — e.g.
`workspace_created` carries `workspace` ([WorkspaceInfo](#workspaceinfo)); `pane_created`
carries `pane` ([PaneInfo](#paneinfo)); `pane_agent_status_changed` carries `pane_id`,
`workspace_id`, `agent_status` ([AgentStatus](#agentstatus)) plus optional `agent`,
`display_agent`, `title`, `state_labels`; `layout_updated` carries `layout`
([PaneLayoutSnapshot](#panelayoutsnapshot)). See `api/events.md` for per-event payload
tables.

Validated 2026-09-19 against herdr 0.9.1 for `workspace_created`, `pane_created`, and
`layout_updated` (each carries exactly the keys above, live). `pane_agent_status_changed`'s
payload was not triggered in this pass (no agent-bearing pane was exercised here), so its
optional-field list (`agent`/`display_agent`/`title`/`state_labels`) is Constructed from
schema; not live-validated (2026-09-19, herdr 0.9.1: reason above).

## Reason & auxiliary enums

Small enums that annotate *why* an operation produced its result. Each appears as the
`reason` field (nullable) of the correspondingly named result shape above.

| enum | values | used by |
| --- | --- | --- |
| `PaneFocusDirectionReason` | `no_neighbor` | `PaneFocusDirectionResult` |
| `PaneResizeReason` | `unchanged` | `PaneResizeResult` |
| `PaneSwapReason` | `no_neighbor`, `same_pane`, `not_found`, `cross_tab` | `PaneSwapResult` |
| `PaneZoomReason` | `single_pane`, `already_zoomed`, `already_unzoomed` | `PaneZoomResult` |
| `PaneMoveReason` | `same_tab`, `zoomed_tab` | `PaneMoveResult` |
| `NotificationShowReason` | `shown`, `disabled`, `rate_limited`, `no_foreground_client`, `busy` | `notification.show` result (`scratch/notification-show.json` → `disabled`) |
| `ClientWindowTitleReason` | `set`, `cleared`, `no_foreground_client` | `client.window_title.set` result (`raw/client-window-title-set.json` → `no_foreground_client`) |

Validated 2026-09-19 against herdr 0.9.1 for `no_neighbor`, `unchanged`,
`same_pane`/`not_found`/`no_neighbor` (Swap), `single_pane`/`already_zoomed`/
`already_unzoomed`, `same_tab`/`zoomed_tab`, and `no_foreground_client` (both
`NotificationShowReason` and `ClientWindowTitleReason`), all observed live. `shown`,
`disabled`, `rate_limited`, `busy` (`NotificationShowReason`) and `set`, `cleared`
(`ClientWindowTitleReason`) require a real foreground client attached to the session, which
this headless probing never had, and remain schema-confirmed only.

---

## Appendix: on-disk persistence (session.json, version 3)

Herdr persists layout topology to a `session.json` file so a session can be restored. The
default-session file is `~/.config/herdr/session.json`; named sessions use
`~/.config/herdr/sessions/<name>/session.json`. This on-disk shape is **internal and
distinct from the API entities above** — it uses internal integer pane numbers (not public
`w1:p1` IDs) and capitalized `Split`/`Pane` layout variants (not the API's lowercase
`split`/`pane`). It is documented here only to relate the two; the socket API never returns
it.

Top-level object (`version: 3`):

| field | type | meaning |
| --- | --- | --- |
| `version` | integer | schema version (currently `3`) |
| `workspaces` | array | persisted workspaces (see below) |
| `active` | integer | index of the active workspace |
| `selected` | integer | index of the selected workspace |
| `sidebar_width` | integer | UI sidebar width |
| `sidebar_section_split` | number | UI sidebar section split ratio |
| `collapsed_space_keys` | array | collapsed sidebar groups |

Validated 2026-09-19 against herdr 0.9.1 (a scratch session's `session.json` has exactly
these top-level keys, `version:3`).

Each **workspace** entry: `id` (string), `custom_name` (string \| null), `identity_cwd`
(string), `public_pane_numbers` (object mapping internal→public pane number),
`next_public_pane_number` (int), `public_tab_numbers` (array), `next_public_tab_number`
(int), `tabs` (array), `active_tab` (int).

Each **tab** entry: `custom_name` (string \| null), `layout` (recursive tree, below),
`panes` (object keyed by internal pane number → `{ cwd, agent_session? }`, where
`agent_session` is `{ source, agent, kind, value }` when present), `zoomed` (bool),
`focused` (int pane number), `root_pane` (int).

`agent_session` is omitted from a pane's persisted entry entirely (not sent as `null`) when
that pane has no attached agent — an ordinary shell pane persists as just `{"cwd": "…"}`.
Validated 2026-09-19 against herdr 0.9.1 (workspace and tab key sets both confirmed exactly
as listed above; the unconditional `{ cwd, agent_session }` shape previously shown here was
corrected to reflect this).

The persisted **layout** tree is a recursive union with capitalized variant keys, and the
direction value is likewise PascalCase:

```json
{ "Split": { "direction": "Horizontal", "ratio": 0.5,
             "first": { "Pane": 2 }, "second": { "Pane": 5 } } }
```

— a leaf is `{ "Pane": <internal-number> }`; an interior node is `{ "Split": { direction,
ratio, first, second } }`. The embedded `agent_session` object mirrors
[AgentSessionInfo](#agentsessioninfo) (`source`, `agent`, `kind`, `value`), which is how a
restored pane re-attaches to its agent's native session. Validated 2026-09-19 against herdr
0.9.1 (a live split persisted as `{"Split":{"direction":"Horizontal","ratio":0.5,
"first":{"Pane":6},"second":{"Pane":10}}}`; pane numbers above are illustrative, matching
this page's convention, not that literal capture).

Restarting a server against a pre-existing, non-empty session directory (named or default)
respawns a real shell process for every persisted pane at its persisted `cwd` rather than
starting clean — reusing a named session directory should be expected to bring back
whatever it last held. Validated 2026-09-19 against herdr 0.9.1.

(Structure read from `~/.config/herdr/session.json`; concrete `cwd`/`identity_cwd` values
are omitted here as they are user paths.)
