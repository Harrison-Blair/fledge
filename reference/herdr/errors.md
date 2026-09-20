# herdr API: error handling

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](README.md). Wire format: [protocol.md](protocol.md).

Every herdr request either succeeds with a `result` or fails with a single `error` object
on the same connection. This file documents the error envelope, how errors surface through
the CLI, and every error `code` observed against a live herdr 0.9.1 server, with the trigger
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
- The one-request-per-connection rule is enforced by an active close, not a passive one:
  after a response has been read, writing a second request on the same connection raises a
  hard reset (`ECONNRESET`) rather than being silently dropped or ignored.

Validated 2026-09-19 against herdr 0.9.1.

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

Validated 2026-09-19 against herdr 0.9.1 — both exit paths reproduced byte-for-byte,
including the page's own `pane read w1:p99` example.

## Observed error codes

| code | trigger | evidence (probe) |
| --- | --- | --- |
| `workspace_not_found` | A method referenced a workspace ID that does not exist (`workspace w99`). | `scratch/err-ws-get.err` |
| `pane_not_found` | A method referenced a pane ID that does not exist (`pane w1:p99`). | `scratch/err-bad-pane.err` |
| `agent_pane_not_found` | An `agent.*` method targeted a pane that does not exist (`agent target pane w1:p99 not found`). Distinct from `pane_not_found`: raised on the agent-command path when resolving the agent's target pane. | `scratch/agent-start-err.err` |
| `invalid_request` | The request envelope or `params` failed to deserialize. Covers several triggers (see variants below). | `raw/err-bad-params.json`, `raw/err-missing-id.json`, `raw/err-unknown-method.json` |
| `popup_not_open` | `popup.close` (or another popup op) was called when no popup is open. | `raw/popup-close.json` |
| `feature_disabled` | A method needs an experimental/optional feature that is off. In 0.8.2, `pane.graphics.info` required `experimental.kitty_graphics`; in 0.9.1 `kitty_graphics` defaults to `true` and moved out of `[experimental]` into `[terminal]`, so that specific trigger no longer reproduces on a default config. Re-probed on 0.9.1 against a scratch server with `terminal.kitty_graphics` explicitly set to `false`: `pane.graphics.info`/`.set`/`.clear` all still return it, `pane graphics are disabled by terminal.kitty_graphics`. | `raw/pane-graphics-info.json` (0.8.2 capture), `scratch/pane-graphics-info-disabled.err` (0.9.1, non-default config) |
| `split_not_found` | `layout.set_split_ratio` was given a split `path` that does not resolve to an existing split. | `raw/layout-set-split-ratio.json` |
| `unsupported_event_wait_match` | `events.wait` was given a `match_event` other than a pane agent-status match (`tab_created`, `pane_created`, `workspace_focused`, `pane_agent_detected`, …); on 0.9.1, as in 0.8.2, only pane agent-status matches are supported, despite the schema allowing broader match shapes. A `pane_agent_status_changed` match_event also requires an `agent_status` field — omitting it fails as `invalid_request` before the unsupported-shape check runs. | `raw/events-wait.json` |
| `stale_content` | A method that takes a `content_revision` (optional on `pane.selection.read`, `pane.copy_motion`, `pane.link.activate`; required on `pane.copy_search`) was called with a revision that no longer matches the pane's current content revision. | `raw/pane-selection-stale-content.json` |
| `cell_size_unavailable` | New in 0.9.1. `pane.graphics.info` on a headless server (or any outer terminal that hasn't reported cell size) — now reachable by default since `kitty_graphics` defaults on. | `raw/pane-graphics-info-091.json` |
| `connection_local_only` | New in 0.9.1. `client_shell.surface.set` was called over a plain API connection; it is only available through a client-shell endpoint connection. | `raw/client-shell-surface-set.json` |
| `command_not_found` | New in 0.9.1. `command.invoke` was given a `command_id` the client-shell command manifest does not recognize. The message is always `custom command manifest is stale; reload configuration` — it fires this way even on a freshly-started server whose manifest was never loaded, so "stale/reload" is generic fallback text, not evidence the manifest actually changed. | `raw/command-invoke-not-found.json` |
| `stale_announcement` | New in 0.9.1. `product_announcement.dismiss` was given an `id`/`version` pair that is no longer the current announcement. | `raw/product-announcement-dismiss-stale.json` |
| `stale_release_notes` | New in 0.9.1. `release_notes.dismiss` was given a `version` that is no longer the current release notes version. | `raw/release-notes-dismiss-stale.json` |
| `tab_not_found` | A method referenced a tab ID that does not exist (`tab w1:t99`). Not previously listed here despite being alongside `workspace_not_found`/`pane_not_found`. | `scratch/err-bad-tab.err` |
| `unsupported_agent_kind` | `agent.start` was given a `kind` the server does not recognize as an interactive harness. | `scratch/agent-start-bad-kind.err` |
| `invalid_agent_name` | `agent.start` was given a `name` that does not match the required pattern: must start with a lowercase letter and contain only lowercase letters, digits, `-` or `_` (1–32 characters). | `scratch/agent-start-bad-name.err` |
| `invalid_agent_timeout` | `agent.start`'s `timeout_ms` was outside the accepted range (must be greater than 3000ms and at most 300000ms). | `scratch/agent-start-bad-timeout.err` |
| `agent_not_found` | An `agent.*` method (`agent.get`, `.prompt`, `.wait`, …) targeted a `name` with no matching live agent. Distinct from `agent_pane_not_found`, which is raised when the *pane* doesn't resolve; this is raised when the *name* doesn't. | `scratch/agent-get-unknown.err` |
| `empty_agent_prompt` | `agent.prompt` was given an empty `text`. | `scratch/agent-prompt-empty.err` |
| `agent_name_taken` | New in 0.9.1. `agent.start` was given a `name` already used by another agent whose startup is still `launch_pending`; the message lists that agent's `terminal_id`/`pane_id`/`workspace_id`/`tab_id`/`cwd`/`status` as disambiguation candidates. | `scratch/agent-start-duplicate-name.err` |
| `agent_launch_pending` | New in 0.9.1. `agent.rename` was called on an agent whose `agent.start` launch is still pending; the name cannot change until startup settles. A third state-guard code alongside `agent_not_ready`/`agent_blocked`. | `scratch/agent-rename-pending.err` |
| `layout_not_found` | `layout.set_split_ratio` was given both a `tab_id` and a `pane_id` as the target — the two are mutually exclusive. A different code from `split_not_found`, which covers an unresolvable `path`. | `raw/layout-both-targets.json` |
| `query_too_large` | `pane.copy_search`'s `query` exceeded the server's length limit. | `scratch/copy-search-query-too-large.err` |
| `invalid_metadata_ttl` | `pane.report_metadata`'s `ttl_ms` was out of range: must be at least 1 and at most 86400000. | `scratch/report-metadata-bad-ttl.err` |
| `invalid_metadata_request` | `pane.report_metadata` set neither a field to update nor to clear. | `scratch/report-metadata-empty.err` |
| `invalid_params` | `notification.show` was given an empty `title`. | `scratch/notification-empty-title.err` |
| `not_linked_worktree` | `worktree.remove` targeted a workspace Herdr does not track as a linked worktree checkout. Observed even for a workspace whose `cwd` isn't inside a Git work tree at all — `worktree.remove` doesn't distinguish that case from `not_git_worktree` below the way `worktree.list`/`.create` do. | `scratch/worktree-remove-not-linked.err` |
| `not_git_worktree` | `worktree.list`/`.create` (and presumably other read/create paths) were called against a workspace whose `cwd` is not inside a Git work tree at all. | `scratch/worktree-list-nongit.err` |
| `plugin_not_found` | A `plugin.*` method (e.g. `plugin.pane.open`) referenced a `plugin_id` that isn't linked/known. | `scratch/plugin-pane-open-unknown.err` |
| `invalid_image` | `pane.graphics.set` was given image dimensions that are not both greater than zero. | `scratch/graphics-set-bad-image.err` |
| `invalid_pane_swap` | `pane.swap` was called without either a `direction` or a `source_pane_id`/`target_pane_id` pair. | `scratch/pane-swap-missing.err` |
| `workspace_move_block_failed` | `workspace.move_block` was given an empty `workspace_ids` list. | `scratch/workspace-move-block-empty.err` |
| `timeout` | New in 0.9.1. Generic across any method that takes a `timeout_ms` and finds no match before it elapses — observed on `events.wait` and `pane.wait_for_output`, not only the `agent.prompt --wait --timeout` framing implied below under skill-documented codes. | `scratch/events-wait-timeout.err`, `scratch/pane-wait-for-output-timeout.err` |

Note: `pane.read` requires a `source` field (`visible`, `recent`, or `recent-unwrapped`);
omitting it fails as `invalid_request` (`missing field \`source\``) before pane existence is
even checked, so a bad `pane_id` only surfaces as `pane_not_found` once `source` is present.

Validated 2026-09-19 against herdr 0.9.1.

### `invalid_request` variants

`invalid_request` is a single code covering all envelope/params deserialization failures.
The `message` distinguishes the cause, and these requests return `"id": ""` because parsing
failed before the `id` was usable:

| variant | message shape | evidence |
| --- | --- | --- |
| unknown method | `invalid request: unknown variant \`bogus.method\`, expected one of \`ping\`, \`server.stop\`, …` (the message lists every valid method name) | `raw/err-unknown-method.json` |
| missing required param field | `invalid request: missing field \`workspace_id\` at line 1 column 66` | `raw/err-bad-params.json` |
| missing envelope field (`id`) | `invalid request: missing field \`id\` at line 1 column 32` | `raw/err-missing-id.json` |
| non-finite number | `invalid request: number out of range at line 1 column 82` — a value like `layout.set_split_ratio`'s `ratio: 1e400` (JSON-overflows to infinity) is rejected at parse time, before any semantic range check. | `scratch/layout-ratio-1e400.err` |

A wrong-type field (e.g. a string where an integer is expected) also surfaces as
`invalid_request` with a serde-style type-mismatch message; treat any `invalid_request` as
a client bug to fix rather than a runtime condition to retry.

Validated 2026-09-19 against herdr 0.9.1.

## Skill-documented codes (not probed)

[raw/skill.md](raw/skill.md) documents these agent-lifecycle error codes for the
`agent.*` methods. They were not reproduced by the probe sweep, so they are listed here as
**skill-documented** rather than probe-verified:

| code | trigger (per skill.md) |
| --- | --- |
| `agent_not_ready` | `agent.start` returns immediately with this code when the agent is blocked during startup; the name stays available for `agent read` / `agent send-keys`. Wait until the agent becomes idle before prompting. This `agent.start` behavior is per skill.md and was not observed on 0.9.1 — measured trials, including blocked-startup cases, never saw `agent.start` itself return it. On 0.9.1, `agent.prompt` also returns `agent_not_ready` (message `agent <name> is not an active named agent`) for a target whose `agent.start` launch is still pending (`launch_pending: true`). |
| `agent_blocked` | `agent.prompt` rejects an agent already waiting at an approval or question dialog, before sending any input. Inspect the blocked UI and ask the user before answering. |
| `agent_prompt_stalled` | A prompt sent from a non-working state produced no observed lifecycle change within five seconds, so Herdr returns this instead of waiting indefinitely. |
| `timeout` | New in 0.9.1. Per skill.md, `agent.prompt --wait` with a caller `--timeout` returns this if the caller's timeout expires before `agent_prompt_stalled`'s five-second activity check would fire. The code itself is confirmed and generic — see `timeout` in the code catalog above, probe-verified via `events.wait`/`pane.wait_for_output` — but the specific `agent.prompt --wait --timeout` race was not exercised. |

Validated 2026-09-19 against herdr 0.9.1 (`agent_blocked` and `agent_prompt_stalled` not
exercised — reproducing them needs prompting an agent while it is blocked at a dialog, or
engineering a stalled non-working state, neither attempted this run; `agent_not_ready` and
`timeout` were confirmed, but via different call paths than skill.md's framing here — see
their rows above).

## Notes for implementers

- `error.code` is a free-form string in the schema; the two lists above are observational
  and **not exhaustive**. Other methods can return codes not seen here — handle unknown
  codes gracefully.
- Match on `code`, never on `message`.
- Reserve exit-2 handling for CLI syntax errors; every server error is exit 1 with a JSON
  body on stderr.
- After any response — success or error — the connection is closed; open a fresh connection
  per request (except a `events.subscribe` connection, which stays open).
