# Migrating a fledge 0.1.0 repo to 0.2.0

fledge 0.2.0 moves the orchestration workflow out of `.claude/` into an
agent-neutral location so every harness (Claude Code, pi, Codex, …) loads the
same skill. fledge will not move your files, but `fledge init` refuses to
create a duplicate skill, so a 0.1.0 repo needs these one-time manual moves.

## What changed

- The orchestration skill now lives at `.fledge/skills/fledge-orchestrate/`
  (was `.claude/skills/fledge-orchestrate/`).
- The interrogation skill ships with fledge as
  `.fledge/skills/fledge-interrogate/` (was a repo-local `interrogate` skill,
  if you had one).
- `.claude/settings.local.json`'s `Bash(fledge …)` allow-list is now generated
  from the CLI's actual command set — the 0.1.0 hand-maintained list was stale.
- Each scaffolded harness gets a generated `fledge-adapter.md` (its primitive
  map) and, for Claude, `team-loop.md` (harness runtime notes).

## Steps

1. Remove the old skill copies (git keeps the history; the new copies are
   scaffolded by fledge):

   ```sh
   git rm -r .claude/skills/fledge-orchestrate
   git rm -r .claude/skills/interrogate   # if present
   ```

2. Re-run init to scaffold the new layout (additive; regenerates the managed
   adapter files, including the stale allow-list):

   ```sh
   fledge init
   ```

3. Review and commit. If you had local edits to the old skill prose, port them
   into `.fledge/skills/` — those files are yours after init (skip-if-exists;
   `fledge init --refresh` syncs them back to the shipped versions).

Nothing else moves: `.fledge/nest/`, `pluma/`, and all spec files are
untouched, and every CLI command behaves as before.
