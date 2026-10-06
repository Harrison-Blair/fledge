# herdr API: wire protocol

> herdr 0.9.3 · protocol 22 · schema_version 1 · captured 2026-10-06
> Part of the fledge herdr reference. Index: [README.md](README.md). IDs: [addressing.md](addressing.md). Access model: [environment.md](environment.md).

herdr exposes a running server through a Unix domain socket that speaks newline-delimited JSON. A client opens the socket, writes exactly one request line, reads one response line, and the server closes the connection — except for a connection that called `events.subscribe`, which stays open to receive pushed event lines. There is no HTTP, no framing header, and no multiplexing: each connection carries one request/response exchange (or one long-lived subscription). This file documents the transport, the three envelope shapes, connection semantics, the ping/pong handshake, protocol/version negotiation, and how subscription connections differ. Per-method params and results live in the `api/*.md` files.

## Transport

- The server listens on a Unix domain stream socket whose path is exported to every managed pane as `$HERDR_SOCKET_PATH` (see [environment.md](environment.md)). For the default (unnamed) session this is the socket directly under the config dir, e.g. `/home/penguin/.config/herdr/herdr.sock`. A named session's socket instead lives under `sessions/<name>/`, e.g. `/home/penguin/.config/herdr/sessions/<name>/herdr.sock`. The same path is reported by `herdr status server` under `socket:`. Validated 2026-10-06 against herdr 0.9.3.
- The socket file is created with mode `0600` (owner read/write only; Validated 2026-10-06 against herdr 0.9.3); authorization is OS filesystem permission on that path plus the context herdr injects into managed panes. There is no token, session cookie, or API key. See [environment.md](environment.md).
- The `herdr` CLI is a thin client over this same socket. A server-side error surfaces on stderr with process exit status 1 as the same error body, but re-serialized by the CLI with its own id — not byte-identical to the socket response: key order is reversed (`error` before `id`) and `id` is the CLI's own synthetic token, not the caller's. For example, `herdr workspace get w999` (exit 1) printed `{"error":{"code":"workspace_not_found","message":"workspace w999 not found"},"id":"cli:workspace:get"}`, while the equivalent socket request returned `{"id":"cmp","error":{"code":"workspace_not_found","message":"workspace w999 not found"}}`. A CLI-syntax error (bad flags/arguments, never sent to the server) exits with status 2: `herdr workspace get` with no argument prints `usage: herdr workspace get <workspace_id>`, and an unknown subcommand prints the group's command list. An unknown `--flag` in an argument position is not always a syntax error: `herdr workspace get --bogus` sends `--bogus` as the ID and fails with `workspace_not_found` (exit 1). Validated 2026-10-06 against herdr 0.9.3.
- Byte encoding is UTF-8. Requests and responses are compact JSON objects; the server does not require pretty-printing and emits compact single-line JSON. Validated 2026-10-06 against herdr 0.9.3.

## Framing

- Both directions are **newline-delimited JSON**: one complete JSON object per line, terminated by a single LF (`\n`).
- A request is one line written to the socket. A response is one line read back. There is no length prefix and no envelope beyond the JSON object itself.
- A subscription connection reads multiple lines back over time: first one acknowledgement line, then zero or more pushed event lines, each its own LF-terminated JSON object.
- Clients must read a full line (up to and including LF) before parsing. The server writes each object followed by LF and does not split an object across writes at the protocol level.
- Malformed input at the framing level is handled by silently resetting the connection, not by the `invalid_request` envelope the Error envelope section describes for parse/validation failures. Confirmed cases: a request line over 1 MiB (1,048,577 bytes including the LF is answered, 1,048,578 is not — zero response bytes, connection reset); an empty or whitespace-only first line (`\n`, `\r\n`, `   \n`) closes the connection with zero response bytes, and a blank line followed by a valid request on the same connection consumes the connection's one request slot so the valid line is never answered (zero response bytes, connection reset); invalid UTF-8 bytes anywhere in the request line likewise produce zero response bytes and an immediate close. Validated 2026-10-06 against herdr 0.9.3.

