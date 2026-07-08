# FTHR-005 Evidence

## AC-1

Tests listed above were observed failing before implementation and pass after.

### Pre-implementation failures

**Command:** `go test ./internal/nest/...`
```
github.com/Harrison-Blair/fledge/internal/nest: no non-test Go files in /home/penguin/source/fledge/.fledge/burrows/FTHR-005/internal/nest
FAIL	github.com/Harrison-Blair/fledge/internal/nest [build failed]
FAIL
```

**Command:** `go test ./cmd/fledge -run TestScripts/nest`
```
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/nest (0.00s)
        testscript.go:584: # fledge nest new: creates known concern docs, rejects unknown, --force, --json (0.002s)
            # nest new creates a known doc in .fledge/nest/ (0.001s)
            > exec fledge nest new architecture
            [stderr]
            fledge: unknown command "nest"
            
            usage: fledge <command> [args]
            
            commands:
              fledge init [--agent <name>]... [--refresh] [--list-agents] [--json]
              fledge agents [--json]
              fledge scan [--json]
              fledge new plumage --title <t> [--priority P1] [--agent <s>] [--json]
              fledge new feather --title <t> --plumage PLM-### [--depends-on a,b] [--priority P1] [--oversight merge|during] [--force] [--json]
              fledge preen [--strict] [--json]
              fledge ready [--json]
              fledge vee [--format text|dot|json] [--json] [PLM-###]
              fledge colony [--json]
              fledge unfledged [--plumage] [--feathers] [--json]
              fledge status <ID> [<new-status>] [--force] [--json]
              fledge set <ID> <field> <value> [--json]  (fields: priority, oversight, depends_on, title)
              fledge criteria <ID> [--json] | fledge criteria check|uncheck <ID> <AC-N> [--json]
              fledge brood FTHR-### --owner <name> [--branch <b>] [--json]
              fledge abandon FTHR-### [--fledged] [--force] [--json]
              fledge broods [--json]
              fledge version [--json]
            [exit status 2]
            FAIL: testdata/nest.txtar:6: unexpected command failure
            
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.004s
FAIL
```

Expected reasons: `internal/nest` package does not exist; `fledge nest` command is not registered.

### Post-implementation passing run

**Command:** `go test ./internal/nest/... -v`
```
=== RUN   TestConcernFrontmatterKeyOrder
--- PASS: TestConcernFrontmatterKeyOrder (0.00s)
=== RUN   TestScoutFrontmatterKeyOrder
--- PASS: TestScoutFrontmatterKeyOrder (0.00s)
=== RUN   TestRenderPreservesBody
--- PASS: TestRenderPreservesBody (0.00s)
=== RUN   TestYAMLScalarQuoting
--- PASS: TestYAMLScalarQuoting (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
```

**Command:** `go test ./cmd/fledge -run TestScripts/nest`
```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.046s
```

**Command:** `go test ./... && go vet ./...`
```
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

## AC-2

`internal/nest` renders both frontmatter schemas in canonical fixed-key order with body byte-preservation.

Verified by unit tests (see AC-1 post-implementation run):
- `TestConcernFrontmatterKeyOrder` — pins `generated/commit/agent/fledge_version` order and both `---` fences.
- `TestScoutFrontmatterKeyOrder` — pins `module/authored/agent/fledge_version` order and both `---` fences.
- `TestRenderPreservesBody` — `SplitFrontmatter(Render())` returns body bytes unchanged.

**Command:** `go test ./internal/nest/... -v -run TestConcernFrontmatterKeyOrder`
```
=== RUN   TestConcernFrontmatterKeyOrder
--- PASS: TestConcernFrontmatterKeyOrder (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
```

## AC-3

`fledge nest new <doc>` behavior verified by `cmd/fledge/testdata/nest.txtar`:
- Creates `.fledge/nest/architecture.md` with stamped frontmatter (`generated`, `commit`, `fledge_version`) and `# Architecture` body heading.
- Unknown doc `nest new bogus` → exit 2, stderr contains `bogus`.
- Existing file without `--force` → exit 1, stderr contains `architecture`.
- Existing file with `--force` → overwrites, exit 0.
- `--json` emits `{"path": ".fledge/nest/modules.md"}` shape.

**Command:** `go test ./cmd/fledge -run TestScripts/nest`
```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.046s
```

## AC-4

`nest` added to `commandOrder` in `internal/cli/cli.go` at position after `new`. The generated Claude `settings.local.json` now includes `Bash(fledge nest *)`. The `init.txtar` fixture updated to assert `grep 'fledge nest' .claude/settings.local.json`. All of `go test ./...` and `go vet ./...` pass.

**Command:** `go test ./... && go vet ./...`
```
(all packages pass — see AC-1 post-implementation run)
```
