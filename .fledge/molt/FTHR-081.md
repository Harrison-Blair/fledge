# FTHR-081 evidence — refresh preserves dev links instead of resetting them to copies

## AC-1

**Recaptured per skua review finding 1.** The first capture below was taken against an
earlier, minimal (2-file) version of the `dev_refresh.txtar` fixture that predates the
"AC-3/AC-6 strengthening" commit (`c150800`), which fleshed the fixture out to the full
scaffolded file list. Against that earlier fixture, the unfixed code crashed immediately at
the bare `--refresh` step. Against the *final, tracked* fixture, the unfixed code's bare
`--refresh` step does not crash — it "succeeds" (exit 0, reports `updated` for every
dev-linked path) while silently writing embedded shipped content *through the still-intact
symlink* into the fake dev source tree, corrupting it; the test then fails two blocks later,
at the AC-3 `devSource` grep, because the rewritten stamp has no `devSource` key. That is a
more severe manifestation of the same bug (real corruption of the developer's source
checkout, not just a crash) and is what the recapture below shows.

Command:

```
go test ./cmd/fledge -run TestScripts/dev_refresh -v
```

Pre-implementation run, reverting only `internal/cli/init.go` to its pre-FTHR-081 state
(`10e3183~1`) and keeping the current tracked `dev_refresh.txtar` as-is, captured verbatim:

