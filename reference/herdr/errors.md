# herdr API: error handling

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](README.md). Wire format: [protocol.md](protocol.md).

Every herdr request either succeeds with a `result` or fails with a single `error` object
on the same connection. This file documents the error envelope, how errors surface through
the CLI, and every error `code` observed against a live herdr 0.8.2 server, with the trigger
and the probe capture that evidences it. All probe paths below are relative to
`scratchpad/probes/`.

## Error envelope

A failed request returns exactly one JSON object, LF-terminated, then the server closes the
connection (the same one-request-per-connection rule as success responses):

```json
{"id": "<echoed request id>", "error": {"code": "<snake_case>", "message": "<human text>"}}
```

Defined by the `error_response` schema (`ErrorBody`): `error.code` and `error.message` are
both required strings.

- **`id`** echoes the request's `id`. When the request could not be parsed far enough to
  read its `id` (malformed envelope, missing `id`), the server returns `"id": ""` — see the
  `invalid_request` variants below.
- **`error.code`** is a stable, machine-matchable snake_case slug. **The schema does not
  enumerate the code set** — `code` is typed as a free `string` — so the catalog below is
  *observational*, gathered from probes and [raw/skill.md](raw/skill.md), and is **not
  exhaustive**. Match on codes you have handled and treat unknown codes as generic failures.
- **`error.message`** is human-facing text and may include specific IDs/paths (e.g.
  `pane w1:p99 not found`). Do not parse it for control flow; use `code`.

## CLI surfacing

The CLI is a thin client over the socket, so a server error becomes the **same JSON object
printed to stderr**, and the process exits **1**:

```json
{"error":{"code":"pane_not_found","message":"pane w1:p99 not found"},"id":"cli:pane:read"}
```

(Note the CLI sets `id` to a `cli:<group>:<command>` string.) A CLI **syntax** error —
unknown flag, bad argument, missing required option — is caught client-side before any
socket request, is *not* JSON, and exits **2**. Example (`scratch/worktree-remove.err`):

```text
unknown option: --path
```

So callers can distinguish: exit 2 = malformed CLI invocation; exit 1 = server-side error
whose JSON `code` is one of the below.

## Observed error codes

| code | trigger | evidence (probe) |
| --- | --- | --- |
| `workspace_not_found` | A method referenced a workspace ID that does not exist (`workspace w99`). | `scratch/err-ws-get.err` |
| `pane_not_found` | A method referenced a pane ID that does not exist (`pane w1:p99`). | `scratch/err-bad-pane.err` |
| `agent_pane_not_found` | An `agent.*` method targeted a pane that does not exist (`agent target pane w1:p99 not found`). Distinct from `pane_not_found`: raised on the agent-command path when resolving the agent's target pane. | `scratch/agent-start-err.err` |
| `invalid_request` | The request envelope or `params` failed to deserialize. Covers several triggers (see variants below). | `raw/err-bad-params.json`, `raw/err-missing-id.json`, `raw/err-unknown-method.json` |
| `popup_not_open` | `popup.close` (or another popup op) was called when no popup is open. | `raw/popup-close.json` |
| `feature_disabled` | A method needs an experimental/optional feature that is off — here `pane.graphics.info` requires `experimental.kitty_graphics`. | `raw/pane-graphics-info.json` |
| `split_not_found` | `layout.set_split_ratio` was given a split `path` that does not resolve to an existing split. | `raw/layout-set-split-ratio.json` |
| `unsupported_event_wait_match` | `events.wait` was given a `match_event` other than a pane agent-status match; in 0.8.2 only pane agent-status matches are supported, despite the schema allowing broader match shapes. | `raw/events-wait.json` |

### `invalid_request` variants

`invalid_request` is a single code covering all envelope/params deserialization failures.
The `message` distinguishes the cause, and these requests return `"id": ""` because parsing
failed before the `id` was usable:

| variant | message shape | evidence |
| --- | --- | --- |
| unknown method | `invalid request: unknown variant \`bogus.method\`, expected one of \`ping\`, \`server.stop\`, …` (the message lists every valid method name) | `raw/err-unknown-method.json` |
| missing required param field | `invalid request: missing field \`workspace_id\` at line 1 column 66` | `raw/err-bad-params.json` |
| missing envelope field (`id`) | `invalid request: missing field \`id\` at line 1 column 32` | `raw/err-missing-id.json` |

A wrong-type field (e.g. a string where an integer is expected) also surfaces as
`invalid_request` with a serde-style type-mismatch message; treat any `invalid_request` as
a client bug to fix rather than a runtime condition to retry.

## Skill-documented codes (not probed)

[raw/skill.md](raw/skill.md) documents these agent-lifecycle error codes for the
`agent.*` methods. They were not reproduced by the probe sweep, so they are listed here as
**skill-documented** rather than probe-verified:

| code | trigger (per skill.md) |
| --- | --- |
| `agent_not_ready` | `agent.start` returns immediately with this code when the agent is blocked during startup; the name stays available for `agent read` / `agent send-keys`. Wait until the agent becomes idle before prompting. |
| `agent_blocked` | `agent.prompt` rejects an agent already waiting at an approval or question dialog, before sending any input. Inspect the blocked UI and ask the user before answering. |
| `agent_prompt_stalled` | A prompt sent from a non-working state produced no observed lifecycle change within five seconds, so Herdr returns this instead of waiting indefinitely. |

## Notes for implementers

- `error.code` is a free-form string in the schema; the two lists above are observational
  and **not exhaustive**. Other methods can return codes not seen here — handle unknown
  codes gracefully.
- Match on `code`, never on `message`.
- Reserve exit-2 handling for CLI syntax errors; every server error is exit 1 with a JSON
  body on stderr.
- After any response — success or error — the connection is closed; open a fresh connection
  per request (except a `events.subscribe` connection, which stays open).
