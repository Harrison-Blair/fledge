# FTHR-055 — Wire implementation.md spawn/teardown/resume to fledge roster — evidence

## AC-1
Test observed failing before implementation, passing after; captured verbatim.

Test added to `cmd/fledge/testdata/init.txtar` (block after the FTHR-047 colony
assertions): asserts the scaffolded `implementation.md` §3.1 instructs
`fledge roster assign --feather FTHR-### --pair` (and documents the solo
`without --pair` form + references `internal/roster`'s species list by package
name), §3.2 instructs `fledge roster release <name>`, and §6 instructs running
`fledge roster` for resume reconstruction; negative assertions confirm the old
"18 extant penguin species" bare-count assertion, the "mapping internally"
note, and "clear the roster" bookkeeping language are gone.

### Before implementation — FAIL (against current scaffolded content)

Command (run from worktree, embedded source still the old prose):
```
$ go test ./cmd/fledge -run TestScripts/init 2>&1 | tail -6
```
Output:
```
FAIL: testdata/init.txtar:98: no match for `fledge roster assign --feather FTHR-### --pair` found in .fledge/skills/fledge-orchestrate/implementation.md

FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.049s
FAIL
```
The first new positive assertion has no match because the current
`implementation.md` still uses in-context bookkeeping prose (the old
allocation rule, the "18 extant penguin species" count, and the enumerated
species list) — exactly the language this feather replaces.

### After implementation — PASS

Rewrote §3.1/§3.2/§6 in the embedded source
(`internal/bootstrap/core/skills/fledge-orchestrate/implementation.md`),
`go install ./cmd/fledge` + `hash -r`, `fledge init --refresh` to regenerate
the scaffolded copy, then:
```
$ go test ./cmd/fledge -run TestScripts 2>&1 | tail -1
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.103s
```
The `init` script (which contains the new assertions at
`testdata/init.txtar:98`ff) passes: all positive greps match and all negative
greps confirm the old bookkeeping language is gone.

## AC-2
§3.1, §3.5(→§3.2), and §6 rewritten per FC-5 (satisfies PLM-026 FC-5, AC-5).

Verified against the scaffolded `.fledge/skills/fledge-orchestrate/implementation.md`:

```
$ f=.fledge/skills/fledge-orchestrate/implementation.md
--- POSITIVE (each appears) ---
fledge roster assign --feather FTHR-### --pair:        FOUND   (§3.1 pair spawn)
without `--pair`:                                      FOUND   (§3.1 solo spawn)
internal/roster:                                       FOUND   (§3.1 species list by package)
fledge roster release <name>:                          FOUND   (§3.2 teardown)
run `fledge roster` to read the current name:          FOUND   (§6 resume)
--- NEGATIVE (each gone) ---
18 extant penguin species:              gone   (bare-count assertion removed)
mapping internally:                     gone   ("keep name→feather mapping internally" dropped)
clear the roster:                       gone   (§6 no longer clears the roster)
erect-crested:                          gone   (enumerated species list removed)
append a numeric suffix:                gone   (CLI now handles overflow)
```

Every other instruction in the three sections was preserved: §3.1's
"one species per **worker pair**", scouts-exempt, orchestrator-takes-no-species,
the roster-delta report line, and the **confirmed shutdown** definition; §3.2's
graceful-shutdown-then-force-terminate step; §6's "respawn a fresh pair into
the existing worktree" (step 3, untouched).

## AC-3
`fledge init --refresh` regenerated this repo's scaffolded copy to match, and
`go test ./cmd/fledge -run TestScripts` passes.

```
$ fledge init --refresh   # regenerated .fledge/skills/.../implementation.md + scaffold.json
$ git status --short
 M .fledge/scaffold.json
 M .fledge/skills/fledge-orchestrate/implementation.md
 M cmd/fledge/testdata/init.txtar
 M internal/bootstrap/core/skills/fledge-orchestrate/implementation.md
?? .fledge/molt/FTHR-055.md

$ go test ./cmd/fledge -run TestScripts 2>&1 | tail -1
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.103s
```

## AC-4
`go test ./...` passes and `fledge preen` reports the scaffold healthy after the
refresh (satisfies PLM-026 AC-6).

```
$ go test ./... 2>&1 | tail -20
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.106s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.011s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.123s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.006s

$ fledge preen; echo "exit $?"
spec clean: 26 plumages, 56 feathers
exit 0
```
`fledge preen` exits 0 — it validates `.fledge/scaffold.json` presence and
consistency (scaffold healthy) in addition to spec cleanliness. `gofmt -l .`
clean and `go vet ./...` clean.

