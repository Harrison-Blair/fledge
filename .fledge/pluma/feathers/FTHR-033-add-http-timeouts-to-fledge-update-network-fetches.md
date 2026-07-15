---
id: FTHR-033
title: Add HTTP timeouts to fledge update network fetches
plumage: PLM-016
status: fledged
priority: P2
depends_on: []
authored: 2026-07-15T15:13:39Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-033: Add HTTP timeouts to fledge update network fetches

## Description
Bound the network operations in `fledge update`. Both `fetchLatestRelease` and
`downloadBytes` call the package-level `http.Get`, which uses `http.DefaultClient` with no
timeout, so a peer that accepts the connection but stalls hangs the command forever.
Introduce a client whose configuration fails a stalled request within a bounded time,
without truncating a legitimately large (but progressing) binary download.

Satisfies PLM-016 FC-4.

## Affected Modules
- `internal/cli/update.go` — `fetchLatestRelease` (line 113, `http.Get` at 115) and
  `downloadBytes` (line 157, `http.Get` at 158). See `.fledge/nest/modules.md → internal/cli`.

## Approach
1. Replace the two `http.Get` calls with a shared `*http.Client` configured so a
   connect-but-no-progress peer fails quickly while an in-progress download is not cut off
   by a wall-clock cap. Prefer a custom `http.Transport` with `DialContext` timeout,
   `TLSHandshakeTimeout`, and `ResponseHeaderTimeout` (catches stalls before/at the header)
   over a blunt total `Client.Timeout` that would also bound large-but-healthy downloads.
   A modest total `Timeout` on the small JSON metadata fetch is fine if simpler; the asset
   download should rely on the stall-oriented transport timeouts.
2. Make the client/timeouts injectable for testing — e.g. a package-level default client
   plus a test seam (an unexported variable or a parameter) so a test can install a
   very short timeout. `fetchLatestRelease` already takes an injectable `baseURL`; extend
   that seam so the download path is testable too.

Constraints: no change to the download/checksum/verify/atomic-swap logic — only how the
HTTP requests are issued. No retry/backoff (explicitly out of scope in PLM-016). Keep error
messages actionable (distinguish a timeout from an HTTP error status).

## Tests
Written test-first, failing before, passing after:
- `TestFetchLatestReleaseTimesOut` (`internal/cli`) — pins FC-4: stand up an `httptest`
  server whose handler blocks (e.g. sleeps past the configured timeout, or never writes the
  response header), point the fetch at it with a short injected timeout, and assert the call
  returns a (timeout) error within a bounded wall-clock margin rather than hanging. Against
  the current `http.Get`, the request would not time out (the test would hang / fail its
  deadline).
- `TestDownloadBytesTimesOut` (`internal/cli`) — same shape for the asset-download path.

Use the injected short timeout so the test is fast and deterministic; assert on bounded
elapsed time and an error, not on an exact message.

## Acceptance Criteria
- [x] AC-1: The tests above were observed failing (hang / no timeout) before implementation and pass after.
- [x] AC-2: Both `fetchLatestRelease` and `downloadBytes` issue requests through a client that fails a stalled peer within a bounded time (FC-4), with the timeout injectable for tests.
- [x] AC-3: No change to the download/checksum/swap behavior; a healthy (progressing) download is not truncated by a wall-clock cap.
- [x] AC-4: `fledge preen` passes and `go test ./...` is green.
