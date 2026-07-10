# FTHR-016 Evidence

## Binary reinstall

```
$ go install ./cmd/fledge && hash -r && command -v fledge && fledge version
/home/penguin/go/bin/fledge
fledge 0.3.0
```

VERSION file contents: `0.3.0` — version match confirmed.

## AC-1

### Before state (pre-refresh) — drift observed

```
$ fledge preen
WARN  .claude/agents/fledge-incubator.md: scaffold file is missing — run fledge init --refresh
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/foraging.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
WARN  .fledge/skills/fledge-orchestrate/planning.md: scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh
5 warning(s)
```

```
$ ls .claude/agents/
fledge-brooder.md
fledge-context-scout.md
fledge-forager.md
fledge-skua.md
```

`fledge-incubator.md` is ABSENT — confirming the expected pre-refresh failing state.

### After state (post-refresh) — preen clean

```
$ fledge init --refresh
note: refreshed 3 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
created .fledge/nest/raw/.gitkeep
created .claude/agents/fledge-incubator.md
created .claude/settings.local.json
updated .fledge/skills/fledge-orchestrate/foraging.md
updated .fledge/skills/fledge-orchestrate/planning.md
updated .fledge/scaffold.json
exists .fledge/broods/.gitkeep
exists .fledgeignore
exists pluma/plumage/.gitkeep
exists pluma/feathers/.gitkeep
exists .gitignore
exists .fledge/skills/fledge-interrogate/SKILL.md
exists .fledge/skills/fledge-orchestrate/SKILL.md
exists .fledge/skills/fledge-orchestrate/implementation.md
exists .fledge/skills/fledge-orchestrate/templates/context-doc.md
exists .fledge/skills/fledge-orchestrate/templates/feather.md
exists .fledge/skills/fledge-orchestrate/templates/plumage.md
exists .fledge/skills/fledge-orchestrate/templates/scout-report.md
exists .fledge/skills/fledge-orchestrate/worker-protocols.md
exists .claude/agents/fledge-brooder.md
exists .claude/agents/fledge-forager.md
exists .claude/agents/fledge-context-scout.md
exists .claude/agents/fledge-skua.md
exists .claude/settings.json
exists .claude/team-loop.md
exists .claude/fledge-adapter.md
exists .claude/skills/fledge-orchestrate
exists .claude/skills/fledge-interrogate
exists CLAUDE.md
scaffolded agents: claude
```

```
$ fledge preen
spec clean: 10 plumages, 16 feathers
```

Preen reports CLEAN (no drift, no warnings). The drift transition AC-1 is satisfied.

## AC-2

```
$ ls .claude/agents/
fledge-brooder.md
fledge-context-scout.md
fledge-forager.md
fledge-incubator.md
fledge-skua.md
```

`.claude/agents/fledge-incubator.md` exists. Head of file:

```
---
name: fledge-incubator
description: One-shot, non-interactive spec-body drafter for fledge. Given the orchestrator's resolved decisions and pointers (prospective ID, template path, concern docs to cite, and feather-specific fields), reads the template and cited docs and returns the full drafted body as its single final message. Mutates no spec — never runs fledge CLI commands. Not intended for direct use.
model: claude-sonnet-5
---
```

FTHR-015 delegation prose confirmed in planning.md:

```
$ grep -n "incubator" .fledge/skills/fledge-orchestrate/planning.md | head -5
26:4. When that plumage's tree is resolved, draft the full file (frontmatter — with the prospective next `PLM-###` ID — plus the body sections filled from the interrogation). **The body drafting is capability-conditional on `spawn-worker`:** if you provide `spawn-worker`, delegate the body draft to an incubator worker ...
38:6. Author the feathers **one at a time**, in dependency order: for each, draft the full file (frontmatter — prospective next `FTHR-###` ID, plumage link, `depends_on`, `oversight`, priority — plus the body sections: Description, Affected Modules, Approach, Tests, Acceptance Criteria as unchecked `- [ ] AC-N: …` boxes). **The body drafting is capability-conditional on `spawn-worker`:** if you provide `spawn-worker`, delegate the body draft to an incubator worker ...
```

FTHR-015 empty-nest marker confirmed in foraging.md:

```
$ grep -n "empty.nest\|empty state\|empty template" .fledge/skills/fledge-orchestrate/foraging.md | head -5
17:3. **Full regeneration.** Run `fledge nest scaffold` from the repo root. ... **Important:** immediately after `fledge nest scaffold`, `.fledge/nest/` contains only empty template stubs — placeholder concern docs, unfilled `raw/*.md`, and `index.md` frontmatter stamped to HEAD. This empty state is the expected intermediate after scaffolding; scouts and synthesis fill it in steps 4–6 below. It is not a failure and must not be flagged as one.
```

`.fledge/scaffold.json` updated (hashes reflect new embedded content including incubator agent).

Git status confirms only expected regenerated files changed:

```
$ git status
On branch feather/FTHR-016-dogfood-refresh
Changes not staged for commit:
  modified:   .fledge/scaffold.json
  modified:   .fledge/skills/fledge-orchestrate/foraging.md
  modified:   .fledge/skills/fledge-orchestrate/planning.md

Untracked files:
  .claude/agents/fledge-incubator.md
  .fledge/molt/FTHR-016.md
```

(`.claude/settings.local.json` and `.fledge/nest/raw/.gitkeep` are git-ignored per `.gitignore` and global git config — confirmed via `git check-ignore`.)

## AC-3

```
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.076s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
?   	github.com/Harrison-Blair/fledge/internal/repo	[no test files]
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.002s
```

```
$ go vet ./...
(no output — clean)
```

```
$ fledge preen
spec clean: 10 plumages, 16 feathers
```

All green.

