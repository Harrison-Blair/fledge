# herdr API: workspace methods

> herdr 0.9.3 · protocol 22 · schema_version 1 · captured 2026-10-06
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `workspace` namespace manages herdr's top-level containers. A workspace holds one or
more tabs, each tab holds one or more panes, and every pane runs a terminal that herdr may
recognize as a coding agent. These methods create, inspect, focus, rename, reorder, close,
and attach display-only metadata to workspaces. Workspace IDs are short opaque strings
assigned as a Crockford-style base32 counter — `w1`, `w2`, … `w9`, `wA`, `wB`, … `wZ`,
skipping the letters `I`, `L`, `O`, and `U` — and are never reused after a workspace is
closed. The counter does not continue as plain base32 past `wZ`: on 0.9.3 the next
workspace was `w0`, then `w11`, `w12`, … (`w10` was never issued). Treat IDs as opaque
rather than assuming any digit shape. Validated 2026-10-06 against herdr 0.9.3. Reads (`get`,
`list`) do not mark an agent's tab as seen; only focusing does. Most mutations emit a
push event to connections subscribed via `events.subscribe`, delivered on a fixed ~100 ms
tick after the response it corresponds to (see [events.md](../events.md); a subscription
frame arrived about 100 ms after the call on 0.9.3); each Events row
below gives both the `events.subscribe` subscription type (dotted, e.g. `workspace.closed`)
and the event actually delivered on the wire (underscored, e.g. `workspace_closed`, in both
its `event` and `data.type` fields).

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

