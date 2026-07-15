# FTHR-040 evidence

## AC-1

Test written first: `cmd/fledge/testdata/forager_contract.txtar` (new file).

Command: `go test ./cmd/fledge -run TestScripts/forager_contract -v`

### Pre-implementation run (FAILING, expected reason)

Run against the unedited source (before any prose edits), inside the worktree:

```
        # forbidden: pipeline-stage / failure-mode leakage (0.000s)
        > ! grep 'step-4→step-5' .fledge/skills/fledge-orchestrate/planning.md
        > ! grep 'synthesis boundary' .fledge/skills/fledge-orchestrate/planning.md
        > ! grep 'half-filled nest' .fledge/skills/fledge-orchestrate/planning.md
        [.fledge/skills/fledge-orchestrate/planning.md]
        ... (full generated planning.md dumped by testscript on failure — contains
        "half-filled nest", "mid-pipeline", and the removed foraging.md pointer
        sentence, matching internal/bootstrap/core/skills/fledge-orchestrate/planning.md
        unedited) ...

        FAIL: testdata/forager_contract.txtar:12: unexpected match for `half-filled nest` found in .fledge/skills/fledge-orchestrate/planning.md: half-filled nest

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/forager_contract (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.006s
FAIL
```

Testscript stops at the first failing assertion, so this confirms the forbidden
string `half-filled nest` was present in the scaffolded (and therefore source)
`planning.md` pre-edit — the test fails for the expected reason (forbidden
pipeline-stage/failure-mode vocabulary still present), not on a setup error or
unrelated test.

### Post-implementation run (PASSING)

After the source edits to `internal/bootstrap/core/skills/fledge-orchestrate/planning.md`
and `worker-protocols.md`, reinstall (`go install ./cmd/fledge && hash -r`,
`fledge version` → `0.5.4`, matches `VERSION`):

```
        # forbidden: pipeline-stage / failure-mode leakage (0.000s)
        > ! grep 'step-4→step-5' .fledge/skills/fledge-orchestrate/planning.md
        > ! grep 'synthesis boundary' .fledge/skills/fledge-orchestrate/planning.md
        > ! grep 'half-filled nest' .fledge/skills/fledge-orchestrate/planning.md
        > ! grep 'mid-pipeline' .fledge/skills/fledge-orchestrate/planning.md
        > ! grep 'see `foraging.md` for what that expected intermediate state looks like' .fledge/skills/fledge-orchestrate/planning.md
        > ! grep 'step-4→step-5' .fledge/skills/fledge-orchestrate/worker-protocols.md
        > ! grep 'synthesis boundary' .fledge/skills/fledge-orchestrate/worker-protocols.md
        > ! grep 'half-filled nest' .fledge/skills/fledge-orchestrate/worker-protocols.md
        > ! grep 'mid-pipeline' .fledge/skills/fledge-orchestrate/worker-protocols.md
        > ! grep 'see `foraging.md` for what that expected intermediate state looks like' .fledge/skills/fledge-orchestrate/worker-protocols.md
        # required: hardened two-input framing present in both files (0.000s)
        > grep 'only\*\* signal that it is done is its explicit final message' .fledge/skills/fledge-orchestrate/planning.md
        > grep 'never an input' .fledge/skills/fledge-orchestrate/planning.md
        > grep 'only\*\* signal that it is done is its explicit final message' .fledge/skills/fledge-orchestrate/worker-protocols.md
        > grep 'never an input' .fledge/skills/fledge-orchestrate/worker-protocols.md
        PASS

--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/forager_contract (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	(cached)
```

## AC-2

Both source files state the two-input contract (final message = done; prolonged
silence = suspected stall → existing escalation) and explicitly state on-disk
`.fledge/nest/` state is never an input:

`internal/bootstrap/core/skills/fledge-orchestrate/planning.md` §2 (excerpt,
post-edit):

> Wait for it as a strict two-input state machine: the **only** signal that it
> is done is its explicit final message (the coverage summary it sends you by
> name); anything else — a bare idle notification, a "worker finished"
> notification, file changes — is not completion and is not evidence of
> anything. On-disk `.fledge/nest/` state, including a half-populated nest
> mid-synthesis, is never an input to this decision; never inspect it to judge
> progress or declare a stall.

`internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` §
Incubator "Foraging:" paragraph (excerpt, post-edit):

> Honor the same completion contract as step 2: the **only** signal that it is
> done is its explicit final message — a bare idle notification is not
> completion. On-disk `.fledge/nest/` state is never an input to this decision,
> including a half-populated nest mid-synthesis; do not inspect it as evidence
> either way.

