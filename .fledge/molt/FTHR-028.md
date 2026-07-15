# FTHR-028 evidence

## AC-1
Tests observed failing before implementation, passing after.

### `TestCommandOrderMatchesRegistrations` (internal/cli/command_parity_test.go)

Pre-implementation (unchanged `cli.go`, `commandOrder` missing `"update"`):

```
$ go test ./internal/cli -run TestCommandOrderMatchesRegistrations -v
=== RUN   TestCommandOrderMatchesRegistrations
    command_parity_test.go:20: command "update" is registered but missing from commandOrder
--- FAIL: TestCommandOrderMatchesRegistrations (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/cli	0.001s
FAIL
```

Post-implementation (`"update"` appended to `commandOrder`):

```
$ go test ./internal/cli -run TestCommandOrderMatchesRegistrations -v
=== RUN   TestCommandOrderMatchesRegistrations
--- PASS: TestCommandOrderMatchesRegistrations (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.001s
```

### `cmd/fledge/testdata/init.txtar` (new `grep 'Bash\(fledge update \*\)' .claude/settings.local.json` line)

Pre-implementation (fixture updated, source unchanged) — fails for the expected reason (no such allow-list entry generated yet):

```
$ go test ./cmd/fledge -run TestScripts/init -v
...
FAIL: testdata/init.txtar:29: no match for `Bash\(fledge update \*\)` found in .claude/settings.local.json
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/init (0.00s)
    --- PASS: TestScripts/init_agents (0.04s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.045s
FAIL
```

Post-implementation:

```
$ go test ./cmd/fledge -run TestScripts/init -v
...
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/init (0.01s)
    --- PASS: TestScripts/init_agents (0.04s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.044s
```

`init_agents.txtar` was also checked (per the spec's Affected Modules note) and does not
assert per-command allow-list content — confirmed via `grep -n "update\|Bash(fledge"` across
all `cmd/fledge/testdata/*.txtar`; only `init.txtar` needed a fixture change. Similarly,
`internal/bootstrap/registry_test.go:TestClaudeAllowListGenerated` feeds its own synthetic
`commandOrder` (`init`, `preen`, `brood`) rather than the real one, so it required no change.

## AC-2
`fledge` run with no command lists `update` among its commands (FC-1), captured from the
built binary:

```
$ go build -o /tmp/fledge-ftr028 ./cmd/fledge && /tmp/fledge-ftr028
usage: fledge <command> [args]

commands:
  fledge init [--agent <name>]... [--refresh] [--force] [--list-agents] [--json]
  fledge agents [--json]
  fledge scan [--json]
  fledge new plumage --title <t> [--priority P1] [--agent <s>] [--json]
  fledge new feather --title <t> --plumage PLM-### [--depends-on a,b] [--priority P1] [--oversight merge|during] [--force] [--json]
  fledge nest new <doc> | scaffold | scout --module <m> | stamp <file> [flags]
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
  fledge update [--yes] [--json]
```
(exit code 2, usage error, as expected when run with no command — the listing itself is
what's asserted here.)

## AC-3
The generated `.claude/settings.local.json` produced by `fledge init` contains a
`Bash(fledge update *)` allow-list entry, asserted by the updated `init.txtar` fixture (see
the new grep line and the passing run under AC-1 above). The template
(`internal/bootstrap/adapters/claude/settings.local.json`) ranges over `.CommandOrder`
unchanged — no template edit was needed, only the `commandOrder` slice addition and the
fixture assertion.

## AC-4
`TestCommandOrderMatchesRegistrations` (`internal/cli/command_parity_test.go`) enforces
bidirectional parity: it iterates `commands` (the registration map) checking each name is in
`commandOrder`, and iterates `commandOrder` checking each name resolves in `commands`,
failing loudly by name in either direction. See AC-1 for the failing-then-passing run
demonstrating the direction exercised by this feather (`update` present in `commands`,
absent from `commandOrder`).

## AC-5
`fledge preen` passes and `go test ./...` is green.

```
$ go build ./... && go vet ./... && go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.089s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.160s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.003s
```

```
$ fledge preen
WARN  .fledge/pluma/feathers/FTHR-028-add-update-to-commandorder-and-bidirectional-command-parity-guard-test.md: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-031-make-id-allocation-atomic-with-a-flock-allocation-lock.md: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-032-atomic-brood-file-writes-and-corrupt-file-resilient-broods-listing.md: status hatching but no brood is held
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
5 warning(s)
```
`preen` exits 0 (pass — warnings only). The five warnings are pre-existing repo state
(sibling feathers' brood-claim status and this repo's own scaffold not being refreshed for
this dev worktree) unrelated to and unmodified by this feather's change; they are present
identically before and after this feather's diff.
