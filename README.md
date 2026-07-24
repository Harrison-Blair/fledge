# fledge

[![release](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Harrison-Blair/fledge/badges/release.json)](https://github.com/Harrison-Blair/fledge/releases)

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
./scripts/install.sh   # build with -tags dev (version gets a -dev suffix) and install to GOBIN (or GOPATH/bin); BINDIR= to override
```

After installing a new binary while a flock is running, use
`fledge restart [name]` from inside that workspace to hand the flock to the
new executable without closing its Herdr panes. Inside a flock pane the name
can be omitted because `FLEDGE_FLOCK` is already set.

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
Once authenticated startup finishes, Fledge splits the right-hand CLI pane
evenly downward, runs `fledge watch <flock>` in the original upper pane, and
leaves the new lower pane as an interactive project-root shell:

```text
Workspace: fledge-orchestrator
orchestrator | fledge watch
             | project shell
```

The two columns are equal width, the two right panes are equal height, and
focus remains on the orchestrator. The watcher is a normal shell process, not
a Herdr or Fledge agent. If the split fails, the original right-hand shell
remains. If starting the watcher fails, Fledge closes the added lower pane when
possible to restore the two-pane layout. Either way the healthy flock remains,
the CLI shows the manual watch command, and Fledge attempts to refocus the
orchestrator. The workspace is labelled `fledge-orchestrator` and its tab
`orchestrator` (both on scripted starts too — they are session metadata).
The shipped `fledge-orchestrator` definition is profile-agnostic, so an
interactive start offers Claude Code and Codex profiles directly, plus one
`Browse Pi profiles…` entry that opens a provider-grouped Pi submenu. Managed
`fledge-*` profiles are hidden from both startup screens. It then completes authenticated
readiness with the managed role already installed; no lifecycle text is sent
through the orchestrator pane. Messages remain durable until claimed. The
current Herdr-owned interactive launcher has no same-session integration
control channel, so orchestrator readiness reports manual inbox delivery
instead of starting a second inference process. If no profile can be started
or the pick is cancelled, the whole start is rolled back: the daemon is stopped
and nothing is attached. An interactive start never lands you in a flock
without an orchestrator. A start whose stdout is not a terminal stops once the
daemon is up: no orchestrator, no picker, no attach. Commands work from
anywhere inside the workspace — the root is discovered git-style by walking up
to the nearest `.fledge/`. Starting a flock that is already running reattaches
to it without starting another watcher.

Inside a pane of that session — where `FLEDGE_FLOCK` is already exported:

```bash
fledge agent types                    # what you can spawn
fledge agent spawn opus48             # -> reviewer-emperor, plus its pane id
fledge agent list                     # roster with liveness

export FLEDGE_AGENT_NAME="$(fledge agent register operator)"
fledge agent msg send reviewer-emperor "review internal/daemon"
fledge agent msg wait --timeout 30s
```

Restart a daemon in place with `fledge restart [name]`. The command asks the
running daemon for its status, tells it to shut down, waits for it to go away,
then starts a replacement with the current `fledge` executable and the same
Herdr session binding. It prints the old and new daemon PID, version, and
session. Older daemons that do not understand in-place shutdown are left
running and report the normal stop/start sequence instead.
The handoff invariant is strict: the new daemon must report a different PID,
the current Fledge version, and the same Herdr session binding. If the
replacement or post-spawn verification fails, Fledge leaves the Herdr session
running and reports the daemon log path.

Tear down with `fledge flock stop`. That ends the herdr session; the daemon
polls its session socket and follows it down, and the managed session's
record is deleted from herdr's session list. (An operator-named `--session`
is stopped but its record is kept.)

`fledge stop` uses pane context: when `FLEDGE_FLOCK` is set, it lists and
offers to stop only that flock. Outside a flock it lists every flock in the
workspace and offers to stop them all. It is interactive only — without a
terminal on both stdin and stdout it refuses, and there is no flag to skip the
confirmation. Scripts use `fledge flock stop <name>`.

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
that accept a positional flock name (`restart`, `flock stop`, `flock status`,
and `watch`) use that explicit name first and otherwise fall back to
`FLEDGE_FLOCK`.
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
| Direct-message delivery | durable mailbox | durable mailbox | durable mailbox |
| Stop | `pane.close`, or `workspace.close` when dedicated | `pane.close`, or `workspace.close` when dedicated | `pane.close` |
| Visible | yes — you can watch and type into it | yes — you can watch and type into it | yes — you can watch and type into it |

Every spawned agent survives a daemon restart because its pane does. Journals
written by older fledge versions may record pi agents with no pane (the removed
RPC-subprocess shape); those replay as `orphaned`.

**Journal** — `.fledge/flocks/<name>/journal.jsonl`, append-only JSON lines,
written *before* an operation is acknowledged. Current events are
`daemon.started`; `agent.registered`, `agent.launching`, `agent.placed`,
`agent.spawned`, `agent.ready`, `agent.stopped`; `msg.sent`, `msg.delivered`,
`inbox.notified` (legacy or a future owned same-session adapter); and the
`tab.*`/`workspace.*` placement lifecycle events.
`agent.settled` is a real legacy event from the removed pi RPC shape and is
recognized on replay but never emitted. The daemon
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
fledge init [--fresh] [dir]    create or refresh the .fledge directory and
                               regenerate catalog.json from the installed
                               integrations
fledge context scan [dir]      list every file not excluded by .fledgeignore
fledge context graph [dir]     show visible directories, files, sizes and counts
fledge context compose analyzer-request [--in-place] [--worksheet PATH] FILE
                               inject template instructions into a request
fledge context compose worksheet [--output FILE] REQUEST
                               stamp an analyzer worksheet from its template
fledge context validate analyzer-request [FILE|-]
                               validate a strict analyzer request
fledge context validate analyzer-reply --request FILE [FILE|-]
                               validate and correlate an analyzer reply
fledge context render-project RUN_DIR
                               validate and publish a completed context run

fledge start                   bring up a flock and attach to its herdr UI
fledge restart [name]          restart a running flock daemon in place
                               (default: FLEDGE_FLOCK)
fledge stop                    stop FLEDGE_FLOCK, or every flock when unset,
                               after a [y/N] confirmation (terminal only)
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
fledge agent ready             authenticate and wait for first message
fledge agent ready --no-wait   authenticate and print assigned name
fledge agent stop <name>       stop a spawned agent
fledge agent list              roster with liveness

fledge agent msg send <to> [body]  send a message, print its id
                                   (--body-file FILE|- is also accepted)
fledge agent msg reply <id> [body] safely reply to a claimed inbound message
fledge agent msg inbox             claim oldest available message, or null
fledge agent msg wait [--from NAME] [--reply-to ID]
                                   block until a matching message, print JSON
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

`context scan --json` emits the strict version 1 run input. `root` is the
canonical workspace path, and `file_count` and `total_size` are derived from
`files`, so its output can be saved unchanged as a run's `scan.json`:

```json
{"schema_version":1,"root":"/workspace","file_count":1,"total_size":12,
 "files":[{"path":"main.go","size":12}]}
```

```
--version -V   --help -H       --json -J        --species -S
--pid -P       --model -M      --profile -L     --provider -D    --cwd -C
--flock -K     --session -N    --reply-to -R    --timeout -T
--integration -I  --workspace -W  --tab -B      --body-file -F
--request -Q      --no-wait -O    --in-place -A
--worksheet -E    --output -U     --fresh -X
--from
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
│   ├── fledge/fledge-analyzer/fledge-analyzer.agent.md
│   ├── fledge/user-agents.json       generated user index
│   ├── fledge/managed-agents.json    generated managed index
│   └── fledge/catalog.json           generated machine model profiles
├── .fledgeignore    ignore patterns, relative to the workspace root
├── flocks/          per-flock state: journal.jsonl, daemon.log
├── context/
├── locks/
└── pluma/{plumage,feathers}/
```

User Markdown is intended to be tracked. Managed definitions and all generated
indexes are gitignored. Context scanning exposes only user `.agent.md` files
from this tree. `fledge deinit` removes the whole tree, including user
definitions.

`fledge init --fresh [dir]` is the destructive counterpart to a normal
refresh. When `.fledge` exists, it lists the complete tree, warns separately
about files that are both untracked and not ignored by Git, and requires a
default-No confirmation on terminal stdin and stdout. It refuses while any
flock is running, deletes the entire tree only after confirmation, then runs
the normal scaffold, discovery, and synchronization flow from scratch.
`--fresh` cannot be combined with `--json`.

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
An omitted Claude `permission_mode` launches with `bypassPermissions`;
explicit modes such as `plan` and `acceptEdits` are preserved.
`fledge.worktree` is reserved
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
already-registered identity; structured reply guidance using `fledge agent
msg reply <message-id> <body>`; and the exact authoritative Markdown role. Raw
profile and model spawns receive only the identity and messaging guidance.
Claude and Pi receive this as the final `--append-system-prompt`; Codex receives
it as the final TOML-encoded `developer_instructions` config. The readiness
bootstrap is the CLI's initial positional prompt. Workers run `fledge agent
ready`, which authenticates the one-use token, journals readiness, then waits
for the first mailbox message and prints the same JSON object as `fledge agent
msg wait`. The managed orchestrator runs `fledge agent ready --no-wait`, which
authenticates and prints the assigned name without waiting. No startup
`pane.send_input` or lifecycle `msg.sent` events are produced. Readiness warns
that inbox delivery is manual: Herdr starts and owns the interactive Claude,
Codex, or Pi process, and exposes pane lifecycle/status but no persistent native
input/control stream that Fledge can safely serialize with user turns. Fledge
does not use `claude --resume --print`, `codex exec resume`, `pi --session
--print`, pane input, toasts, or polling as substitutes. Messages remain pending
for `agent msg inbox` or `agent msg wait`. Herdr remains limited to launch,
placement, status, and teardown. If an
integration sandbox cannot open the daemon socket, `agent ready --no-wait`
atomically publishes the token hash plus any integration runtime session id
under the flock directory for the daemon to validate and consume. The default
readiness timeout is two minutes (`--timeout`, `-T`). A timeout or launch
failure stops the transport and releases its name.

Automatic delivery requires a launcher redesign, not a resume command: a
Fledge-owned proxy in the pane must own one persistent Claude stream-json, Pi
RPC, or Codex app-server/thread connection and the user-facing frontend attached
to that same process/session. One per-agent serializer must accept both user
submissions and inbox metadata without blocking submission, then feed the
native control stream in order. The existing bounded/coalescing retry worker and
atomic ready-plus-arm journal transition are retained for such an owned adapter,
but production does not arm them until that channel exists.

Spawned agents also present that injected launch credential automatically on
`agent msg send` and `agent msg wait`; the daemon checks it against the claimed
spawned identity. Self-registered agents have no launch credential and rely on
the per-user runtime directory and Unix socket. This is a same-OS-user trust
boundary, not protection from another process running as that user.

The orchestrator is user-driven but receives its managed Markdown role through
the same native instruction path as every other definition. With the current
launcher it checks its durable mailbox explicitly; Fledge makes no automatic
background-delivery guarantee. Spawned Claude, Codex, Pi, and self-registered
agents all receive messages through the same durable mailbox. `agent msg send`
journals the message and offers it to a matching live waiter, or leaves it
pending until `agent msg wait` or `agent msg inbox` receives it. Current CLI
clients acknowledge only after printing the complete JSON response
successfully; a disconnect or output failure before that acknowledgement
leaves the same message available to an exact retry. Inbox checks return
immediately, print the oldest matching message as JSON, and print `null` when
empty. Optional `--from` and `--reply-to` filters are conjunctive.

A recipient replies with `fledge agent msg reply <message-id> <body>`.
Fledge validates that the referenced message was claimed by this identity,
derives its original sender, and sets exact causality in `reply_to`; the sender
waits for exactly that response with `fledge agent msg wait --from
<assigned-sender-name> --reply-to <message-id>`. When both filters are
supplied, both must match; a wrong-sender reply remains available for a valid
matching waiter. Real legacy `msg.delivered` entries had only `id` and `to`;
replay treats them as final because redelivery cannot be recovered soundly
without risking duplicates.

Managed context traffic has a stricter daemon boundary. A
`fledge-forager` request to a `fledge-analyzer` is validated as an exact
analyzer-request object before `msg.sent` is journaled. The analyzer must use
`agent msg reply`; Fledge derives its correlation and validates the exact reply
schema, group, and assigned-file references against the original request before
journaling or delivery. Malformed, uncorrelated, or request-mismatched context
JSON therefore never enters the recipient mailbox.

Spawned message operations require an authenticated, readied, running identity.
Stop cancels and joins any owned inbox-control work before transport teardown.
A stopped pane retains its launch credential only so stale calls can be
authenticated and rejected as unauthorized; it cannot send, wait, claim, reply,
or receive new messages.

### Fledge Forager

`fledge-forager` is a managed coordinator pinned to `claude-sonnet-5` that builds a
validated `.fledge/context/project.md` through file-scoped
`fledge-analyzer` agents. Use the assigned name printed by spawn:

```sh
fledge agent spawn fledge-forager
task_id=$(fledge agent msg send <assigned-forager-name> "Build the project context")
fledge agent msg wait --from <assigned-forager-name> --reply-to "$task_id"
fledge agent stop <assigned-forager-name>
```

Forager launches unfocused beside the `fledge-orchestrator` workspace in the
same Herdr session, in a workspace labelled `fledge-context` with a `context`
tab. It waits for the explicit post-readiness task, creates a
collision-safe run under `.fledge/context/runs/`, and scans once. The forager
does not read repository contents itself. Instead, it partitions every scanned
path exactly once into deterministic analyzer requests, each normally limited
to 50 files and 256000 bytes.

Before spawning, every group gets a stamped worksheet and a composed,
validated request:

```sh
fledge context compose worksheet \
  --output worksheets/<group-id>.md requests/<group-id>.json
fledge context compose analyzer-request --in-place \
  --worksheet .fledge/context/runs/<run-id>/worksheets/<group-id>.md \
  requests/<group-id>.json
fledge context validate analyzer-request requests/<group-id>.json
```

`context compose analyzer-request` deterministically injects the
operator-editable instruction sections from
`.fledge/context/templates/analyzer-request.md` into the request's
`instructions_before` and `instructions_after` fields, substituting
`{group_id}`, `{purpose}`, and `{worksheet_path}` with the request's own
values. The template's sections are delimited by `<instructions_before>` and
`<instructions_after>` XML tags, each required exactly once; text outside the
tags is ignored, and the file is seeded by `fledge init` but never
overwritten. Without `--in-place/-A` the composed request prints to standard
output. The daemon rejects a forager-to-analyzer request whose instruction
fields are missing or blank, so an instruction-less request can never reach
an analyzer.

`context compose worksheet` stamps the operator-editable
`.fledge/context/templates/analyzer-worksheet.md` per group, substituting
`{group_id}`, `{purpose}`, and `{files}` (the assigned files as a Markdown
checklist). The worksheet is the analyzer's scratch pad and human-readable
deliverable, the only file an analyzer may modify. Filled worksheets are
retained evidence: a run directory holding them survives post-publication
cleanup. Context run directories are generated logs and are Git-ignored by
default.

The forager finds its exact workspace id in `fledge agent list --json`, then
places analyzers two per tab:

```sh
fledge agent spawn fledge-analyzer \
  --workspace <workspace-id> --tab analysis-N
```

`--workspace/-W` and `--tab/-B` are an inseparable pair. Each accepts an exact
Herdr id or label. A missing tab label is created in the selected workspace;
an id-shaped missing tab is an error. Placement is resolved before launch, so
ambient focus cannot redirect the analyzer.

Fledge spawns one analyzer per group and places two distinct analyzer panes in
each analysis tab. The managed analyzer definition is pinned to
`claude-haiku-4-5`. Fledge journals a creation
intent before asking Herdr to
create a missing tab.
The tab first receives a random `fledge-create-*` label; after its returned id
is durable, Fledge renames it to the requested label and checks for a
same-label race. This lets restart roll back a create that completed before
Fledge could journal its id without mistaking an unrelated requested-label tab
for its own. Herdr protocol 16 has no atomic idempotency key, so exact
attribution is fundamentally unavailable if an external client deliberately
creates the identical random temporary label. On replay, one temporary-label
match is treated as Fledge's and rolled back, no match resolves as not created,
and multiple matches are preserved and the intent resolved rather than risking
deletion of an external tab.

After every analyzer is ready, the forager dispatches all request files before
waiting for replies:

```sh
dispatch_id=$(fledge agent msg send <analyzer-name> \
  --body-file requests/<group-id>.json)
fledge agent msg wait --from <exact-analyzer-name> \
  --reply-to "$dispatch_id" --timeout 10m
fledge context validate analyzer-reply \
  --request requests/<group-id>.json replies/<group-id>.json
```

`agent msg send` requires exactly one positional body or
`--body-file/-F <file|->`; `-` reads standard input without changing the body.
`context validate analyzer-request` and the reply operand also default to
standard input. Reply validation is strict and correlates group and file
references against the named request. Internal dependency paths are the one
exception: they may be safe normalized project-relative file or directory
paths outside the analyzer's assignment. Final rendering requires each one to
match a scanned file or a directory prefix containing a scanned file.

The forager synthesizes only from validated replies. It builds forager and
analyzer provenance from exact assigned spawn names and their matching
`agent list --json` profile/model metadata, then publishes:

```sh
fledge context render-project .fledge/context/runs/<run-id>
```

The renderer accepts only canonical run directories strictly below
`.fledge/context/runs`, rejects symlinked artifact paths, validates the complete
scan/request/reply/synthesis/provenance set, and atomically replaces
`.fledge/context/project.md`. Immediately after the project document is
published it also publishes the run provenance as a separate JSON object at
`.fledge/context/provenance.json`; provenance is no longer rendered into
`project.md`. It prints the published path, SHA-256, the `provenance_path`,
and any post-publication durability or cleanup warnings as JSON. Validation and
publication failures leave the run evidence intact; cleanup failures preserve
the valid publication and are warnings rather than rollback-like errors. The
forager stops only the analyzers it spawned and verifies cleanup. Its
correlated completion body is exactly one JSON object. Success has this shape:

```json
{
  "schema_version": 1,
  "status": "ok",
  "artifact": {
    "path": ".fledge/context/project.md",
    "sha256": "..."
  },
  "file_count": 0,
  "total_size": 0,
  "group_count": 0,
  "analyzer_count": 0,
  "warnings": [],
  "leftover_agents": []
}
```

All dispatches complete before any correlated wait starts, and there are no
retries. Success hashes come from the renderer; counts come from validated scan,
request, and provenance data; warnings come from renderer or cleanup results;
and leftovers come from the final roster. Cleanup trouble after publication is
still success with warnings. On a pre-publication failure, the run evidence is
retained and the reply reports `status`, `stage`, `message`, observed
`failed_groups`, and the exact `run_path`. Stopping the forager closes only
`fledge-context`, leaving the orchestrator tab and watcher pane intact.

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
