# FTHR-060 evidence — Repoint txtar fixture assertions to per-role files

## AC-1

Test-first: both fixtures run against the unchanged worktree (post-FTHR-057 split, pre-this-feather edits) and FAIL for the expected reason — the grep targets in `worker-protocols.md` no longer hold the moved content (it now lives in `incubator.md`/`brooder.md`/`skua.md`).

### Pre-edit failing run: forager_contract

Command: `go test ./cmd/fledge -run TestScripts/forager_contract` (in worktree, unchanged code)

```
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/forager_contract (0.00s)
        testscript.go:584: # forager wait-contract: scaffolded planning.md and worker-protocols.md must
            # state the strict two-input state machine (final message = done; prolonged
            # silence = suspected stall) and must not leak forager-internal pipeline-stage
            # or stall-failure-mode vocabulary. See PLM-020 / FTHR-040. (0.004s)
            # forbidden: pipeline-stage / failure-mode leakage (0.000s)
            # required: hardened two-input framing present in both files (0.000s)
            > grep 'only\*\* signal that it is done is its explicit final message' .fledge/skills/fledge-orchestrate/planning.md
            > grep 'never an input' .fledge/skills/fledge-orchestrate/planning.md
            > grep 'only\*\* signal that it is done is its explicit final message' .fledge/skills/fledge-orchestrate/worker-protocols.md
            [.fledge/skills/fledge-orchestrate/worker-protocols.md]
            # Worker protocols
            
            The delegated worker roles, agent-neutral: the planning incubator, and the team-loop (Tier C) brooder and skua. These are spawned workers: a spawn prompt is a worker's entire context (it inherits no conversation history) and must be fully self-contained. A `spawn-worker` is fresh, named, addressable, killable, may idle, and returns one final message.
            
            A worker's spawn prompt tells it which protocol file to follow (incubator, brooder, or skua), its name, the orchestrator's name (the harness-assigned name the orchestrator supplies — address the orchestrator by exactly that name; e.g. on Claude Code it is `team-lead`), and its role-specific fields — for brooders and skuas: feather ID, worktree/branch, evidence-file path, and the paired counterpart's name (same species); for the incubator: the user's feature request verbatim.
            
            Each protocol lives in its own file:
            
            - `incubator.md` — the delegated planner: owns the planning phase end to end; relay envelope, communication rules, drafting, lifecycle.
            - `brooder.md` — the feather implementer: test-first protocol, scope discipline, evidence, handoff and fix loop, lifecycle.
            - `skua.md` — the paired reviewer: review checks, criteria audit, verdict rules, lifecycle.
            
            FAIL: testdata/forager_contract.txtar:26: no match for `only\*\* signal that it is done is its explicit final message` found in .fledge/skills/fledge-orchestrate/worker-protocols.md
            
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.007s
FAIL
```

### Pre-edit failing run: init_agents

Command: `go test ./cmd/fledge -run TestScripts/init_agents` (in worktree, unchanged code)

