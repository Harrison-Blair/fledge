# FTHR-080 evidence

## AC-1

Tests were written first (`TestDriftReportDevLink`, `TestEditedOnRefreshOmitsDevLinks`
in `internal/bootstrap/drift_test.go`; `cmd/fledge/testdata/dev_preen.txtar`) and run
against FTHR-077's unmodified `internal/bootstrap/drift.go` (verified via
`git stash push -- internal/bootstrap/drift.go`) before any fix code was written.

### Pre-implementation: unit tests FAILING for the expected reason

Command: `go test ./internal/bootstrap -run 'TestDriftReportDevLink|TestEditedOnRefreshOmitsDevLinks' -v`

```
=== RUN   TestDriftReportDevLink
=== RUN   TestDriftReportDevLink/up-to-date:_dev-linked_file_whose_source_differs_from_shipped_bytes
    drift_test.go:289: dev-linked file with differing source content: got [{Path:.fledge/skills/fledge-orchestrate/SKILL.md Status:modified Policy:core}], want a single StatusUpToDate entry
=== RUN   TestDriftReportDevLink/missing:_dev-linked_file_with_a_dangling_target
=== RUN   TestDriftReportDevLink/modified:_a_regular_file_sitting_where_a_dev_link_is_expected
=== RUN   TestDriftReportDevLink/non-dev_repo:_an_unrelated_genuinely_user-edited_file_still_reports_modified
--- FAIL: TestDriftReportDevLink (0.00s)
    --- FAIL: TestDriftReportDevLink/up-to-date:_dev-linked_file_whose_source_differs_from_shipped_bytes (0.00s)
    --- PASS: TestDriftReportDevLink/missing:_dev-linked_file_with_a_dangling_target (0.00s)
    --- PASS: TestDriftReportDevLink/modified:_a_regular_file_sitting_where_a_dev_link_is_expected (0.00s)
    --- PASS: TestDriftReportDevLink/non-dev_repo:_an_unrelated_genuinely_user-edited_file_still_reports_modified (0.00s)
=== RUN   TestEditedOnRefreshOmitsDevLinks
    drift_test.go:404: EditedOnRefresh listed dev-linked path ".fledge/skills/fledge-orchestrate/SKILL.md" as user-edited, want it omitted
--- FAIL: TestEditedOnRefreshOmitsDevLinks (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

The core-regression subtest fails exactly as the bug predicts: a dev-linked file whose
live source content differs from the shipped bytes is classified `modified` instead of
`up-to-date`, because `DriftReport` took the `classifyContent` branch (`exp.Target ==
""`) and read through the symlink, hashing live source content that can never match the
shipped hash. The dangling-target and clobbered-link subtests already passed
pre-implementation (existing `classifyContent`-via-symlink-read behavior coincidentally
produces the right answer for those two cases) — they are recorded here as a regression
guard, not as new-behavior proof; AC-3 is satisfied by them continuing to pass
post-implementation with the new dedicated code path.

Command: `go test ./cmd/fledge -run 'TestScripts/dev_preen' -v`

```
> exec fledge preen
[stdout]
WARN  .claude/agents/fledge-brooder.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .claude/agents/fledge-context-scout.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .claude/agents/fledge-forager.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .claude/agents/fledge-incubator.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .claude/agents/fledge-skua.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-interrogate/SKILL.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/SKILL.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/brooder.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/foraging.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/implementation.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/incubator.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/planning.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/skua.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/templates/context-doc.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/templates/feather.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/templates/plumage.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/templates/scout-report.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
WARN  .fledge/skills/fledge-orchestrate/worker-protocols.md: scaffold file is user-edited — fledge init --refresh resets it to the shipped version (confirms first; --force skips)
18 warning(s)
> stdout 'spec clean'
FAIL: testdata/dev_preen.txtar:20: no match for `spec clean` found in stdout
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/dev_preen (0.01s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.015s
```

All 18 dev-linked paths (13 core skill files, 5 claude agent files) report the false
`user-edited` finding, exactly the "~18 false warnings" the feather spec describes.

### Post-implementation: PASSING

Fix: `internal/bootstrap/drift.go` — `DriftReport` gains a switch case that, when the
manifest expects content (`exp.Target == ""`) but the repo is dev-linked
(`stamp.DevSource != ""`) and the stamp records a symlink `Target` for this path, routes
classification to a new `classifyDevLink` function instead of `classifyContent`.
`classifyDevLink` compares only against the stamp's recorded target — never the
manifest's shipped-content hash — and additionally verifies the target resolves
(`os.Stat` through the link) so a dangling dev-source file still reports `missing`.

Command: `go test ./internal/bootstrap -run 'TestDriftReportDevLink|TestEditedOnRefreshOmitsDevLinks' -v`

```
=== RUN   TestDriftReportDevLink
=== RUN   TestDriftReportDevLink/up-to-date:_dev-linked_file_whose_source_differs_from_shipped_bytes
=== RUN   TestDriftReportDevLink/missing:_dev-linked_file_with_a_dangling_target
=== RUN   TestDriftReportDevLink/modified:_a_regular_file_sitting_where_a_dev_link_is_expected
=== RUN   TestDriftReportDevLink/non-dev_repo:_an_unrelated_genuinely_user-edited_file_still_reports_modified
--- PASS: TestDriftReportDevLink (0.00s)
    --- PASS: TestDriftReportDevLink/up-to-date:_dev-linked_file_whose_source_differs_from_shipped_bytes (0.00s)
    --- PASS: TestDriftReportDevLink/missing:_dev-linked_file_with_a_dangling_target (0.00s)
    --- PASS: TestDriftReportDevLink/modified:_a_regular_file_sitting_where_a_dev_link_is_expected (0.00s)
    --- PASS: TestDriftReportDevLink/non-dev_repo:_an_unrelated_genuinely_user-edited_file_still_reports_modified (0.00s)
=== RUN   TestEditedOnRefreshOmitsDevLinks
--- PASS: TestEditedOnRefreshOmitsDevLinks (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

Command: `go test ./cmd/fledge -run TestScripts/dev_preen -v` — see AC-2/AC-4 section
below for the full passing trace (same command, one capture).

## AC-2

`dev_preen.txtar` builds a fake dev source tree containing every one of the 18
dev-linkable paths (13 core skill files under `core/skills/...`, 5 `.claude/agents/*.md`
adapter files) with placeholder content that deliberately differs from the shipped
embedded bytes, runs `fledge init --dev=$WORK/src`, edits one further file live in the
source, then runs `fledge preen`.

Command: `go test ./cmd/fledge -run TestScripts/dev_preen -v`

```
> exec fledge preen
[stdout]
spec clean: 0 plumages, 0 feathers
> stdout 'spec clean'
> ! stdout 'scaffold-drift'
> ! stdout 'user-edited'
> ! stdout 'modified'
> cp stdout $WORK/preen-run1.txt
```

No dev-linked path is reported modified/drifted/user-edited: `preen` prints
`spec clean: 0 plumages, 0 feathers`. Also covered at the unit level by
`TestDriftReportDevLink/up-to-date:...` above (AC-1).

## AC-3

Unit-level (`internal/bootstrap/drift_test.go`, `TestDriftReportDevLink`):

- `missing: dev-linked file with a dangling target` — symlink exists, points at a
  dev-source path that was never created → `StatusMissing`.
- `modified: a regular file sitting where a dev link is expected` — a real file (not a
  symlink) written at the dev-linked path → `StatusModified`.
- `non-dev repo: an unrelated genuinely user-edited file still reports modified` — with
  `stamp.DevSource == ""`, an ordinary edited file still reports `StatusModified` (this
  case doesn't even reach `classifyDevLink` — pins that the new branch is unreachable
  outside a dev-linked repo).

Command: `go test ./internal/bootstrap -run TestDriftReportDevLink -v` (full passing
output under AC-1 above). Findings are not blanket-suppressed for dev-linked paths.

## AC-4

`dev_preen.txtar`, continued:

```
# running preen again is idempotent: identical output, links unchanged (AC-4).
> exec fledge preen
[stdout]
spec clean: 0 plumages, 0 feathers
> stdout 'spec clean'
> cp stdout $WORK/preen-run2.txt
> cmp $WORK/preen-run1.txt $WORK/preen-run2.txt
> exec readlink .fledge/skills/fledge-orchestrate/SKILL.md
[stdout]
$WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
> stdout $WORK/src/internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
> exec readlink .claude/agents/fledge-brooder.md
[stdout]
$WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
> stdout $WORK/src/internal/bootstrap/adapters/claude/agents/fledge-brooder.md
PASS
```

Two consecutive `fledge preen` runs produce byte-identical stdout (`cmp` passes), and
every dev link still resolves to its original target afterwards (`readlink` unchanged).
`preen` never writes to the stamp or the scaffold, so this holds by construction as well
as by this test.

## AC-5

`TestDriftReport` (the pre-existing table test, unchanged, still covering the no-dev
path — none of its cases set `Stamp.DevSource`) continues to pass byte-for-byte,
including its `"modified: disk differs from both stamp and expected"` case. The new
switch branch in `DriftReport` is guarded by `stamp.DevSource != ""`, so it is
unreachable for any of those cases; classification for a non-dev repo takes the exact
same code path as before this feather.

Command: `go test ./internal/bootstrap -run TestDriftReport$ -v`

```
=== RUN   TestDriftReport
=== RUN   TestDriftReport/up-to-date:_disk_matches_expected_bytes
=== RUN   TestDriftReport/stale:_disk_matches_stamp_hash_but_expected_has_moved
=== RUN   TestDriftReport/modified:_disk_differs_from_both_stamp_and_expected
=== RUN   TestDriftReport/missing:_file_absent_from_disk
=== RUN   TestDriftReport/obsolete:_in_stamp_but_not_in_expected
=== RUN   TestDriftReport/symlink_up-to-date:_readlink_matches_expected_target
=== RUN   TestDriftReport/symlink_stale:_disk_matches_stamp_target,_expected_has_moved
=== RUN   TestDriftReport/symlink_modified:_disk_differs_from_both_stamp_and_expected_targets
=== RUN   TestDriftReport/append_up-to-date:_all_lines_present
=== RUN   TestDriftReport/append_missing:_a_required_line_is_absent
=== RUN   TestDriftReport/no-stamp:_nil_stamp_sentinel_—_no_obsolete_entries,_all_disk-based
--- PASS: TestDriftReport (0.00s)
    (all subtests PASS)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.00s
```

Also added `TestDriftReportDevLink/non-dev_repo:...` as an explicit regression pin (see
AC-3).

## AC-6

`TestEditedOnRefreshOmitsDevLinks` (`internal/bootstrap/drift_test.go`): a dev-linked
repo whose linked file's live source content differs from the shipped bytes yields an
`EditedOnRefresh` result that does not contain that path — `EditedOnRefresh` derives
from `DriftReport` and only lists `StatusModified`/edited-`StatusObsolete` paths, so once
`DriftReport` stops classifying the dev-linked path as `modified`, it drops out
automatically. Failing/passing output above (AC-1).

## AC-7

Command: `go build ./... && go vet ./... && go test ./...`

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.114s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.008s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.130s
ok  	github.com/Harrison-Blair/fledge/internal/ledger	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.013s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.013s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.015s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.014s
```

All existing fixtures pass unmodified — no `.txtar` other than the newly added
`dev_preen.txtar` was touched, and no other Go test file changed besides
`internal/bootstrap/drift_test.go` (additions only, no existing case altered).
`gofmt -l .` reports no files needing formatting.
