---
generated: 2026-07-16T21:27:15Z
commit: a1ed62a38540df7ab1cbdc4c486176a64a762018
agent: fledge-forager
fledge_version: 0.5.8
---

# Entry Points

The single binary entry point, its full command surface, build/install/run instructions, and the agent-facing entry points into the orchestration workflow.

## Build, install, run

```sh
go build ./...                    # build everything
go build -o fledge ./cmd/fledge   # build the CLI binary
go install ./cmd/fledge           # install to $(go env GOPATH)/bin
go test ./...                     # run all tests
go vet ./...
```
(`README.md`, root `CLAUDE.md`)

After changing CLI or `internal/bootstrap/...` code, this repo's dogfood loop requires reinstall + verify + regenerate:
```sh
go install ./cmd/fledge
hash -r
command -v fledge
fledge version                    # must match VERSION file
fledge init --refresh             # resync this repo's own scaffold
```
(root `CLAUDE.md`)

## Binary entry point

`cmd/fledge/main.go` — `main()` calls `cli.Run(os.Args[1:])` and `os.Exit`s with its return code. All real logic is in `internal/cli.Run` (`internal/cli/cli.go`), which dispatches the first argument against a command registry.

## Full CLI command surface (`internal/cli/cli.go` `commandOrder`, 19 commands + `abandon`/`broods` variants of `brood`)

- **Scaffold**: `init [--agent ...] [--refresh] [--force] [--list-agents]`, `agents [--json]`
- **Discovery**: `scan [--json]`
- **Spec authoring**: `new plumage|feather`
- **Spec mutation**: `set <ID> <field> <value>`, `status <ID> [<new-status>]`, `criteria <ID> [check|uncheck <AC-N>]`
- **Validation**: `preen [--strict] [--json]`
- **Query**: `ready [--json]`, `vee [--format text|dot|json]`, `colony [--json]`, `unfledged [--plumage|--feathers] [--json]`
- **Claiming**: `brood`, `abandon [--fledged|--force]`, `broods`
- **Context generation**: `nest new|scaffold|scout --module <module>|stamp <file>|status [--json]`
- **Team/species**: `roster assign [--feather FTHR-### --pair|--for planning|foraging]`, `roster release <name>`
- **Meta**: `version [--json]`, `update [--yes]`

Every command supports `--json`. Exit codes: `0` ok, `1` fail, `2` usage, `3` env (`internal/cli/cli.go`).

## Agent-facing entry points (routing into the orchestration workflow)

- **`.fledge/skills/fledge-orchestrate/SKILL.md`** — the entry an agent reads first; routes a user request to `planning.md`, `foraging.md`, or `implementation.md` based on intent ("plan X" → planning; "regenerate context" → foraging; "implement PLM/FTHR-###" → implementation).
- **`.fledge/skills/fledge-interrogate/SKILL.md`** — decision-forcing interrogation protocol used during plumage/feather authoring.
- **Per-harness entry surfaces**, scaffolded per adapter manifest: Claude Code loads `.claude/agents/*.md` (forager, incubator, context-scout, brooder, skua) as teammate system prompts; pi loads `.pi/prompts/fledge-plan.md` / `fledge-implement.md`; Codex is pointed at the skill via an `AGENTS.md` append.
- **Worker spawn prompt** — for any spawned worker (incubator/forager/brooder/skua/scout), the spawn prompt itself *is* the entry point: fully self-contained, no inherited conversation history (stated in `foraging.md`, `worker-protocols.md`, `implementation.md`).

## Notable single-purpose entry points worth knowing by name

- `fledge nest scaffold` — clears and recreates `.fledge/nest/` (used by this forager's own step 3).
- `fledge nest scout --module <module>` — creates one raw scout report file (used by each spawned scout).
- `fledge nest status [--json]` — the authoritative "is the nest done" check; gates a forager's final message.
- `fledge scan [--json]` — produces the module/file-list JSON that is foraging's authoritative work list.
- `fledge preen [--strict] [--json]` — spec + scaffold validation gate; wraps `internal/check.Run` plus scaffold drift detection.
- `fledge init --refresh` — resets all fledge-owned scaffold files to the shipped version; writes `.fledge/scaffold.json`.

## Open Questions

None observed.
