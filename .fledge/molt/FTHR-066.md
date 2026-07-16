# FTHR-066 Evidence — Rebuild, refresh scaffold, and verify scratchpad batching

All commands run inside the worktree `.fledge/burrows/FTHR-066` (branch
`feather/FTHR-066-batching-refresh`, branched from main `a306546`, which
includes dependency feathers FTHR-062..065). The `fledge` on PATH is
`/home/penguin/go/bin/fledge`, reinstalled from this worktree's source.

## AC-1

`fledge version` matches `VERSION` after reinstall.

**Failing pre-state** (test-first framing): the previously installed binary
predated the recent feathers — it reported the FTHR-061 split's *new* files
(`brooder.md`, `skua.md`) as "obsolete (no longer shipped)" and widespread
drift, i.e. its embedded scaffold content was stale relative to source:

```
$ fledge version            # pre-reinstall
fledge 0.5.8
$ fledge preen              # pre-reinstall (excerpt of 15 warnings)
WARN  .claude/agents/fledge-brooder.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .claude/agents/fledge-incubator.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .claude/agents/fledge-skua.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .claude/team-loop.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
WARN  .fledge/skills/fledge-interrogate/SKILL.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/brooder.md: scaffold file is obsolete (no longer shipped) — run fledge init --refresh to prune
WARN  .fledge/skills/fledge-orchestrate/foraging.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/implementation.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/incubator.md: scaffold file is obsolete (no longer shipped) — run fledge init --refresh to prune
WARN  .fledge/skills/fledge-orchestrate/planning.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/skua.md: scaffold file is obsolete (no longer shipped) — run fledge init --refresh to prune
WARN  .fledge/skills/fledge-orchestrate/worker-protocols.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
15 warning(s)
exit=0
```

(One further pre-existing WARN about `.fledge/molt/FTHR-061.md` heading form
is unrelated to the scaffold; see AC-3.)

**Reinstall and verify** (per CLAUDE.md convention, run from the worktree):

```
$ go install ./cmd/fledge && hash -r
$ command -v fledge
/home/penguin/go/bin/fledge
$ fledge version
fledge 0.5.8
$ cat VERSION
0.5.8
```

`fledge version` (0.5.8) matches `VERSION` (0.5.8). ✔

**Post-reinstall, pre-refresh drift** — the freshly built binary reports the
true stale state, including `planning.md` (FTHR-065's scratchpad-batching
clauses landed *after* FTHR-061's refresh, so the scaffolded copy lagged
source):

```
$ fledge preen              # post-reinstall, pre-refresh
WARN  .fledge/pluma/feathers/FTHR-061-refresh-scaffold-and-verify-worker-protocols-split.md: checked criteria missing evidence sections in /home/penguin/source/fledge/.fledge/burrows/FTHR-066/.fledge/molt/FTHR-061.md: AC-1, AC-2, AC-3, AC-4, AC-5 (heading must be the bare form "## AC-N", not "## AC-N: <label>")
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/planning.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
4 warning(s)
exit=0
```

## AC-2

`fledge init --refresh` completes with only the expected file set touched.
No confirmation prompt was needed (all drifted files were unedited /
refresh-safe), so no `--force` was required.

```
$ fledge init --refresh
note: refreshed 2 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
created .fledge/nest/raw/.gitkeep
created .claude/settings.local.json
updated .fledge/skills/fledge-orchestrate/planning.md
updated .fledge/scaffold.json
exists .fledge/broods/.gitkeep
exists .fledgeignore
exists .fledge/pluma/plumage/.gitkeep
exists .fledge/pluma/feathers/.gitkeep
exists .gitignore
exists .fledge/skills/fledge-interrogate/SKILL.md
exists .fledge/skills/fledge-orchestrate/SKILL.md
exists .fledge/skills/fledge-orchestrate/brooder.md
exists .fledge/skills/fledge-orchestrate/foraging.md
exists .fledge/skills/fledge-orchestrate/implementation.md
exists .fledge/skills/fledge-orchestrate/incubator.md
exists .fledge/skills/fledge-orchestrate/skua.md
exists .fledge/skills/fledge-orchestrate/templates/context-doc.md
exists .fledge/skills/fledge-orchestrate/templates/feather.md
exists .fledge/skills/fledge-orchestrate/templates/plumage.md
exists .fledge/skills/fledge-orchestrate/templates/scout-report.md
exists .fledge/skills/fledge-orchestrate/worker-protocols.md
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
exit=0
$ git status --short
 M .fledge/scaffold.json
 M .fledge/skills/fledge-orchestrate/planning.md
```

