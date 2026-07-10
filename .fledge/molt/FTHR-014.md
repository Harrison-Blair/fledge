# FTHR-014 molt evidence

## AC-1

Tests written first in:
- `internal/bootstrap/registry_test.go` — `TestClaudeIncubatorWired`
- `cmd/fledge/testdata/init.txtar` — stdout/exists/grep for incubator
- `cmd/fledge/testdata/init_agents.txtar` — grep for incubator
- `cmd/fledge/testdata/agents.txtar` — exists/grep for incubator after claude init

**Pre-implementation run** (`go test ./internal/bootstrap/... ./cmd/fledge/...`), observed FAILING for expected reason (agent file + manifest entry absent):

```
--- FAIL: TestClaudeIncubatorWired (0.00s)
    registry_test.go:455: claude adapter Files: missing entry for .claude/agents/fledge-incubator.md
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/init (0.02s)
        FAIL: testdata/init.txtar:14: no match for `created .claude/agents/fledge-incubator.md` found in stdout

    --- FAIL: TestScripts/agents (0.03s)
        FAIL: testdata/agents.txtar:24: $WORK/.claude/agents/fledge-incubator.md does not exist

    --- FAIL: TestScripts/init_agents (0.05s)
        FAIL: testdata/init_agents.txtar:105: open $WORK/core-repo/.claude/agents/fledge-incubator.md: no such file or directory

FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.068s
FAIL
```

**Post-implementation run** (after creating `agents/fledge-incubator.md` and adding manifest entry):

```
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.065s
```

## AC-2

`internal/bootstrap/adapters/claude/agents/fledge-incubator.md` created with:
- Frontmatter: `name: fledge-incubator`, description, `model: claude-sonnet-5`
- Body: non-interactive, one-shot, stateless contract; mutates no spec clause

`internal/bootstrap/adapters/claude/manifest.yaml` extended with:
```yaml
  - src: agents/fledge-incubator.md
    dst: .claude/agents/fledge-incubator.md
```
Default copy/skip-if-exists policy (no `overwrite`, `generate`, `symlink`, or `append_if_missing`), matching brooder/forager/scout/skua entries.

`fledge init` now scaffolds `.claude/agents/fledge-incubator.md`. Verified by `TestScripts/init` (`stdout 'created .claude/agents/fledge-incubator.md'`, `exists .claude/agents/fledge-incubator.md`) and `TestScripts/agents` passing.

## AC-3

Agent body defines the input/output contract:
- **Input**: prospective ID, template path, concern docs list, feather-specific fields (plumage link, depends_on, oversight)
- **Action**: reads template and cited docs, drafts full body in one pass
- **Output**: full drafted spec (frontmatter + all sections) as single final message
- **Constraint**: "This agent mutates no spec: do not invoke `fledge new`, `fledge status`, `fledge set`, `fledge criteria`, or any other CLI command."

Stable marker `'mutates no spec'` verified present by `grep 'mutates no spec' .claude/agents/fledge-incubator.md` in all three txtar fixtures (passing).

## AC-4

`TestClaudeIncubatorWired` in `internal/bootstrap/registry_test.go` asserts:
1. `FindAdapter("claude").Files` contains entry with `Dst == ".claude/agents/fledge-incubator.md"`
2. `m.Tier() == "C"` (unchanged)
3. `len(m.TierPrimitives) == len(PrimitiveOrder)` (no new primitive)

Full suite:

```
$ go test ./... && go vet ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap
ok  	github.com/Harrison-Blair/fledge/internal/check
ok  	github.com/Harrison-Blair/fledge/internal/cli
ok  	github.com/Harrison-Blair/fledge/internal/graph
ok  	github.com/Harrison-Blair/fledge/internal/lock
ok  	github.com/Harrison-Blair/fledge/internal/nest
ok  	github.com/Harrison-Blair/fledge/internal/scan
ok  	github.com/Harrison-Blair/fledge/internal/spec
```

`go vet ./...` clean (no output).
