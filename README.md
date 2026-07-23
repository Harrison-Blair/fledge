# fledge

A zero-inference orchestrator for multi-agent coding sessions.

fledge brings up isolated orchestration sessions ("flocks"), launches coding
agents into them, and carries messages between those agents. Every agent runs
in a visible [herdr](https://github.com/Harrison-Blair/herdr) pane you can
watch and type into.

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
  start a flock and for every spawned agent, which lives in a pane.
- **`claude`** on `PATH` for the `claude` integration.
- **`codex`** on `PATH` for the `codex` integration.
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
spawns a daemon bound to that session, and opens the Herdr UI. Once the UI owns
the terminal, Fledge starts the `fledge-orchestrator`, immediately places and
focuses its pane left of the shell. Its assigned identity and authoritative
Markdown role are installed through the integration's native instruction
option, and the readiness bootstrap is the CLI's initial positional prompt.
Once authenticated startup finishes, Fledge creates a second, unfocused Herdr
workspace rooted at the project and starts `fledge watch <flock>` in its normal
root shell, leaving:

```text
Workspace: fledge-orchestrator
orchestrator | CLI

Workspace: fledge-watch
fledge watch
```

The watcher workspace's initial tab is labelled `watch`, and focus returns to
the orchestrator. The watcher is a normal shell process, not a Herdr or Fledge
agent. If watcher setup fails, Fledge closes only the new watcher workspace
when possible; the healthy flock, primary workspace, and CLI remain, and the
CLI shows the manual watch command. The primary workspace is labelled
`fledge-orchestrator` and its tab `orchestrator` (both on scripted starts too —
they are session metadata).
The shipped `fledge-orchestrator` definition is profile-agnostic, so an
interactive start offers the profile picker. It then completes authenticated
readiness with the managed role already installed; no lifecycle text is sent
through the orchestrator pane. Orchestrator messages are consumed by background
`fledge agent msg wait` processes. If no profile can be started
or the pick is cancelled, the whole start is rolled back: the daemon is stopped
and nothing is attached. An interactive start never lands you in a flock
without an orchestrator. A start whose stdout is not a terminal stops once the
daemon is up: no orchestrator, no picker, no attach. Commands work from
anywhere inside the workspace — the root is discovered git-style by walking up
to the nearest `.fledge/`. Starting a flock that is already running reattaches
to it without creating another watcher workspace.

Inside a pane of that session — where `FLEDGE_FLOCK` is already exported:

```bash
fledge agent types                    # what you can spawn
fledge agent spawn opus48             # -> reviewer-emperor, plus its pane id
fledge agent list                     # roster with liveness

export FLEDGE_AGENT_NAME="$(fledge agent register operator)"
fledge agent msg send reviewer-emperor "review internal/daemon"
fledge agent msg wait --timeout 30s
```

Tear down with `fledge flock stop`. That ends the herdr session; the daemon
polls its session socket and follows it down, and the managed session's
record is deleted from herdr's session list. (An operator-named `--session`
is stopped but its record is kept.)

`fledge stop` does the same to every flock in the workspace at once: it lists
them, asks once, and tears down what you confirm. It is interactive only —
without a terminal on both stdin and stdout it refuses, and there is no flag
to skip the confirmation. Scripts use `fledge flock stop <name>`.

`fledge flock clear [name]` permanently forgets saved state for one flock, or
for every flock in the workspace when no name is given. It previews the
targets and their running/down status, then asks once with a default-No `[y/N]`
confirmation. It is interactive only and has no bypass. Running flocks are
skipped and must first be stopped with `fledge flock stop <name>`. Clearing
deletes the flock directory, including its journal, daemon log, file-RPC
remnants, and roster history; it first stops and deletes the corresponding
default Fledge-managed Herdr session entry. The same confirmed operation also
removes project-scoped managed Herdr sessions that have no matching saved flock
directory. Operator-named `--session` records and managed sessions for other
projects are untouched. The parent `flocks/` directory is preserved.