## Request envelope

Every request is a JSON object with these top-level fields:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `id` | string | yes | Client-chosen correlation token. The server echoes it verbatim in the response's `id`, with one known exception: `events.wait`'s `pane_not_found` error path appends a fixed `:sub:0:probe` suffix instead of echoing it verbatim (see `events.wait` under Subscription connections, below). Missing `id` is rejected with `invalid_request` (see below). May be any string; uniqueness is the client's concern. |
| `method` | string | yes | Method name, e.g. `ping`, `workspace.list`, `pane.split`, `agent.prompt`, `events.subscribe`. Must be one of the known variants; an unknown name is rejected with `invalid_request` and the message enumerates every accepted method. A missing `method` is rejected with `invalid_request` (`missing field \`method\``). |
| `params` | object | yes | Method parameters. Required even for methods that take no arguments — send `{}` (schema `PingParams`, `EmptyParams`, etc. are empty objects); a missing `params` is rejected with `invalid_request` (`missing field \`params\``). Each method's `params` shape is defined by its schema variant and documented in the `api/*.md` files. Unknown extra keys, both at the top level and inside `params`, are ignored: `{"id":"a10","method":"ping","params":{},"extra":1}` and `{"id":"a11","method":"ping","params":{"x":1}}` both return `pong` (Validated 2026-10-06 against herdr 0.9.3). |

Schema note: the top-level `request` schema is a `oneOf` over one object per method; each variant requires `method` and `params`. The `id` field is validated by the server (a missing `id` yields `invalid_request`, probe below) but the JSON-Schema variant objects list only `method` and `params` as `required`. Treat all three as required when constructing requests.

```json
{"id":"r8","method":"agent.list","params":{}}
```

Validated 2026-10-06 against herdr 0.9.3 (the identical `agent.list` request line was sent to a scratch server and accepted; first captured 2026-08-19 against 0.8.2).

## Success envelope

A successful call returns one line:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `id` | string | yes | The request's `id`, echoed. |
| `result` | object | yes | The result body. Always carries a `"type"` string discriminant naming the result variant; remaining fields depend on that type. |

The `result` object is the `ResponseResult` `oneOf` in the schema. Each variant is tagged by a `type` const — e.g. `pong`, `session_snapshot`, `workspace_info`, `workspace_list`, `workspace_created`, `tab_list`, `pane_info`, `pane_current`, `pane_list`, `pane_move`, `agent_info`, `agent_list`, `ok`. A method with no meaningful payload returns `{"type":"ok"}` (observed for `pane.report_agent`). Per-method result field tables live in the `api/*.md` files; shared entities (`WorkspaceInfo`, `TabInfo`, `PaneInfo`, `AgentInfo`, …) are defined once in the data-model reference.

```json
{"id":"r8","result":{"type":"agent_list","agents":[]}}
```

Validated 2026-10-06 against herdr 0.9.3 (a scratch-server `agent.list` call returned exactly `{"id":"r8","result":{"type":"agent_list","agents":[]}}`; first captured 2026-08-19 against 0.8.2).

## Error envelope

A failed call returns one line with an `error` object in place of `result`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `id` | string | yes | Echo of the request's `id`, including on most parse/validation failures (unknown method, missing `method`/`params`, malformed params). The server returns `"id": ""` (empty string) only when it cannot recover a single string `id` from the line: the line is not valid JSON, `id` is missing, `id` is not a string, or `id` appears twice. |
| `error` | object | yes | Error body (`ErrorBody`). |
| `error.code` | string | yes | Stable machine-readable code in `snake_case`, e.g. `invalid_request`, `agent_blocked`, `agent_not_ready`, `agent_prompt_stalled`, `timeout`, `unsupported_event_wait_match`. Branch on this. |
| `error.message` | string | yes | Human-readable explanation. Not stable; do not parse. |

Parse/validation failures all use code `invalid_request`. Evidence:

```json
{"request":{"method":"ping","params":{}},"response":{"id":"","error":{"code":"invalid_request","message":"invalid request: missing field `id` at line 1 column 29"}}}
```

