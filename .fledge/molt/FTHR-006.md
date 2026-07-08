# FTHR-006 Evidence

## AC-1

Tests written first, observed failing before implementation.

### Unit test: `go test ./internal/nest -run TestStampPreservesBodyAndDropsUnknownKeys -v`

```
# github.com/Harrison-Blair/fledge/internal/nest_test [github.com/Harrison-Blair/fledge/internal/nest.test]
internal/nest/nest_test.go:119:19: undefined: nest.RefreshDoc
internal/nest/nest_test.go:157:20: undefined: nest.RefreshDoc
internal/nest/nest_test.go:174:20: undefined: nest.RefreshDoc
FAIL	github.com/Harrison-Blair/fledge/internal/nest [build failed]
FAIL
```

### Txtar test: `go test ./cmd/fledge -run TestScripts/nest -v`

(Abbreviated to the failing assertion)

```
# nest scout: missing --module → exit 2 (usage error) (0.001s)
> ! exec fledge nest scout
[stderr]
fledge: fledge nest: unknown verb "scout" (available: new)
[exit status 2]
> stderr 'module'
FAIL: testdata/nest.txtar:35: no match for `module` found in stderr

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/nest (0.01s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.013s
FAIL
```

Both fail for the expected reason: `RefreshDoc` is not yet defined, and the `scout`/`scaffold`/`stamp` verbs do not yet exist.

## AC-2

`nest scaffold` clears all `.md` files in `.fledge/nest/` and the entire `raw/` subdirectory, then recreates all nine concern docs with stamped frontmatter. The txtar test pre-seeds `stale.md` and `raw/x.md`, runs `fledge nest scaffold`, and asserts:

```
exists .fledge/nest/architecture.md
exists .fledge/nest/modules.md
exists .fledge/nest/conventions.md
exists .fledge/nest/data-model.md
exists .fledge/nest/dependencies.md
exists .fledge/nest/entry-points.md
exists .fledge/nest/testing.md
exists .fledge/nest/domain.md
exists .fledge/nest/index.md
! exists .fledge/nest/stale.md
! exists .fledge/nest/raw/x.md
```

All nine `ConcernDocs` are created; stale and stray files are gone. Scaffold output from passing test run:

```
created .fledge/nest/architecture.md
created .fledge/nest/modules.md
created .fledge/nest/conventions.md
created .fledge/nest/data-model.md
created .fledge/nest/dependencies.md
created .fledge/nest/entry-points.md
created .fledge/nest/testing.md
created .fledge/nest/domain.md
created .fledge/nest/index.md
```

## AC-3

`nest scout --module cli` creates `.fledge/nest/raw/cli.md` with correct scout-schema frontmatter (`module:`, `authored:`, `agent:`, `fledge_version:`) and the embedded template body (`# Module: cli`).

- Missing `--module` → exit 2 (`usageErr`), stderr contains `module`
- Existing file without `--force` → exit 1, stderr contains `cli`
- `--force` overwrites successfully

From the passing txtar test run:
```
> exec fledge nest scout --module cli
[stdout] created .fledge/nest/raw/cli.md
> grep 'module: cli' .fledge/nest/raw/cli.md  ✓
> grep 'authored:' .fledge/nest/raw/cli.md    ✓
> grep 'agent: fledge-context-scout' ...       ✓
> grep '# Module: cli' ...                     ✓
> ! exec fledge nest scout (no --module) → exit 2, stderr 'module' ✓
> ! exec fledge nest scout --module cli (exists) → exit 1, stderr 'cli' ✓
```

## AC-4

`nest stamp <file>`:
- Refreshes derived fields (`generated`, `commit`, `fledge_version`), drops unknown keys, preserves `agent` and body byte-for-byte — verified in txtar test with pre-seeded `stale-doc.md` (contains `stale_key:`) and `TestStampPreservesBodyAndDropsUnknownKeys` unit test.
- `--agent` override replaces stored agent — verified in both tests.
- Detects kind by path: `raw/cli.md` → Scout schema (no `generated:` field) — txtar asserts `! grep 'generated:' .fledge/nest/raw/cli.md`.
- `--kind concern` overrides path-based detection: `raw/cli.md --kind concern` → concern schema (`generated:` present, `module:` absent) — txtar asserts `grep 'generated:'` and `! grep 'module:'`.
- Invalid `--kind bogus` → exit 2, stderr `bogus`.
- Out-of-nest path → exit 2 — txtar: `! exec fledge nest stamp outsidepath.md`, stderr `outside`.

From passing txtar run:
```
> exec fledge nest stamp .fledge/nest/stale-doc.md
[stdout] stamped .fledge/nest/stale-doc.md
> ! grep 'stale_key:' .fledge/nest/stale-doc.md  ✓
> grep 'agent: fledge-forager' ...               ✓
> grep 'Original body content' ...               ✓
> exec fledge nest stamp .fledge/nest/stale-doc.md --agent fledge-new-agent
> grep 'agent: fledge-new-agent' ...             ✓
> exec fledge nest stamp .fledge/nest/raw/cli.md
> ! grep 'generated:' .fledge/nest/raw/cli.md    ✓
> exec fledge nest stamp .fledge/nest/raw/cli.md --kind concern
[stdout] stamped .fledge/nest/raw/cli.md
> grep 'generated:' .fledge/nest/raw/cli.md      ✓
> ! grep 'module:' .fledge/nest/raw/cli.md       ✓
> ! exec fledge nest stamp .fledge/nest/stale-doc.md --kind bogus → exit 2, stderr 'bogus' ✓
> ! exec fledge nest stamp outsidepath.md → exit 2, stderr 'outside' ✓
```

## AC-5

All verbs honor `--json` (verified in txtar for scaffold, scout, and stamp). Exit codes: scaffold exits 0 on success; scout exits 2 for missing `--module` and 1 for existing-without-force; stamp exits 2 for out-of-nest path.

Full suite: `go test ./...` and `go vet ./...` pass cleanly:

```
ok  github.com/Harrison-Blair/fledge/cmd/fledge      0.061s
ok  github.com/Harrison-Blair/fledge/internal/bootstrap 0.004s
ok  github.com/Harrison-Blair/fledge/internal/check  0.001s
ok  github.com/Harrison-Blair/fledge/internal/cli    0.002s
ok  github.com/Harrison-Blair/fledge/internal/graph  0.001s
ok  github.com/Harrison-Blair/fledge/internal/lock   0.001s
ok  github.com/Harrison-Blair/fledge/internal/nest   0.001s
ok  github.com/Harrison-Blair/fledge/internal/scan   0.007s
ok  github.com/Harrison-Blair/fledge/internal/spec   0.002s
```

`go vet ./...` — no output (clean).
