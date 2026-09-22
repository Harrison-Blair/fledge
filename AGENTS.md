# AGENTS.md

> **Note:** This file is the single source of truth for all agent instructions in this
> repository. Claude Code reads it directly, so there is no `CLAUDE.md`; edit this file
> instead of adding harness-specific instruction files.

## Project

Fledge is a Go CLI built with Cobra.

## .gitignore policy

`.gitignore` is allowlist-style: it ignores everything (`*`) and then explicitly allows
Go source, `go.mod`/`go.sum`, Markdown docs, `LICENSE`, `.github/`, and test fixtures.
Keep it small. Only add a new `!` allow rule when a file the project genuinely needs
is being ignored, and add the narrowest pattern that covers it. Never remove the
leading `*`.

## Branching

Develop on `dev` or feature branches off `dev`. Changes reach `main` only through a
pull request, which the owner approves. Never commit directly to `main`.

## Releases

Stable Git tags (`vMAJOR.MINOR.PATCH`) are the release version source. There is no
maintained version file. `internal/lib/version.Version()` uses the release value
injected at build time, then Go's embedded module version, then `dev`. Development
builds retain Go's revision and dirty metadata without runtime Git access. Cobra
exposes the version through `--version` and `-V`; there is no `version` subcommand
or lowercase `-v` alias.

`.github/workflows/release.yml` runs only on manual dispatch from `main`. The owner
chooses `patch`, `minor`, or `major`; that request authorizes publication after CI
and packaging pass. Merges alone do not release. An agent explicitly asked to
release may use `gh workflow run release.yml --ref main -f bump=patch` (substitute
the requested bump), watch the run, and report its release URL. Do not trigger a
release merely because release automation was implemented or changed.

Release scripts calculate versions from existing tags, package Linux amd64/arm64
binaries with SHA-256 checksums, and publish only after all assets are verified.
Retries resume the same unfinished release or no-op for an already-published
commit. Never force-move release tags or replace published assets. An unfinished
draft for another commit must be resolved before starting a new release.
Run `python3 -B -m unittest discover -s .github/scripts -p '*_test.py'` when changing
release automation. `fledge update` performs explicit, verified binary updates;
there are no background update checks.

## Test-driven development

