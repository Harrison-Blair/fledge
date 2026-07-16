# FTHR-041 evidence

## AC-1

Tests T1–T6 were added as new/extended `grep`/`! grep` assertions in
`cmd/fledge/testdata/{init_agents,forager_contract,plan_delegation,init,agents}.txtar`.

Pre-implementation: stashed only the five prose files (leaving the new txtar
assertions in place against the *original* unedited scaffolded output), then ran:

```
git stash push -m "FTHR-041 prose edits" -- \
  internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md \
  internal/bootstrap/core/skills/fledge-orchestrate/foraging.md \
  internal/bootstrap/core/skills/fledge-orchestrate/planning.md \
  internal/bootstrap/adapters/claude/team-loop.md \
  internal/bootstrap/adapters/claude/agents/fledge-forager.md

go test ./cmd/fledge -run 'TestScripts/(init|init_agents|forager_contract|plan_delegation|agents)$' -v
```

Captured verbatim failures (one per new assertion, each failing for the expected
reason — the added phrase absent from the current scaffolded output):

```
FAIL: testdata/plan_delegation.txtar:18: no match for `force-terminating it if it does not exit promptly` found in .fledge/skills/fledge-orchestrate/planning.md
FAIL: testdata/forager_contract.txtar:31: no match for `expect it to force-terminate you if you do not exit promptly` found in .fledge/skills/fledge-orchestrate/foraging.md
FAIL: testdata/init.txtar:63: no match for `all four teammate roles — brooder, skua, forager, incubator` found in .claude/team-loop.md
FAIL: testdata/agents.txtar:28: no match for "Gate your final message on \`fledge nest status\`" found in .claude/agents/fledge-forager.md
FAIL: testdata/init_agents.txtar:160: have 2 matches for `force-terminate you if you do not exit promptly, since acknowledging a shutdown request is not the same as ending your session`, want 3

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/plan_delegation (0.00s)
    --- FAIL: TestScripts/forager_contract (0.00s)
    --- FAIL: TestScripts/init (0.01s)
    --- FAIL: TestScripts/agents (0.01s)
    --- FAIL: TestScripts/init_agents (0.05s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.049s
```

Note: `init_agents.txtar:160`'s failure ("have 2 matches ... want 3") is the
expected reason for T1 — the sentence already exists twice (Brooder, Skua
Lifecycle) and the test pins the count going to 3 once the Incubator Lifecycle
sentence is added. `plan_delegation.txtar:18`'s "no match ... found" (T3) is
the expected reason for its `-count=2` assertion: the phrase
`force-terminating it if it does not exit promptly` does not appear anywhere
in the unedited `planning.md` (neither §0 nor §2 has it yet), so the count
check reports no match at all rather than a partial 1-of-2.

Then restored the prose edits (`git stash pop`), reinstalled with the fix in
place, and re-ran the same command — all 5 scripts pass:

```
go build -o fledge ./cmd/fledge && go install ./cmd/fledge && hash -r && fledge version
# fledge 0.5.5 (matches VERSION)

go test ./cmd/fledge -run 'TestScripts/(init|init_agents|forager_contract|plan_delegation|agents)$' -v
...
--- PASS: TestScripts (0.00s)
```

Full suite green post-implementation:

```
go build ./... && go vet ./... && go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.085s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.013s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.009s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.134s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s
```

## AC-2

`internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md`'s
Incubator Lifecycle section now reads:

> An incubator lives for one planning phase. After sending `PHASE-CLOSE` it
> stays alive and addressable — the user may still send follow-up changes
> through the orchestrator. The orchestrator requests its shutdown by name
> once the phase is closed out; comply promptly — and expect the orchestrator
> to force-terminate you if you do not exit promptly, since acknowledging a
> shutdown request is not the same as ending your session. An incubator never
> marks specs beyond `hatched` — `hatching` and `fledged` are
> implementation-phase states.

This mirrors the pre-existing Brooder/Skua Lifecycle sentences verbatim (same
clause structure), per commit 96a3ac3's template. Verified by
`init_agents.txtar`'s new `grep -count=3 'force-terminate you if you do not
exit promptly...'` assertion passing (T1) — see AC-1 output above.

## AC-3

`internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` now documents
the backstop in both required places:

- Forager Lifecycle: "...will request its shutdown by name once the nest
  output is verified — comply promptly, and expect it to force-terminate you
  if you do not exit promptly, since acknowledging a shutdown request is not
  the same as ending your session."
