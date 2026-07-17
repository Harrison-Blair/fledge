# FTHR-077 evidence

## AC-1

Test-first: `cmd/fledge/testdata/dev.txtar` was written before any implementation
change, then run against unchanged code.

Command:

```
go test ./cmd/fledge -run TestScripts/dev -v
```

Verbatim output (pre-implementation, captured at the time — full failure, only
the first sub-test ran before the script aborted):

```
        > mkdir repo
        > cd repo
        $WORK/repo
        > exec git init -q .
        > exec fledge init --dev=$WORK/src
        [stderr]
        flag provided but not defined: -dev
        Usage of init:
          -agent value
            	agent harness to scaffold (repeatable, comma-separated)
          -force
            	with --refresh: skip the confirmation prompt and overwrite user-edited files
          -json
            	machine-readable output
          -list-agents
            	list available agent adapters and exit
          -refresh
            	reset all fledge-owned files to the shipped versions (confirms before overwriting user-edited files)
        [exit status 2]
        FAIL: testdata/dev.txtar:12: unexpected command failure

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/dev (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.004s
```

Expected failure reason confirmed: unknown flag `-dev` (matches the spec's
Tests section). No test was weakened or skipped to reach green; implementation
follows below, and the passing re-run is recorded under AC-2/AC-7.

Post-implementation, the same command now passes in full:

```
go test ./cmd/fledge -run TestScripts/dev -v
...
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/dev (0.02s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.017s
```

## AC-2

`fledge init --dev=<path>` writes the copy-type scaffold (agent definitions,
core skill docs) as symlinks into `<path>`, with absolute targets.

Test: `dev.txtar` sub-test "links copy-type scaffold to source".

```
go test ./cmd/fledge -run TestScripts/dev -v
```

Relevant excerpt:

```
        > exec fledge init --dev=$WORK/src
        stdout 'created .claude/agents/fledge-brooder.md'
        stdout 'created .fledge/skills/fledge-orchestrate/SKILL.md'
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > stdout $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > exec readlink .fledge/skills/fledge-orchestrate/SKILL.md
        [stdout]
        $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        > stdout $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
```

`readlink` (not `exists`, which follows links) confirms both the adapter file
and the core skill doc are real symlinks whose target is the absolute path
inside the dev source tree — `internal/bootstrap/adapters/<name>/<src>` and
`internal/bootstrap/core/<rel>` respectively.

Implementation: `internal/bootstrap/registry.go` — `WriteOpts.DevSource`,
`writeDevLink`, `devCoreTarget`/`devAdapterTarget`; the default-policy branch
in `writeFileEntry` and the per-file loop in `WriteCore` both link into
`opts.DevSource` when it's set. `internal/cli/init.go` resolves and validates
`--dev=<path>` and threads it into `WriteOpts.DevSource`.

## AC-3

An edit saved in the source tree is visible through the consuming repo's
scaffold path with no rebuild, reinstall, or `fledge` command run in between.

Test: `dev.txtar` sub-test "source edits are live".

```
        > cp $WORK/updated-skill.md $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        > exec cat .fledge/skills/fledge-orchestrate/SKILL.md
        [stdout]
        edited directly in the source tree
        > stdout 'edited directly in the source tree'
```

The edit is written straight to the fake source tree with `cp` (no `fledge`
invocation), and reading the consuming repo's scaffolded path
(`.fledge/skills/fledge-orchestrate/SKILL.md`, a symlink) via plain `cat`
immediately observes it — this demonstrates PLM-031 AC-1's headline behavior.

## AC-4

Rendered/merged files (`.claude/fledge-adapter.md`, `.claude/settings.local.json`,
the `CLAUDE.md` append line) are still produced as before and are not symlinks.

Test: `dev.txtar` sub-test "rendered/merged files are not links", run in the
same `--dev`-initialized repo as AC-2/AC-3.

```
        > exists .claude/fledge-adapter.md
        > exists .claude/settings.local.json
        > ! exec readlink .claude/fledge-adapter.md
        [exit status 1]
        > ! exec readlink .claude/settings.local.json
        [exit status 1]
        > grep 'tier' .claude/fledge-adapter.md
        > grep 'fledge: load and follow .fledge/skills/fledge-orchestrate/SKILL.md' CLAUDE.md
```

`readlink` exiting non-zero on both files proves they are regular files, not
links; `grep` confirms they hold real rendered/appended content. Implementation:
the dev-link override in `writeFileEntry` (registry.go) sits in the *default*
policy branch only, after the `Generate`/`PrimitiveMap`/`Overwrite` branches
have already returned — those policies are untouched by dev mode, matching
`ExpectedFilesDev` in stamp.go which only substitutes a `dev-link` entry for
`f.Generate`/`f.PrimitiveMap`/`f.Overwrite`-free (i.e. default-policy) files.

## AC-5

`--dev` against a path whose `go.mod` doesn't declare the fledge module (or
that has no `go.mod` at all) exits non-zero, names the path, and leaves every
scaffold file byte-identical — validated before any write.

Test: `dev.txtar` sub-tests "invalid source is rejected before any write" and
"a nonexistent source path is rejected the same way".

