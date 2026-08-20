# herdr API: events methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](README.md). Wire format: [protocol.md](protocol.md).

The `events` namespace turns the request/response socket into a push channel. `events.subscribe` registers a set of subscriptions on a connection and then leaves that connection open, streaming `{"event":…,"data":…}` lines as matching state changes occur; `events.wait` blocks a single connection until one event matching a predicate arrives (or a timeout elapses) and returns it as an ordinary response. Two distinct envelope families flow over this channel: plain **events** (`EventKind` / `EventData`, snake_case names — the 26 lifecycle notifications in the [event catalog](#event-catalog-eventkind--eventdata)) and **subscription events** (`SubscriptionEventKind` / `SubscriptionEventData`, dotted names — the three parameterized, computed notifications in [subscription events](#subscription-events-subscriptioneventkind--subscriptioneventdata)). A subscription's `type` selects which one you receive.

2 methods:

| method | purpose |
| --- | --- |
| [events.subscribe](#eventssubscribe) | Register subscriptions on the connection and stream matching events until it closes. |
| [events.wait](#eventswait) | Block until one event matching a predicate arrives, then return it. |

Reference sections that both methods draw on:

| section | contents |
| --- | --- |
| [Event catalog](#event-catalog-eventkind--eventdata) | Every `EventKind` value and its `EventData` payload. |
| [Subscription events](#subscription-events-subscriptioneventkind--subscriptioneventdata) | The `pane.output_matched` / `pane.agent_status_changed` / `pane.scroll_changed` push payloads. |
| [Subscription vs EventKind diffs](#subscription-vs-eventkind-name-and-coverage-diffs) | Exact name/coverage differences between the subscribable and observable surfaces. |

## events.subscribe

Registers one or more subscriptions on the current connection and starts streaming. The server first replies once with `{"type":"subscription_started"}` (the ack), then — **on the same connection, which stays open**, contrary to the normal one-request-per-connection rule (see [protocol.md](protocol.md)) — writes one newline-delimited JSON line per matching event for as long as the caller keeps reading. There is no unsubscribe method; close the connection to stop. Each pushed line is an envelope: `{"event":"<kind>","data":{…}}`. For the 24 plain subscription types the envelope is an [EventEnvelope](#event-catalog-eventkind--eventdata) (snake_case `event`, `data` per the catalog); for the three parameterized types it is a [SubscriptionEventEnvelope](#subscription-events-subscriptioneventkind--subscriptioneventdata) (dotted `event`). An empty `subscriptions` array is schema-valid but subscribes to nothing.

**Params**: `EventsSubscribeParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| subscriptions | array of `Subscription` | yes | — | Subscriptions to register on this connection. Each item is one variant below; mix freely. |

### Subscription variants

`Subscription` is a `oneOf` discriminated by `type`. There are 27 variants. Twenty-four carry no fields other than `type` and stream the like-named plain event (dotted `type` → snake_case `EventKind`; see the [diffs section](#subscription-vs-eventkind-name-and-coverage-diffs)). Three are parameterized and stream a subscription event.

| `type` const | fields (besides `type`) | delivers envelope | delivered `event` value |
| --- | --- | --- | --- |
| `workspace.created` | — | EventEnvelope | `workspace_created` |
| `workspace.updated` | — | EventEnvelope | `workspace_updated` |
| `workspace.metadata_updated` | — | EventEnvelope | `workspace_metadata_updated` |
| `workspace.renamed` | — | EventEnvelope | `workspace_renamed` |
| `workspace.moved` | — | EventEnvelope | `workspace_moved` |
| `workspace.reordered` | — | EventEnvelope | `workspace_reordered` |
| `workspace.closed` | — | EventEnvelope | `workspace_closed` |
| `workspace.focused` | — | EventEnvelope | `workspace_focused` |
| `worktree.created` | — | EventEnvelope | `worktree_created` |
| `worktree.opened` | — | EventEnvelope | `worktree_opened` |
| `worktree.removed` | — | EventEnvelope | `worktree_removed` |
| `tab.created` | — | EventEnvelope | `tab_created` |
| `tab.closed` | — | EventEnvelope | `tab_closed` |
| `tab.focused` | — | EventEnvelope | `tab_focused` |
| `tab.renamed` | — | EventEnvelope | `tab_renamed` |
| `tab.moved` | — | EventEnvelope | `tab_moved` |
| `pane.created` | — | EventEnvelope | `pane_created` |
| `pane.closed` | — | EventEnvelope | `pane_closed` |
| `pane.updated` | — | EventEnvelope | `pane_updated` |
| `pane.focused` | — | EventEnvelope | `pane_focused` |
| `pane.moved` | — | EventEnvelope | `pane_moved` |
| `pane.exited` | — | EventEnvelope | `pane_exited` |
| `pane.agent_detected` | — | EventEnvelope | `pane_agent_detected` |
| `pane.output_matched` | `pane_id`, `source`, `match`, `lines`, `strip_ansi` | SubscriptionEventEnvelope | `pane.output_matched` |
| `pane.agent_status_changed` | `pane_id`, `agent_status` | SubscriptionEventEnvelope | `pane.agent_status_changed` |
| `pane.scroll_changed` | `pane_id` | SubscriptionEventEnvelope | `pane.scroll_changed` |
| `layout.updated` | — | EventEnvelope | `layout_updated` |

Note: there is no `pane.output_changed` subscription — output is observed through the parameterized `pane.output_matched` (which filters and reads for you) rather than the raw `pane_output_changed` event. See the [diffs section](#subscription-vs-eventkind-name-and-coverage-diffs).

**`pane.output_matched` fields** — watch a pane's output and push only when a match hits:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Const `pane.output_matched`. |
| pane_id | string | yes | — | Pane to watch. |
| source | `ReadSource` enum | yes | — | Which text to search: `visible`, `recent`, `recent_unwrapped`, `detection`. |
| match | `OutputMatch` | yes | — | Match predicate (see below). |
| lines | integer (uint32) \| null | no | null (inferred: implementation default window) | Number of lines to consider/read around the match (inferred from name). |
| strip_ansi | boolean | no | `true` | Strip ANSI escapes before matching and before returning `matched_line`/`read.text`. |

`OutputMatch` is a `oneOf`:

| `type` | fields | meaning |
| --- | --- | --- |
| `substring` | `value`: string | Match when `value` occurs as a substring. |
| `regex` | `value`: string | Match when the regex `value` matches. |

**`pane.agent_status_changed` fields** — push on a pane's agent-status transitions:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Const `pane.agent_status_changed`. |
| pane_id | string | yes | — | Pane to watch. |
| agent_status | `AgentStatus` enum \| null | no | null | If set, filter to transitions **into** this status; if null/omitted, push on every change. Values: `idle`, `working`, `blocked`, `done`, `unknown`. |

**`pane.scroll_changed` fields** — push on a pane's scroll-position changes:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Const `pane.scroll_changed`. |
| pane_id | string | yes | — | Pane to watch. |

**Result** (ack): `type` const `subscription_started`. Sent once, immediately, before any pushed event.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `subscription_started`. |

**Pushed lines** (after the ack, zero or more): not responses — they carry no `id` and no `result`. Shape is `{"event":<EventKind|SubscriptionEventKind>,"data":<EventData|SubscriptionEventData>}`. See the [event catalog](#event-catalog-eventkind--eventdata) and [subscription events](#subscription-events-subscriptioneventkind--subscriptioneventdata).

**Errors**: none evidenced for a well-formed subscribe; malformed `params`/unknown variant yields the standard `invalid_params`-class error (see [errors.md](errors.md)). Other codes possible.

**CLI**: API-only (no `herdr events` CLI group). Related one-shot waits are exposed as `herdr pane wait-output` and `herdr agent wait`.

**Example** (Validated 2026-08-19 against herdr 0.8.2):

```json
{"id":"e1","method":"events.subscribe","params":{"subscriptions":[{"type":"tab.created"},{"type":"tab.closed"}]}}
{"id":"e1","result":{"type":"subscription_started"}}
{"event":"tab_created","data":{"type":"tab_created","tab":{"tab_id":"w1:t1","workspace_id":"w1","number":1,"label":"1","focused":true,"pane_count":1,"agent_status":"unknown"}}}
{"event":"tab_created","data":{"type":"tab_created","tab":{"tab_id":"w2:t1","workspace_id":"w2","number":1,"label":"1","focused":false,"pane_count":1,"agent_status":"unknown"}}}
```

The first line is the request, the second the ack; subsequent lines are pushed on the same open connection as tabs are created.

## events.wait

Blocks the connection until a single event matching the `match_event` predicate is observed, then returns it as a normal response (`wait_matched`) and closes the connection like any other one-shot request. With `timeout_ms` set, the wait returns an error when the deadline passes before a match; with `timeout_ms` null/omitted the wait is indefinite. Unlike `events.subscribe`, exactly one event is returned and the connection does not stay open.

**0.8.2 implementation gap (validated):** although the `EventMatch` schema admits 19 predicate variants (below), herdr 0.8.2 rejects every match except pane agent-status matches with `unsupported_event_wait_match` — message `events.wait currently supports pane agent status matches`. Treat the schema as the forward-looking surface and, on 0.8.2, only send a `pane_agent_status_changed` match. For the other kinds, subscribe with `events.subscribe` and read the first push instead.

**Params**: `EventsWaitParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| match_event | `EventMatch` | yes | — | Predicate selecting which event to wait for (variants below). |
| timeout_ms | integer (uint64) \| null | no | null | Max milliseconds to wait; null/omitted waits indefinitely. |

### EventMatch variants

`EventMatch` is a `oneOf` discriminated by `event` (snake_case, matching `EventKind`). 19 variants. On 0.8.2 only `pane_agent_status_changed` is honored; the rest are schema-valid but return `unsupported_event_wait_match`. All variants have an optional `min_revision`/`workspace_id`/etc. only where listed; unlisted id fields are required filters.

| `event` const | filter fields | required |
| --- | --- | --- |
| `workspace_created` | `workspace_id`: string \| null | `event` |
| `workspace_updated` | `workspace_id`: string | `event`, `workspace_id` |
| `workspace_closed` | `workspace_id`: string | `event`, `workspace_id` |
| `workspace_renamed` | `workspace_id`: string; `label`: string \| null | `event`, `workspace_id` |
| `workspace_moved` | `workspace_id`: string | `event`, `workspace_id` |
| `workspace_focused` | `workspace_id`: string | `event`, `workspace_id` |
| `tab_created` | `tab_id`: string \| null; `workspace_id`: string \| null | `event` |
| `tab_closed` | `tab_id`: string | `event`, `tab_id` |
| `tab_renamed` | `tab_id`: string; `label`: string \| null | `event`, `tab_id` |
| `tab_moved` | `tab_id`: string | `event`, `tab_id` |
| `tab_focused` | `tab_id`: string | `event`, `tab_id` |
| `pane_created` | `pane_id`: string \| null; `workspace_id`: string \| null | `event` |
| `pane_closed` | `pane_id`: string | `event`, `pane_id` |
| `pane_focused` | `pane_id`: string | `event`, `pane_id` |
| `pane_moved` | `pane_id`: string | `event`, `pane_id` |
| `pane_output_changed` | `pane_id`: string; `min_revision`: integer (uint64) \| null | `event`, `pane_id` |
| `pane_exited` | `pane_id`: string | `event`, `pane_id` |
| `pane_agent_detected` | `pane_id`: string; `agent`: string \| null | `event`, `pane_id` |
| `pane_agent_status_changed` | `pane_id`: string; `agent_status`: `AgentStatus` (`idle`\|`working`\|`blocked`\|`done`\|`unknown`) | `event`, `pane_id`, `agent_status` |

The `EventMatch` surface is narrower than the full `EventKind` list: it omits `workspace_metadata_updated`, `workspace_reordered`, `worktree_created`, `worktree_opened`, `worktree_removed`, `tab_created`'s sibling `tab`-only kinds already listed, `pane_updated`, and `layout_updated` — those 7 kinds (`workspace_metadata_updated`, `workspace_reordered`, `worktree_created`, `worktree_opened`, `worktree_removed`, `pane_updated`, `layout_updated`) cannot be waited on even in schema.

**Result**: `type` const `wait_matched`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `wait_matched`. |
| event | `EventEnvelope` | yes | — | The matched event: `{"event":<EventKind>,"data":<EventData>}`. See the [event catalog](#event-catalog-eventkind--eventdata). |

**Errors**:

| code | when |
| --- | --- |
| `unsupported_event_wait_match` | The `match_event` variant is not `pane_agent_status_changed` on herdr 0.8.2. Message: `events.wait currently supports pane agent status matches`. |

A timeout is also expected to surface as an error when `timeout_ms` elapses before a match (code not captured; likely a `timeout`-class code — verify against [errors.md](errors.md)). Other codes possible.

**CLI**: API-only (no `herdr events` CLI group). `herdr agent wait <TARGET>` covers the supported agent-status wait, and `herdr pane wait-output` covers output waiting.

**Example** (Validated 2026-08-19 against herdr 0.8.2 — shows the 0.8.2 rejection of a non-agent-status match):

```json
{"id":"e3","method":"events.wait","params":{"match_event":{"event":"tab_created"},"timeout_ms":5000}}
{"id":"e3","error":{"code":"unsupported_event_wait_match","message":"events.wait currently supports pane agent status matches"}}
```

A supported call substitutes `"match_event":{"event":"pane_agent_status_changed","pane_id":"w1:p1","agent_status":"idle"}` and returns `{"id":"e3","result":{"type":"wait_matched","event":{"event":"pane_agent_status_changed","data":{…}}}}` (Constructed from schema; not live-validated).

## Event catalog (EventKind / EventData)

The 26 `EventKind` values and their `EventData` payloads. Every pushed plain event and every `wait_matched.event` is an `EventEnvelope`: `{"event":<EventKind>,"data":<EventData>}`, where `data.type` repeats the kind. `EventData` is a `oneOf` discriminated by `data.type`; the tables below list each variant's fields. Domain entities (`WorkspaceInfo`, `TabInfo`, `PaneInfo`, `WorktreeInfo`, `PaneLayoutSnapshot`, `AgentStatus`) are defined in [data-model.md](data-model.md); only their field is named here.

`EventEnvelope`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| event | `EventKind` | yes | The kind (snake_case; one of the 26 below). |
| data | `EventData` | yes | Payload whose `type` equals `event`. |

**Workspace kinds**

| `type` | payload fields (required unless noted) |
| --- | --- |
| `workspace_created` | `workspace`: WorkspaceInfo |
| `workspace_updated` | `workspace`: WorkspaceInfo |
| `workspace_metadata_updated` | `workspace`: WorkspaceInfo |
| `workspace_closed` | `workspace_id`: string; `workspace`: WorkspaceInfo \| null (optional) |
| `workspace_renamed` | `workspace_id`: string; `label`: string |
| `workspace_moved` | `workspace_id`: string; `insert_index`: integer (uint); `workspaces`: array of WorkspaceInfo (the new order) |
| `workspace_reordered` | `workspace_ids`: array of string (new order); `workspaces`: array of WorkspaceInfo; `before_workspace_id`: string \| null (optional) |
| `workspace_focused` | `workspace_id`: string |

**Worktree kinds**

| `type` | payload fields (required unless noted) |
| --- | --- |
| `worktree_created` | `workspace`: WorkspaceInfo; `worktree`: WorktreeInfo |
| `worktree_opened` | `workspace`: WorkspaceInfo; `worktree`: WorktreeInfo; `already_open`: boolean |
| `worktree_removed` | `workspace_id`: string; `worktree`: WorktreeInfo; `forced`: boolean; `workspace`: WorkspaceInfo \| null (optional) |

**Tab kinds**

| `type` | payload fields (required unless noted) |
| --- | --- |
| `tab_created` | `tab`: TabInfo |
| `tab_closed` | `tab_id`: string; `workspace_id`: string |
| `tab_renamed` | `tab_id`: string; `workspace_id`: string; `label`: string |
| `tab_moved` | `tab_id`: string; `workspace_id`: string; `insert_index`: integer (uint); `tabs`: array of TabInfo (new order) |
| `tab_focused` | `tab_id`: string; `workspace_id`: string |

**Pane kinds**

| `type` | payload fields (required unless noted) |
| --- | --- |
| `pane_created` | `pane`: PaneInfo |
| `pane_closed` | `pane_id`: string; `workspace_id`: string |
| `pane_updated` | `pane`: PaneInfo |
| `pane_focused` | `pane_id`: string; `workspace_id`: string |
| `pane_moved` | `previous_pane_id`: string; `previous_workspace_id`: string; `previous_tab_id`: string; `pane`: PaneInfo. Optional: `closed_tab_id`: string \| null; `closed_workspace_id`: string \| null; `created_tab`: TabInfo \| null; `created_workspace`: WorkspaceInfo \| null |
| `pane_output_changed` | `pane_id`: string; `workspace_id`: string; `revision`: integer (uint64) |
| `pane_exited` | `pane_id`: string; `workspace_id`: string |
| `pane_agent_detected` | `pane_id`: string; `workspace_id`: string. Optional: `agent`: string \| null; `final_status`: AgentStatus \| null; `released`: boolean |
| `pane_agent_status_changed` | `pane_id`: string; `workspace_id`: string; `agent_status`: AgentStatus. Optional: `agent`: string \| null; `display_agent`: string \| null; `title`: string \| null; `state_labels`: map<string,string> |

**Layout kind**

| `type` | payload fields (required unless noted) |
| --- | --- |
| `layout_updated` | `layout`: PaneLayoutSnapshot |

`AgentStatus` enum (used above): `idle`, `working`, `blocked`, `done`, `unknown`.

Cross-check: the validated `tab.created` capture pushed `{"event":"tab_created","data":{"type":"tab_created","tab":{…TabInfo…}}}`, matching the `tab_created` row (single `tab: TabInfo` field, `data.type` echoing `event`).

## Subscription events (SubscriptionEventKind / SubscriptionEventData)

The three parameterized subscription types stream a **different** envelope than the plain event catalog: `SubscriptionEventEnvelope`, with a dotted `event` (`SubscriptionEventKind`) and a `SubscriptionEventData` payload computed for the subscription. This envelope is only ever produced by `events.subscribe`; it is never returned by `events.wait` (whose result embeds the plain `EventEnvelope`).

`SubscriptionEventKind` enum: `pane.output_matched`, `pane.agent_status_changed`, `pane.scroll_changed`.

`SubscriptionEventEnvelope`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| event | `SubscriptionEventKind` | yes | Dotted kind (one of the three above). |
| data | `SubscriptionEventData` | yes | Payload — one of the three variants below (an `anyOf`; distinguish by `event`, since the payloads have no `type` discriminator). |

**`pane.output_matched` → `PaneOutputMatchedEvent`**

| field | type | required | meaning |
| --- | --- | --- | --- |
| pane_id | string | yes | Pane that matched. |
| matched_line | string | yes | The line that satisfied the `OutputMatch` (ANSI-stripped if `strip_ansi`). |
| read | `PaneReadResult` | yes | A snapshot read of the pane around the match. |

`PaneReadResult`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| pane_id | string | yes | Pane read. |
| workspace_id | string | yes | Owning workspace. |
| tab_id | string | yes | Owning tab. |
| source | `ReadSource` enum | yes | `visible`, `recent`, `recent_unwrapped`, or `detection`. |
| format | `ReadFormat` enum | yes | `text` or `ansi`. |
| text | string | yes | The read text. |
| revision | integer (uint64) | yes | Output revision this read reflects. |
| truncated | boolean | yes | Whether `text` was truncated. |

**`pane.agent_status_changed` → `PaneAgentStatusChangedEvent`**

| field | type | required | meaning |
| --- | --- | --- | --- |
| pane_id | string | yes | Pane whose agent status changed. |
| workspace_id | string | yes | Owning workspace. |
| agent_status | `AgentStatus` enum | yes | New status: `idle`, `working`, `blocked`, `done`, `unknown`. |
| agent | string \| null | no | Agent identifier, if known. |
| display_agent | string \| null | no | Human-facing agent name, if known. |
| title | string \| null | no | Pane/agent title, if known. |
| state_labels | map<string,string> | no | Agent-provided state labels. |

Note: this payload matches the plain `pane_agent_status_changed` `EventData` field-for-field, but arrives under the dotted `SubscriptionEventEnvelope` when reached via a `pane.agent_status_changed` subscription.

**`pane.scroll_changed` → `PaneScrollChangedEvent`**

| field | type | required | meaning |
| --- | --- | --- | --- |
| pane_id | string | yes | Pane whose scroll position changed. |
| workspace_id | string | yes | Owning workspace. |
| scroll | `PaneScrollInfo` | yes | Scroll snapshot (below). |

`PaneScrollInfo`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| offset_from_bottom | integer (uint64) | yes | Current scrollback offset from the bottom (0 = pinned to bottom). |
| max_offset_from_bottom | integer (uint64) | yes | Maximum possible offset. |
| viewport_rows | integer (uint64) | yes | Visible viewport height in rows. |

`ReadFormat` enum: `text`, `ansi`. `ReadSource` enum: `visible`, `recent`, `recent_unwrapped`, `detection`.

## Subscription vs EventKind: name and coverage diffs

Three surfaces name the same domain differently; a client must map between them explicitly. `Subscription.type` and `SubscriptionEventKind` use **dotted** names; `EventKind`, `EventData.type`, and `EventMatch.event` use **snake_case** names. For the 24 plain subscription types, mapping is a literal dot→underscore substitution (`tab.created` → `tab_created`). The exceptions, verified by comparing the enum lists directly:

**Subscription types with no like-named `EventKind`** (they do not simply dot→underscore to a catalog event):

| `Subscription.type` | what it actually delivers |
| --- | --- |
| `pane.output_matched` | A `SubscriptionEventEnvelope` with `event` `pane.output_matched` (`PaneOutputMatchedEvent`). There is no `pane_output_matched` `EventKind`. The raw catalog analog is `pane_output_changed`, which is **not** independently subscribable — output is only reachable via this filtered/parameterized subscription. |
| `pane.scroll_changed` | A `SubscriptionEventEnvelope` with `event` `pane.scroll_changed` (`PaneScrollChangedEvent`). There is no `pane_scroll_changed` `EventKind` — scroll changes exist only as a subscription event, not in the plain catalog. |

**`EventKind` values not directly subscribable by matching `Subscription.type`:**

| `EventKind` | note |
| --- | --- |
| `pane_output_changed` | No `pane.output_changed` subscription exists; use `pane.output_matched` instead. It remains observable in `EventData` and matchable in `EventMatch`. |

Everything else lines up one-to-one: the 24 plain subscription types each map to a distinct catalog `EventKind`, and the `pane.agent_status_changed` subscription maps to both the plain `pane_agent_status_changed` `EventKind` (via `events.wait`/other emitters) and the dotted `pane.agent_status_changed` `SubscriptionEventKind` (via this subscription).

**`EventMatch` (events.wait) coverage:** narrower still — only 19 of the 26 `EventKind` values are expressible as a match. Not matchable in schema: `workspace_metadata_updated`, `workspace_reordered`, `worktree_created`, `worktree_opened`, `worktree_removed`, `pane_updated`, `layout_updated`. And on herdr 0.8.2 only `pane_agent_status_changed` matches are actually honored (all others → `unsupported_event_wait_match`).
