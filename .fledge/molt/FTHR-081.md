# FTHR-081 evidence — refresh preserves dev links instead of resetting them to copies

## AC-1

Command:

```
go test ./cmd/fledge -run TestScripts/dev_refresh -v
```

Pre-implementation run (unchanged FTHR-078 code), captured verbatim — fails at the second
`fledge init --refresh` (bare, no `--dev`) in a dev-linked repo, exactly the regression
this feather exists to close: refresh drops `devSource`, so `ExpectedFilesDev` reverts
every dev-linked path to a content-hash expectation, and `EditedOnRefresh`'s drift
comparison then tries to read disk content through what is still (on disk) a dangling
symlink to a nonexistent embedded path, and errors out instead of silently overwriting —
either way, dev mode is broken by the refresh:

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
        # --------------------------------------------------------------- (0.002s)
        > exec fledge init --refresh
        [stderr]
        fledge: open $WORK/repo/.fledge/skills/fledge-interrogate/SKILL.md: no such file or directory
        [exit status 1]
        FAIL: testdata/dev_refresh.txtar:22: unexpected command failure

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/dev_refresh (0.01s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.009s
FAIL
```

(Post-implementation run recorded below once the fix lands.)
