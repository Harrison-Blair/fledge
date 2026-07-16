# FTHR-059 evidence

## AC-1

New test `TestClaudeAgentDefsRepointToRoleFiles` added to
`internal/bootstrap/registry_test.go` (alongside the existing agent-definition
assertions such as `TestClaudeIncubatorWired`; placement confirmed via
`grep -rl "fledge-incubator.md" --include="*_test.go" internal/` →
`internal/bootstrap/registry_test.go`).

Pre-implementation run against the unchanged agent files (expected failure —
old worker-protocols.md phrasing still present):

```
$ go test ./internal/bootstrap -run TestClaudeAgentDefsRepointToRoleFiles
--- FAIL: TestClaudeAgentDefsRepointToRoleFiles (0.00s)
    registry_test.go:445: adapters/claude/agents/fledge-incubator.md: missing reference to .fledge/skills/fledge-orchestrate/incubator.md
    registry_test.go:448: adapters/claude/agents/fledge-incubator.md: still references worker-protocols.md
    registry_test.go:445: adapters/claude/agents/fledge-brooder.md: missing reference to .fledge/skills/fledge-orchestrate/brooder.md
    registry_test.go:448: adapters/claude/agents/fledge-brooder.md: still references worker-protocols.md
    registry_test.go:445: adapters/claude/agents/fledge-skua.md: missing reference to .fledge/skills/fledge-orchestrate/skua.md
    registry_test.go:448: adapters/claude/agents/fledge-skua.md: still references worker-protocols.md
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

Post-implementation run (after the three agent-file edits):

```
$ go test ./internal/bootstrap -run TestClaudeAgentDefsRepointToRoleFiles -v
=== RUN   TestClaudeAgentDefsRepointToRoleFiles
--- PASS: TestClaudeAgentDefsRepointToRoleFiles (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

## AC-2

Each of the three Claude adapter agent definitions now references its own
per-role file instead of a `worker-protocols.md` section, per the spec's
Approach — one-line change per file, nothing else touched:

```
$ git diff main -- internal/bootstrap/adapters/claude/agents/
diff --git a/internal/bootstrap/adapters/claude/agents/fledge-brooder.md b/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
index c4d6e9e..64b78d2 100644
--- a/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
+++ b/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
@@ -6,7 +6,7 @@ model: inherit
 
 You are a fledge brooder, a Claude Code teammate spawned by the orchestrator (your team lead). You own exactly one feather for your entire lifetime. Your spawn prompt is your entire context — you inherit no conversation history.
 
-**Read the "Brooder" section of `.fledge/skills/fledge-orchestrate/worker-protocols.md` and follow it exactly.** It defines your orient → test-first → scope-discipline → evidence → commit → handoff → fix-loop protocol, your communication rules, and your lifecycle.
+**Read `.fledge/skills/fledge-orchestrate/brooder.md` and follow it exactly.** It defines your orient → test-first → scope-discipline → evidence → commit → handoff → fix-loop protocol, your communication rules, and your lifecycle.
 
 Claude-runtime specifics:
 
diff --git a/internal/bootstrap/adapters/claude/agents/fledge-incubator.md b/internal/bootstrap/adapters/claude/agents/fledge-incubator.md
index 360a484..22e6b74 100644
--- a/internal/bootstrap/adapters/claude/agents/fledge-incubator.md
+++ b/internal/bootstrap/adapters/claude/agents/fledge-incubator.md
@@ -10,7 +10,7 @@ You are a fledge incubator: the delegated planner for a fledge-managed reposito
 Read and follow, in order:
 
 1. **`.fledge/skills/fledge-orchestrate/planning.md`** — the phase you execute (steps 1–4; step 0 is the orchestrator's side of the delegation).
-2. **`.fledge/skills/fledge-orchestrate/worker-protocols.md`, "Incubator" section** — your relay envelope (`GATE review`, `GATE decision`, `QUESTION`, `SPAWN-REQUEST`, `PHASE-CLOSE`), communication rules, drafting rules, and lifecycle.
+2. **`.fledge/skills/fledge-orchestrate/incubator.md`** — your relay envelope (`GATE review`, `GATE decision`, `QUESTION`, `SPAWN-REQUEST`, `PHASE-CLOSE`), communication rules, drafting rules, and lifecycle.
 
 Claude-runtime specifics:
 
diff --git a/internal/bootstrap/adapters/claude/agents/fledge-skua.md b/internal/bootstrap/adapters/claude/agents/fledge-skua.md
index a2cf16d..65a8990 100644
--- a/internal/bootstrap/adapters/claude/agents/fledge-skua.md
+++ b/internal/bootstrap/adapters/claude/agents/fledge-skua.md
@@ -7,7 +7,7 @@ tools: Read, Grep, Glob, Bash, SendMessage
 
 You are a fledge skua, a Claude Code teammate spawned by the orchestrator (your team lead) together with your paired brooder at feather dispatch — you share a species name. You review exactly one feather from exactly one brooder, across as many review cycles as it needs. Being idle while your brooder implements is normal — stay alive and responsive. You read code and run tests, but never modify code, never merge, and never fix anything yourself.
 
-**Read the "Skua" section of `.fledge/skills/fledge-orchestrate/worker-protocols.md` and follow it exactly.** It defines your review checks (tests pass now, tests failed first, diff vs. spec, scope/simplicity, criteria audit), your verdict rules (findings / third-rejection / pass), and your lifecycle.
+**Read `.fledge/skills/fledge-orchestrate/skua.md` and follow it exactly.** It defines your review checks (tests pass now, tests failed first, diff vs. spec, scope/simplicity, criteria audit), your verdict rules (findings / third-rejection / pass), and your lifecycle.
 
 Claude-runtime specifics:
 
```

The test's second assertion (no `worker-protocols.md` occurrence anywhere in
any of the three files) pins this repointing. The repo's own
`.claude/agents/*.md` are symlinks into these source files, so they pick the
change up with no further edits.

## AC-3

```
$ go test ./internal/bootstrap/...
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
```

Full suite also run for safety: `go test ./...` — all packages ok
(cmd/fledge, internal/bootstrap, check, ciconfig, cli, doctest, graph,
hooktest, lock, nest, repo, roster, scan, spec).

