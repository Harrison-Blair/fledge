# Fledge

A Go CLI built with Cobra, with agent commands for Herdr.

```sh
go run . --help
go run . --version
go install .
```

## Agents

Run agent commands inside Herdr, with `HERDR_ENV=1` and `HERDR_SOCKET_PATH` set.
Fledge talks directly to that session's Unix socket.

```sh
fledge agent spawn --name reviewer --harness claude --model sonnet
fledge agent spawn --name builder --harness codex --workspace backend --tab builds
fledge agent spawn --name task --harness codex --workspace backend --worktree new --branch feature/task
fledge agent spawn --name existing --harness claude --pane w2:p3
fledge agent list --json
fledge agent message --name reviewer --body 'Review the current diff'
fledge agent message --pane w2:p3 --file task.md
cat task.md | fledge agent message --name reviewer --file -
```

Spawn requires a unique live `--name` and a `--harness`. By default it creates a
new tab in the caller's workspace. Workspace and tab names match exactly and
case-sensitively: missing destination names are created, ambiguous names fail,
and an existing tab receives a fresh split. New containers reuse their initial
children. `--workspace-id` and `--tab-id` select existing IDs instead of names.
`--pane` uses an existing shell and excludes other placement, cwd, env, and split
flags. `--label` and `--focus` still apply to that pane.

Ordinary spawn honors Herdr's configured cwd policy unless `--cwd` is supplied.
Use repeatable `--env KEY=VALUE` for new ordinary shells. `--direction` defaults
to `right`; `--ratio` delegates to Herdr when omitted. These flags only affect
splits. `--focus` defaults to false and focuses the destination before launch.
`--timeout` is a duration, default `30s`; its millisecond value must be greater
than 3000 and at most 300000.

With `--worktree new`, workspace selectors identify an **existing source**
repository workspace. That source takes precedence over `--cwd`; without either,
the source is Fledge's working directory. `--branch` defaults to the agent name,
and `--base` optionally selects a starting ref. Checkouts are created beneath the
primary checkout at `.fledge/worktrees/<branch>`, even when invoked from a linked
worktree. Branch slashes create nested directories. Existing branches or paths
fail; Fledge does not invent suffixes. A managed `.gitignore` excludes those
checkouts without changing the root ignore file.

`--worktree PATH` opens an existing checkout. Without an explicit source, the
absolute checkout path determines its repository. A newly opened workspace uses
its initial pane; an already-open workspace receives a new tab. `--tab NAME`
selects or creates a tab there. Worktree shells use the checkout directory.
Worktree mode excludes `--env`, `--pane`, and, in this version, `--tab-id`.
`--branch` and `--base` apply only to creation.

All 24 documented Herdr harness kinds are accepted. Fledge's optional `--model`
translates to verified native flags; it is currently unavailable for `kiro`,
`amp`, `muse`, `mastracode`, and `qodercli`. Those harnesses can still be launched
and receive native arguments. Model values remain opaque. Hermes uses its `chat`
subcommand. Documented native model conflicts are rejected when `--model` is set;
harness resume behavior can still affect model selection.

Pass native tokens with repeatable `--args=TOKEN`, then optional trailing
`-- TOKEN...`. Tokens retain whitespace and commas; Fledge does not parse a shell
command string. For example:

```sh
fledge agent spawn --name reviewer --harness claude --args=--permission-mode --args=plan
```

List includes unnamed agents and agents launched outside Fledge. Message accepts
exactly one of `--name`/`--pane` and one of `--body`/`--file`; it preserves newlines
and rejects empty or invalid UTF-8 content. Success acknowledges **submission**,
without waiting for the agent to begin or finish. Blocked agents require the
user to handle their approval dialog.

### Outcomes and recovery

Each command supports `--json`, emitting one Fledge object with `operation`,
`status`, `result`, `effects`, and `error`. Results use stable Fledge fields rather
than raw Herdr responses; unavailable values are null. Effects report known
resources and IDs or paths. Errors include a code, message, and failing phase.
Exit codes are 0 for success, 2 for invalid input, and 1 for runtime failures.

- `success`: the requested operation completed.
- `rejected`: failure is known to precede mutation, without uncertainty.
- `partial`: confirmed mutations preceded failure, or Herdr acknowledged a failed
  startup. A startup timeout can leave a process running, even in an existing pane.
- `unknown`: a mutation may have applied, but its response was lost or unusable.

Fledge retains created resources and never automatically retries mutations or
cleans up after failure. Inspect the reported pane and resources in Herdr before
retrying. For an unknown message outcome, inspect the conversation first to avoid
submitting the same prompt twice. A launched process is not proof of readiness.

A fresh split can briefly return `agent_pane_busy`. After inspecting the preserved
pane and confirming it is an idle shell with no launched agent, retry into that
specific pane instead of creating another one:

```sh
fledge agent spawn --name reviewer --harness claude --pane w2:p3
```

Use the actual pane ID from the partial outcome and retain your intended harness,
model, and native arguments. Fledge does not retry automatically.

## Layout

- `main.go` delegates to `cmd.Execute()` and handles the exit status.
- `cmd/` constructs fresh command trees with `NewRootCmd()` and provides
  `ExecuteWithArgs()` for tests.
- `cmd/<name>/` owns Cobra wiring; `internal/<name>/` owns the implementation.
- `internal/version/VERSION` is the sole release version source, embedded in the
  binary and exposed through `--version` and `-V`.

New subcommands export `New() *cobra.Command` and are registered by their parent.
Keep application logic in `internal/`, independent of Cobra.

## Development

Check formatting from the repository root using Bash. This includes tracked and
new nonignored Go files, skips deleted files, and excludes ignored worktrees:

```bash
set -euo pipefail
git ls-files --cached --others --exclude-standard --deduplicate -z -- '*.go' |
  (
    status=0
    while IFS= read -r -d '' file; do
      [[ -f "$file" ]] || continue
      unformatted="$(gofmt -l -- "$file")" || exit "$?"
      if [[ -n "$unformatted" ]]; then
        printf '%s\n' "$unformatted" >&2
        status=1
      fi
    done
    exit "$status"
  )
```

Then run:

```sh
go vet ./...
go test -race ./...
go build -o /tmp/fledge .
```

The existing GitHub workflows lint, test, and build for Linux amd64 and arm64.
Merges to `main` may create or refresh a release draft using the version file;
they do not publish releases automatically.

## License

[MIT](LICENSE).
