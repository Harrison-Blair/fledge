# FTHR-078 evidence

## AC-1

Tests written first: `cmd/fledge/testdata/dev_rails.txtar` (all 8 scenarios
from the feather's Tests section — bare self-link, bare outside a checkout,
tracked-path refusal, gitignore + no git-visible links, skew mismatch, skew
match/silent, space-form footgun, general stray positional).

Command: `go test ./cmd/fledge -run TestScripts/dev_rails -v`

**Pre-implementation (against FTHR-077's code, unmodified) — FAILS for the
expected reason**: bare `--dev` is still unconditionally rejected (FTHR-077
scoped that inference out), so the very first scenario fails immediately:

```
> cd srcrepo
$WORK/srcrepo
> exec git init -q .
> exec fledge init --dev
[stderr]
fledge: --dev requires a source path: --dev=<path> (inferring the source from inside a checkout is not yet supported)
[exit status 2]
FAIL: testdata/dev_rails.txtar:10: unexpected command failure

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/dev_rails (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.005s
```

**Post-implementation — PASSES** (full trace below, `.claude` self-detection
note on stderr is expected/benign — no `.claude` dir exists yet in the
fabricated fixtures):

```
=== RUN   TestScripts
=== RUN   TestScripts/dev_rails
...
        # ---- bare --dev inside a fledge source checkout self-links relatively (AC-2) ----
        > cd srcrepo
        > exec git init -q .
        > exec fledge init --dev
        [stdout] created .claude/agents/fledge-brooder.md ... created .fledge/scaffold.json
        > stdout 'created .claude/agents/fledge-brooder.md'
        > stdout 'created .fledge/skills/fledge-orchestrate/SKILL.md'
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout] ../../internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > stdout '^\.\./\.\./internal/bootstrap/adapters/claude/agents/fledge-brooder\.md$'
        > exec readlink .fledge/skills/fledge-orchestrate/SKILL.md
        [stdout] ../../../internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        > stdout '^\.\./\.\./\.\./internal/bootstrap/core/skills/fledge-orchestrate/SKILL\.md$'
        # ---- bare --dev outside a source checkout fails (AC-3) ----
        > ! exec fledge init --dev
        [stderr] fledge: --dev requires a source path outside a fledge source checkout: $WORK/plain is not a fledge source tree (could not read go.mod): open $WORK/plain/go.mod: no such file or directory
        > stderr 'requires a source path'
        > ! exists .claude
        > ! exists .fledge/scaffold.json
        # ---- --dev refuses when a target path is already tracked (AC-4) ----
        > exec fledge init
        > exec git add -A
        > exec git commit -q -m 'track the scaffold'
        > ! exec fledge init --dev=$WORK/src
        [stderr]
        refusing --dev: the following dev-linked path(s) are already tracked by git:
          .claude/agents/fledge-brooder.md
          .claude/agents/fledge-context-scout.md
          ... (all tracked dev-link paths listed)
        untrack them first: git rm --cached .claude/agents/fledge-brooder.md ...
        > stderr 'already tracked'
        > stderr '\.claude/agents/fledge-brooder\.md'
        > stderr 'git rm --cached'
        > exec git status --porcelain
        > cmp stdout $WORK/empty.txt        (passes: working tree exactly as committed)
        > ! exec readlink .claude/agents/fledge-brooder.md   (still a regular file)
        # ---- --dev writes ignore rules and leaves no git-visible dev-linked paths (AC-5) ----
        > exec fledge init --dev=$WORK/src
        > grep 'fledge dev mode' .gitignore
        > grep '^\.claude/agents/fledge-brooder\.md$' .gitignore
        > grep '^\.fledge/skills/fledge-orchestrate/SKILL\.md$' .gitignore
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout] $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > exec git status --porcelain -- .claude/agents .fledge/skills
        > cmp stdout $WORK/empty.txt        (passes: git ignores every dev-linked path)
        # ---- version skew is reported on mismatch, exit still succeeds (AC-6) ----
        > exec fledge init --dev=$WORK/src-skew
        [stderr] fledge: dev source is fledge 9.9.9, binary is 0.5.8 — linked skill/agent prose may reference CLI behavior this binary lacks
        > stderr 'dev source is fledge 9\.9\.9, binary is 0\.5\.8'
        (exit code 0 — a report, not a gate)
        # ---- matching versions report no skew note (AC-6 negative half) ----
        > exec fledge init --dev=$WORK/src-match
        > ! stderr 'dev source is fledge'
        # ---- space form is rejected inside a checkout, not silently mis-linked (AC-8) ----
        > ! exec fledge init --dev $WORK/src
        [stderr] fledge: unexpected argument "$WORK/src" after a bare --dev — use --dev=$WORK/src, not --dev $WORK/src
        > stderr 'unexpected argument'
        > stderr '\-\-dev='
        > ! exec readlink .claude/agents/fledge-brooder.md
        > ! exists .fledge/scaffold.json
        # ---- stray positional is rejected outside a checkout too (AC-8) ----
        > ! exec fledge init somearg
        [stderr] fledge: unexpected argument "somearg"
        > stderr 'unexpected argument'
        PASS
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/dev_rails (0.05s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.104s
```

(Full untruncated trace was inspected during implementation; condensed here to
the per-scenario assertions and their captured output for readability. Every
`stdout`/`stderr`/`cmp`/`exists`/`readlink` line shown above passed.)

## AC-2

Bare `fledge init --dev` inside a fledge source checkout links relatively.
Implemented in `internal/cli/init.go` (`dev.bare` branch validates `r.Root`
itself via `bootstrap.ValidateDevSource`, sets `devSource = r.Root`,
`selfLink = true`) and `internal/bootstrap/registry.go`/`stamp.go`
(`WriteOpts.SelfLink` + new `relDevTarget` helper: `filepath.Rel` from the
linked file's directory to the absolute dev target).

Command: `go test ./cmd/fledge -run TestScripts/dev_rails -v` (first
scenario). Captured above under AC-1's post-implementation trace:
`readlink .claude/agents/fledge-brooder.md` → `../../internal/bootstrap/adapters/claude/agents/fledge-brooder.md`,
`readlink .fledge/skills/fledge-orchestrate/SKILL.md` →
`../../../internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md` — both
relative, both resolve into the same tree.

## AC-3

Bare `--dev` outside a checkout exits non-zero (`fail` → `ExitFail`) naming
"requires a source path", and writes nothing (`! exists .claude`,
`! exists .fledge/scaffold.json`) because the check runs immediately after
`repo.Find()`, before any write. Captured above under AC-1 ("bare --dev
outside a source checkout fails" scenario).

## AC-4

`--dev` refuses when any to-be-linked path is already tracked. Implemented in
`internal/cli/init.go`: `devLinkedPaths` collected from `allFiles` (policy ==
"dev-link") right after the duplicate-skills guard (still a pre-write check),
`trackedDevPaths` shells `git -C <root> ls-files -- <paths>`; any hit prints
the tracked paths and the `git rm --cached` remedy and returns `ExitFail`
before the write phase begins.

Captured above under AC-1 ("--dev refuses when a target path is already
tracked" scenario): stderr names `.claude/agents/fledge-brooder.md` and
`git rm --cached`; `git status --porcelain` after the refusal is byte-empty
(`cmp stdout $WORK/empty.txt` passes — working tree exactly as committed);
`readlink` on the path fails (still a regular file, not a link).

## AC-5

After a successful `--dev`, ignore rules cover every dev-linked path and git
reports none of them as a change. Implemented via `ensureGitignoreBlock`
(factored out of the pre-existing `ensureGitignore`) and a new
`ensureDevGitignore` that writes `devLinkedPaths` as their own labeled,
conditional block in `.gitignore` (only present when dev-linking).

Captured above under AC-1 ("--dev writes ignore rules..." scenario): `.gitignore`
contains the `# fledge dev mode —` header and exact-path lines for
`.claude/agents/fledge-brooder.md` and `.fledge/skills/fledge-orchestrate/SKILL.md`;
`git status --porcelain -- .claude/agents .fledge/skills` is byte-empty (git
ignores every dev-linked path, so none show as untracked or otherwise
changed).

## AC-6

Version skew is reported (stderr, naming both versions) on mismatch without
failing the command, and silent when versions match. Implemented in
`internal/cli/init.go` right after `devSource` is resolved:
`(&repo.Repo{Root: devSource}).Version("")` compared against `binaryVersion`
— matches `cli.go`'s existing stamp-mismatch phrasing idiom, but only fires
when the source's `VERSION` is non-empty (a source tree with no `VERSION`
file is "unknown", not "mismatched", so it stays silent — this is not gated
behavior, per the plumage's open question, just a report).

Captured above under AC-1: `src-skew` (VERSION `9.9.9`) → stderr
`fledge: dev source is fledge 9.9.9, binary is 0.5.8 — ...`, exit 0;
`src-match` (VERSION `0.5.8`, pinned to `binaryVersion`) → no such stderr
line.

## AC-7

Command: `go test ./...`

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/check	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/cli	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/doctest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/graph	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/ledger	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/lock	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/nest	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/repo	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/roster	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/scan	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/spec	(cached)
```

Specifically re-verified `init.txtar` and FTHR-077's `dev.txtar` unmodified
and green:

```
$ go test ./cmd/fledge -run 'TestScripts/init$|TestScripts/dev$' -v
...
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/init (0.02s)
    --- PASS: TestScripts/dev (0.02s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.026s
```

`go build ./...`, `go vet ./...`, and `gofmt -l .` are all clean (no output).

## AC-8

Space form (`fledge init --dev <path>`) exits non-zero naming `--dev=<path>`
as the correct form and creates no links, including inside a fledge source
checkout (the dangerous case). General stray-positional rejection
(`fledge init somearg`) also exits non-zero. Implemented as a single
`fs.NArg() > 0` check in `internal/cli/init.go`, placed immediately after
`fs.Parse` (before any repo resolution or write): when `dev.bare` is also
true it names the `--dev=` fix specifically; otherwise a generic "unexpected
argument" usage error.

Captured above under AC-1 ("space form is rejected..." and "stray positional
is rejected..." scenarios): stderr for the space form names
`--dev=$WORK/src`; `readlink` on the would-be-linked path fails and no
`.fledge/scaffold.json` is written; the bare positional case likewise exits
non-zero with "unexpected argument".
