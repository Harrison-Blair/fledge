# herdr API: server methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `server` namespace controls the lifecycle and configuration of the running headless herdr server itself, rather than any workspace, pane, or agent it hosts. These methods stop the server, hot-reload `config.toml`, inspect and reload the agent-detection manifests that tell herdr how to recognize agent processes, and perform a live handoff to a replacement binary while preserving session state. All five methods are exposed through the `herdr server` CLI command group (plus `server.live_handoff`, which backs `herdr update --handoff`). Every method takes `params` (even when empty) and each request occupies its own socket connection.

5 methods:

| method | purpose |
| --- | --- |
| [server.agent_manifests](#serveragent_manifests) | List the active agent-detection manifests and their remote-update status. |
| [server.live_handoff](#serverlive_handoff) | Hand the running server off to a replacement binary, preserving session state. |
| [server.reload_agent_manifests](#serverreload_agent_manifests) | Reload local agent-detection manifest overrides from disk. |
| [server.reload_config](#serverreload_config) | Re-read and apply `config.toml` in the running server. |
| [server.stop](#serverstop) | Shut down the running server via the socket API. |

## server.agent_manifests

Returns the set of agent-detection manifests the server currently has active, one entry per known agent, together with the timestamp and outcome of the most recent remote-update check. This is a read-only inspection call; it reports cached state and does not itself fetch from the network. Each manifest records where it was sourced from (bundled with the binary, or a remote-fetched TOML file), the active version, the cached remote version, and any warning raised while resolving precedence between a bundled and a remote manifest.

**Params**: `EmptyParams` — `{}`. No fields.

**Result** — `type: "agent_manifest_status"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"agent_manifest_status"` | yes | — | Result discriminator. |
| `manifests` | array of `AgentManifestInfo` | yes | — | One entry per known agent (see field table below). |
| `last_check_unix` | integer (uint64) \| null | no | null | Unix time (seconds) of the last remote-update check, or null if never checked. |
| `last_result` | string \| null | no | null | Outcome label of the last remote check (e.g. `"checked"`), or null. |

`AgentManifestInfo` object fields:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `agent` | string | yes | — | Agent identifier (e.g. `pi`, `claude`, `codex`). |
| `source` | string | yes | — | Origin of the active manifest: `"bundled"`, or `"remote:<path>"` for a fetched TOML file. |
| `source_kind` | string | yes | — | Category of the source (observed: `"bundled"`, `"remote"`). (inferred from probe values.) |
| `local_override_shadowing_remote` | boolean | yes | — | True when a local override manifest is taking precedence over the remote one. |
| `active_version` | string \| null | no | null | Version of the manifest currently in effect. |
| `cached_remote_version` | string \| null | no | null | Version of the last remote manifest cached on disk. |
| `remote_last_checked_unix` | integer (uint64) \| null | no | null | Unix time (seconds) this agent's remote manifest was last checked. |
| `remote_update_result` | string \| null | no | null | Per-agent remote check outcome (observed: `"current"`). |
| `remote_update_error` | string \| null | no | null | Error text from the last remote update, or null on success. |
| `warning` | string \| null | no | null | Advisory raised while resolving this manifest (e.g. a remote version ignored because it is older than the bundled one). |

**Errors**: none observed. Other codes possible.

**CLI**: `herdr server agent-manifests [--json]` (human table by default; `--json` emits the raw result).

**Example**

```json
{"id":"r5","method":"server.agent_manifests","params":{}}
{"id":"r5","result":{"type":"agent_manifest_status","last_check_unix":1787190524,"last_result":"checked","manifests":[{"agent":"pi","source":"remote:/home/penguin/.local/state/herdr/agent-detection/remote/pi.toml","source_kind":"remote","active_version":"2026.06.10.1","cached_remote_version":"2026.06.10.1","local_override_shadowing_remote":false,"remote_update_result":"current","remote_last_checked_unix":1787190524},{"agent":"grok","source":"bundled","source_kind":"bundled","active_version":"2026.07.16.2","cached_remote_version":"2026.07.16.1","local_override_shadowing_remote":false,"remote_update_result":"current","remote_last_checked_unix":1787190524,"warning":"ignored remote manifest /home/penguin/.local/state/herdr/agent-detection/remote/grok.toml because cached version 2026.07.16.1 is older than bundled 2026.07.16.2"}, …]}}
```

Validated 2026-08-19 against herdr 0.8.2.

## server.live_handoff

Hands the running server off to a replacement binary without dropping session state: the current process launches (or execs into) another herdr executable and transfers ownership of the live sessions, so attached clients and agents continue across the swap. This is the mechanism behind `herdr update --handoff` and `--handoff` on remote attach. The optional guard fields let the caller assert what it expects the incoming binary to be — refusing the handoff if the target's protocol or version does not match — and to name the executable to hand off to.

**Params** — `ServerLiveHandoffParams`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `expected_protocol` | integer (uint32) \| null | no | null | If set, require the target binary to speak this protocol version; the handoff is refused on mismatch. (inferred) |
| `expected_version` | string \| null | no | null | If set, require the target binary to report this herdr version. (inferred) |
| `import_exe` | string \| null | no | null | Path to the replacement executable to hand off to; null uses the default/updated binary. (inferred) |

**Result** — `type: "ok"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"ok"` | yes | — | Success acknowledgement. |

**Errors**: none captured. A protocol/version mismatch against `expected_protocol`/`expected_version` is expected to fail; other codes possible.

**CLI**: no direct `herdr server` subcommand — reached via `herdr update --handoff` (and `--handoff` on remote/app attach).

**Example**

```json
{"id":"h1","method":"server.live_handoff","params":{"expected_protocol":20,"expected_version":"0.8.2","import_exe":null}}
{"id":"h1","result":{"type":"ok"}}
```

Constructed from schema; not live-validated. (server.live_handoff was not probed.)

## server.reload_agent_manifests

Reloads the local agent-detection manifest overrides from disk and returns the resulting active manifest set. Unlike [server.agent_manifests](#serveragent_manifests), this re-reads the on-disk overrides and re-resolves precedence, but it does not perform a remote fetch (that is `herdr server update-agent-manifests`), so the result omits the top-level `last_check_unix`/`last_result` fields. Use it after editing a local override manifest to apply the change to a running server.

**Params**: `EmptyParams` — `{}`. No fields.

**Result** — `type: "agent_manifest_reload"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"agent_manifest_reload"` | yes | — | Result discriminator. |
| `manifests` | array of `AgentManifestInfo` | yes | — | The active manifest set after reload. See the `AgentManifestInfo` field table under [server.agent_manifests](#serveragent_manifests). |

**Errors**: none observed. Other codes possible.

**CLI**: `herdr server reload-agent-manifests` (reload local overrides). Compare `herdr server update-agent-manifests`, which additionally fetches from the network.

**Example**

```json
{"id":"m1","method":"server.reload_agent_manifests","params":{}}
{"id":"m1","result":{"type":"agent_manifest_reload","manifests":[{"agent":"pi","source":"remote:/home/penguin/.local/state/herdr/agent-detection/remote/pi.toml","source_kind":"remote","active_version":"2026.06.10.1","cached_remote_version":"2026.06.10.1","local_override_shadowing_remote":false,"remote_update_result":"current","remote_last_checked_unix":1787190524}, …]}}
```

Constructed from schema; not live-validated. (Manifest field shapes mirror the probed `server.agent_manifests` response.)

## server.reload_config

Re-reads `config.toml` from disk and applies it to the running server, returning whether the reload fully applied along with any diagnostics produced while parsing or applying the new configuration. This is the socket equivalent of the global menu's "reload config" action and lets configuration edits take effect without restarting the server.

**Params**: `EmptyParams` — `{}`. No fields.

**Result** — `type: "config_reload"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"config_reload"` | yes | — | Result discriminator. |
| `status` | `ConfigReloadStatus` enum | yes | — | Overall outcome. One of: `"applied"` (fully applied), `"partial"` (some settings applied, some rejected), `"failed"` (nothing applied). |
| `diagnostics` | array of string | yes | — | Human-readable messages about settings that were rejected or adjusted; empty on a clean `applied` reload. |

**Errors**: none observed on a valid config. A malformed config surfaces through `status` (`partial`/`failed`) and `diagnostics` rather than a protocol error. Other codes possible.

**CLI**: `herdr server reload-config`.

**Example**

```json
{"id":"cli:server:reload-config","method":"server.reload_config","params":{}}
{"id":"cli:server:reload-config","result":{"type":"config_reload","status":"applied","diagnostics":[]}}
```

Validated 2026-08-19 against herdr 0.8.2.

## server.stop

Shuts down the running server via the socket API, terminating all its sessions. After acknowledging the request the server tears itself down and closes the socket, so callers should expect the connection to end. The CLI prints nothing on success (empty stdout, exit status 0).

**Params**: `EmptyParams` — `{}`. No fields.

**Result** — `type: "ok"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"ok"` | yes | — | Success acknowledgement; the server then terminates. |

**Errors**: none observed. If no server is running the CLI reports a socket error on stderr with exit status 1.

**CLI**: `herdr server stop`. Never stop the primary herdr server during agent experiments; use a named test session for isolated work.

**Example**

```json
{"id":"cli:server:stop","method":"server.stop","params":{}}
{"id":"cli:server:stop","result":{"type":"ok"}}
```

Validated 2026-08-19 against herdr 0.8.2. The mutating probe produced empty stdout on success; the `{"type":"ok"}` result is the schema's sole non-error variant for this method.
