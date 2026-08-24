# herdr API: ui methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `ui` namespace drives client-facing surface elements: the foreground client's window title, desktop/toast notifications, and the client popup overlay. These operations act on whichever client is currently in the foreground; several return a boolean plus a `reason` enum reporting whether the action took effect, because the target surface may be absent (no foreground client), disabled, or otherwise unavailable rather than failing outright.

4 methods:

| method | purpose |
| --- | --- |
| [client.window_title.clear](#clientwindow_titleclear) | Restore the foreground client's window title to its default. |
| [client.window_title.set](#clientwindow_titleset) | Set the foreground client's window title. |
| [notification.show](#notificationshow) | Show a desktop/toast notification through the foreground client. |
| [popup.close](#popupclose) | Close the currently open client popup overlay. |

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
