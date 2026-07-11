# Migrating a fledge 0.3.x repo to 0.4.0

fledge 0.4.0 moves the spec-directory convention from root `pluma/` to
`.fledge/pluma/`, so every plumage and feather lives under `.fledge/` instead
of the repo root. fledge will not move your files, so a 0.3.x repo needs one
manual move plus a refresh.

## What changed

- The spec-directory convention is now `.fledge/pluma/` (was `pluma/` at the
  repo root). `plumage/PLM-###` and `feathers/FTHR-###` subpaths are unchanged
  — only the parent directory moved.
- The `fledge` CLI, the scaffolded skills, and this repo's own docs all
  reference `.fledge/pluma/...` now; a 0.3.x repo's on-disk `pluma/` and its
  doc references are stale until migrated.

## Steps

1. Move the spec directory (git keeps the history):

   ```sh
   git mv pluma .fledge/pluma
   ```

2. Rebuild and reinstall `fledge` against the version that expects
   `.fledge/pluma/` (skip this if you're already running it from a package
   manager or release build):

   ```sh
   go install ./cmd/fledge
   hash -r
   fledge version
   ```

3. Re-run refresh to sync the scaffold to the new convention:

   ```sh
   fledge init --refresh
   ```

4. Review and commit:

   ```sh
   git add -A
   git commit -m "chore: migrate pluma/ to .fledge/pluma/ (fledge 0.4.0)"
   ```

---

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

Everything else is unchanged: `.fledge/nest/`, spec files, and all CLI
commands behave as before. (`pluma/` itself later moved to `.fledge/pluma/`
in 0.4.0 — see the migration section above.)

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

Nothing else moves: `.fledge/nest/` and all spec files are untouched, and
every CLI command behaves as before. (`pluma/` itself later moved to
`.fledge/pluma/` in 0.4.0 — see the migration section above.)
