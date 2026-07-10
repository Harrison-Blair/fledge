---
generated: 2026-07-10T20:53:53Z
commit: f28efebd76d6aa135adb0956a3337a40a8d98351
agent: fledge-forager
fledge_version: 0.3.0
---

# Entry Points

How to build/run/test fledge, and every CLI command's public surface.

## Build, run, test

```sh
go build ./...                        # build everything
go build -o fledge ./cmd/fledge       # build the CLI binary
go test ./...                         # run all tests
go vet ./...
go test ./cmd/fledge -run TestScripts               # all txtar acceptance tests
go test ./cmd/fledge -run TestScripts/init          # one script (init.txtar)
go test ./cmd/fledge -run TestScripts/init -v       # verbose, shows script trace
go test ./internal/spec -run TestAllocateID         # a single unit test
```
Go 1.26. No Makefile — use `go` directly. `scripts/install.sh` wraps `go install ./cmd/fledge` + `hash -r` + version verification for reinstall workflows (see `CLAUDE.md` "Rebuild, reinstall, and verify the installed binary").

## Binary entry point

`cmd/fledge/main.go:main()` — 11 lines, pure delegation: `os.Exit(cli.Run(os.Args[1:]))`. All logic lives in `internal/cli.Run(args []string) int`.

## `internal/cli.Run` — the CLI dispatcher

Dispatches `args[0]` to the command registered under that name (`internal/cli/cli.go`). Returns one of the shared exit codes: `ExitOK`(0), `ExitFail`(1), `ExitUsage`(2), `ExitEnv`(3). Every command below additionally accepts `--json` for machine-readable output.

## Commands (one file each in `internal/cli/`, alphabetical by file, listed in `commandOrder` for usage/allow-list order)

- **`fledge init [--agent <harness>] [--refresh] [--force] [--list-agents]`** (`init.go`) — scaffold `.fledge/`, `pluma/`, harness adapter files; auto-detects harness from marker dirs (`.claude/`, `.pi/`) or defaults to Claude Code; `--refresh` re-syncs against `.fledge/scaffold.json`, preserving user edits; `--force` overwrites them.
- **`fledge agents`** (`agents.go`) — lists known adapters with derived tier, detector, and scaffolded status.
- **`fledge scan`** (`scan.go`) — inventories the repo by module (name/files/count/bytes), `.fledgeignore`-filtered; the authoritative work list for context foraging.
- **`fledge new plumage|feather`** (`new.go`) — creates a new spec with CLI-allocated ID and rendered frontmatter/template.
- **`fledge nest new|scaffold|scout|stamp`** (`nest.go`) — manages `.fledge/nest/`: `scaffold` clears and recreates concern-doc stubs, `scout --module <name>` creates a raw scout report stub, `stamp <file>` refreshes frontmatter.
- **`fledge preen [--strict]`** (`preen.go`) — validates all specs (`internal/check.Run`) plus scaffold drift (`internal/bootstrap.DriftReport`); `--strict` upgrades warnings.
- **`fledge ready`** (`ready.go`) — lists feathers with all dependencies satisfied (pipping-eligible), via `graph.Ready()`.
- **`fledge vee [PLM-###]`** (`vee.go`) — dependency graph; text, dot, or JSON output; waves and cycle detection via `internal/graph`.
- **`fledge colony`** (`colony.go`) — full spec inventory / status summary report (counts, plumage completion, orphans, blocked, locks, issues).
- **`fledge status <ID> <new-status> [--force]`** (`status.go`) — lifecycle transitions, gated by `taskTransitions`/`reqTransitions` legality matrices and (for fledging) unchecked-criteria checks.
- **`fledge set <ID> <field> <value>`** (`set.go`) — mutates `priority`, `oversight`, `depends_on` (cycle-checked via `graph.Cycle()`), `title`.
- **`fledge criteria <ID> [check|uncheck] [N]`** (`criteria.go`) — list/check/uncheck acceptance criteria by number or label.
- **`fledge brood <FTHR-ID>` / `fledge abandon <FTHR-ID> [--fledged] [--force]` / `fledge broods`** (`brood.go`) — claim/release/list feather locks via `internal/lock`.
- **`fledge unfledged [--plumage] [--feathers]`** (`unfledged.go`) — lists all non-fledged specs, priority-then-ID order.
- **`fledge version`** (`version.go`) — CLI binary version (`binaryVersion`, checked against `VERSION` file by `version_test.go`) plus repo spec/scaffold version.

## Bootstrap/adapter entry points (`internal/bootstrap`, invoked by `internal/cli/init.go`)

- **`LoadAdapters()`** — all manifests, sorted by name.
- **`FindAdapter(name)`** — look up one adapter.
- **`DetectAdapters(root)`** — auto-sense which adapters' marker files are present.
- **`WriteCore(root, opts)`** — writes `.fledge/skills/*`; returns created/updated/skipped/preserved counts.
- **`Manifest.WriteAdapter(root, commandOrder, opts)`** — writes one harness's files per its manifest's policies.
- **`CheckDuplicateSkills(root)`** — guards against leftover native-skill-dir copies of core skills.
- **`DriftReport(root, stamp, expected)`** — read-only classification of on-disk scaffold state (used by `preen`, never mutates).

## Orchestration entry points (agent-facing, not CLI)

- **`.fledge/skills/fledge-orchestrate/SKILL.md`** — routing entry point for planning/implementation phases; read first by any agent driving fledge's own workflow.
- **`.fledge/skills/fledge-orchestrate/foraging.md`** — the protocol this very document set was generated by (forager + scout roles).
- **`.fledge/skills/fledge-interrogate/SKILL.md`** — interrogation protocol for stress-testing plumages before commit.
- **`.claude/fledge-adapter.md`** — Claude-specific primitive map (which Claude mechanism realizes each of the 6 primitives).

## Open Questions

None survive synthesis.
</content>
