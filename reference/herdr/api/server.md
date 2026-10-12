# herdr API: server methods

> herdr 0.9.3 · protocol 22 · schema_version 1 · captured 2026-10-06
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `server` namespace controls the lifecycle and configuration of the running headless herdr server itself, rather than any workspace, pane, or agent it hosts. These methods stop the server, hot-reload `config.toml`, inspect and reload the agent-detection manifests that tell herdr how to recognize agent processes, perform a live handoff to a replacement binary while preserving session state, and (new in 0.9.3) register a remote-host SSH agent socket for the life of one API connection. Five of the six methods are exposed through the `herdr server` CLI command group, but `herdr server --help` lists only four of them: `server.live_handoff` has a `herdr server live-handoff` subcommand that appears only in the group's plain usage list (printed, for example, by `herdr server bogus`), and is also reached through `herdr update --handoff` (and `--handoff` on remote/app attach — see [server.live_handoff](#serverlive_handoff)). `server.ssh_agent.register` has no CLI subcommand at all. Every method takes `params` (even when empty) and each request occupies its own socket connection; a request with `params` missing or `null` (or a scalar such as `5`) fails `invalid_request`, but unknown keys inside `params` are ignored and even a bare `[]` is accepted in its place. Since 0.9.3 these envelope-level failures echo the request's `id` whenever the line parses as JSON with a string `id`; only an unparseable line or a missing or non-string `id` answers with `"id":""` (in 0.9.1 every envelope-level failure answered `"id":""`). See [protocol.md](../protocol.md). None of `server.reload_config`, `server.reload_agent_manifests`, or `server.agent_manifests` emits a subscription event, even though the first two can change server-visible state. Validated 2026-10-06 against herdr 0.9.3 (envelope matrix `params` absent/`null`/`[]`/`{"zz":1}`/`5` on `agent_manifests`, `reload_agent_manifests` and `reload_config`; the no-event claim was not re-probed and stays from the 0.9.1 pass).

