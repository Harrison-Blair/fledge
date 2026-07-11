---
id: FTHR-027
title: "fledge update — download, checksum verify, and atomic binary swap"
plumage: PLM-014
status: hatching
priority: P1
depends_on: [FTHR-026]
authored: 2026-07-11T06:06:56Z
agent: fledge-orchestrate/planning
fledge_version: 0.4.0
---

# FTHR-027: fledge update — download, checksum verify, and atomic binary swap

## Description
Replace FTHR-026's placeholder confirm-path action with the real update mechanics: download the release asset matching the running platform, verify its SHA-256 against the release's `checksums.txt`, and atomically replace the currently-running binary. Completes PLM-014 end-to-end.

## Affected Modules
Per PLM-012's established asset-naming convention (`fledge_<GOOS>_<GOARCH>[.exe]`, `checksums.txt`) and `.fledge/nest/conventions.md`'s atomic-write discipline (`spec.WriteFileAtomic`: temp file + `os.Rename`):
- Modified: `internal/cli/update.go` — replaces FTHR-026's placeholder confirm-path branch with real logic.
- New: additional test cases in `internal/cli/update_test.go` (or a new `internal/cli/update_swap_test.go` if that keeps the file more manageable).
- No new external dependency: archive extraction uses stdlib `archive/tar` + `compress/gzip` (for `.tar.gz` platforms) and `archive/zip` (for the Windows `.zip`); checksum verification uses stdlib `crypto/sha256`.

## Approach
1. Add a target-path seam: an unexported var/param defaulting to `os.Executable()`'s result, overridable in tests to point at a throwaway temp file instead of the real test binary.
2. On confirm (prompt-yes or `--yes`), using the already-fetched release's `Assets[]`: find the entry whose name matches `fledge_<runtime.GOOS>_<runtime.GOARCH>` (with the platform's expected archive extension); if none matches, print an error naming the platform and exit non-zero — no changes made.
3. Download that asset and the release's `checksums.txt` asset (both via the injectable HTTP client/base-URL seam from FTHR-026) to temp files.
4. Extract the binary from the downloaded archive to another temp file; compute its SHA-256; compare against the matching line in `checksums.txt` (matched by the archive's filename). On mismatch: print an error, delete the temp files, exit non-zero — the target path is never touched.
5. On match: write the extracted binary to a temp file in the same directory as the target path (required for `os.Rename` to be atomic on the same filesystem), `chmod` it executable (mode matching the target's current mode, or `0755`), then `os.Rename` the temp file over the target path. If any step above this point fails, the target path is provably untouched (rename is the only mutating step and it's atomic).
6. Print a success message naming the new version once the rename completes.

## Tests
Extending `internal/cli/update_test.go` (or a new file), using `httptest.NewServer` serving a small fake archive + matching/mismatched `checksums.txt` (no real network, no real archive/binary needed — a tiny fake payload suffices since only bytes-in/bytes-out and checksum matching are being tested), written first against the not-yet-implemented download/verify/swap (fails: FTHR-026's placeholder runs instead), then passing once implemented:
- `TestUpdate_DownloadVerifyAndSwap_Success` — server serves a valid archive + matching checksum; confirm via `--yes`; assert the injected target-path temp file's content becomes the fake payload's extracted content, and the process reports success. Pins FC-7, FC-8.
- `TestUpdate_ChecksumMismatch_AbortsWithoutSwap` — server serves an archive whose checksum doesn't match `checksums.txt`; confirm via `--yes`; assert non-zero exit, an error naming the mismatch, and the target-path temp file's content is byte-identical to before the run. Pins FC-7, FC-9.
- `TestUpdate_MissingPlatformAsset_AbortsWithoutSwap` — server's release has no asset matching a deliberately-unsupported fake `GOOS`/`GOARCH` combo (test overrides the runtime platform lookup, if feasible, or simulates via a controlled assets list); assert non-zero exit, no swap. Pins FC-9.
- `TestUpdate_NetworkFailureDuringDownload_AbortsWithoutSwap` — server closed/unreachable mid-flow; assert non-zero exit, target path untouched. Pins FC-9.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after.
- [ ] AC-2: Confirming an update (prompt-yes or `--yes`) downloads the correct platform asset, verifies its checksum, and replaces the target binary such that its content is the newly downloaded one (satisfies PLM-014 AC-4).
- [ ] AC-3: A checksum mismatch aborts with an error and leaves the target binary provably unchanged (satisfies PLM-014 AC-5).
- [ ] AC-4: Any failure before the final rename (missing platform asset, network failure) aborts with a non-zero exit and leaves the target binary unchanged (satisfies PLM-014 FC-9).