```
=== RUN   TestScripts
=== RUN   TestScripts/dev_refresh
=== PAUSE TestScripts/dev_refresh
=== CONT  TestScripts/dev_refresh
    testscript.go:584: WORK=$WORK
        ...
        > exec fledge init --dev=$WORK/src
        [stdout]
        created .fledge/nest/raw/.gitkeep
        created .fledge/broods/.gitkeep
        created .fledgeignore
        created .fledge/pluma/plumage/.gitkeep
        created .fledge/pluma/feathers/.gitkeep
        created .gitignore
        created .fledge/skills/fledge-interrogate/SKILL.md
        created .fledge/skills/fledge-orchestrate/SKILL.md
        created .fledge/skills/fledge-orchestrate/brooder.md
        created .fledge/skills/fledge-orchestrate/foraging.md
        created .fledge/skills/fledge-orchestrate/implementation.md
        created .fledge/skills/fledge-orchestrate/incubator.md
        created .fledge/skills/fledge-orchestrate/planning.md
        created .fledge/skills/fledge-orchestrate/skua.md
        created .fledge/skills/fledge-orchestrate/templates/context-doc.md
        created .fledge/skills/fledge-orchestrate/templates/feather.md
        created .fledge/skills/fledge-orchestrate/templates/plumage.md
        created .fledge/skills/fledge-orchestrate/templates/scout-report.md
        created .fledge/skills/fledge-orchestrate/worker-protocols.md
        created .claude/agents/fledge-brooder.md
        created .claude/agents/fledge-forager.md
        created .claude/agents/fledge-context-scout.md
        created .claude/agents/fledge-skua.md
        created .claude/agents/fledge-incubator.md
        created .claude/settings.json
        created .claude/settings.local.json
        created .claude/team-loop.md
        created .claude/fledge-adapter.md
        created .claude/skills/fledge-orchestrate
        created .claude/skills/fledge-interrogate
        created CLAUDE.md
        created .fledge/scaffold.json
        scaffolded agents: claude
        [stderr]
        note: no agent harness detected; scaffolded the claude adapter by default. Run `fledge init --agent <name>` to add another (see `fledge init --list-agents`).
        > stdout 'created .claude/agents/fledge-brooder.md'
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > stdout $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > exec readlink .fledge/skills/fledge-orchestrate/SKILL.md
        [stdout]
        $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        > stdout $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        # ---------------------------------------------------------------
        # bare --refresh preserves dev links, no --dev given (AC-2, PLM-031 AC-11):
        # every dev-linked path is still a symlink to the same source afterwards.
        # --------------------------------------------------------------- (0.003s)
        > exec fledge init --refresh
        [stdout]
        updated .fledge/skills/fledge-interrogate/SKILL.md
        updated .fledge/skills/fledge-orchestrate/SKILL.md
        updated .fledge/skills/fledge-orchestrate/brooder.md
        updated .fledge/skills/fledge-orchestrate/foraging.md
        updated .fledge/skills/fledge-orchestrate/implementation.md
        updated .fledge/skills/fledge-orchestrate/incubator.md
        updated .fledge/skills/fledge-orchestrate/planning.md
        updated .fledge/skills/fledge-orchestrate/skua.md
        updated .fledge/skills/fledge-orchestrate/templates/context-doc.md
        updated .fledge/skills/fledge-orchestrate/templates/feather.md
        updated .fledge/skills/fledge-orchestrate/templates/plumage.md
        updated .fledge/skills/fledge-orchestrate/templates/scout-report.md
        updated .fledge/skills/fledge-orchestrate/worker-protocols.md
        updated .claude/agents/fledge-brooder.md
        updated .claude/agents/fledge-forager.md
        updated .claude/agents/fledge-context-scout.md
        updated .claude/agents/fledge-skua.md
        updated .claude/agents/fledge-incubator.md
        updated .fledge/scaffold.json
        exists .fledge/nest/raw/.gitkeep
        exists .fledge/broods/.gitkeep
        exists .fledgeignore
        exists .fledge/pluma/plumage/.gitkeep
        exists .fledge/pluma/feathers/.gitkeep
        exists .gitignore
        exists .claude/settings.json
        exists .claude/settings.local.json
        exists .claude/team-loop.md
        exists .claude/fledge-adapter.md
        exists .claude/skills/fledge-orchestrate
        exists .claude/skills/fledge-interrogate
        exists CLAUDE.md
        scaffolded agents: claude
        [stderr]
        note: refreshed 19 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > stdout $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > exec readlink .fledge/skills/fledge-orchestrate/SKILL.md
        [stdout]
        $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        > stdout $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        # source edits saved after the refresh are still live through the repo's
        # scaffold path (AC-2, PLM-031 AC-11) — proves the links are functional,
        # not merely present. (0.000s)
        > cp $WORK/updated-skill.md $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        > exec cat .fledge/skills/fledge-orchestrate/SKILL.md
        [stdout]
        edited directly in the source tree
        > stdout 'edited directly in the source tree'
        # ---------------------------------------------------------------
        # dev source survives in the rewritten stamp (AC-3): guards the
        # evaporate-one-refresh-later failure.
        # --------------------------------------------------------------- (0.000s)
        > grep '"devSource": ' .fledge/scaffold.json
        [.fledge/scaffold.json]
        {
          "fledgeVersion": "0.5.8",
          "agents": [
            "claude"
          ],
          "files": {
            ... (every entry reverted to a plain content hash — "policy": "default" /
            "core" / etc. — no "policy": "dev-link" entries survive; devSource is absent
            entirely)
          }
        }

        FAIL: testdata/dev_refresh.txtar:39: no match for `"devSource": ` found in .fledge/scaffold.json

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/dev_refresh (0.01s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.010s
FAIL
```

Note the `updated .fledge/skills/fledge-interrogate/SKILL.md` etc. lines during the bare
`--refresh`: with the unfixed code, `ExpectedFilesDev` is called with `devSource=""`, so
every dev-linked path is expected as a content hash again. Since the on-disk path is a
symlink whose target now differs from that hash, `writeFileEntry` follows the symlink and
overwrites the *target file in the fake dev source tree* with the embedded shipped content
— i.e. the unfixed refresh silently corrupts the developer's actual source checkout through
the still-live symlink, not just the repo's own scaffold. The `readlink` right after still
shows the same target (the symlink itself is untouched), which is precisely why this
corruption would go unnoticed by only checking `readlink` post-refresh, and precisely why
this feather's tests assert on stamp/content rather than link presence alone.

After restoring the fix (`internal/cli/init.go` back to its current, committed state):

```
go test ./cmd/fledge -run TestScripts/dev_refresh -v
...
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/dev_refresh (0.02s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	(cached)
```

Full suite, all green:

```
$ go test ./... 2>&1 | tail
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	1.275s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.015s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.004s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.018s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.140s
ok  	github.com/Harrison-Blair/fledge/internal/ledger	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.016s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.014s
```

