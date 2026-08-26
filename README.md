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
Fledge's startup picker offers Pi, Claude Code, Codex, OpenCode, and Cursor.

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
pane; choose `none — shell only` if you do not want an agent. Later starts
reattach to the same running session. To stop the project's sessions and their
panes, return to a shell and run:

```sh
fledge stop
```

Run agent commands from a session pane or anywhere inside the initialized
project:

```sh
fledge agent list
fledge agent spawn reviewer --kind codex
fledge agent message reviewer "Review the current changes." --wait
fledge agent stop reviewer
```

Use `fledge --help` or `fledge <command> --help` for all commands and flags.

## References

- [herdr documentation](https://herdr.dev/docs/)
- [GNU Affero General Public License v3](LICENSE)
