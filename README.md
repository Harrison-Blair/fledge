# fledge

A zero-inference orchestrator for multi-agent coding sessions.

fledge brings up isolated orchestration sessions ("flocks"), launches coding
agents into them, and carries messages between those agents. It runs Claude
Code agents in visible [herdr](https://github.com/Harrison-Blair/herdr) panes
you can watch and type into, and `pi` agents as supervised RPC subprocesses.

Two invariants shape the whole design:

1. **The Go CLI is the state authority.** herdr and agent events are *input
   signals*; fledge's own append-only journal is truth. herdr loses metadata
   across server restarts, so it is never treated as durable state.
2. **Zero inference in the orchestrator.** fledge issues socket commands,
   consumes events, advances a deterministic state machine, and writes its
   log — it never makes an LLM call. All inference happens inside agent
   processes, in panes the operator can see and interrupt.

## Requirements

- **Go 1.26+** to build. Zero third-party dependencies — stdlib only.
- **Unix** (Linux or macOS). Uses unix sockets, `setsid`, and signal-0
  liveness probes.
- **`herdr`** on `PATH`, protocol 16 (verified against 0.7.4). Required to
  start a flock, and required for any Claude-integration agent, which needs a
  pane to live in.
- **`claude`** on `PATH` for the `claude` integration.
- **`pi`** on `PATH` for the `pi` integration.

You only need the CLI for the integrations you actually spawn.

## Install

```bash
./scripts/build.sh     # -> bin/fledge
./scripts/install.sh   # copy to GOBIN (or GOPATH/bin); BINDIR= to override
```

## Quickstart

```bash
cd your-project
fledge init                 # create the .fledge/ tree
fledge start                # start a flock, then attach to its herdr UI
```

`fledge start` mints a flock name (`flock1`, `flock2`, …), starts a herdr
session named `fledge-<dir>-<hash>-<flock>` (workspace-scoped, so same-named
flocks in different projects never collide) rooted at the workspace root,
spawns a daemon bound to that session, spawns the `fledge-orchestrator` agent,
and execs into the herdr UI with the orchestrator pane left of the shell pane.
The workspace it creates is labelled `fledge-orchestrator` and its tab
`orchestrator` (both on scripted starts too — they are session metadata).
If there is no `fledge-orchestrator` entry in `agents.json` it offers the agent
picker instead, and the config you pick runs as the orchestrator: it takes the
same reserved `fledge-orchestrator` name, and `fledge agent list` shows the
config it came from so you can still see which model is running.
If no orchestrator can be started at all (empty catalog,
or a cancelled pick) the whole start is rolled back: the daemon is stopped and
nothing is attached. An interactive start never lands you in a flock without an
orchestrator. A start whose stdout is not a terminal stops once the daemon is
up: no orchestrator, no picker, no attach. Commands work from anywhere inside
the workspace — the root is discovered git-style by walking up to the nearest
`.fledge/`. Starting a flock that is already running reattaches to it.

Inside a pane of that session — where `FLEDGE_FLOCK` is already exported:

```bash
fledge agent types                    # what you can spawn
fledge agent spawn opus48             # -> reviewer-emperor, plus its pane id
fledge agent list                     # roster with liveness

fledge agent msg send reviewer-emperor "review internal/daemon" --from operator
fledge agent msg wait --as reviewer-emperor --timeout 30s
```

Tear down with `fledge flock stop`. That ends the herdr session; the daemon
polls its session socket and follows it down, and the managed session's
record is deleted from herdr's session list. (An operator-named `--session`
is stopped but its record is kept.)

`fledge stop` does the same to every flock in the workspace at once: it lists
them, asks once, and tears down what you confirm. It is interactive only —
without a terminal on both stdin and stdout it refuses, and there is no flag
to skip the confirmation. Scripts use `fledge flock stop <name>`.

## Concepts

**Flock** — one isolated orchestration session: its own daemon, agent roster,
journal, and unix socket. Multiple flocks coexist in a workspace without
seeing each other. State lives in `.fledge/flocks/<name>/`; the socket lives
under `$XDG_RUNTIME_DIR` instead, because `sun_path` is capped at 108 bytes
and network filesystems cannot bind unix sockets.

