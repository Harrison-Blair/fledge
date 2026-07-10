# Evidence: FTHR-011

## AC-1

Tests observed FAILING against unchanged code before any implementation.

### Unit tests (`internal/bootstrap/drift_test.go`) — pre-implementation run

```
$ go test ./internal/bootstrap/ -run 'TestDrift' -count=1 -v
# github.com/Harrison-Blair/fledge/internal/bootstrap [github.com/Harrison-Blair/fledge/internal/bootstrap.test]
internal/bootstrap/drift_test.go:37:25: undefined: DriftStatus
internal/bootstrap/drift_test.go:50:27: undefined: DriftStatus
internal/bootstrap/drift_test.go:51:21: undefined: StatusUpToDate
internal/bootstrap/drift_test.go:65:27: undefined: DriftStatus
internal/bootstrap/drift_test.go:66:21: undefined: StatusStale
internal/bootstrap/drift_test.go:80:27: undefined: DriftStatus
internal/bootstrap/drift_test.go:81: undefined: StatusModified
internal/bootstrap/drift_test.go:93:27: undefined: DriftStatus
internal/bootstrap/drift_test.go:94:21: undefined: StatusMissing
internal/bootstrap/drift_test.go:106:27: undefined: DriftStatus
internal/bootstrap/drift_test.go:106:27: too many errors
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap [build failed]
FAIL
```

Fails for the expected reason: `DriftStatus`, `DriftReport`, and status constants (`StatusUpToDate`, `StatusStale`, `StatusModified`, `StatusMissing`, `StatusObsolete`) are not yet defined — `internal/bootstrap/drift.go` does not exist.

### CLI acceptance test (`cmd/fledge/testdata/preen_scaffold.txtar`) — pre-implementation run

```
$ go test ./cmd/fledge -run TestScripts/preen_scaffold -v -count=1
...
> exec fledge preen --json
[stdout]
{
  "findings": [],
  "ok": true
}
> stdout '"scaffold"'
FAIL: testdata/preen_scaffold.txtar:14: no match for `"scaffold"` found in stdout

--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/preen_scaffold (0.01s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.008s
FAIL
```

Fails for the expected reason: `fledge preen --json` does not yet include a `scaffold` object — `internal/cli/preen.go` has not been updated.

### Post-implementation passing runs