Same "never an input" / "only** signal" strings verified present in the
*regenerated scaffold* copies (`.fledge/skills/fledge-orchestrate/planning.md`,
`.fledge/skills/fledge-orchestrate/worker-protocols.md`) by the required-string
`grep` assertions in `forager_contract.txtar`, passing per AC-1's
post-implementation run above.

## AC-3

`forager_contract.txtar`'s forbidden-string assertions (`! grep`) all pass
post-edit against both generated files — see the "Post-implementation run"
transcript under AC-1: none of `step-4→step-5`, `synthesis boundary`,
`half-filled nest`, `mid-pipeline`, or the `foraging.md` pointer sentence
appear in either `planning.md` or `worker-protocols.md`.

Manual re-read of both edited paragraphs (source files) confirms no other
forager-internal pipeline-stage or stall-failure-mode vocabulary remains: the
old text described the forager's scouts finishing before synthesis
("structurally expected *mid-pipeline*") and warned that "inspecting the
half-filled nest ... will mislead you into declaring a stall" — both replaced
with the flat statement that on-disk `.fledge/nest/` state is simply never an
input, with no description of *why* internally (no scout/synthesis internals
named as the reason).

## AC-4

Diff of the suspected-stall paragraph in `planning.md` §2 — unchanged, byte
for byte, across the edit (confirmed via `git diff`: no changed lines in that
paragraph):

> If the forager idles without ever sending its final message, treat it as a
> *suspected* stall, not a confirmed one: send it one by-name message
> (`message-peer`) asking it to continue synthesis and report when done, then
> wait ~2 minutes. Repeat this at most **three times, ~2 minutes apart**. If it
> has still sent no final message after the third query, do not decide
> unilaterally — run a `confirm-gate` (decision) surfacing the situation to the
> user: intervene (terminate the forager and respawn a fresh one, or fall back
> to inline synthesis per the no-`spawn-worker` path below) or keep waiting.
> Only the user chooses to abandon a forager; you never autonomously churn one.

`worker-protocols.md`'s "Apply the same suspected-stall procedure: ... at most
three by-name missing-output queries ~2 minutes apart ... escalate to the user
... relay a `GATE decision` (intervene or keep waiting) up through the
orchestrator." sentence is also unchanged (only the preceding sentence, about
the completion signal, was rewritten — verified via `git diff` showing only
that one line changed in the paragraph).

`git diff --stat` for the source edit:

```
 internal/bootstrap/core/skills/fledge-orchestrate/planning.md         | 4 +---
 internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md | 2 +-
```

Confirms the edits are localized to the completion-signal sentences; the
query-count/interval/cap/confirm-gate escalation mechanics were not touched.

## AC-5

`go install ./cmd/fledge && hash -r && fledge version` → `fledge 0.5.4`,
matches `VERSION` file (`0.5.4`).

`fledge init --refresh --force` run in the worktree; `git status --short`
after showed only the intended files changed:

```
 M .fledge/scaffold.json
 M .fledge/skills/fledge-orchestrate/planning.md
 M .fledge/skills/fledge-orchestrate/worker-protocols.md
 M internal/bootstrap/core/skills/fledge-orchestrate/planning.md
 M internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md
?? .fledge/molt/FTHR-040.md
?? cmd/fledge/testdata/forager_contract.txtar
```

(`.fledge/scaffold.json`'s diff is only the two updated sha256 hashes for
`planning.md` and `worker-protocols.md`.)

Full suite:

```
$ go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.088s

$ go vet ./...
(no output — clean)
```

Checked no other txtar fixture pins the removed language:

```
$ grep -rn "step-4→step-5\|synthesis boundary\|half-filled nest\|mid-pipeline\|see \`foraging.md\` for what that expected intermediate state looks like" cmd/fledge/testdata/*.txtar
cmd/fledge/testdata/forager_contract.txtar:10:...
cmd/fledge/testdata/forager_contract.txtar:11:...
cmd/fledge/testdata/forager_contract.txtar:12:...
cmd/fledge/testdata/forager_contract.txtar:13:...
cmd/fledge/testdata/forager_contract.txtar:14:...
cmd/fledge/testdata/forager_contract.txtar:16:...
cmd/fledge/testdata/forager_contract.txtar:17:...
cmd/fledge/testdata/forager_contract.txtar:18:...
cmd/fledge/testdata/forager_contract.txtar:19:...
cmd/fledge/testdata/forager_contract.txtar:20:...
```

Only `forager_contract.txtar` itself matches (its own negative-match
assertion lines) — no other fixture (`init.txtar`, `init_agents.txtar`,
`agents.txtar`) needed updating.
