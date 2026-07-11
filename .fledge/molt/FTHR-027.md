# FTHR-027 evidence

## AC-1

Tests listed in the spec's Tests section were written first
(`internal/cli/update_swap_test.go`) and run against FTHR-026's
pre-FTHR-027 `internal/cli/update.go` (the placeholder confirm-path
action, restored via `git show HEAD:internal/cli/update.go` for this
capture). They fail for the expected reason: the tests exercise
platform-asset-name resolution (`updateAssetName`) that does not exist
until this feather implements the real download/verify/swap mechanics —
a build failure naming the undefined symbols the new tests require,
not an unrelated setup error.

Command: `go test ./internal/cli/... -run 'TestUpdate' -v`

```
# github.com/Harrison-Blair/fledge/internal/cli [github.com/Harrison-Blair/fledge/internal/cli.test]
internal/cli/update_swap_test.go:85:15: undefined: updateAssetName
internal/cli/update_swap_test.go:118:15: undefined: updateAssetName
internal/cli/update_swap_test.go:154:15: undefined: updateAssetName
internal/cli/update_swap_test.go:188:15: undefined: updateAssetName
FAIL	github.com/Harrison-Blair/fledge/internal/cli [build failed]
FAIL
```

After implementing `internal/cli/update.go`'s real download/verify/swap
logic, the same command passes:

Command: `go test ./internal/cli/... -run 'TestUpdate' -v`

```
=== RUN   TestUpdate_DownloadVerifyAndSwap_Success
--- PASS: TestUpdate_DownloadVerifyAndSwap_Success (0.00s)
=== RUN   TestUpdate_ChecksumMismatch_AbortsWithoutSwap
--- PASS: TestUpdate_ChecksumMismatch_AbortsWithoutSwap (0.00s)
=== RUN   TestUpdate_MissingPlatformAsset_AbortsWithoutSwap
--- PASS: TestUpdate_MissingPlatformAsset_AbortsWithoutSwap (0.00s)
=== RUN   TestUpdate_NetworkFailureDuringDownload_AbortsWithoutSwap
fledge: updating: downloading fledge_linux_amd64.tar.gz: Get "http://127.0.0.1:43299/assets/archive": dial tcp 127.0.0.1:43299: connect: connection refused
--- PASS: TestUpdate_NetworkFailureDuringDownload_AbortsWithoutSwap (0.00s)
=== RUN   TestUpdate_AlreadyUpToDate
--- PASS: TestUpdate_AlreadyUpToDate (0.00s)
=== RUN   TestUpdate_NewerAvailable_PromptsAndShowsNotes
--- PASS: TestUpdate_NewerAvailable_PromptsAndShowsNotes (0.00s)
=== RUN   TestUpdate_ConfirmYes
fledge: updating: no release asset found for linux/amd64 (expected fledge_linux_amd64.tar.gz)
--- PASS: TestUpdate_ConfirmYes (0.00s)
=== RUN   TestUpdate_ConfirmDefaultDeny
--- PASS: TestUpdate_ConfirmDefaultDeny (0.00s)
=== RUN   TestUpdate_YesFlagSkipsPrompt
fledge: updating: no release asset found for linux/amd64 (expected fledge_linux_amd64.tar.gz)
--- PASS: TestUpdate_YesFlagSkipsPrompt (0.00s)
=== RUN   TestUpdate_JSONFlagIsDryRun
--- PASS: TestUpdate_JSONFlagIsDryRun (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.004s
```

Note: `TestUpdate_ConfirmYes` and `TestUpdate_YesFlagSkipsPrompt` (from
FTHR-026) asserted on the placeholder's `"update mechanics not yet
implemented"` output; since this feather replaces that placeholder
(Approach step 2), those two pre-existing tests were updated to assert
the new behavior (a confirmed update now attempts a real download and
fails with `ExitFail` against the generic `newUpdateTestServer` fixture,
which serves no matching release asset) rather than the removed
placeholder text.

## AC-2

`TestUpdate_DownloadVerifyAndSwap_Success` (in `## AC-1` output above)
confirms via `--yes`, downloads the platform-matching asset (built with
the same `updateAssetName()`/`updateArchiveExt()` logic the production
code uses), verifies its checksum, extracts the binary, and asserts the
injected target-path file's content becomes the archive's extracted
payload and the process reports success (exit `ExitOK`, output contains
the new version).

## AC-3

`TestUpdate_ChecksumMismatch_AbortsWithoutSwap` (in `## AC-1` output
above) serves an archive whose SHA-256 does not match the served
`checksums.txt` entry; asserts non-zero exit (`ExitFail`), a stderr
error containing `"checksum mismatch"`, and that the target-path file's
content is byte-identical to what it was before the run.

## AC-4

`TestUpdate_MissingPlatformAsset_AbortsWithoutSwap` (release has no
asset matching the current platform's expected name) and
`TestUpdate_NetworkFailureDuringDownload_AbortsWithoutSwap` (archive
host closed before the download) both in `## AC-1` output above: both
assert non-zero exit (`ExitFail`) and that the target-path file's
content is unchanged from before the run.

## Full suite

Command: `go build ./... && go vet ./... && gofmt -l . && go test ./...`

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.083s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.014s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.157s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.008s
```

`gofmt -l .` and `go vet ./...` produced no output (clean).