(Reran after rebasing onto main, which by then included FTHR-079's `fledge dev status`
command — see AC-3 below; full suite reconfirmed green post-rebase, see AC-8.)

## AC-2

`fledge init --refresh` with no `--dev` flag, in a dev-linked repo, leaves every dev-linked
path a symlink to the same source, and a source edit made afterwards is immediately visible
through the repo's scaffold path.

Command: `go test ./cmd/fledge -run TestScripts/dev_refresh -v` (script section "bare
--refresh preserves dev links" + "source edits saved after the refresh are still live").
Captured output:

```
        # ---------------------------------------------------------------
        # bare --refresh preserves dev links, no --dev given (AC-2, PLM-031 AC-11):
        # every dev-linked path is still a symlink to the same source afterwards.
        # --------------------------------------------------------------- (0.003s)
        > exec fledge init --refresh
        [stdout]
        exists .fledge/nest/raw/.gitkeep
        ...
        scaffolded agents: claude
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > stdout $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > exec readlink .fledge/skills/fledge-orchestrate/SKILL.md
        [stdout]
        $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        > stdout $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        # source edits saved after the refresh are still live through the repo's
        # scaffold path (AC-2, PLM-031 AC-11) — proves the links are functional,
        # not merely present. (0.000s)
        > cp $WORK/updated-skill.md $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        > exec cat .fledge/skills/fledge-orchestrate/SKILL.md
        [stdout]
        edited directly in the source tree
        > stdout 'edited directly in the source tree'
```

Note: on the refresh line the file listing shows only `exists`, no `updated` — confirming
the dev-linked paths were left untouched (matched their expectation), not rewritten. PASS.

## AC-3

After a refresh, the scaffold stamp still records the dev source, and `fledge dev status`
still reports the repo as dev-linked (guards the evaporate-one-refresh-later failure).

Captured output:

```
        # ---------------------------------------------------------------
        # dev source survives in the rewritten stamp (AC-3): guards the
        # evaporate-one-refresh-later failure.
        # --------------------------------------------------------------- (0.002s)
        > grep '"devSource": ' .fledge/scaffold.json
        > grep $WORK/src .fledge/scaffold.json
        > grep '"policy": "dev-link"' .fledge/scaffold.json
        > exec fledge dev status
        [stdout]
        dev-linked: source=$WORK/src files=18
        > stdout 'dev-linked'
        > stdout 'source=.*/src'
        > ! stdout 'broken links'
```

`fledge dev status` (FTHR-079) landed on `main` mid-implementation of this feather; the
branch was rebased onto it so AC-3 could be tested against the literal command named in
the spec rather than just the raw stamp JSON. PASS.

## AC-4

A refresh of a dev-linked repo still updates rendered/appended files that dev mode does not
cover (FC-11, PLM-031 AC-12); dev links stay untouched by the same run.

Captured output — `.claude/fledge-adapter.md`'s on-disk content is first overwritten with
unrelated ("stale rendered content") bytes, then `--refresh --force` puts the correct
rendered content back while the dev links are unchanged:

```
        # ---------------------------------------------------------------
        # rendered files still refresh under dev mode (AC-4, PLM-031 AC-12): a
        # rendered file that diverged from what fledge would generate is put back;
        # dev links are untouched by the same run.
        # --------------------------------------------------------------- (0.003s)
        > cp $WORK/stale-adapter.md .claude/fledge-adapter.md
        > exec fledge init --refresh --force
        [stdout]
        updated .claude/fledge-adapter.md
        ...
        [stderr]
        note: refreshed 1 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
        > stdout 'updated .claude/fledge-adapter.md'
        > grep 'tier' .claude/fledge-adapter.md
        > ! grep 'stale rendered content' .claude/fledge-adapter.md
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > stdout $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > exec readlink .fledge/skills/fledge-orchestrate/SKILL.md
        [stdout]
        $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
        > stdout $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
```

PASS.

## AC-5

A refresh whose recorded dev source no longer validates exits non-zero, names that path,
and does not silently replace dev links with copies.

Captured output — the fake source tree is deleted (`rm $WORK/src`), then `fledge init
--refresh` (bare) is run:

```
        # ---------------------------------------------------------------
        # refresh with a vanished dev source fails loudly (AC-5): exits non-zero,
        # names the recorded source path, and does not silently replace dev links
        # with copies — the paths stay linked (now dangling), never reverted.
        # --------------------------------------------------------------- (0.002s)
        > cp .fledge/scaffold.json $WORK/before-scaffold.json
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > cp stdout $WORK/before-target.txt
        > rm $WORK/src
        > ! exec fledge init --refresh
        [stderr]
        fledge: dev source $WORK/src (recorded in .fledge/scaffold.json) is no longer a valid fledge source tree: $WORK/src is not a fledge source tree (could not read go.mod): open $WORK/src/go.mod: no such file or directory — re-point with --dev=<path>
        [exit status 1]
        > stderr 'no longer a valid fledge source tree'
        > stderr 'src'
        > cmp .fledge/scaffold.json $WORK/before-scaffold.json
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > cmp stdout $WORK/before-target.txt
```

Exit code non-zero, path named, the stamp is byte-identical to before the failed run
(`cmp`), and the symlink target is unchanged (`cmp`) — no reversion, no partial write.
PASS.

## AC-6

`fledge init --refresh --dev=<path>` re-points an already-dev-linked repo at `<path>` and
records it in the stamp.

Captured output:

```
        # ---------------------------------------------------------------
        # explicit --dev on a refresh re-points an already-dev-linked repo (AC-6).
        # --------------------------------------------------------------- (0.003s)
        > exec fledge init --refresh --dev=$WORK/src2
        [stdout]
        updated .fledge/skills/fledge-interrogate/SKILL.md
        updated .fledge/skills/fledge-orchestrate/SKILL.md
        ...
        updated .fledge/scaffold.json
        ...
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src2/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > stdout $WORK/src2/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > grep $WORK/src2 .fledge/scaffold.json
```

Explicit `--dev=$WORK/src2` on the refresh re-links every dev-linked path to `src2` and the
stamp records the new source. PASS.

## AC-7

Two successive refreshes of a dev-linked repo leave identical link targets.

Captured output:

```
        # ---------------------------------------------------------------
        # refresh is idempotent under dev mode (AC-7): two successive refreshes
        # leave identical link targets.
        # --------------------------------------------------------------- (0.006s)
        > exec fledge init --refresh
        [stdout]
        ...
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src2/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > cp stdout $WORK/target-run1.txt
        > exec fledge init --refresh
        [stdout]
        ...
        > exec readlink .claude/agents/fledge-brooder.md
        [stdout]
        $WORK/src2/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
        > cp stdout $WORK/target-run2.txt
        > cmp $WORK/target-run1.txt $WORK/target-run2.txt
        PASS
```

`cmp` confirms the two runs' captured link targets are byte-identical. PASS.

## AC-8

`go test ./...` passes; `init.txtar` unmodified (non-dev refresh behavior unchanged).

```
$ git status --porcelain -- cmd/fledge/testdata/init.txtar
(no output — file unchanged)

$ go vet ./...
(no output — clean)

$ gofmt -l .
(no output — clean)

$ go test ./... 2>&1 | tail
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	...
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	...
ok  	github.com/Harrison-Blair/fledge/internal/check	...
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	...
ok  	github.com/Harrison-Blair/fledge/internal/cli	...
ok  	github.com/Harrison-Blair/fledge/internal/doctest	...
ok  	github.com/Harrison-Blair/fledge/internal/graph	...
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	...
ok  	github.com/Harrison-Blair/fledge/internal/ledger	...
ok  	github.com/Harrison-Blair/fledge/internal/lock	...
ok  	github.com/Harrison-Blair/fledge/internal/nest	...
ok  	github.com/Harrison-Blair/fledge/internal/repo	...
ok  	github.com/Harrison-Blair/fledge/internal/roster	...
ok  	github.com/Harrison-Blair/fledge/internal/scan	...
ok  	github.com/Harrison-Blair/fledge/internal/spec	...
```

PASS — full run reconfirmed after the rebase onto main (picking up FTHR-079); see
handoff message to the skua for the exact re-run command and timing.
