# FTHR-071 evidence

Doc-only integration feather: refresh this repo's scaffold so the phase-close
digest additions from FTHR-067/068/069 (already merged to main at fbb0793)
are resynced into `.fledge/skills/` copies, then verify the full suite.

No Go source changed since FTHR-066's rebuild; installed binary already at
0.5.8, but per instructions the refresh was run with a binary built from this
worktree's source (`go build -o ./fledge ./cmd/fledge`; the root `fledge`
artifact is gitignored, so this does not dirty the tree).

```
$ ./fledge version
fledge 0.5.8
$ cat VERSION
0.5.8
```

## AC-1

Test-first framing (this feather's "failing state"): pre-refresh `fledge preen`
shows the scaffold staleness the refresh must clear.

Pre-refresh capture (verbatim):

```
$ ./fledge preen
WARN  .fledge/pluma/feathers/FTHR-061-refresh-scaffold-and-verify-worker-protocols-split.md: checked criteria missing evidence sections in /home/penguin/source/fledge/.fledge/burrows/FTHR-071/.fledge/molt/FTHR-061.md: AC-1, AC-2, AC-3, AC-4, AC-5 (heading must be the bare form "## AC-N", not "## AC-N: <label>")
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/foraging.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/implementation.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/planning.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
6 warning(s)
preen exit: 0
```

Notes on the pre-state:
- The three stale skill copies are exactly the digest-feature files (FTHR-067/068/069).
- The FTHR-061 molt-heading warning is pre-existing on main, unrelated to this feather.
- `.claude/settings.local.json` and `.fledge/nest/raw/.gitkeep` are gitignored/untracked
  files not present in a fresh worktree; the refresh recreates them (no git impact).

Refresh run (verbatim; completed non-interactively, no `--force` needed since all
stale files were unedited/refresh-safe):

```
$ ./fledge init --refresh
note: refreshed 4 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
created .fledge/nest/raw/.gitkeep
created .claude/settings.local.json
updated .fledge/skills/fledge-orchestrate/foraging.md
updated .fledge/skills/fledge-orchestrate/implementation.md
updated .fledge/skills/fledge-orchestrate/planning.md
updated .fledge/scaffold.json
[... 25 "exists" lines for already-current files ...]
scaffolded agents: claude
refresh exit: 0
```

Only the expected file set touched:

```
$ git status --porcelain
 M .fledge/scaffold.json
 M .fledge/skills/fledge-orchestrate/foraging.md
 M .fledge/skills/fledge-orchestrate/implementation.md
 M .fledge/skills/fledge-orchestrate/planning.md
$ git diff --stat
 .fledge/scaffold.json                               | 6 +++---
 .fledge/skills/fledge-orchestrate/foraging.md       | 2 +-
 .fledge/skills/fledge-orchestrate/implementation.md | 4 ++++
 .fledge/skills/fledge-orchestrate/planning.md       | 3 ++-
 4 files changed, 10 insertions(+), 5 deletions(-)
```

`.claude/team-loop.md` (listed in the spec's Affected Modules) reported `exists`
— it was already current on main, so it produced no diff. The spec's Approach
anticipates this ("confirm only the expected files changed"; a smaller diff is
acceptable and documented here). The diff content is exactly the phase-close
digest additions: foraging.md gains the `digest-foraging.md` commissioner step,
implementation.md gains the digest-planning read + `digest-implementation.md`
write, planning.md gains the digest-implementation read + `digest-planning.md`
write (step 7).

## AC-2

Post-refresh `fledge preen` (verbatim):

```
$ ./fledge preen
WARN  .fledge/pluma/feathers/FTHR-061-refresh-scaffold-and-verify-worker-protocols-split.md: checked criteria missing evidence sections in /home/penguin/source/fledge/.fledge/burrows/FTHR-071/.fledge/molt/FTHR-061.md: AC-1, AC-2, AC-3, AC-4, AC-5 (heading must be the bare form "## AC-N", not "## AC-N: <label>")
1 warning(s)
preen exit: 0
```

Passes (exit 0). All scaffold-drift warnings cleared. The single remaining
warning is the pre-existing FTHR-061 molt-heading issue already present on
main before this feather; it is not produced by this change.

## AC-3

```
$ go vet ./...
VET: clean (exit 0, no output)

$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.095s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.010s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.125s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.008s
```

All packages pass, including `internal/bootstrap` (G1-G4's doc-assertion tests).

## AC-4

```
$ go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.096s
```

Full txtar suite passes.

## AC-5

Not verifiable yet — recorded as such by direction of the orchestrator
(team-lead), who is handling AC-5's disposition with the user.

```
$ ls .fledge/scratch/digest-planning.md
ls: cannot access '.fledge/scratch/digest-planning.md': No such file or directory
```

`.fledge/scratch/digest-planning.md` does not exist: the planning phase that
authored this spec closed before the digest feature (FTHR-067) landed, so no
planning close-out has yet run the new `planning.md` step that writes it.
Nothing was fabricated; the FC-3 format check can only happen after a future
planning phase closes under the refreshed scaffold. This AC remains open.