```json
{"request":{"id":"r9","method":"bogus.method","params":{}},"response":{"id":"r9","error":{"code":"invalid_request","message":"invalid request: unknown variant `bogus.method`, expected one of `ping`, `server.stop`, `server.live_handoff`, … "}}}
```

```json
{"request":{"id":"r11","method":"workspace.get","params":{"nope":true}},"response":{"id":"r11","error":{"code":"invalid_request","message":"invalid request: missing field `workspace_id` at line 1 column 60"}}}
```

The column in these messages is a byte offset into the request line as sent, so it changes with key order and spacing; do not parse it. Cases that still return `"id": ""`:

```json
{"request":"{\"id\":\"a5\",\"method\":\"ping\",\"params\":{}","response":{"id":"","error":{"code":"invalid_request","message":"invalid request: EOF while parsing an object at line 1 column 38"}}}
{"request":{"id":7,"method":"ping","params":{}},"response":{"id":"","error":{"code":"invalid_request","message":"invalid request: invalid type: integer `7`, expected a string at line 1 column 7"}}}
{"request":"{\"id\":\"a12\",\"id\":\"a13\",\"method\":\"ping\",\"params\":{}}","response":{"id":"","error":{"code":"invalid_request","message":"invalid request: duplicate field `id` at line 1 column 16"}}}
```

Validated 2026-10-06 against herdr 0.9.3 (all examples replayed on a scratch server). **Changed from 0.9.1**, where every parse failure, including an unknown method or malformed params, returned `"id": ""`. On 0.9.3 a client can correlate a parse error by `id` whenever the request line is valid JSON with exactly one string `id`; it must still accept `""` for the cases above. The unknown-variant message enumerates 102 methods (`ping`, `server.stop`, `server.live_handoff`, … first), exactly the set in `raw/schema.json`'s 102-method request `oneOf`; the undocumented `pane.graphics.stream` that 0.9.1 also accepted is gone. Calling a removed graphics method (`pane.graphics.set`, `pane.graphics.stream`) does not return `invalid_request`: it returns a separate code, `unknown_method`, with the `id` echoed: `{"id":"g1","error":{"code":"unknown_method","message":"unknown method: pane.graphics.set"}}`. A name herdr never had (`nope.nope`) still returns `invalid_request` with the unknown-variant message.

## One request per connection

The server serves **exactly one request per connection** and then closes it. After the single response line is written, the socket is closed; a second request written on the same connection is never read and never answered.

Probe: two requests were written back-to-back on one connection. Only the first produced a response; the second produced nothing before close.

```json
{
  "first":  [ {"id":"m1","result":{"type":"pong","version":"0.8.2","protocol":20,"capabilities":{"live_handoff":true,"detached_server_daemon":false}}} ],
  "second": null
}
```

The capture above is from herdr 0.8.2 (2026-08-19). Validated 2026-10-06 against herdr 0.9.3: the re-run answered the first `ping`, then reset the connection without answering the second line. A client that needs to issue N calls must open N connections (this is exactly what the CLI does per invocation). The sole exception is `events.subscribe` — see below — and that exception is read-only: writing any second line on a subscribe connection resets it (the next read fails with a connection reset), losing the subscription. Validated 2026-10-06 against herdr 0.9.3.

## Ping/pong handshake

`ping` is the liveness and capability probe. Params are an empty object. The response is a `pong` result carrying the server's version, protocol number, and capability flags:

