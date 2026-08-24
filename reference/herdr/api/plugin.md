# herdr API: plugin methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `plugin.*` namespace manages herdr's plugin registry: linking a plugin directory into
the session, enabling/disabling installed plugins, enumerating plugins and their declared
actions, invoking an action, reading plugin command logs, and driving plugin-owned terminal
panes. Plugins are described by a manifest (parsed into `InstalledPluginInfo`) that declares
actions, event hooks, link handlers, panes, build/startup commands, and target platforms.
The entire namespace is **API-only**: there is no `herdr plugin` CLI group, so every method
below is reached by sending the request envelope over `$HERDR_SOCKET_PATH` directly.

Only `plugin.list` and `plugin.action.list` were exercised against the live session (both
returned empty collections). The mutating methods — `plugin.link`, `plugin.unlink`,
`plugin.enable`, `plugin.disable`, `plugin.action.invoke`, and all `plugin.pane.*` — were
**not** live-validated; their examples are constructed from the schema and labeled as such.

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
[plugin.log.list](#pluginloglist)). Side-effecting: spawns a process. Not live-validated.

**Params** (`PluginActionInvokeParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `action_id` | string | yes | — | Id of the action to invoke (matches `PluginActionInfo.action_id`). |
| `plugin_id` | string \| null | no | null | Restrict resolution to this plugin's actions; when null the action id is resolved across all installed plugins. |
| `context` | [PluginInvocationContext](#plugininvocationcontext-request) \| null | no | null | Invocation context supplied to the action (workspace/tab/pane/selection/link details). |

**Result** — `type: "plugin_action_invoked"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_action_invoked"` | Result discriminator. |
| `action` | [PluginActionInfo](#pluginactioninfo) | The action that was invoked. |
| `context` | [PluginInvocationContext](#plugininvocationcontext-response) | The (possibly server-enriched) context the action ran with. |
| `log` | [PluginCommandLogInfo](#plugincommandloginfo) | Log entry recording the command run. |

**Errors**: `plugin_not_found` (unknown `plugin_id`) or an action-not-found error (unknown
`action_id`), plus validation errors, are likely for bad input; herdr uses entity-specific
`<entity>_not_found` codes. Not live-validated, so other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"plugin.action.invoke","params":{"action_id":"format-buffer","plugin_id":"acme.tools","context":{"workspace_id":"ws-1","focused_pane_id":"pane-7","selected_text":"…"}}}
{"id":"1","result":{"type":"plugin_action_invoked","action":{"plugin_id":"acme.tools","action_id":"format-buffer","title":"Format buffer","command":["fmt","--stdin"],"contexts":["selection"]},"context":{"workspace_id":"ws-1","focused_pane_id":"pane-7","selected_text":"…"},"log":{"log_id":"log-42","plugin_id":"acme.tools","action_id":"format-buffer","command":["fmt","--stdin"],"status":"succeeded","started_unix_ms":1755640000000,"finished_unix_ms":1755640000120,"exit_code":0}}}
```

---

## plugin.action.list

Lists all actions declared by installed plugins, optionally filtered to a single `plugin_id`.
Read-only. Live-validated against the empty session (no plugins linked → empty `actions`).

**Params** (`PluginActionListParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string \| null | no | null | Restrict to actions from this plugin; when null, actions from all installed plugins are returned. |

**Result** — `type: "plugin_action_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_action_list"` | Result discriminator. |
| `actions` | array of [PluginActionInfo](#pluginactioninfo) | Declared actions (empty when no plugins/actions match). |

**Errors**: none observed. Other codes possible on invalid `plugin_id` (not validated).

**CLI**: API-only (no CLI subcommand).

**Example** — Validated 2026-08-19 against herdr 0.8.2.

```json
{"id":"r4","method":"plugin.action.list","params":{}}
{"id":"r4","result":{"type":"plugin_action_list","actions":[]}}
```

---

## plugin.disable

Disables an installed plugin by id: its actions, event hooks, link handlers, panes, and
startup commands stop being active, but the registry entry is retained (contrast
[plugin.unlink](#pluginunlink), which removes it). Side-effecting. Not live-validated.

**Params** (`PluginSetEnabledParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string | yes | — | Id of the plugin to disable. |

**Result** — `type: "plugin_disabled"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_disabled"` | Result discriminator. |
| `plugin` | [InstalledPluginInfo](#installedplugininfo) | The updated plugin entry (with `enabled: false`). |

**Errors**: `plugin_not_found` likely for an unknown `plugin_id`; not live-validated, other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"plugin.disable","params":{"plugin_id":"acme.tools"}}
{"id":"1","result":{"type":"plugin_disabled","plugin":{"plugin_id":"acme.tools","name":"Acme Tools","version":"1.0.0","manifest_path":"/home/u/.herdr/plugins/acme/herdr-plugin.toml","plugin_root":"/home/u/.herdr/plugins/acme","enabled":false}}}
```

---

## plugin.enable

Enables an installed plugin by id, re-activating its actions, event hooks, link handlers,
panes, and startup commands. Side-effecting. Not live-validated.

**Params** (`PluginSetEnabledParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string | yes | — | Id of the plugin to enable. |

**Result** — `type: "plugin_enabled"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_enabled"` | Result discriminator. |
| `plugin` | [InstalledPluginInfo](#installedplugininfo) | The updated plugin entry (with `enabled: true`). |

**Errors**: `plugin_not_found` likely for an unknown `plugin_id`; not live-validated, other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"plugin.enable","params":{"plugin_id":"acme.tools"}}
{"id":"1","result":{"type":"plugin_enabled","plugin":{"plugin_id":"acme.tools","name":"Acme Tools","version":"1.0.0","manifest_path":"/home/u/.herdr/plugins/acme/herdr-plugin.toml","plugin_root":"/home/u/.herdr/plugins/acme","enabled":true}}}
```

---

## plugin.link

Links a plugin directory into the registry from a filesystem `path`, parsing its manifest into
an `InstalledPluginInfo`. The plugin is enabled by default. An optional `source` records
provenance (local vs. GitHub, repo/owner/ref/commit). Non-fatal manifest problems (unknown
event names, missing files) are collected into the entry's `warnings`. Side-effecting: mutates
the registry. Not live-validated.

**Params** (`PluginLinkParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `path` | string | yes | — | Filesystem path to the plugin directory (or manifest) to link. |
| `enabled` | boolean | no | `true` | Whether the linked plugin is enabled immediately. |
| `source` | [PluginSourceInfo](#pluginsourceinfo) \| null | no | null | Provenance metadata recorded for the plugin (defaults to local kind when the object is present). |

**Result** — `type: "plugin_linked"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_linked"` | Result discriminator. |
| `plugin` | [InstalledPluginInfo](#installedplugininfo) | The newly linked plugin entry. |

**Errors**: filesystem/manifest-parse errors likely for a bad `path`; not live-validated, other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"plugin.link","params":{"path":"/home/u/dev/acme-plugin","enabled":true,"source":{"kind":"github","owner":"acme","repo":"herdr-acme","requested_ref":"v1.0.0"}}}
{"id":"1","result":{"type":"plugin_linked","plugin":{"plugin_id":"acme.tools","name":"Acme Tools","version":"1.0.0","manifest_path":"/home/u/dev/acme-plugin/herdr-plugin.toml","plugin_root":"/home/u/dev/acme-plugin","enabled":true,"source":{"kind":"github","owner":"acme","repo":"herdr-acme","requested_ref":"v1.0.0"},"actions":[],"warnings":[]}}}
```

---

## plugin.list

Lists installed plugins and their parsed manifests, optionally filtered to a single
`plugin_id`. Read-only. Live-validated against the empty session (no plugins → empty `plugins`).

**Params** (`PluginListParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string \| null | no | null | Restrict to this plugin; when null, all installed plugins are returned. |

**Result** — `type: "plugin_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_list"` | Result discriminator. |
| `plugins` | array of [InstalledPluginInfo](#installedplugininfo) | Installed plugin entries (empty when none installed). |

**Errors**: none observed. Other codes possible on invalid `plugin_id` (not validated).

**CLI**: API-only (no CLI subcommand).

**Example** — Validated 2026-08-19 against herdr 0.8.2.

```json
{"id":"r3","method":"plugin.list","params":{}}
{"id":"r3","result":{"type":"plugin_list","plugins":[]}}
```

---

## plugin.log.list

Lists recent plugin command-execution log entries (action invocations, event-hook runs,
startup/build commands), newest-first, optionally filtered by `plugin_id` and capped by
`limit`. Read-only. Not live-validated.

**Params** (`PluginLogListParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string \| null | no | null | Restrict to logs from this plugin; when null, logs from all plugins are returned. |
| `limit` | integer (uint, ≥ 0) \| null | no | null | Maximum number of log entries to return; when null the server applies its own cap. |

**Result** — `type: "plugin_log_list"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_log_list"` | Result discriminator. |
| `logs` | array of [PluginCommandLogInfo](#plugincommandloginfo) | Command-execution log entries. |

**Errors**: none observed; not live-validated, other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"plugin.log.list","params":{"plugin_id":"acme.tools","limit":20}}
{"id":"1","result":{"type":"plugin_log_list","logs":[{"log_id":"log-42","plugin_id":"acme.tools","action_id":"format-buffer","command":["fmt","--stdin"],"status":"succeeded","started_unix_ms":1755640000000,"finished_unix_ms":1755640000120,"exit_code":0,"stdout":"…","stderr":null,"error":null,"event":null}]}}
```

---

## plugin.pane.close

Closes a plugin-owned terminal pane by id. Side-effecting: destroys the pane. Not live-validated.

**Params** (`PluginPaneCloseParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `pane_id` | string | yes | — | Id of the plugin pane to close (matches `PluginPaneInfo.pane.pane_id`). |

**Result** — `type: "plugin_pane_closed"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_pane_closed"` | Result discriminator. |
| `pane_id` | string | Id of the closed pane. |

**Errors**: `pane_not_found` likely for an unknown `pane_id`; not live-validated, other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"plugin.pane.close","params":{"pane_id":"pane-9"}}
{"id":"1","result":{"type":"plugin_pane_closed","pane_id":"pane-9"}}
```

---

## plugin.pane.focus

Focuses an existing plugin-owned terminal pane by id, bringing it forward. Side-effecting:
changes UI focus. Not live-validated.

**Params** (`PluginPaneFocusParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `pane_id` | string | yes | — | Id of the plugin pane to focus. |

**Result** — `type: "plugin_pane_focused"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_pane_focused"` | Result discriminator. |
| `plugin_pane` | [PluginPaneInfo](#pluginpaneinfo) | The focused plugin pane. |

**Errors**: `pane_not_found` likely for an unknown `pane_id`; not live-validated, other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"plugin.pane.focus","params":{"pane_id":"pane-9"}}
{"id":"1","result":{"type":"plugin_pane_focused","plugin_pane":{"plugin_id":"acme.tools","entrypoint":"dashboard","pane":{"pane_id":"pane-9","terminal_id":"term-3","workspace_id":"ws-1","tab_id":"tab-2","focused":true,"agent_status":"unknown","revision":5}}}}
```

---

## plugin.pane.open

Opens a new plugin-owned terminal pane running the plugin's `entrypoint` command, placed
according to `placement` (overlay/popup/split/tab/zoomed) with optional size, split
`direction`, working directory, environment overrides, and target pane/workspace. Optionally
focuses the new pane. Side-effecting: spawns a process and creates a pane. Not live-validated.

**Params** (`PluginPaneOpenParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `plugin_id` | string | yes | — | Id of the plugin that owns the pane. |
| `entrypoint` | string | yes | — | Plugin pane entrypoint (identifies the manifest pane / command to run). |
| `placement` | [PluginPanePlacement](#pluginpaneplacement) \| null | no | null | Placement of the pane; enum `overlay`, `popup`, `split`, `tab`, `zoomed` (manifest default is `overlay` when unset). |
| `direction` | [SplitDirection](#splitdirection) \| null | no | null | Split direction when placement splits; enum `right`, `down`. |
| `width` | [PopupSize](#popupsize) \| null | no | null | Outer width, in terminal cells (integer 0–65535) or a percentage string like `"80%"`. |
| `height` | [PopupSize](#popupsize) \| null | no | null | Outer height, in terminal cells or a percentage string. |
| `cwd` | string \| null | no | null | Working directory for the pane process. |
| `env` | object (string → string) | no | `{}` | Environment variable overrides for the pane process. |
| `target_pane_id` | string \| null | no | null | Existing pane to anchor placement against (e.g. the pane to split). |
| `workspace_id` | string \| null | no | null | Workspace to open the pane in; when null the current/target workspace is used. |
| `focus` | boolean | no | `false` | Whether to focus the new pane after opening. |

**Result** — `type: "plugin_pane_opened"`:

| field | type | meaning |
| --- | --- | --- |
| `type` | const `"plugin_pane_opened"` | Result discriminator. |
| `plugin_pane` | [PluginPaneInfo](#pluginpaneinfo) | The newly opened plugin pane. |

**Errors**: `plugin_not_found` (unknown `plugin_id`) or `pane_not_found` (unknown `target_pane_id`) likely; validation errors for an unknown `entrypoint` or a malformed `PopupSize`. Not live-validated, other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"plugin.pane.open","params":{"plugin_id":"acme.tools","entrypoint":"dashboard","placement":"split","direction":"right","width":"40%","focus":true}}
{"id":"1","result":{"type":"plugin_pane_opened","plugin_pane":{"plugin_id":"acme.tools","entrypoint":"dashboard","pane":{"pane_id":"pane-9","terminal_id":"term-3","workspace_id":"ws-1","tab_id":"tab-2","focused":true,"agent_status":"unknown","revision":0}}}}
```

---

## plugin.unlink

Removes a plugin from the registry by id, undoing a [plugin.link](#pluginlink). The result's
`removed` flag reports whether an entry was actually present and removed. Side-effecting. Not
live-validated.

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

**Errors**: none observed; a missing plugin may return `removed: false` rather than an error. Not live-validated, other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Constructed from schema; not live-validated.

```json
{"id":"1","method":"plugin.unlink","params":{"plugin_id":"acme.tools"}}
{"id":"1","result":{"type":"plugin_unlinked","plugin_id":"acme.tools","removed":true}}
```

---

## Shared plugin types

Composite entities referenced by the methods above. `PaneInfo` is documented in
[../data-model.md](../data-model.md); the rest are plugin-namespace-specific and defined here.

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
| `min_herdr_version` | string | no | `""` | Minimum required herdr version (empty when unspecified). |
| `source` | [PluginSourceInfo](#pluginsourceinfo) | no | `{"kind":"local"}` | Provenance metadata. |
| `platforms` | array of [PluginPlatform](#pluginplatform) \| null | no | null | Platforms the plugin targets; null means unrestricted. |
| `actions` | array of [PluginManifestAction](#pluginmanifestaction) | no | `[]` | Declared actions. |
| `build` | array of [PluginManifestBuild](#pluginmanifestbuild) | no | `[]` | Build commands. |
| `startup` | array of [PluginManifestStartup](#pluginmanifeststartup) | no | `[]` | Startup commands. |
| `events` | array of [PluginManifestEventHook](#pluginmanifesteventhook) | no | `[]` | Event hooks. |
| `link_handlers` | array of [PluginManifestLinkHandler](#pluginmanifestlinkhandler) | no | `[]` | Link handlers. |
| `panes` | array of [PluginManifestPane](#pluginmanifestpane) | no | `[]` | Declared panes. |
| `warnings` | array of string | no | `[]` | Non-fatal warnings collected at link time or on registry load (e.g. unknown event names, missing manifest file); the entry is kept and surfaced by `plugin.list`. |

### PluginSourceInfo

Provenance of a plugin. Used both as a request field (on `plugin.link`) and a response field.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `kind` | [PluginSourceKind](#pluginsourcekind) | no | `"local"` | Source kind; enum `local`, `github`. |
| `owner` | string \| null | no | null | Repository owner (GitHub sources). |
| `repo` | string \| null | no | null | Repository name (GitHub sources). |
| `subdir` | string \| null | no | null | Subdirectory within the repo. |
| `requested_ref` | string \| null | no | null | Requested git ref (branch/tag). |
| `resolved_commit` | string \| null | no | null | Resolved commit SHA. |
| `managed_path` | string \| null | no | null | Managed on-disk path for fetched sources. |
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

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `on` | string | yes | — | Event name the hook fires on. |
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