**`FLEDGE_FLOCK`** — the only way to select a flock. There is deliberately no
override flag, so a pane started in one flock cannot address another. `fledge
start` exports it into the session it launches, and every pane inherits it.
`fledge start`, `flock stop`, `flock list`, and `agent types` need no flock
context.

**Agent** — a coding process the daemon tracks. Agents are named
`<type>-<species>`, where the species is drawn from a fixed pool of 18 penguin
slugs (`emperor`, `king`, `adelie`, …) allocated per type, so you get
`reviewer-emperor` and `builder-emperor` but never two live `reviewer-emperor`s.
A slug frees up once its process dies. Agents either *self-register* (`agent
register`, supplying their own pid) or are *spawned* by the daemon, which then
owns their lifecycle.

The one exception is `fledge-orchestrator`, the profile `fledge start` brings
up: it runs under that exact name, with no species suffix, so the agent you
always have is the one whose name you already know. That holds however it joins
the roster — spawned or self-registered — so an orchestrator is the same agent
either way. Its name pool is therefore that single name: a second one while the
first is alive fails the way an exhausted species pool does.

**Integration** — how an agent is launched and talked to.

| | `claude` | `pi` |
|---|---|---|
| Runs in | a herdr pane | a supervised subprocess |
| Launch | `claude --session-id <uuid> [--permission-mode …] [--model …]` | `pi --mode rpc [--provider …] [--model …]` |
| Delivery | `pane.send_input` with `keys:["enter"]` | JSONL `prompt` frame on stdin |
| Stop | `pane.close` | `abort` frame, stdin EOF, then SIGKILL after 3s |
| Visible | yes — you can watch and type into it | no |

Claude agents survive a daemon restart because their pane does; pi agents are
marked `orphaned` on journal replay, since their pipes died with the daemon.

**Journal** — `.fledge/flocks/<name>/journal.jsonl`, append-only JSON lines,
written *before* an operation is acknowledged. It records `daemon.started`,
`agent.registered`, `agent.spawned`, `agent.settled`, `agent.stopped`,
`msg.sent`, and `msg.delivered`. The daemon rebuilds its entire roster and
pending-message queue by replaying it. A malformed final line is treated as a
torn write and ignored; malformed anywhere else is corruption and fails
startup.

## Commands

```
fledge init [dir]              create or refresh the .fledge directory
fledge context scan [dir]      list every file not excluded by .fledgeignore

fledge start                   bring up a flock and attach to its herdr UI
fledge stop                    stop every flock in the workspace, after a
                               [y/N] confirmation (terminal only)
fledge flock stop [name]       end the herdr session; the daemon follows
fledge flock list              every flock in the workspace, one line each
fledge flock status [name]     one flock in detail

fledge agent types             what spawn can launch, from agents.json
fledge agent spawn [config]    launch an agent, print its assigned name
                               (bare, on a terminal: pick from a menu)
fledge agent register <type>   register an already-running agent
fledge agent stop <name>       stop a spawned agent
fledge agent list              roster with liveness

fledge agent msg send <to> <body>  send a message, print its id
fledge agent msg wait              block until a message arrives, print it as JSON
```

Flags follow one convention throughout: `--whole-flag` and a unique
`-CAPITAL`. Short flags are unique across the entire CLI, never just within a
subcommand.

```
--version -V   --help -H       --json -J        --species -S
--pid -P       --model -M      --provider -D    --cwd -C
--flock -K     --session -N    --from -F        --reply-to -R
--as -A        --timeout -T
```

Help is contextual: `fledge --help` lists only top-level commands and global
flags. Enter a command group such as `fledge agent`, or run `fledge agent
--help`, to see its immediate subcommands. Use `fledge agent spawn --help` or
`fledge help agent spawn` for a leaf command's arguments and flags.

## Configuration

`fledge init` writes this tree, never clobbering a file that already exists:

