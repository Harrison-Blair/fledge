# AGENTS.md

> **Note:** This file is the single source of truth for all agent instructions in this
> repository. `CLAUDE.md` contains only an `@AGENTS.md` import so Claude Code picks up
> the same instructions. Do not add instructions to `CLAUDE.md` directly; edit this
> file instead.

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

When working in this repository, use Fledge itself for agent coordination: `fledge agent spawn` to launch agents, `fledge agent list` to discover them, `fledge agent get` to inspect one, and `fledge agent message` to delegate tasks and exchange messages. Treat this as dogfooding: exercise the project CLI in real work and surface bugs or missing capabilities instead of silently bypassing it with another coordination tool.

Maintain `reference/dogfood/` as the record of dogfooding information for this repository.
Whenever an agent or one of its subagents hits a Fledge bug, missing capability, or
workaround, append an entry to `reference/dogfood/friction.md` using its Issue / Summary /
Reproduction steps format.

Before launching agents, check `fledge agent --help` for the commands needed for
both the task and cleanup. If the installed binary lacks commands present in this
checkout, build the current source into a temporary directory and use that binary
consistently for the task, including cleanup.

Fledge's `agent spawn`, `get`, `list`, `message`, and `stop` commands connect to Herdr's
local Unix socket. In Codex's restricted sandbox, request
`sandbox_permissions: "require_escalated"` on the first invocation of these
commands and of Herdr session-control commands, with a task-specific justification
and a narrow command prefix. Do not first run a socket command in the sandbox to
rediscover the known `connect: operation not permitted` failure. Use the normal
approval mechanism; these instructions do not override an approval denial or
authorize unrelated session changes. Help, version, and `agent models` do not
require Herdr socket access.

Name the tab an agent runs in after the agent's own name or role so panes are identifiable at a glance:

```sh
fledge agent spawn --name reviewer --harness claude --tab reviewer
```

Stop agents when their task is finished instead of leaving idle agents and tabs behind:

```sh
fledge agent stop --name reviewer
```

Agents that are `working`, `blocked`, or `unknown` require `--force`. When acting as an orchestrator, stop only the workers you spawned, and only after their work and any verification or follow-up have been read.
