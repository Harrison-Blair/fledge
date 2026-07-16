# FTHR-056 evidence — Tighten force-terminate wording to the kill-primitive holder

Feather: `.fledge/pluma/feathers/FTHR-056-tighten-force-terminate-wording-to-the-kill-primitive-holder.md`
Branch: `feather/FTHR-056-force-terminate-wording`

## AC-1
The test listed in the feather's Tests section was observed FAILING before
implementation and PASSING after; evidence captured verbatim.

Test edited first: `cmd/fledge/testdata/forager_contract.txtar` — the fixture
that already asserts on the scaffolded `foraging.md` force-terminate wording
(FTHR-041 AC-3). New assertions require the tightened wording (naming the
orchestrator / kill-primitive holder) and require the old
`(the incubator or the orchestrator)` disjunction to be gone.

New assertions added (run against the scaffolded `foraging.md` produced by
`fledge init` inside the testscript):

```
grep 'to force-terminate you if you do not exit promptly' .fledge/skills/fledge-orchestrate/foraging.md
grep 'force-terminates it if it does not exit promptly' .fledge/skills/fledge-orchestrate/foraging.md
grep -count=2 'party holding the .spawn-worker./kill primitive' .fledge/skills/fledge-orchestrate/foraging.md
! grep 'the incubator or the orchestrator' .fledge/skills/fledge-orchestrate/foraging.md
! grep 'expect it to force-terminate you' .fledge/skills/fledge-orchestrate/foraging.md
! grep 'worker that commissioned it' .fledge/skills/fledge-orchestrate/foraging.md
```

### Fail-first (against current/old embedded content, BEFORE editing foraging.md)

Command:
```
go test ./cmd/fledge -run 'TestScripts/forager_contract'
```

Output (verbatim):
```
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/forager_contract (0.01s)
            FAIL: testdata/forager_contract.txtar:35: no match for `force-terminates it if it does not exit promptly` found in .fledge/skills/fledge-orchestrate/foraging.md
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.012s
FAIL
```

The old scaffolded `foraging.md` uses `force-terminating it if it does not exit
promptly` (Commissioner) and `expect it to force-terminate you` with the
`(the incubator or the orchestrator)` disjunction (Forager Lifecycle), so the
new assertions fail on the current content as required.

### Pass (after editing embedded foraging.md)

Command:
```
go test ./cmd/fledge -run 'TestScripts/forager_contract'
```

Output (verbatim):
```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.007s
```

The testscript compiles `fledge` from the current source tree, so the pass
reflects the newly-edited embedded `foraging.md` scaffolding out through
`fledge init`. All six new assertions (2 positive tightened-wording greps, 1
`-count=2` kill-primitive-holder grep, 3 negative old-phrasing greps) hold.

## AC-2
`foraging.md`'s force-terminate wording names the orchestrator as the
kill-primitive holder instead of the "commissioner" disjunction (satisfies
FC-1).

Both force-terminate spots in `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md`
now name the kill-primitive holder; the old
`(the incubator or the orchestrator)` disjunction is gone:

```
$ grep -n "party holding the .spawn-worker./kill primitive\|force-terminates it\|to force-terminate you" internal/bootstrap/core/skills/fledge-orchestrate/foraging.md
25: ... request the forager's graceful shutdown by name; the party holding the `spawn-worker`/kill primitive (on Claude Code, the orchestrator, `team-lead`) force-terminates it if it does not exit promptly ...
68: ... expect the orchestrator (on Claude Code, `team-lead`; the party holding the `spawn-worker`/kill primitive) to force-terminate you if you do not exit promptly ...

$ grep -c "the incubator or the orchestrator\|expect it to force-terminate you\|worker that commissioned it" internal/bootstrap/core/skills/fledge-orchestrate/foraging.md
0
```

Behavioral meaning preserved: the commissioner still *requests* shutdown by
name (a message the incubator can send), only the *force-terminate* action is
attributed to the party that actually holds the `spawn-worker`/kill primitive
(the orchestrator, `team-lead`, on Claude Code); confirmed-shutdown definition
and "species frees only once shutdown is confirmed" are unchanged. Wording-only,
no behavior change (matching PLM-021's prose-only framing).

## AC-3
`worker-protocols.md`'s Incubator Lifecycle section was checked for the same
phrasing and tightened to match if present.

Checked — the Incubator Lifecycle section (and every other Lifecycle section)
in `worker-protocols.md` already names **the orchestrator** for both the
shutdown request and the force-terminate; it does NOT use the
`(incubator or orchestrator)` disjunction. No change was needed (and none was
made), so nothing to tighten:

```
$ grep -n "expect the orchestrator to force-terminate you" internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md
40: ... expect the orchestrator to force-terminate you if you do not exit promptly ...   (Incubator Lifecycle)
72: ... expect the orchestrator to force-terminate you ...   (Brooder Lifecycle)
110: ... expect the orchestrator to force-terminate you ...   (Skua Lifecycle)

$ grep -c "the incubator or the orchestrator\|worker that commissioned it" internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md
0
```

## AC-4
`fledge init --refresh` regenerates this repo's scaffolded copies to match, and
`go test ./cmd/fledge -run TestScripts` passes.

```
$ fledge init --refresh
note: refreshed 2 file(s) to the shipped versions ...
updated .fledge/skills/fledge-orchestrate/foraging.md
updated .fledge/scaffold.json
scaffolded agents: claude

$ diff -q internal/bootstrap/core/skills/fledge-orchestrate/foraging.md .fledge/skills/fledge-orchestrate/foraging.md
(no output — MATCH)

$ go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.096s
```

(Binary reinstalled from this worktree first: `go install ./cmd/fledge && hash -r`.)

## AC-5
`go test ./...` passes and `fledge preen` reports the scaffold healthy after the
refresh.

```
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.100s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.009s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.124s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.014s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.012s

$ fledge preen
spec clean: 26 plumages, 56 feathers
(exit 0 — scaffold stamp present and consistent, no scaffold warnings)
```

gofmt (`gofmt -l .` → no files) and `go vet ./...` (no output) are also clean.