```
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/init_agents (0.05s)
        testscript.go:584: # multi-agent init: --list-agents, auto-detect, --agent, --refresh,
            # duplicate guard, --json agents field
            # --list-agents enumerates adapters and derived tiers; works outside a repo (0.001s)
            # auto-detect: a .pi/ marker scaffolds only the pi adapter (0.003s)
            # auto-detect: both markers scaffold both adapters (0.004s)
            # --agent overrides detection; re-running with another agent is additive (0.005s)
            # the AGENTS.md pointer line is appended once, never duplicated (0.002s)
            # the Claude CLAUDE.md pointer: created when absent, exact wording, idempotent (0.003s)
            # exact pointer line, differing from the Codex line only in the adapter path (0.000s)
            # re-running appends the line once, never duplicated (0.002s)
            # an existing CLAUDE.md is preserved; the pointer is appended additively (0.003s)
            # unknown agent → usage error pointing at --list-agents (0.002s)
            # --refresh: user-edited files make a non-interactive refresh refuse (exit 1,
            # listing the files); --refresh --force resets them to the shipped version
            # (with a stderr note); a second refresh with no edits is silent. (0.013s)
            # duplicate guard: a same-name skill in a harness-native skills dir refuses init (0.003s)
            # --json reports which adapters were scaffolded (0.003s)
            # incubator force-terminate backstop (FTHR-041 AC-2): worker-protocols.md's
            # Incubator Lifecycle section mirrors the existing Brooder/Skua Lifecycle
            # force-terminate sentence (pin the exact phrase 3x, not just the word, which
            # already appears twice for Brooder/Skua) (0.003s)
            > cd $WORK
            $WORK
            > mkdir ftbackstop-repo
            > cd ftbackstop-repo
            > exec git init -q .
            > exec fledge init
            [stdout]
            created .fledge/nest/raw/.gitkeep
            created .fledge/broods/.gitkeep
            created .fledgeignore
            created .fledge/pluma/plumage/.gitkeep
            created .fledge/pluma/feathers/.gitkeep
            created .gitignore
            created .fledge/skills/fledge-interrogate/SKILL.md
            created .fledge/skills/fledge-orchestrate/SKILL.md
            created .fledge/skills/fledge-orchestrate/brooder.md
            created .fledge/skills/fledge-orchestrate/foraging.md
            created .fledge/skills/fledge-orchestrate/implementation.md
            created .fledge/skills/fledge-orchestrate/incubator.md
            created .fledge/skills/fledge-orchestrate/planning.md
            created .fledge/skills/fledge-orchestrate/skua.md
            created .fledge/skills/fledge-orchestrate/templates/context-doc.md
            created .fledge/skills/fledge-orchestrate/templates/feather.md
            created .fledge/skills/fledge-orchestrate/templates/plumage.md
            created .fledge/skills/fledge-orchestrate/templates/scout-report.md
            created .fledge/skills/fledge-orchestrate/worker-protocols.md
            created .claude/agents/fledge-brooder.md
            created .claude/agents/fledge-forager.md
            created .claude/agents/fledge-context-scout.md
            created .claude/agents/fledge-skua.md
            created .claude/agents/fledge-incubator.md
            created .claude/settings.json
            created .claude/settings.local.json
            created .claude/team-loop.md
            created .claude/fledge-adapter.md
            created .claude/skills/fledge-orchestrate
            created .claude/skills/fledge-interrogate
            created CLAUDE.md
            created .fledge/scaffold.json
            scaffolded agents: claude
            [stderr]
            note: no agent harness detected; scaffolded the claude adapter by default. Run `fledge init --agent <name>` to add another (see `fledge init --list-agents`).
            > grep -count=3 'force-terminate you if you do not exit promptly, since acknowledging a shutdown request is not the same as ending your session' .fledge/skills/fledge-orchestrate/worker-protocols.md
            [.fledge/skills/fledge-orchestrate/worker-protocols.md]
            # Worker protocols
            
            The delegated worker roles, agent-neutral: the planning incubator, and the team-loop (Tier C) brooder and skua. These are spawned workers: a spawn prompt is a worker's entire context (it inherits no conversation history) and must be fully self-contained. A `spawn-worker` is fresh, named, addressable, killable, may idle, and returns one final message.
            
            A worker's spawn prompt tells it which protocol file to follow (incubator, brooder, or skua), its name, the orchestrator's name (the harness-assigned name the orchestrator supplies — address the orchestrator by exactly that name; e.g. on Claude Code it is `team-lead`), and its role-specific fields — for brooders and skuas: feather ID, worktree/branch, evidence-file path, and the paired counterpart's name (same species); for the incubator: the user's feature request verbatim.
            
            Each protocol lives in its own file:
            
            - `incubator.md` — the delegated planner: owns the planning phase end to end; relay envelope, communication rules, drafting, lifecycle.
            - `brooder.md` — the feather implementer: test-first protocol, scope discipline, evidence, handoff and fix loop, lifecycle.
            - `skua.md` — the paired reviewer: review checks, criteria audit, verdict rules, lifecycle.
            
            FAIL: testdata/init_agents.txtar:160: no match for `force-terminate you if you do not exit promptly, since acknowledging a shutdown request is not the same as ending your session` found in .fledge/skills/fledge-orchestrate/worker-protocols.md
            
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.050s
FAIL
```

Both failures are exactly the expected ones: the assertions still target `worker-protocols.md`, which post-split is a pointer stub without the moved prose.

### Post-edit passing runs

Commands: `go test ./cmd/fledge -run TestScripts/forager_contract && go test ./cmd/fledge -run TestScripts/init_agents` (in worktree, after editing the two txtar files)

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.006s
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.048s
```

## AC-2

`forager_contract.txtar`'s forbidden/required-phrase blocks now target `incubator.md`; the `planning.md` and `foraging.md` blocks are byte-unchanged. Diff of the file (command: `git diff cmd/fledge/testdata/forager_contract.txtar`):

```
diff --git a/cmd/fledge/testdata/forager_contract.txtar b/cmd/fledge/testdata/forager_contract.txtar
index 0e41101..1801f37 100644
--- a/cmd/fledge/testdata/forager_contract.txtar
+++ b/cmd/fledge/testdata/forager_contract.txtar
@@ -1,4 +1,4 @@
-# forager wait-contract: scaffolded planning.md and worker-protocols.md must
+# forager wait-contract: scaffolded planning.md and incubator.md must
 # state the strict two-input state machine (final message = done; prolonged
 # silence = suspected stall) and must not leak forager-internal pipeline-stage
 # or stall-failure-mode vocabulary. See PLM-020 / FTHR-040.
@@ -13,18 +13,18 @@ exec fledge init
 ! grep 'mid-pipeline' .fledge/skills/fledge-orchestrate/planning.md
 ! grep 'see `foraging.md` for what that expected intermediate state looks like' .fledge/skills/fledge-orchestrate/planning.md
 
