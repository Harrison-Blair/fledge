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

No Makefile, no CI, no dependencies — `go.mod` has zero requires, Go 1.26.
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
Currently `--json -J` exists on `context scan`, `agent list`, `agent models`;
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
- **Flock selection is `FLEDGE_FLOCK` env only** — no override flag, by design,
  so a pane in one flock can't address another. `fledge start` exports it into
  the herdr session it launches.
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
- **Agent names** are `<type>-<species>`, species drawn from a fixed pool of 18
  penguin slugs per type (`internal/species`). Exception:
  `fledge-orchestrator` (`agentcfg.ReservedOrchestrator`) — the only name with
  a hyphen and no species suffix. It is special-cased at **three** validation
  seams (`agentcfg.validName`, `daemon.validType`, and the name-collision logic
  in both register and reserve); miss one and it can't be spawned.
- **Model routing** (`agentcfg.Route`) is a fixed prefix table, never guessed:
  `claude*` → claude integration; `gpt*`/`codex*`/o-series → pi with provider
  `openai-codex`. `agent spawn --integration -I` overrides the route (pi vs
  codex for the same model id). `provider` is pi-only, `permission_mode`
  claude-only, `sandbox` codex-only — validation cross-checks.
- **Model catalog** (`internal/catalog`): `fledge init` execs
  `pi --list-models` / `codex debug models` and regenerates
  `.fledge/catalog.json` (gitignored, per-machine) wholesale; `agents.json`
  stays operator-owned and shadows catalog names in `agentcfg.Load`. Empty
  discovery keeps the old catalog.
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

README.md documents the full command surface, `.fledge/` tree, and
`agents.json` format — it is accurate; read it before adding commands.
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
