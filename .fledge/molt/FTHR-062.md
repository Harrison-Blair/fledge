# FTHR-062 evidence

## AC-1

Pre-implementation failing run. The new assertion `grep '.fledge/scratch/' .gitignore` was added to `cmd/fledge/testdata/init.txtar` (after the `.fledge/roster/` grep) while `gitignoreLines` in `internal/cli/init.go` was still unchanged.

Command: `go test ./cmd/fledge -run TestScripts/init` (output tail):

```
            > exists .claude/team-loop.md
            > exists .fledge/skills/fledge-orchestrate/templates/plumage.md
            > exists .claude/skills/fledge-orchestrate/SKILL.md
            > exists .claude/skills/fledge-interrogate/SKILL.md
            > grep '.fledge/nest/raw/' .gitignore
            > grep '.fledge/broods/' .gitignore
            > grep '.fledge/roster/' .gitignore
            > grep '.fledge/scratch/' .gitignore
            [.gitignore]
            # fledge — per-run intermediates, regenerable
            .fledge/nest/raw/
            .fledge/broods/
            .fledge/roster/
            .alloc.lock
            
            FAIL: testdata/init.txtar:29: no match for `.fledge/scratch/` found in .gitignore
            
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.049s
FAIL
```

The failure is exactly the expected one: the new grep assertion fails because `.fledge/scratch/` is not yet in the scaffolded `.gitignore`.

Post-implementation passing run, after adding `".fledge/scratch/"` to `gitignoreLines` in `internal/cli/init.go`:

Command: `go test ./cmd/fledge -run TestScripts/init` (output tail):

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.050s
```

## AC-2

Built the CLI from this worktree and ran `fledge init` in a fresh git repo, then inspected the written `.gitignore`:

Command:
`go build -o /tmp/fthr062-fledge ./cmd/fledge && mkdir -p /tmp/fthr062-fresh && cd /tmp/fthr062-fresh && git init -q . && /tmp/fthr062-fledge init >/dev/null 2>&1; echo "--- .gitignore ---" && cat .gitignore`

Output:

```
--- .gitignore ---
# fledge — per-run intermediates, regenerable
.fledge/nest/raw/
.fledge/broods/
.fledge/roster/
.fledge/scratch/
.alloc.lock
```

`.fledge/scratch/` is present in the scaffolded `.gitignore`, in the same trailing-slash directory style as `.fledge/roster/` (satisfies PLM-028 FC-1, AC-1).

## AC-3

Command: `go vet ./internal/cli ./cmd/fledge && gofmt -l internal/cli cmd/fledge && go test ./cmd/fledge ./internal/cli`

Output (tail; gofmt printed nothing, vet passed):

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.110s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.010s
```

`go test ./cmd/fledge -run TestScripts/init` passes (included in the `./cmd/fledge` run above and shown directly under AC-1's post-implementation run).
