# FTHR-054 evidence: fledge roster assign/release/list commands

## AC-1
The tests listed in the feather's Tests section were observed FAILING before
implementation and PASS after; evidence captured verbatim.

Test: `cmd/fledge/testdata/roster.txtar` (new), exercising `fledge roster
assign --feather ... [--pair]`, `fledge roster release <name>`, and
`fledge roster [--json]` (list), including the 18-species overflow and the
pair per-member release semantics.

### Pre-implementation (FAILING) — `roster` command not yet registered

Command:
```
go test ./cmd/fledge -run TestScripts/roster
```

Output (verbatim):
```
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/roster (0.00s)
        testscript.go:584: # fledge roster: assign/release/list subcommands wrapping internal/roster (0.002s)
            # list with no assignments → empty JSON list (not null) (0.001s)
            > exec fledge roster --json
            [stderr]
            fledge: unknown command "roster"

            usage: fledge <command> [args]
            ...
            [exit status 2]
            FAIL: testdata/roster.txtar:7: unexpected command failure

FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.005s
FAIL
```

The command fails at the very first `exec fledge roster --json`: `roster` is an
unknown command (exit 2), confirming the test exercises unimplemented behavior.

### Post-implementation (PASSING)

Command:
```
go test ./cmd/fledge -run TestScripts/roster
```
Output:
```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.045s
```

Same test, same assertions — failing before `internal/cli/roster.go` +
`commandOrder` change, passing after. No test was weakened.

## AC-2
`fledge roster assign --feather FTHR-### --pair` allocates and returns
species/names as specified, with numeric-suffix overflow past 18 (PLM-026 FC-1,
AC-1).

Covered by `roster.txtar`:
- `assign --feather FTHR-001 --pair --json` → two distinct names sharing the
  species `adelie` (`fledge-brooder-adelie`, `fledge-skua-adelie`).
- A second `assign` for a different feather → next unused species `emperor`.
- 18 assigns exhaust the base species (18th = `northernrockhopper`); the 19th
  overflows to `adelie-2`.

Live binary smoke test (`fledge roster ...` in the worktree):
```
$ fledge roster assign --feather FTHR-001 --pair --json
{
  "names": [
    "fledge-brooder-adelie",
    "fledge-skua-adelie"
  ]
}
$ fledge roster assign --feather FTHR-002
emperor
```

## AC-3
`fledge roster release <name>` frees a species only once every member sharing it
is released (PLM-026 FC-2, AC-2).

Covered by `roster.txtar`: releasing `fledge-brooder-adelie` leaves `adelie`
still listed; releasing `fledge-skua-adelie` removes it; a subsequent `assign`
reuses `adelie`. The CLI strips the `fledge-brooder-`/`fledge-skua-` role prefix
to reach `internal/roster`'s species-token space (`speciesToken`).

Live binary smoke test:
```
$ fledge roster release fledge-brooder-adelie
released fledge-brooder-adelie
$ fledge roster --json     # adelie still present, released:[true,false]
    ... "species": "adelie", "released": [ true, false ] ...
$ fledge roster release fledge-skua-adelie
released fledge-skua-adelie
$ fledge roster assign --feather FTHR-003 --json
{ "names": [ "adelie" ] }   # freed species reused
```
Also: releasing an unknown name is a domain failure (exit 1, stderr names it).

## AC-4
`fledge roster [--json]` lists current name→feather assignments, omitting
fully-released species (PLM-026 FC-3, AC-3).

Covered by `roster.txtar`:
- `roster --json` with no assignments → `[]` (empty list, not `null`).
- With assignments → entries with species + feather; text mode prints
  `<species>\t<feather>` lines.
- After a species is fully released it no longer appears (`! stdout 'adelie'`).

Empty-list normalization verified live: `fledge roster --json` on a clean state
prints `[]`.

## AC-5
`go test ./internal/cli/... ./cmd/fledge -run TestScripts` passes.

```
$ go test ./internal/cli/... ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/internal/cli	[no tests to run]
ok  	github.com/Harrison-Blair/fledge/cmd/fledge
exit=0
```

Full suite `go test ./...` is green (all 14 packages ok), including the
`TestCommandOrderMatchesRegistrations` parity test that pins the new `roster`
entry in `commandOrder` against its `register(...)` call:
```
$ go test ./internal/cli -run TestCommandOrderMatchesRegistrations -v
--- PASS: TestCommandOrderMatchesRegistrations (0.00s)
```

Scaffold health after adding `roster` to `commandOrder` (regenerates the Claude
allow-list):
```
$ go install ./cmd/fledge && hash -r
$ fledge init --refresh --force   # settings.local.json now has Bash(fledge roster *)
$ fledge preen
spec clean: 26 plumages, 56 feathers   (exit 0)
```

