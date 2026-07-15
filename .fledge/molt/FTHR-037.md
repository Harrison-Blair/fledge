# FTHR-037 Evidence

## AC-1

Commands run, in order, inside the worktree
(`/home/penguin/source/fledge/.fledge/burrows/FTHR-037`):

```
$ cat VERSION
0.5.4
$ go install ./cmd/fledge && hash -r && command -v fledge && fledge version
/home/penguin/go/bin/fledge
fledge 0.5.4
```

**Failing capture — `fledge preen -strict` BEFORE `fledge init --refresh`:**

```
$ fledge preen -strict; echo "EXIT: $?"
WARN  .fledge/pluma/feathers/FTHR-038-auto-resolve-tmux-precondition.md: checked criteria missing evidence sections in /home/penguin/source/fledge/.fledge/burrows/FTHR-037/.fledge/molt/FTHR-038.md: AC-5, AC-6
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/worker-protocols.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
4 warning(s)
EXIT: 1
```

This shows `worker-protocols.md` flagged **stale (unedited, refresh-safe)** — the
expected reason: the freshly rebuilt binary embeds FTHR-036's hardened skua
prose, but the on-disk copy in this worktree still has the old content. (The
`.claude/settings.local.json` and `.fledge/nest/raw/.gitkeep` "missing"
warnings and the FTHR-038 evidence warning are pre-existing/out-of-scope —
gitignored scaffold files absent in a fresh worktree and an already-merged
feather's evidence gap, respectively — not this feather's concern.)

**Refresh:**

```
$ fledge init --refresh; echo "EXIT: $?"
note: refreshed 2 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
created .fledge/nest/raw/.gitkeep
created .claude/settings.local.json
updated .fledge/skills/fledge-orchestrate/worker-protocols.md
updated .fledge/scaffold.json
exists .fledge/broods/.gitkeep
exists .fledgeignore
exists .fledge/pluma/plumage/.gitkeep
exists .fledge/pluma/feathers/.gitkeep
exists .gitignore
exists .fledge/skills/fledge-interrogate/SKILL.md
exists .fledge/skills/fledge-orchestrate/SKILL.md
exists .fledge/skills/fledge-orchestrate/foraging.md
exists .fledge/skills/fledge-orchestrate/implementation.md
exists .fledge/skills/fledge-orchestrate/planning.md
exists .fledge/skills/fledge-orchestrate/templates/context-doc.md
exists .fledge/skills/fledge-orchestrate/templates/feather.md
exists .fledge/skills/fledge-orchestrate/templates/plumage.md
exists .fledge/skills/fledge-orchestrate/templates/scout-report.md
exists .claude/agents/fledge-brooder.md
exists .claude/agents/fledge-forager.md
exists .claude/agents/fledge-context-scout.md
exists .claude/agents/fledge-skua.md
exists .claude/agents/fledge-incubator.md
exists .claude/settings.json
exists .claude/team-loop.md
exists .claude/fledge-adapter.md
exists .claude/skills/fledge-orchestrate
exists .claude/skills/fledge-interrogate
exists CLAUDE.md
scaffolded agents: claude
EXIT: 0
```

`.claude/settings.local.json` and `.fledge/nest/raw/.gitkeep` are gitignored
(untracked) — refresh created them locally in this worktree but they do not
appear in `git status` and are not part of this feather's committed change.
No unexpected user-edit confirmation was hit for any committed file.

**Passing capture — `fledge preen -strict` AFTER refresh:**

```
$ fledge preen -strict; echo "EXIT: $?"
WARN  .fledge/pluma/feathers/FTHR-038-auto-resolve-tmux-precondition.md: checked criteria missing evidence sections in /home/penguin/source/fledge/.fledge/burrows/FTHR-037/.fledge/molt/FTHR-038.md: AC-5, AC-6
1 warning(s)
EXIT: 1
```

`worker-protocols.md` no longer appears in the warnings — it is now clean.
The single remaining warning is FTHR-038's pre-existing, already-merged,
out-of-scope evidence gap (explicitly called out as not-to-touch in this
feather's assignment).

## AC-2

```
$ go vet ./...
(no output — clean)

$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.076s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.009s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.126s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s
```

All packages pass, including `cmd/fledge`'s `TestScripts` (the
`cmd/fledge/testdata/*.txtar` acceptance scripts, run as part of that
package's test binary).

Re-verified against the actual FTHR-036 diff (not just the planning-time
grep): no fixture references `worker-protocols.md` or any of its prose:

```
$ grep -rl "worker-protocols" cmd/fledge/testdata/*.txtar
(no matches)
$ grep -rl "Red-team\|scope creep\|Scope and simplicity" cmd/fledge/testdata/*.txtar
(no matches)
```

No txtar fixture needed updating.

## AC-3

```
$ diff .fledge/skills/fledge-orchestrate/worker-protocols.md internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md && echo "IDENTICAL"
IDENTICAL
```

`.fledge/scaffold.json` diff (content hash updated by the refresh):

```
$ git diff .fledge/scaffold.json
     ".fledge/skills/fledge-orchestrate/worker-protocols.md": {
       "policy": "core",
-      "sha256": "88e70904f3ef26ba652704f78c2eb1835c68b14dc397951b0af1683b621bb287"
+      "sha256": "98d2bb2fdf71b1b47707f8b231c26f86843be88e742b089eafda3460211f9b03"
     },
```

## AC-4

```
$ git status --short
 M .fledge/scaffold.json
 M .fledge/skills/fledge-orchestrate/worker-protocols.md
```

Only the two expected files changed: the regenerated scaffold copy and the
scaffold stamp. No txtar fixture needed updating (see AC-2), and no
unrelated drift.
