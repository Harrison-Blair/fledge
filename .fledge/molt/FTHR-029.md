# FTHR-029 evidence

## AC-1: tests observed failing before implementation, passing after

Test file: `internal/doctest/docs_test.go` (new package, test-first).

### Failing run (unchanged tree, before any doc edits)

```
$ go test ./internal/doctest/... -v
```

```
=== RUN   TestReadmeDocumentsUpdateCommand
    docs_test.go:66: README.md Commands section does not mention `fledge update`:
        ...
        | Command | Purpose |
        |---|---|
        | `fledge init` / `fledge agents` | scaffold repo + harness adapters; list them |
        | `fledge scan` | inventory the repo for context foraging |
        | `fledge new plumage\|feather` | create specs (IDs, filenames, frontmatter) |
        | `fledge preen` | validate all specs (`--strict` for warnings) |
        | `fledge ready` | feathers whose dependencies are fledged |
        | `fledge vee [PLM-###]` | dependency graph (text/dot/json) |
        | `fledge colony` | full spec inventory |
        | `fledge status` / `fledge set` / `fledge criteria` | update lifecycle, fields, acceptance criteria |
        | `fledge brood` / `fledge abandon` / `fledge broods` | claim, release, list feather claims |
        | `fledge version` | CLI + repo spec version |
        
        Every command takes `--json`. Feather lifecycle: `egg → pipping → hatching →
        fledged`.
    docs_test.go:71: README.md Upgrading section does not mention `fledge update` (binary self-update):
        ...
        - Core skill files under `.fledge/skills/` and adapter agent files (e.g.
          `.claude/agents/*.md`) are yours after init (skip-if-exists); `fledge init
          --refresh` resets all fledge-owned files to the shipped versions. ...
        - Generated adapter files (`fledge-adapter.md`, `settings.local.json`, …) are
          regenerated on every init — don't hand-edit those.
        - Coming from 0.1.0 (skill under `.claude/skills/`)? See
          [MIGRATION.md](MIGRATION.md).
        
        Design rationale for the multi-harness architecture lives in
        [docs/generalization-plan.md](docs/generalization-plan.md).
--- FAIL: TestReadmeDocumentsUpdateCommand (0.00s)
=== RUN   TestReleasingDocCoversScaffoldRefresh
    docs_test.go:79: reading ../../RELEASING.md: open ../../RELEASING.md: no such file or directory
--- FAIL: TestReleasingDocCoversScaffoldRefresh (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
```

Both tests fail for the expected reason: README lacked any `fledge update`
mention (Commands table + Upgrading section), and `RELEASING.md` didn't
exist at all.

### Passing run (after implementation)

```
$ go test ./internal/doctest/... -v
=== RUN   TestReadmeDocumentsUpdateCommand
--- PASS: TestReadmeDocumentsUpdateCommand (0.00s)
=== RUN   TestReleasingDocCoversScaffoldRefresh
--- PASS: TestReleasingDocCoversScaffoldRefresh (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
```

## AC-2: README Commands table + Upgrading section cover `fledge update`

- `README.md` §Commands: added row
  `| \`fledge update\` | check for and install a newer \`fledge\` binary (dry-run by default; \`--yes\` to apply) |`.
- `README.md` §Upgrading: added a leading bullet distinguishing binary
  self-update (`fledge update`) from scaffold refresh (`fledge init
  --refresh`), and cross-linking `RELEASING.md`.
- Verified by `TestReadmeDocumentsUpdateCommand` (passing run above), and by
  reading the rendered section (see `git diff README.md` on the branch).

## AC-3: RELEASING.md documents version-bump steps + refresh/commit-stamp requirement

- New `RELEASING.md` at repo root, verified against the actual repo rather
  than guessed:
  - Version-stamped locations confirmed by reading
    `internal/cli/version.go` (`binaryVersion` var) and
    `cmd/fledge/testdata/stamp_warning.txtar` (fixture pins an
    old/new version pair), alongside `VERSION`.
  - Release trigger confirmed by reading `.github/workflows/release.yml`:
    it triggers on push to `main`, `detect-version` diffs `VERSION` against
    the previous commit, and only builds/releases (tag `v$VERSION`,
    4-platform archives) when it changed — no separate manual tag/push step.
  - New requirement documented: `fledge init --refresh` after reinstalling
    the bumped binary, then commit the regenerated `.fledge/scaffold.json`
    so the dogfood stamp tracks the new version (ties back to the
    `stamp_warning.txtar` mismatch-warning behavior).
- Cross-linked from `README.md` §Upgrading (`[RELEASING.md](RELEASING.md)`).
- Verified by `TestReleasingDocCoversScaffoldRefresh` (passing run above).

## AC-4: `fledge preen` passes and `go test ./...` is green

```
$ go build ./... && go vet ./... && go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.079s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.133s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.006s
```

```
$ fledge preen
WARN  .fledge/pluma/feathers/FTHR-029-...: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-032-...: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-033-...: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-034-...: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-035-...: status hatching but no brood is held
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
7 warning(s)
exit=0
```

`fledge preen` exits 0 ("passes" — warnings only, all pre-existing/unrelated
to this feather's doc-only scope: brood-claim state for concurrently
in-flight feathers on other worktrees, and pre-existing scaffold gaps not
touched by this change).
