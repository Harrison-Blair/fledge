# fledge
<!-- USER_MAINTAINED_SECTION:START - DO NOT EDIT BELOW -->

<!-- USER_MAINTAINED_SECTION:END - DO NOT EDIT ABOVE -->

> [!IMPORTANT]
> Everything below this notice was written by AI and reviewed and approved by a human.

Fledge manages project-local [herdr](https://herdr.dev/) sessions for coding
agents. It initializes a project, starts or reattaches to its persistent
terminal workspace, and provides commands to spawn, message, list, and stop
agents.

## Install

Fledge currently supports Linux on `amd64` and `arm64`. Install
[herdr](https://herdr.dev/docs/install/) first and make sure `herdr` is on your
`PATH`. To run an agent, its CLI must also be installed and authenticated;
Fledge supports Pi, Claude Code, Codex, and Cursor as agent harnesses. OpenCode
is not a supported harness. Pi may still expose provider-qualified OpenCode API
models in its model picker; those models run through the Pi harness.

### User

1. Download the archive for your architecture from the
   [latest release](https://github.com/Harrison-Blair/fledge/releases/latest).
2. Extract it and place `fledge` somewhere on your `PATH`:

   ```sh
   tar -xzf fledge_*_linux_*.tar.gz
   install -Dm755 fledge "$HOME/.local/bin/fledge"
   ```

3. Confirm the installation with `fledge --version`.

### Dev

Install the Go version declared in [`go.mod`](go.mod), then clone and install:

```sh
git clone https://github.com/Harrison-Blair/fledge.git
cd fledge
go install .
go test ./...
```

## Configuration

No manual configuration is required. `fledge init` creates
`.fledge/config.json` and a `.fledge/.gitignore` that excludes local session
state.

## Usage at a Glance

Initialize a project and start its session:

```sh
cd /path/to/project
fledge init
fledge start
```

The first start asks which agent harness and model to run in the orchestrator
pane; choose `none — shell only` if you do not want an agent. The root agent
silently receives the managed `fledge-orchestrator` profile before its first
turn. Use `--no-profile` to opt out explicitly.

Pi, Claude Code, and Codex receive profiles through their native instruction
channels. Cursor cannot load profiles yet. Selecting Cursor interactively asks
for confirmation before continuing without the profile; non-interactive Cursor
starts must pass `--no-profile`.

Later starts reattach to the same running session. Harness, model, and profile
flags on `fledge start` apply only when creating a fresh session; they do not
change an existing session on reattach. Each fresh session stores an immutable
snapshot of its selected profile, so an existing session retains the exact
instructions it started with after Fledge is upgraded. Run `fledge start --new`
to discard a stopped session's claim and choose a fresh agent and session; it
refuses while a session is running. To stop the project's sessions and their
panes, return to a shell and run:

```sh
fledge stop
```

Run agent commands from a session pane or anywhere inside the initialized
project:

```sh
fledge agent list
fledge agent spawn reviewer --harness codex
fledge agent message reviewer "Review the current changes." --wait
fledge agent stop reviewer
```

`fledge start` and `fledge agent spawn` share the same launch resolution: an
explicit harness or model overrides a profile default, and any remaining choice
opens a picker when both input and output are terminals. Non-interactive use
must provide both `--harness` and `--model`. Interactive `agent spawn` also
offers a profile picker with `None` and the available managed profiles;
non-interactive spawning uses no profile unless `--profile` is passed. The
`--profile` and `--no-profile` flags are mutually exclusive.

Inspect the profiles shipped with Fledge and the exact instructions they inject:

```sh
fledge profile list
fledge profile show fledge-orchestrator
```

Use `fledge --help` or `fledge <command> --help` for all commands and flags.

## Compatibility

Until Fledge 1.0, CLI flags, commands, configuration schemas, and persisted
internal state may change without compatibility aliases or migrations. Breaking
changes are still documented in release notes.

## References

- [herdr documentation](https://herdr.dev/docs/)
- [GNU Affero General Public License v3](LICENSE)