- Commissioner "On the final message, verify and release": "...request the
  forager's graceful shutdown by name, force-terminating it if it does not
  exit promptly — acknowledging a shutdown request is not the same as ending
  its session. Its species frees only once shutdown is confirmed."

Verified by `forager_contract.txtar`'s two new `grep` assertions (T2) passing
— see AC-1 output above.

## AC-4

`internal/bootstrap/core/skills/fledge-orchestrate/planning.md`:

- §0 incubator-release: "...then request the incubator's graceful shutdown by
  name, force-terminating it if it does not exit promptly; its species frees
  once shutdown is confirmed."
- §2 forager-release: "...then request the forager's graceful shutdown by
  name (`message-peer`), force-terminating it if it does not exit promptly;
  its species frees only after shutdown is confirmed."

Verified by `plan_delegation.txtar`'s new `grep -count=2 'force-terminating it
if it does not exit promptly'` assertion (T3) passing — see AC-1 output above.

## AC-5

`internal/bootstrap/adapters/claude/team-loop.md`'s "Shutting down teammates"
section's "Procedure per worker" bullet now reads: "(do this for all four
teammate roles — brooder, skua, forager, incubator — at that role's teardown
moment: green teardown for a brooder/skua pair per `implementation.md` §3.2,
phase close for an incubator per `planning.md` §0, and nest-status
verification for a forager per `foraging.md` §Commissioner)", replacing the
old brooder/skua-only framing.

Verified by `init.txtar`'s new assertions (T4): `grep 'all four teammate roles
— brooder, skua, forager, incubator'` passes, and `! grep 'do this for the
brooder \*and\* its paired skua at green teardown'` (the old sentence)
confirms it no longer exists — see AC-1 output above.

## AC-6

`internal/bootstrap/adapters/claude/team-loop.md`'s "Confirmed shutdown"
definition now reads: "the teammate no longer appears in the team roster and,
if it was given a pane, that pane has closed (no-tmux/degraded sessions have
no pane to check — roster absence alone suffices there)."

Verified by `init.txtar`'s new assertions (T5): `grep 'roster absence alone
suffices there'` passes, and `! grep 'the teammate no longer appears in the
team roster and its tmux pane has closed'` (the old unconditional sentence)
confirms it no longer exists — see AC-1 output above.

## AC-7

`internal/bootstrap/adapters/claude/agents/fledge-forager.md`'s dropped bullet
is restored between the `nest stamp` bullet and the teammate-exit bullet:

> - **Gate your final message on `fledge nest status`.** Before you send it,
>   run `fledge nest status`; it must exit 0 (`complete: true`). If it reports
>   any doc still a stub or missing, or the index stale, you are not done —
>   it names exactly what remains; finish that and re-run until clean. Run it
>   on any wake to check whether you still owe synthesis: a passing verdict,
>   not "my scouts finished", is what means the nest is done.

Wording restored verbatim from the bullet dropped in a later regression
(confirmed via `git log --follow -p -- internal/bootstrap/adapters/claude/agents/fledge-forager.md`,
which shows this exact sentence present pre-regression).

Verified by `agents.txtar`'s new `grep 'Gate your final message on
\`fledge nest status\`'` assertion (T6) passing — see AC-1 output above.

## AC-8

```
go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.084s
```

All 5 modified fixtures (`init_agents.txtar`, `forager_contract.txtar`,
`plan_delegation.txtar`, `init.txtar`, `agents.txtar`) plus the full 23-file
acceptance suite pass.

## AC-9

```
$ fledge init --refresh
note: refreshed 5 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
...
updated .fledge/skills/fledge-orchestrate/foraging.md
updated .fledge/skills/fledge-orchestrate/planning.md
updated .fledge/skills/fledge-orchestrate/worker-protocols.md
updated .claude/team-loop.md
updated .fledge/scaffold.json
...
scaffolded agents: claude

$ fledge preen
spec clean: 22 plumages, 42 feathers
```

`.claude/agents/fledge-forager.md` is a symlink into
`internal/bootstrap/adapters/claude/agents/fledge-forager.md` (confirmed via
`ls -la` + `diff`, byte-identical), so its edit is reflected automatically and
did not need a separate `--refresh` "updated" line.

`git diff` on the regenerated scaffolded copies (`.fledge/skills/...`,
`.claude/team-loop.md`) shows only the intended one-line-per-section changes
matching the source edits — no unrelated drift.
