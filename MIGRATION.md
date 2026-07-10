# Migrating a fledge 0.2.x repo to 0.3.0

fledge 0.3.0 adds a scaffold stamp file (`.fledge/scaffold.json`) that records
which files fledge owns and at what content hash. The stamp enables the new
preserve/prune semantics: `fledge init --refresh` keeps user-edited files
as-is and prunes files that no longer belong to the scaffold.

## What changed

- `fledge init --refresh` now writes `.fledge/scaffold.json` after each run.
  Prior releases had no stamp; first-time refresh on a 0.2.x repo is
  unconditional and creates the stamp from scratch — no manual steps needed.
- `fledge preen` validates the stamp; it reports healthy once the stamp exists
  and is consistent with the on-disk scaffold.
- `.fledge/scaffold.json` is deterministic (keys sorted, stable JSON), so
  merge conflicts are straightforward to resolve. If two branches both ran
  `init --refresh`, accept whichever is more recent (or re-run refresh after
  merge to regenerate a consistent stamp).

## Steps

No manual steps required. Run `fledge init --refresh` once after upgrading:

```sh
fledge init --refresh     # creates .fledge/scaffold.json; zero prunes on a clean repo
git add .fledge/scaffold.json
git commit -m "chore: create scaffold stamp (fledge 0.3.0)"
```

Everything else is unchanged: `.fledge/nest/`, `pluma/`, spec files, and all
CLI commands behave as before.

---

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
