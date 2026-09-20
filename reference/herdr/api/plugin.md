# herdr API: plugin methods

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `plugin.*` namespace manages herdr's plugin registry: linking a plugin directory into
**the registry** (see below — it is not scoped to the session), enabling/disabling installed
plugins, enumerating plugins and their declared actions, invoking an action, reading plugin
command logs, and driving plugin-owned terminal panes. Plugins are described by a manifest
(parsed into `InstalledPluginInfo`) that declares actions, event hooks, link handlers, panes,
build/startup commands, and target platforms. As of 0.9.1 every method below also has a
`herdr plugin` CLI equivalent (see each method's **CLI** row); `herdr plugin install`,
`herdr plugin uninstall`, and `herdr plugin config-dir` are additional CLI subcommands with
no direct `plugin.*` wire-method equivalent documented here.

The registry is **global to the herdr config root**, not scoped to the session: the store is
`<config root>/plugins.json`, guarded by a `.plugins.lock` file at the config root. A plugin
linked from one session is visible to every other session sharing that config root, and the
registry survives a server restart. Sandboxing `plugin.link`/`unlink`/`enable`/`disable`
therefore requires relocating the whole config root (e.g. via `XDG_CONFIG_HOME`), not just
using a scratch session directory.

All 11 methods below are now live-validated on herdr 0.9.1, against an isolated scratch
server with its config root relocated so the registry probes could not escape it. Coverage
is not complete everywhere: `herdr plugin install`/`uninstall` (which fetch and write a
managed checkout, and are the only path that appears to drive `[[build]]` commands),
`integration.install`/`integration.uninstall`, real link-handler activation, and
`platform_unsupported` enforcement (needs a non-Linux host) were out of scope and were not
probed; see each method's stamp for exact coverage and caveats.

11 methods:

| method | purpose |
| --- | --- |
| [plugin.action.invoke](#pluginactioninvoke) | Run a declared plugin action with an optional invocation context. |
| [plugin.action.list](#pluginactionlist) | List actions declared by installed plugins. |
| [plugin.disable](#plugindisable) | Disable an installed plugin by id. |
| [plugin.enable](#pluginenable) | Enable an installed plugin by id. |
| [plugin.link](#pluginlink) | Link a plugin directory into the registry from a filesystem path. |
| [plugin.list](#pluginlist) | List installed plugins and their manifests. |
| [plugin.log.list](#pluginloglist) | List recent plugin command-execution logs. |
| [plugin.pane.close](#pluginpaneclose) | Close a plugin-owned pane by id. |
| [plugin.pane.focus](#pluginpanefocus) | Focus a plugin-owned pane by id. |
| [plugin.pane.open](#pluginpaneopen) | Open a new plugin-owned terminal pane. |
| [plugin.unlink](#pluginunlink) | Remove a plugin from the registry by id. |

Composite response entities (`InstalledPluginInfo`, `PluginActionInfo`,
`PluginCommandLogInfo`, `PluginPaneInfo`, and their nested manifest/source types) are defined
once in [Shared plugin types](#shared-plugin-types) at the end of this file and referenced by
each method. `PaneInfo` is a cross-namespace domain entity documented in
[../data-model.md](../data-model.md).

---

## plugin.action.invoke

Executes a declared plugin action, identified by `action_id` (optionally narrowed to a single
`plugin_id`), passing an optional `PluginInvocationContext` describing the workspace, tab,
pane, selection, and/or clicked link that triggered the invocation. The action's command runs
as a plugin subprocess and its execution is recorded as a `PluginCommandLogInfo` (see
[plugin.log.list](#pluginloglist)). The invoke is **non-blocking**: the response is produced
at spawn time, with `log.status` always `"running"` and no `finished_unix_ms`, `exit_code`,
`stdout` or `stderr` yet — to learn how the command finished, poll
[plugin.log.list](#pluginloglist) for the returned `log_id`; there is no wait/completion
method. Side-effecting: spawns a process. Validated 2026-09-19 against herdr 0.9.1.

**Params** (`PluginActionInvokeParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `action_id` | string | yes | — | Id of the action to invoke (matches `PluginActionInfo.action_id`). |
| `plugin_id` | string \| null | no | null | Restrict resolution to this plugin's actions; when null the action id is resolved across all installed plugins — but if more than one installed plugin declares the same `action_id`, the omitted form fails with `ambiguous_plugin_action` (see **Errors**). |
| `context` | [PluginInvocationContext](#plugininvocationcontext-request) \| null | no | null | Invocation context supplied to the action (workspace/tab/pane/selection/link details). Accepted absent, explicit `null`, `{}`, or fully populated. |

**Result** — `type: "plugin_action_invoked"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_action_invoked"` | Result discriminator. |
| `action` | [PluginActionInfo](#pluginactioninfo) | The action that was invoked. |
| `context` | [PluginInvocationContext](#plugininvocationcontext-response) | The (possibly server-enriched) context the action ran with. If the caller omits `invocation_source` or `correlation_id`, the server fills them in as `"api"` and the request envelope's `id`, respectively. |
| `log` | [PluginCommandLogInfo](#plugincommandloginfo) | Log entry recording the command run, captured at spawn time with `status: "running"` (see above — this is never the finished entry). |

**Errors**: `plugin_not_found` (unknown `plugin_id`), `plugin_action_not_found` (unknown
`action_id`), `ambiguous_plugin_action` (`action_id` matches more than one installed plugin's
action and `plugin_id` was omitted — message: "plugin action id matches more than one action;
include plugin_id"), `plugin_disabled` (the resolved plugin is disabled — message: "plugin
\<id\> is disabled"). Other codes possible — see [errors.md](../errors.md).

**Events**: emits no event to `events.subscribe` subscribers.

**CLI**: `herdr plugin action invoke <ACTION_ID> [--plugin <ID>]`

**Example** — Validated 2026-09-19 against herdr 0.9.1.

```json
{"id":"1","method":"plugin.action.invoke","params":{"action_id":"format-buffer","plugin_id":"acme.tools","context":{"workspace_id":"ws-1","focused_pane_id":"pane-7","selected_text":"…"}}}
{"id":"1","result":{"type":"plugin_action_invoked","action":{"plugin_id":"acme.tools","action_id":"format-buffer","title":"Format buffer","command":["fmt","--stdin"],"contexts":["selection"]},"context":{"workspace_id":"ws-1","focused_pane_id":"pane-7","selected_text":"…","invocation_source":"api","correlation_id":"1"},"log":{"log_id":"log-42","plugin_id":"acme.tools","action_id":"format-buffer","command":["fmt","--stdin"],"status":"running","started_unix_ms":1755640000000}}}
```

The response above is the *entire* observable result of an invoke: `log` never carries
`finished_unix_ms`, `exit_code`, `stdout` or `stderr` at this point, regardless of how fast the
command actually finishes.

---

## plugin.action.list

Lists all actions declared by installed plugins, optionally filtered to a single `plugin_id`.
The listing does **not** filter on `enabled`: actions belonging to a disabled plugin are
still returned here, but invoking one fails with `plugin_disabled`
([plugin.action.invoke](#pluginactioninvoke)); cross-reference `plugin.list`'s `enabled`
field to know which listed actions are actually callable. Read-only. Validated 2026-09-19
against herdr 0.9.1 (both against the empty session and against a populated registry
including a disabled plugin).

**Params** (`PluginActionListParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string \| null | no | null | Restrict to actions from this plugin; when null, actions from all installed plugins are returned. |

**Result** — `type: "plugin_action_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_action_list"` | Result discriminator. |
| `actions` | array of [PluginActionInfo](#pluginactioninfo) | Declared actions (empty when no plugins/actions match), including actions of disabled plugins. |

**Errors**: none observed. Other codes possible on invalid `plugin_id` (not validated).

**CLI**: `herdr plugin action list [--plugin <ID>]`

**Example** — Validated 2026-08-19 against herdr 0.8.2 (empty-session case; re-confirmed
2026-09-19 against 0.9.1).

```json
{"id":"r4","method":"plugin.action.list","params":{}}
{"id":"r4","result":{"type":"plugin_action_list","actions":[]}}
```

---

## plugin.disable

Disables an installed plugin by id: its actions and panes become un-invokable/un-openable
(`plugin_disabled`) and its event hooks stop firing, but the registry entry is retained
(contrast [plugin.unlink](#pluginunlink), which removes it). Disabling an already-disabled
plugin (or re-enabling an already-enabled one) succeeds idempotently. Side-effecting.
Validated 2026-09-19 against herdr 0.9.1.

**Params** (`PluginSetEnabledParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string | yes | — | Id of the plugin to disable. |

**Result** — `type: "plugin_disabled"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_disabled"` | Result discriminator. |
| `plugin` | [InstalledPluginInfo](#installedplugininfo) | The updated plugin entry (with `enabled: false`). |

**Errors**: `plugin_not_found` (unknown `plugin_id`; message: "plugin not found"). Other codes
possible — see [errors.md](../errors.md).

**Events**: emits no event to `events.subscribe` subscribers.

**CLI**: `herdr plugin disable <PLUGIN_ID>`

**Example** — Validated 2026-09-19 against herdr 0.9.1.

```json
{"id":"1","method":"plugin.disable","params":{"plugin_id":"acme.tools"}}
{"id":"1","result":{"type":"plugin_disabled","plugin":{"plugin_id":"acme.tools","name":"Acme Tools","version":"1.0.0","manifest_path":"/home/u/.herdr/plugins/acme/herdr-plugin.toml","plugin_root":"/home/u/.herdr/plugins/acme","enabled":false}}}
```

---

## plugin.enable

Enables an installed plugin by id, re-activating its actions, event hooks, link handlers, and
panes. **Startup commands do not run on enable**: enabling a plugin produced no new
`plugin_command_log` entry for its `[[startup]]` commands, while restarting the server with
the same plugin already enabled in the registry did (logged with `event: "startup"`; see
[plugin.log.list](#pluginloglist)) — startup commands run once per enabled plugin at server
boot, not on this call. Side-effecting. Validated 2026-09-19 against herdr 0.9.1.

**Params** (`PluginSetEnabledParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string | yes | — | Id of the plugin to enable. |

**Result** — `type: "plugin_enabled"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_enabled"` | Result discriminator. |
| `plugin` | [InstalledPluginInfo](#installedplugininfo) | The updated plugin entry (with `enabled: true`). |

**Errors**: `plugin_not_found` (unknown `plugin_id`; message: "plugin not found"). Other codes
possible — see [errors.md](../errors.md).

**Events**: emits no event to `events.subscribe` subscribers.

**CLI**: `herdr plugin enable <PLUGIN_ID>`

**Example** — Validated 2026-09-19 against herdr 0.9.1.

```json
{"id":"1","method":"plugin.enable","params":{"plugin_id":"acme.tools"}}
{"id":"1","result":{"type":"plugin_enabled","plugin":{"plugin_id":"acme.tools","name":"Acme Tools","version":"1.0.0","manifest_path":"/home/u/.herdr/plugins/acme/herdr-plugin.toml","plugin_root":"/home/u/.herdr/plugins/acme","enabled":true}}}
```

---

## plugin.link

Links a plugin directory into the registry from a filesystem `path`, parsing its manifest into
an `InstalledPluginInfo`. The plugin is enabled by default. An optional `source` records
provenance (local vs. GitHub, repo/owner/ref/commit) — a GitHub source additionally
**requires** `managed_path` (see [PluginSourceInfo](#pluginsourceinfo)). The manifest's
`min_herdr_version` is mandatory (see [InstalledPluginInfo](#installedplugininfo)); a value
newer than the running herdr is refused outright with `plugin_requires_newer_herdr`, not
recorded as a warning. Non-fatal manifest problems — an undeclared `platforms` field, or an
event hook naming an unknown wire event — are instead collected into the entry's `warnings`
and the plugin still links; `platforms: []` (an explicit empty array) is rejected with
`invalid_plugin_platform`. Re-linking an already-linked `plugin_id` silently **replaces** the
existing entry — there is no "already linked" error. Side-effecting: mutates the registry.
Validated 2026-09-19 against herdr 0.9.1.

**Params** (`PluginLinkParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `path` | string | yes | — | Filesystem path to the plugin directory (or manifest) to link. |
| `enabled` | boolean | no | `true` | Whether the linked plugin is enabled immediately. |
| `source` | [PluginSourceInfo](#pluginsourceinfo) \| null | no | null | Provenance metadata recorded for the plugin (defaults to local kind when the object is present). A `"github"` `kind` without `managed_path` is rejected. |

**Result** — `type: "plugin_linked"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_linked"` | Result discriminator. |
| `plugin` | [InstalledPluginInfo](#installedplugininfo) | The newly linked plugin entry. Collection fields (`actions`, `build`, `startup`, `events`, `link_handlers`, `panes`, `warnings`) that end up empty are omitted from the JSON entirely, not sent as `[]`. |

**Errors**: `plugin_manifest_not_found` (bad `path`; message carries the underlying I/O error,
e.g. "No such file or directory (os error 2)"), `invalid_plugin_min_herdr_version` (manifest
omits `min_herdr_version`), `plugin_requires_newer_herdr` (manifest's `min_herdr_version`
exceeds the running herdr version), `invalid_plugin_platform` (`platforms` is an explicit
empty array), `invalid_plugin_source` (GitHub `source` without `managed_path`). Other codes
possible for other malformed manifests — see [errors.md](../errors.md).

**Events**: emits no event to `events.subscribe` subscribers.

**CLI**: `herdr plugin link <PATH> [--enabled | --disabled]`

**Example** — Validated 2026-09-19 against herdr 0.9.1.

```json
{"id":"1","method":"plugin.link","params":{"path":"/home/u/dev/acme-plugin/herdr-plugin.toml"}}
{"id":"1","result":{"type":"plugin_linked","plugin":{"plugin_id":"acme.tools","name":"Acme Tools","version":"1.0.0","min_herdr_version":"1.0.0","manifest_path":"/home/u/dev/acme-plugin/herdr-plugin.toml","plugin_root":"/home/u/dev/acme-plugin","enabled":true,"actions":[{"id":"format-buffer","title":"Format buffer","command":["fmt","--stdin"]}],"source":{"kind":"local"},"warnings":["manifest does not declare platforms; platform support unknown"]}}}
```

This manifest didn't declare `platforms`, hence the standing warning; `build`, `startup`,
`events`, and `link_handlers` are all absent because the manifest declared none of them, not
sent as `[]`. A GitHub source needs `managed_path` to be accepted:

```json
{"id":"2","method":"plugin.link","params":{"path":"/home/u/dev/acme-plugin","source":{"kind":"github","owner":"acme","repo":"herdr-acme","requested_ref":"v1.0.0"}}}
{"id":"2","error":{"code":"invalid_plugin_source","message":"GitHub plugin source requires managed_path"}}
```

---

## plugin.list

Lists installed plugins and their parsed manifests, optionally filtered to a single
`plugin_id`. Read-only. Validated 2026-09-19 against herdr 0.9.1, against both the empty
session (no plugins → empty `plugins`) and a populated registry (linked, disabled,
warning-carrying, and platform-restricted plugins).

**Params** (`PluginListParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string \| null | no | null | Restrict to this plugin; when null, all installed plugins are returned. |

**Result** — `type: "plugin_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_list"` | Result discriminator. |
| `plugins` | array of [InstalledPluginInfo](#installedplugininfo) | Installed plugin entries (empty when none installed). |

**Errors**: `invalid_request` for a non-string `plugin_id` (e.g. an integer). No error observed
for an unknown `plugin_id` — an empty `plugins` array is returned instead. Other codes
possible — see [errors.md](../errors.md).

**CLI**: `herdr plugin list [--plugin <ID>] [--json]`

**Example** — Validated 2026-09-17 against herdr 0.9.1 (empty registry; re-confirmed 2026-09-19
against 0.9.1).

```json
{"id":"r3","method":"plugin.list","params":{}}
{"id":"r3","result":{"type":"plugin_list","plugins":[]}}
```

---

## plugin.log.list

Lists recent plugin command-execution log entries (action invocations and event-hook runs are
both logged live; startup commands are logged once per enabled plugin at server boot — see
[plugin.enable](#pluginenable) — but build commands were never observed to be logged at all,
even across link/enable/restart, and appear to be driven only by `herdr plugin install`),
**oldest-first**, optionally filtered by `plugin_id` and capped by `limit`. The in-memory log
store holds at most 50 entries — after that, the oldest are evicted — and `log_id` restarts
from `plugin-log-1` after a server restart; anything evicted or from a previous server run is
unrecoverable. Read-only. Validated 2026-09-19 against herdr 0.9.1.

**Params** (`PluginLogListParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string \| null | no | null | Restrict to logs from this plugin; when null, logs from all plugins are returned. |
| `limit` | integer (uint, ≥ 0) \| null | no | null | Selects the last N entries (still returned oldest-first), then applied on top of the 50-entry cap; `limit: 0` returns **one** entry, not zero — the effective count is `max(limit, 1)`. A negative value fails `invalid_request` ("expected usize"); values up to `u64::MAX` are accepted, larger ones are rejected as a non-integer float. |

**Result** — `type: "plugin_log_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_log_list"` | Result discriminator. |
| `logs` | array of [PluginCommandLogInfo](#plugincommandloginfo) | Command-execution log entries, oldest-first. |

**Errors**: `invalid_request` for a negative or non-integer `limit`. Other codes possible — see
[errors.md](../errors.md).

**CLI**: `herdr plugin log list [--plugin <ID>] [--limit <N>]`

**Example** — Validated 2026-09-19 against herdr 0.9.1.

```json
{"id":"1","method":"plugin.log.list","params":{"plugin_id":"acme.tools","limit":2}}
{"id":"1","result":{"type":"plugin_log_list","logs":[{"log_id":"log-41","plugin_id":"acme.tools","action_id":"format-buffer","command":["fmt","--stdin"],"status":"failed","started_unix_ms":1755640000000,"finished_unix_ms":1755640000003,"exit_code":3,"stdout":"out\n","stderr":"err\n"},{"log_id":"log-42","plugin_id":"acme.tools","action_id":"format-buffer","command":["fmt","--stdin"],"status":"succeeded","started_unix_ms":1755640000004,"finished_unix_ms":1755640000120,"exit_code":0,"stdout":"…","stderr":""}]}}
```

`limit: 2` returns the last two entries, oldest of the two first. `stdout`/`stderr` are empty
strings, not `null`, when the command produced no output; `error` (spawn failure) and `event`
(event-hook name, in place of `action_id`) are both omitted, not `null`, when not applicable —
consistent with every other optional field on this page (see
[InstalledPluginInfo](#installedplugininfo)).

---

## plugin.pane.close

Closes a plugin-owned terminal pane by id. This method is strictly for panes this plugin API
opened: a real but non-plugin pane, an unknown id, and an already-closed plugin pane all fail
the same way (see **Errors**) — use [pane.close](pane.md#paneclose) for ordinary panes.
Side-effecting: destroys the pane. Validated 2026-09-19 against herdr 0.9.1.

**Params** (`PluginPaneCloseParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `pane_id` | string | yes | — | Id of the plugin pane to close (matches `PluginPaneInfo.pane.pane_id`). |

**Result** — `type: "plugin_pane_closed"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_pane_closed"` | Result discriminator. |
| `pane_id` | string | Id of the closed pane. |

**Errors**: `plugin_pane_not_found` (message: "plugin pane not found") — for an unknown
`pane_id`, for a real pane that is not plugin-owned, and for a plugin pane that is already
closed. Other codes possible — see [errors.md](../errors.md).

**Events**: emits `pane_closed` and `layout.updated` to subscribers.

**CLI**: `herdr plugin pane close <PANE_ID>`

**Example** — Validated 2026-09-19 against herdr 0.9.1.

```json
{"id":"1","method":"plugin.pane.close","params":{"pane_id":"pane-9"}}
{"id":"1","result":{"type":"plugin_pane_closed","pane_id":"pane-9"}}
```

---

## plugin.pane.focus

Focuses an existing plugin-owned terminal pane by id, bringing it forward. Like
[plugin.pane.close](#pluginpaneclose), this is strictly for plugin-owned panes: a real but
non-plugin pane fails the same way as an unknown id. Side-effecting: changes UI focus.
Validated 2026-09-19 against herdr 0.9.1.

**Params** (`PluginPaneFocusParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `pane_id` | string | yes | — | Id of the plugin pane to focus. |

**Result** — `type: "plugin_pane_focused"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_pane_focused"` | Result discriminator. |
| `plugin_pane` | [PluginPaneInfo](#pluginpaneinfo) | The focused plugin pane. |

**Errors**: `plugin_pane_not_found` (message: "plugin pane not found") — for an unknown
`pane_id` and for a real pane that is not plugin-owned. Other codes possible — see
[errors.md](../errors.md).

**Events**: emits `workspace_focused`, `tab_focused`, and `pane_focused` to subscribers.

**CLI**: `herdr plugin pane focus <PANE_ID>`

**Example** — Validated 2026-09-19 against herdr 0.9.1.

```json
{"id":"1","method":"plugin.pane.focus","params":{"pane_id":"pane-9"}}
{"id":"1","result":{"type":"plugin_pane_focused","plugin_pane":{"plugin_id":"acme.tools","entrypoint":"dashboard","pane":{"pane_id":"pane-9","terminal_id":"term-3","workspace_id":"ws-1","tab_id":"tab-2","focused":true,"agent_status":"unknown","revision":5}}}}
```

---

## plugin.pane.open

Opens a new plugin-owned terminal pane running the plugin's `entrypoint` command, placed
according to `placement` (overlay/popup/split/tab/zoomed), with a working directory that
**defaults to the plugin's `plugin_root`** (not the caller's or server's cwd) and a `label`
taken from the manifest pane's `title`. `placement: "tab"` opens the pane in a **new tab**,
not the current one. This method requires a foreground UI client attached to the session (a
headless `herdr server` with no TUI attached can never satisfy it — see **Errors**). Optionally
focuses the new pane. Side-effecting: spawns a process and creates a pane. Validated
2026-09-19 against herdr 0.9.1 (popup's pane body could not be inspected — it never appears in
`pane.list` — so only its documented result shape and its errors were exercised).

**Params** (`PluginPaneOpenParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string | yes | — | Id of the plugin that owns the pane. |
| `entrypoint` | string | yes | — | Plugin pane entrypoint (identifies the manifest pane / command to run). An unknown entrypoint fails `plugin_pane_not_found`, not a validation error. |
| `placement` | [PluginPanePlacement](#pluginpaneplacement) \| null | no | null | Placement of the pane; enum `overlay`, `popup`, `split`, `tab`, `zoomed` (manifest default is `overlay` when unset). |
| `direction` | [SplitDirection](#splitdirection) \| null | no | null | Split direction; enum `right`, `down`. Accepted only when `placement` is `split`; rejected with `invalid_params` for `tab` ("tab plugin panes support workspace_id but not target_pane_id or direction"). |
| `width` | [PopupSize](#popupsize) \| null | no | null | Outer width, in terminal cells (integer 0–65535) or a percentage string like `"80%"`. **Accepted only when `placement` is `popup`** — any other placement, including the omitted/default case, fails `invalid_params` ("width and height are only supported when placement is popup"). |
| `height` | [PopupSize](#popupsize) \| null | no | null | Outer height, terminal cells or a percentage string. Same popup-only restriction as `width`. |
| `cwd` | string \| null | no | null | Working directory for the pane process; defaults to the plugin's `plugin_root` when omitted, not the caller's or server's cwd. |
| `env` | object (string → string) | no | `{}` | Environment variable overrides for the pane process. |
| `target_pane_id` | string \| null | no | null | Existing pane to anchor placement against. **Placement-restricted**: accepted only for `split` and `zoomed`; `overlay`/`popup` reject it with `invalid_params` ("overlay and popup plugin panes target the active pane"); `tab` rejects it with the same message as `direction` above. |
| `workspace_id` | string \| null | no | null | Workspace to open the pane in; when null the current/target workspace is used. |
| `focus` | boolean | no | `false` | Whether to focus the new pane after opening. |

**Result**: for placement `overlay`, `split`, `tab`, or `zoomed`, `type: "plugin_pane_opened"`
with a `plugin_pane`. For placement **`popup`, the result is a bare `{"type":"ok"}`** with no
`plugin_pane` at all — and the opened popup never appears in `pane.list`, so a caller has no
`pane_id` to later pass to `plugin.pane.focus`/`plugin.pane.close`; a second `popup` open while
one is already showing fails with `ui_busy` ("a popup pane is already open"), and the only way
to dismiss it is [popup.close](ui.md#popupclose) (not `plugin.pane.close`).

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_pane_opened"` | Result discriminator (non-popup placements only; popup returns `"ok"` instead — see above). |
| `plugin_pane` | [PluginPaneInfo](#pluginpaneinfo) | The newly opened plugin pane (absent for popup). |

**Errors**: `plugin_not_found` (unknown `plugin_id`), `plugin_disabled` (the resolved plugin is
disabled), `plugin_pane_not_found` (unknown `entrypoint`, message: "plugin pane entrypoint
'\<id\>' not found"), `pane_not_found` (unknown `target_pane_id`), `workspace_not_found`
(unknown `workspace_id`), `invalid_params` (placement-restricted `width`/`height`/
`direction`/`target_pane_id` misuse — see the params above), `ui_busy` (a popup is already
open). With no foreground UI client attached, every placement fails instead with one of three
"no active …" codes: `plugin_pane_open_failed` ("no active workspace") for `overlay`/`popup`,
`no_active_workspace` for `tab`, and `no_active_pane` for `split`/`zoomed`. Other codes
possible — see [errors.md](../errors.md).

**Events**: for placements other than `popup`, emits `pane.created` and `layout.updated` to
subscribers. A `popup` open emits no event.

**CLI**: `herdr plugin pane open --plugin <ID> --entrypoint <ID> [--placement <overlay|split|tab|zoomed>] [--workspace <ID>] [--target-pane <PANE>] [--direction <right|down>] [--cwd <PATH>] [--env <KEY=VALUE>]... [--focus | --no-focus]` (the CLI's `--placement` omits `popup`, which remains valid on the wire).

**Example** — Validated 2026-09-19 against herdr 0.9.1.

```json
{"id":"1","method":"plugin.pane.open","params":{"plugin_id":"acme.tools","entrypoint":"dashboard","placement":"split","direction":"down","target_pane_id":"w1:p1","focus":true}}
{"id":"1","result":{"type":"plugin_pane_opened","plugin_pane":{"plugin_id":"acme.tools","entrypoint":"dashboard","pane":{"pane_id":"w1:p2","terminal_id":"term_65bdd5782da1a5","workspace_id":"w1","tab_id":"w1:t1","focused":true,"cwd":"/home/u/dev/acme-plugin","foreground_cwd":"/home/u/dev/acme-plugin","label":"Dashboard","agent_status":"unknown","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":0,"viewport_rows":49},"revision":0}}}}
```

`cwd`/`foreground_cwd` default to the plugin's `plugin_root` (`/home/u/dev/acme-plugin`)
because the request above omits `cwd`. A `popup` open instead answers like this:

```json
{"id":"2","method":"plugin.pane.open","params":{"plugin_id":"acme.tools","entrypoint":"dashboard","placement":"popup","width":"40%","height":12}}
{"id":"2","result":{"type":"ok"}}
```

---

## plugin.unlink

Removes a plugin from the registry by id, undoing a [plugin.link](#pluginlink). The result's
`removed` flag reports whether an entry was actually present and removed. Side-effecting.
Validated 2026-09-19 against herdr 0.9.1.

**Params** (`PluginUnlinkParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string | yes | — | Id of the plugin to unlink. |

**Result** — `type: "plugin_unlinked"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_unlinked"` | Result discriminator. |
| `plugin_id` | string | Id that was requested for removal. |
| `removed` | boolean | Whether a matching entry existed and was removed. |

**Errors**: none observed; a missing plugin returns `removed: false` rather than an error, and
unlinking the same `plugin_id` twice returns `true` then `false`. Other codes possible — see
[errors.md](../errors.md).

**Events**: emits no event to `events.subscribe` subscribers.

**CLI**: `herdr plugin unlink <PLUGIN_ID>`

**Example** — Validated 2026-09-19 against herdr 0.9.1.

```json
{"id":"1","method":"plugin.unlink","params":{"plugin_id":"acme.tools"}}
{"id":"1","result":{"type":"plugin_unlinked","plugin_id":"acme.tools","removed":true}}
```

---

## Shared plugin types

Composite entities referenced by the methods above. `PaneInfo` is documented in
[../data-model.md](../data-model.md); the rest are plugin-namespace-specific and defined here.
Validated 2026-09-19 against herdr 0.9.1: on the wire, an absent/null optional field
(`description`, `warnings`, and every declaration array below) is **omitted from the JSON
object entirely**, never sent as `null` or `[]` — read a missing key the same as its default.

### InstalledPluginInfo

A linked plugin entry with its parsed manifest.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string | yes | — | Stable plugin identifier. |
| `name` | string | yes | — | Human-readable plugin name. |
| `version` | string | yes | — | Plugin version string. |
| `manifest_path` | string | yes | — | Path to the plugin manifest file. |
| `plugin_root` | string | yes | — | Root directory of the plugin. |
| `enabled` | boolean | yes | — | Whether the plugin is currently enabled. |
| `description` | string \| null | no | null | Plugin description. |
| `min_herdr_version` | string | no | `""` | Minimum required herdr version. The schema marks this optional with an empty-string default, but the running server treats it as **mandatory in the manifest**: omitting it fails the link with `invalid_plugin_min_herdr_version` ("plugin min_herdr_version is required") — there is no observed path to the schema's `""` default. Must be semver; a value newer than the running herdr fails the link outright with `plugin_requires_newer_herdr`, e.g. "plugin requires Herdr 99.0.0 or newer; current Herdr is 0.9.1". |
| `source` | [PluginSourceInfo](#pluginsourceinfo) | no | `{"kind":"local"}` | Provenance metadata. |
| `platforms` | array of [PluginPlatform](#pluginplatform) \| null | no | null | Platforms the plugin targets; null/absent means unrestricted, but an undeclared manifest `platforms` is not silent — it adds a standing warning ("manifest does not declare platforms; platform support unknown") to `warnings` on every link and every `plugin.list`. An explicit empty array is rejected with `invalid_plugin_platform` ("platforms must not be an empty array; omit the field to leave platforms undeclared"). |
| `actions` | array of [PluginManifestAction](#pluginmanifestaction) | no | `[]` | Declared actions. Defaults to `[]` on input (the manifest); an empty result is omitted from the response, not sent as `[]`. |
| `build` | array of [PluginManifestBuild](#pluginmanifestbuild) | no | `[]` | Build commands. Same omit-when-empty rule as `actions`. Not observed to run from `plugin.link`, `plugin.enable`, or server startup — only `herdr plugin install` appears to drive `[[build]]`, and that CLI path was out of scope for this probe. |
| `startup` | array of [PluginManifestStartup](#pluginmanifeststartup) | no | `[]` | Startup commands. Same omit-when-empty rule as `actions`. Run once per already-enabled plugin at **server boot** (logged with `event: "startup"`), not on `plugin.link` or `plugin.enable` — see [plugin.enable](#pluginenable). |
| `events` | array of [PluginManifestEventHook](#pluginmanifesteventhook) | no | `[]` | Event hooks. Same omit-when-empty rule as `actions`. |
| `link_handlers` | array of [PluginManifestLinkHandler](#pluginmanifestlinkhandler) | no | `[]` | Link handlers. Same omit-when-empty rule as `actions`. |
| `panes` | array of [PluginManifestPane](#pluginmanifestpane) | no | `[]` | Declared panes. Same omit-when-empty rule as `actions`. |
| `warnings` | array of string | no | `[]` | Non-fatal warnings collected at link time or on registry load (e.g. an unknown event name, an undeclared `platforms`); the entry is kept and surfaced by `plugin.list`. Omitted, not sent as `[]`, when there are none. |

### PluginSourceInfo

Provenance of a plugin. Used both as a request field (on `plugin.link`) and a response field.
As a request field it is validated more tightly than the field list below implies: `kind:
"github"` additionally **requires** `managed_path` — omitting it fails `plugin.link` with
`invalid_plugin_source` ("GitHub plugin source requires managed_path"). Validated 2026-09-19
against herdr 0.9.1 (only the `managed_path` requirement itself was exercised; the further
constraints a schema reader might expect around a managed checkout's own path layout were not
probed).

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `kind` | [PluginSourceKind](#pluginsourcekind) | no | `"local"` | Source kind; enum `local`, `github`. |
| `owner` | string \| null | no | null | Repository owner (GitHub sources). |
| `repo` | string \| null | no | null | Repository name (GitHub sources). |
| `subdir` | string \| null | no | null | Subdirectory within the repo. |
| `requested_ref` | string \| null | no | null | Requested git ref (branch/tag). |
| `resolved_commit` | string \| null | no | null | Resolved commit SHA. |
| `managed_path` | string \| null | no | null | Managed on-disk path for fetched sources; **required** when `kind` is `"github"`. |
| `installed_unix_ms` | integer (uint64, ≥ 0) \| null | no | null | Install timestamp in Unix milliseconds. |

### PluginSourceKind

Enum: `local`, `github`.

### PluginPlatform

Enum: `linux`, `macos`, `windows`.

### PluginActionContext

Enum: `global`, `workspace`, `tab`, `pane`, `selection`. Where an action may be surfaced/invoked.

### PluginPanePlacement

Enum: `overlay`, `popup`, `split`, `tab`, `zoomed`. How a plugin pane is placed.

### SplitDirection

Enum: `right`, `down`. Split direction when a plugin pane placement splits.

### PopupSize

A `oneOf`: either an integer (terminal cells, 0–65535, including the border) or a percentage
string matching `^(100|[1-9][0-9]?)%$` (e.g. `"80%"`) of the terminal area.

### PluginManifestAction

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `id` | string | yes | — | Action id. |
| `title` | string | yes | — | Display title. |
| `command` | array of string | yes | — | Command argv to run. |
| `contexts` | array of [PluginActionContext](#pluginactioncontext) | no | `[]` | Contexts the action applies to. |
| `description` | string \| null | no | null | Action description. |
| `platforms` | array of [PluginPlatform](#pluginplatform) \| null | no | null | Platforms this action targets. |

### PluginManifestBuild

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `command` | array of string | yes | — | Build command argv. |
| `platforms` | array of [PluginPlatform](#pluginplatform) \| null | no | null | Platforms this build applies to. |

### PluginManifestStartup

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `command` | array of string | yes | — | Startup command argv. |
| `platforms` | array of [PluginPlatform](#pluginplatform) \| null | no | null | Platforms this startup command applies to. |

### PluginManifestEventHook

`on` must use the **dotted wire event name** used by
[events.subscribe](../protocol.md#subscription-connections), e.g. `"pane.focused"`. There are
no `plugin.*` event types: the manifest hook names a herdr event to react to. The underscored
form (`"pane_focused"`) links successfully but with a warning ("unknown event 'pane_focused'")
and then never fires. A matching hook run is logged via [plugin.log.list](#pluginloglist) with
`event` set and `action_id` absent, and hooks stop firing while the plugin is disabled.
Validated 2026-09-19 against herdr 0.9.1.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `on` | string | yes | — | Event name the hook fires on; must be a dotted wire event name (see above) to actually fire. |
| `command` | array of string | yes | — | Command argv to run on the event. |
| `platforms` | array of [PluginPlatform](#pluginplatform) \| null | no | null | Platforms this hook applies to. |

### PluginManifestLinkHandler

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `id` | string | yes | — | Link handler id. |
| `title` | string | yes | — | Display title. |
| `pattern` | string | yes | — | URL/link pattern the handler matches. |
| `action` | string | yes | — | Action id invoked when a matching link is activated. |
| `platforms` | array of [PluginPlatform](#pluginplatform) \| null | no | null | Platforms this handler applies to. |

### PluginManifestPane

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `id` | string | yes | — | Pane id. |
| `title` | string | yes | — | Display title. |
| `command` | array of string | yes | — | Command argv to run in the pane. |
| `description` | string \| null | no | null | Pane description. |
| `placement` | [PluginPanePlacement](#pluginpaneplacement) | no | `"overlay"` | Default placement. |
| `width` | [PopupSize](#popupsize) \| null | no | null | Default width. |
| `height` | [PopupSize](#popupsize) \| null | no | null | Default height. |
| `platforms` | array of [PluginPlatform](#pluginplatform) \| null | no | null | Platforms this pane applies to. |

### PluginActionInfo

A resolved action returned by `plugin.action.list` and `plugin.action.invoke`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string | yes | — | Owning plugin id. |
| `action_id` | string | yes | — | Action id. |
| `title` | string | yes | — | Display title. |
| `command` | array of string | yes | — | Command argv. |
| `contexts` | array of [PluginActionContext](#pluginactioncontext) | no | `[]` | Contexts the action applies to. |
| `description` | string \| null | no | null | Action description. |
| `platforms` | array of [PluginPlatform](#pluginplatform) \| null | no | null | Platforms this action targets. |

### PluginCommandLogInfo

A record of one plugin command execution.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `log_id` | string | yes | — | Log entry id. |
| `plugin_id` | string | yes | — | Owning plugin id. |
| `command` | array of string | yes | — | Command argv that was run. |
| `status` | [PluginCommandStatus](#plugincommandstatus) | yes | — | Execution status; enum `running`, `succeeded`, `failed`. |
| `started_unix_ms` | integer (uint64, ≥ 0) | yes | — | Start time in Unix milliseconds. |
| `action_id` | string \| null | no | null | Action id, when the command was an action invocation. |
| `event` | string \| null | no | null | Event name, when the command was an event-hook run. |
| `finished_unix_ms` | integer (uint64, ≥ 0) \| null | no | null | Finish time in Unix milliseconds (null while running). |
| `exit_code` | integer (int32) \| null | no | null | Process exit code (null while running / on spawn failure). |
| `stdout` | string \| null | no | null | Captured standard output. |
| `stderr` | string \| null | no | null | Captured standard error. |
| `error` | string \| null | no | null | Error message when the command failed to run. |

### PluginCommandStatus

Enum: `running`, `succeeded`, `failed`.

### PluginPaneInfo

A plugin-owned pane.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string | yes | — | Owning plugin id. |
| `entrypoint` | string | yes | — | Pane entrypoint that was launched. |
| `pane` | [PaneInfo](../data-model.md) | yes | — | The underlying pane (cross-namespace domain entity). |

### PluginInvocationContext (request)

Optional context supplied on `plugin.action.invoke`. Every field is optional and nullable.

| field | type | meaning |
| --- | --- | --- |
| `workspace_id` | string \| null | Triggering workspace id. |
| `workspace_label` | string \| null | Triggering workspace label. |
| `workspace_cwd` | string \| null | Triggering workspace working directory. |
| `worktree` | [WorkspaceWorktreeInfo](#workspaceworktreeinfo) \| null | Worktree metadata for the workspace. |
| `tab_id` | string \| null | Triggering tab id. |
| `tab_label` | string \| null | Triggering tab label. |
| `focused_pane_id` | string \| null | Focused pane id at invocation. |
| `focused_pane_cwd` | string \| null | Focused pane working directory. |
| `focused_pane_agent` | string \| null | Agent bound to the focused pane. |
| `focused_pane_status` | [AgentStatus](#agentstatus) \| null | Focused pane agent status; enum `idle`, `working`, `blocked`, `done`, `unknown`. |
| `selected_text` | string \| null | Text selected when invoked. |
| `clicked_url` | string \| null | URL clicked to trigger a link handler. |
| `link_handler_id` | string \| null | Link handler id that matched. |
| `invocation_source` | string \| null | Where the invocation originated. |
| `correlation_id` | string \| null | Caller-supplied correlation id. |

### PluginInvocationContext (response)

Returned on `plugin.action.invoke`. Structurally identical to the request form above (all
fields optional/nullable); the server may enrich fields the caller left null.

### WorkspaceWorktreeInfo

Git worktree metadata for a workspace, embedded in `PluginInvocationContext`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `repo_key` | string | yes | — | Stable key identifying the repository. |
| `repo_name` | string | yes | — | Repository name. |
| `repo_root` | string | yes | — | Repository root path. |
| `checkout_path` | string | yes | — | Checkout path of this worktree. |
| `is_linked_worktree` | boolean | yes | — | Whether this is a linked (secondary) git worktree. |

### AgentStatus

Enum: `idle`, `working`, `blocked`, `done`, `unknown`.
