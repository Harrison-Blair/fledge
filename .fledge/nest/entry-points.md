---
generated: 2026-07-17T17:48:26Z
commit: 1c9011d6e6a06f72f96bc98e3b2bd99c408ab79e
agent: fledge-forager
fledge_version: 0.6.10
---

# Entry Points

How to build, run, and invoke `fledge`, plus the CLI's full command surface and the orchestration workflow's phase entry points.

## Build & install

```sh
go build ./...                 # build everything
go build -o fledge ./cmd/fledge   # produce the binary at repo root
go install ./cmd/fledge        # install to $(go env GOPATH)/bin
```

`scripts/install.sh` automates build + install + version verification, with an optional `--refresh` flag that re-syncs the scaffold via `fledge init --refresh` afterward.

After changing CLI or `internal/bootstrap/...` code: `go install ./cmd/fledge && hash -r && command -v fledge && fledge version` — verify the installed binary matches `VERSION` before relying on it (per CLAUDE.md).

## Binary entry point

`cmd/fledge/main.go` (11 lines): `os.Exit(cli.Run(os.Args[1:]))` — the entire binary is `internal/cli.Run`.

## CLI command surface (26 commands, `internal/cli/cli.go` `commandOrder`)

- **Spec lifecycle**: `new`, `status`, `set`, `criteria`, `unfledged`
- **Spec validation/inspection**: `preen`, `vee` (dependency graph), `scan`, `ready`, `colony`
- **Feather claims**: `brood`, `abandon`, `broods`
- **Worker/ledger coordination**: `heartbeat`, `await`, `pulse`, `verdict`, `escalate`, `ledger` (with `read` subcommand)
- **Context**: `nest` (with `new`/`status`/`scout`/`scaffold`/`stamp` subcommands), `scan`
- **Configuration/system**: `init`, `agents`, `roster`, `dev` (with `status` subcommand), `version`, `update`

Every command accepts `--json`. Usage: `fledge <command> [flags]`; `fledge <command> -h` for per-command usage.

## Orchestration workflow entry point (agent-facing)

`.fledge/skills/fledge-orchestrate/SKILL.md` — the routing entry point every agent loads first. Maps a request to one of: Planning, Foraging, Implementation, or "not built yet". CLAUDE.md's routing table sends "plan X" / "write a plumage" / "break into feathers" → Planning; "implement PLM/FTHR-###" / "run the feathers" → Implementation.

- **Planning** → `planning.md` §0 (delegate to incubator or run inline) → §1 (freshness gate) → §2 (context gathering, delegates to `foraging.md` when `spawn-worker` is available) → §3 (plumage interrogation) → §4 (feather interrogation) → phase-close digest.
- **Foraging** → `foraging.md`: Commissioner spawns Forager; Forager scans (`fledge scan`), plans scout split, scaffolds (`fledge nest scaffold`), fans out Scouts, synthesizes 8 concern docs + index, verifies (`fledge nest status`). *This document set is exactly that pipeline's output.*
- **Implementation** → `implementation.md` §1 (scope resolution) → §2 (solo, Tier A/B) or §3 (team-loop dispatch, Tier C: brooder+skua pairs) → §4 (escalation triage) → §5 (end-of-run digest) → §6 (resume recovery).

## Harness-specific entry points (post-`fledge init`)

- **Claude Code** (Tier C): `.claude/agents/fledge-{brooder,context-scout,forager,incubator,skua}.md` (agent definitions, symlinks into `internal/bootstrap`); `.claude/settings.json` (`teammateMode: tmux`); teammates spawned via the `spawn-worker` primitive.
- **Codex** (Tier A): `.codex/fledge-adapter.md` (primitive map), `.codex/skills/fledge-{orchestrate,interrogate}/SKILL.md`.
- **pi** (Tier A): `.pi/prompts/fledge-{plan,implement}.md` (phase entry prompts), `.pi/settings.json` (points skills to `../.fledge/skills`).

## Test entry points

```sh
go test ./...                                        # everything
go vet ./...
go test ./cmd/fledge -run TestScripts                 # all txtar acceptance tests
go test ./cmd/fledge -run TestScripts/init             # one script
go test ./cmd/fledge -run TestScripts/init -v          # verbose, shows script trace
go test ./internal/spec -run TestAllocateID            # a single unit test
```

## Release entry point

`.github/workflows/release.yml` — triggers on every push to main; builds/publishes 4 platform binaries (linux/{amd64,arm64}, darwin/{amd64,arm64}) only if `VERSION` changed since the previous commit. `RELEASING.md` documents the manual version-bump procedure (touches `VERSION` + `internal/cli/version.go` + a third file).

## Dogfooding entry point (this repo on itself)

This repo has its own `.fledge/` directory: specs at `.fledge/pluma/`, claims at `.fledge/broods/`, context at `.fledge/nest/` (this document set), scaffolded Claude adapter at `.claude/`. `fledge init --refresh` resyncs fledge-owned files after a bootstrap/adapter source change; `fledge preen` validates scaffold-stamp health.
