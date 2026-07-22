# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
./scripts/build.sh              # go build -> bin/fledge
./scripts/install.sh            # copy to GOBIN (or GOPATH/bin); BINDIR= to override
go test ./...
go test -run TestSpawn ./internal/daemon/   # single test
gofmt -l . && go vet ./...
```

No Makefile or CI. Go 1.26; YAML frontmatter uses `github.com/goccy/go-yaml`.
Unix-only (unix sockets, `setsid`, signal-0 probes).

## cli flags

Hand-rolled parsing (no `flag` package): `takeFlag`/`takeBoolFlag`/`rejectFlags`
in `cmd/fledge/main.go`. One convention throughout:

--[wholeflag]
-[CAPITAL-LETTER]

Short flags are unique across the **entire CLI**, never just within a
subcommand (e.g. `--provider` is `-D` because `-P` is taken by `--pid`).
Check the table in README.md before minting a new one.

## Agent first design

when adding new commands, prompt the user if they want a --json output.
Currently `--json -J` exists on `context scan`, `context graph`, `agent list`,
`agent models`, `agent types`, `init`;
`agent msg wait` is JSON-only.

## What fledge is

A **zero-inference Go orchestrator** for a multi-agent coding stack: Herdr (pane
bus), Pi, and Claude Code. Two invariants:

1. **The Go CLI is the state authority.** Herdr and agent events are *input
   signals*; fledge's append-only journal is truth. Herdr loses metadata across
   server restarts, so it is never durable state.
2. **Zero inference in the orchestrator.** It issues socket commands, consumes
   events, advances deterministic state, and writes its journal — never an LLM
   call. All inference happens inside visible, operator-interactable panes.

## Architecture

One binary, two processes: the CLI, and a per-flock daemon that is the same
binary re-exec'd as `fledge daemon run` (hidden command) under `setsid`. They
meet only over a unix socket speaking newline-delimited JSON, one
request/response per connection (`internal/protocol` is the contract; both
sides import it, plus `client` imports `daemon` for the socket-path helper
only).

- **Flock** = one isolated orchestration session: own daemon, roster, journal,
  socket, herdr session. State: `.fledge/flocks/<name>/`. Socket:
  `$XDG_RUNTIME_DIR/fledge/<workspaceHash>/<flock>.sock` — deliberately outside
  the workspace (108-byte `sun_path` cap; NFS can't bind unix sockets).
- **Flock selection defaults to `FLEDGE_FLOCK`** — `fledge start` exports it
  into every session pane. Operational commands with a positional flock name
  (`flock stop`, `flock status`, `watch`) use that explicit name first; agent
  commands remain scoped to their inherited flock.
- **Managed session recovery and cleanup**: starting a down flock recreates
  its deterministic default Herdr session, so stale panes cannot collide with
  the journal's empty roster. Confirmed `flock clear` stops and deletes each
  target's default managed session and sweeps other session records carrying
  this workspace's managed prefix when no saved flock directory links them.
  Operator-named sessions and other workspaces are never swept.
- **Journal** (`internal/daemon/journal.go`): append-only
  `journal.jsonl`, written **before** the client is ack'd — the core invariant.
  The daemon rebuilds roster + pending messages by replay. Torn final line =
  tolerated; malformed earlier line = corruption, startup fails. Anything not
  journaled must not be left running (spawn failure ⇒ teardown).
- **Three integrations, two shapes** (`internal/daemon/spawn.go`): `claude`
  and `codex` are pane-hosted (`agentcfg.PaneHosted`) — a visible herdr pane,
  input via `pane.send_input` + `keys:["enter"]`, survives daemon restart
  because the pane does; `pi` runs as a supervised `pi --mode rpc` subprocess
  over JSONL (`internal/pirpc`; marked `orphaned` on replay since its pipes
  died with the daemon).
- **Two herdr packages by design**: `internal/herdr` shells out to the herdr
  CLI for session lifecycle (no socket API for that); `internal/herdrwire`
  speaks the socket directly for pane ops. Pinned to herdr 0.7.4 / protocol 16
  with live-verified quirks documented inline.
- **Agent names** are kebab-case `<type>-<species>`, with species drawn from a
  fixed pool (`internal/species`). User definitions and profiles cannot use
  the reserved `fledge-*` namespace. Managed types retain species suffixes;
  exact `fledge-orchestrator` is the singleton exception.
- **Model routing** (`agentcfg.Route`) is a fixed prefix table, never guessed:
  `claude*` → claude integration; `gpt*`/`codex*`/o-series → pi with provider
  `openai-codex`. `agent spawn --integration -I` overrides the route (pi vs
  codex for the same model id). `provider` is pi-only, `permission_mode`
  claude-only, `sandbox` codex-only — validation cross-checks.
- **Definitions and profiles** (`internal/agentcfg`): portable Markdown under
  `.fledge/agents/{user,fledge}/` is authoritative. Synchronization validates
  path/name/namespace rules and atomically writes versioned `agents.json` and
  `fledge-agents.json` indexes with separate agent/profile maps. The generated
  catalog is the third profile source; differing declarations are errors.
- **Model catalog** (`internal/catalog`): `fledge init` probes
  `claude --version`, execs `pi --list-models` / `codex debug models`, and
  regenerates `.fledge/agents/catalog.json` (gitignored, per-machine) wholesale.
  Claude discovery contributes a model-less default plus native Opus, Fable,
  Sonnet, and Haiku launchers;
  Empty discovery keeps the old catalog.
- **Authenticated readiness**: spawn journals `starting`, injects a one-use
  name/token, and sends only the bootstrap instruction. `agent ready` hashes
  and validates the token, journals `agent.ready`, then spawn delivers the
  Markdown role prompt. Sandboxed agents that cannot open the daemon socket
  atomically publish the digest under the flock directory for the daemon to
  validate and consume. Interactive start attaches Herdr before beginning this
  lifecycle. Every post-readiness prompt starts with the daemon-assigned,
  already-registered name and direct-send reply syntax; spawned agents receive
  messages in their sessions rather than polling `agent msg wait`;
  raw profile/model spawns receive that preamble even without an authored role.
  Immediately after `agent.start`, interactive start swaps and focuses the managed
  orchestrator into its final left position before registration or readiness.
- After readiness and role delivery complete, interactive fresh starts keep
  the primary `fledge-orchestrator` workspace as `orchestrator | CLI`, then
  create an unfocused `fledge-watch` workspace rooted at the project. Its
  initial tab is labelled `watch`, and its normal root shell execs the current
  executable as `fledge watch <flock>`; it is not a Herdr or Fledge agent.
  Focus returns to the orchestrator. Setup is transactional: failure closes
  only the created watcher workspace when possible and keeps the healthy flock,
  primary workspace, and CLI. Reattach and scripted starts do not create
  watchers. Timeout or delivery failure tears the transport down.
- **Sandboxed daemon access**: clients try the runtime-directory Unix socket
  first, then the daemon's ephemeral workspace-local `.rpc/` request/response
  bridge. The fallback dispatches the full protocol, so orchestrators can
  spawn, message, wait, list, and stop even when their sandbox denies sockets.
- **Liveness differs by kind**: self-registered agents are probed by pid
  (signal 0; the pid defaults to the *session leader*, not the parent);
  spawned agents change state only on an *observed* event, never inference.
- **Root discovery** (`internal/workspace`): git-style walk up to the nearest
  `.fledge/` directory, then `EvalSymlinks` — client and daemon must agree on
  the canonical path because the hash keys the socket namespace and session
  name.
- `d.mu` must never be held across a herdr call or `runner.Stop` — they take
  seconds. Spawn reserves the name under the lock (pid −1), launches unlocked,
  releases on failure.

README.md documents the full command surface, `.fledge/` tree, and portable
agent format — it is accurate; read it before adding commands.
The `pluma/{plumage,feathers}` dirs are scaffolded but nothing reads them yet.

## `docs/` is a completed legacy experiment

Everything in `docs/` predates the rewrite and documents "Stage 0", which has
been **run to completion**. Its roadmap, ground rules, and repo layout are
historical; do not treat it as the current plan. `docs/handoff-stage0.md` is a
finished commissioning brief — ignore it as instruction entirely.
`docs/reference/*` is a fixed 2026-07-17 snapshot, never edited (ADR-006).

What carries forward is the verified findings (raw: `docs/EXPERIMENTS.md`; wire
facts: ADR-017), now encoded in the code:

- **EXP1** — `pane.report_agent` is metadata-only; native screen detection
  wins. Fledge never seizes agent authority (see `spawn.go` doc comments).
- **EXP2** — `pane.send_input {text, keys:["enter"]}` submits reliably; a bare
  `\r` does not.
- **EXP3** — no practical concurrency ceiling; don't pre-cap concurrent panes.
  Rate limits are handled reactively (ADR-014).

**Git history is not design input** (ADR-010) — don't resurrect designs from
it. That's about *design*, not forensics; reading history to see what was
deleted is fine.

## Versioning

`internal/version/VERSION` is the single source of truth, `//go:embed`-ed by
`version.go` — bumping means editing that one file. No ldflags. It sits inside
the package because embed can't cross directory boundaries; a release workflow
must watch that path, not a root `VERSION`.

## Re-verify before you rely

Herdr, Pi, and Claude Code are fast-moving pre-1.0 surfaces; pinned versions in
`docs/INTEGRATION-CONTRACTS.md` are from 2026-07-17. Check live (`herdr api
schema --json`) before building on any version-specific claim. ADR-017 shows
what drift looks like: client targeted protocol v15, server was 16, five shapes
differed.
