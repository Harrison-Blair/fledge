# herdr API: environment and access model

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](README.md). Wire format: [protocol.md](protocol.md). IDs: [addressing.md](addressing.md).

herdr has no token, session-key, or API-key authentication. A client is authorized by two ambient facts: it can open the server's Unix socket (an OS filesystem-permission check), and — when it runs inside a managed pane — herdr has injected that pane's identity into its environment. This file documents the injected environment variables, the socket path and its permissions, the explicit absence of any credential auth, the on-disk session directory layout, and how named sessions provide isolation. Sources: environment variables observed in a live herdr pane, `raw/skill.md`, `raw/agent-guide.md`, `raw/default-config.toml`, and the top-level CLI help.

## Environment variables

herdr injects the caller's context into each managed pane, so a process can discover both where it is and how to reach the server without any configuration. Observed variables:

| variable | meaning | source |
| --- | --- | --- |
| `HERDR_ENV` | Set to `1` inside a herdr-managed pane. The gate for all control: a client must verify `test "${HERDR_ENV:-}" = 1` before issuing commands, and stop if it is unset. | skill.md §top; agent-guide.md |
| `HERDR_SOCKET_PATH` | Absolute path to the server's Unix domain socket for this session. The transport endpoint (see [protocol.md](protocol.md)). | observed in a live pane |
| `HERDR_SESSION` | Name of the session this pane belongs to (e.g. `fledge-dev`). Selects which server/socket/log set under the session directory. | observed in a live pane |
| `HERDR_WORKSPACE_ID` | Public ID of the workspace hosting the calling pane (e.g. `w1`). Feeds `--workspace` and target resolution. | skill.md §"Use IDs and caller context" |
| `HERDR_TAB_ID` | Public ID of the tab hosting the calling pane (e.g. `w1:t1`). | skill.md |
| `HERDR_PANE_ID` | Public ID of the calling pane itself (e.g. `w1:p1`). Equivalent to `--current` for pane commands. | skill.md |
| `HERDR_BIN_PATH` | Absolute path to the `herdr` binary that matches this server, so scripts invoke the right client. | observed in a live pane |
| `HERDR_CONFIG_PATH` | Overrides the config file path. Documented in `herdr --help` ("HERDR_CONFIG_PATH overrides config file path"). | CLI help |

Usage notes:

- The three ID variables are the injected caller context. Prefer `--current` (or `--pane "$HERDR_PANE_ID"`) over omitting a target; an omitted target may act on another client's UI-focused pane. See [addressing.md](addressing.md).
- A moved process keeps its inherited `HERDR_PANE_ID`, so *inside* that process the old pane ID still resolves even after a `pane move` re-identifies the pane for external callers.
- `HERDR_ENV` is a presence/gate flag; the rest are data. Read them; do not assume them when unset.

## Socket path and permissions

- The server listens on a Unix domain **stream** socket at `$HERDR_SOCKET_PATH`. For the `fledge-dev` session the observed path is `/home/penguin/.config/herdr/sessions/fledge-dev/herdr.sock`; `herdr status server` reports the same path under `socket:`.
- The socket file is created with permission mode **`0600`** (owner read/write only, observed). Only the owning OS user can connect. There is no network listener — the socket is local to the host (remote control is tunneled over SSH via `herdr --remote`, not by exposing the socket).
- A companion client socket `herdr-client.sock` sits alongside it in the session directory (used for client↔server coordination).

## No token or API-key authentication

herdr performs **no credential-based authorization**. There is explicitly:

- **no** bearer token, API key, or shared secret in the request envelope — the [request envelope](protocol.md#request-envelope) is only `id` + `method` + `params`;
- **no** login, session-token, or handshake step that mints a credential (the `ping` handshake exchanges version/protocol/capabilities, not auth — see [protocol.md](protocol.md));
- **no** per-request signature or authorization header.

Authorization is entirely: (1) **OS socket permissions** — you must be the owning user and able to open the `0600` socket path; plus (2) **injected context** — herdr trusts the workspace/tab/pane identity it placed in a managed pane's environment. Any process that can read `$HERDR_SOCKET_PATH` and open that socket is fully authorized to drive the server. Treat filesystem access to the session directory as equivalent to full control of the session, and rely on OS user isolation, not on herdr, for access control.

## Session directory layout

Each named session owns a directory under the herdr config root. On Linux/macOS the config root is `~/.config/herdr/` (`%APPDATA%\herdr\` on Windows; agent-guide.md §troubleshooting). The default session's files live directly in the config root; a **named** session gets its own subdirectory:

```text
~/.config/herdr/
├── config.toml                     # user configuration (HERDR_CONFIG_PATH overrides)
└── sessions/
    └── <name>/                     # one directory per named session, e.g. fledge-dev/
        ├── herdr.sock              # server API socket  ($HERDR_SOCKET_PATH), mode 0600
        ├── herdr-client.sock       # client↔server coordination socket
        ├── herdr.log               # combined session log
        ├── herdr-client.log        # client-side log
        └── herdr-server.log        # server-side log
```

The `herdr --help` footer for the `fledge-dev` session confirms the log set: `Logs: /home/penguin/.config/herdr/sessions/fledge-dev/herdr.log (plus herdr-client.log, herdr-server.log)`, and agent-guide.md confirms "Named-session logs live under `sessions/<name>/`". `herdr status`, `herdr status server`, and `herdr status client` summarize the runtime and print the resolved socket path.

## Named sessions as isolation

A session is the unit of isolation: one server process, one socket, one workspace/tab/pane namespace, one log set, one directory.

- **Selecting a session:** `herdr --session <name>` uses or creates a named persistent session; `herdr session attach <name>` attaches to it. `herdr session list [--json]`, `herdr session stop <name>`, and `herdr session delete <name>` manage them. Inside a pane, `$HERDR_SESSION` names the current one.
- **Isolation boundary:** each session has its own socket path and its own ID space, so `w1` in one session is unrelated to `w1` in another, and a client bound to one socket cannot see or affect another session's topology. Because authorization is just socket access, separate sessions are also separate access domains for anyone who can be restricted to one directory.
- **Use for experiments:** skill.md is explicit — "Use named test sessions for experiments that need an isolated server," and "Never kill the main Herdr process." Run mutating or destructive probing against a scratch named session so a mistake cannot touch the primary session's panes and processes. `herdr server stop` and `--no-session` (monolithic, no server/client) are escape hatches to use deliberately, not in an active shared session.