-! grep 'step-4→step-5' .fledge/skills/fledge-orchestrate/worker-protocols.md
-! grep 'synthesis boundary' .fledge/skills/fledge-orchestrate/worker-protocols.md
-! grep 'half-filled nest' .fledge/skills/fledge-orchestrate/worker-protocols.md
-! grep 'mid-pipeline' .fledge/skills/fledge-orchestrate/worker-protocols.md
-! grep 'see `foraging.md` for what that expected intermediate state looks like' .fledge/skills/fledge-orchestrate/worker-protocols.md
+! grep 'step-4→step-5' .fledge/skills/fledge-orchestrate/incubator.md
+! grep 'synthesis boundary' .fledge/skills/fledge-orchestrate/incubator.md
+! grep 'half-filled nest' .fledge/skills/fledge-orchestrate/incubator.md
+! grep 'mid-pipeline' .fledge/skills/fledge-orchestrate/incubator.md
+! grep 'see `foraging.md` for what that expected intermediate state looks like' .fledge/skills/fledge-orchestrate/incubator.md
 
 # required: hardened two-input framing present in both files
 grep 'only\*\* signal that it is done is its explicit final message' .fledge/skills/fledge-orchestrate/planning.md
 grep 'never an input' .fledge/skills/fledge-orchestrate/planning.md
 
-grep 'only\*\* signal that it is done is its explicit final message' .fledge/skills/fledge-orchestrate/worker-protocols.md
-grep 'never an input' .fledge/skills/fledge-orchestrate/worker-protocols.md
+grep 'only\*\* signal that it is done is its explicit final message' .fledge/skills/fledge-orchestrate/incubator.md
+grep 'never an input' .fledge/skills/fledge-orchestrate/incubator.md
 
 # forager force-terminate backstop (FTHR-041 AC-3, tightened by FTHR-056): both
 # the Forager Lifecycle section and the Commissioner's "verify and release" step
```

Note: the file's own header comment (line 1) also named `worker-protocols.md`; it was updated to `incubator.md` so the comment matches the assertions it describes — no other lines touched. `grep -n 'foraging.md' cmd/fledge/testdata/forager_contract.txtar` confirms the foraging.md block (lines 34–39, 44) is intact, and the diff shows no hunk touching it or the planning.md blocks.

## AC-3

`init_agents.txtar`'s single `grep -count=3 ... worker-protocols.md` assertion is replaced by three `grep -count=1` lines, one per role file, and the comment above it now describes three files. Diff (command: `git diff cmd/fledge/testdata/init_agents.txtar`):

```
diff --git a/cmd/fledge/testdata/init_agents.txtar b/cmd/fledge/testdata/init_agents.txtar
index 2dcd296..635c667 100644
--- a/cmd/fledge/testdata/init_agents.txtar
+++ b/cmd/fledge/testdata/init_agents.txtar
@@ -148,16 +148,18 @@ stdout '"agents": \['
 stdout '"pi"'
 ! stdout '"claude"'
 
-# incubator force-terminate backstop (FTHR-041 AC-2): worker-protocols.md's
-# Incubator Lifecycle section mirrors the existing Brooder/Skua Lifecycle
-# force-terminate sentence (pin the exact phrase 3x, not just the word, which
-# already appears twice for Brooder/Skua)
+# incubator force-terminate backstop (FTHR-041 AC-2): each per-role protocol
+# file's Lifecycle section carries the same force-terminate sentence — pin the
+# exact phrase once in each of incubator.md, brooder.md, and skua.md (three
+# files instead of one combined file's three subsections)
 cd $WORK
 mkdir ftbackstop-repo
 cd ftbackstop-repo
 exec git init -q .
 exec fledge init
-grep -count=3 'force-terminate you if you do not exit promptly, since acknowledging a shutdown request is not the same as ending your session' .fledge/skills/fledge-orchestrate/worker-protocols.md
+grep -count=1 'force-terminate you if you do not exit promptly, since acknowledging a shutdown request is not the same as ending your session' .fledge/skills/fledge-orchestrate/incubator.md
+grep -count=1 'force-terminate you if you do not exit promptly, since acknowledging a shutdown request is not the same as ending your session' .fledge/skills/fledge-orchestrate/brooder.md
+grep -count=1 'force-terminate you if you do not exit promptly, since acknowledging a shutdown request is not the same as ending your session' .fledge/skills/fledge-orchestrate/skua.md
```

Sanity check that the phrase really exists once per file in the embedded source (command: `grep -c 'force-terminate you if you do not exit promptly, since acknowledging a shutdown request is not the same as ending your session' internal/bootstrap/core/skills/fledge-orchestrate/{incubator,brooder,skua}.md`):

```
internal/bootstrap/core/skills/fledge-orchestrate/skua.md:1
internal/bootstrap/core/skills/fledge-orchestrate/brooder.md:1
internal/bootstrap/core/skills/fledge-orchestrate/incubator.md:1
```

## AC-4

Full txtar suite, uncached (command: `go test -count=1 ./cmd/fledge -run TestScripts`, in worktree, after edits):

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.103s
```

Only the two fixtures were changed (command: `git diff --stat`):

```
 cmd/fledge/testdata/forager_contract.txtar | 16 ++++++++--------
 cmd/fledge/testdata/init_agents.txtar      | 12 +++++++-----
 2 files changed, 15 insertions(+), 13 deletions(-)
```