**Params** (`WorkspaceCloseParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | ID of the workspace to close (e.g. `w2`). |
| `close_group` | boolean | no | `false` | If true, also close the target's linked worktree workspaces (its worktree group). |

Closing the root of a properly-formed worktree group (a workspace opened on a repo's
primary checkout that has a linked worktree opened against it) without `close_group` is
refused — see `workspace_group_close_required` below; `close_group` is not optional sugar
for a group root, it is mandatory. `close_group` is also asymmetric: from the group root it
closes the root and every linked child; from a linked child it closes only that child and
leaves the root alive. On a workspace with no group at all, `close_group: true` is accepted
and behaves as a plain close.

**Result** — `type: "ok"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"ok"` | Acknowledgment; no payload. |

**Errors**

| code | when |
| --- | --- |
| `workspace_not_found` | `workspace_id` does not match a live workspace. |
| `workspace_group_close_required` | The target is the root of a worktree group and `close_group` was not `true`. |

Other codes possible.

**Events**: emits a `workspace_closed` event to subscribers (subscription type
`workspace.closed`) carrying the full final `WorkspaceInfo` and `workspace_id`. A group
close emits one `workspace_closed` per closed workspace, root first.

**CLI**: `herdr workspace close <workspace_id> [--group]` (`--group` maps to `close_group`;
it is missing from `herdr workspace close --help`'s own usage line but appears in the usage
printed for a bad invocation and in the full `herdr workspace` command listing).

**Example** — Captured 2026-09-19 against herdr 0.9.1; Validated 2026-10-06 against herdr 0.9.3 (plain close, unknown ID, `close_group: true` on an ungrouped workspace, root close refused without `close_group`, child-only and whole-group closes, and the `--group` help omission).

```json
{"id":"c1","method":"workspace.close","params":{"workspace_id":"w2"}}
{"id":"c1","result":{"type":"ok"}}
```

`close_group` also validated, against a real root+linked-child worktree group:

```json
{"id":"c2","method":"workspace.close","params":{"workspace_id":"w1"}}
{"id":"c2","error":{"code":"workspace_group_close_required","message":"workspace has linked worktree workspaces; use --group (close_group=true in the API) to close the group"}}
```

```json
{"id":"c3","method":"workspace.close","params":{"workspace_id":"w1","close_group":true}}
{"id":"c3","result":{"type":"ok"}}
```

## workspace.create

Create a new workspace containing a single root tab and a single root pane. The response
exposes the IDs to use next: `workspace`, `tab`, and `root_pane`. Per skill.md, do not
create a new workspace unless the user explicitly requested that topology. The new pane's
working directory is resolved with this precedence: `cwd` if given, else the pane cwd of
`source_workspace_id` if given, else herdr's default (the user's home directory); any
supplied `env` is overlaid on top. A `cwd` that does not exist is not an error — the pane
silently falls back to herdr's default, and the only way to detect this is to compare the
requested `cwd` against the returned `root_pane.cwd`. The returned path is normalized
(a `cwd` of `…/plain/../plain` came back as `…/plain`). The very first workspace created on a
server comes back `focused: true` even when `focus` is omitted or `false` (focus has to
live somewhere); on a server that already has a workspace, `focus` defaults to `false` as
documented below.

**Params** (`WorkspaceCreateParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `cwd` | string \| null | no | null | Working directory for the root pane's process; see the precedence and fallback above. |
| `env` | object (string→string) | no | `{}` | Environment variables set for the launched process. |
| `focus` | boolean | no | `false` | If true, focus the new workspace in the UI; false creates it in the background. |
| `label` | string \| null | no | null | Display label; null lets herdr auto-assign one: the basename of the Git work tree root when the effective cwd is inside a Git work tree (a cwd of `…/lbl/deep/er` in repo `lbl` gives `lbl`), else the basename of the effective cwd, or `~` for the home directory. Validated 2026-10-06 against herdr 0.9.3. |
| `source_workspace_id` | string \| null | no | null | Workspace whose focused pane supplies the fallback cwd (see precedence above). |

**Result** — `type: "workspace_created"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_created"` | Result discriminant. |
| `workspace` | WorkspaceInfo | The created workspace ([../data-model.md](../data-model.md)). |
| `tab` | TabInfo | The workspace's root tab ([../data-model.md](../data-model.md)). |
| `root_pane` | PaneInfo | The root pane of the root tab ([../data-model.md](../data-model.md)). |

**Errors**: `invalid_request` (malformed input, e.g. a wrong JSON type for a field —
schema-level rejections come back as `invalid_request` with a serde message, never as
`invalid_params`; on 0.9.3 the response echoes the request's own `id` when the line is
valid JSON with a string `id`, where 0.9.1 answered `"id":""`),
`workspace_not_found` (unknown `source_workspace_id`); other codes possible.

**Events**: emits three events in order to subscribers: `workspace_created` (subscription
type `workspace.created`), `tab_created` (subscription type `tab.created`), and
`pane_created` (subscription type `pane.created`). On 0.9.3 these are followed by
`layout_updated` and, once the root shell settles, `pane_updated`. When the new workspace
becomes focused (the first workspace on a server), `workspace_focused`, `tab_focused`, and
`pane_focused` are interleaved after their matching `*_created` event. Validated 2026-10-06 against herdr 0.9.3.

**CLI**: `herdr workspace create [--cwd PATH] [--label TEXT] [--env KEY=VALUE] [--focus] [--no-focus]`

**Example** — Captured 2026-09-19 against herdr 0.9.1; Validated 2026-10-06 against herdr 0.9.3 (first-workspace focus, nonexistent-cwd fallback, label rules, `source_workspace_id` cwd precedence, and unknown `source_workspace_id`).

```json
{"id":"cli:workspace:create","method":"workspace.create","params":{"label":"docs-ws","focus":true}}
{"id":"cli:workspace:create","result":{"type":"workspace_created","workspace":{"active_tab_id":"w1:t1","agent_status":"unknown","focused":true,"label":"docs-ws","number":1,"pane_count":1,"tab_count":1,"workspace_id":"w1"},"tab":{"agent_status":"unknown","focused":true,"label":"1","number":1,"pane_count":1,"tab_id":"w1:t1","workspace_id":"w1"},"root_pane":{"agent_status":"unknown","cwd":"/…/scratch-repo","focused":true,"foreground_cwd":"/…/scratch-repo","pane_id":"w1:p1","revision":0,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":40},"tab_id":"w1:t1","terminal_id":"term_65970bc8958f71","workspace_id":"w1"}}}
```

`source_workspace_id` also validated 2026-09-19 against herdr 0.9.1 (this capture) and re-validated 2026-10-06 against herdr 0.9.3:

```json
{"id":"w2","method":"workspace.create","params":{"label":"child-ws","focus":false,"source_workspace_id":"w1"}}
{"id":"w2","result":{"type":"workspace_created","workspace":{"active_tab_id":"w2:t1","agent_status":"unknown","focused":false,"label":"child-ws","number":2,"pane_count":1,"tab_count":1,"workspace_id":"w2"},"tab":{"agent_status":"unknown","focused":false,"label":"1","number":1,"pane_count":1,"tab_id":"w2:t1","workspace_id":"w2"},"root_pane":{"agent_status":"unknown","cwd":"/home/penguin","focused":false,"foreground_cwd":"/home/penguin","pane_id":"w2:p1","revision":0,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":40},"tab_id":"w2:t1","terminal_id":"term_65bb4d8e42c402","workspace_id":"w2"}}}
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

**Result** — `type: "workspace_info"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_info"` | Result discriminant. |
| `workspace` | WorkspaceInfo | The now-focused workspace ([../data-model.md](../data-model.md)). |

**Errors**

| code | when |
| --- | --- |
| `workspace_not_found` | `workspace_id` does not match a live workspace. |

Other codes possible.

**Events**: emits a `workspace_focused` event to subscribers (subscription type
`workspace.focused`) carrying `{workspace_id}` only, not a full `WorkspaceInfo`. On 0.9.3
the same tick also carries `tab_focused` and `pane_focused` for the workspace's active
tab and pane. Focusing a workspace that is already focused produces no event at all — a caller
that focuses then waits for `workspace_focused` can hang when the target was already
focused.

**CLI**: `herdr workspace focus <workspace_id>`

**Example** — Captured 2026-09-19 against herdr 0.9.1; Validated 2026-10-06 against herdr 0.9.3. (`agent_status: "working"` is
illustrative; both passes exercised the result shape with plain shell panes, not a live
agent.)

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

**Example** — Captured 2026-09-19 against herdr 0.9.1 (live session); Validated 2026-10-06 against herdr 0.9.3 on a scratch server.

```json
{"id":"cli:workspace:get","method":"workspace.get","params":{"workspace_id":"w2"}}
{"id":"cli:workspace:get","result":{"type":"workspace_info","workspace":{"active_tab_id":"w2:t1","agent_status":"working","focused":false,"label":"fledge","number":2,"pane_count":1,"tab_count":1,"workspace_id":"w2"}}}
```

The `workspace_not_found` error is confirmed by probe (same message on 0.9.3):

```json
{"id":"cli:workspace:get","method":"workspace.get","params":{"workspace_id":"w99"}}
{"id":"cli:workspace:get","error":{"code":"workspace_not_found","message":"workspace w99 not found"}}
```

## workspace.list

List every workspace in display order. Read-only; does not mark any tab as seen. A caller
enumerating workspaces this way may see one more than expected: calling
[`worktree.create`](worktree.md#worktreecreate) with a bare `cwd` pointing directly at a
repo's primary checkout — no workspace already open there, and no `workspace_id` passed —
silently opens a second, hidden workspace for that primary checkout in addition to the
linked-worktree workspace the call actually returns; the response never mentions the hidden
workspace, and it only surfaces via a later `workspace.list` or `workspace.get`.
Validated 2026-10-06 against herdr 0.9.3.

**Params** (`EmptyParams`): none. Send `params: {}`.

**Result** — `type: "workspace_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_list"` | Result discriminant. |
| `workspaces` | array of WorkspaceInfo | All workspaces, in display order ([../data-model.md](../data-model.md)). |

**Errors**: other codes possible.

**CLI**: `herdr workspace list`

**Example** — Captured 2026-09-19 against herdr 0.9.1 (live session); Validated 2026-10-06 against herdr 0.9.3 on a scratch server.

```json
{"id":"cli:workspace:list","method":"workspace.list","params":{}}
{"id":"cli:workspace:list","result":{"type":"workspace_list","workspaces":[{"active_tab_id":"w1:t1","agent_status":"idle","focused":true,"label":"fledge","number":1,"pane_count":1,"tab_count":1,"workspace_id":"w1"},{"active_tab_id":"w2:t1","agent_status":"working","focused":false,"label":"fledge","number":2,"pane_count":1,"tab_count":1,"workspace_id":"w2"}]}}
```

## workspace.move

Move one workspace to a position in the display order. `insert_index` is resolved against
the **pre-move** order, as in [`tab.move`](tab.md#tabmove): the workspace is inserted before
whatever currently occupies that index, and only then is its old slot removed. Moving a
workspace forward therefore lands it one slot earlier than the index names — moving the
first of ten workspaces to `insert_index: 2` put it at index 1, and moving a workspace to
its own index or the index just after it is a no-op. The accepted range is `0..=N`
inclusive, where `N` is the workspace count including the one being moved:
`insert_index=N` places it last, `N-1` places it second-to-last when it starts earlier in
the list, and `N+1` is refused (see `workspace_move_failed` below). The response returns
the full reordered list, whose `number` fields reflect the new positions. Validated 2026-10-06 against herdr 0.9.3.

**Params** (`WorkspaceMoveParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | ID of the workspace to move. |
| `insert_index` | integer (uint, ≥ 0) | yes | — | Zero-based target index, evaluated against the display order **before** the move. Valid range: `0..=N`. |

**Result** — `type: "workspace_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_list"` | Result discriminant. |
| `workspaces` | array of WorkspaceInfo | The full list after reordering ([../data-model.md](../data-model.md)). |

**Errors**: `workspace_not_found` (unknown `workspace_id`), `workspace_move_failed`
(`insert_index` out of bounds, e.g. `"insert_index 8 is out of bounds"`); other codes
possible. A negative `insert_index` fails at the schema layer instead, as `invalid_request`
(`insert_index` is an unsigned integer).

**Events**: emits exactly one `workspace_moved` event to subscribers (subscription type
`workspace.moved`) carrying `{workspace_id, insert_index, workspaces}`; `move` never emits
`workspace_reordered` — that name belongs to `move_block`, below. A move to the workspace's
current index is a no-op and emits nothing. herdr also pushes a redundant
`workspace_focused` for whichever workspace currently holds focus in the same delivery
tick, even when focus did not change and the focused workspace was not the one moved; on
0.9.3 that redundant `workspace_focused` comes with matching `tab_focused` and
`pane_focused` events.

**CLI**: API-only (no CLI subcommand). The `herdr workspace` command group lists only
list/create/get/focus/rename/report-metadata/close.

**Example** — Captured 2026-09-19 against herdr 0.9.1; Validated 2026-10-06 against herdr 0.9.3 (`N`, `N-1`, `N+1` → `"insert_index 6 is out of bounds"`, `-1` → `invalid_request` "expected usize", no-op moves with no event).

```json
{"id":"wm1","method":"workspace.move","params":{"workspace_id":"w2","insert_index":0}}
{"id":"wm1","result":{"type":"workspace_list","workspaces":[{"workspace_id":"w2","number":1,"label":"second-ws","focused":false,"pane_count":1,"tab_count":1,"active_tab_id":"w2:t1","agent_status":"unknown"},{"workspace_id":"w1","number":2,"label":"--label docs-ws-renamed","focused":true,"pane_count":6,"tab_count":6,"active_tab_id":"w1:t1","agent_status":"unknown","tokens":{"branch":"main"},"worktree":{"repo_key":"/…/scratch-repo/.git","repo_name":"scratch-repo","repo_root":"/…/scratch-repo","checkout_path":"/…/scratch-repo","is_linked_worktree":false}},{"workspace_id":"w3","number":3,"label":"docs-probe","focused":false,"pane_count":1,"tab_count":1,"active_tab_id":"w3:t1","agent_status":"unknown","worktree":{"repo_key":"/…/scratch-repo/.git","repo_name":"scratch-repo","repo_root":"/…/scratch-repo","checkout_path":"/…/worktrees/scratch-repo/docs-probe","is_linked_worktree":true}}]}}
```

## workspace.move_block

Move a block of workspaces so that they end up contiguous, immediately before a target
workspace, preserving the relative order given in `workspace_ids`. The IDs need not already
be adjacent in the display order — "contiguous" describes the result, not a precondition;
non-adjacent IDs are pulled together at the target position. When `before_workspace_id` is
null the block is moved to the end of the display order. The response returns the full
reordered list.

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

**Errors**: `workspace_not_found` (any ID, in `workspace_ids` or `before_workspace_id`, that
does not match a live workspace), `workspace_move_block_failed` (`workspace_ids` is empty —
`"workspace_ids must not be empty"`; an ID repeated within `workspace_ids` —
`"workspace w1 appears more than once"`; or `before_workspace_id` is itself inside
`workspace_ids` — `"before_workspace_id must not be part of workspace_ids"`); other codes
possible.

**Events**: emits exactly one `workspace_reordered` event to subscribers (subscription type
`workspace.reordered`) carrying `{workspace_ids, before_workspace_id, workspaces}`
(`before_workspace_id` is omitted when the request sent `null`);
`move_block` never emits `workspace_moved` — that name belongs to `move`, above. As with
`move`, herdr also pushes a redundant `workspace_focused` (with `tab_focused` and
`pane_focused` on 0.9.3) for the currently focused workspace in the same delivery tick,
even when focus did not change.

**CLI**: API-only (no CLI subcommand).

**Example** — Captured 2026-09-19 against herdr 0.9.1; Validated 2026-10-06 against herdr 0.9.3 (null and non-null `before_workspace_id`, non-adjacent IDs, and all five error cases).

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
| `label` | string | yes | — | The new display label. No length or content validation: an empty string and a 500-character string are both accepted and echoed verbatim. |

**Result** — `type: "workspace_info"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"workspace_info"` | Result discriminant. |
| `workspace` | WorkspaceInfo | The renamed workspace ([../data-model.md](../data-model.md)). |

**Errors**: `workspace_not_found` (unknown `workspace_id`); other codes possible.

**Events**: emits a `workspace_renamed` event to subscribers (subscription type
`workspace.renamed`) carrying `{workspace_id, label}` only, not a full `WorkspaceInfo`.
Unlike `workspace.focus`, a no-op rename (to the workspace's existing label) still emits
the event.

**CLI**: `herdr workspace rename <workspace_id> <label>` (the CLI accepts the label as one
or more trailing arguments: `rename <WORKSPACE_ID> <LABEL>...`).

**Example** — Captured 2026-09-19 against herdr 0.9.1; Validated 2026-10-06 against herdr 0.9.3 (empty and 500-character labels, same-label rename still emitting `workspace_renamed`, unknown ID, multi-word CLI label).

```json
{"id":"cli:workspace:rename","method":"workspace.rename","params":{"workspace_id":"w1","label":"--label docs-ws-renamed"}}
{"id":"cli:workspace:rename","result":{"type":"workspace_info","workspace":{"active_tab_id":"w1:t1","agent_status":"unknown","focused":true,"label":"--label docs-ws-renamed","number":1,"pane_count":1,"tab_count":1,"workspace_id":"w1"}}}
```

## workspace.report_metadata

Attach display-only metadata to a workspace: a bag of short string tokens keyed by name.
Tokens are advisory display state (they surface in `WorkspaceInfo.tokens`, e.g.
`{"branch":"main"}`); they do not change topology or agent status. Despite `source`'s name,
it does **not** scope the token values it owns — any source can set, overwrite, or clear
(via a `null` value) tokens set by a different source; the tokens are a single shared bag
per workspace. The only thing `source` actually scopes is the `seq` counter (below). A
token whose value is `null` clears that token; clearing a token that was never set is a
silent no-op. When the last token on a workspace is cleared, the `tokens` key disappears
from `WorkspaceInfo` entirely rather than becoming `{}`. `seq` provides ordering so stale
reports can be discarded, per source, and `ttl_ms` bounds how long a token remains before
it expires — expiry is server-initiated: the server clears the token and pushes
`workspace_metadata_updated` on its own, with no triggering client request.

**Params** (`WorkspaceReportMetadataParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `workspace_id` | string | yes | — | ID of the workspace to annotate. |
| `source` | string | yes | — | Namespace/identity of the reporter. Must be non-empty (`""` fails `invalid_metadata_source`). Scopes only the `seq` counter below — not the token values (see above). |
| `tokens` | object (string→(string \| null)) | yes | — | Token map. Must not be empty (`{}` fails `invalid_metadata_token`, `"missing token to set or clear"`). Keys match `^[A-Za-z0-9_-]{1,32}$`; at most 16 entries per report (a workspace accumulates tokens across separate reports up to a separate, undocumented cap of 32 total — see Errors); a null value clears that token. |
| `seq` | integer (uint64, ≥ 0) \| null | no | null | Monotonic sequence number for this source; lets the server drop out-of-order reports. Strictly greater than the previous seq for this source — an equal seq is also discarded, not only a lesser one, and the rejection is silent (still `{"type":"ok"}`, no error, and no `workspace_metadata_updated` event). |
| `ttl_ms` | integer (uint64) \| null | no | null | Token lifetime in milliseconds, 1 … 86400000 (24 h); null means no explicit expiry. |

**Result** — `type: "ok"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | string const `"ok"` | Acknowledgment; no payload. |

**Errors**: `workspace_not_found` (unknown `workspace_id`), `invalid_metadata_source`
(`source` is empty — `"metadata source must not be empty"`), `invalid_metadata_token` (a
token key does not match `^[A-Za-z0-9_-]{1,32}$` — `"invalid metadata token key: <key>"`;
a single report updates more than 16 tokens — `"a metadata report may update at most 16
tokens"`; or `tokens` is empty — `"missing token to set or clear"`),
`metadata_token_limit` (adding a new token while the workspace already holds 32 —
`"workspace metadata may contain at most 32 tokens"`; overwriting an existing token at
the cap still succeeds), `invalid_metadata_ttl` (`ttl_ms` outside `1..=86400000` —
`"metadata ttl_ms must be at least 1"` / `"metadata ttl_ms must be 86400000 or less"`);
other codes possible. None of these is `invalid_params`; that code
does not appear anywhere in this namespace.

**Events**: emits a `workspace_metadata_updated` event to subscribers (subscription type
`workspace.metadata_updated`) carrying the full post-update `WorkspaceInfo` (including
`tokens`) — unlike most other workspace events, not just an id.

**CLI**: `herdr workspace report-metadata <workspace_id> --source ID [--token NAME=VALUE] [--clear-token NAME] [--seq N] [--ttl-ms N]`
(`--token` sets a token; `--clear-token` sends a null value for that token).

**Example** — Captured 2026-09-19 against herdr 0.9.1; Validated 2026-10-06 against herdr 0.9.3 (shared token bag across sources, null clears, silent clear of an unset token, `tokens` key removed when empty, per-source `seq` with silent equal/lesser discard, every error above, and a 300 ms `ttl_ms` whose server-initiated expiry event arrived about 390 ms after the report).

```json
{"id":"1","method":"workspace.report_metadata","params":{"workspace_id":"w1","source":"my-tool","tokens":{"branch":"main"},"ttl_ms":60000}}
{"id":"1","result":{"type":"ok"}}
```

The 16-token cap applies per report; hitting the separate, undocumented 32-token-per-workspace cap looks like this:

```json
{"id":"2","method":"workspace.report_metadata","params":{"workspace_id":"w1","source":"my-tool","tokens":{"one_more":"x"}}}
{"id":"2","error":{"code":"metadata_token_limit","message":"workspace metadata may contain at most 32 tokens"}}
```
