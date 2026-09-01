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
Fledge supports Pi, Claude Code, and Codex as agent harnesses. Cursor and
OpenCode are not supported harnesses. Pi may still expose provider-qualified
OpenCode API models in its model picker; those models run through the Pi
harness.

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

Pi, Claude Code, and Codex all receive profiles through their native
instruction channels: `--append-system-prompt`/`--append-system-prompt-file`
for Pi and Claude Code, and a `developer_instructions` config entry for Codex.
A harness argument that conflicts with that channel — for example a
`--system-prompt` flag alongside a selected profile — is rejected with
guidance to pass `--no-profile` instead, which starts the agent with your
native arguments and no managed instructions.

Later starts reattach to the same running session. Harness, model, and profile
flags on `fledge start` apply only when creating a fresh session; they do not
change an existing session on reattach. Each fresh session stores an immutable
snapshot of its selected profile, so an existing session retains the exact
instructions it started with after Fledge is upgraded. Run `fledge start --new`
to discard a stopped session's claim and choose a fresh agent and session; it
refuses while a session is running. A fresh root session's tab is always
labeled `fledge-orchestrator`; the agent's callback identity for
`fledge agent message` is the separate, fixed name `orchestrator` regardless of
the tab label. Reattaching to an existing session, or a tab renamed after
startup, is left untouched. To stop the project's sessions and their panes,
return to a shell and run:

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
offers a profile picker with `None`, `fledge-general`, and `fledge-orchestrator`;
non-interactive spawning applies no profile unless `--profile` names one
explicitly — `fledge-general` is never applied silently. `fledge start`
defaults an unspecified choice to `fledge-orchestrator` instead. The
`--profile` and `--no-profile` flags are mutually exclusive.

`fledge agent spawn` can also deliver one initial prompt with `--prompt TEXT`
or `--prompt-file PATH` (mutually exclusive; `--prompt-file -` is rejected, so
the prompt cannot be read from stdin). Flags meant for Fledge go before a `--`
separator; anything after it is passed to the harness unchanged:

```sh
fledge agent spawn worker --profile fledge-general --harness claude --model claude-fable-5-1 \
  --prompt 'Implement the change described in TASK.md and report back.' \
  -- --effort high --permission-mode auto
```

A prompt must be non-empty, valid UTF-8, free of NUL bytes, and at most
100 KiB; Fledge validates it before launching the agent. Prompt text is not
confidential — it eventually travels through process arguments — so never put
secrets in it. The prompt is submitted once, immediately after the agent
starts, without waiting for a reply: a successful spawn means the prompt was
submitted, not that the worker finished its task. If delivery cannot be
confirmed, the agent and any profile artifact are left running and the spawn
output gains an `initial_prompt` object with `status: "delivery_unconfirmed"`,
a `code` of `agent_blocked`, `agent_pane_not_found`, or `unknown`, and a
`retry_argv` for manual recovery:

```sh
fledge agent message <agent> -- '<prompt>'
```

Because the prompt may already have reached the agent, Fledge never retries,
polls, or stops it automatically on this condition; recovery is always a
manual, explicit choice.

Inspect the profiles shipped with Fledge and the exact instructions they inject:

```sh
fledge profile list
fledge profile show fledge-orchestrator
```

`fledge-general` is role-neutral guidance for any Fledge-managed worker, and
`fledge-orchestrator` is guidance for the root session that delegates to them;
these are the only two selectable profiles. Both are behavioral guidance
delivered to the agent's instructions — not a security, sandbox, or
authentication boundary.

The `fledge-orchestrator` profile documents an automatic model-routing scheme
for the workers it dispatches: a Codex/GPT family and a Claude family, each
with cheap, mid-tier, decent, and strongest tiers, spawned with
`--profile fledge-general`, an explicit reasoning effort, and — for Claude —
`--permission-mode auto`. Other models a harness exposes, including Fable 5
and Haiku, remain available through the ordinary model picker; they are simply
not part of that automatic routing. Run
`fledge profile show fledge-orchestrator` for the exact, current model map.

Use `fledge --help` or `fledge <command> --help` for all commands and flags.

## Compatibility

Until Fledge 1.0, CLI flags, commands, configuration schemas, and persisted
internal state may change without compatibility aliases or migrations. Breaking
changes are still documented in release notes.

## References

- [herdr documentation](https://herdr.dev/docs/)
- [GNU Affero General Public License v3](LICENSE)