Every `herdr server` subcommand except `stop` and `live-handoff` first sends a `ping` preflight on its own connection before the real request on a second connection. `herdr server update-agent-manifests` has no method of its own: it performs a client-side network fetch (rewriting `~/.local/state/herdr/agent-detection/status.toml`), then sends [server.reload_agent_manifests](#serverreload_agent_manifests) followed by [server.agent_manifests](#serveragent_manifests). `reload-agent-manifests` and `reload-config` print the whole JSON response envelope on stdout although neither has a `--json` flag, and `agent-manifests --json` also prints the envelope rather than the bare result. Error reporting is inconsistent across the group: `stop` prints a plain sentence on failure, `reload-config`, `agent-manifests` and `reload-agent-manifests` print a JSON `server_not_running` error envelope, and a socket path too long for `sun_path` makes all but `stop` leak a raw Rust debug string (`Error: Custom { kind: InvalidInput, error: "local socket name length exceeds capacity of sun_path of sockaddr_un" }`) instead of either. Validated 2026-10-06 against herdr 0.9.3 (wire traffic captured through a logging proxy in front of a scratch server; `update-agent-manifests` was not run this pass because its fetch rewrites shared state under `~/.local/state/herdr`, so its wire sequence is from the 0.9.1 pass).

6 methods:

| method | purpose |
| --- | --- |
| [server.agent_manifests](#serveragent_manifests) | List the active agent-detection manifests and their remote-update status. |
| [server.live_handoff](#serverlive_handoff) | Hand the running server off to a replacement binary, preserving session state. |
| [server.reload_agent_manifests](#serverreload_agent_manifests) | Reload local agent-detection manifest overrides from disk. |
| [server.reload_config](#serverreload_config) | Re-read and apply `config.toml` in the running server. |
| [server.ssh_agent.register](#serverssh_agentregister) | Register a remote-host SSH agent socket for as long as this API connection stays open. |
| [server.stop](#serverstop) | Shut down the running server via the socket API. |

## server.agent_manifests

Returns the set of agent-detection manifests the server currently has active, one entry per known agent, together with the timestamp and outcome of the most recent remote-update check. This is a read-only inspection call; it reports cached state and does not itself fetch from the network. Each manifest records where it was sourced from (bundled with the binary, remote-fetched, or a local override file), the active version, the cached remote version, and any warning raised while resolving precedence between a bundled, remote, and local-override manifest.

A freshly started server briefly reports `last_check_unix: null` and `last_result: null` with every manifest as `"bundled"`, until its automatic startup check completes — typically within a few seconds; a client that reads `server.agent_manifests` immediately after starting a server races this update. A server whose catalog URL is unreachable stays on all-`"bundled"` manifests indefinitely (no retry observed within 31 s), with the failure visible only in the top-level `last_result`; per-agent `remote_update_error` stays absent even then. (Validated 2026-09-19 against herdr 0.9.1; not reproduced 2026-10-06 on herdr 0.9.3: a scratch server with an empty relocated state directory already reported `last_result: "checked"` and 22 `"remote"` manifests on the first read, made as soon as its socket appeared, so the race window was shorter than that read. The unreachable-catalog case was not re-probed.)

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

Validated 2026-10-06 against herdr 0.9.3 for the result shape, the 22-entry count, the key set of a normally-updated server (every entry `"remote"`, no `warning`, no `remote_update_error`), and the CLI row (`--json` envelope with alphabetical keys). The local-override, malformed-override, and `warning` claims, the override capture above, and the `!` marker in the human table were not re-probed, because creating an override writes to the shared `~/.config/herdr/agent-detection/`; they stay Validated 2026-09-19 against herdr 0.9.1. (`remote_update_error` could not be provoked — see the field note above.)

## server.live_handoff

Hands the running server off to a replacement binary without dropping session state: the current process launches (or execs into) another herdr executable and transfers ownership of the live sessions, so attached clients and agents continue across the swap. This is the mechanism behind `herdr update --handoff` and `--handoff` on remote attach. The optional guard fields let the caller assert what it expects the incoming binary to be — refusing the handoff if the target's protocol or version does not match — and to name the executable to hand off to.

Calling this method with empty params (`{}`) is not a no-op or a dry run: it performs a real handoff using the default/updated binary. After a successful handoff the process's argv changes from `herdr --session <name> server` to `<abs path>/herdr server --handoff-import <session-dir>/herdr-handoff-<oldpid>.sock <oldpid>-<nonce>` (the default binary is the installed one, e.g. `/home/penguin/.local/bin/herdr`), reparented away from the caller (`ppid` 1, or the user's subreaper — `systemd --user` in the 0.9.3 probe), so tooling that finds servers with `pgrep -f "herdr --session <name>"` stops finding them. Session state (workspaces, tabs, panes, focus, counts) is unchanged across the swap, but every pane's `terminal_id` is reissued, and every OPEN connection — including an `events.subscribe` stream — is silently dropped with no event and no close reason (the subscriber reads EOF); a client must reconnect and, if it was subscribed, re-subscribe. After the handoff, `ping` reports `detached_server_daemon: true` where the original server reported `false` (see [protocol.md](../protocol.md)). An `import_exe` that spawns but never speaks the handoff protocol (e.g. `/bin/true`) holds the request until a server-side timeout: on 0.9.3 it failed `handoff_failed` with `timed out waiting for handoff import connection` after 30.0 s, in two runs. (The 0.9.1 pass gave up waiting before the timeout and called the wait indefinite. Another 0.9.3 probe of this pass reported about 15 s; the 30.0 s here was measured from the request to the error reply.) The server stays only partly reachable during the wait: `ping`, `workspace.list` and `server.reload_config` still answer, but `pane.list` blocks and answers only when the timeout fires. Afterward the original server keeps running normally.

**Params** — `ServerLiveHandoffParams`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `expected_protocol` | integer (uint32) \| null | no | null | If set, require the target binary to speak this protocol version; refused (`handoff_failed`) on mismatch. A negative or out-of-range integer, or a non-integer value, fails `invalid_request` instead. |
| `expected_version` | string \| null | no | null | If set, require the target binary to report this herdr version; refused (`handoff_failed`) on mismatch. A non-string value fails `invalid_request`. |
| `import_exe` | string \| null | no | null | Path to the replacement executable to hand off to; null or omitted uses the default/updated binary. A missing or non-executable path fails `handoff_failed` naming the path and the OS error; a path to a process that never speaks the handoff protocol holds the request for 30 s and then fails `handoff_failed` (see above). |

**Result** — `type: "ok"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"ok"` | yes | — | Success acknowledgement; the process then hands off to the replacement binary (see above) rather than continuing to serve the current connection. |

**Errors**: `handoff_failed` is the only error code observed. A protocol/version guard mismatch returns `{"code":"handoff_failed","message":"handoff stream closed while reading line"}` — the message does not name the mismatch. A bad `import_exe` returns `handoff_failed` with a message naming the path and the OS error, e.g. `failed to spawn handoff import server at /nonexistent/rvprobe-herdr: No such file or directory (os error 2)`, or `Permission denied (os error 13)` for a path that is not executable. An `import_exe` that never connects back fails after 30 s with `timed out waiting for handoff import connection`. Malformed `params` (wrong types, out-of-range integers) fail `invalid_request` instead, before any handoff is attempted. Other codes possible.

**CLI**: `herdr server live-handoff [--import-exe <path>] [--expected-protocol <n>] [--expected-version <version>]`. `herdr server --help` does not list it (it lists only `stop`, `reload-config`, `agent-manifests`, `update-agent-manifests`, and `reload-agent-manifests`); the group's plain usage list does, as "hand off live panes to a new local server". It sends `server.live_handoff` directly with id `cli:server:live-handoff`, with no `ping` preflight, and only the flags given (no flags → `"params":{}`). On success it prints `live handoff complete; server log: <path>` and exits 0, but `<path>` is the default server's log (`~/.config/herdr/herdr-server.log`) even when `HERDR_SOCKET_PATH` points at a `--session` server, whose handoff is logged in its own session directory. On an error it prints the JSON error envelope (keys in alphabetical order) and exits 1. The method is also reached via `herdr update --handoff` (`herdr update --help` documents `--handoff` as "Try live handoff after installing") and `--handoff` on remote/app attach; those entry points were not exercised because `herdr update` installs a new binary into `~/.local/bin`, which is outside the scope of a scratch-session probe. Validated 2026-10-06 against herdr 0.9.3 (the wire requests and the error output were captured against a fake socket that answered locally; one successful CLI handoff ran against the scratch server).

**Example** — matching guards:

```json
{"id":"h1","method":"server.live_handoff","params":{"expected_protocol":22,"expected_version":"0.9.3","import_exe":null}}
{"id":"h1","result":{"type":"ok"}}
```

A guard mismatch fails rather than falling back to an unguarded handoff:

```json
{"id":"h2","method":"server.live_handoff","params":{"expected_protocol":20,"expected_version":"0.8.2"}}
{"id":"h2","error":{"code":"handoff_failed","message":"handoff stream closed while reading line"}}
```

Validated 2026-10-06 against herdr 0.9.3. (On an isolated scratch server: the four malformed-param cases, both bad-`import_exe` cases, three guard mismatches (`expected_protocol` only, `expected_version` only, both), the `/bin/true` timeout (with `pane.list` timed during the wait), two real matching-guard handoffs with a subscriber open, comparing `terminal_id`s, argv and parent process, and one `herdr server live-handoff` CLI handoff. The `herdr update --handoff` CLI entry point and a handoff against a genuinely different herdr version were not exercised — see CLI above.)

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

Validated 2026-10-06 against herdr 0.9.3 for the result shape (only `type` and `manifests`, no `last_check_unix`/`last_result`) and the CLI row (envelope printed on stdout). The example above is the 0.9.1 capture; a 0.9.3 reply has the same shape with current manifest versions. The override-related claims in the first paragraph (where overrides live, no file watcher, a malformed or unknown-agent override absorbed silently) were not re-probed, because they need a file in the shared `~/.config/herdr/agent-detection/`; they stay Validated 2026-09-19 against herdr 0.9.1.

## server.reload_config

Re-reads `config.toml` from disk and applies it to the running server, returning whether the reload fully applied along with any diagnostics produced while parsing or applying the new configuration. This is the socket equivalent of the global menu's "reload config" action and lets configuration edits take effect without restarting the server.

**Params**: `EmptyParams` — `{}`. No fields.

**Result** — `type: "config_reload"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"config_reload"` | yes | — | Result discriminator. |
| `status` | `ConfigReloadStatus` enum | yes | — | Overall outcome. One of: `"applied"` (fully applied), `"partial"` (some settings applied, some rejected), `"failed"` (nothing applied). |
| `diagnostics` | array of string | yes | — | Human-readable messages about settings that were rejected or adjusted; empty on a clean `applied` reload. |

A key placed at the wrong nesting level is reported as merely unknown, not misplaced: `manifest_check = false` at the top level (it belongs under `[update]`) yields `status: "partial"` with `diagnostics: ["unknown config key manifest_check; ignoring key"]`. Unknown keys never fail a reload. (Validated 2026-09-19 against herdr 0.9.1; not re-probed on 0.9.3, because a scratch session reads the shared `~/.config/herdr/config.toml`, which this pass did not edit.)

**Errors**: none observed on a valid config. A malformed config surfaces through `status` (`partial`/`failed`) and `diagnostics` rather than a protocol error. Other codes possible.

**CLI**: `herdr server reload-config`. With no server running it prints a JSON error envelope to stderr instead of failing at the protocol layer, e.g. `{"id":"cli:server:reload-config","error":{"code":"server_not_running","message":"no herdr server is running at <path>; run \`herdr\` to start or attach it"}}`, unlike `server stop`'s plain-sentence error (see below). It also prints the whole response envelope to stdout on success even though it has no `--json` flag.

**Example**

```json
{"id":"cli:server:reload-config","method":"server.reload_config","params":{}}
{"id":"cli:server:reload-config","result":{"type":"config_reload","status":"applied","diagnostics":[]}}
```

Validated 2026-10-06 against herdr 0.9.3 (raw socket and CLI on a scratch server with a valid config, plus the CLI's no-server error; the misplaced-key diagnostic above keeps its 0.9.1 stamp).

## server.ssh_agent.register

New in 0.9.3. Registers a remote-host SSH agent socket with the server. The registration is scoped to the API connection that sent it: per the schema it "lasts until this API connection closes", so a caller sends the request, reads the one-line `{"type":"ok"}` reply, and then keeps the connection open for as long as the registration should last. The server sends nothing more on that connection. `ping` advertises support through the capability flag `ssh_agent_registration` (default `false`; `true` on 0.9.3 — see [protocol.md](../protocol.md)). The schema calls the socket a "remote-host agent socket", which suggests the method serves remote attach (`herdr --remote`), but no first-party caller was observed, because remote attach was not exercised.

On a local scratch server the registration had no observable effect. The session directory's `herdr.sock.agent` symlink, which the server creates at startup pointing at the agent it inherited through `SSH_AUTH_SOCK`, kept its target. A pane's `SSH_AUTH_SOCK` still resolved through that symlink. The registered socket (a listener the probe created) received no connection while the registration was held. The registration did not appear in the server log either. This held for a single registration, for two overlapping registrations on two connections, and after each connection closed. What the registration changes for a remote session is unknown.

**Params** — `ServerSshAgentRegisterParams`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `socket_path` | string | yes | — | Absolute path to the agent socket. It must be an existing Unix socket owned by the calling user: an empty string, a relative path, a missing path, a regular file and a directory all fail `invalid_ssh_agent`. A non-string value fails `invalid_request`. |

**Result** — `type: "ok"`:

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `type` | string const `"ok"` | yes | — | Registration accepted. The connection stays open after this reply. |

**Errors**:

| code | when |
| --- | --- |
| `invalid_ssh_agent` | `socket_path` is not an absolute path to an existing socket owned by the user (message: `SSH agent must be an absolute, user-owned socket`). The other-owner case was not exercised; the message names it. |
| `invalid_request` | `socket_path` missing or not a string. |

Other codes possible.

**CLI**: API-only (no CLI subcommand). `herdr server --help` lists no SSH agent subcommand.

**Example**

```json
{"id":"s1","method":"server.ssh_agent.register","params":{"socket_path":"/home/u/.fledge/tmp/fake-agent.sock"}}
{"id":"s1","result":{"type":"ok"}}
{"id":"s1","method":"server.ssh_agent.register","params":{"socket_path":"relative/agent.sock"}}
{"id":"s1","error":{"code":"invalid_ssh_agent","message":"SSH agent must be an absolute, user-owned socket"}}
```

Validated 2026-10-06 against herdr 0.9.3 (on a scratch server only, with listener sockets the probe created under its own scratch directory; the live server's agent symlink was unchanged before and after). The effect of a registration on a remote session, and the case of a socket owned by another user, were not exercised.

## server.stop

Shuts down the running server via the socket API, terminating all its sessions (a server hosts exactly one session, so this always means the one it hosts). After acknowledging the request the server tears itself down: it closes every connection, including an open `events.subscribe` stream (the subscriber reads EOF with no closing event), and unlinks the socket file — a follow-up connect fails immediately (`FileNotFoundError`) rather than hanging. The CLI prints nothing on success (empty stdout, exit status 0), but does not return as soon as the acknowledgement arrives: it polls by reconnecting to the socket (about 40 empty connections per second, sending nothing) until a connect fails, and if the socket still accepts connections after 15 s, it exits 1 with `server did not stop within 15000ms; sockets are still reachable at <path>`; a normal stop completed in well under a second in testing.

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

Validated 2026-10-06 against herdr 0.9.3. The raw-socket stop (with a subscriber open) was run on a scratch server; the CLI's request id, empty stdout, no-server message and 15 s timeout were captured against a logging fake socket; and the CLI stop of the scratch server at the end of the pass produced empty stdout and exit status 0. `{"type":"ok"}` is the schema's sole non-error variant for this method.