## Concepts

**Flock** — one isolated orchestration session: its own daemon, agent roster,
journal, and unix socket. Multiple flocks coexist in a workspace without
seeing each other. State lives in `.fledge/flocks/<name>/`; the socket lives
under `$XDG_RUNTIME_DIR` instead, because `sun_path` is capped at 108 bytes
and network filesystems cannot bind unix sockets. If an agent sandbox blocks
Unix sockets, the CLI automatically carries the same request/response protocol
through ephemeral `.fledge/flocks/<name>/.rpc/` files. This fallback supports
the full command surface, including spawn, messaging, waits, and stop.

**`FLEDGE_FLOCK`** — the ambient flock for scoped commands. `fledge start`
exports it into the session it launches, and every pane inherits it. Commands
that accept a positional flock name (`flock stop`, `flock status`, and `watch`)
use that explicit name first and otherwise fall back to `FLEDGE_FLOCK`.
Bare `flock clear` always targets all saved flocks and never uses
`FLEDGE_FLOCK`. `fledge start`, `flock list`, `flock clear`, and `agent types`
need no flock context.

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

| | `claude` | `codex` | `pi` |
|---|---|---|---|
| Runs in | a herdr pane | a herdr pane | a herdr pane |
| Launch instructions | final `--append-system-prompt` | final `--config developer_instructions=<TOML string>` | final `--append-system-prompt` |
| Direct-message delivery | `pane.send_input` with `keys:["enter"]` | `pane.send_input` with `keys:["enter"]` | `pane.send_input` with `keys:["enter"]` |
| Stop | `pane.close`, or `workspace.close` when dedicated | `pane.close`, or `workspace.close` when dedicated | `pane.close` |
| Visible | yes — you can watch and type into it | yes — you can watch and type into it | yes — you can watch and type into it |

Every spawned agent survives a daemon restart because its pane does. Journals
written by older fledge versions may record pi agents with no pane (the removed
RPC-subprocess shape); those replay as `orphaned`.

**Journal** — `.fledge/flocks/<name>/journal.jsonl`, append-only JSON lines,
written *before* an operation is acknowledged. It records `daemon.started`,
`agent.registered`, `agent.launching`, `agent.spawned`, `agent.ready`, `agent.stopped`,
`msg.sent`, and `msg.delivered` (`agent.settled` is a legacy event from the
removed pi RPC shape — recognized on replay, never emitted). The daemon
rebuilds its entire roster and
pending-message queue by replaying it. A malformed final line is treated as a
torn write and ignored; malformed anywhere else is corruption and fails
startup. Registration and resolved launch intent are committed atomically
before Herdr starts the CLI. `agent.launching` includes the readiness-token
hash and SHA-256 `instruction_hash`; `agent.spawned` follows with the resolved
PID and pane/workspace metadata. An incomplete launching attempt replays as
`orphaned` with its token invalidated.

## Commands

```
fledge init [dir]              create or refresh the .fledge directory and
                               regenerate catalog.json from the installed
                               integrations
fledge context scan [dir]      list every file not excluded by .fledgeignore
fledge context graph [dir]     show visible directories, files, sizes and counts

fledge start                   bring up a flock and attach to its herdr UI
fledge stop                    stop every flock in the workspace, after a
                               [y/N] confirmation (terminal only)
fledge flock stop [name]       end the herdr session; the daemon follows
fledge flock clear [name]      permanently remove one or all stopped flocks
                               after a [y/N] confirmation (terminal only)
fledge flock list              every flock in the workspace, one line each
fledge flock status [name]     one flock in detail
fledge watch [name]            print all daemon.log history, then follow it
                               (default: FLEDGE_FLOCK)

fledge agent types             configured portable Markdown definitions
fledge agent models            configured launch profiles
fledge agent spawn [agent]     launch a prompted definition
fledge agent spawn --profile NAME  launch an unprompted raw profile
fledge agent register <type|agent.md> register an already-running agent
fledge agent ready             authenticate a newly spawned process
fledge agent stop <name>       stop a spawned agent
fledge agent list              roster with liveness

fledge agent msg send <to> <body>  send a message, print its id
fledge agent msg wait              block until a message arrives, print it as JSON
```