```
        > exec fledge init
        [stdout] ... (normal init succeeds)
        > cp .fledge/scaffold.json $WORK/before-scaffold.json
        > cp .claude/agents/fledge-brooder.md $WORK/before-brooder.md
        > ! exec fledge init --dev=$WORK/bad-src
        [stderr]
        fledge: $WORK/bad-src is not a fledge source tree (go.mod does not declare module github.com/Harrison-Blair/fledge)
        [exit status 1]
        > stderr 'not a fledge source tree'
        > stderr 'bad-src'
        > cmp .fledge/scaffold.json $WORK/before-scaffold.json
        > cmp .claude/agents/fledge-brooder.md $WORK/before-brooder.md
        > ! exec readlink .claude/agents/fledge-brooder.md
        [exit status 1]

        > ! exec fledge init --dev=$WORK/does-not-exist
        [stderr]
        fledge: $WORK/does-not-exist is not a fledge source tree (could not read go.mod): open $WORK/does-not-exist/go.mod: no such file or directory
        [exit status 1]
        > stderr 'not a fledge source tree'
        > stderr 'does-not-exist'
        > cmp .fledge/scaffold.json $WORK/before-scaffold.json
```

`cmp` against a pre-run copy of the stamp and an agent file (both taken right
after a successful plain `fledge init`) passes — proves byte-for-byte identity
after the rejected `--dev` attempt — and `readlink` failing confirms the file
was never converted to a link. Implementation: `bootstrap.ValidateDevSource`
(registry.go) runs in `runInit` before `repo.Find`/any manifest resolution or
write, so a validation failure returns before touching the filesystem.

Also covered (decided detail, not a numbered AC but explicitly called for): the
space-separated form `fledge init --dev <path>` parses as a bare `--dev` plus a
stray positional argument. Rather than silently linking nothing (or the wrong
thing) it errors clearly:

```
        > ! exec fledge init --dev $WORK/src
        [stderr]
        fledge: unexpected argument "$WORK/src" after a bare --dev — use --dev=$WORK/src, not --dev $WORK/src
        [exit status 2]
        > stderr 'unexpected argument'
        > ! exec readlink .claude/agents/fledge-brooder.md
        > cmp .fledge/scaffold.json $WORK/before-scaffold.json
```

And a bare `--dev` with no path at all (source inference is FTHR-078's FC-3,
out of scope here) also errors rather than guessing:

```
        > ! exec fledge init --dev
        [stderr]
        fledge: --dev requires a source path: --dev=<path> (inferring the source from inside a checkout is not yet supported)
        [exit status 2]
        > stderr 'requires a source path'
        > ! exists .claude
```

## AC-6

The scaffold stamp records the absolute dev source path, and linked entries
record their symlink target rather than a content hash.

Test: `dev.txtar` sub-test "dev state recorded in stamp".

```
        > grep '"devSource": ' .fledge/scaffold.json
        > grep $WORK/src .fledge/scaffold.json
        > grep '"policy": "dev-link"' .fledge/scaffold.json
        > ! grep '"policy": "core"' .fledge/scaffold.json
```

All four assertions pass: `Stamp.DevSource` (`internal/bootstrap/stamp.go`) is
populated with the absolute source path; every copy-type entry's `Policy` is
`"dev-link"` with `Target` set to the absolute source path (via
`ExpectedFilesDev`, which is what `drift.go`'s `classifySymlink` (keyed off
`StampEntry.Target != ""`, not the policy string) already knows how to compare
— no new drift machinery was added, per the spec's constraint). No entry keeps
the old content-hash `"core"` policy once dev mode is active.

## AC-7

Re-running the same `fledge init --dev=<path>` succeeds and leaves every dev
link unchanged (no `created`/`updated`, only `exists`).

Test: `dev.txtar` sub-test "re-running --dev is idempotent".

```
        > exec fledge init --dev=$WORK/src
        [stdout]
        exists .fledge/nest/raw/.gitkeep
        ...
        exists .claude/agents/fledge-brooder.md
        ...
        scaffolded agents: claude
        > ! stdout 'created'
        > ! stdout 'updated'
        > stdout 'exists .claude/agents/fledge-brooder.md'
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > stdout $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
```

Implementation: `writeDevLink` (registry.go) checks `os.Readlink` against the
target first and reports `devLinkUnchanged` (classified as `exists`) when the
symlink already points at the right place — it only removes-and-recreates when
the on-disk state differs (a real file, or a link to somewhere else), which is
also what makes converting a previously-copied file into a dev link safe on
first use.

## AC-8

`go test ./...` passes, and `cmd/fledge/testdata/init.txtar` is unmodified.

```
$ git diff --stat cmd/fledge/testdata/init.txtar
(no output — file unchanged from main)

$ go clean -testcache && go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.112s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.013s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.173s
ok  	github.com/Harrison-Blair/fledge/internal/ledger	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.008s
```

Also `go vet ./...` and `gofmt -l .` both report nothing.

`git status --short` at this point shows only the intended change set:

```
 M internal/bootstrap/registry.go
 M internal/bootstrap/stamp.go
 M internal/cli/init.go
?? .fledge/molt/FTHR-077.md
?? cmd/fledge/testdata/dev.txtar
```

`ExpectedFiles`'s exported signature (`func ExpectedFiles(m *Manifest,
commandOrder []string) (map[string]StampEntry, error)`) is unchanged — dev
support is a new `ExpectedFilesDev(m, commandOrder, devSource)` that
`ExpectedFiles` now delegates to with `devSource=""`, per the plan to keep
FTHR-080's later `drift.go`-only fix from colliding with this feather's changes
to `init.go`.
