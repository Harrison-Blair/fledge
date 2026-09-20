# herdr API: server methods

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `server` namespace controls the lifecycle and configuration of the running headless herdr server itself, rather than any workspace, pane, or agent it hosts. These methods stop the server, hot-reload `config.toml`, inspect and reload the agent-detection manifests that tell herdr how to recognize agent processes, and perform a live handoff to a replacement binary while preserving session state. Four of the five methods are exposed through the `herdr server` CLI command group; `server.live_handoff` has no `herdr server` subcommand of its own and is reached only through `herdr update --handoff` (and `--handoff` on remote/app attach — see [server.live_handoff](#serverlive_handoff)). Every method takes `params` (even when empty) and each request occupies its own socket connection; a request with `params` missing or `null` fails `invalid_request`, but unknown keys inside `params` are ignored and even a bare `[]` is accepted in its place. Any envelope-level failure — including these — answers with `"id":""` rather than echoing the request's id, so a client cannot correlate a rejected request by id; only a successful result echoes it. None of `server.reload_config`, `server.reload_agent_manifests`, or `server.agent_manifests` emits a subscription event, even though the first two can change server-visible state.

Every `herdr server` subcommand except `stop` first sends a `ping` preflight on its own connection before the real request on a second connection. `herdr server update-agent-manifests` has no method of its own: it performs a client-side network fetch (rewriting `~/.local/state/herdr/agent-detection/status.toml`), then sends [server.reload_agent_manifests](#serverreload_agent_manifests) followed by [server.agent_manifests](#serveragent_manifests). `reload-agent-manifests` and `reload-config` print the whole JSON response envelope on stdout although neither has a `--json` flag, and `agent-manifests --json` also prints the envelope rather than the bare result. Error reporting is inconsistent across the group: `stop` prints a plain sentence on failure, `reload-config` prints a JSON error envelope, and a session socket path too long for `sun_path` makes the others leak a raw Rust debug string instead of either.

5 methods:

| method | purpose |
| --- | --- |
| [server.agent_manifests](#serveragent_manifests) | List the active agent-detection manifests and their remote-update status. |
| [server.live_handoff](#serverlive_handoff) | Hand the running server off to a replacement binary, preserving session state. |
| [server.reload_agent_manifests](#serverreload_agent_manifests) | Reload local agent-detection manifest overrides from disk. |
| [server.reload_config](#serverreload_config) | Re-read and apply `config.toml` in the running server. |
| [server.stop](#serverstop) | Shut down the running server via the socket API. |

## server.agent_manifests

Returns the set of agent-detection manifests the server currently has active, one entry per known agent, together with the timestamp and outcome of the most recent remote-update check. This is a read-only inspection call; it reports cached state and does not itself fetch from the network. Each manifest records where it was sourced from (bundled with the binary, remote-fetched, or a local override file), the active version, the cached remote version, and any warning raised while resolving precedence between a bundled, remote, and local-override manifest.

A freshly started server briefly reports `last_check_unix: null` and `last_result: null` with every manifest as `"bundled"`, until its automatic startup check completes — typically within a few seconds; a client that reads `server.agent_manifests` immediately after starting a server races this update. A server whose catalog URL is unreachable stays on all-`"bundled"` manifests indefinitely (no retry observed within 31 s), with the failure visible only in the top-level `last_result`; per-agent `remote_update_error` stays absent even then.

**Params**: `EmptyParams` — `{}`. No fields.

**Result** — `type: "agent_manifest_status"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"agent_manifest_status"` | yes | — | Result discriminator. |
| `manifests` | array of `AgentManifestInfo` | yes | — | One entry per known agent (see field table below). Always 22 entries, one per bundled agent id, on every server observed. |
| `last_check_unix` | integer (uint64) \| null | no | null | Unix time (seconds) of the last remote-update check, or null if never checked. |
| `last_result` | string \| null | no | null | Outcome label of the last remote check: `"checked"` on success, a free-text `"failed: <reason>"` string on failure (e.g. `"failed: failed to fetch https://127.0.0.1:9/agent-detection/index.toml"`), or null if never checked. |

`AgentManifestInfo` object fields:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `agent` | string | yes | — | Agent identifier (e.g. `pi`, `claude`, `codex`). |
| `source` | string | yes | — | Origin of the active manifest: `"bundled"`, `"remote:<path>"` for a fetched TOML file, or a bare absolute path (no prefix) to a local override file, e.g. `/home/penguin/.config/herdr/agent-detection/claude.toml` (see [server.reload_agent_manifests](#serverreload_agent_manifests) for where overrides live). |
| `source_kind` | string | yes | — | Category of the source: `"bundled"`, `"remote"`, or `"local override"` (with a space). |
| `local_override_shadowing_remote` | boolean | yes | — | True whenever a local override file exists for this agent — not whether it is in effect. A malformed override still reports `true` here while `source`/`source_kind` stay on the remote or bundled manifest and `warning` explains the rejection; read `source`/`source_kind`, not this flag, to know what is actually active. |
| `active_version` | string \| null | no | null | Version of the manifest currently in effect. Present on every entry observed; never seen null or absent. |
| `cached_remote_version` | string \| null | no | null | Version of the last remote manifest cached on disk. The key is absent, not null, when there is no usable cached remote manifest (e.g. an all-bundled server). |
| `remote_last_checked_unix` | integer (uint64) \| null | no | null | Unix time (seconds) this agent's remote manifest was last checked. Absent, not null, until a remote check has completed for this agent. |
| `remote_update_result` | string \| null | no | null | Per-agent remote check outcome: `"current"` (already up to date) or `"updated"` (freshly fetched); absent, not null, when no remote check has succeeded for this agent. |
| `remote_update_error` | string \| null | no | null | Error text from the last remote update, or null on success. Not reproduced live: with the catalog index itself unreachable, the whole fetch fails before any per-agent fetch runs, so this field never appeared in any probe (schema-derived). |
| `warning` | string \| null | no | null | Advisory raised while resolving this manifest; absent, not null, when there is no warning. Observed forms: a cached remote version older than the bundled one, a remote manifest that failed to parse, and an override file that failed to parse. |

A local override in effect looks like this (captured after writing a local override file for `claude` and calling `server.reload_agent_manifests`):

```json
{"agent":"claude","source":"/home/penguin/.config/herdr/agent-detection/claude.toml","source_kind":"local override","active_version":"9999.01.01.1","cached_remote_version":"2026.09.11.1","local_override_shadowing_remote":true,"remote_update_result":"updated","remote_last_checked_unix":1789855952}
```

**Errors**: none observed. Other codes possible.

**CLI**: `herdr server agent-manifests [--json]` — human table by default; `--json` prints the entire response envelope (`{"id":...,"result":{...}}`), not the bare result, with keys re-serialized in alphabetical order. The human table adds a `!` marker and a "local override shadows cached remote rules" line for entries where `local_override_shadowing_remote` is true.

**Example**

```json
{"id":"r5","method":"server.agent_manifests","params":{}}
{"id":"r5","result":{"type":"agent_manifest_status","last_check_unix":1789682177,"last_result":"checked","manifests":[{"agent":"pi","source":"remote:/home/penguin/.local/state/herdr/agent-detection/remote/pi.toml","source_kind":"remote","active_version":"2026.09.14.1","cached_remote_version":"2026.09.14.1","local_override_shadowing_remote":false,"remote_update_result":"current","remote_last_checked_unix":1789682177},{"agent":"grok","source":"bundled","source_kind":"bundled","active_version":"2026.07.16.2","cached_remote_version":"2026.07.16.1","local_override_shadowing_remote":false,"remote_update_result":"current","remote_last_checked_unix":1789682177,"warning":"ignored remote manifest /home/penguin/.local/state/herdr/agent-detection/remote/grok.toml because cached version 2026.07.16.1 is older than bundled 2026.07.16.2"}, …]}}
```

This bundled+warning shape for `grok` is a historical capture and is reproducible (downgrading a cached remote manifest triggers it), but on a normally-updated server every agent will typically show `"remote"` with no warning instead.

Validated 2026-09-19 against herdr 0.9.1. (`remote_update_error` could not be provoked — see the field note above.)

## server.live_handoff

Hands the running server off to a replacement binary without dropping session state: the current process launches (or execs into) another herdr executable and transfers ownership of the live sessions, so attached clients and agents continue across the swap. This is the mechanism behind `herdr update --handoff` and `--handoff` on remote attach. The optional guard fields let the caller assert what it expects the incoming binary to be — refusing the handoff if the target's protocol or version does not match — and to name the executable to hand off to.

Calling this method with empty params (`{}`) is not a no-op or a dry run: it performs a real handoff using the default/updated binary. After a successful handoff the process's argv changes from `herdr --session <name> server` to `herdr server --handoff-import <session-dir>/herdr-handoff-<oldpid>.sock <oldpid>-<nonce>` with `ppid` 1, so tooling that finds servers with `pgrep -f "herdr --session <name>"` stops finding them. Session state (workspaces, tabs, panes, focus, counts) is unchanged across the swap, but every pane's `terminal_id` is reissued, and every OPEN connection — including an `events.subscribe` stream — is silently dropped with no event and no close reason; a client must reconnect and, if it was subscribed, re-subscribe. There is no server-side timeout on the handoff import handshake: an `import_exe` that spawns but never speaks the handoff protocol (e.g. `/bin/true`) leaves the request hanging indefinitely while the server itself stays alive and reachable.

**Params** — `ServerLiveHandoffParams`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `expected_protocol` | integer (uint32) \| null | no | null | If set, require the target binary to speak this protocol version; refused (`handoff_failed`) on mismatch. A negative or out-of-range integer, or a non-integer value, fails `invalid_request` instead. |
| `expected_version` | string \| null | no | null | If set, require the target binary to report this herdr version; refused (`handoff_failed`) on mismatch. A non-string value fails `invalid_request`. |
| `import_exe` | string \| null | no | null | Path to the replacement executable to hand off to; null or omitted uses the default/updated binary. A missing or non-executable path fails `handoff_failed` naming the path and the OS error; a path to a process that never speaks the handoff protocol hangs the request indefinitely (see above). |

**Result** — `type: "ok"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"ok"` | yes | — | Success acknowledgement; the process then hands off to the replacement binary (see above) rather than continuing to serve the current connection. |

**Errors**: `handoff_failed` is the only error code observed. A protocol/version guard mismatch returns `{"code":"handoff_failed","message":"handoff stream closed while reading line"}` — the message does not name the mismatch. A bad `import_exe` returns `handoff_failed` with a message naming the path and the OS error, e.g. `failed to spawn handoff import server at /nonexistent/rvprobe-herdr: No such file or directory (os error 2)`, or `Permission denied (os error 13)` for a path that is not executable. Malformed `params` (wrong types, out-of-range integers) fail `invalid_request` instead, before any handoff is attempted. Other codes possible.

**CLI**: no direct `herdr server` subcommand — `herdr server --help` lists only `stop`, `reload-config`, `agent-manifests`, `update-agent-manifests`, and `reload-agent-manifests`, none of which issues `server.live_handoff` on the wire. It is reached only via `herdr update --handoff` (`herdr update --help` documents `--handoff` as "Try live handoff after installing") and `--handoff` on remote/app attach; the CLI entry point itself was not exercised here because `herdr update` installs a new binary into `~/.local/bin`, which is outside the scope of a scratch-session probe.

**Example** — matching guards:

```json
{"id":"h1","method":"server.live_handoff","params":{"expected_protocol":22,"expected_version":"0.9.1","import_exe":null}}
{"id":"h1","result":{"type":"ok"}}
```

A guard mismatch fails rather than falling back to an unguarded handoff:

```json
{"id":"h2","method":"server.live_handoff","params":{"expected_protocol":20,"expected_version":"0.8.2"}}
{"id":"h2","error":{"code":"handoff_failed","message":"handoff stream closed while reading line"}}
```

Validated 2026-09-19 against herdr 0.9.1. (17 parameter combinations exercised on an isolated scratch server, including two real handoffs; the `herdr update --handoff` CLI entry point and a handoff against a genuinely different herdr version were not exercised — see CLI above.)

## server.reload_agent_manifests

Reloads the local agent-detection manifest overrides from disk and returns the resulting active manifest set. Unlike [server.agent_manifests](#serveragent_manifests), this re-reads the on-disk overrides and re-resolves precedence, but it does not perform a remote fetch (that is `herdr server update-agent-manifests`), so the result omits the top-level `last_check_unix`/`last_result` fields. Use it after editing a local override manifest — at `$XDG_CONFIG_HOME/herdr/agent-detection/<agent>.toml` (i.e. `~/.config/herdr/agent-detection/<agent>.toml`) — to apply the change to a running server; there is no filesystem watcher, so without a reload the server keeps serving the old manifest. A malformed override file, or one for an agent id herdr does not know, is absorbed silently: the former surfaces only as a `warning` on that agent's entry (with `source`/`source_kind` staying on the remote or bundled manifest), and the latter adds no entry and no warning.

**Params**: `EmptyParams` — `{}`. No fields.

**Result** — `type: "agent_manifest_reload"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"agent_manifest_reload"` | yes | — | Result discriminator. |
| `manifests` | array of `AgentManifestInfo` | yes | — | The active manifest set after reload. See the `AgentManifestInfo` field table under [server.agent_manifests](#serveragent_manifests). |

**Errors**: none observed. Other codes possible.

**CLI**: `herdr server reload-agent-manifests` (reload local overrides; prints the whole JSON response envelope on stdout, though it has no `--json` flag). Compare `herdr server update-agent-manifests`, which additionally performs a client-side network fetch (rewriting `~/.local/state/herdr/agent-detection/status.toml`) before sending this method and then `server.agent_manifests` — it has no method of its own.

**Example**

```json
{"id":"m1","method":"server.reload_agent_manifests","params":{}}
{"id":"m1","result":{"type":"agent_manifest_reload","manifests":[{"agent":"pi","source":"remote:/home/penguin/.local/state/herdr/agent-detection/remote/pi.toml","source_kind":"remote","active_version":"2026.09.14.1","cached_remote_version":"2026.09.14.1","local_override_shadowing_remote":false,"remote_update_result":"current","remote_last_checked_unix":1789682177}, …]}}
```

Validated 2026-09-19 against herdr 0.9.1.

## server.reload_config

Re-reads `config.toml` from disk and applies it to the running server, returning whether the reload fully applied along with any diagnostics produced while parsing or applying the new configuration. This is the socket equivalent of the global menu's "reload config" action and lets configuration edits take effect without restarting the server.

**Params**: `EmptyParams` — `{}`. No fields.

**Result** — `type: "config_reload"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"config_reload"` | yes | — | Result discriminator. |
| `status` | `ConfigReloadStatus` enum | yes | — | Overall outcome. One of: `"applied"` (fully applied), `"partial"` (some settings applied, some rejected), `"failed"` (nothing applied). |
| `diagnostics` | array of string | yes | — | Human-readable messages about settings that were rejected or adjusted; empty on a clean `applied` reload. |

A key placed at the wrong nesting level is reported as merely unknown, not misplaced: `manifest_check = false` at the top level (it belongs under `[update]`) yields `status: "partial"` with `diagnostics: ["unknown config key manifest_check; ignoring key"]`. Unknown keys never fail a reload.

**Errors**: none observed on a valid config. A malformed config surfaces through `status` (`partial`/`failed`) and `diagnostics` rather than a protocol error. Other codes possible.

**CLI**: `herdr server reload-config`. With no server running it prints a JSON error envelope to stderr instead of failing at the protocol layer, e.g. `{"id":"cli:server:reload-config","error":{"code":"server_not_running","message":"no herdr server is running at <path>; run \`herdr\` to start or attach it"}}`, unlike `server stop`'s plain-sentence error (see below). It also prints the whole response envelope to stdout on success even though it has no `--json` flag.

**Example**

```json
{"id":"cli:server:reload-config","method":"server.reload_config","params":{}}
{"id":"cli:server:reload-config","result":{"type":"config_reload","status":"applied","diagnostics":[]}}
```

Validated 2026-09-19 against herdr 0.9.1.

## server.stop

Shuts down the running server via the socket API, terminating all its sessions (a server hosts exactly one session, so this always means the one it hosts). After acknowledging the request the server tears itself down: it closes every connection, including an open `events.subscribe` stream (with no closing event), and unlinks the socket file — a follow-up connect fails immediately (`FileNotFoundError`) rather than hanging. The CLI prints nothing on success (empty stdout, exit status 0), but does not return as soon as the acknowledgement arrives: it polls for the socket file to disappear and, if it is still present after 15 s, exits 1 with `server did not stop within 15000ms; sockets are still reachable at <path>`; a normal stop completed in well under a second in testing.

**Params**: `EmptyParams` — `{}`. No fields.

**Result** — `type: "ok"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"ok"` | yes | — | Success acknowledgement; the server then terminates. |

**Errors**: none observed for a well-formed request. If no server is running the CLI reports `server is not running or cannot be reached at <path>: No such file or directory (os error 2)` on stderr with exit status 1; the same message covers both a stale socket path and one that never existed.

**CLI**: `herdr server stop`. Never stop the primary herdr server during agent experiments; use a named test session for isolated work.

**Example**

```json
{"id":"cli:session:stop","method":"server.stop","params":{}}
{"id":"cli:session:stop","result":{"type":"ok"}}
```

The CLI sends id `cli:session:stop`, not `cli:server:stop` — the only `herdr server` subcommand on this page whose id doesn't follow the `cli:server:<subcommand>` pattern.

Validated 2026-09-19 against herdr 0.9.1. The mutating probe produced empty stdout on success and was also confirmed directly on the raw socket; `{"type":"ok"}` is the schema's sole non-error variant for this method.
