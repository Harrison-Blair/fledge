# Fledge

Fledge is a Go CLI for managing coding agents in [Herdr](reference/herdr/README.md).
It starts agents, tracks tasks, shares project memory, and manages Git worktrees.

## Install and update

Install with Go:

```sh
go install github.com/Harrison-Blair/fledge@latest
fledge --version
```

Alternatively, download a Linux amd64 or arm64 archive and `checksums.txt` from
[Releases](https://github.com/Harrison-Blair/fledge/releases/latest).
Check the archive before you extract it:

```sh
sha256sum --check --ignore-missing checksums.txt
```

Release archives contain `fledge`, `LICENSE`, and this README.
The updater supports Linux amd64 and arm64:

```sh
fledge update --check
fledge update
fledge update --yes
```

`--check` reports availability without installing anything.
An update asks for confirmation. Use `--yes` for noninteractive updates.
Fledge checks the archive's SHA-256 before replacing the executable.
It needs write access to the installation directory. There are no background checks.

See the [installation and update guide](reference/usage/releases.md#install-and-update) for details.

## Quick start

Run agent and worktree commands inside Herdr.
Herdr supplies `HERDR_ENV=1` and `HERDR_SOCKET_PATH` for its session.
Install the harness you want to use, such as Codex or Claude Code.

Start in your project's Git checkout:

```sh
fledge doctor
fledge agent spawn --name worker --harness codex --profile implementer
fledge task create --title "Fix the parser" --body "Fix the parser and check its tests."
```

Use the task ID printed by `task create`:

```sh
fledge task assign --id TASK_ID --name worker
fledge agent read --name worker
fledge task get --id TASK_ID
```

The worker reports its result with `fledge task complete`.
A different registered agent checks the result and records it with `fledge task verify`.
An agent's `idle` or `done` state does not prove that its task succeeded.

After you read and accept the results, inspect cleanup before running it:

```sh
fledge agent cleanup --dry-run
fledge agent cleanup
```

Cleanup requires a registered caller and acts on workers that caller spawned.
It holds workers with unfinished tasks. For a worker without tasks, use `--results-collected` after reading its reports.
See the [cleanup rules](reference/usage/agents.md#cleanup).

## Agents

Every spawn requires a unique live name and an explicit harness.
Profiles supply role instructions. They do not choose a harness or model.

```sh
fledge agent models --harness codex
fledge agent profiles
fledge agent list
fledge agent get --name worker
fledge agent message --name worker --body "Read the task result before continuing."
fledge agent wait --name worker --timeout 10m
fledge agent usage --name worker
```

Spawn opens a new tab unless you pass `--pane` to reuse an existing shell.
`--tab` labels the new tab. It does not select an existing tab.
Use `--file brief.md` on spawn to send an initial task after the agent becomes ready.
Messages and task briefs to registered agents are stored in their inbox and announced by a one-line doorbell; recipients run `fledge inbox read`.
A background supervision daemon wakes parents when a worker blocks, exits, or goes quiet; `fledge attention` summarizes what needs you.

Codex spawns default to `--yolo`. Claude spawns default to `--permission-mode bypassPermissions`.
Explicit native permission options take precedence.
Use `--no-permission-bypass` to leave permissions to native arguments and harness configuration.

An agent you started yourself registers itself: run `fledge agent adopt` inside that agent, in its own pane.
Adopt verifies that it runs inside the agent and cannot register another pane. Start Codex with `codex --no-daemon` so its tools can adopt it.
An unnamed agent requires `--name`. Use `fledge agent rename --to NAME` to change an existing name.

See the [agent guide](reference/usage/agents.md) for targeting, profiles, identity, usage, interruption, and recovery.

## Tasks

Tasks store briefs and results in `.fledge/state/tasks/` under the primary checkout.
They survive an agent's pane closing. All linked worktrees share them.

A task normally moves through `created` → `assigned` → `completed` → `verified` → `landed`.
Completion and verification require separate commands. Cancellation ends a task early.
`fledge task verify --commit REV` binds a verification to a commit.
`fledge task land --id TASK_ID` then merges that commit locally or through a pull request, as the task's delivery mode says.

```sh
fledge task template
fledge task list --ready
fledge task list --status completed
fledge task get --id TASK_ID
```

Briefs accept any nonblank UTF-8 text without NUL. The six-heading template is optional.
Use `--parent` for subtasks. Use `--after` for prerequisites.
Use `fledge task import --file plan.toml --dry-run` to inspect a proposal before creating its tasks.

See the [task guide](reference/usage/tasks.md) for assignment, verification, landing, dependencies, and proposals.

## Memory

Memories store durable project facts in `.fledge/memories/` under the primary checkout.
All linked worktrees share them. Memory commands work without Herdr.

```sh
fledge memory list
fledge memory get --name herdr-socket
```

Profile spawns include the destination repository's memory index in the brief.
See the [memory guide](reference/usage/memory.md) to add or remove memories.

## Worktrees

Managed worktrees live under `.fledge/worktrees/`.
Each checkout gets its own Herdr workspace.

```sh
fledge worktree list
fledge worktree create --branch feature/parser --base dev
fledge agent spawn --name builder --harness codex --worktree new --branch feature/task --base dev
fledge worktree remove --branch feature/parser
```

Removal keeps the branch. It refuses the primary checkout and checkouts in use by live agents.
Dirty or unmerged checkouts require `--force`.

See the [worktree guide](reference/usage/worktrees.md) for integration branch settings and removal guards.

## Doctor

`fledge doctor` checks Herdr connectivity, protocol compatibility, harness installations, local model caches, and configuration.
It does not change files or Herdr state, and it does not contact the network.

```sh
fledge doctor
fledge doctor --verbose
fledge doctor --json
```

A failed check produces exit code 1. Warnings alone produce exit code 0.
See the [diagnostics guide](reference/usage/doctor.md).

## Command help

Use CLI help for commands and flags:

```sh
fledge --help
fledge agent spawn --help
fledge task import --help
fledge worktree remove --help
```

Commands support `--json` for structured results.
Agent and task outcomes report known effects separately from errors.
Inspect resources after a partial or uncertain outcome before retrying a command.
See [outcomes and recovery](reference/usage/agents.md#outcomes-and-recovery).

The [migration guide](reference/usage/migration.md) covers removed flags and the change from TOML to Markdown profiles.
If you read this README from a release archive, the guides are available in the
[online repository](https://github.com/Harrison-Blair/fledge/tree/dev/reference/usage).

## Releases

Stable Git tags use `vMAJOR.MINOR.PATCH`. There is no maintained version file.
Merges do not publish releases.
The owner dispatches the Release workflow from `main` and chooses `patch`, `minor`, or `major`.

See the [release procedure](reference/usage/releases.md#releases) for publication, retries, and unfinished drafts.
Never move an existing release tag or replace published assets.

## Development

Develop on `dev` or a feature branch based on `dev`.
Changes reach `main` through a pull request that the owner approves.
The committed `.fledge/config.toml` names `dev` as the [integration branch](reference/usage/worktrees.md#integration-branch).

Read [AGENTS.md](AGENTS.md) for contributor rules and package structure.
Go changes require tests before implementation.
Run these checks before completing work:

```sh
bash .github/scripts/gofmt-check.sh
go vet ./...
go test -race ./...
go build -o /tmp/fledge .
```

See the [development guide](reference/usage/development.md) for the layout and release automation checks.

## License

[MIT](LICENSE).
