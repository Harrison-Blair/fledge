# FTHR-043 evidence — Version-consistency test covers stamp_warning.txtar

Feather: `.fledge/pluma/feathers/FTHR-043-version-consistency-test-covers-stamp-warning-txtar.md`
Branch: `feather/FTHR-043-version-consistency-stamp-warning`

## AC-1

_Tests observed failing before implementation (deliberate-divergence step) and pass after; evidence captured verbatim_

New test: `TestStampWarningTxtarVersionMatchesBinary` in `internal/cli/version_test.go`.

Per the feather's Tests section, the fixture is already correct in committed state, so the "observed failing" evidence is captured by temporarily editing the fixture's pinned version to a divergent value (`9.9.9`), running the test, then reverting.

### Step 1 — test passes against the current (correct) fixture

```
$ go test ./internal/cli/ -run TestStampWarningTxtarVersionMatchesBinary -v
=== RUN   TestStampWarningTxtarVersionMatchesBinary
--- PASS: TestStampWarningTxtarVersionMatchesBinary (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.001s
```

### Step 2 — deliberately diverge the fixture, test FAILS for the expected reason

Edited `cmd/fledge/testdata/stamp_warning.txtar` line 9, changing `binary is 0\.5\.5`
to `binary is 9\.9\.9`:

```
$ go test ./internal/cli/ -run TestStampWarningTxtarVersionMatchesBinary -v
=== RUN   TestStampWarningTxtarVersionMatchesBinary
    version_test.go:39: stamp_warning.txtar pinned version = "9.9.9", binaryVersion = "0.5.5" — bump cmd/fledge/testdata/stamp_warning.txtar
--- FAIL: TestStampWarningTxtarVersionMatchesBinary (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/cli	0.001s
FAIL
```

The failure is the wrong-version message naming the txtar file — exactly the divergence this
test exists to catch. The test cannot pass while the fixture disagrees with `binaryVersion`.

### Step 3 — reverted the fixture edit

Reverted line 9 back to `binary is 0\.5\.5`. `git status --short` then shows only
`internal/cli/version_test.go` modified (fixture unchanged from committed state):

```
$ git status --short
 M internal/cli/version_test.go
```

## AC-2

_`go test ./internal/cli/...` passes, including the new test, at rest (fixture reverted) — satisfies PLM-023 FC-1, AC-1_

```
$ go test ./internal/cli/...
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.009s
```

Supplementary CI-parity checks:

```
$ gofmt -l internal/cli/version_test.go
(no output — formatting clean)

$ go vet ./internal/cli/
vet-ok
```
