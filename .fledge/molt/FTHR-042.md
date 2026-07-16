# FTHR-042 evidence

## AC-1

Tests T1–T4 were added as new `grep`/`! grep` assertions appended to
`cmd/fledge/testdata/init.txtar` (following the existing team-loop.md
assertion block added by FTHR-041), against the *unedited* scaffolded
`.claude/team-loop.md` (this repo's own scaffold copy, pre-fix, matching what
`internal/bootstrap/adapters/claude/team-loop.md` currently emits):

```
grep 'must be the complete `fledge-<role>-<species>` string' .claude/team-loop.md
grep 'name: "fledge-brooder-adelie"' .claude/team-loop.md
grep 'never just `name: "adelie"`' .claude/team-loop.md
! grep 'named per the penguin-species scheme in `implementation.md` §3.1.' .claude/team-loop.md
grep 'already pass the complete `fledge-incubator-<species>`/`fledge-forager-<species>` string' .claude/team-loop.md
grep 'consistent with the binding in' .claude/team-loop.md
grep 'appears in the team roster under that exact full name before proceeding' .claude/team-loop.md
```

Ran (in this worktree, before any prose edit):

```
$ go test ./cmd/fledge -run 'TestScripts/init$' -v
```

Captured verbatim failure (testscript halts at the first failing line in a
script, so the trace shows the first new assertion failing for the expected
reason — the new binding sentence is simply absent from the unedited file):

```
FAIL: testdata/init.txtar:73: no match for "must be the complete `fledge-<role>-<species>` string" found in .claude/team-loop.md

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/init (0.01s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.012s
FAIL
```

To confirm every one of T1–T4's *individual* new assertions (not just the
first) fails/passes for the expected reason pre-edit — since testscript's
halt-on-first-failure hides the rest — each pattern was grepped directly
against the unedited `.claude/team-loop.md` outside testscript:

```
$ for pat in \
  'must be the complete `fledge-<role>-<species>` string' \
  'name: "fledge-brooder-adelie"' \
  'never just `name: "adelie"`' \
  'already pass the complete `fledge-incubator-<species>`/`fledge-forager-<species>` string' \
  'consistent with the binding in' \
  'appears in the team roster under that exact full name before proceeding' \
  ; do echo "=== POSITIVE CHECK: $pat ==="; grep -F -- "$pat" .claude/team-loop.md && echo FOUND || echo "NOT FOUND (expected pre-edit)"; done
echo "=== NEGATIVE CHECK (should be found pre-edit): named per the penguin-species scheme ==="
grep -F 'named per the penguin-species scheme in `implementation.md` §3.1.' .claude/team-loop.md && echo FOUND
```

Output:

```
=== POSITIVE CHECK: must be the complete `fledge-<role>-<species>` string ===
NOT FOUND (expected pre-edit)
=== POSITIVE CHECK: name: "fledge-brooder-adelie" ===
NOT FOUND (expected pre-edit)
=== POSITIVE CHECK: never just `name: "adelie"` ===
NOT FOUND (expected pre-edit)
=== POSITIVE CHECK: already pass the complete `fledge-incubator-<species>`/`fledge-forager-<species>` string ===
NOT FOUND (expected pre-edit)
=== POSITIVE CHECK: consistent with the binding in ===
NOT FOUND (expected pre-edit)
=== POSITIVE CHECK: appears in the team roster under that exact full name before proceeding ===
NOT FOUND (expected pre-edit)
=== NEGATIVE CHECK (should be found pre-edit): named per the penguin-species scheme ===
- Spawn a teammate of a given agent type (e.g. `fledge-brooder`) named per the penguin-species scheme in `implementation.md` §3.1. The teammate's agent definition (`.claude/agents/fledge-<role>.md`) is its system prompt; the spawn prompt you pass is its task context. Both are the teammate's entire context — it inherits no conversation history.
FOUND
```

Every T1 (name-argument binding), T2 (old phrasing still present, confirming
the `! grep` would currently fail), T3 (planning-delegation cross-reference),
and T4 (post-spawn roster self-check) assertion fails for the expected
reason: the new text is simply not yet in the file / the old text is still
there.

(Post-implementation passing run recorded below under AC-5.)

## AC-2

`internal/bootstrap/adapters/claude/team-loop.md`'s "Spawning and addressing
teammates" section now states the full-string requirement explicitly, with a
correct/incorrect example, replacing the old ambiguous "named per the
penguin-species scheme" phrasing. Diff:

```
$ git diff -- internal/bootstrap/adapters/claude/team-loop.md
```

