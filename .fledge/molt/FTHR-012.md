# FTHR-012 Evidence

## AC-1

Tests written test-first. Run against **unchanged code** before any implementation.

### `go test ./internal/bootstrap/...`

```
# github.com/Harrison-Blair/fledge/internal/bootstrap [github.com/Harrison-Blair/fledge/internal/bootstrap.test]
internal/bootstrap/registry_test.go:473:13: undefined: WriteOpts
internal/bootstrap/registry_test.go:479:49: undefined: WriteOpts
internal/bootstrap/registry_test.go:484:48: undefined: WriteOpts
internal/bootstrap/registry_test.go:489:56: undefined: WriteOpts
internal/bootstrap/registry_test.go:494:47: undefined: WriteOpts
internal/bootstrap/registry_test.go:508:37: assignment mismatch: 5 variables but m.WriteAdapter returns 4 values
internal/bootstrap/registry_test.go:618:30: undefined: PruneObsolete
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap [build failed]
FAIL
```

New tests `TestPreserveDecision` and `TestPruneObsolete` fail to build because `WriteOpts` and `PruneObsolete` do not yet exist in the bootstrap package, and `WriteAdapter` still returns 4 values (not 5). Expected failure.

### `go test ./cmd/fledge -run TestScripts/refresh_scaffold`

```
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/refresh_scaffold (0.00s)
        ...
        > exec fledge init --refresh
        [stdout]
        created .fledge/nest/raw/.gitkeep
        ...
        updated .fledge/scaffold.json
        > stdout 'removed .fledge/old-skill/SKILL.md'
        FAIL: testdata/refresh_scaffold.txtar:11: no match for `removed .fledge/old-skill/SKILL.md` found in stdout
FAIL
```

The `refresh_scaffold.txtar` test fails because `fledge init --refresh` does not yet implement the prune pass (no "removed" output for obsolete stamp entries). Expected failure.

## AC-2

Satisfy PLM-009 FC-4: refresh rewrites provably-unedited files, keeps and
reports user-edited ones, and `--force` restores overwrite behavior.
Always-managed policies (generate, primitive_map, overwrite, symlink) are
unaffected.

**Preserve decision table (`TestPreserveDecision` in `registry_test.go`):**
- `unedited`: disk == old-stamp hash → rewrite (updated)
- `edited`: disk ≠ old-stamp hash → preserve (preserved/kept)
- `no-stamp-entry`: no old stamp entry for path → preserve (kept)
- `force`: overwrite regardless of hash or stamp (`--force`)

**Always-managed policies tested in `TestWriteAdapterRefresh`** (existing test
updated for new signature): overwrite-policy files remain always repaired;
generate/primitive_map/symlink policies are unchanged.

**CLI output verified in `refresh_scaffold.txtar`:**
- Section B: user-edited brooder.md → `kept .claude/agents/fledge-brooder.md (user-edited; use --force)`
- Section C: same file with `--force` → `updated .claude/agents/fledge-brooder.md`

All passing:
```
ok  github.com/Harrison-Blair/fledge/internal/bootstrap
ok  github.com/Harrison-Blair/fledge/cmd/fledge
```

## AC-3

Satisfy PLM-009 FC-5: prune only stamp-proven, content-matching obsolete
files (or symlinks at recorded target); report everything else without
deleting; never touch paths outside the stamp; stampless path adopts without
pruning.

**Prune decision table (`TestPruneObsolete` in `registry_test.go`):**
- `hash-match`: disk hash == stamp hash → deleted
- `mismatch`: disk hash ≠ stamp hash → kept + reported (not deleted)
- `missing`: file absent from disk → no-op (not an error)
- `symlink-at-target`: symlink points at recorded target → deleted
- `symlink-repointed`: symlink repointed by user → kept + reported

**Prune orchestration in `init.go`:**
- Paths absent from stamp are never passed to `PruneObsolete` (enforced by
  building `newExpected` from `allFiles` only)
- Stampless refresh: `oldStamp == nil` → prune pass skipped entirely

**CLI output verified in `refresh_scaffold.txtar`:**
- Section A: hash-matching obsolete file → `removed .fledge/old-skill/SKILL.md`
- Section E: user-edited obsolete file → `obsolete .fledge/old-skill/mismatch.md (user-edited — remove manually)`
- Section F: stampless refresh → no `removed` output, custom edits preserved

## AC-4

`--refresh` help text and init refresh note updated:

**Flag help text (init.go):**
```
--refresh  sync fledge-owned files to the shipped versions; user-edited
           default-policy files are kept and reported unless --force is
           also set
--force    with --refresh: overwrite user-edited files (restores old
           overwrite behavior)
```

**Usage string updated:**
```
fledge init [--agent <name>]... [--refresh] [--force] [--list-agents] [--json]
```

**Stderr note (`init.go`):** now only emitted when `len(updated) > 0` (files
actually rewritten), not when files are merely preserved.

**Full suite green:**
```
ok  github.com/Harrison-Blair/fledge/cmd/fledge
ok  github.com/Harrison-Blair/fledge/internal/bootstrap
ok  github.com/Harrison-Blair/fledge/internal/check
ok  github.com/Harrison-Blair/fledge/internal/cli
ok  github.com/Harrison-Blair/fledge/internal/graph
ok  github.com/Harrison-Blair/fledge/internal/lock
ok  github.com/Harrison-Blair/fledge/internal/nest
ok  github.com/Harrison-Blair/fledge/internal/scan
ok  github.com/Harrison-Blair/fledge/internal/spec
go vet ./... → (no output, clean)
```
