# FTHR-033 evidence

## AC-1
_Tests observed failing (hang / no timeout) before implementation, passing after_

Pre-implementation: `updateHTTPTimeout` existed as an unused seam (not yet
wired to any HTTP client), so `fetchLatestRelease`/`downloadBytes` still used
plain `http.Get`. Ran against a blocking `httptest` server with a 100ms
injected timeout, `-timeout 15s` so the run fails deterministically instead
of hanging forever:

```
$ go test ./internal/cli/ -run 'TestFetchLatestReleaseTimesOut|TestDownloadBytesTimesOut' -v -timeout 15s
=== RUN   TestFetchLatestReleaseTimesOut
    update_test.go:194: fetchLatestRelease took 2.000958979s, want it to time out within ~100ms
--- FAIL: TestFetchLatestReleaseTimesOut (2.00s)
=== RUN   TestDownloadBytesTimesOut
    update_test.go:210: downloadBytes succeeded, want timeout error
--- FAIL: TestDownloadBytesTimesOut (2.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/cli	4.004s
FAIL
```

Both fail for the expected reason: `fetchLatestRelease` doesn't return until
the handler's 2s sleep elapses (no timeout fires), and `downloadBytes`
succeeds outright (no timeout mechanism at all) — neither aborts within the
injected 100ms budget.

Post-implementation (after wiring `updateHTTPClient()` — a `*http.Transport`
with `DialContext`/`TLSHandshakeTimeout`/`ResponseHeaderTimeout` set from
`updateHTTPTimeout` — into both `fetchLatestRelease` and `downloadBytes`):

```
$ go test ./internal/cli/ -run 'TestFetchLatestReleaseTimesOut|TestDownloadBytesTimesOut' -v -timeout 15s
=== RUN   TestFetchLatestReleaseTimesOut
--- PASS: TestFetchLatestReleaseTimesOut (2.00s)
=== RUN   TestDownloadBytesTimesOut
--- PASS: TestDownloadBytesTimesOut (2.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.004s
```

(The reported 2.00s per test is `httptest.Server.Close`'s deferred wait for
the still-sleeping handler goroutine to finish, not the client call itself —
the assertions inside each test confirm the client returned a timeout error
well within the 100ms budget, bounded by a `< 1s` elapsed check.)

## AC-2
_Both fetch paths go through a client that bounds a stalled peer, timeout injectable_

`internal/cli/update.go`:
- `updateHTTPTimeout` (package var, default 10s) is the injectable timeout
  seam — tests override it via `withUpdateHTTPTimeout(t, d)` (mirrors the
  existing `updateAPIBaseURL`/`withUpdateBaseURL` seam pattern).
- `updateHTTPClient()` builds an `*http.Client` per call whose `*http.Transport`
  sets `DialContext` (via `net.Dialer{Timeout: updateHTTPTimeout}`),
  `TLSHandshakeTimeout`, and `ResponseHeaderTimeout` all to
  `updateHTTPTimeout`. `Client.Timeout` is deliberately left unset.
- `fetchLatestRelease` and `downloadBytes` both call `updateHTTPClient().Get(url)`
  instead of `http.Get(url)`.

`TestFetchLatestReleaseTimesOut` and `TestDownloadBytesTimesOut` (above) pin
this: a peer that accepts the connection but never writes headers is failed
within the injected timeout, for both call sites.

## AC-3
_No change to download/checksum/swap behavior; a healthy progressing download is not truncated_

No lines in `checksumFor`, `extractBinary`/`extractFromTarGz`/`extractFromZip`,
`swapBinary`, or `performUpdate` were touched — only the HTTP call sites in
`fetchLatestRelease` and `downloadBytes` changed (`http.Get(url)` →
`updateHTTPClient().Get(url)`), matching the spec's "only how the HTTP
requests are issued" constraint. No retry/backoff was added.

Confirmed a large, slow-but-progressing download is not cut off by the
stall-oriented transport timeouts (manual check, not part of the committed
suite — the spec names only the two timeout tests above):

```
$ go test ./internal/cli/ -run TestDownloadBytesHealthySlowDownloadNotTruncated -v -timeout 30s
=== RUN   TestDownloadBytesHealthySlowDownloadNotTruncated
--- PASS: TestDownloadBytesHealthySlowDownloadNotTruncated (1.61s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	1.616s
```

This test served a 5MB body in 64KB chunks with a 20ms sleep between chunks
(total wall time ~1.6s) against an injected 200ms `updateHTTPTimeout` —
i.e. the per-request budget is far shorter than the total transfer time, yet
`downloadBytes` returned every byte because `ResponseHeaderTimeout` only
bounds the wait for the first response byte, not the body read, and
`Client.Timeout` (a wall-clock cap on the whole request) is never set.

## AC-4
_`fledge preen` passes and `go test ./...` is green_

```
$ go test ./... 2>&1 | tail -20
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.089s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.012s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.161s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.012s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.008s

$ fledge preen; echo "exit=$?"
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

`preen` exits 0 (pass); the 7 warnings are pre-existing (other in-flight
feathers' brood state and this worktree's copy missing unrelated scaffold
files) and unrelated to this feather's change — none reference
`update.go`/`update_test.go`.