| result field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | const `"pong"` | yes | Result discriminant. |
| `version` | string | yes | Server release version, e.g. `"0.9.3"`. |
| `protocol` | integer (uint32) | yes | Wire protocol number, e.g. `22`. Compare against the protocol you were built for. |
| `capabilities` | object \| null | no (default `null`) | `ServerCapabilities`. When present: `live_handoff` (boolean, required) — server can hand off a live session across an update/attach; `detached_server_daemon` (boolean, default `false`) — server runs as a detached daemon; `health_check` (boolean, default `false`) — server supports endpoint health probes; `surface_interest` (boolean, default `false`) — server supports explicit client-shell surface interest; `endpoint_protocol_generation` (integer (uint32) \| null, optional, no default) — stable client-owned endpoint generation supported by this server; `ssh_agent_registration` (boolean, default `false`; new in 0.9.3's schema) — server supports connection-scoped `server.ssh_agent.register` on the local JSON API. |

Canonical exchange — request then response:

```json
{"id":"m1","method":"ping","params":{}}
{"id":"m1","result":{"type":"pong","version":"0.9.3","protocol":22,"capabilities":{"live_handoff":true,"detached_server_daemon":false,"endpoint_protocol_generation":1,"surface_interest":true,"health_check":true,"ssh_agent_registration":true}}}
```

Validated 2026-10-06 against herdr 0.9.3 (captured live on a scratch server; a read-only `ping` to the live default socket returned the same capability set except `detached_server_daemon: true`). Use `ping` as the first call of a client to confirm the server is reachable and to read `protocol`/`version`/`capabilities` before relying on version-specific behavior.

## Protocol and version negotiation

herdr does not negotiate the protocol mid-connection; a client instead **reads** the server's protocol number and version and decides for itself whether it is compatible.

- The document header reports `protocol` and `schema_version` (this schema: protocol 22, schema_version 1). Every `pong`, and the `session.snapshot` result, echo `protocol` and `version`, so a client can check compatibility from either. Validated 2026-10-06 against herdr 0.9.3.
- `herdr status` (plain text) prints nested `client:`/`server:`/`update:` blocks with `private_protocol_compatible`/`endpoint_compatible` verdicts and `restart_needed` plus `server_binary_stale` under `update:`; `herdr status --json` exposes `client.protocol`/`server.protocol` plus `server.compatible`, `server.restart_needed`, and `server.server_binary_stale` (nested, not top-level; `update.restart_needed`/`update.server_binary_stale` repeat the last two). Client and server sharing protocol 22 report `private_protocol_compatible: yes` (`server.compatible: true` in `--json`). Validated 2026-10-06 against herdr 0.9.3.
- The one place a client asserts an expected protocol/version is the live-handoff request. `server.live_handoff` takes `ServerLiveHandoffParams`:

  | field | type | required | meaning |
  | --- | --- | --- | --- |
  | `expected_protocol` | integer (uint32) \| null | no | Protocol number the caller was built against. |
  | `expected_version` | string \| null | no | Version string the caller expects. |
  | `import_exe` | string \| null | no | Path to the executable to hand the live session to. |

  All three fields are optional; omitting all of them is accepted and the server proceeds into the handoff path anyway. Validated 2026-09-19 against herdr 0.9.1 (not re-probed with all fields omitted on 0.9.3).

- With no `import_exe`, `expected_protocol` does not gate a documented rejection error. A `server.live_handoff` call whose `expected_protocol` **matches** the live protocol (22) and carries no `import_exe` returns `{"type":"ok"}` and performs a real handoff to the server's own binary: the original server process exits, and a new process, `herdr server --handoff-import <session dir>/herdr-handoff-<old pid>.sock <token>`, takes over the same socket path, after which `ping` reports `detached_server_daemon: true`. The identical call with a **mismatched** `expected_protocol` (e.g. `1`) fails with `handoff_failed` ("handoff stream closed while reading line"), leaves the original process running, and leaves `detached_server_daemon` at `false`:

  ```json
  {"id":"lh-mismatch","method":"server.live_handoff","params":{"expected_protocol":1}}
  {"id":"lh-mismatch","error":{"code":"handoff_failed","message":"handoff stream closed while reading line"}}
  {"id":"post-lh-ping","method":"ping","params":{}}
  {"id":"post-lh-ping","result":{"type":"pong","version":"0.9.3","protocol":22,"capabilities":{"live_handoff":true,"detached_server_daemon":false,"endpoint_protocol_generation":1,"surface_interest":true,"health_check":true,"ssh_agent_registration":true}}}
  ```

  ```json
  {"id":"lh-match","method":"server.live_handoff","params":{"expected_protocol":22}}
  {"id":"lh-match","result":{"type":"ok"}}
  {"id":"post-lh-ping","method":"ping","params":{}}
  {"id":"post-lh-ping","result":{"type":"pong","version":"0.9.3","protocol":22,"capabilities":{"live_handoff":true,"detached_server_daemon":true,"endpoint_protocol_generation":1,"surface_interest":true,"health_check":true,"ssh_agent_registration":true}}}
  ```

  Validated 2026-10-06 against herdr 0.9.3 (both directions on one scratch server, mismatch first; the process replacement was confirmed with `pgrep` and `ss -xlp`). The 0.9.1 pass recorded only the capability flip and described it as the server detaching into daemon mode; it did not check the process table. Two consequences for a caller: the server PID changes, and the replacement process command line no longer contains `--session <name>`, so a cleanup that finds a scratch server with `pgrep -f "session <name>"` misses it — find it by socket (`ss -xlp | grep <session dir>`) instead. With `import_exe` set to `/bin/true`, `server.live_handoff` sent no response within a 15 s client read timeout, for both a matching and a mismatching `expected_protocol`, and the original server kept running with `detached_server_daemon: false`. Validated 2026-10-06 against herdr 0.9.3. The server does answer later: the [api/server.md](api/server.md#serverlive_handoff) pass waited longer and measured `handoff_failed` (`timed out waiting for handoff import connection`) 30.0 s after the request, twice. Whether a handoff to a different binary performs its own protocol check remains unverified: completing one would replace the server with that binary.

- `grep expected_protocol schema.json` matches only inside `ServerLiveHandoffParams`; it is the sole request field that carries a caller-asserted protocol. General method calls do not send a protocol and are not version-gated at the envelope level — a wrong-protocol client simply risks a method the server does not recognize or a required field it does not send, which surfaces as `invalid_request`. An extra field the server does not recognize does not surface at all: it is silently ignored (see Request envelope), so a field added in a newer protocol is dropped without an error by an older server. Validated 2026-10-06 against herdr 0.9.3 (`raw/schema.json` re-checked).

## Subscription connections

`events.subscribe` turns the connection into a long-lived push channel — the one exception to one-request-per-connection. The client sends a subscribe request; the server replies with an acknowledgement result and then keeps the connection open, writing one JSON line per matching event until the client disconnects.

Request `params` (`EventsSubscribeParams`): `subscriptions` (array, required) — a list of `Subscription` objects. 24 of the 27 selectors are bare `{"type": <event-name>}` objects, but the three streaming pane selectors need more fields: `pane.agent_status_changed` and `pane.scroll_changed` also require `pane_id`, and `pane.output_matched` also requires `pane_id`, `source`, and `match`. Omitting a required field is rejected with `invalid_request` (e.g. `{"type":"pane.output_matched"}` alone fails with `missing field \`pane_id\``). Validated 2026-10-06 against herdr 0.9.3 (the `pane.output_matched` case; the `pane_id` requirement of the other two is from `raw/schema.json`). The acknowledgement result type is `subscription_started`.

```json
{
  "subscribe_request": {"id":"e1","method":"events.subscribe","params":{"subscriptions":[{"type":"tab.created"},{"type":"tab.closed"}]}},
  "ack": [ {"id":"e1","result":{"type":"subscription_started"}} ],
  "pushes": [
    {"event":"tab_created","data":{"type":"tab_created","tab":{"tab_id":"w1:t1","workspace_id":"w1","number":1,"label":"1","focused":true,"pane_count":1,"agent_status":"unknown"}}},
    {"event":"tab_created","data":{"type":"tab_created","tab":{"tab_id":"w2:t1","workspace_id":"w2","number":1,"label":"1","focused":false,"pane_count":1,"agent_status":"unknown"}}}
  ]
}
```

Validated 2026-10-06 against herdr 0.9.3 (subscribing to `tab.created`/`tab.closed` on a fresh scratch server and running `workspace.create` then `tab.create` pushed the two `tab_created` lines above with the same field set, `w1:t1` then `w1:t2`; first captured 2026-08-19 against 0.8.2).

How a subscription connection differs from an ordinary one:

- It is **not closed after the first response**. The `subscription_started` ack is followed by an open-ended stream of event lines.
- Event lines use the **event envelope**, not the success envelope: they have `event` and `data`, and **no `id`** (they are server-initiated, not correlated to a request).
- The client ends the subscription by closing the socket. There is no unsubscribe method on the wire.
- The connection is push-only from the server's side once subscribed: writing a second request line on it is not a way to add subscriptions or issue other calls. The server resets the connection (a `ConnectionResetError` on the next read), and the subscription is lost. Validated 2026-10-06 against herdr 0.9.3.

For a one-shot wait instead of a stream, `events.wait` blocks the single response until a matching event (or timeout). It currently only supports pane agent-status matches; other matchers are rejected:

```json
{"request":{"id":"e3","method":"events.wait","params":{"match_event":{"event":"tab_created"},"timeout_ms":5000}},"response":{"id":"e3","error":{"code":"unsupported_event_wait_match","message":"events.wait currently supports pane agent status matches"}}}
```

Validated 2026-10-06 against herdr 0.9.3 (the identical request with `timeout_ms: 500` got the identical response on a scratch server; first captured 2026-08-19 against 0.8.2).

`events.wait`'s `id` echo is not reliable on every error path: its `pane_not_found` error appends a fixed `:sub:0:probe` suffix to the request's `id` instead of echoing it verbatim, e.g. request `{"id":"ZZZ","method":"events.wait","params":{"match_event":{"event":"pane_agent_status_changed","pane_id":"w1:p1","agent_status":"unknown"},"timeout_ms":1000}}` returned `{"id":"ZZZ:sub:0:probe","error":{"code":"pane_not_found","message":"pane w1:p1 not found"}}`. Validated 2026-10-06 against herdr 0.9.3 (with `w9:p9`, a pane that did not exist). `events.subscribe` no longer has this quirk on 0.9.3: its `pane_not_found` errors echo the request `id` unchanged at any item index (see [events.md](events.md#eventssubscribe)).

## Event envelope

Pushed events (from `events.subscribe`) are objects with:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `event` | string | yes | Event kind. The subscription-schema `SubscriptionEventKind` enumerates the streaming pane events: `pane.output_matched`, `pane.agent_status_changed`, `pane.scroll_changed`. Lifecycle events observed on the wire use snake_case names such as `tab_created`. |
| `data` | object | yes | Event payload. For lifecycle events (snake_case names such as `tab_created`) it carries its own `type` discriminant matching the event, e.g. a `tab_created` payload carrying a `TabInfo`. The three streaming pane events do **not** carry a `type` in `data`: `PaneOutputMatchedEvent` is exactly `{pane_id, matched_line, read}`, `PaneAgentStatusChangedEvent` is `{pane_id, workspace_id, agent_status, agent}`, `PaneScrollChangedEvent` is `{pane_id, workspace_id, scroll}` — no discriminant field at all. Confirmed live: subscribing to `pane.output_matched`, then triggering `echo HELLO_MARKER` in the pane, pushed `{"data":{"matched_line":"...HELLO_MARKER","pane_id":"w1:p1","read":{...}},"event":"pane.output_matched"}` with no `type` key anywhere in `data`; `pane.agent_status_changed` (`{"data":{"agent":"rvbot","agent_status":"working","pane_id":"w1:p2","workspace_id":"w1"},"event":"pane.agent_status_changed"}`) and `pane.scroll_changed` pushes likewise had none; `raw/schema.json`'s definitions for all three streaming pane events agree. Validated 2026-10-06 against herdr 0.9.3. |

Event lines never carry an `id`. A lifecycle event can be matched by `data.type`; the three streaming pane events (`pane.output_matched`, `pane.agent_status_changed`, `pane.scroll_changed`) have no `data.type` to key on, so match those by the top-level `event` field alone.
