# Development

[README](../../README.md)

## Layout

[AGENTS.md](../../AGENTS.md#layout) holds the contributor rules for package structure.
This map is a starting point.
A package with several files has a `doc.go` that summarizes them.

- `main.go` is the installable entrypoint.
  `cmd/` holds the root command and the thin Cobra wiring for each subcommand.
- `internal/<name>/` and `internal/<parent>/<subcommand>/` implement each command.
- `internal/lib/<capability>/` holds code that several commands share.

Examples of shared capabilities:

- `internal/lib/version` reports the release tag or Go build metadata through `--version` and `-V`, without a maintained version file.
- `internal/lib/harness` defines the harness kinds, one typed profile per kind, and fixed capability rows derived from those profiles.
  `agent pause`, `agent spawn`, and `agent models` read these profiles.
  It imports nothing from Fledge.
- `internal/lib/memory` stores memory files and their generated index for the `memory` commands and the spawn brief.
- `internal/lib/usage` reads measured tokens and harness-recorded cost estimates for `agent usage`.
  Sources include Claude, Codex, and pi session files, and `opencode export`.
  The command reads the whole session.
  The library also supports time windows, but no command uses them.
  Missing or unreadable data is `unavailable` with a reason.

## Checks

Run the CLI from the checkout, or install the local source:

```sh
go run . --help
go run . --version
go install .
```

Check formatting from the repository root using Bash.
This includes tracked and new nonignored Go files, skips deleted files, and excludes ignored worktrees:

```sh
bash .github/scripts/gofmt-check.sh
```

Then run:

```sh
go vet ./...
go test -race ./...
go build -o /tmp/fledge .
```

The GitHub workflows lint, test, and build for Linux amd64 and arm64.
Release workflow and script changes also trigger these checks.
Test release automation locally without contacting GitHub:

```sh
python3 -B -m unittest discover -s .github/scripts -p '*_test.py'
bash .github/scripts/package.sh v0.0.0 /tmp/fledge-package-smoke
```

The packaging command uses a synthetic version for a local smoke test only.
It neither creates tags nor publishes releases.