(see committed diff; first bullet under "## Spawning and addressing
teammates" rewritten to read:)

```
- Spawn a teammate of a given agent type (e.g. `fledge-brooder`) — the spawn tool's `name` argument must be the complete `fledge-<role>-<species>` string, e.g. `name: "fledge-brooder-adelie"`, never just `name: "adelie"`. The species scheme in `implementation.md` §3.1 governs which species token to append; the role prefix is fixed by which kind of worker you are spawning. The teammate's agent definition (`.claude/agents/fledge-<role>.md`) is its system prompt; the spawn prompt you pass is its task context. Both are the teammate's entire context — it inherits no conversation history.
```

Verified with:

```
$ grep -F 'must be the complete `fledge-<role>-<species>` string' .claude/team-loop.md
$ grep -F 'name: "fledge-brooder-adelie"' .claude/team-loop.md
$ grep -F 'never just `name: "adelie"`' .claude/team-loop.md
$ grep -F 'named per the penguin-species scheme in `implementation.md` §3.1.' .claude/team-loop.md
# (no output — confirms removal)
```

All three positive greps match; the old phrase grep returns nothing.

## AC-3

The "Planning delegation" section's incubator/forager spawn callouts already
used the full `fledge-incubator-<species>`/`fledge-forager-<species>` form
(unchanged prose), and now carry an explicit one-line cross-reference to the
"Spawning and addressing teammates" binding, added as a new bullet:

```
- Both callouts above already pass the complete `fledge-incubator-<species>`/`fledge-forager-<species>` string as the `name` argument, consistent with the binding in "Spawning and addressing teammates" above.
```

Verified with:

```
$ grep -F 'already pass the complete `fledge-incubator-<species>`/`fledge-forager-<species>` string' .claude/team-loop.md
$ grep -F 'consistent with the binding in' .claude/team-loop.md
```

Both match.

## AC-4

A new bullet states the post-spawn roster self-check, immediately after the
rewritten first bullet in "Spawning and addressing teammates":

```
- After the spawn call returns, confirm the teammate appears in the team roster under that exact full name before proceeding — a cheap self-check that catches a dropped role prefix immediately, rather than downstream in a failed `SendMessage` or a mis-named task-list entry.
```

Verified with:

```
$ grep -F 'appears in the team roster under that exact full name before proceeding' .claude/team-loop.md
```

Matches.

## AC-5

Post-implementation, ran the new-assertion script alone, then the full CLI
acceptance suite, then the whole repo's test suite:

```
$ go test ./cmd/fledge -run 'TestScripts/init$' -v
... (all init.txtar lines shown passing, including the 7 new T1-T4 lines)
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/init (0.01s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.013s

$ go test ./cmd/fledge -run TestScripts -v 2>&1 | tail -25
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/plan_delegation (0.01s)
    --- PASS: TestScripts/scan (0.02s)
    --- PASS: TestScripts/ready (0.02s)
    --- PASS: TestScripts/forager_contract (0.01s)
    --- PASS: TestScripts/agents (0.03s)
    --- PASS: TestScripts/init (0.03s)
    --- PASS: TestScripts/stamp_warning (0.03s)
    --- PASS: TestScripts/graph (0.02s)
    --- PASS: TestScripts/new (0.04s)
    --- PASS: TestScripts/unfledged (0.05s)
    --- PASS: TestScripts/report (0.05s)
    --- PASS: TestScripts/set (0.05s)
    --- PASS: TestScripts/status (0.05s)
    --- PASS: TestScripts/nest_status (0.05s)
    --- PASS: TestScripts/check (0.03s)
    --- PASS: TestScripts/criteria (0.04s)
    --- PASS: TestScripts/refresh_scaffold (0.04s)
    --- PASS: TestScripts/nest (0.06s)
    --- PASS: TestScripts/e2e (0.06s)
    --- PASS: TestScripts/preen_scaffold (0.08s)
    --- PASS: TestScripts/lock (0.07s)
    --- PASS: TestScripts/init_agents (0.07s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.088s

$ go test ./... 2>&1
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.083s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.010s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.128s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.008s
```

All green.

## AC-6

```
$ go install ./cmd/fledge && hash -r && command -v fledge && fledge version
/home/penguin/go/bin/fledge
fledge 0.5.5

$ fledge init --refresh --force
note: refreshed 2 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
...
updated .claude/team-loop.md
updated .fledge/scaffold.json
...
scaffolded agents: claude

$ git status --short
 M .claude/team-loop.md
 M .fledge/scaffold.json
 M cmd/fledge/testdata/init.txtar
 M internal/bootstrap/adapters/claude/team-loop.md
?? .fledge/molt/FTHR-042.md

$ git diff -- .claude/team-loop.md .fledge/scaffold.json
# .claude/team-loop.md diff matches the internal/bootstrap source edit
# exactly (Spawning-and-addressing bullet rewrite + roster self-check
# bullet + Planning-delegation cross-reference bullet); scaffold.json's
# only change is the updated sha256 for .claude/team-loop.md.

$ fledge preen
spec clean: 22 plumages, 42 feathers
```

`fledge preen` passes clean; `git status` shows only the four scoped files
changed plus the new evidence file.
