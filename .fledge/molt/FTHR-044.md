# FTHR-044 evidence — Forager/scout exact-computation rule for reported counts

## AC-1

_Tests observed failing before implementation, passing after_

### Test added (test-first)

Added to `cmd/fledge/testdata/forager_contract.txtar` (asserts the scaffolded
`foraging.md` states the exact-computation-for-counts rule):

```
# exact-computation-for-counts rule (FTHR-044): any count reported in a scout
# report or synthesized doc must come from an exact computation run at write
# time, never estimated by eye
grep 'exact computation run at write time' .fledge/skills/fledge-orchestrate/foraging.md
```

### Pre-implementation run — FAILING (rule text not yet present)

Command:

```
go test ./cmd/fledge -run 'TestScripts/forager_contract'
```

Output (verbatim, trimmed to the failure):

```
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/forager_contract (0.00s)
            FAIL: testdata/forager_contract.txtar:37: no match for `exact computation run at write time` found in .fledge/skills/fledge-orchestrate/foraging.md
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.006s
FAIL
```

The failure is for the expected reason: the new rule text does not yet exist in
the scaffolded `foraging.md`.

### Post-implementation run — PASSING

Command:

```
go test ./cmd/fledge -run 'TestScripts/forager_contract'
```

Output:

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.006s
```

## AC-2

_foraging.md core source states the rule as a generic authoring requirement_

Added to the Scout section's Rules in the core source
`internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` (generic — no
repo-specific numbers, per PLM-023 Out of Scope):

```
- Any count, total, or enumerated size you state (e.g. "N commands," "N fixtures," "N files in module X") must come from an exact computation run at write time — a `grep -c`, a `find`/glob count, `wc -l`, or equivalent — never estimated by eye or recalled from memory. Cite or show the command that produced it so the count is re-derivable by a later reader, not merely asserted. This applies equally to counts carried into any synthesized doc.
```

Verified present in the core source:

```
$ grep -n 'exact computation run at write time' internal/bootstrap/core/skills/fledge-orchestrate/foraging.md
82:- Any count, total, or enumerated size you state (e.g. "N commands," ...
```

## AC-3

_fledge init --refresh regenerates scaffold; TestScripts passes_

Rebuilt/reinstalled the binary from worktree source, then refreshed:

```
$ go install ./cmd/fledge && hash -r && fledge version
fledge 0.5.5            # matches VERSION

$ fledge init --refresh --force    # regenerated .fledge/skills/... from embedded core

$ grep -n 'exact computation run at write time' .fledge/skills/fledge-orchestrate/foraging.md
82:- Any count, total, or enumerated size you state (e.g. "N commands," ...

$ fledge preen ; echo exit=$?
spec clean: 26 plumages, 56 feathers
exit=0

$ go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.096s

$ go test ./...   # entire suite
ok  (all packages)

$ gofmt -l .    # (no output — clean)
$ go vet ./...  # (no output — clean)
```