Flags follow one convention throughout: `--whole-flag` and a unique
`-CAPITAL`. Short flags are unique across the entire CLI, never just within a
subcommand.

`context graph` applies the same workspace discovery, subtree selection, and
`.fledgeignore` rules as `context scan`. Its text tree lists directories before
files and annotates them with recursive visible sizes and file counts. With
`--json`, `root` is the canonical workspace path, `scope` and node paths stay
workspace-relative, and immediate parent-child links are emitted as
`"contains"` edges; no language imports or external dependencies are inferred.

```
--version -V   --help -H       --json -J        --species -S
--pid -P       --model -M      --profile -L     --provider -D    --cwd -C
--flock -K     --session -N    --reply-to -R    --timeout -T
--integration -I
```

Help is contextual: `fledge --help` lists only top-level commands and global
flags. Enter a command group such as `fledge agent`, or run `fledge agent
--help`, to see its immediate subcommands. Use `fledge agent spawn --help` or
`fledge help agent spawn` for a leaf command's arguments and flags.

## Configuration

`fledge init` writes the portable definition tree and regenerates indexes:

```
.fledge/
├── agents/
│   ├── user/<name>/<name>.agent.md
│   ├── fledge/fledge-orchestrator/fledge-orchestrator.agent.md
│   ├── fledge/fledge-forager/fledge-forager.agent.md
│   ├── agents.json          generated user index
│   ├── fledge-agents.json   generated managed index
│   └── catalog.json         generated machine model profiles
├── .fledgeignore    ignore patterns, relative to the workspace root
├── flocks/          per-flock state: journal.jsonl, daemon.log
├── context/
├── locks/
└── pluma/{plumage,feathers}/
```

User Markdown and deterministic `agents.json` are intended to be tracked.
Managed definitions, `fledge-agents.json`, and the machine-specific catalog
are gitignored. Context scanning exposes only user `.agent.md` files from this
tree. `fledge deinit` removes the whole tree, including user definitions.

`catalog.json` is generated state: init probes
Claude Code with `claude --version`, asks Pi and Codex what they serve (`pi
--list-models`, `codex debug models`), and rewrites the file wholesale. A
successful Claude probe generates a model-less `defaultcl` profile plus
`opuscl`, `fablecl`, `sonnetcl`, and `haikucl` profiles using Claude Code's
matching model-family aliases. `defaultcl` leaves model selection to Claude
Code, including its configured or last-selected default. Those launchers use the
Claude Code account already logged in on the machine and the limits of that
Claude plan; they do not require an Anthropic API key. The catalog is
per-machine state and gitignored. Generated model names are the model id reduced
to lowercase alphanumerics plus a source suffix — `cl` for claude, `cx` for
codex, and `pi`/`oc`/`og` for pi's openai-codex/opencode/opencode-go providers —
so every generated name carries its source (the gitignored catalog never
collides with a committed `opus` or `default` profile a user declares), `gpt-5.5`
served by two sources is spawnable as either `gpt55cx` or `gpt55pi`, and a name
never changes when a later re-init finds new sources. Claude Code has no model-list
command, so its four family choices are a fixed discovered set rather than an
enumeration of every model ID. Optional version-specific or permission-specific
### Portable agent definitions

Definitions use the VS Code/GitHub `.agent.md` shape: required `name` and
`description`, optional `tools` and `model`, then a Markdown role prompt. The
folder, filename, and frontmatter name must agree. User names are kebab-case;
the entire `fledge-*` namespace is reserved for managed definitions and
profiles.

