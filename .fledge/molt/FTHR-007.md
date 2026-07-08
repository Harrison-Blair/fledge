# FTHR-007 Evidence

Feather: migrate foraging protocol and forager/scout prose to fledge nest

## AC-1

Test-first: added `grep 'fledge nest scaffold'` and `grep 'fledge nest scout --module'` assertions to `init.txtar` and `init_agents.txtar`, then ran the suite **before** editing any prose to confirm they fail for the expected reason (verbs absent from embedded agent files).

**Command (pre-implementation):**
```
go test ./cmd/fledge -run TestScripts/init -v
```

**Failing output (pre-implementation):**
```
=== RUN   TestScripts/init
=== RUN   TestScripts/init_agents
--- FAIL: TestScripts/init ---
> grep 'fledge nest scaffold' .claude/agents/fledge-forager.md
FAIL: testdata/init.txtar: no match for `fledge nest scaffold` found in .claude/agents/fledge-forager.md

--- FAIL: TestScripts/init_agents ---
> grep 'fledge nest scaffold' .claude/agents/fledge-forager.md
FAIL: testdata/init_agents.txtar:71: no match for `fledge nest scaffold` found in .claude/agents/fledge-forager.md

FAIL    github.com/Harrison-Blair/fledge/cmd/fledge    0.022s
```

**Command (post-implementation):**
```
go test ./cmd/fledge -run TestScripts
```

**Passing output:**
```
ok      github.com/Harrison-Blair/fledge/cmd/fledge     0.049s
```

## AC-2

`foraging.md` and the two Claude agent files updated to use fledge nest verbs:

**Command:**
```
grep -n 'fledge nest' internal/bootstrap/core/skills/fledge-orchestrate/foraging.md
grep -n 'fledge nest' internal/bootstrap/adapters/claude/agents/fledge-forager.md
grep -n 'fledge nest' internal/bootstrap/adapters/claude/agents/fledge-context-scout.md
```

**Output:**
```
foraging.md:17: Run `fledge nest scaffold` from the repo root.
foraging.md:18: run `fledge nest scout --module <module>` to create the report file
foraging.md:37: `fledge nest scaffold` stamps every file; `fledge nest stamp <file>` refreshes
foraging.md:55: Run `fledge nest scout --module <module>` to create .fledge/nest/raw/<module>.md
foraging.md:57: Frontmatter is stamped by `fledge nest scout`; refresh with `fledge nest stamp <file>`

fledge-forager.md:15: run `fledge nest scaffold` to clear and recreate `.fledge/nest/`
fledge-forager.md:16: run `fledge nest scout --module <module>` to create the report file
fledge-forager.md:18: `fledge nest stamp <file>` if needed

fledge-context-scout.md:14: Run `fledge nest scout --module <module>` to create `.fledge/nest/raw/<module>.md`
```

No hand-writing frontmatter or hand-deleting the nest remains in any of these files.

## AC-3

`context-doc.md` and `templates/scout-report.md` frontmatter YAML blocks removed; replaced with CLI pointers.

**Command:**
```
grep -n 'generated:' internal/bootstrap/core/skills/fledge-orchestrate/templates/context-doc.md
grep -n 'generated:' internal/bootstrap/core/skills/fledge-orchestrate/templates/scout-report.md
```

**Output:** (empty — no YAML fields remain)

Both files now point to the CLI as the single source of the schema.

## AC-4

`TestCoreNeutral`, `TestCorePrimitivesReferenced`, full test suite, and vet all pass.

**Command:**
```
go test ./internal/bootstrap -run "TestCoreNeutral|TestCorePrimitivesReferenced" -v
go test ./... && go vet ./...
```

**Output:**
```
=== RUN   TestCorePrimitivesReferenced
--- PASS: TestCorePrimitivesReferenced (0.00s)
=== RUN   TestCoreNeutral
--- PASS: TestCoreNeutral (0.00s)

ok      github.com/Harrison-Blair/fledge/cmd/fledge
ok      github.com/Harrison-Blair/fledge/internal/bootstrap
ok      github.com/Harrison-Blair/fledge/internal/check
ok      github.com/Harrison-Blair/fledge/internal/cli
ok      github.com/Harrison-Blair/fledge/internal/graph
ok      github.com/Harrison-Blair/fledge/internal/lock
ok      github.com/Harrison-Blair/fledge/internal/nest
ok      github.com/Harrison-Blair/fledge/internal/scan
ok      github.com/Harrison-Blair/fledge/internal/spec
```

`fledge init --refresh` in the worktree updated only the three expected `.fledge/skills/` files (core copies):
- `.fledge/skills/fledge-orchestrate/foraging.md` — updated
- `.fledge/skills/fledge-orchestrate/templates/context-doc.md` — updated
- `.fledge/skills/fledge-orchestrate/templates/scout-report.md` — updated

The `.claude/agents/` files in this repo are symlinks to the adapter source, so they automatically reflect the updated source bytes (no separate regeneration step needed).

---

### Post-rebase revalidation (2026-07-07)

Main advanced with `38e483e` (skua redesign) and `c4b34ec` (version bump) before FTHR-007 merged. FTHR-007 rebased cleanly onto `c4b34ec` — no conflicts, no file overlap.

**`.claude/agents` sync check:**
```
ls -la .claude/agents/
diff .claude/agents/fledge-forager.md internal/bootstrap/adapters/claude/agents/fledge-forager.md
diff .claude/agents/fledge-context-scout.md internal/bootstrap/adapters/claude/agents/fledge-context-scout.md
```
Both `.claude/agents/fledge-forager.md` and `.claude/agents/fledge-context-scout.md` are **symlinks** into `internal/bootstrap/adapters/claude/agents/`, so they automatically reflect the updated source bytes. Both `diff` commands returned no output (byte-for-byte match).

**Post-rebase full suite:**
```
go test ./... && go vet ./...
```
```
ok      github.com/Harrison-Blair/fledge/cmd/fledge     0.056s
ok      github.com/Harrison-Blair/fledge/internal/bootstrap     0.005s
ok      github.com/Harrison-Blair/fledge/internal/check (cached)
ok      github.com/Harrison-Blair/fledge/internal/cli   0.002s
ok      github.com/Harrison-Blair/fledge/internal/graph (cached)
ok      github.com/Harrison-Blair/fledge/internal/lock  (cached)
ok      github.com/Harrison-Blair/fledge/internal/nest  (cached)
ok      github.com/Harrison-Blair/fledge/internal/scan  0.008s
ok      github.com/Harrison-Blair/fledge/internal/spec  (cached)
```
`go vet ./...` — no output (clean). No txtar fixture updates were needed; the redesign touched different files (worker-protocols.md, implementation.md, brooder/skua agents, primitives.go) with no overlap with FTHR-007's foraging.md/forager/scout/templates changes.
