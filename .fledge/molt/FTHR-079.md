# FTHR-079 Evidence

## AC-1

Tests written first: `cmd/fledge/testdata/dev_status.txtar`, run via
`go test ./cmd/fledge -run TestScripts/dev_status -v`.

### Pre-implementation run (FAILING, expected reason: unknown command "dev")

Command: `go test ./cmd/fledge -run TestScripts/dev_status -v`

```
        > exec fledge dev status
        [stderr]
        fledge: unknown command "dev"

        usage: fledge <command> [args]

        commands:
          fledge init [--agent <name>]... [--dev=<path>] [--refresh] [--force] [--list-agents] [--json]
          fledge agents [--json]
          fledge scan [--json]
          fledge new plumage --title <t> [--priority P1] [--agent <s>] [--json]
          fledge new feather --title <t> --plumage PLM-### [--depends-on a,b] [--priority P1] [--oversight merge|during] [--force] [--json]
          fledge nest new <doc> | scaffold | scout --module <m> | stamp <file> | status [flags]
          fledge preen [--strict] [--json]
          fledge ready [--json]
          fledge vee [--format text|dot|json] [--json] [PLM-###]
          ...
          fledge unfledged [--plumage] [--feathers] [--json]
          fledge status <ID> [<new-status>] [--force] [--json]
          fledge set <ID> <field> <value> [--json]  (fields: priority, oversight, depends_on, title)
          fledge criteria <ID> [--json] | fledge criteria check|uncheck <ID> <AC-N> [--json]
          fledge brood FTHR-### --owner <name> [--branch <b>] [--worktree <path>] [--json]
          fledge abandon FTHR-### [--fledged] [--force] [--json]
          fledge broods [--stale] [--json]
          fledge heartbeat <name> [--note <text>] [--json]
          fledge await <subject> --kind <kind> [--timeout <duration>] [--json]
          fledge roster [--json] | roster assign (--feather FTHR-### [--pair] | --for <purpose>) [--json] | roster release <name> [--json]
          fledge version [--json]
          fledge update [--yes] [--json]
        [exit status 2]
        FAIL: testdata/dev_status.txtar:8: unexpected command failure

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/dev_status (0.01s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.012s
```

Failed for the expected reason: `dev` was not yet a registered command.

### Post-implementation run (PASSING)

Command: `go test ./cmd/fledge -run TestScripts/dev_status -v`

```
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/dev_status (0.02s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.022s
```

## AC-2

`fledge dev status` in a repo that is not dev-linked (a normal `fledge init`)
and in a repo with no scaffold stamp at all (`git init` only, no `fledge init`
ever run) both exit zero and print `not dev-linked`. Pinned by the
`dev_status.txtar` sections "not dev-linked reports plainly" (lines 3-10) and
"no stamp at all is not an error" (lines 12-19), both asserting `exec fledge
dev status` (no `!` prefix, so testscript requires exit 0) and `stdout 'not
dev-linked'`. See the passing run under AC-1 (same test file, same run).

Implementation: `internal/cli/dev.go:runDevStatus` calls
`bootstrap.LoadStamp(r.Root)`, which returns `(nil, nil)` when
`.fledge/scaffold.json` is absent (`stamp.go:50-63`); `stamp == nil ||
stamp.DevSource == ""` is treated identically as "not dev-linked", printing
the message and returning `ExitOK` — no `RequireFledge()` call, so a repo
with no `.fledge/` directory at all does not error either.

## AC-3

`fledge dev status` in a dev-linked repo (`fledge init --dev=<src>`) reports
the absolute source path and the linked-file count. Pinned by
`dev_status.txtar` "dev-linked reports source and count" (lines 21-30):
asserts `exec fledge dev status` succeeds and stdout matches `dev-linked`,
`source=.*/src` (the absolute fake-source path), and `files=[0-9]+` (18 in
this fixture — 13 core-skill files + 5 default-policy claude agent files, the
full dev-linkable set), with no `broken links` line. See the passing run
under AC-1.

Implementation: `runDevStatus` iterates `stamp.Files`, counting every entry
whose `Policy == "dev-link"` (the shape FTHR-077 writes via `ExpectedFilesDev`
in `stamp.go`), and reports `stamp.DevSource` as the source.

