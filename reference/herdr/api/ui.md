# herdr API: ui methods

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `ui` namespace drives client-facing surface and shell methods: the foreground client's window title, desktop/toast notifications, the client popup overlay, the client-shell surface interest lease, command invocation through the client-shell projection, and dismissal of product announcements and release notes. These operations act on whichever client is currently in the foreground, or on the connected client-shell endpoint; several return a boolean plus a `reason` enum reporting whether the action took effect, because the target surface may be absent (no foreground client), disabled, or otherwise unavailable rather than failing outright.

8 methods:

| method | purpose |
| --- | --- |
| [client.window_title.clear](#clientwindow_titleclear) | Restore the foreground client's window title to its default. |
| [client.window_title.set](#clientwindow_titleset) | Set the foreground client's window title. |
| [client_shell.surface.set](#client_shellsurfaceset) | Acquire or release the client-shell surface interest lease. |
| [command.invoke](#commandinvoke) | Invoke a command surfaced through the client-shell projection. |
| [notification.show](#notificationshow) | Show a desktop/toast notification through the foreground client. |
| [popup.close](#popupclose) | Close the currently open client popup overlay. |
| [product_announcement.dismiss](#product_announcementdismiss) | Dismiss a product announcement. |
| [release_notes.dismiss](#release_notesdismiss) | Dismiss the release notes for a version. |

## client.window_title.clear

Clears any title override previously applied to the foreground client's window and restores the client's default title. Acts on the foreground client only; if no client is in the foreground the request succeeds but reports that nothing changed.

**Params**: `EmptyParams` — an empty object. Send `{}`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| _(none)_ | — | — | — | No parameters. `params` must be present; besides `{}`, herdr 0.9.1 also accepts an empty array `[]` and an object with unknown extra keys. Only a missing `params`, `null`, or a non-object/array scalar is rejected. |

**Result**: `type` const `client_window_title`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `client_window_title`. |
| changed | boolean | yes | — | `true` whenever a foreground client accepted the clear, even if no title override had ever been set; `false` only when there is no foreground client. The method does not track whether an override existed. |
| reason | string (enum) | yes | — | Outcome detail. One of: `set`, `cleared`, `no_foreground_client`. For this method the effective values are `cleared` (title reset) and `no_foreground_client` (no client to act on). |

**Errors**: none evidenced; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"1","method":"client.window_title.clear","params":{}}
{"id":"1","result":{"type":"client_window_title","changed":true,"reason":"cleared"}}
```

Validated 2026-09-19 against herdr 0.9.1. (`changed:true` requires a foreground client; with none attached the result is `changed:false` / `reason:"no_foreground_client"`.)

## client.window_title.set

Overrides the foreground client's window title with the supplied string. Acts on the foreground client only; if no client is in the foreground the request succeeds but reports that nothing changed. The change is applied by writing an OSC 0 escape sequence (`\x1b]0;<title>\x07`) to the client's terminal; the string is passed through unsanitized, including embedded control characters. There is also a startup race: for roughly the first seconds after a TUI client launches, and again the instant it exits, the server still reports `no_foreground_client` even though a session exists.

**Params**: `ClientWindowTitleSetParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| title | string | yes | — | The window title to display for the foreground client. Must be non-empty (see Errors); otherwise unconstrained — an 8192-character title, Unicode/emoji, and embedded control characters (BEL, ESC, newline) are all accepted. |

**Result**: `type` const `client_window_title`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `client_window_title`. |
| changed | boolean | yes | — | `true` if the title was applied; `false` if it could not be (e.g. no foreground client). |
| reason | string (enum) | yes | — | Outcome detail. One of: `set`, `cleared`, `no_foreground_client`. For this method the effective values are `set` (title applied) and `no_foreground_client` (no client to act on). |

**Errors**:

| code | when |
| --- | --- |
| invalid_params | `title` is empty (message: `window title is empty`). |

Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"r6","method":"client.window_title.set","params":{"title":"doc-probe"}}
{"id":"r6","result":{"type":"client_window_title","changed":false,"reason":"no_foreground_client"}}
```

Validated 2026-09-19 against herdr 0.9.1. (Probe ran with no foreground client, so `changed` is `false` and `reason` is `no_foreground_client`. With a client attached, the same request returns `changed:true` / `reason:"set"`.)

## client_shell.surface.set

Sets whether the requesting client-shell connection holds the surface interest lease — i.e. whether it receives and controls pane presentation. Only a client-shell endpoint connection may call this method; other API connections are rejected with `connection_local_only`. This method is new on the endpoint protocol, so its result carries a `projection_revision` that establishes an activation floor for the client-shell projection.

**Params**: `ClientShellSurfaceSetParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| active | boolean | yes | — | `true` to acquire the client-shell surface interest lease (receive and control pane presentation); `false` to release it. |

**Result**: `type` const `client_shell_surface_set`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `client_shell_surface_set`. |
| active | boolean | yes | — | The resulting lease state after the request. |
| projection_revision | integer (uint64) | yes | — | Revision floor for the client-shell projection established by this acknowledgement. |

**Errors**:

| code | when |
| --- | --- |
| connection_local_only | The request did not come from a client-shell endpoint connection (message: `client_shell.surface.set is only available through a client shell endpoint`). |

Other codes possible. Envelope validation runs before the connection-kind check: a malformed body (e.g. a missing or mistyped `active`) reports `invalid_request` even over a non-endpoint connection, not `connection_local_only`.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"2","method":"client_shell.surface.set","params":{"active":true}}
{"id":"2","error":{"code":"connection_local_only","message":"client_shell.surface.set is only available through a client shell endpoint"}}
```

Validated 2026-09-19 against herdr 0.9.1 (the `connection_local_only` rejection path only, reproduced both over a plain API connection and with a real TUI client attached — the caller's connection kind is what matters, not session state). The client-shell endpoint's success response, with its `active`/`projection_revision` shape, remains constructed from schema and not live-validated: the per-session `herdr-client.sock` resets the connection for every framing tried (plain newline-JSON, a greeting-prefixed line, and a 4-byte length-prefixed frame), and no external handshake to it was found.

## command.invoke

Invokes a command surfaced to the requesting client-shell connection through the client-shell projection. `command_id` is an opaque, endpoint-issued identifier obtained from that projection, not a fixed enum chosen by the caller; the `command_id` lookup happens before `pane_id`/`tab_id`/`workspace_id`/`selection` are resolved, so an unknown id masks any problem with those fields behind `command_not_found`. `pane_id`, `tab_id`, and `workspace_id` optionally scope the invocation to a specific target, and `selection` optionally carries client-owned selection coordinates (points are `{row, col}`, not `column`), which the schema says are validated against the pane's content revision (see [pane.md](pane.md)); no endpoint-issued `command_id` was reachable to observe that validation happen.

**Params**: `CommandInvokeParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| command_id | string | yes | — | Opaque endpoint-issued command identifier from the client-shell projection. |
| pane_id | string \| null | no | null | Pane to scope the invocation to, if any. |
| tab_id | string \| null | no | null | Tab to scope the invocation to, if any. |
| workspace_id | string \| null | no | null | Workspace to scope the invocation to, if any. |
| selection | PaneSelectionReadParams \| null | no | null | Client-owned selection coordinates, validated against the pane's content revision (see [pane.md](pane.md)). |

**Result**: `type` const `ok`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `ok`. |

**Errors**:

| code | when |
| --- | --- |
| command_not_found | The `command_id` is not currently valid. Observed for every unrecognized id tried, including on a server with a freshly loaded, valid custom-command manifest — the message names a stale manifest as the cause even when it is not (message observed: `custom command manifest is stale; reload configuration`). |

Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"5","method":"command.invoke","params":{"command_id":"probe.command"}}
{"id":"5","error":{"code":"command_not_found","message":"custom command manifest is stale; reload configuration"}}
```

Validated 2026-09-19 against herdr 0.9.1 (only the `command_not_found` rejection path — no endpoint-issued `command_id` is reachable from the public API socket, so the `{"type":"ok"}` success path and `selection`'s content-revision validation remain constructed from schema and not live-validated).

## notification.show

Shows a desktop/toast notification via the foreground client. Delivery is best-effort: notifications may be suppressed when the client has notifications disabled, when the client is rate-limited, when it is busy, or when there is no foreground client. The result's `shown` flag and `reason` enum report the actual outcome; a suppressed notification is still a successful request (not an error). `shown:true` means herdr accepted the notification for delivery, not that anything was actually presented: a foreground client with its own toast delivery turned off (e.g. `[ui.toast] delivery = "off"`) still reports `shown:true` / `reason:"shown"` while presenting nothing, and the caller cannot detect that from the response.

**Params**: `NotificationShowParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| title | string | yes | — | Notification title (primary line). Must be non-empty (see Errors). |
| body | string \| null | no | null | Optional notification body text. |
| position | ToastHerdrPosition \| null | no | null | Optional on-screen corner for the toast. One of: `top-left`, `top-right`, `bottom-left`, `bottom-right`. `null` uses the client default. |
| sound | NotificationShowSound (enum) | no | — | Sound to play. One of: `none`, `done`, `request`. Unlike `body` and `position`, this field does not accept an explicit `null` (rejected with `invalid_request`) — omit it instead of sending `null`. |

**Result**: `type` const `notification_show`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `notification_show`. |
| shown | boolean | yes | — | `true` if herdr accepted the notification for delivery, to a foreground client or via the no-client fallback path; `false` if no delivery path was available at all. Does not guarantee the notification was actually presented — see `disabled` and the note above. |
| reason | string (enum) | yes | — | Outcome detail. One of: `shown` (accepted for delivery), `disabled` (no foreground client, and the no-client fallback delivery path is turned off in config), `rate_limited` (a notification was already delivered within roughly the last second), `no_foreground_client` (no foreground client, and the fallback path is not disabled), `busy` (client not accepting notifications right now; defined in the schema but never produced live — see below). |

`disabled` only appears when there is no foreground client at all; a foreground client with its own delivery turned off still reports `shown` (see above). The rate limiter allows roughly one notification per second per server: a second call within about a second of the first is `rate_limited`, with delivery recovering after ~1.00 s, and a back-to-back burst has every call after the first dropped. `busy` was probed for with no client, an idle client, a client mid-startup, and a client with a popup overlay open, and was never produced.

**Errors**:

| code | when |
| --- | --- |
| invalid_params | `title` is empty (message: `notification title is empty`). |

Other codes possible.

**CLI**: `herdr notification show <TITLE> [--body <TEXT>] [--position <top-left|top-right|bottom-left|bottom-right>] [--sound <none|done|request>]`. Result keys print in alphabetical order (`reason`, `shown`, `type`), unlike the wire example below. A missing `TITLE` or an out-of-enum `--position` is rejected locally by the CLI (exit 2) and never reaches the socket.

**Example**:

```json
{"id":"cli:notification:show","method":"notification.show","params":{"title":"probe"}}
{"id":"cli:notification:show","result":{"type":"notification_show","shown":false,"reason":"disabled"}}
```

Validated 2026-09-19 against herdr 0.9.1. (Probe target had `[ui.toast] delivery = "off"` and no foreground client, which is required to observe `disabled`; the same request with a client attached — including a normal CLI invocation — instead returns `shown:true` / `reason:"shown"`.)

## popup.close

Closes the client popup overlay if one is currently open. If no popup is open, the request fails with `popup_not_open` rather than succeeding as a no-op. A popup overlay is not otherwise exclusive: while one is open, the other `ui` methods (`notification.show`, `client.window_title.set`/`clear`) continue to behave normally.

**Params**: `EmptyParams` — an empty object. Send `{}`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| _(none)_ | — | — | — | No parameters. `params` must be present; besides `{}`, herdr 0.9.1 also accepts an empty array `[]` and an object with unknown extra keys. Only a missing `params`, `null`, or a non-object/array scalar is rejected. |

**Result**: `type` const `ok`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `ok`. Indicates the popup was closed. |

**Errors**:

| code | when |
| --- | --- |
| popup_not_open | No popup is open, so there is nothing to close (message: `no popup is open`). |

Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"r7","method":"popup.close","params":{}}
{"id":"r7","error":{"code":"popup_not_open","message":"no popup is open"}}
```

Validated 2026-09-19 against herdr 0.9.1. (This example is the no-popup-open case. The success path is now live-validated too: opening a popup via `plugin.pane.open` with `placement: "popup"` and then calling `popup.close` returns `{"type":"ok"}`, and an immediately repeated call returns `popup_not_open` again.)

## product_announcement.dismiss

Records that the caller has dismissed a product announcement, identified by `id` and `version`. If the announcement named is no longer the current one the server has queued, the request fails with `stale_announcement` rather than silently succeeding.

**Params**: `ProductAnnouncementDismissParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| id | string | yes | — | Identifier of the announcement being dismissed. An empty string is accepted by the envelope and simply treated as not current (unlike `notification.show`'s `title`, it is not rejected with `invalid_params`). |
| version | string | yes | — | Version of the announcement being dismissed. Same empty-string handling as `id`. |

**Result**: `type` const `ok`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `ok`. |

**Errors**:

| code | when |
| --- | --- |
| stale_announcement | `id`/`version` no longer match the current product announcement (message: `the product announcement is no longer current`). |

Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"3","method":"product_announcement.dismiss","params":{"id":"probe-announcement","version":"0.9.1"}}
{"id":"3","error":{"code":"stale_announcement","message":"the product announcement is no longer current"}}
```

Validated 2026-09-19 against herdr 0.9.1 (the `stale_announcement` error path only — no scratch server had a current product announcement queued, and no API or config mechanism was found to inject one, so the `{"type":"ok"}` success path remains constructed from schema and not live-validated).

## release_notes.dismiss

Records that the caller has dismissed the release notes for `version`. If `version` does not match the release notes the server currently considers current, the request fails with `stale_release_notes`. "Current" is read from the `version` field of the config directory's `release-notes.json`, not from the running binary's version — the same `version` string returns `ok` against a config dir that has fetched release notes for it and `stale_release_notes` against an otherwise identical server whose config dir lacks that file. Matching is exact string comparison: no trimming of whitespace, no `v` prefix tolerance, and no semver normalization.

**Params**: `ReleaseNotesDismissParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| version | string | yes | — | Version of the release notes being dismissed, matched exact-string against `release-notes.json`. An empty string is accepted by the envelope and simply treated as not current (not rejected with `invalid_params`). |

**Result**: `type` const `ok`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `ok`. Repeating a successful dismissal also returns `ok` (idempotent). |

**Errors**:

| code | when |
| --- | --- |
| stale_release_notes | `version` does not match the release notes the server currently considers current (message: `the release notes are no longer current`). |

Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"4","method":"release_notes.dismiss","params":{"version":"0.9.1"}}
{"id":"4","result":{"type":"ok"}}
```

Validated 2026-09-19 against herdr 0.9.1 (against a config dir whose `release-notes.json` already named `0.9.1` current; whether the dismissal is itself persisted to disk was not exercised — a successful call left `release-notes.json`'s mtime and contents unchanged in every probe, which does not distinguish an in-memory-only effect from a write to a store not inspected here).
