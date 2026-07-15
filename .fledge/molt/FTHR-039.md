# FTHR-039 evidence

## AC-1

Rebuild + verify installed binary matches VERSION:

```
$ go install ./cmd/fledge && hash -r && command -v fledge && fledge version && cat VERSION
/home/penguin/go/bin/fledge
fledge 0.5.4
0.5.4
```

**Failing capture** — `fledge preen -strict` run immediately after rebuild, BEFORE `fledge init --refresh`. Both target scaffold files are flagged stale/missing (embedded content in the freshly-rebuilt binary moved on from FTHR-038, on-disk copies didn't):

```
$ fledge preen -strict; echo "EXIT: $?"
WARN  .fledge/pluma/feathers/FTHR-038-auto-resolve-tmux-precondition.md: checked criteria missing evidence sections in /home/penguin/source/fledge/.fledge/burrows/FTHR-039/.fledge/molt/FTHR-038.md: AC-5, AC-6
WARN  .fledge/pluma/feathers/FTHR-036-harden-skua-review-protocol-prose.md: status hatching but no brood is held
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .claude/team-loop.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/implementation.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
6 warning(s)
EXIT: 1
```

Both target files present: `.claude/team-loop.md` ("scaffold file is stale (unedited, refresh-safe)") and `.fledge/skills/fledge-orchestrate/implementation.md` ("scaffold file is stale (unedited, refresh-safe)"). (`.claude/settings.local.json` and `.fledge/nest/raw/.gitkeep` are gitignored, worktree-local scaffold entries — expected to show as missing in a fresh worktree checkout, unrelated to this feather's two target files.)

Refresh:

```
$ fledge init --refresh; echo "EXIT: $?"
note: refreshed 3 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
created .fledge/nest/raw/.gitkeep
created .claude/settings.local.json
updated .fledge/skills/fledge-orchestrate/implementation.md
updated .claude/team-loop.md
updated .fledge/scaffold.json
...
scaffolded agents: claude
EXIT: 0
```

**Passing capture** — `fledge preen -strict` run again after `fledge init --refresh`. Both target scaffold files no longer flagged (0 scaffold/drift findings for `.claude/team-loop.md` and `.fledge/skills/fledge-orchestrate/implementation.md`, down from stale in the capture above):

```
$ fledge preen -strict; echo "EXIT: $?"
WARN  .fledge/pluma/feathers/FTHR-038-auto-resolve-tmux-precondition.md: checked criteria missing evidence sections in /home/penguin/source/fledge/.fledge/burrows/FTHR-039/.fledge/molt/FTHR-038.md: AC-5, AC-6
WARN  .fledge/pluma/feathers/FTHR-036-harden-skua-review-protocol-prose.md: status hatching but no brood is held
2 warning(s)
EXIT: 1
```

The 2 remaining warnings are pre-existing and out of this feather's scope — unrelated to scaffold/tmux drift:
- `FTHR-038 ... missing evidence sections ... AC-5, AC-6` — reproduced identically by running `fledge preen -strict` directly on `main` (unmodified, before any of this feather's changes):
  ```
  $ cd /home/penguin/source/fledge && fledge preen -strict
  WARN  .fledge/pluma/feathers/FTHR-038-auto-resolve-tmux-precondition.md: checked criteria missing evidence sections in /home/penguin/source/fledge/.fledge/molt/FTHR-038.md: AC-5, AC-6
  WARN  .claude/settings.local.json: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
  WARN  .claude/team-loop.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
  WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
  WARN  .fledge/skills/fledge-orchestrate/implementation.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
  5 warning(s)
  ```
  confirming the FTHR-038 evidence warning predates and is independent of this feather.
- `FTHR-036 ... no brood is held` — a brood-lock-state warning about a different in-progress feather's worktree (`.fledge/burrows/FTHR-036`), unrelated to scaffold content; not in this feather's Affected Modules.

Neither warning is about scaffold drift for `.claude/team-loop.md` or `.fledge/skills/fledge-orchestrate/implementation.md` — both are absent from the passing capture, satisfying AC-1.

## AC-2

```
$ go vet ./...
(no output, exit 0)

$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.083s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.010s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.165s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s
```

All packages pass, including `cmd/fledge` (`TestScripts`, the txtar acceptance suite). Re-verified no fixture references tmux prose:

```
$ grep -rl "tmux" cmd/fledge/testdata/*.txtar
(no matches)
```

No fixture tripped on the regenerated content; none needed updating.

## AC-3

```
$ diff .claude/team-loop.md internal/bootstrap/adapters/claude/team-loop.md && echo "team-loop.md IDENTICAL"
team-loop.md IDENTICAL

$ diff .fledge/skills/fledge-orchestrate/implementation.md internal/bootstrap/core/skills/fledge-orchestrate/implementation.md && echo "implementation.md IDENTICAL"
implementation.md IDENTICAL
```

`.fledge/scaffold.json` diff shows both new content hashes recorded:

```diff
     ".claude/team-loop.md": {
       "policy": "overwrite",
-      "sha256": "dd07fb9d6e63ccb1586138faa1a76e8646ff0cb735d397f6c7b2236686371bdc"
+      "sha256": "7df66f7f5b43cc7ab93a0ee69beb3e418c22c328ab0b241ca82be531c12e19fa"
     },
     ".fledge/skills/fledge-orchestrate/implementation.md": {
       "policy": "core",
-      "sha256": "162190f8c2e540050e6ddcbfb7aa84ea1973c8810add4978772106b096a74841"
+      "sha256": "ba9a439b86434c31dfdb50e2d546c0044879d6b408d2992c3c0770d210b7aa98"
     },
```

## AC-4

```
$ git status
On branch feather/FTHR-039-rebuild-and-resync-scaffold-for-tmux-auto-default
Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
  (use "git restore <file>..." to discard changes in working directory)
	modified:   .claude/team-loop.md
	modified:   .fledge/scaffold.json
	modified:   .fledge/skills/fledge-orchestrate/implementation.md

no changes added to commit (use "git add" and/or "git commit -a")
```

Only the two regenerated scaffold copies and the scaffold stamp changed — no unrelated drift, no txtar fixture needed updating. (`fledge init --refresh` also created two gitignored worktree-local files, `.claude/settings.local.json` and `.fledge/nest/raw/.gitkeep`, which do not appear in `git status` since they are ignored — expected in a fresh worktree checkout.)