```yaml
---
name: code-reviewer
description: Review a change for correctness and regression risk.
tools: [read, search]
model: claude-opus-4-8
fledge:
  profile: opus-plan
  workspace:
    label: review-space
    tab: review
  launch:
    integration: claude
    permission_mode: plan
    cwd: .
    argv: []
    env: {}
---
Review the requested change and report concrete findings first.
```

Indexes contain separate `agents` and `profiles` maps and a schema version;
Markdown remains authoritative. If a referenced profile does not already
exist, `model` must be routable and `fledge.launch` is merged over its derived
integration/provider. Differing declarations of the same profile are errors.
`provider` is pi-only, `permission_mode` is Claude-only, and `sandbox` is
Codex-only. `argv` is option-only and cannot contain `--`; Fledge appends its
own instruction arguments after it so the assigned role wins for that run.
Permission bypass is never inferred. `fledge.worktree` is reserved
and rejected until worktree lifecycle support exists.

`fledge.workspace` is definition-level placement metadata, independent of the
profile selected by the caller. Both `label` and `tab` are required. At spawn,
Fledge creates an unfocused workspace in the flock's existing Herdr session,
renames its initial tab, targets the agent start into that workspace, and
removes the initial shell pane. Claude and Codex profiles support this; a Pi
profile fails with a clear placement error. The workspace id and label are
journaled and shown by `agent list`, including JSON output. Launch, readiness,
spawn-journal, and readiness failures close the whole workspace. `agent stop`
does the same; ordinary pane-hosted agents still close only their pane.

You can skip the config file and spawn a bare model — `fledge agent spawn
--model claude-opus-4-8` — as long as a prefix routing table recognizes it:
`claude*` routes to the claude integration; `gpt*`, `codex*`, and o-series
route to pi/`openai-codex`; `opencode-go*` and `opencode*` route to their
matching pi providers. An unrecognized model is a hard error rather than a
guess — define a profile in an agent Markdown file. `--integration -I` overrides the route, so
the same model id can run under either harness: `fledge agent spawn --model
gpt-5.6-sol --integration codex`.

Run bare on a terminal and Fledge lists definitions, not profiles. Before
launch, Fledge builds one native instruction document: the assigned,
already-registered identity; direct-message reply guidance using `fledge agent
msg send <recipient> <body>`; and the exact authoritative Markdown role. Raw
profile and model spawns receive only the identity and messaging guidance.
Claude and Pi receive this as the final `--append-system-prompt`; Codex receives
it as the final TOML-encoded `developer_instructions` config. The readiness
bootstrap is the CLI's initial positional prompt, and `fledge agent ready`
authenticates its one-use token. No startup `pane.send_input` or lifecycle
`msg.sent` events are produced. If an integration sandbox cannot open the
daemon socket, `agent ready`
atomically publishes only the token hash under the flock directory for the
daemon to validate and consume. The default readiness timeout is two minutes
(`--timeout`, `-T`). A
timeout or launch failure stops the transport and releases its name.

The orchestrator is user-driven but receives its managed Markdown role through
the same native instruction path as every other definition. Use
`fledge agent msg wait` in a background orchestrator-side process to consume
messages sent to it. Spawned Claude, Codex, and Pi workers continue to receive
direct pane pushes.

Ordinary direct pushes carry correlation metadata before their body:

```text
Fledge direct message
id: <message-id>
from: <assigned-sender-name>
reply_to: <original-id>        # present only on a reply

<body>
```

A recipient replies with `fledge agent
msg send <sender> <body> --reply-to <message-id>`; the sender waits for exactly
that response with `fledge agent msg wait --reply-to <message-id>`.

### Fledge Forager

`fledge-forager` is a managed, profile-agnostic definition for proposing a
file-based sub-agent architecture. Select a Claude or Codex profile, then use
the assigned name printed by spawn:

