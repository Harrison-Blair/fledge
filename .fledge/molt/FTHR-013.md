# FTHR-013 Evidence

## AC-1

`internal/cli/version_test.go` pins `binaryVersion` == VERSION; observed
failing when only VERSION is bumped, passing when both are bumped.

### Pre-implementation: VERSION bumped to 0.3.0, binaryVersion still 0.2.1

```
$ go test ./internal/cli -run TestBinaryVersionMatchesVersionFile -v -count=1
=== RUN   TestBinaryVersionMatchesVersionFile
    version_test.go:18: binaryVersion = "0.2.1", VERSION file = "0.3.0" — bump internal/cli/version.go
--- FAIL: TestBinaryVersionMatchesVersionFile (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/cli	0.001s
FAIL
```

### Post-implementation: both bumped to 0.3.0

```
$ go test ./internal/cli -run TestBinaryVersionMatchesVersionFile -v -count=1
=== RUN   TestBinaryVersionMatchesVersionFile
--- PASS: TestBinaryVersionMatchesVersionFile (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.001s
```

## AC-2

CLAUDE.md updated to describe preserve/prune refresh semantics and `.fledge/scaffold.json`.
MIGRATION.md gains a `0.2.x → 0.3.0` section covering the stamp file,
deterministic merge-conflict behavior, and the one-time init --refresh step.

See: `CLAUDE.md` (Rebuild/reinstall section and Architecture section), `MIGRATION.md` (new top section).

## AC-3

Local binary built from worktree (`go build -o /tmp/fledge-0.3.0-test ./cmd/fledge`).

### Version output

```
$ /tmp/fledge-0.3.0-test version
fledge 0.3.0
```

### fledge init --refresh at worktree root

```
$ /tmp/fledge-0.3.0-test init --refresh
created .fledge/nest/raw/.gitkeep
created .claude/settings.local.json
created .fledge/scaffold.json
exists .fledge/broods/.gitkeep
exists .fledgeignore
exists pluma/plumage/.gitkeep
exists pluma/feathers/.gitkeep
exists .gitignore
exists .fledge/skills/fledge-interrogate/SKILL.md
exists .fledge/skills/fledge-orchestrate/SKILL.md
exists .fledge/skills/fledge-orchestrate/foraging.md
exists .fledge/skills/fledge-orchestrate/implementation.md
exists .fledge/skills/fledge-orchestrate/planning.md
exists .fledge/skills/fledge-orchestrate/templates/context-doc.md
exists .fledge/skills/fledge-orchestrate/templates/feather.md
exists .fledge/skills/fledge-orchestrate/templates/plumage.md
exists .fledge/skills/fledge-orchestrate/templates/scout-report.md
exists .fledge/skills/fledge-orchestrate/worker-protocols.md
exists .claude/agents/fledge-brooder.md
exists .claude/agents/fledge-forager.md
exists .claude/agents/fledge-context-scout.md
exists .claude/agents/fledge-skua.md
exists .claude/settings.json
exists .claude/team-loop.md
exists .claude/fledge-adapter.md
exists .claude/skills/fledge-orchestrate
exists .claude/skills/fledge-interrogate
scaffolded agents: claude
```

`git status` after: only `.fledge/scaffold.json` was created (untracked), zero
prunes, zero `kept (user-edited)` lines.

### fledge preen after refresh (text + JSON)

```
$ /tmp/fledge-0.3.0-test preen
spec clean: 9 plumages, 13 feathers

$ /tmp/fledge-0.3.0-test preen --json
{
  "findings": [],
  "ok": true,
  "scaffold": {
    "stampVersion": "0.3.0",
    "binaryVersion": "0.3.0",
    "files": [ ... all 26 files "up-to-date" ... ]
  }
}
```

`ok: true`, `stampVersion` and `binaryVersion` both `0.3.0`, all scaffold files `up-to-date`.

## AC-4

### go test ./... -count=1

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.057s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
?   	github.com/Harrison-Blair/fledge/internal/repo	[no test files]
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.002s
```

### go vet ./...

```
(no output — clean)
```
