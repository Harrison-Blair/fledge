---
generated: 2026-07-11T01:58:32Z
commit: 96a3ac38bc843217824d6d6886c49906053bf686
agent: fledge-forager
fledge_version: 0.3.4
---

# Entry Points

Where execution enters fledge, and how to build/run/test it.

## Build & install

```sh
go build ./...                       # build everything
go build -o fledge ./cmd/fledge      # build the CLI binary
go install ./cmd/fledge              # install to $(go env GOPATH)/bin
```

`scripts/install.sh` wraps this: reads `VERSION`, sets ldflags, installs, verifies the installed binary's `fledge version` matches `VERSION`, and optionally runs `fledge init --refresh`. End-user quick-start is `go install github.com/Harrison-Blair/fledge/cmd/fledge@latest` then `fledge init` in a target repo (`README.md`).

## Binary entry point

- `func main()` (`cmd/fledge/main.go`) — calls `cli.Run(os.Args[1:])` and `os.Exit`s with its return code. This is the only `main()` in the repo.
- `func Run(args []string) int` (`internal/cli/cli.go`) — the real dispatcher: looks up `args[0]` in the registered command map, runs it, returns the exit code (`ExitOK`=0, `ExitFail`=1, `ExitUsage`=2, `ExitEnv`=3).

## CLI command surface (17 subcommands, `commandOrder` in `internal/cli/cli.go`)

`init`, `agents`, `scan`, `new`, `nest`, `preen`, `ready`, `vee`, `colony`, `unfledged`, `status`, `set`, `criteria`, `brood`, `abandon`, `broods`, `version`. Every command supports `--json` for machine-readable output where applicable. `nest` itself has subcommands: `new`, `scaffold`, `scout --module <name>`, `stamp <file>`.

## Tests

```sh
go test ./...                                        # everything
go vet ./...
go test ./cmd/fledge -run TestScripts                 # all CLI acceptance (txtar) tests
go test ./cmd/fledge -run TestScripts/init -v         # one acceptance test, verbose
go test ./internal/spec -run TestAllocateID           # one unit test
```

`TestScripts` (`cmd/fledge/main_test.go`) is the testscript runner that drives every `.txtar` file in `cmd/fledge/testdata/` as an acceptance test against a real built binary.

## Scaffold regeneration (agent-facing)

- `fledge init --refresh` — resets all fledge-owned files to the shipped versions, prunes obsolete ones, writes `.fledge/scaffold.json`.
- `fledge preen` — reports scaffold health (drift) plus spec validation findings.
- `fledge nest scaffold` — clears and recreates `.fledge/nest/` (context docs + `raw/`), used by the forager at the start of context regeneration (this document's own pipeline).
- Orchestration entry point for agents: `.fledge/skills/fledge-orchestrate/SKILL.md` (routing) → `planning.md` or `implementation.md`; Claude-specific primitive map at `.claude/fledge-adapter.md`.

## Open Questions

None observed.
