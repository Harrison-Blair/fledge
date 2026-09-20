# herdr API Reference

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-19

Exhaustive, agent-optimized documentation of the **herdr** socket API, written for
implementing fledge's herdr client. herdr is a terminal workspace manager for AI coding
agents: a client/server daemon whose server exposes a JSON request/response protocol over
a **Unix domain socket** (`$HERDR_SOCKET_PATH`) — not HTTP. The `herdr` CLI is a thin
client over the same socket.

Every fact here was derived from the binary's own emitted contract and validated against
a live server where marked. When these docs and `raw/schema.json` disagree, the schema
wins — and the docs have a bug.

## Reading order for implementers

1. [protocol.md](protocol.md) — transport, framing, envelopes, **one request per
   connection**, ping/pong handshake, version negotiation.
2. [addressing.md](addressing.md) — the `w1` / `w1:t1` / `w1:p1` ID scheme and targeting rules.
3. [environment.md](environment.md) — ambient access model (`HERDR_SOCKET_PATH`,
   `HERDR_ENV`, caller-context IDs); there is no token auth.
4. [data-model.md](data-model.md) — every domain entity (WorkspaceInfo, PaneInfo,
   AgentInfo/AgentStatus, LayoutNode, …).
5. [errors.md](errors.md) — error envelope and observed error codes.
6. [events.md](events.md) — `events.subscribe` / `events.wait`, subscription semantics,
   and the full event catalog.
7. [cli-mapping.md](cli-mapping.md) — socket method ⇄ CLI command mapping, CLI-only
   commands, CLI conventions.

## Method reference (103 methods)

| File | Namespace | Methods |
|---|---|---|
| [api/agent.md](api/agent.md) | `agent.*` | 12 |
| [api/pane.md](api/pane.md) | `pane.*` | 37 |
| [api/workspace.md](api/workspace.md) | `workspace.*` | 9 |
| [api/tab.md](api/tab.md) | `tab.*` | 7 |
| [api/worktree.md](api/worktree.md) | `worktree.*` | 4 |
| [api/layout.md](api/layout.md) | `layout.*` (API-only, no CLI group) | 3 |
| [api/plugin.md](api/plugin.md) | `plugin.*` (`herdr plugin` CLI group new in 0.9.1) | 11 |
| [api/server.md](api/server.md) | `server.*` | 5 |
| [api/session.md](api/session.md) | `session.snapshot`, `ping` | 2 |
| [api/integration.md](api/integration.md) | `integration.*` | 3 |
| [api/ui.md](api/ui.md) | `notification.show`, `popup.close`, `client.window_title.*`, `client_shell.surface.set`, `command.invoke`, `product_announcement.dismiss`, `release_notes.dismiss` | 8 |
| [events.md](events.md) | `events.*` | 2 |

## Raw artifacts (ground truth, `raw/`)

| File | Origin |
|---|---|
| [raw/schema.json](raw/schema.json) | `herdr api schema --json` — the canonical machine-readable contract (~275 KB) |
| [raw/skill.md](raw/skill.md) | `herdr --skill` — the agent-integration guide bundled in the binary |
| [raw/default-config.toml](raw/default-config.toml) | `herdr --default-config` |
| [raw/agent-guide.md](raw/agent-guide.md) | fetched from https://herdr.dev/agent-guide.md |
| [raw/llms.txt](raw/llms.txt) | fetched from https://herdr.dev/llms.txt |

## Provenance and validation

- Params, results, and types come from `raw/schema.json` (herdr 0.9.1).
- Two evidence stamps appear on individual claims and examples throughout these pages.
  **Validated YYYY-MM-DD against herdr X.Y.Z** means the claim was actually exercised:
  a real captured exchange, read-only methods against a live session and mutating
  methods against an isolated scratch server (`herdr --session <name> server`).
  **Constructed from schema; not live-validated** means the claim comes only from
  `raw/schema.json`'s shapes and was never exercised, including methods a pass
  deliberately skipped because probing them would write outside an isolated scratch
  session (e.g. `integration.install`/`integration.uninstall`), noted inline with the
  reason. A section carries the Validated stamp only for the parts of it actually
  exercised in that pass, so dates can differ section-to-section within a page as it is
  re-verified over time; see [RESYNC.md](RESYNC.md) for how a resync pass decides which.
- herdr self-updates (`herdr update`); these docs describe **0.9.1 / protocol 22** and
  will drift. See [RESYNC.md](RESYNC.md) for the refresh procedure.
