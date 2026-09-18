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
| _(none)_ | — | — | — | No parameters. `params` must be present and be `{}`. |

**Result**: `type` const `client_window_title`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `client_window_title`. |
| changed | boolean | yes | — | `true` if the title was actually cleared; `false` if there was nothing to clear or no foreground client. |
| reason | string (enum) | yes | — | Outcome detail. One of: `set`, `cleared`, `no_foreground_client`. For this method the effective values are `cleared` (title reset) and `no_foreground_client` (no client to act on). |

**Errors**: none evidenced; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"1","method":"client.window_title.clear","params":{}}
{"id":"1","result":{"type":"client_window_title","changed":true,"reason":"cleared"}}
```

Constructed from schema; not live-validated.

## client.window_title.set

Overrides the foreground client's window title with the supplied string. Acts on the foreground client only; if no client is in the foreground the request succeeds but reports that nothing changed.

**Params**: `ClientWindowTitleSetParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| title | string | yes | — | The window title to display for the foreground client. |

**Result**: `type` const `client_window_title`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `client_window_title`. |
| changed | boolean | yes | — | `true` if the title was applied; `false` if it could not be (e.g. no foreground client). |
| reason | string (enum) | yes | — | Outcome detail. One of: `set`, `cleared`, `no_foreground_client`. For this method the effective values are `set` (title applied) and `no_foreground_client` (no client to act on). |

**Errors**: none evidenced; other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"r6","method":"client.window_title.set","params":{"title":"doc-probe"}}
{"id":"r6","result":{"type":"client_window_title","changed":false,"reason":"no_foreground_client"}}
```

Validated 2026-08-19 against herdr 0.8.2. (Probe ran with no foreground client, so `changed` is `false` and `reason` is `no_foreground_client`.)

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

Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"2","method":"client_shell.surface.set","params":{"active":true}}
{"id":"2","error":{"code":"connection_local_only","message":"client_shell.surface.set is only available through a client shell endpoint"}}
```

Validated 2026-09-17 against herdr 0.9.1. (Probe ran over a plain API connection, not a client-shell endpoint, so the request returned `connection_local_only`. A client-shell endpoint call instead returns `{"type":"client_shell_surface_set","active":...,"projection_revision":...}`.)

## command.invoke

Invokes a command surfaced to the requesting client-shell connection through the client-shell projection. `command_id` is an opaque, endpoint-issued identifier obtained from that projection, not a fixed enum chosen by the caller. `pane_id`, `tab_id`, and `workspace_id` optionally scope the invocation to a specific target, and `selection` optionally carries client-owned selection coordinates, validated against the pane's content revision (see [pane.md](pane.md)).

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
| command_not_found | The `command_id` is not currently valid, e.g. the client-shell's custom command manifest is stale (message observed: `custom command manifest is stale; reload configuration`). |

Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example**:

```json
{"id":"5","method":"command.invoke","params":{"command_id":"probe.command"}}
{"id":"5","error":{"code":"command_not_found","message":"custom command manifest is stale; reload configuration"}}
```

Validated 2026-09-17 against herdr 0.9.1. (Probe used a synthetic `command_id` with no client-shell projection behind it, so the request returned `command_not_found`. A valid, endpoint-issued command ID instead returns `{"type":"ok"}`.)

## notification.show

Shows a desktop/toast notification via the foreground client. Delivery is best-effort: notifications may be suppressed when the client has notifications disabled, when the client is rate-limited, when it is busy, or when there is no foreground client. The result's `shown` flag and `reason` enum report the actual outcome; a suppressed notification is still a successful request (not an error).

**Params**: `NotificationShowParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| title | string | yes | — | Notification title (primary line). |
| body | string \| null | no | null | Optional notification body text. |
| position | ToastHerdrPosition \| null | no | null | Optional on-screen corner for the toast. One of: `top-left`, `top-right`, `bottom-left`, `bottom-right`. `null` uses the client default. |
| sound | NotificationShowSound (enum) | no | — | Sound to play. One of: `none`, `done`, `request`. |

**Result**: `type` const `notification_show`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `notification_show`. |
| shown | boolean | yes | — | `true` if the notification was actually presented; `false` if suppressed. |
| reason | string (enum) | yes | — | Outcome detail. One of: `shown` (delivered), `disabled` (client has notifications turned off), `rate_limited` (too many notifications recently), `no_foreground_client` (no client to deliver to), `busy` (client not accepting notifications right now). |

**Errors**: none evidenced; other codes possible.

**CLI**: `herdr notification show <TITLE> [--body <TEXT>] [--position <top-left|top-right|bottom-left|bottom-right>] [--sound <none|done|request>]`

**Example**:

```json
{"id":"cli:notification:show","method":"notification.show","params":{"title":"probe"}}
{"id":"cli:notification:show","result":{"type":"notification_show","shown":false,"reason":"disabled"}}
```

Validated 2026-08-19 against herdr 0.8.2. (Probe target had notifications disabled, so `shown` is `false` and `reason` is `disabled`.)

## popup.close

Closes the client popup overlay if one is currently open. If no popup is open, the request fails with `popup_not_open` rather than succeeding as a no-op.

**Params**: `EmptyParams` — an empty object. Send `{}`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| _(none)_ | — | — | — | No parameters. `params` must be present and be `{}`. |

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

Validated 2026-08-19 against herdr 0.8.2. (Probe ran with no popup open, so the request returned the `popup_not_open` error. A successful close returns `{"type":"ok"}`.)

## product_announcement.dismiss

Records that the caller has dismissed a product announcement, identified by `id` and `version`. If the announcement named is no longer the current one the server has queued, the request fails with `stale_announcement` rather than silently succeeding.

**Params**: `ProductAnnouncementDismissParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| id | string | yes | — | Identifier of the announcement being dismissed. |
| version | string | yes | — | Version of the announcement being dismissed. |

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

Validated 2026-09-17 against herdr 0.9.1. (Probe target had no matching current announcement, so the request returned `stale_announcement`. A dismissal matching the current announcement instead returns `{"type":"ok"}`.)

## release_notes.dismiss

Records that the caller has dismissed the release notes for `version`. If `version` does not match the release notes the server currently considers current, the request fails with `stale_release_notes`.

**Params**: `ReleaseNotesDismissParams`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| version | string | yes | — | Version of the release notes being dismissed. |

**Result**: `type` const `ok`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| type | string | yes | — | Always `ok`. |

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

Validated 2026-09-17 against herdr 0.9.1.