## AC-4

With a dev link's target removed, and separately with the whole source tree
renamed, `fledge dev status` exits non-zero and names every broken path, not
merely the first.

Pinned by `dev_status.txtar`:
- "broken link is detected and named" (lines 32-36): deletes one file
  (`SKILL.md`) from the fake source, asserts `! exec fledge dev status` (must
  fail) and stdout names `.fledge/skills/fledge-orchestrate/SKILL.md`.
- "a moved source reports every broken link" (lines 38-44): renames the
  entire fake source directory, asserts `! exec fledge dev status` and stdout
  names both `.fledge/skills/fledge-orchestrate/SKILL.md` (already broken)
  *and* `.claude/agents/fledge-brooder.md` (newly broken because every
  dev-linked path now dangles at once), proving more than one broken path is
  reported.

See the passing run under AC-1 (same test file). An intermediate development
run (before the fixture restored the deleted `SKILL.md` ahead of the `--json`
section) captured the full broken-path listing verbatim, confirming every
dev-linked path is reported once its source directory is gone, not just the
first — reproduced here:

```
        [stdout]
        dev-linked: source=$WORK/src files=18
        broken links:
          .claude/agents/fledge-brooder.md
          .claude/agents/fledge-context-scout.md
          .claude/agents/fledge-forager.md
          .claude/agents/fledge-incubator.md
          .claude/agents/fledge-skua.md
          .fledge/skills/fledge-interrogate/SKILL.md
          .fledge/skills/fledge-orchestrate/SKILL.md
          .fledge/skills/fledge-orchestrate/brooder.md
          .fledge/skills/fledge-orchestrate/foraging.md
          .fledge/skills/fledge-orchestrate/implementation.md
          .fledge/skills/fledge-orchestrate/incubator.md
          .fledge/skills/fledge-orchestrate/planning.md
          .fledge/skills/fledge-orchestrate/skua.md
          .fledge/skills/fledge-orchestrate/templates/context-doc.md
          .fledge/skills/fledge-orchestrate/templates/feather.md
          .fledge/skills/fledge-orchestrate/templates/plumage.md
          .fledge/skills/fledge-orchestrate/templates/scout-report.md
          .fledge/skills/fledge-orchestrate/worker-protocols.md
        [exit status 1]
```

Implementation: `runDevStatus` checks every `dev-link` entry with `os.Stat`
(follows the symlink) and collects `IsNotExist` failures into `broken`
(`sort.Strings`-ed for deterministic output); `len(broken) > 0` returns
`ExitFail` instead of `ExitOK`.

## AC-5

`fledge dev status --json` on a dev-linked repo emits JSON carrying `linked`,
`source`, `count`, and `broken`. Pinned by `dev_status.txtar` "--json emits
the documented shape" (lines 46-56): asserts stdout matches `"linked": true`,
`"source": `, `"count": [0-9]+`, `"broken": \[\]`. See the passing run under
AC-1; sample output captured during development:

```
{
  "linked": true,
  "source": "$WORK/src",
  "count": 18,
  "broken": []
}
```

Implementation: `devStatusJSON` struct (`internal/cli/dev.go`) with json tags
`linked`, `source`, `count`, `broken`; emitted via the existing `emitJSON`
helper (`cli.go:112`), matching the `preen`/`nest status` JSON-struct idiom.
`Broken` is normalized to `[]string{}` (never nil) before emission so it
renders as `[]`, not `null`, matching the `--json` convention
(`conventions.md`).

## AC-6

Command: `go test ./...`

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	1.239s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.009s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.138s
ok  	github.com/Harrison-Blair/fledge/internal/ledger	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.017s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.017s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.017s
```

`git status` confirms `cmd/fledge/testdata/init.txtar` is untouched (not in
the diff — only `internal/cli/cli.go`, `internal/cli/dev.go`, and the new
`cmd/fledge/testdata/dev_status.txtar` changed).

`"dev"` was added to `commandOrder` (`internal/cli/cli.go:109`):

```
	"update", "dev",
```

and appears in `fledge` usage output:

```
$ fledge
...
  fledge dev status [--json]
```

Also ran `gofmt -l .` (no output — clean) and `go vet ./...` (clean, no
output) before this run.
