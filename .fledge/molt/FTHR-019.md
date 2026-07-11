# FTHR-019 Evidence

## AC-1

Grep-based before/after check for stale `pluma/plumage`/`pluma/feathers` path
references across the three prose docs (per the feather's Approach steps 1
and 4), pinning FC-4 for this surface.

### Before (pre-edit) — FAILING evidence: non-zero stale references

```
$ grep -n 'pluma/plumage\|pluma/feathers' README.md CLAUDE.md docs/generalization-plan.md
README.md:8:The bird theme, decoded: a **plumage** (`pluma/plumage/PLM-###`) is a
README.md:9:requirement/feature spec; a **feather** (`pluma/feathers/FTHR-###`) is one
CLAUDE.md:8:agent-assisted repos. It keeps feature intent (**plumages**, `pluma/plumage/PLM-###`)
CLAUDE.md:9:and implementable tasks (**feathers**, `pluma/feathers/FTHR-###`) as validated

$ grep -c 'pluma/plumage\|pluma/feathers' README.md CLAUDE.md docs/generalization-plan.md
README.md:2
CLAUDE.md:2
docs/generalization-plan.md:0
```

4 stale references observed in README.md/CLAUDE.md (docs/generalization-plan.md's
one reference — `pluma/` at line 18 — doesn't contain the literal substrings
`pluma/plumage`/`pluma/feathers`, but was also updated below since it's the
same stale root-`pluma/` location per the feather's Description/Affected
Modules). This is the FAILING/pre-edit state: non-zero stale references
exist, matching the expected pre-implementation condition.

### After (post-edit) — passing evidence: zero stale references

```
$ grep -n 'pluma/plumage\|pluma/feathers' README.md CLAUDE.md docs/generalization-plan.md | grep -v '\.fledge/pluma'
(no output, grep exit 1)
```

Zero stale (non-`.fledge/pluma/`) references remain. All three docs now read
`.fledge/pluma/plumage/PLM-###`, `.fledge/pluma/feathers/FTHR-###`, or plain
`.fledge/pluma/` where they previously read the bare `pluma/` form.

Full `pluma/` sweep across the three files, confirming every remaining
occurrence is prefixed `.fledge/`:

```
$ grep -n 'pluma/' README.md CLAUDE.md docs/generalization-plan.md
README.md:8:The bird theme, decoded: a **plumage** (`.fledge/pluma/plumage/PLM-###`) is a
README.md:9:requirement/feature spec; a **feather** (`.fledge/pluma/feathers/FTHR-###`) is one
README.md:20:fledge init                 # scaffold .fledge/ (including .fledge/pluma/) and your agent's adapter
CLAUDE.md:8:agent-assisted repos. It keeps feature intent (**plumages**, `.fledge/pluma/plumage/PLM-###`)
CLAUDE.md:9:and implementable tasks (**feathers**, `.fledge/pluma/feathers/FTHR-###`) as validated
CLAUDE.md:16:`.fledge/broods/`), specs under `.fledge/pluma/`, and a scaffolded Claude adapter under
docs/generalization-plan.md:18:1. **The `fledge` CLI** (`cmd/fledge`, `internal/*` except `internal/bootstrap/`) — already fully agent-agnostic. It manages specs on disk (`.fledge/`, `.fledge/pluma/`), allocates IDs, validates, locks, renders JSON. Zero agent references outside `init.go`'s hardcoded `.claude` destination.
```

## AC-2

None of the three docs contain a bare `pluma/plumage` or `pluma/feathers`
path reference (confirmed by the "after" grep above returning zero matches
outside `.fledge/pluma/`). Satisfies PLM-011 FC-4 for this surface.

## AC-3

`MIGRATION.md` gained a new top section, "Migrating a fledge 0.3.x repo to
0.4.0", documenting:
- What changed: the spec-directory convention moved from root `pluma/` to
  `.fledge/pluma/`.
- The manual steps: `git mv pluma .fledge/pluma`, rebuild/reinstall fledge
  (`go install ./cmd/fledge && hash -r && fledge version`), `fledge init
  --refresh`, and commit.

This matches, step for step, what FTHR-021 ("Migrate this repo's specs to
.fledge/pluma/, bump to 0.4.0, and refresh the dogfood scaffold") actually
performs per its own Approach section: bump version, rebuild/reinstall,
`git mv pluma .fledge/pluma`, `fledge init --refresh`, verify, commit —
confirmed by reading `pluma/feathers/FTHR-021-*.md` directly.

The two previously-false statements are corrected:
- Line 30 (0.3.0 section): was "Everything else is unchanged: `.fledge/nest/`,
  `pluma/`, spec files, and all CLI commands behave as before." — now reads
  "Everything else is unchanged: `.fledge/nest/`, spec files, and all CLI
  commands behave as before. (`pluma/` itself later moved to `.fledge/pluma/`
  in 0.4.0 — see the migration section above.)"
- Line 75 (0.2.0 section): was "Nothing else moves: `.fledge/nest/`, `pluma/`,
  and all spec files are untouched, and every CLI command behaves as before."
  — now reads "Nothing else moves: `.fledge/nest/` and all spec files are
  untouched, and every CLI command behaves as before. (`pluma/` itself later
  moved to `.fledge/pluma/` in 0.4.0 — see the migration section above.)"

Confirmed no remaining false claims in the file:

```
$ grep -n "pluma/" MIGRATION.md
3:fledge 0.4.0 moves the spec-directory convention from root `pluma/` to
4:`.fledge/pluma/`, so every plumage and feather lives under `.fledge/` instead
10:- The spec-directory convention is now `.fledge/pluma/` (was `pluma/` at the
14:  reference `.fledge/pluma/...` now; a 0.3.x repo's on-disk `pluma/` and its
26:   `.fledge/pluma/` (skip this if you're already running it from a package
45:   git commit -m "chore: migrate pluma/ to .fledge/pluma/ (fledge 0.4.0)"
80:commands behave as before. (`pluma/` itself later moved to `.fledge/pluma/`
126:every CLI command behaves as before. (`pluma/` itself later moved to
127:`.fledge/pluma/` in 0.4.0 — see the migration section above.)
```

No statement claims `pluma/` is unaffected/untouched by the 0.4.0 migration;
both stale claims are qualified with a forward pointer to the new section.
Satisfies PLM-011 AC-4.
