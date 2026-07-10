# FTHR-015 molt evidence

## AC-1

Tests observed failing before implementation (markers absent from embedded core).

Command:
```
go test ./cmd/fledge -run TestScripts/plan_delegation -v
```

Output (pre-implementation, expected failure):
```
=== RUN   TestScripts
=== RUN   TestScripts/plan_delegation
=== PAUSE TestScripts/plan_delegation
=== CONT  TestScripts/plan_delegation
    testscript.go:584: WORK=$WORK
        ...
        # delegation marker: pins capability-conditional incubator delegation in planning.md (0.000s)
        > grep 'incubator never runs fledge new' .fledge/skills/fledge-orchestrate/planning.md
        ...
        FAIL: testdata/plan_delegation.txtar:9: no match for `incubator never runs fledge new` found in .fledge/skills/fledge-orchestrate/planning.md

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/plan_delegation (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.005s
```

Both grep assertions fail against the unchanged embedded core — the delegation marker is absent
from planning.md (test aborts before reaching the foraging.md grep). This is the expected
pre-implementation failure.

Post-implementation passing run:

Command:
```
go test ./cmd/fledge -run TestScripts/plan_delegation -v
```

Output (post-implementation, passes):
```
=== RUN   TestScripts/plan_delegation
...
        > grep 'incubator never runs .fledge new. or mutates specs' .fledge/skills/fledge-orchestrate/planning.md
        # empty-nest marker: pins the expected-intermediate-state clarification in foraging.md
        > grep 'expected intermediate after scaffolding' .fledge/skills/fledge-orchestrate/foraging.md
        PASS
--- PASS: TestScripts/plan_delegation (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.006s
```

## AC-2

`planning.md` steps 3.4 and 4.6 each contain a delegation branch:

- Step 3.4: "The body drafting is capability-conditional on `spawn-worker`: if you provide
  `spawn-worker`, delegate the body draft to an incubator worker — pass it the resolved decisions
  and pointers (prospective `PLM-###` ID, the plumage template at `templates/plumage.md`, and the
  concern docs to cite); the incubator reads the template and concern docs itself and returns the
  full body. If you do not provide `spawn-worker`, draft the body inline yourself. Either way you
  (the orchestrator) run a `confirm-gate` (review)… On "Accept", create the file with
  `fledge new plumage …`… The incubator never runs `fledge new` or mutates specs, so no un-gated
  file is ever written."

- Step 4.6: Same pattern, adapted for feathers (also passes `FTHR-###` ID, plumage link,
  `depends_on`, `oversight`, the feather template, etc.).

Both steps: confirm-gate runs with the orchestrator; `fledge new` is the orchestrator's act;
incubator is barred from spec mutation.

## AC-3

`foraging.md` step 3 now contains:

"**Important:** immediately after `fledge nest scaffold`, `.fledge/nest/` contains only empty
template stubs — placeholder concern docs, unfilled `raw/*.md`, and `index.md` frontmatter stamped
to HEAD. This empty state is the expected intermediate after scaffolding; scouts and synthesis fill
it in steps 4–6 below. It is not a failure and must not be flagged as one."

`planning.md` step 2 contains the pointer:

"Note: immediately after `fledge nest scaffold`, `.fledge/nest/` holds only empty template stubs
— see `foraging.md` for what that expected intermediate state looks like and why it is not a
failure."

## AC-4

TestCoreNeutral passes (no harness-native paths in core prose):

```
go test ./internal/bootstrap -run TestCoreNeutral -v
--- PASS: TestCoreNeutral (0.00s)
PASS
```

Full suite green:

```
go test ./... && go vet ./...
ok  github.com/Harrison-Blair/fledge/cmd/fledge           0.075s
ok  github.com/Harrison-Blair/fledge/internal/bootstrap   0.007s
ok  github.com/Harrison-Blair/fledge/internal/check       0.002s
ok  github.com/Harrison-Blair/fledge/internal/cli         0.002s
ok  github.com/Harrison-Blair/fledge/internal/graph       0.001s
ok  github.com/Harrison-Blair/fledge/internal/lock        0.001s
ok  github.com/Harrison-Blair/fledge/internal/nest        0.001s
ok  github.com/Harrison-Blair/fledge/internal/scan        0.008s
ok  github.com/Harrison-Blair/fledge/internal/spec        0.002s
```
