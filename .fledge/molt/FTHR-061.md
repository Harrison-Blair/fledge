# FTHR-061 Evidence — Refresh scaffold and verify worker-protocols split

All commands run inside the worktree `.fledge/burrows/FTHR-061` (branch
`feather/FTHR-061-refresh-scaffold-split`), using a binary built from this
worktree's source: `go build -o ./fledge-local ./cmd/fledge` (reports
`fledge 0.5.8`, matching `VERSION`). The `fledge-local` binary is not committed.

## AC-1: preen reports scaffold drift before `--refresh`, clean after

Pre-refresh state (failing for the expected reason — the merged
FTHR-057/058/059/060 source changed `internal/bootstrap` content, but this
repo's own scaffolded copies had not been resynced; note the split's new
`brooder.md`/`incubator.md`/`skua.md` reported as missing):

```
$ ./fledge-local preen; echo "EXIT=$?"
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .claude/team-loop.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
WARN  .fledge/skills/fledge-interrogate/SKILL.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/brooder.md: scaffold file is missing — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/foraging.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/implementation.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/incubator.md: scaffold file is missing — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/planning.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/skua.md: scaffold file is missing — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/worker-protocols.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .gitignore: scaffold file is missing — run fledge init --refresh
12 warning(s)
EXIT=0
```

Post-refresh run: see AC-3 (clean, 0 findings).

Note on the "missing" entries beyond the split files:
`.claude/settings.local.json` and `.fledge/nest/raw/.gitkeep` are gitignored,
so they exist on the main working tree but were absent from this fresh
worktree checkout; `.gitignore` is an `append_if_missing` scaffold entry.
These are worktree-checkout artifacts, not PLM-027 regressions.

## AC-2: `fledge init --refresh` completes with only the expected file set touched

```
$ ./fledge-local init --refresh; echo "EXIT=$?"
note: refreshed 7 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
created .fledge/nest/raw/.gitkeep
created .gitignore
created .fledge/skills/fledge-orchestrate/brooder.md
created .fledge/skills/fledge-orchestrate/incubator.md
created .fledge/skills/fledge-orchestrate/skua.md
created .claude/settings.local.json
updated .fledge/skills/fledge-interrogate/SKILL.md
updated .fledge/skills/fledge-orchestrate/foraging.md
updated .fledge/skills/fledge-orchestrate/implementation.md
updated .fledge/skills/fledge-orchestrate/planning.md
updated .fledge/skills/fledge-orchestrate/worker-protocols.md
updated .claude/team-loop.md
updated .fledge/scaffold.json
[... "exists" lines for all unchanged scaffold files elided; full run showed
 every other scaffold entry as `exists` ...]
scaffolded agents: claude
EXIT=0
```

No confirmation prompt was raised (all stale files were "unedited,
refresh-safe"), so no force flag was needed.

Resulting git change set:

```
$ git status --short
 M .claude/team-loop.md
 M .fledge/scaffold.json
 M .fledge/skills/fledge-interrogate/SKILL.md
 M .fledge/skills/fledge-orchestrate/foraging.md
 M .fledge/skills/fledge-orchestrate/implementation.md
 M .fledge/skills/fledge-orchestrate/planning.md
 M .fledge/skills/fledge-orchestrate/worker-protocols.md
 M .gitignore
?? .fledge/molt/FTHR-061.md
?? .fledge/skills/fledge-orchestrate/brooder.md
?? .fledge/skills/fledge-orchestrate/incubator.md
?? .fledge/skills/fledge-orchestrate/skua.md
?? fledge-local
```

(`fledge-local` is the uncommitted local build; `.fledge/molt/FTHR-061.md` is
this evidence file. `.claude/settings.local.json` and
`.fledge/nest/raw/.gitkeep` are gitignored so they do not appear.)

**Deviation from the spec's Affected Modules list, with attribution.** The
spec predicted only `.fledge/scaffold.json` +
`.fledge/skills/fledge-orchestrate/{worker-protocols,incubator,brooder,skua}.md`.
Six additional files changed, but every one is refresh-produced output of
`internal/bootstrap` source already merged to main by sibling feathers — none
is unrelated drift:

- `planning.md`, `foraging.md`, `implementation.md` — FTHR-058's repointing of
  `worker-protocols.md §Incubator` references to `incubator.md` (verified: the
  diffs are exactly those reference swaps).
- `.fledge/skills/fledge-interrogate/SKILL.md` — FTHR-063's scratchpad-batching
  exception sentence.
- `.gitignore` — FTHR-063's `append_if_missing` line `.fledge/scratch/`
  (+ its comment line).
- `.claude/team-loop.md` — the merged "Digest and compaction" section from the
  digest plumage.

Byte-identity of every regenerated skill file against the embedded source of
truth was verified directly:

```
$ for f in worker-protocols brooder incubator skua planning foraging implementation; do
    diff -q internal/bootstrap/core/skills/fledge-orchestrate/$f.md .fledge/skills/fledge-orchestrate/$f.md && echo "$f.md OK"; done
worker-protocols.md OK
brooder.md OK
incubator.md OK
skua.md OK
planning.md OK
foraging.md OK
implementation.md OK
$ diff -q internal/bootstrap/core/skills/fledge-interrogate/SKILL.md .fledge/skills/fledge-interrogate/SKILL.md && echo "interrogate SKILL.md OK"
interrogate SKILL.md OK
```

The core PLM-027 change set (the split itself):

```
$ git diff --stat .fledge/skills/fledge-orchestrate/worker-protocols.md .fledge/scaffold.json
 .fledge/scaffold.json                              |  31 ++++--
 .../skills/fledge-orchestrate/worker-protocols.md  | 109 +--------------------
 2 files changed, 27 insertions(+), 113 deletions(-)
```

## AC-3: `fledge preen` passes after refresh

```
$ ./fledge-local preen; echo "EXIT=$?"
spec clean: 29 plumages, 71 feathers
EXIT=0
```

Zero findings (the 12 pre-refresh scaffold warnings are gone).

## AC-4: `go vet ./...` and `go test ./...` pass

```
$ go vet ./... && echo "VET OK"
VET OK
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.094s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.010s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.142s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.008s
```

The PLM-027 doc tests are present and pass (verbose spot-check of
`internal/bootstrap`): `TestBrooderDocSections`, `TestBrooderFixLoopInvariant`,
`TestIncubatorDocSections`, `TestSkuaDocSections` (via `skua_test.go`),
`TestCoreDocsRepointToRoleFiles`, `TestInterrogateSkillDocumentsBatchingException`
— all `--- PASS`.

## AC-5: `go test ./cmd/fledge -run TestScripts` passes in full

```
$ go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.101s
```