Only two tracked files changed: the `.fledge/scaffold.json` stamp and the
regenerated `planning.md`. The two `created` files are gitignored/untracked
by design (`.claude/settings.local.json` is in the global gitignore;
`.fledge/nest/raw/` is ignored):

```
$ git status --porcelain --ignored .claude/settings.local.json .fledge/nest/raw/.gitkeep
!! .claude/settings.local.json
!! .fledge/nest/raw/
```

**Smaller-than-predicted diff, documented:** the spec's Affected Modules
also predicted `incubator.md`, `.fledge/skills/fledge-interrogate/SKILL.md`,
and a `.gitignore` change. FTHR-061's refresh (merged at the branch point,
`a306546`) had already brought `incubator.md` and the interrogate `SKILL.md`
current (`exists` above), and `.gitignore` already contains the
`.fledge/scratch/` entry from PLM-027's cycle:

```
$ grep -n "scratch" .gitignore
16:.fledge/scratch/
```

The `planning.md` diff is exactly FTHR-065's two scratchpad-batching
clauses (step 3 and step 5.1 of the planning workflow):

```
$ git diff .fledge/skills/fledge-orchestrate/planning.md
-3. Author plumages **one at a time**. ... every decision put to the user. Walk the branches: ...
+3. Author plumages **one at a time**. ... every decision put to the user — though when the phase is delegated, independent resolvable questions may travel as a scratchpad batch per `incubator.md`'s scratchpad-batching rule. Walk the branches: ...
-1. For the current plumage, continue interrogating — still one question at a time — over the decomposition: ...
+1. For the current plumage, continue interrogating — still one question at a time, with the same delegated-phase option to batch independent questions via a scratchpad per `incubator.md` — over the decomposition: ...
```

(Excerpted for width; full diff is on the branch commit.)

## AC-3

`fledge preen` passes after the refresh (exit 0; all scaffold drift cleared):

```
$ fledge preen
WARN  .fledge/pluma/feathers/FTHR-061-refresh-scaffold-and-verify-worker-protocols-split.md: checked criteria missing evidence sections in /home/penguin/source/fledge/.fledge/burrows/FTHR-066/.fledge/molt/FTHR-061.md: AC-1, AC-2, AC-3, AC-4, AC-5 (heading must be the bare form "## AC-N", not "## AC-N: <label>")
1 warning(s)
exit=0
```

The single remaining WARN is **pre-existing on main** and out of this
feather's scope: `.fledge/molt/FTHR-061.md` (committed in `566d3e8`,
unmodified on this branch — `git diff HEAD --stat` on it is empty) uses
labelled `## AC-N: <label>` headings rather than the bare `## AC-N` form.
It is unrelated to the scaffold/refresh; all scaffold warnings from the
pre-state are gone.

## AC-4

`go vet ./...` and `go test ./...` pass:

```
$ go vet ./...
exit=0
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.101s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.010s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.130s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s
exit=0
```

All packages pass, including `internal/bootstrap` (FTHR-063/064/065's
doc-assertion tests) and `cmd/fledge` (FTHR-062's extended `init.txtar`).

## AC-5

`go test ./cmd/fledge -run TestScripts` passes in full — all 25 fixtures,
uncached (`-count=1`), zero failures or skips:

```
$ go test ./cmd/fledge -run TestScripts -count=1
ok  	github.com/Harrison-Blair/fledge	0.098s
$ go test ./cmd/fledge -run TestScripts -count=1 -v 2>&1 | grep -cE "^    --- PASS: TestScripts/"
25
$ go test ./cmd/fledge -run TestScripts -count=1 -v 2>&1 | grep -E "^    --- (FAIL|SKIP)" || echo "(no FAIL/SKIP)"
(no FAIL/SKIP)
$ ls cmd/fledge/testdata/*.txtar | wc -l
25
$ go test ./cmd/fledge -run TestScripts -count=1 -v 2>&1 | tail -3
    --- PASS: TestScripts/init_agents (0.06s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.094s
```

The new `.fledge/scratch/` gitignore assertion (FTHR-062) is present in the
fixture and exercised by the passing `TestScripts/init`:

```
$ grep -n "scratch" cmd/fledge/testdata/init.txtar
29:grep '.fledge/scratch/' .gitignore
```