All Go changes are test-first. Write a failing test, run it and confirm it fails for
the right reason, write the minimal code to pass, refactor, repeat. Before declaring
a task done, run the [Git-aware formatting check](README.md#development),
`go vet ./...`, and `go test -race ./...` and report the output. The formatting
check covers existing tracked and new nonignored Go files in this checkout; it
must not descend into ignored managed worktrees. Never weaken or skip a test to get green.

## Layout

The module root's `main.go` is the installable entrypoint and delegates to
`cmd.Execute()`, handling the process exit status, so `go install .` produces a binary
named `fledge`. `cmd/` is a library package holding the root command and per-subcommand
wiring. `NewRootCmd()` builds a fresh command tree; `ExecuteWithArgs()` lets tests
inject arguments and output without changing process globals.

Every subcommand gets its own folder as `cmd/<name>/` or
`cmd/<parent>/<subcommand>/`, containing thin Cobra wiring: a
`New() *cobra.Command` constructor that calls into `internal/`. The immediate
parent registers its children. `internal/` mirrors that command nesting as
`internal/<name>/` and `internal/<parent>/<subcommand>/`. Each internal command
leaf owns its options, orchestration, result types, human rendering, and tests.
Internal parent packages may coordinate their nested components, as `doctor`
does with `checks`/`report` and `update` with
`release`/`archive`/`install`/`confirm`; child packages do not import their
parents. `internal/lib/<capability>/` contains focused shared code used by
multiple commands or packages; do not create one flat grab-bag lib package and
do not extract speculative utilities. Internal packages do not import Cobra or
`cmd/`. The version flags use `cmd/version.Configure()` instead of a standalone
command.

Use direct functions and explicit dependencies; do not assemble command trees with
`init` functions or shared command globals. Keep packages flat until a distinct
responsibility needs a narrow API. Packages with multiple non-test files have a
`doc.go` with a package summary and a one-line map of each file.

## Instructions

### Dogfooding

When working in this repository, use Fledge for every capability it offers, including
over the harness's own built-in tools. Treat this as dogfooding: exercise the project
CLI in real work and surface bugs or missing capabilities instead of silently bypassing
it with another tool.

| Need | Fledge command | Instead of |
| --- | --- | --- |
| Launch a sub-agent | `fledge agent spawn` | Claude's Agent tool, Codex subagents |
| Track and hand off work between agents | `fledge task create`/`depend`/`assign`/`complete`/`verify`/`cancel`/`list`/`get` | harness todo or task lists |
| Create, list, or remove a checkout | `fledge worktree create`/`list`/`remove` | `git worktree`, Claude `isolation: "worktree"` |
| Discover, inspect, read, wait on, interrupt, or stop agents | `fledge agent list`/`get`/`read`/`wait`/`pause`/`stop` | raw `herdr` CLI, harness TaskStop |
| Message another agent | `fledge agent message` | Claude's SendMessage, raw `herdr` pane input |
| Type raw input or keys into an agent (slash commands, dialog answers) | `fledge agent send` | raw `herdr pane send-text`/`send-keys` |
| Register an already-running agent | `fledge agent adopt` | — |
| Check the environment | `fledge doctor` | ad hoc probes |
| Discover models | `fledge agent models` | reading harness config |
| Update the binary | `fledge update` | manual downloads |

Harness built-ins remain allowed only where Fledge has no equivalent yet, such as
quick read-only lookups inside a single agent, or where Fledge fails.

Maintain `reference/dogfood/` as the record of dogfooding information for this repository.
Whenever an agent or one of its subagents hits a Fledge bug, missing capability, or
workaround, including any fallback to a harness built-in caused by one, append an entry
to `reference/dogfood/friction.md` using its Issue / Summary / Reproduction steps format.

Before using Fledge, check `--help` for the command groups needed for both the task and
cleanup (`fledge agent`, `task`, `worktree`). If the installed binary lacks commands
present in this checkout, build the current source into a temporary directory and use
that binary consistently for the task, including cleanup.

Known workarounds: spawn prompts and messages always start with a sender header, so ask
a spawned agent in plain words to invoke a skill (a leading slash command will not run);
pass absolute paths to `--cwd`; see `reference/dogfood/friction.md` for current issues.

Agents in this repository usually run inside a managed Fledge session: a Herdr pane,
often spawned by an orchestrator and given a Fledge agent record id. Expect messages
from the orchestrator and other agents. Each starts with a one-line header,
`ᛉ fledge message from <name> (<pane>) · id m-<hex> · reply: fledge agent message --name <name>`;
unnamed senders appear as `unnamed agent (<pane>)` and non-agent panes as `pane <pane>`,
with no reply command. A `fledge task assign` brief adds a line naming the task, its
title, and `complete with: fledge task complete --id <task> --summary "..."`, and a
task's creator, when it is another registered agent, receives a `task completed:`
notification naming `fledge task verify`. Treat these as coordination input: reply with
the header's reply command (or `--pane <pane>` when the header has no reply command),
and finish assigned tasks with `fledge task complete`.

Every Fledge `agent` command except `models`, plus
`task create`/`assign`/`complete`/`verify`, the `worktree` commands, and `doctor`,
connect to Herdr's local Unix socket (`task create`/`verify` only when run inside a
Herdr pane). In Codex's restricted sandbox, request
`sandbox_permissions: "require_escalated"` on the first invocation of these commands and
of Herdr session-control commands, with a task-specific justification and a narrow
command prefix. Do not first run a socket command in the sandbox to rediscover the known
`connect: operation not permitted` failure. Use the normal approval mechanism; these
instructions do not override an approval denial or authorize unrelated session changes.
Help, version, `agent models`, `task get`/`list`/`cancel`/`depend`, and `update` do not require
Herdr socket access.

Name the tab an agent runs in after the agent's own name or role so panes are identifiable at a glance:

```sh
fledge agent spawn --name reviewer --harness claude --tab reviewer
```

Stop agents when their task is finished instead of leaving idle agents and tabs behind:

```sh
fledge agent stop --name reviewer
```

Agents that are `working`, `blocked`, or `unknown` require `--force`. When acting as an orchestrator, stop only the workers you spawned, and only after their work and any verification or follow-up have been read.

When acting as an orchestrator, run a feature's verifier in that feature's managed
worktree (`fledge agent spawn --worktree <feature checkout path>`), not in the primary
checkout, a separate copy, or a temporary directory. The implementer and verifier take
turns on the shared checkout: the implementer commits and leaves a clean tree before
verification starts and makes no edits while it runs; the verifier undoes every
experimental change, such as mutation tests, and confirms `git status` is clean before
reporting; repairs go back to the implementer through the orchestrator. Verifiers run
`fledge task verify` only when no findings remain open, because a verified task cannot
be reopened.
