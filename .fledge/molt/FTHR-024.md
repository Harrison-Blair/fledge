# FTHR-024 evidence

## AC-1

Test written first (extending `internal/ciconfig/release_workflow_test.go` with
`TestReleaseWorkflow_BuildsAllFivePlatforms`, plus a `matrixIncludePairs` helper)
against the not-yet-widened `.github/workflows/release.yml` (linux/amd64 only,
no `strategy.matrix`).

Command:

```
go test ./internal/ciconfig/... -run TestReleaseWorkflow_BuildsAllFivePlatforms -v
```

Pre-implementation output (FAILING, expected reason — no `strategy.matrix.include`
in the workflow yet, so all 5 platform pairs are reported missing):

```
=== RUN   TestReleaseWorkflow_BuildsAllFivePlatforms
    release_workflow_test.go:180: matrix.include missing "linux/amd64"; entries seen: []map[string]interface {}(nil)
    release_workflow_test.go:180: matrix.include missing "linux/arm64"; entries seen: []map[string]interface {}(nil)
    release_workflow_test.go:180: matrix.include missing "darwin/amd64"; entries seen: []map[string]interface {}(nil)
    release_workflow_test.go:180: matrix.include missing "darwin/arm64"; entries seen: []map[string]interface {}(nil)
    release_workflow_test.go:180: matrix.include missing "windows/amd64"; entries seen: []map[string]interface {}(nil)
--- FAIL: TestReleaseWorkflow_BuildsAllFivePlatforms (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/ciconfig	0.001s
FAIL
```

Implementation: widened `.github/workflows/release.yml` — replaced the single
`build`-and-`release` job with a `build` job using `strategy: matrix: include:`
over the 5 `{goos, goarch}` pairs (linux/amd64, linux/arm64, darwin/amd64,
darwin/arm64, windows/amd64), each building with the same ldflag version
injection (`-X github.com/Harrison-Blair/fledge/internal/cli.binaryVersion=$VERSION`),
naming archives `fledge_<goos>_<goarch>.tar.gz` (`.zip` with `fledge.exe` inside
for windows), uploading each as a build artifact via `actions/upload-artifact@v4`.
A separate `release` job (`needs: build`) downloads all 5 artifacts via
`actions/download-artifact@v4` with `pattern: dist-*` / `merge-multiple: true`,
merges the 5 per-platform `checksums.txt` fragments into one combined
`checksums.txt`, and calls `gh release create` once with all 5 archives plus
the combined checksums file.

Also updated the pre-existing `TestReleaseWorkflow_BuildsLinuxAmd64AndReleases`
(same file, in this feather's Affected Modules): its literal `"GOOS=linux"` /
`"GOARCH=amd64"` substring checks no longer match now that the build step is
parameterized via `${{ matrix.goos }}` / `${{ matrix.goarch }}` per the spec's
Approach step 1 (replace the single build step with a matrix). Updated it to
assert a parameterized `GOOS=`/`GOARCH=` build step exists and that
`fledge_linux_amd64` is still referenced (now in the release job's asset list),
preserving the original intent (linux/amd64 still built and released) without
weakening the assertion.

Post-implementation output (PASSING), after widening `release.yml`:

```
=== RUN   TestReleaseWorkflow_TriggersOnPushToMain
--- PASS: TestReleaseWorkflow_TriggersOnPushToMain (0.00s)
=== RUN   TestReleaseWorkflow_RunsSafetyNet
--- PASS: TestReleaseWorkflow_RunsSafetyNet (0.00s)
=== RUN   TestReleaseWorkflow_BuildsLinuxAmd64AndReleases
--- PASS: TestReleaseWorkflow_BuildsLinuxAmd64AndReleases (0.00s)
=== RUN   TestReleaseWorkflow_BuildsAllFivePlatforms
--- PASS: TestReleaseWorkflow_BuildsAllFivePlatforms (0.00s)
=== RUN   TestPRCheckWorkflow_TriggersOnMainPRs
--- PASS: TestPRCheckWorkflow_TriggersOnMainPRs (0.00s)
=== RUN   TestPRCheckWorkflow_RunsLintBuildTest
--- PASS: TestPRCheckWorkflow_RunsLintBuildTest (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.002s
```

Full-repo safety net after the change (confirms no regressions):

```
$ gofmt -l . && go build ./... && go vet ./... && go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.086s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.002s
```

## AC-2 / AC-3

**Not yet gathered — requires a real, live GitHub Release.** Per the feather
spec's Tests section and this brooder's spawn instructions, AC-2 and AC-3 can
only be observed against the next real `VERSION`-bumping release on the live
repo after this feather merges (never a throwaway/reverted tag). That release
must be cut by team-lead/maintainer coordination, not by this brooder — this
brooder does not push tags or cut releases. Once that real release (v0.5.2)
exists:

- AC-2: inspect the release's assets — confirm exactly 5 binary archives
  (`fledge_linux_amd64.tar.gz`, `fledge_linux_arm64.tar.gz`,
  `fledge_darwin_amd64.tar.gz`, `fledge_darwin_arm64.tar.gz`,
  `fledge_windows_amd64.zip`) plus one `checksums.txt` covering all 5.
- AC-3: download and verify each archive's checksum against `checksums.txt`
  (`sha256sum -c checksums.txt`), and spot-check at least `linux_amd64` and one
  other platform's binary reports the correct version.