```sh
fledge agent spawn fledge-forager --profile <profile>
task_id=$(fledge agent msg send <assigned-forager-name> "Propose the sub-agent architecture")
fledge agent msg wait --reply-to "$task_id"
fledge agent stop <assigned-forager-name>
```

Forager launches unfocused beside `fledge-orchestrator` and `fledge-watch` in
the same Herdr session, in a workspace labelled `fledge-context` with a
`context` tab. It waits for the explicit post-readiness task, runs only
`fledge context scan --json`, reads no file contents, changes nothing, and
spawns no agents. Its correlated reply body is exactly one JSON object, with
no Markdown fence or surrounding prose:

```json
{
  "schema_version": 1,
  "file_count": 0,
  "total_size": 0,
  "subagent_count": 0,
  "subagents": [
    {
      "id": "kebab-case-role",
      "purpose": "Responsibility",
      "total_size": 0,
      "files": [{"path": "relative/path", "size": 0}]
    }
  ]
}
```

Every scanned file must occur exactly once with its scan-reported size.
`file_count`, `total_size`, every sub-agent `total_size`, and `subagent_count`
must reconcile; Forager chooses the proposed sub-agent count. Stopping it closes
only `fledge-context`, leaving the orchestrator and watcher workspaces intact.

### `.fledgeignore`

Gitignore syntax, patterns relative to the workspace root: last match wins,
`!` re-includes, trailing `/` is directory-only, `**` spans directories. It
adds one directive — `#include <path>` splices in another ignore file,
resolved from the workspace root, with cycle detection. It is spelled as a
comment so the file stays valid gitignore syntax:

```
#include .gitignore
```

Defaults are the `.gitignore` include, then:

```
.*/
!.github/
!.fledge/
.fledge/*
!.fledge/agents/
.fledge/agents/*
.fledge/agents/fledge/**
!.fledge/agents/user/
.fledge/agents/user/**
!.fledge/agents/user/**/
!.fledge/agents/user/**/*.agent.md
```

Dot-directories are excluded at any depth, with `.github/` and only portable
user agent Markdown carved back in. Generated JSON, managed definitions, and
all other `.fledge` state remain hidden.

The `.gitignore` include is active when the workspace has a `.gitignore` at
`fledge init` time, and commented out otherwise. It is conditional because a
directive naming a file that does not exist is an error, not an empty splice,
so an unconditional one would break every scan in a tree with no `.gitignore`.

## Development

```bash
go test ./...
go test -run TestGet ./internal/version/
gofmt -l . && go vet ./...
```

There is no Makefile. YAML frontmatter uses `github.com/goccy/go-yaml`.

`internal/version/VERSION` is the single source of truth for the version,
`//go:embed`-ed by `version.go`. It is also the release contract and must contain
exactly a strict `MAJOR.MINOR.PATCH` version. It sits inside the package because
`embed` cannot cross directory boundaries.

Pull requests into `main` run independent formatting/vet, test, static Linux
amd64/arm64 build, and version checks when opened, reopened, or updated with a
new commit. PR title and body edits do not rerun them. Tests report total
coverage in the Actions job summary without enforcing a threshold or uploading
the coverage file. The version must be greater than every existing semantic
release tag, and its `v<version>` tag must not already exist. After the initial
untagged release, bump `internal/version/VERSION` in every releasable PR.

Merging a PR into `main` reruns lint and tests against the exact merge commit.
If both pass, GitHub Actions tags that commit and publishes a GitHub Release
with generated notes and these assets:

```text
fledge_<version>_linux_amd64.tar.gz
fledge_<version>_linux_arm64.tar.gz
SHA256SUMS
```

Each archive contains the executable `fledge` binary and `LICENSE`. Release
reruns resume a matching draft or accept an already-published release at the
same commit; conflicting versions or tags fail instead of replacing a release.

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
| `internal/agentcfg` | `.agent.md` parsing, index synchronization, profiles and model routing |
| `internal/catalog` | model discovery from the installed integrations |
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
