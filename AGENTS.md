# AGENTS.md

> **Note:** This file is the single source of truth for all agent instructions in this
> repository. `CLAUDE.md` contains only an `@AGENTS.md` import so Claude Code picks up
> the same instructions. Do not add instructions to `CLAUDE.md` directly; edit this
> file instead.

## Project

Fledge is a Go CLI built with Cobra.

## .gitignore policy

`.gitignore` is allowlist-style: it ignores everything (`*`) and then explicitly allows
Go source, `go.mod`/`go.sum`, Markdown docs, `LICENSE`, `.github/`, test fixtures, and
`internal/version/VERSION`. Keep it small. Only add a new `!` allow rule when a file
the project genuinely needs is being ignored, and add the narrowest pattern that
covers it. Never remove the leading `*`.

## Branching

Develop on `dev` or feature branches off `dev`. Changes reach `main` only through a
pull request, which the owner approves. Never commit directly to `main`.

## Releases

`internal/version/VERSION` is the sole source of truth for the release version.
`internal/version.Version()` embeds and returns that value, so binaries do not depend
on their runtime working directory. Do not duplicate the version in Go code, tests,
or build scripts. Cobra exposes it through `fledge --version` and `fledge -V`; there
is no `version` subcommand or lowercase `-v` alias.

The preserved release-draft workflow runs on pushes to `main` and manual dispatch.
It validates the version file and release state, runs lint, test, and build checks,
then creates or refreshes a draft release when eligible. It does not publish releases
or automatically bump the version.

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

Every subcommand gets its own folder under `cmd/<name>/` with a `New() *cobra.Command`
constructor that calls into `internal/`. The immediate parent registers its children.
`internal/` mirrors that: one folder per capability, plus folders for shared logic.
Command files hold wiring only; logic and its tests live in `internal/`. Internal
packages must not import Cobra or `cmd/`. The version flags use
`cmd/version.Configure()` instead of a standalone command.

Use direct functions and explicit dependencies; do not assemble command trees with
`init` functions or shared command globals. Keep packages flat until a distinct
responsibility needs a narrow API. Packages with multiple non-test files have a
`doc.go` with a package summary and a one-line map of each file.

## Instructions

### Dogfooding

When working in this repository, use Fledge itself for agent coordination: `fledge agent spawn` to launch agents, `fledge agent list` to discover them, and `fledge agent message` to delegate tasks and exchange messages. Treat this as dogfooding: exercise the project CLI in real work and surface bugs or missing capabilities instead of silently bypassing it with another coordination tool.

Name the tab an agent runs in after the agent's own name or role so panes are identifiable at a glance:

```sh
fledge agent spawn --name reviewer --harness claude --tab reviewer
```

Stop agents when their task is finished instead of leaving idle agents and tabs behind:

```sh
fledge agent stop --name reviewer
```

Agents that are `working`, `blocked`, or `unknown` require `--force`. When acting as an orchestrator, stop only the workers you spawned, and only after their work and any verification or follow-up have been read.