```
.fledge/
├── agents.json      named agent configs
├── .fledgeignore    ignore patterns, relative to the workspace root
├── flocks/          per-flock state: journal.jsonl, daemon.log
├── context/
├── locks/
└── pluma/{plumage,feathers}/
```

### `agents.json`

Maps a name (lowercase alphanumerics) to a launch config. `fledge-orchestrator`
is the single reserved name exempt from that rule — hyphen included — because
`fledge start` looks it up by that exact string; no other name may contain a
hyphen:

```json
{
  "opus48":  { "integration": "claude", "model": "claude-opus-4-8" },
  "planner": { "integration": "claude", "permission_mode": "plan" },
  "gpt55":   { "integration": "pi", "provider": "openai-codex", "model": "gpt-5.5" },
  "fledge-orchestrator": { "integration": "claude", "model": "claude-opus-4-8" }
}
```

`integration` is required and must be `claude` or `pi`. `provider` is pi-only,
`permission_mode` is claude-only, and setting either on the wrong integration
is an error. `cwd`, `env`, and `argv` (appended last) are optional.

You can skip the config file and spawn a bare model — `fledge agent spawn
--model claude-opus-4-8` — as long as a prefix routing table recognizes it:
`claude*` routes to the claude integration; `gpt*`, `codex*`, and o-series
route to pi/`openai-codex`; `opencode-go*` and `opencode*` route to their
matching pi providers. An unrecognized model is a hard error rather than a
guess — add it to `agents.json`.

Run bare on a terminal — `fledge agent spawn` with no config name, `--model`,
or `--provider` — it lists the configured agents grouped by provider and
numbered, and spawns whichever you pick by number or name. Scripted use, where
stdin is not a terminal, keeps the usage error.

### `.fledgeignore`

Gitignore syntax, patterns relative to the workspace root: last match wins,
`!` re-includes, trailing `/` is directory-only, `**` spans directories. It
adds one directive — `#include <path>` splices in another ignore file,
resolved from the workspace root, with cycle detection. It is spelled as a
comment so the file stays valid gitignore syntax:

```
#include .gitignore
```

Defaults are just `.fledge/` and `.git/`, with the `.gitignore` include
present but commented out.

## Development

```bash
go test ./...
go test -run TestGet ./internal/version/
gofmt -l . && go vet ./...
```

No Makefile, no CI, no dependencies.

`internal/version/VERSION` is the single source of truth for the version,
`//go:embed`-ed by `version.go` — bumping means editing that one file. It sits
inside the package because `embed` cannot cross directory boundaries.

### Layout

| Package | Role |
|---|---|
| `cmd/fledge` | CLI: hand-rolled dispatch, no flag package |
| `internal/daemon` | per-flock server, spawning, journal, session watch |
| `internal/protocol` | request/response types for the daemon socket |
| `internal/client` | dials the daemon socket |
| `internal/flock` | flock naming, layout, socket paths, `FLEDGE_FLOCK` |
| `internal/herdr` | shells out to the herdr CLI for session lifecycle |
| `internal/herdrwire` | speaks the herdr socket API directly |
| `internal/pirpc` | pi subprocess supervision over JSONL |
| `internal/agentcfg` | `agents.json` parsing and model routing |
| `internal/species` | the penguin slug pool |
| `internal/scaffold` | creates the `.fledge/` tree |
| `internal/ignore` `internal/scan` | `.fledgeignore` matching and workspace scan |

### A note on `docs/`

`docs/` predates the current code and documents a prior exploration that was
run to completion and then torn down. Its roadmap and repo layout are
historical and describe packages that no longer exist. What carries forward is
the verified findings in `docs/EXPERIMENTS.md` — particularly why fledge sends
input with an explicit `enter` key, and why it never claims agent authority
over a pane.

herdr, pi, and Claude Code are fast-moving pre-1.0 surfaces. The versions
pinned in `docs/INTEGRATION-CONTRACTS.md` are a fixed snapshot; check live
(`herdr api schema --json`) before relying on any version-specific claim.

## License

GNU Affero General Public License v3.0. See [LICENSE](LICENSE).