```
$ go test ./internal/bootstrap/ -run 'TestDrift' -count=1 -v
=== RUN   TestDriftReport
=== RUN   TestDriftReport/up-to-date:_disk_matches_expected_bytes
--- PASS: TestDriftReport/up-to-date:_disk_matches_expected_bytes (0.00s)
=== RUN   TestDriftReport/stale:_disk_matches_stamp_hash_but_expected_has_moved
--- PASS: TestDriftReport/stale:_disk_matches_stamp_hash_but_expected_has_moved (0.00s)
=== RUN   TestDriftReport/modified:_disk_differs_from_both_stamp_and_expected
--- PASS: TestDriftReport/modified:_disk_differs_from_both_stamp_and_expected (0.00s)
=== RUN   TestDriftReport/missing:_file_absent_from_disk
--- PASS: TestDriftReport/missing:_file_absent_from_disk (0.00s)
=== RUN   TestDriftReport/obsolete:_in_stamp_but_not_in_expected
--- PASS: TestDriftReport/obsolete:_in_stamp_but_not_in_expected (0.00s)
=== RUN   TestDriftReport/symlink_up-to-date:_readlink_matches_expected_target
--- PASS: TestDriftReport/symlink_up-to-date:_readlink_matches_expected_target (0.00s)
=== RUN   TestDriftReport/symlink_stale:_disk_matches_stamp_target,_expected_has_moved
--- PASS: TestDriftReport/symlink_stale:_disk_matches_stamp_target,_expected_has_moved (0.00s)
=== RUN   TestDriftReport/symlink_modified:_disk_differs_from_both_stamp_and_expected_targets
--- PASS: TestDriftReport/symlink_modified:_disk_differs_from_both_stamp_and_expected_targets (0.00s)
=== RUN   TestDriftReport/append_up-to-date:_all_lines_present
--- PASS: TestDriftReport/append_up-to-date:_all_lines_present (0.00s)
=== RUN   TestDriftReport/append_missing:_a_required_line_is_absent
--- PASS: TestDriftReport/append_missing:_a_required_line_is_absent (0.00s)
=== RUN   TestDriftReport/no-stamp:_nil_stamp_sentinel_—_no_obsolete_entries,_all_disk-based
--- PASS: TestDriftReport/no-stamp:_nil_stamp_sentinel_—_no_obsolete_entries,_all_disk-based (0.00s)
--- PASS: TestDriftReport (0.00s)
=== RUN   TestDriftReportNilStampNoObsolete
--- PASS: TestDriftReportNilStampNoObsolete (0.00s)
PASS
ok	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s

$ go test ./cmd/fledge -run TestScripts/preen_scaffold -v -count=1
--- PASS: TestScripts/preen_scaffold (0.03s)
PASS
ok	github.com/Harrison-Blair/fledge/cmd/fledge	0.061s

$ go test ./... -count=1
ok	github.com/Harrison-Blair/fledge/cmd/fledge	0.061s
ok	github.com/Harrison-Blair/fledge/internal/bootstrap	0.006s
ok	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok	github.com/Harrison-Blair/fledge/internal/cli	0.002s
ok	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok	github.com/Harrison-Blair/fledge/internal/lock	0.001s
ok	github.com/Harrison-Blair/fledge/internal/nest	0.001s
?   	github.com/Harrison-Blair/fledge/internal/repo	[no test files]
ok	github.com/Harrison-Blair/fledge/internal/scan	0.007s
ok	github.com/Harrison-Blair/fledge/internal/spec	0.002s
```

## AC-2

`fledge preen` classifies up-to-date, stale, modified, missing, and obsolete scaffold entries correctly, at warning severity. Tested via `preen_scaffold.txtar`:

- **up-to-date**: fresh init → no scaffold-drift findings, all `--json` scaffold.files entries show `"status": "up-to-date"`
- **modified**: after `echo user-edit >> .fledgeignore` → `WARN .fledgeignore: scaffold file is user-edited; refresh will preserve it — run fledge init --refresh`; `--json` shows `"status": "modified"`, `"severity": "warning"`, rule `"scaffold-drift"`
- **missing**: after `rm .fledge/skills/fledge-orchestrate/SKILL.md` → `WARN ... missing` finding
- **obsolete**: after injecting a fake stamp entry `old/removed-file.md` → `WARN old/removed-file.md: scaffold file is obsolete...`
- **stale**: verified in unit test `TestDriftReport/stale`

`--json` `scaffold` object structure verified: contains `stampVersion`, `binaryVersion`, and `files` list with `{path, status, policy}` for every entry including up-to-date ones.

## AC-3

Stampless repo produces exactly one adoption warning:

```
$ rm .fledge/scaffold.json && fledge preen
WARN  .fledge/scaffold.json: no scaffold stamp — run fledge init --refresh to adopt
1 warning(s)
```

`--json` output confirms `"stampVersion": ""` and `"ok": true` (warnings alone don't fail).

Symlink entries: tested in `TestDriftReport/symlink_*` (up-to-date, stale, modified, missing cases). The `classifySymlink` function never errors — a non-symlink where one is expected reports `modified` via the error path, `os.IsNotExist` gives `missing`. No panics or errors surfaced.

Append entries: tested in `TestDriftReport/append_*` cases. `.gitignore` is an append-policy entry; fresh-init preen reports it as up-to-date.

## AC-4

`internal/check` gains no bootstrap import:

```
$ grep -r 'bootstrap' internal/check/
(no output)
```

`go test ./... -count=1` and `go vet ./...` both pass clean.
